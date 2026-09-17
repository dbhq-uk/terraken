package assess

import (
	"encoding/json"
	"os"
	"testing"

	tfjson "github.com/hashicorp/terraform-json"
)

// loadRealFixture reads a genuine "terraform show -json" plan straight off
// disk. Every other fixture in this package is hand-written to satisfy a
// specific test; this one is untouched real output, generated once from a
// throwaway local configuration (real Terraform v1.16.1, real destructive
// actions, no real infrastructure) and checked in as-is. It exists to catch
// assumptions the hand-written fixtures share with the code, which a
// hand-written fixture can never do by construction.
func loadRealFixture(t *testing.T) *tfjson.Plan {
	t.Helper()
	path := "../../testdata/real-plan.json"
	b, err := os.ReadFile(path)
	if err != nil {
		// Fatal, not Skip. This fixture is committed, so it is never
		// legitimately absent - and skipping on a missing file means
		// deleting it turns four tests green instead of red.
		t.Fatalf("committed fixture %s is missing: %v", path, err)
	}
	var p tfjson.Plan
	if err := json.Unmarshal(b, &p); err != nil {
		t.Fatalf("fixture %s did not parse: %v", path, err)
	}
	return &p
}

// TestRealPlanEveryChangeProducesOneFinding checks the parser survives
// genuine plan JSON: every resource change becomes exactly one finding,
// each with a real address and a level that resolved to something other
// than the String() fallback.
func TestRealPlanEveryChangeProducesOneFinding(t *testing.T) {
	p := loadRealFixture(t)
	r := Assess(p)

	if len(r.Findings) != len(p.ResourceChanges) {
		t.Errorf("got %d findings for %d resource changes - every change must produce exactly one finding",
			len(r.Findings), len(p.ResourceChanges))
	}
	for _, f := range r.Findings {
		if f.Address == "" {
			t.Error("a finding has no address")
		}
		if f.LevelName == "" || f.LevelName == "unknown" {
			t.Errorf("finding %s has level %q", f.Address, f.LevelName)
		}
	}
}

// TestRealPlanUnrecognisedProviderDeletesAreHigh checks the two deletes in
// the fixture - local_file.doomed and terraform_data.keep - are both types
// outside the curated azurerm_/aws_/google_ prefixes, so neither can be
// assumed safe to destroy. Each must be High and carry an
// unrecognised-provider annotation: this is the honest-degradation rule
// working on real data, not a hand-written case built to exercise it.
func TestRealPlanUnrecognisedProviderDeletesAreHigh(t *testing.T) {
	p := loadRealFixture(t)
	r := Assess(p)

	for _, addr := range []string{"local_file.doomed", "terraform_data.keep"} {
		var found bool
		for _, f := range r.Findings {
			if f.Address != addr {
				continue
			}
			found = true
			if f.Level != High {
				t.Errorf("%s: Level = %v, want High", addr, f.Level)
			}
			if _, ok := annotationFor(f, AnnUnknownVendor); !ok {
				t.Errorf("%s: expected an unrecognised-provider annotation", addr)
			}
		}
		if !found {
			t.Errorf("no finding for %s", addr)
		}
	}
}

// TestRealPlanCreateWithComputedAttributesIsUnverifiable checks
// terraform_data.kept, a create whose id and output are not known until
// apply. Both must be named in an unverifiable-until-apply annotation
// rather than silently dropped.
func TestRealPlanCreateWithComputedAttributesIsUnverifiable(t *testing.T) {
	p := loadRealFixture(t)
	r := Assess(p)

	var f Finding
	var found bool
	for _, cand := range r.Findings {
		if cand.Address == "terraform_data.kept" {
			f, found = cand, true
		}
	}
	if !found {
		t.Fatal("no finding for terraform_data.kept")
	}

	a, ok := annotationFor(f, AnnUnverifiable)
	if !ok {
		t.Fatal("expected an unverifiable-until-apply annotation")
	}
	want := map[string]bool{"id": true, "output": true}
	if len(a.Paths) != len(want) {
		t.Fatalf("Paths = %v, want id and output", a.Paths)
	}
	for _, p := range a.Paths {
		if !want[p] {
			t.Errorf("unexpected path %q", p)
		}
	}
}

