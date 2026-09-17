package assess

import (
	"encoding/json"
	"os"
	"reflect"
	"strings"
	"testing"

	tfjson "github.com/hashicorp/terraform-json"
)

// Failed checks, from REAL Terraform output.
//
// The roadmap said in bold: do not build this before the fixture exists.
// testdata/real-checks.json is a genuine `terraform show -json` from a
// throwaway local root - terraform_data and nothing else, no cloud, no
// credential - carrying a check block that fails at plan time and a resource
// postcondition expanded over for_each.
//
// What that plan established, and none of it was guessable from the
// specification alone:
//
//   - A standalone check block DOES appear in plan JSON, with kind "check".
//     The roadmap's open question was whether it would, and said that if it
//     did this would be the most valuable thing in the layer.
//   - It can FAIL at plan time. Terraform reports that as a Warning and the
//     plan still succeeds, so CI passes over it with exit 0 - which is the
//     whole reason to report it.
//   - A resource postcondition appears too, with kind "resource", one instance
//     per expanded object: terraform_data.app["production"] and ["staging"].
//   - A failing instance carries problems[].message, and that message is
//     author-written text Terraform INTERPOLATES. A separate plan proved it
//     can contain a live credential, which is why nothing here prints it.

func realChecks(t *testing.T, name string) *tfjson.Plan {
	t.Helper()
	b, err := os.ReadFile("../../testdata/" + name)
	if err != nil {
		t.Fatalf("committed fixture %s is missing: %v", name, err)
	}
	var p tfjson.Plan
	if err := json.Unmarshal(b, &p); err != nil {
		t.Fatalf("fixture %s did not parse: %v", name, err)
	}
	return &p
}

func TestAFailedCheckIsReported(t *testing.T) {
	r := Assess(realChecks(t, "real-checks.json"))

	if len(r.Checks) != 1 {
		t.Fatalf("got %d check findings, want 1 - only budget_is_set fails: %+v", len(r.Checks), r.Checks)
	}
	c := r.Checks[0]
	if c.Address != "check.budget_is_set" {
		t.Errorf("Address = %q", c.Address)
	}
	if c.Kind != CheckKindCheckBlock {
		t.Errorf("Kind = %q, want %q", c.Kind, CheckKindCheckBlock)
	}
	if c.Status != "fail" {
		t.Errorf("Status = %q, want fail", c.Status)
	}
	if c.Problems != 1 {
		t.Errorf("Problems = %d, want 1", c.Problems)
	}
}

// THE MESSAGE IS NEVER PRINTED, AND NEVER CARRIED. error_message is written by
// whoever wrote the configuration and Terraform interpolates it, so it can
// hold an attribute value - a plan generated for this proved it can hold a
// live GitHub token. The report says a check failed and how many problems it
// had; the reader gets the message from the plan output, where they already
// have it.
func TestACheckMessageNeverReachesTheReport(t *testing.T) {
	p := realChecks(t, "real-checks.json")
	// The message this fixture actually carries.
	const message = "more environments are configured than the budget allows"

	raw, err := json.Marshal(Assess(p))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), message) {
		t.Errorf("a check's error_message reached the report:\n%s", raw)
	}
	// And the type has nowhere to put one.
	for _, f := range reflectFields(CheckFinding{}) {
		if strings.Contains(strings.ToLower(f), "message") {
			t.Errorf("CheckFinding has a %s field, which is somewhere a message could land", f)
		}
	}
}

// A resource condition is reported under the resource it belongs to, and a
// check block on its own terms. They are different things and a reader needs
// to tell them apart.
//
// IT IS NOT CALLED A POSTCONDITION. Terraform aggregates a resource's
// preconditions AND postconditions under one checkable object and the JSON
// does not say which of them failed, so naming one would be a claim the file
// cannot substantiate.
func TestAResourceConditionIsDistinguishedFromACheckBlock(t *testing.T) {
	p := realChecks(t, "real-checks-undetermined.json")
	r := Assess(p)

	byAddress := map[string]CheckFinding{}
	for _, c := range r.Checks {
		byAddress[c.Address] = c
	}

	post, ok := byAddress["terraform_data.with_postcondition"]
	if !ok {
		t.Fatalf("the postcondition was not reported: %+v", r.Checks)
	}
	if post.Kind != CheckKindResourceCondition {
		t.Errorf("Kind = %q, want %q", post.Kind, CheckKindResourceCondition)
	}

	block, ok := byAddress["check.depends_on_unknown"]
	if !ok {
		t.Fatalf("the check block was not reported: %+v", r.Checks)
	}
	if block.Kind != CheckKindCheckBlock {
		t.Errorf("Kind = %q, want %q", block.Kind, CheckKindCheckBlock)
	}
}

// PASSING CHECKS ARE NOT REPORTED. A report listing everything that went right
// is a report nobody reads to the end, and terraken says what needs attention.
// Undetermined ones ARE reported, because "this could not be checked" is the
// kind of thing this tool exists to say.
func TestOnlyFailedAndUndeterminedChecksAreReported(t *testing.T) {
	r := Assess(realChecks(t, "real-checks.json"))
	for _, c := range r.Checks {
		if c.Status == "pass" {
			t.Errorf("%s passed and was reported anyway", c.Address)
		}
	}
	// The fixture has two passing checks and one failing one.
	if len(r.Checks) != 1 {
		t.Errorf("got %d reported, want 1: %+v", len(r.Checks), r.Checks)
	}

	undetermined := Assess(realChecks(t, "real-checks-undetermined.json"))
	var unknowns int
	for _, c := range undetermined.Checks {
		if c.Status == "unknown" {
			unknowns++
		}
	}
	if unknowns == 0 {
		t.Error("an undetermined check must be reported: not knowing is the point")
	}
}

