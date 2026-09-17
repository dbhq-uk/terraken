package assess

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/dbhq-uk/terraken/internal/plan"
	tfjson "github.com/hashicorp/terraform-json"
)

// How much of this change can be checked before it is applied, and what the
// part that cannot is.
//
// Five separate silences in five different fields are the same fact, and the
// tool said none of them. This is constraint 5 - say when something cannot be
// known - applied to the plan rather than to a single resource.

func boolp(b bool) *bool { return &b }

// A plan with nothing hidden says so, in one line. Silence would leave the
// reader to infer it from an absent section, which is the same as not being
// told.
func TestAFullyCheckablePlanSaysSo(t *testing.T) {
	p := planOf(change("azurerm_virtual_network.a", "azurerm_virtual_network", tfjson.ActionCreate))
	// Something drifted, so the drift silence does not apply: that is the only
	// way a plan can be free of it, because an absent array and an empty one
	// say the same thing. See TestNoDriftRecordedIsAlwaysASilence.
	p.ResourceDrift = []*tfjson.ResourceChange{
		change("azurerm_virtual_network.a", "azurerm_virtual_network", tfjson.ActionUpdate),
	}

	c := Assess(p).Coverage
	if len(c.Gaps) != 0 {
		t.Fatalf("expected no gaps, got %+v", c.Gaps)
	}
	if !c.Complete() {
		t.Error("Complete() must be true when nothing is hidden")
	}
	if !strings.Contains(c.Headline, "Nothing here is hidden from review") {
		t.Errorf("Headline = %q, want it to say nothing is hidden", c.Headline)
	}
	if c.Changes != 1 || c.Assessed != 1 {
		t.Errorf("Changes = %d, Assessed = %d; want 1 and 1", c.Changes, c.Assessed)
	}
}

// Each of the five is reported SEPARATELY. "Unknown until apply" and "refresh
// was skipped" are different kinds of not-knowing, and a reader who cannot
// tell them apart has been given one fact where there were two.
func TestEachSilenceIsNamedSeparately(t *testing.T) {
	unknown := change("azurerm_virtual_network.a", "azurerm_virtual_network", tfjson.ActionCreate)
	unknown.Change.AfterUnknown = map[string]interface{}{"id": true}

	p := planOf(unknown,
		change("azurerm_virtual_network.b", "azurerm_virtual_network", tfjson.ActionCreate))
	// No resource_drift array at all: refresh may have been skipped, and
	// nothing in the file says which.
	p.ResourceDrift = nil
	p.Checks = []tfjson.CheckResultStatic{{
		Status: tfjson.CheckStatusUnknown,
		Instances: []tfjson.CheckResultDynamic{
			{Status: tfjson.CheckStatusUnknown},
			{Status: tfjson.CheckStatusPass},
		},
	}}
	p.DeferredChanges = []*tfjson.DeferredResourceChange{
		{Reason: "instance_count_unknown"},
	}

	r := AssessWithStatus(p, nil, plan.Status{Complete: boolp(false)})
	c := r.Coverage

	want := map[string]bool{
		GapUnknownUntilApply:  false,
		GapDriftNotKnown:      false,
		GapIncompletePlan:     false,
		GapChecksUndetermined: false,
		GapDeferred:           false,
	}
	for _, g := range c.Gaps {
		if _, ok := want[g.Code]; !ok {
			t.Errorf("unexpected gap %q", g.Code)
			continue
		}
		want[g.Code] = true
		if g.Detail == "" {
			t.Errorf("gap %q has no sentence", g.Code)
		}
	}
	for code, found := range want {
		if !found {
			t.Errorf("gap %q was not reported", code)
		}
	}
	if c.Complete() {
		t.Error("Complete() must be false when anything is hidden")
	}
}

