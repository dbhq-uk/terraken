package assess

import (
	"encoding/json"
	"os"
	"regexp"
	"strings"
	"testing"

	tfjson "github.com/hashicorp/terraform-json"
)

// What each resource actually changes, at every level.
//
// The report ranked a change and named the attributes that FORCED it, but
// never said what the change touches. A reviewer reading "update in place" with
// one reordering annotation under it has no idea whether the plan is editing a
// tag or a security rule.
//
// ATTRIBUTE NAMES ARE NOT VALUES, which is why this is allowed at all.
// AGENTS.md is explicit: paths, counts, levels and the tool's own sentences are
// what it may show. Every name here comes from the plan's attribute keys and no
// value is read to produce them.
//
// TOP-LEVEL ONLY. A change to tags.owner is reported as `tags`. The roll-up in
// rewritten.go already counts top-level attributes for the same reason - it is
// what a reader sees the plan show as changed - and the nested paths that
// matter are already named by the annotations that care about them: what forced
// a replacement, what is unknown until apply, what is sensitive.

func changedFor(t *testing.T, r Report, addr string) *Annotation {
	t.Helper()
	f := findingFor(t, r, addr)
	for i := range f.Annotations {
		if f.Annotations[i].Code == AnnChangedAttributes {
			return &f.Annotations[i]
		}
	}
	return nil
}

// TestAnUpdateNamesWhatItTouches is the gap this closes.
func TestAnUpdateNamesWhatItTouches(t *testing.T) {
	a := changedFor(t, Assess(loadFixture(t, "demo.json")), "azurerm_network_security_group.web")
	if a == nil {
		t.Fatal("an update in place does not say which attributes it changes")
	}
	// BOTH, and the second one is the point. This resource's report already
	// named security_rules, through the reordering annotation. It changes tags
	// as well, and nothing in the report said so - a reviewer reading "update
	// in place, same elements different order" would have concluded the plan
	// touched one attribute and been wrong.
	if !equalStrings(a.Paths, []string{"security_rules", "tags"}) {
		t.Errorf("Paths = %v, want both attributes this update touches", a.Paths)
	}
	if !strings.Contains(a.Summary, "changes 2 attributes") {
		t.Errorf("Summary = %q, want it to count what it names", a.Summary)
	}
}

// TestASingleChangedAttributeReadsAsOne keeps the grammar honest on the count
// that is easiest to get wrong.
func TestASingleChangedAttributeReadsAsOne(t *testing.T) {
	seen := false
	for _, fixture := range []string{"demo.json", "real-plan.json", "critical.json", "large-estate.json"} {
		for _, f := range Assess(loadFixture(t, fixture)).Findings {
			for _, a := range f.Annotations {
				if a.Code != AnnChangedAttributes || len(a.Paths) != 1 {
					continue
				}
				seen = true
				if strings.Contains(a.Summary, "1 attributes") {
					t.Errorf("%s/%s: Summary reads %q", fixture, f.Address, a.Summary)
				}
			}
		}
	}
	if !seen {
		t.Skip("no fixture has a change touching exactly one attribute")
	}
}

// TestACreateNamesWhatItSets. A create has no before, so every attribute is
// new - which is the whole of what the plan says it will make.
func TestACreateNamesWhatItSets(t *testing.T) {
	a := changedFor(t, Assess(loadFixture(t, "demo.json")), "azurerm_subnet.application")
	if a == nil {
		t.Fatal("a create does not say which attributes it sets")
	}
	// THE EXACT SET, including the ones Terraform does not put in `after`.
	// An attribute whose value is unknown until apply is dropped from `after`
	// entirely and marked in `after_unknown`, so reading `after` alone reports
	// a create as setting fewer attributes than it sets - and the ones left
	// out are exactly the ones nobody can check before apply. Asserting only
	// that the list is non-empty missed that: dropping the unknowns still left
	// plenty of names behind.
	want := []string{
		"address_prefixes", "etag", "id", "name", "private_endpoint_network_policies",
		"resource_group_name",
		"service_endpoints", "virtual_network_name",
	}
	if !equalStrings(a.Paths, want) {
		t.Errorf("Paths = %v, want %v - id and etag are unknown until apply and are in "+
			"after_unknown rather than after", a.Paths, want)
	}
	if !strings.HasPrefix(a.Summary, "sets ") {
		t.Errorf("Summary = %q, want it to say the attributes are being SET rather than "+
			"changed - there is nothing here to change from", a.Summary)
	}
}

// TestADeleteNamesWhatItHad. Nothing changes on a delete: the resource goes.
// Naming what it held is the useful fact, and the verb has to say so.
func TestADeleteNamesWhatItHad(t *testing.T) {
	a := changedFor(t, Assess(loadFixture(t, "demo.json")), "azurerm_subnet.app")
	if a == nil {
		t.Fatal("a delete does not say which attributes it had")
	}
	if !strings.HasPrefix(a.Summary, "had ") {
		t.Errorf("Summary = %q, want the past tense - a delete changes nothing, it goes",
			a.Summary)
	}
}