// The instance address is the expanded one where there is one. A postcondition
// over for_each gives terraform_data.app["production"], which is what a reader
// needs to find the object.
func TestAnExpandedCheckReportsItsInstanceAddress(t *testing.T) {
	p := realChecks(t, "real-checks.json")
	// Make the expanded postcondition fail so it is reported.
	for i := range p.Checks {
		if p.Checks[i].Address.Kind != "resource" {
			continue
		}
		for j := range p.Checks[i].Instances {
			p.Checks[i].Instances[j].Status = tfjson.CheckStatusFail
		}
	}

	var addresses []string
	for _, c := range Assess(p).Checks {
		addresses = append(addresses, c.Address)
	}
	joined := strings.Join(addresses, " ")
	for _, want := range []string{`terraform_data.app["production"]`, `terraform_data.app["staging"]`} {
		if !strings.Contains(joined, want) {
			t.Errorf("expected the expanded instance %q in %v", want, addresses)
		}
	}
}

// Checks are not findings and not in the counts. A failed check is not a
// resource change, and `critical` in the counts means one resource change
// losing data.
// Checks change NOTHING about the rest of the assessment.
//
// Asserting "no critical count" was far too weak - a check added as a high, a
// low or an info finding would have passed it, and one line compared a value
// with itself and could not fail at all. This compares the whole assessment
// with the checks present against the same plan with them removed: findings,
// both count maps, Max, Shape and drift all have to be identical.
func TestChecksChangeNothingElseInTheAssessment(t *testing.T) {
	withChecks := realChecks(t, "real-checks.json")
	r := Assess(withChecks)
	if len(r.Checks) == 0 {
		t.Fatal("no checks reported, so this proves nothing")
	}

	without := realChecks(t, "real-checks.json")
	without.Checks = nil
	bare := Assess(without)

	if len(bare.Checks) != 0 {
		t.Fatal("the control still has checks")
	}
	compare := func(name string, a, b interface{}) {
		t.Helper()
		x, err := json.Marshal(a)
		if err != nil {
			t.Fatal(err)
		}
		y, err := json.Marshal(b)
		if err != nil {
			t.Fatal(err)
		}
		if string(x) != string(y) {
			t.Errorf("%s changed when checks were present:\n%s\n%s", name, x, y)
		}
	}
	compare("findings", r.Findings, bare.Findings)
	compare("counts", r.CountsByName, bare.CountsByName)
	compare("shape", r.Shape, bare.Shape)
	compare("drift", r.Drift, bare.Drift)

	maxA, anyA := r.Max()
	maxB, anyB := bare.Max()
	if maxA != maxB || anyA != anyB {
		t.Errorf("Max() = %v/%v with checks and %v/%v without", maxA, anyA, maxB, anyB)
	}
	if r.Unassessed != bare.Unassessed {
		t.Errorf("Unassessed = %d with checks and %d without", r.Unassessed, bare.Unassessed)
	}
}

// Every undetermined check is reported, not just one of them.
func TestEveryUndeterminedCheckIsReported(t *testing.T) {
	p := realChecks(t, "real-checks-undetermined.json")

	var want int
	for _, c := range p.Checks {
		for _, i := range c.Instances {
			if i.Status == tfjson.CheckStatusUnknown {
				want++
			}
		}
	}
	if want < 2 {
		t.Fatalf("the fixture holds %d undetermined instances, which is too few to prove this", want)
	}

	var got int
	for _, c := range Assess(p).Checks {
		if c.Status == "unknown" {
			got++
		}
	}
	if got != want {
		t.Errorf("reported %d undetermined checks, want %d", got, want)
	}
}

// A status this build does not recognise is said to be unrecognised, and is
// NOT interpolated into the tool's own sentence. Terraform emits a fixed
// vocabulary, but Report is built from a file this package does not validate,
// and a status read out of one is plan-derived text.
func TestAnUnrecognisedCheckStatusIsNotQuotedBackIntoTheSentence(t *testing.T) {
	p := realChecks(t, "real-checks.json")
	const planted = "ghp_R7tQm2xLvB9nKpZa4WcYeD6sJhF1gU3oNi0T"
	for i := range p.Checks {
		for j := range p.Checks[i].Instances {
			p.Checks[i].Instances[j].Status = tfjson.CheckStatus(planted)
		}
	}

	for _, c := range Assess(p).Checks {
		if strings.Contains(c.Detail, planted) {
			t.Errorf("a status from the plan was quoted into the tool's own sentence: %q", c.Detail)
		}
		if !strings.Contains(c.Detail, "does not recognise") {
			t.Errorf("Detail = %q, want it to say the status is unrecognised", c.Detail)
		}
	}
}

// reflectFields names a struct's fields, so a test can assert that a type has
// nowhere to put something.
func reflectFields(v interface{}) []string {
	rt := reflect.TypeOf(v)
	out := make([]string, 0, rt.NumField())
	for i := 0; i < rt.NumField(); i++ {
		out = append(out, rt.Field(i).Name)
	}
	return out
}