// COUNTS WITH A NAMED DENOMINATOR, NEVER A PERCENTAGE. A bare "62% reviewable"
// is itself a verdict, and the roll-up states a fact rather than ruling.
func TestCoverageCountsHaveADenominatorAndNoPercentage(t *testing.T) {
	a := change("azurerm_virtual_network.a", "azurerm_virtual_network", tfjson.ActionCreate)
	a.Change.AfterUnknown = map[string]interface{}{"id": true}
	b := change("azurerm_virtual_network.b", "azurerm_virtual_network", tfjson.ActionCreate)
	b.Change.AfterUnknown = map[string]interface{}{"id": true}

	c := Assess(planOf(a, b,
		change("azurerm_virtual_network.c", "azurerm_virtual_network", tfjson.ActionCreate))).Coverage

	if c.Changes != 3 {
		t.Errorf("Changes = %d, want 3", c.Changes)
	}
	if c.Assessed != 1 {
		t.Errorf("Assessed = %d, want 1 - two of the three carry unknowns", c.Assessed)
	}

	var found bool
	for _, g := range c.Gaps {
		if g.Code != GapUnknownUntilApply {
			continue
		}
		found = true
		if g.Count != 2 || g.Of != 3 {
			t.Errorf("got %d of %d, want 2 of 3", g.Count, g.Of)
		}
	}
	if !found {
		t.Fatal("the unknown-until-apply gap was not reported")
	}

	// Nothing in the whole structure may be a percentage or a score.
	for _, g := range c.Gaps {
		for _, banned := range []string{"%", "percent", "score", "grade"} {
			if strings.Contains(strings.ToLower(g.Detail), banned) {
				t.Errorf("gap %q says %q, which is a verdict rather than a fact: %s",
					g.Code, banned, g.Detail)
			}
		}
	}
	if strings.Contains(c.Headline, "%") {
		t.Errorf("Headline = %q, which carries a percentage", c.Headline)
	}
}

// NO DRIFT RECORDED IS ALWAYS A SILENCE, whether the field is absent or an
// empty array.
//
// The first version of this believed the opposite: that an empty array proved
// refresh had run and found nothing, and only an absent field was ambiguous.
// It would have been a useful distinction if it existed. It does not -
// Terraform writes the array only when something actually drifted, so a plan
// from a clean refresh and a plan from -refresh=false look identical here.
// docs/roadmap.md had recorded that before any of this was written, and the
// code contradicted it; a report claiming "nothing drifted" on a plan where
// nobody looked is exactly the quiet wrongness this tool exists to correct.
func TestNoDriftRecordedIsAlwaysASilence(t *testing.T) {
	for _, tc := range []struct {
		name  string
		drift []*tfjson.ResourceChange
		gap   bool
	}{
		{"the field is absent", nil, true},
		{"the field is an empty array", []*tfjson.ResourceChange{}, true},
		{"something actually drifted", []*tfjson.ResourceChange{
			change("azurerm_virtual_network.a", "azurerm_virtual_network", tfjson.ActionUpdate),
		}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := planOf(change("azurerm_virtual_network.a", "azurerm_virtual_network", tfjson.ActionCreate))
			p.ResourceDrift = tc.drift

			var found bool
			for _, g := range Assess(p).Coverage.Gaps {
				if g.Code == GapDriftNotKnown {
					found = true
				}
			}
			if found != tc.gap {
				t.Errorf("drift gap reported = %v, want %v", found, tc.gap)
			}
		})
	}
}

// An operation the tool could not read is not an assessed one.
//
// assessOne returns before it reaches the unknown-value check, so an
// unsupported change carried no unverifiable annotation and was counted as
// fully assessed: a plan whose only change terraken explicitly could not
// understand reported "everything could be assessed". The kind is what
// decides, not the level, because a team rule giving one a severity does not
// make the operation understood.
func TestAnUnreadableOperationIsNotCountedAsAssessed(t *testing.T) {
	p := planOf(
		change("terraform_data.unreadable", "terraform_data", "reconcile"),
		change("azurerm_virtual_network.b", "azurerm_virtual_network", tfjson.ActionCreate),
	)
	p.ResourceDrift = []*tfjson.ResourceChange{
		change("x.y", "x", tfjson.ActionUpdate),
	}
	c := Assess(p).Coverage

	if c.Assessed != 1 {
		t.Errorf("Assessed = %d, want 1 of 2 - one of them could not be read at all", c.Assessed)
	}
	var found bool
	for _, g := range c.Gaps {
		if g.Code != GapUnreadableOperation {
			continue
		}
		found = true
		if g.Count != 1 || g.Of != 2 {
			t.Errorf("got %d of %d, want 1 of 2", g.Count, g.Of)
		}
	}
	if !found {
		t.Fatalf("no unreadable-operation gap: %+v", c.Gaps)
	}
	if c.Complete() {
		t.Error("a plan holding an operation the tool cannot read is not fully assessed")
	}
}