// TestUnsupportedOperationGoldenShape pins the exact shape of the one
// finding type that says nothing about what the plan does.
//
// It is pinned harder than the others on purpose. Every field here is load
// bearing: the kind is what a machine consumer branches on, the level name
// is what says the ranking does not apply, the empty counts map is the
// "outside the severity counts" rule holding, and the quoted actions are
// what stops a plan file recolouring the report that reads it. A change
// that alters any of them is a change to the contract, and it should arrive
// as a diff here rather than as nothing.
func TestUnsupportedOperationGoldenShape(t *testing.T) {
	b, err := os.ReadFile("../../testdata/unsupported-action.json")
	if err != nil {
		t.Fatalf("committed fixture is missing: %v", err)
	}
	var p tfjson.Plan
	if err := json.Unmarshal(b, &p); err != nil {
		t.Fatalf("fixture did not parse: %v", err)
	}
	r := Assess(&p)

	if r.Unassessed != 2 {
		t.Errorf("Unassessed = %d, want 2", r.Unassessed)
	}
	if r.CountsByName["unranked"] != 0 {
		t.Errorf("CountsByName holds an unranked tally: %v", r.CountsByName)
	}
	if r.CountsByName["high"] != 1 || len(r.CountsByName) != 1 {
		t.Errorf("CountsByName = %v, want only the delete's high", r.CountsByName)
	}

	// The exact sentence, pinned. It is the whole of what a reviewer is told
	// about an operation nobody understood, so a change to it is a change to
	// the report's meaning and should arrive as a diff here.
	const detail = "this build does not recognise the operation this plan asks for, " +
		"so its impact cannot be assessed and nothing below it was ranked"

	want := []struct {
		address string
		kind    Kind
		level   string
		actions []string
	}{
		{"terraform_data.quarantined", KindUnsupported, "unranked", []string{`"delete"`, `"quarantine"`}},
		{"terraform_data.reconciled", KindUnsupported, "unranked", []string{`"reconcile"`}},
		{"local_file.ordinary", KindDelete, "high", nil},
	}
	if len(r.Findings) != len(want) {
		t.Fatalf("got %d findings, want %d", len(r.Findings), len(want))
	}
	for i, w := range want {
		f := r.Findings[i]
		if f.Address != w.address || f.Kind != w.kind || f.LevelName != w.level {
			t.Errorf("finding %d = %s/%s/%s, want %s/%s/%s",
				i, f.Address, f.Kind, f.LevelName, w.address, w.kind, w.level)
		}
		if f.DataLoss {
			t.Errorf("%s: DataLoss must be false", f.Address)
		}
		a, ok := annotationFor(f, AnnUnsupportedAction)
		if w.actions == nil {
			if ok {
				t.Errorf("%s: a recognised action carries an unsupported-operation annotation", f.Address)
			}
			continue
		}
		if !ok {
			t.Fatalf("%s: no unsupported-operation annotation", f.Address)
		}
		// Exactly one annotation, not "at least this one". Anything else on
		// the finding would be a claim about an operation nobody understood.
		if len(f.Annotations) != 1 {
			t.Errorf("%s: annotations = %+v, want exactly one", f.Address, f.Annotations)
		}
		if a.Detail != detail {
			t.Errorf("%s: Detail = %q, want %q", f.Address, a.Detail, detail)
		}
		if len(a.Paths) != len(w.actions) {
			t.Fatalf("%s: actions = %v, want %v", f.Address, a.Paths, w.actions)
		}
		for j, act := range w.actions {
			if a.Paths[j] != act {
				t.Errorf("%s: action %d = %q, want %q", f.Address, j, a.Paths[j], act)
			}
		}
	}
}

// TestRealPlanDoesNotFlagMissedMoveOnSmallResourceType documents a real
// limitation rather than hiding it. terraform_data.keep is deleted and
// terraform_data.kept is created in the same plan - exactly the orphaned
// delete/create shape the missed-moved-block detector looks for - but it
// does not fire here.
//
// terraform_data exposes very few top-level attributes, and two of them
// (id, output) are computed and excluded from comparison, and two more
// (store, triggers_replace) are null on both sides and also excluded as
// uninformative. That leaves a single comparable attribute ("input") for
// this pair, which falls below minComparableAttrs (3): matching on one
// attribute is not enough evidence to suggest a moved block, so the
// detector correctly declines to guess.
//
// This is the threshold behaving as designed on a very small resource
// type, not a bug. If someone later lowers minComparableAttrs, this test
// will start failing and tell them they have changed a real behaviour.
func TestRealPlanDoesNotFlagMissedMoveOnSmallResourceType(t *testing.T) {
	p := loadRealFixture(t)
	r := Assess(p)

	for _, f := range r.Findings {
		if _, ok := annotationFor(f, AnnMissedMoved); ok {
			t.Errorf("%s: did not expect a missed-moved-block annotation - terraform_data.keep/kept fall below minComparableAttrs by design",
				f.Address)
		}
	}
}