// TestTheCountAlwaysMatchesTheNames. A sentence and its own evidence
// disagreeing is worse than either being wrong alone, because each looks like
// it corroborates the other. This session found that class four times.
func TestTheCountAlwaysMatchesTheNames(t *testing.T) {
	checked := 0
	for _, fixture := range []string{"demo.json", "real-plan.json", "critical.json", "large-estate.json"} {
		for _, f := range Assess(loadFixture(t, fixture)).Findings {
			for _, a := range f.Annotations {
				if a.Code != AnnChangedAttributes {
					continue
				}
				checked++
				// THE WHOLE PHRASE, NOT A SUBSTRING. Astra reported "had 16
				// attributes" for six names and this passed, because
				// "16 attributes" contains "6 attribute".
				if !strings.Contains(a.Summary, " "+itoa(len(a.Paths))+" attribute") {
					t.Errorf("%s/%s: Summary %q does not count the %d attributes it names: %v",
						fixture, f.Address, a.Summary, len(a.Paths), a.Paths)
				}
			}
		}
	}
	if checked == 0 {
		t.Fatal("no changed-attribute annotation was checked, so this test proves nothing")
	}
}

// TestNothingIsClaimedForAnOperationNobodyRead. classify ends at
// KindUnsupported when it cannot read the action, and assessOne returns before
// anything else is claimed. A list of attributes under an operation nobody
// understood would be exactly the confident guess that kind exists to refuse.
func TestNothingIsClaimedForAnOperationNobodyRead(t *testing.T) {
	for _, f := range Assess(loadFixture(t, "unsupported-action.json")).Findings {
		if f.Kind != KindUnsupported {
			continue
		}
		for _, a := range f.Annotations {
			if a.Code == AnnChangedAttributes {
				t.Errorf("%s: an unreadable operation claims %q", f.Address, a.Summary)
			}
		}
	}
}

// TestANoOpNamesNothing. A resource this plan does not change has no attributes
// to report, and "changes 0 attributes" under every no-op is how a reader is
// taught to skip the annotations.
func TestANoOpNamesNothing(t *testing.T) {
	for _, f := range Assess(loadFixture(t, "sequence-chain.json")).Findings {
		if f.Kind != KindNoOp {
			continue
		}
		for _, a := range f.Annotations {
			if a.Code == AnnChangedAttributes {
				t.Errorf("%s is a no-op and claims %q", f.Address, a.Summary)
			}
		}
	}
}

// TestEveryRankedChangeSaysWhatItTouches is the acceptance: at every level, a
// finding that does something to a resource says what.
func TestEveryRankedChangeSaysWhatItTouches(t *testing.T) {
	for _, fixture := range []string{"demo.json", "real-plan.json"} {
		r := Assess(loadFixture(t, fixture))
		for _, f := range r.Findings {
			switch f.Kind {
			case KindCreate, KindUpdate, KindDelete, KindReplace:
			default:
				continue
			}
			if changedFor(t, r, f.Address) == nil {
				t.Errorf("%s/%s is a %s at %s and does not say which attributes it touches",
					fixture, f.Address, f.Kind, f.Level)
			}
		}
	}
}

// TestNoValueReachesTheAttributeList is constraint 1 on this annotation. The
// names come from the plan's keys; nothing here may carry what was in them.
func TestNoValueReachesTheAttributeList(t *testing.T) {
	r := Assess(loadFixture(t, "unmarked-credentials.json"))
	secrets := []string{
		"AKIA", "-----BEGIN", "ghp_", "xoxb-", "cloudflare", "eyJ",
	}
	checked := 0
	for _, f := range r.Findings {
		for _, a := range f.Annotations {
			if a.Code != AnnChangedAttributes {
				continue
			}
			checked++
			hay := strings.ToLower(strings.Join(append(a.Paths, a.Summary, a.Detail), " "))
			for _, s := range secrets {
				if strings.Contains(hay, strings.ToLower(s)) {
					t.Errorf("%s: the attribute list carries %q, which came out of a value",
						f.Address, s)
				}
			}
		}
	}
	if checked == 0 {
		t.Fatal("no changed-attribute annotation was checked against the credentials fixture")
	}
}

// TestEveryReportedNameLooksLikeASchemaAttribute is the structural argument for
// why this annotation cannot leak, rather than the case-by-case one.
//
// A resource's before and after are a flat object of SCHEMA attributes, and a
// provider schema names its attributes in lower snake case. Taking top-level
// keys only means every name reported here came from a schema rather than from
// data - a map with attacker-chosen keys is the VALUE of an attribute called
// `tags`, and its keys are one level down where this never looks.
//
// So a name that could not be a schema attribute is evidence that something
// from a value has reached the list, whatever the route. That is a stronger
// guard than checking for particular secrets, because it does not depend on
// recognising what leaked.
func TestEveryReportedNameLooksLikeASchemaAttribute(t *testing.T) {
	schemaName := regexp.MustCompile(`^[a-z][a-z0-9_]*$`)

	checked := 0
	for _, fixture := range fixtureFiles(t) {
		for _, f := range Assess(loadFixture(t, fixture)).Findings {
			for _, a := range f.Annotations {
				if a.Code != AnnChangedAttributes {
					continue
				}
				for _, name := range a.Paths {
					checked++
					if !schemaName.MatchString(name) {
						t.Errorf("%s/%s: %q is not a shape a provider schema gives an "+
							"attribute, so it did not come from one - something from a "+
							"value has reached the list", fixture, f.Address, name)
					}
				}
			}
		}
	}
	if checked == 0 {
		t.Fatal("no attribute name was checked, so this test proves nothing")
	}
	t.Logf("checked %d attribute names across %d fixtures", checked, len(fixtureFiles(t)))
}