// An output is not a resource change and has no finding to hang from, so it
// needs its own count. An output-only plan carrying an unknown value used to
// report that there was nothing to assess and nothing unknown.
func TestAnUnknownOutputIsCountedOnItsOwnTerms(t *testing.T) {
	p := planOf()
	p.ResourceDrift = []*tfjson.ResourceChange{change("x.y", "x", tfjson.ActionUpdate)}
	p.OutputChanges = map[string]*tfjson.Change{
		"endpoint": {Actions: tfjson.Actions{tfjson.ActionCreate}, AfterUnknown: true},
		"region":   {Actions: tfjson.Actions{tfjson.ActionCreate}, After: "eu-west-2"},
	}

	c := Assess(p).Coverage
	if c.Outputs != 2 || c.UnknownOutputs != 1 {
		t.Errorf("Outputs = %d, UnknownOutputs = %d; want 2 and 1", c.Outputs, c.UnknownOutputs)
	}
	var found bool
	for _, g := range c.Gaps {
		if g.Code != GapUnknownOutputs {
			continue
		}
		found = true
		if g.Count != 1 || g.Of != 2 {
			t.Errorf("got %d of %d, want 1 of 2", g.Count, g.Of)
		}
	}
	if !found {
		t.Fatalf("no unknown-outputs gap: %+v", c.Gaps)
	}
	// And the resource denominator is untouched by it: an output is not a
	// resource change, and folding the two together would put a number in the
	// report that matches nothing else in it.
	if c.Changes != 0 {
		t.Errorf("Changes = %d, want 0 - there are no resource changes here", c.Changes)
	}
}

// A check with no instances is not one checked object.
//
// Terraform emits a static result with no instances in two cases that mean
// opposite things: pass with none means expansion found zero objects, and
// unknown with none means it could not establish them. Counting either as one
// invents a cardinality the plan does not state.
func TestTheChecksDenominatorCountsInstantiatedObjectsOnly(t *testing.T) {
	p := planOf(change("azurerm_virtual_network.a", "azurerm_virtual_network", tfjson.ActionCreate))
	p.ResourceDrift = []*tfjson.ResourceChange{change("x.y", "x", tfjson.ActionUpdate)}
	p.Checks = []tfjson.CheckResultStatic{
		// Expanded to zero objects. Not a checked object at all.
		{Status: tfjson.CheckStatusPass},
		// Two real objects, one of them undetermined.
		{Status: tfjson.CheckStatusUnknown, Instances: []tfjson.CheckResultDynamic{
			{Status: tfjson.CheckStatusUnknown},
			{Status: tfjson.CheckStatusPass},
		}},
	}

	var got Gap
	for _, g := range Assess(p).Coverage.Gaps {
		if g.Code == GapChecksUndetermined {
			got = g
		}
	}
	if got.Code == "" {
		t.Fatal("no checks gap")
	}
	if got.Count != 1 || got.Of != 2 {
		t.Errorf("got %d of %d, want 1 of 2 - the zero-instance check is not an object", got.Count, got.Of)
	}
}

// And a check Terraform could not expand at all is said in its own words,
// because its cardinality is unknown rather than one.
func TestACheckThatCouldNotBeExpandedIsSaidSeparately(t *testing.T) {
	p := planOf(change("azurerm_virtual_network.a", "azurerm_virtual_network", tfjson.ActionCreate))
	p.ResourceDrift = []*tfjson.ResourceChange{change("x.y", "x", tfjson.ActionUpdate)}
	p.Checks = []tfjson.CheckResultStatic{{Status: tfjson.CheckStatusUnknown}}

	var got Gap
	for _, g := range Assess(p).Coverage.Gaps {
		if g.Code == GapChecksUndetermined {
			got = g
		}
	}
	if got.Code == "" {
		t.Fatalf("no checks gap for a check that could not be expanded")
	}
	if got.Of != 0 {
		t.Errorf("Of = %d, want 0 - there are no instantiated objects to be a denominator", got.Of)
	}
	if !strings.Contains(got.Detail, "could not be expanded") {
		t.Errorf("Detail = %q, want it to say the check could not be expanded", got.Detail)
	}
}

// Coverage counts the WHOLE plan, always. --min-level changes what a reader
// sees and must not change what the tool says it could check, for the same
// reason it does not change the shape summary.
func TestAFilterDoesNotChangeCoverage(t *testing.T) {
	a := change("azurerm_virtual_network.a", "azurerm_virtual_network", tfjson.ActionCreate)
	a.Change.AfterUnknown = map[string]interface{}{"id": true}
	full := Assess(planOf(a,
		change("azurerm_mssql_database.db", "azurerm_mssql_database", tfjson.ActionDelete)))

	filtered := full.AtLeast(Critical)
	if len(filtered.Findings) == len(full.Findings) {
		t.Fatal("the filter should have hidden something, or this proves nothing")
	}
	if filtered.Coverage.Changes != full.Coverage.Changes {
		t.Errorf("Changes changed under a filter: %d then %d",
			full.Coverage.Changes, filtered.Coverage.Changes)
	}
	if len(filtered.Coverage.Gaps) != len(full.Coverage.Gaps) {
		t.Errorf("Gaps changed under a filter: %d then %d",
			len(full.Coverage.Gaps), len(filtered.Coverage.Gaps))
	}
}