// fixtureFiles is every committed plan fixture, so a guard like the one above
// runs against everything in the repository rather than a list somebody has to
// remember to extend.
func fixtureFiles(t *testing.T) []string {
	t.Helper()
	entries, err := os.ReadDir("../../testdata")
	if err != nil {
		t.Fatalf("cannot read testdata: %v", err)
	}
	var out []string
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		// Not plans: two are deliberately broken and one is a rules file
		// that happens to live here.
		switch e.Name() {
		case "malformed.json", "notaplan.json", "rules-example.json":
			continue
		}
		out = append(out, e.Name())
	}
	return out
}

// TestEveryReportedNameIsATopLevelKeyOfThisChange is the exact version of the
// guard above, and it exists because the loose one was not enough.
//
// The shape check catches a name that could not have come from a schema. It
// does NOT catch a name that came from one level too deep and happens to look
// like an attribute - descending into an attribute's value and reporting the
// keys of a `tags` map passed it, because tag names are lower snake case too.
//
// This compares against the actual top-level keys of THIS resource's change.
// Any descent, from any route, reports a name that is not among them.
func TestEveryReportedNameIsATopLevelKeyOfThisChange(t *testing.T) {
	checked := 0
	for _, fixture := range fixtureFiles(t) {
		p := loadFixture(t, fixture)

		// Every entry for an address, because a deposed object shares one with
		// its resource and the finding could have come from either.
		top := map[string]map[string]bool{}
		for _, rc := range p.ResourceChanges {
			if rc == nil || rc.Change == nil {
				continue
			}
			if top[rc.Address] == nil {
				top[rc.Address] = map[string]bool{}
			}
			for _, side := range []interface{}{rc.Change.Before, rc.Change.After, rc.Change.AfterUnknown} {
				if m, ok := side.(map[string]interface{}); ok {
					for k := range m {
						top[rc.Address][k] = true
					}
				}
			}
		}

		for _, f := range Assess(p).Findings {
			for _, a := range f.Annotations {
				if a.Code != AnnChangedAttributes {
					continue
				}
				for _, name := range a.Paths {
					checked++
					if !top[f.Address][name] {
						t.Errorf("%s/%s reports %q, which is not a top-level key of that "+
							"resource's before, after or after_unknown - so it came from "+
							"inside a value", fixture, f.Address, name)
					}
				}
			}
		}
	}
	if checked == 0 {
		t.Fatal("no attribute name was checked, so this test proves nothing")
	}
}

// TestAnEmptyUnknownContainerIsNotAChange. Astra's first correctness finding,
// on a plan from real Terraform.
//
// after_unknown mirrors the shape of the resource, and Terraform writes an
// EMPTY container for an attribute that is fully known - "input": {} means
// there are no unknowns inside input, not that input changed. Treating any
// non-null, non-false entry as evidence of change reported an attribute whose
// before and after are identical.
//
// In testdata/unknown-containers.json, terraform_data.probe has
// "triggers_replace": {} and the same value on both sides. Terraform's own
// display shows input and output changing and nothing else.
func TestAnEmptyUnknownContainerIsNotAChange(t *testing.T) {
	a := changedFor(t, Assess(loadFixture(t, "unknown-containers.json")), "terraform_data.probe")
	if a == nil {
		t.Fatal("the update says nothing about what it changes")
	}
	want := []string{"input", "output"}
	if !equalStrings(a.Paths, want) {
		t.Errorf("Paths = %v, want %v - triggers_replace is identical on both sides and its "+
			"after_unknown entry is an empty container, which means no unknowns inside rather "+
			"than a change", a.Paths, want)
	}
}

// TestAnUnsetAttributeIsNotSet. Astra's second correctness finding, also on a
// plan from real Terraform.
//
// terraform_data.empty sets nothing. Terraform emits every optional attribute
// as a known null in `after` and displays only `id` being created. Counting the
// nulls made the number depend on how many optional attributes the provider
// happens to expose, which is a fact about the schema rather than about the
// change - and "sets 5 attributes" about a resource that sets none is simply
// false.
func TestAnUnsetAttributeIsNotSet(t *testing.T) {
	a := changedFor(t, Assess(loadFixture(t, "all-null-create.json")), "terraform_data.empty")
	if a == nil {
		t.Fatal("the create says nothing about what it sets")
	}
	if !equalStrings(a.Paths, []string{"id"}) {
		t.Errorf("Paths = %v, want only id - every other attribute is a known null, which is "+
			"an unset schema slot rather than something being set", a.Paths)
	}
	if !strings.Contains(a.Summary, "sets 1 attribute") {
		t.Errorf("Summary = %q, want the singular count of what it actually sets", a.Summary)
	}
}

// The exact wording, evidence and caveat, pinned per kind.
//
// Astra ran eight mutations past the suite: keeping only the first key of a
// delete or a replacement, saying "sets" for a replacement, disabling the
// unknown loop, returning nothing when either side is absent, suppressing the
// annotation on deposed entries, and emptying Detail and Note. Every one of
// them changes what a reader is told, and every one was green, because the
// tests asserted a prefix here and a count there and nothing asserted the whole
// thing on a known plan.
func TestTheAnnotationIsExactPerKind(t *testing.T) {
	cases := []struct {
		fixture, address string
		wantSummary      string
		wantPaths        []string
	}{
		{"all-null-create.json", "terraform_data.empty", "sets 1 attribute", []string{"id"}},
		{"unknown-containers.json", "terraform_data.probe", "changes 2 attributes", []string{"input", "output"}},
		{"demo.json", "azurerm_subnet.app", "had 6 attributes", []string{
			"address_prefixes", "name", "private_endpoint_network_policies",
			"resource_group_name", "service_endpoints", "virtual_network_name",
		}},
		{"demo.json", "azurerm_postgresql_flexible_server.main", "changes 3 attributes",
			[]string{"fqdn", "id", "zone"}},
		{"demo.json", "azurerm_network_security_group.web", "changes 2 attributes",
			[]string{"security_rules", "tags"}},
	}
	for _, c := range cases {
		t.Run(c.fixture+"/"+c.address, func(t *testing.T) {
			a := changedFor(t, Assess(loadFixture(t, c.fixture)), c.address)
			if a == nil {
				t.Fatal("no changed-attribute annotation")
			}
			if a.Summary != c.wantSummary {
				t.Errorf("Summary = %q, want %q", a.Summary, c.wantSummary)
			}
			if !equalStrings(a.Paths, c.wantPaths) {
				t.Errorf("Paths = %v, want %v", a.Paths, c.wantPaths)
			}
			// Detail carries the sentence AND the caveat, because a JSON
			// consumer and a markdown row have no footer to lift it into.
			if !strings.HasPrefix(a.Detail, a.Summary) {
				t.Errorf("Detail %q does not begin with its own Summary", a.Detail)
			}
			if a.Note == "" || !strings.HasSuffix(a.Detail, a.Note) {
				t.Errorf("Detail does not end with a standing caveat: Detail=%q Note=%q",
					a.Detail, a.Note)
			}
		})
	}
}

// TestAReplacementSaysChangesRatherThanSets. A replacement has a before, so its
// attributes are being changed. "sets" belongs to a create and passed
// everywhere, because no test pinned a replacement's verb.
func TestAReplacementSaysChangesRatherThanSets(t *testing.T) {
	seen := 0
	for _, fixture := range []string{"demo.json", "sequence-chain.json", "replace-create-first.json"} {
		r := Assess(loadFixture(t, fixture))
		for _, f := range r.Findings {
			if f.Kind != KindReplace {
				continue
			}
			a := changedFor(t, r, f.Address)
			if a == nil {
				continue
			}
			seen++
			if !strings.HasPrefix(a.Summary, "changes ") {
				t.Errorf("%s/%s is a replacement and says %q", fixture, f.Address, a.Summary)
			}
		}
	}
	if seen == 0 {
		t.Fatal("no replacement was checked, so this test proves nothing")
	}
}

// TestAnUnknownOnlyAttributeIsNamed. An attribute whose value is unknown until
// apply is dropped from `after` entirely, so it reaches the list only through
// the unknown pass. Disabling that pass stayed green.
func TestAnUnknownOnlyAttributeIsNamed(t *testing.T) {
	a := changedFor(t, Assess(loadFixture(t, "sequence-chain.json")), "terraform_data.base")
	if a == nil {
		t.Fatal("no changed-attribute annotation")
	}
	for _, want := range []string{"id", "output"} {
		if !contains(a.Paths, want) {
			t.Errorf("Paths = %v, missing %q - it is unknown until apply, so Terraform keeps "+
				"it out of `after` and only after_unknown names it", a.Paths, want)
		}
	}
}