// The gaps come out in a fixed order whatever order the plan put them in, so
// two runs of the same plan produce the same report.
func TestCoverageGapsAreDeterministic(t *testing.T) {
	build := func() Coverage {
		u := change("azurerm_virtual_network.a", "azurerm_virtual_network", tfjson.ActionCreate)
		u.Change.AfterUnknown = map[string]interface{}{"id": true}
		p := planOf(u)
		p.ResourceDrift = nil
		p.Checks = []tfjson.CheckResultStatic{{
			Status:    tfjson.CheckStatusUnknown,
			Instances: []tfjson.CheckResultDynamic{{Status: tfjson.CheckStatusUnknown}},
		}}
		p.DeferredChanges = []*tfjson.DeferredResourceChange{{Reason: "x"}}
		return AssessWithStatus(p, nil, plan.Status{Complete: boolp(false)}).Coverage
	}
	first := build()
	// THE WHOLE STRUCTURE, not the gap codes. Comparing codes alone passed on
	// constant empty coverage, and left every count, denominator and sentence
	// unchecked.
	if len(first.Gaps) < 4 {
		t.Fatalf("the fixture should produce several gaps, or this proves little: %+v", first.Gaps)
	}
	want, err := json.Marshal(first)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 20; i++ {
		got, err := json.Marshal(build())
		if err != nil {
			t.Fatal(err)
		}
		if string(got) != string(want) {
			t.Fatalf("run %d differs from the first:\n%s\n%s", i, want, got)
		}
	}
}

// Each of the five conditions, alone, produces its own gap and no other. The
// combined fixture proves they all appear together; this proves each one is
// what causes its own.
func TestEachConditionIndependentlyCausesItsOwnGap(t *testing.T) {
	// A baseline with nothing hidden, so any gap seen below is caused by the
	// one thing the case adds.
	baseline := func() *tfjson.Plan {
		p := planOf(change("azurerm_virtual_network.a", "azurerm_virtual_network", tfjson.ActionCreate))
		p.ResourceDrift = []*tfjson.ResourceChange{change("x.y", "x", tfjson.ActionUpdate)}
		return p
	}
	if gaps := Assess(baseline()).Coverage.Gaps; len(gaps) != 0 {
		t.Fatalf("the baseline is not clean: %+v", gaps)
	}

	for _, tc := range []struct {
		name string
		code string
		make func() (*tfjson.Plan, plan.Status)
	}{
		{"a value unknown until apply", GapUnknownUntilApply, func() (*tfjson.Plan, plan.Status) {
			p := baseline()
			p.ResourceChanges[0].Change.AfterUnknown = map[string]interface{}{"id": true}
			return p, plan.Status{}
		}},
		{"no drift recorded", GapDriftNotKnown, func() (*tfjson.Plan, plan.Status) {
			p := baseline()
			p.ResourceDrift = nil
			return p, plan.Status{}
		}},
		{"the plan is not complete", GapIncompletePlan, func() (*tfjson.Plan, plan.Status) {
			return baseline(), plan.Status{Complete: boolp(false)}
		}},
		{"a check that could not be determined", GapChecksUndetermined, func() (*tfjson.Plan, plan.Status) {
			p := baseline()
			p.Checks = []tfjson.CheckResultStatic{{
				Status:    tfjson.CheckStatusUnknown,
				Instances: []tfjson.CheckResultDynamic{{Status: tfjson.CheckStatusUnknown}},
			}}
			return p, plan.Status{}
		}},
		{"work Terraform postponed", GapDeferred, func() (*tfjson.Plan, plan.Status) {
			p := baseline()
			p.DeferredChanges = []*tfjson.DeferredResourceChange{{Reason: "instance_count_unknown"}}
			return p, plan.Status{}
		}},
		{"an operation the tool cannot read", GapUnreadableOperation, func() (*tfjson.Plan, plan.Status) {
			p := baseline()
			p.ResourceChanges[0].Change.Actions = tfjson.Actions{"reconcile"}
			return p, plan.Status{}
		}},
		{"an output unknown until apply", GapUnknownOutputs, func() (*tfjson.Plan, plan.Status) {
			p := baseline()
			p.OutputChanges = map[string]*tfjson.Change{
				"endpoint": {Actions: tfjson.Actions{tfjson.ActionCreate}, AfterUnknown: true},
			}
			return p, plan.Status{}
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p, st := tc.make()
			gaps := AssessWithStatus(p, nil, st).Coverage.Gaps
			if len(gaps) != 1 {
				t.Fatalf("want exactly one gap, got %+v", gaps)
			}
			if gaps[0].Code != tc.code {
				t.Errorf("got gap %q, want %q", gaps[0].Code, tc.code)
			}
			if gaps[0].Detail == "" {
				t.Error("the gap has no sentence")
			}
		})
	}
}