// TestADeposedEntryStillSaysWhatItHad. A deposed object is a real thing being
// destroyed and it gets its own finding, so suppressing the annotation there
// would leave one of two entries at the same address silent.
func TestADeposedEntryStillSaysWhatItHad(t *testing.T) {
	p := loadFixture(t, "sequence-deposed.json")
	deposed := 0
	for _, rc := range p.ResourceChanges {
		if rc.DeposedKey != "" {
			deposed++
		}
	}
	if deposed == 0 {
		t.Fatal("the fixture no longer carries a deposed object")
	}
	// THE FINDING BEING ITERATED, NOT THE FIRST ONE AT ITS ADDRESS. changedFor
	// looks a finding up by address, and a deposed object SHARES its address
	// with the resource it belongs to - so suppressing the annotation on every
	// deposed entry left this green, because the replacement at the same
	// address still had one. Astra found it: seven of eight mutations killed,
	// this was the eighth.
	deletes := 0
	for _, f := range Assess(p).Findings {
		if f.Kind != KindDelete {
			continue
		}
		deletes++
		found := false
		for _, a := range f.Annotations {
			if a.Code == AnnChangedAttributes {
				found = true
			}
		}
		if !found {
			t.Errorf("%s is a delete and says nothing about what it had", f.Address)
		}
	}
	if deletes == 0 {
		t.Fatal("no delete was checked, so this test proves nothing")
	}

	// AND THE WHOLE LIST, not merely that one exists. Retaining only the first
	// attribute of a deposed delete passed, because nothing pinned the
	// deposed entry's contents.
	for _, rc := range p.ResourceChanges {
		if rc.DeposedKey == "" || rc.Change == nil {
			continue
		}
		before, _ := rc.Change.Before.(map[string]interface{})
		want := 0
		for _, v := range before {
			if v != nil {
				want++
			}
		}
		if want < 2 {
			continue // cannot tell a truncated list from a whole one
		}
		a, ok := changedAnnotation(rc, KindDelete)
		if !ok {
			t.Fatalf("%s (deposed) has no annotation", rc.Address)
		}
		if len(a.Paths) != want {
			t.Errorf("%s (deposed) names %d attributes and had %d: %v",
				rc.Address, len(a.Paths), want, a.Paths)
		}
	}
}

// TestReadImportAndForgetSayNothing. None of them changes an attribute, and the
// switch names every kind it handles rather than defaulting - so a kind added
// later arrives as nothing rather than as a confident list.
func TestReadImportAndForgetSayNothing(t *testing.T) {
	for _, k := range []Kind{KindRead, KindImport, KindForget, KindNoOp, KindUnsupported} {
		rc := &tfjson.ResourceChange{
			Address: "terraform_data.x",
			Change: &tfjson.Change{
				Before: map[string]interface{}{"input": "a"},
				After:  map[string]interface{}{"input": "b"},
			},
		}
		if _, ok := changedAnnotation(rc, k); ok {
			t.Errorf("%s produces a changed-attribute annotation", k)
		}
	}
}

func contains(hay []string, needle string) bool {
	for _, h := range hay {
		if h == needle {
			return true
		}
	}
	return false
}

// TestAMissingSideFallsBackToTheOneThatExists pins a path no committed fixture
// reaches: an update or replacement whose before or after is null.
//
// Astra could not produce one from real Terraform and did not promote it to a
// finding for that reason, which is right - but the branch exists, and removing
// it left the whole suite green. A replacement whose before is absent still
// sets everything in its after, and reporting nothing there would be the
// silence this tool exists to prevent.
//
// Called directly rather than through a fixture, because the point is that the
// shape does not occur in one.
func TestAMissingSideFallsBackToTheOneThatExists(t *testing.T) {
	cases := []struct {
		name          string
		before, after interface{}
		unknown       interface{}
		want          []string
	}{
		{
			name:    "before absent",
			after:   map[string]interface{}{"input": "new", "store": nil},
			unknown: map[string]interface{}{"id": true},
			want:    []string{"id", "input"},
		},
		{
			name:   "after absent",
			before: map[string]interface{}{"input": "old", "store": nil},
			want:   []string{"input"},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			rc := &tfjson.ResourceChange{
				Address: "terraform_data.x",
				Change: &tfjson.Change{
					Before: c.before, After: c.after, AfterUnknown: c.unknown,
				},
			}
			a, ok := changedAnnotation(rc, KindReplace)
			if !ok {
				t.Fatal("a replacement with one side absent reports nothing at all")
			}
			if !equalStrings(a.Paths, c.want) {
				t.Errorf("Paths = %v, want %v", a.Paths, c.want)
			}
		})
	}
}

// TestTwoNumbersTooBigForAFloatAreStillDifferent.
//
// encoding/json decodes every JSON number into a float64, and a float64 cannot
// tell 9007199254740992 from 9007199254740993 - both round to the same value,
// so every comparison in this package called them equal and the change
// disappeared. Terraform's own display shows the attribute changing.
//
// This is older than the annotation that exposed it: jsonEqual has always
// compared decoded values, so rewritten.go skipped the pair as unchanged too.
// What the annotation added was a sentence stating the wrong answer out loud -
// "changes 1 attribute: output" on a plan that changes two.
//
// json.NumberDecoding preserves the digits, so the comparison is on what the
// file actually says. See internal/plan for where that happens and why it
// cannot be done with the pinned decoder alone.
func TestTwoNumbersTooBigForAFloatAreStillDifferent(t *testing.T) {
	a := changedFor(t, Assess(loadFixture(t, "big-numbers.json")), "terraform_data.probe")
	if a == nil {
		t.Fatal("the update says nothing about what it changes")
	}
	want := []string{"input", "output"}
	if !equalStrings(a.Paths, want) {
		t.Errorf("Paths = %v, want %v - input goes from 9007199254740992 to "+
			"9007199254740993, which a float64 cannot tell apart", a.Paths, want)
	}
}

// TestABigNumberIsNotCalledTheSameNumberWrittenDifferently is the other half,
// and the worse one. rewritten.go claims an attribute's before and after are
// the same value written another way; on two numbers a float64 cannot
// distinguish it would have said so about a real change.
func TestABigNumberIsNotCalledTheSameNumberWrittenDifferently(t *testing.T) {
	for _, f := range Assess(loadFixture(t, "big-numbers.json")).Findings {
		for _, a := range f.Annotations {
			if a.Code != AnnSameNumber && a.Code != AnnAllRewritten {
				continue
			}
			for _, p := range a.Paths {
				if p == "input" {
					t.Errorf("%s: input is called %q, and its two values differ",
						f.Address, a.Code)
				}
			}
			if a.Code == AnnAllRewritten {
				t.Errorf("%s: every changed attribute is claimed to be a rewrite, and input "+
					"is a real change", f.Address)
			}
		}
	}
}

// TestTheEdgesOfWhatCountsAsAnAttribute, as a table.
//
// Every row is a mutation Astra walked past the suite. Called directly rather
// than through fixtures, because several of these shapes do not occur in a
// committed plan and the point of each is the rule rather than the plan.
func TestTheEdgesOfWhatCountsAsAnAttribute(t *testing.T) {
	num := func(s string) json.Number { return json.Number(s) }

	cases := []struct {
		name          string
		kind          Kind
		before, after interface{}
		unknown       interface{}
		want          []string
		wantNone      bool
	}{{
		name:   "a delete does not count its unset slots",
		kind:   KindDelete,
		before: map[string]interface{}{"input": "a", "id": "x", "store": nil, "triggers_replace": nil},
		want:   []string{"id", "input"},
	}, {
		name:   "a delete of a wholly unset resource says nothing at all",
		kind:   KindDelete,
		before: map[string]interface{}{"store": nil, "triggers_replace": nil},
		// "had 0 attributes" is a sentence with no subject, and printing one
		// under every such delete is how a reader learns to skip the
		// annotations.
		wantNone: true,
	}, {
		name:   "an update TO null is a change",
		kind:   KindUpdate,
		before: map[string]interface{}{"input": "set"},
		after:  map[string]interface{}{"input": nil},
		want:   []string{"input"},
	}, {
		name:   "null on both sides is not a change",
		kind:   KindUpdate,
		before: map[string]interface{}{"input": nil, "other": "a"},
		after:  map[string]interface{}{"input": nil, "other": "b"},
		want:   []string{"other"},
	}, {
		name:    "an unknown marker inside an array counts",
		kind:    KindUpdate,
		before:  map[string]interface{}{"input": []interface{}{nil, false}},
		after:   map[string]interface{}{"input": []interface{}{nil, false}},
		unknown: map[string]interface{}{"input": []interface{}{true, false}},
		want:    []string{"input"},
	}, {
		name:    "a false marker is not an unknown",
		kind:    KindUpdate,
		before:  map[string]interface{}{"input": nil},
		after:   map[string]interface{}{"input": nil},
		unknown: map[string]interface{}{"input": false},
		// Nothing changed and nothing is unknown, so there is nothing to say.
		wantNone: true,
	}, {
		name:  "a create counts a known empty collection",
		kind:  KindCreate,
		after: map[string]interface{}{"input": []interface{}{}, "store": map[string]interface{}{}, "name": "n"},
		// An empty list somebody wrote is set, and differs from one nobody
		// wrote. Only a null is an unset slot.
		want: []string{"input", "name", "store"},
	}, {
		name:   "two numbers a float64 cannot tell apart",
		kind:   KindUpdate,
		before: map[string]interface{}{"n": num("9007199254740992")},
		after:  map[string]interface{}{"n": num("9007199254740993")},
		want:   []string{"n"},
	}, {
		name:   "the same number written differently is still one number",
		kind:   KindUpdate,
		before: map[string]interface{}{"n": num("1000"), "other": "a"},
		after:  map[string]interface{}{"n": num("1e3"), "other": "b"},
		// It IS a change as far as the rendered value goes, so it is counted
		// here - the rewritten-value rules are what say it is cosmetic, and
		// they say so separately.
		want: []string{"n", "other"},
	}}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			rc := &tfjson.ResourceChange{
				Address: "terraform_data.x",
				Change: &tfjson.Change{
					Before: c.before, After: c.after, AfterUnknown: c.unknown,
				},
			}
			a, ok := changedAnnotation(rc, c.kind)
			if c.wantNone {
				if ok {
					t.Fatalf("expected no annotation, got %q with %v", a.Summary, a.Paths)
				}
				return
			}
			if !ok {
				t.Fatal("expected an annotation and got none")
			}
			if !equalStrings(a.Paths, c.want) {
				t.Errorf("Paths = %v, want %v", a.Paths, c.want)
			}
			if !strings.Contains(a.Summary, " "+itoa(len(c.want))+" attribute") {
				t.Errorf("Summary %q does not count its %d names", a.Summary, len(c.want))
			}
		})
	}
}

// TestTheCaveatSaysTheCountIsNotASize. Astra replaced it with the opposite -
// "The count measures how big the change is" - and the suite passed. The
// sentence exists because one attribute holding a hundred nested changes counts
// once, so a reader who takes the number for a size is reading it backwards.
func TestTheCaveatSaysTheCountIsNotASize(t *testing.T) {
	if !strings.Contains(changedNote, "not a measure of how big the change is") {
		t.Errorf("the caveat no longer says the count is not a size: %q", changedNote)
	}
	if !strings.Contains(changedNote, "top-level") {
		t.Errorf("the caveat no longer says these are top-level attributes: %q", changedNote)
	}
}

// TestABigNumberInsideAJSONDocumentIsNotARewrite. The same defect one level
// further in, and the same consequence: a rule whose job is telling a reviewer
// a difference is cosmetic, saying so about a real change.
//
// A JSON policy document carried as a string is exactly where a large
// identifier or a quota lives, and jsonDocument decoded it without keeping the
// digits - so two documents differing in the sixteenth figure compared equal
// and came back as "the same JSON written differently".
func TestABigNumberInsideAJSONDocumentIsNotARewrite(t *testing.T) {
	rc := &tfjson.ResourceChange{
		Address: "terraform_data.policy",
		Change: &tfjson.Change{
			Actions: tfjson.Actions{tfjson.ActionUpdate},
			Before:  map[string]interface{}{"input": `{"quota":9007199254740992}`},
			After:   map[string]interface{}{"input": `{"quota":9007199254740993}`},
		},
	}
	f := assessOne(rc)
	for _, a := range f.Annotations {
		if a.Code == AnnSameJSON || a.Code == AnnAllRewritten {
			t.Errorf("a document holding 9007199254740992 and one holding 9007199254740993 "+
				"are called %q", a.Code)
		}
	}

	// And the same two documents with the number written another way ARE the
	// same document, which is the case the rule exists for.
	rc.Change.After = map[string]interface{}{"input": `{ "quota" : 9007199254740992 }`}
	same := assessOne(rc)
	found := false
	for _, a := range same.Annotations {
		if a.Code == AnnSameJSON {
			found = true
		}
	}
	if !found {
		t.Error("the same document with different spacing is no longer recognised as a rewrite")
	}
}

// TestKnownFalseZeroAndEmptyAreSet. Only a NULL is an unset schema slot. A
// false, a zero and an empty string are values somebody chose, and treating
// them as unset would drop them from a create that sets them.
func TestKnownFalseZeroAndEmptyAreSet(t *testing.T) {
	rc := &tfjson.ResourceChange{
		Address: "terraform_data.x",
		Change: &tfjson.Change{
			After: map[string]interface{}{
				"flag": false, "count": json.Number("0"), "name": "", "unset": nil,
			},
		},
	}
	a, ok := changedAnnotation(rc, KindCreate)
	if !ok {
		t.Fatal("no annotation")
	}
	if !equalStrings(a.Paths, []string{"count", "flag", "name"}) {
		t.Errorf("Paths = %v, want count, flag and name - only the null is unset", a.Paths)
	}
}

// TestAnAdditionOrARemovalIsAChange. An attribute on one side only differs by
// definition, and both directions count.
func TestAnAdditionOrARemovalIsAChange(t *testing.T) {
	cases := []struct {
		name          string
		before, after map[string]interface{}
		want          []string
	}{
		{"added", map[string]interface{}{"a": "1"}, map[string]interface{}{"a": "1", "b": "2"}, []string{"b"}},
		{"removed", map[string]interface{}{"a": "1", "b": "2"}, map[string]interface{}{"a": "1"}, []string{"b"}},
		{"null to known", map[string]interface{}{"a": nil}, map[string]interface{}{"a": "1"}, []string{"a"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			rc := &tfjson.ResourceChange{
				Address: "terraform_data.x",
				Change:  &tfjson.Change{Before: c.before, After: c.after},
			}
			a, ok := changedAnnotation(rc, KindUpdate)
			if !ok {
				t.Fatal("no annotation")
			}
			if !equalStrings(a.Paths, c.want) {
				t.Errorf("Paths = %v, want %v", a.Paths, c.want)
			}
		})
	}
}

// TestTheMissingAfterFallbackDoesNotInventUnknowns. With no after there is
// nothing for an unknown marker to describe, and naming one would report an
// attribute the resource does not have.
func TestTheMissingAfterFallbackDoesNotInventUnknowns(t *testing.T) {
	rc := &tfjson.ResourceChange{
		Address: "terraform_data.x",
		Change: &tfjson.Change{
			Before:       map[string]interface{}{"input": "old"},
			AfterUnknown: map[string]interface{}{"phantom": true},
		},
	}
	a, ok := changedAnnotation(rc, KindReplace)
	if !ok {
		t.Fatal("no annotation")
	}
	if !equalStrings(a.Paths, []string{"input"}) {
		t.Errorf("Paths = %v, want only input - there is no after for an unknown to be in", a.Paths)
	}
}

// TestAFractionIsNotANumber. big.Rat parses "1/2" as a rational, and a plan
// holding the string "1/2" is holding text rather than a number - reading it as
// one would make "1/2" and "0.5" the same value written differently.
func TestAFractionIsNotANumber(t *testing.T) {
	if sameNumber("1/2", "0.5") {
		t.Error(`"1/2" and "0.5" are called the same number, and "1/2" is a string a provider ` +
			`round-tripped rather than a number anybody wrote`)
	}
	// The cases the rule does exist for still hold.
	for _, c := range [][2]interface{}{
		{json.Number("1000"), "1e3"},
		{json.Number("80"), "80"},
		{json.Number("1"), "1.0"},
	} {
		if !sameNumber(c[0], c[1]) {
			t.Errorf("%v and %v are no longer the same number", c[0], c[1])
		}
	}
}

// TestTheCountedOnceCaveatSaysOnce. Astra changed "counted once" to "counted
// twice" and the suite passed.
func TestTheCountedOnceCaveatSaysOnce(t *testing.T) {
	if !strings.Contains(changedNote, "counted once") {
		t.Errorf("the caveat no longer says a nested change is counted once: %q", changedNote)
	}
}

// TestTheNumericGrammarIsJSONsOwn. big.Rat accepts far more than a plan can
// hold, and each of these came back as "the same number written differently"
// about two different pieces of text a provider round-tripped.
func TestTheNumericGrammarIsJSONsOwn(t *testing.T) {
	refused := [][2]string{
		{"0b10", "2"}, {"0o10", "8"}, {"0x10", "16"}, {"1p3", "8"},
		{"1/2", "0.5"}, {"1_000", "1000"}, {"", "0"}, {"nan", "0"},
	}
	for _, c := range refused {
		if sameNumber(c[0], c[1]) {
			t.Errorf("%q and %q are called the same number, and %q is not a number a plan "+
				"can hold", c[0], c[1], c[0])
		}
	}
	// The decimal grammar JSON does have, all of which a provider can
	// round-trip into a string.
	accepted := [][2]string{
		{"1e3", "1000"}, {"1.0", "1"}, {"+1", "1"}, {".5", "0.5"},
		{"-0", "0"}, {"007", "7"}, {"1E3", "1000"},
	}
	for _, c := range accepted {
		if !sameNumber(c[0], c[1]) {
			t.Errorf("%q and %q are not called the same number, and they are", c[0], c[1])
		}
	}
}

// TestADocumentWithTrailingContentIsNotADocument. Decoder.More answers false at
// a closing delimiter rather than establishing end of input, so `{"n":1}]` and
// `{"n":1}}garbage` were accepted as documents and compared as JSON.
func TestADocumentWithTrailingContentIsNotADocument(t *testing.T) {
	for _, bad := range []string{`{"n":1}]`, `{"n":1}}garbage`, `{"n":1} {"n":2}`, `[1,2]]`} {
		rc := &tfjson.ResourceChange{
			Address: "terraform_data.x",
			Change: &tfjson.Change{
				Actions: tfjson.Actions{tfjson.ActionUpdate},
				Before:  map[string]interface{}{"input": bad},
				After:   map[string]interface{}{"input": `{"n":1}`},
			},
		}
		for _, a := range assessOne(rc).Annotations {
			if a.Code == AnnSameJSON || a.Code == AnnAllRewritten {
				t.Errorf("%q is treated as a JSON document and called %q", bad, a.Code)
			}
		}
	}
}

// TestTwoDocumentsHoldingTheSameNumberAreStillTheSameDocument. Preserving the
// digits meant {"n":1e3} and {"n":1000} stopped comparing equal, so a rewrite
// this rule exists to recognise was reported as a change. Both halves have to
// be true at once.
func TestTwoDocumentsHoldingTheSameNumberAreStillTheSameDocument(t *testing.T) {
	same := &tfjson.ResourceChange{
		Address: "terraform_data.x",
		Change: &tfjson.Change{
			Actions: tfjson.Actions{tfjson.ActionUpdate},
			Before:  map[string]interface{}{"input": `{"n":1e3}`},
			After:   map[string]interface{}{"input": `{"n":1000}`},
		},
	}
	found := false
	for _, a := range assessOne(same).Annotations {
		if a.Code == AnnSameJSON {
			found = true
		}
	}
	if !found {
		t.Error(`{"n":1e3} and {"n":1000} hold the same number and are no longer recognised ` +
			`as the same document`)
	}
}
