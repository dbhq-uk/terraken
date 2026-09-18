package assess

import (
	"strings"
	"testing"
)

// What the sequencing feature is allowed to claim, and it is narrower than the
// issue that asked for it.
//
// #32 offers this as an example of a fact terraken may state: "Six resources
// depend on this and cannot be reached until it is recreated". After #43 that
// sentence is not one this tool can say. "Cannot be reached" is a claim about
// the world; the plan states the order of operations and nothing about whether
// a provider's destroy takes the thing away, or whether anything was reaching
// it. The same local_file counterexample that cut eleven claims out of #43
// cuts this one.
//
// So the report names ORDER and stops. Two claims, reported separately, both
// from Terraform's documented ordering and both verified by running it:
//
//   - a dependant is destroyed BEFORE this resource is destroyed
//   - a dependant is created or updated AFTER this resource is created
//
// Each is anchored to one step of THIS resource, which is what makes them safe
// to state together. See testdata/_gen for the roots and the observed apply
// orders.

// both is the two ordering annotations on a finding, either of which may be
// absent.
func both(t *testing.T, r Report, addr string) (before, after *Annotation) {
	t.Helper()
	f := findingFor(t, r, addr)
	for i := range f.Annotations {
		switch f.Annotations[i].Code {
		case AnnDestroyedBefore:
			before = &f.Annotations[i]
		case AnnChangedAfter:
			after = &f.Annotations[i]
		}
	}
	return before, after
}

// TestAChainIsOrderedAroundTheResourceItDependsOn is the feature, on a real
// plan. terraform_data.middle and .leaf both depend on .base, all three are
// replaced, and .unrelated is a no-op that must stay out of it.
func TestAChainIsOrderedAroundTheResourceItDependsOn(t *testing.T) {
	r := Assess(loadFixture(t, "sequence-chain.json"))
	before, after := both(t, r, "terraform_data.base")
	if before == nil || after == nil {
		t.Fatalf("terraform_data.base is missing an ordering annotation: before=%v after=%v",
			before != nil, after != nil)
	}
	want := []string{"terraform_data.middle", "terraform_data.leaf"}
	if got := addressesOf(before.DestroyedFirst); !equalStrings(got, want) {
		t.Errorf("DestroyedFirst = %v, want middle then leaf - nearest first, and a "+
			"dependant is destroyed before the thing it depends on", got)
	}
	if got := addressesOf(after.ChangedAfter); !equalStrings(got, want) {
		t.Errorf("ChangedAfter = %v, want middle then leaf", got)
	}
}

// TestEachClauseIsAnchoredToAStepOfThisResource is the P1 Astra found, as a
// wording rule.
//
// The two facts used to be welded into one sentence - "destroys these before
// this one, and changes them again afterwards" - which assumed this resource's
// destroy comes before its create. Under create_before_destroy it does not:
// the creates run first. Each sentence now names which step of THIS resource
// it is relative to, so neither depends on the other being true.
func TestEachClauseIsAnchoredToAStepOfThisResource(t *testing.T) {
	before, after := both(t, Assess(loadFixture(t, "sequence-chain.json")), "terraform_data.base")
	if before == nil || after == nil {
		t.Fatal("terraform_data.base is missing an ordering annotation")
	}
	if !strings.Contains(before.Summary, "before destroying this one") {
		t.Errorf("Summary = %q, want it anchored to this resource's destroy", before.Summary)
	}
	if !strings.Contains(after.Summary, "after creating this one") {
		t.Errorf("Summary = %q, want it anchored to this resource's create", after.Summary)
	}
	// The welded phrasings, banned by name. Each states a combined narrative
	// that create_before_destroy makes false.
	for _, ann := range []*Annotation{before, after} {
		for _, wrong := range []string{"again afterwards", "comes back", "again after"} {
			if strings.Contains(ann.Summary, wrong) {
				t.Errorf("Summary = %q contains %q, which welds the two orderings into one "+
					"narrative", ann.Summary, wrong)
			}
		}
	}
}

// TestAPureDeleteClaimsNothingDownstream is the other half of the P1. A
// resource that is only destroyed has no create for a dependant to be ordered
// after, so no such claim is made - including for the delete of a DEPOSED
// object left behind by a failed create_before_destroy, where the dependant's
// create has in fact already happened.
func TestAPureDeleteClaimsNothingDownstream(t *testing.T) {
	seen := 0
	for _, fixture := range []string{"sequence-disjoint.json", "sequence-deposed.json"} {
		for _, f := range Assess(loadFixture(t, fixture)).Findings {
			if f.Kind != KindDelete {
				continue
			}
			seen++
			for _, a := range f.Annotations {
				if a.Code == AnnChangedAfter {
					t.Errorf("%s/%s is a pure delete and claims %q - it has no create for "+
						"anything to be ordered after", fixture, f.Address, a.Summary)
				}
			}
		}
	}
	if seen == 0 {
		t.Fatal("no pure delete was checked, so this test proves nothing")
	}
}

// TestALeafOrdersNothing checks the far end of the chain says nothing. Nothing
// depends on terraform_data.leaf, so there is no order to state, and an
// annotation saying "0 resources" under every finding is how a reader learns to
// skip the annotations - the same rule blast.go already follows.
func TestALeafOrdersNothing(t *testing.T) {
	before, after := both(t, Assess(loadFixture(t, "sequence-chain.json")), "terraform_data.leaf")
	if before != nil || after != nil {
		t.Error("terraform_data.leaf carries an ordering annotation and nothing depends on it")
	}
}

// TestANoOpIsNotInTheSequence keeps a resource this plan does not change out of
// an ordering claim about this plan. terraform_data.unrelated is a no-op, so
// there is no step of the apply it occupies.
func TestANoOpIsNotInTheSequence(t *testing.T) {
	r := Assess(loadFixture(t, "sequence-chain.json"))
	for _, f := range r.Findings {
		for _, a := range f.Annotations {
			if a.Code != AnnDestroyedBefore && a.Code != AnnChangedAfter {
				continue
			}
			for _, addr := range append(addressesOf(a.DestroyedFirst), addressesOf(a.ChangedAfter)...) {
				if addr == "terraform_data.unrelated" {
					t.Errorf("%s orders terraform_data.unrelated, which this plan does not change",
						f.Address)
				}
			}
		}
	}
}

// TestOnlyADestructiveChangeIsSequenced. An update in place does not order its
// dependants, on the same terms as the blast radius.
//
// THE FIRST FIXTURE IS THE ONE THAT MAKES THIS TEST MEAN ANYTHING. Removing the
// destructive guard left the whole suite green, because in every other fixture
// no non-destructive change HAD a dependant - so there was nothing for the
// guard to hold back.
func TestOnlyADestructiveChangeIsSequenced(t *testing.T) {
	base := findingFor(t, Assess(loadFixture(t, "sequence-update-with-dependant.json")), "terraform_data.base")
	if base.Kind != KindUpdate {
		t.Fatalf("terraform_data.base is a %s, so this fixture no longer carries a "+
			"non-destructive change with a dependant", base.Kind)
	}

	for _, fixture := range []string{
		"sequence-update-with-dependant.json", "sequence-chain.json", "demo.json", "real-plan.json",
	} {
		for _, f := range Assess(loadFixture(t, fixture)).Findings {
			for _, a := range f.Annotations {
				if a.Code != AnnDestroyedBefore && a.Code != AnnChangedAfter {
					continue
				}
				if f.Kind != KindDelete && f.Kind != KindReplace {
					t.Errorf("%s: %s is a %s and carries %s", fixture, f.Address, f.Kind, a.Code)
				}
			}
		}
	}
}

// TestTheSequenceNeverClaimsAvailability is the wording ban, and it is the
// whole reason this feature is shaped the way it is. See the file comment.
func TestTheSequenceNeverClaimsAvailability(t *testing.T) {
	banned := []string{
		"unavailable", "available", "reach", "unreachable", "offline",
		"outage", "downtime", "window", "does not exist", "gone for",
		"minute", "second", "how long", "duration",
	}
	checked := 0
	for _, fixture := range []string{
		"sequence-chain.json", "sequence-partial.json", "sequence-disjoint.json",
		"sequence-update.json", "sequence-deposed.json",
	} {
		for _, f := range Assess(loadFixture(t, fixture)).Findings {
			for _, a := range f.Annotations {
				if a.Code != AnnDestroyedBefore && a.Code != AnnChangedAfter {
					continue
				}
				checked++
				text := strings.ToLower(a.Detail + " " + a.Summary + " " + a.Note)
				for _, w := range banned {
					if strings.Contains(text, w) {
						t.Errorf("%s/%s: the sentence says %q, which is a claim about "+
							"availability or duration that the plan does not support: %q",
							fixture, f.Address, w, a.Detail)
					}
				}
			}
		}
	}
	if checked == 0 {
		// Fatal, not a silent pass. A wording ban that holds only while the
		// annotation exists goes green the moment the feature is deleted.
		t.Fatal("no ordering annotation was checked, so this test proves nothing")
	}
}

// TestTheSequenceSaysWhatItCannotSee carries the caveat, and the caveat carries
// three limits rather than one. The third was added after review: a dependency
// that IS written down and simply not read - depends_on, and any resource
// expanded by count or for_each - is not covered by "not written down", and #57
// tracks fixing the graph.
func TestTheSequenceSaysWhatItCannotSee(t *testing.T) {
	before, _ := both(t, Assess(loadFixture(t, "sequence-chain.json")), "terraform_data.base")
	if before == nil {
		t.Fatal("no ordering annotation")
	}
	if before.Note == "" {
		t.Fatal("the ordering annotation carries no standing caveat")
	}
	if !strings.Contains(before.Detail, before.Note) {
		t.Error("Detail does not end with the caveat, so a consumer reading Detail alone - a " +
			"JSON client, a markdown row - gets the claim without the limit on it")
	}
	note := strings.ToLower(before.Note)
	// THE BOUNDARY, NOT A LIST OF EXCEPTIONS. Naming three omissions read as
	// though they were the only ones; a dependency through a local, a module
	// or a data source is equally invisible. The caveat has to say the graph
	// is partial and why, so a reader cannot take the examples for a set.
	for _, want := range []string{
		"wrote down", "not ordered", "same time",
		"floor", "direct references", "local", "module", "data source",
		"depends_on", "for_each",
	} {
		if !strings.Contains(note, want) {
			t.Errorf("the caveat does not mention %q: %q", want, before.Note)
		}
	}
}

// TestTheCountsReadAsEnglish. terraform_data.middle has exactly one dependant,
// and the first version said "1 resources that depend on it". AGENTS.md names
// this case: "1 findings" is the kind of line that makes a reader trust nothing
// else on the page.
func TestTheCountsReadAsEnglish(t *testing.T) {
	before, _ := both(t, Assess(loadFixture(t, "sequence-chain.json")), "terraform_data.middle")
	if before == nil {
		t.Fatal("terraform_data.middle has no ordering annotation, and leaf depends on it")
	}
	if len(before.DestroyedFirst) != 1 {
		t.Fatalf("expected exactly 1 ordered dependant, got %d", len(before.DestroyedFirst))
	}
	if strings.Contains(before.Summary, "1 resources") {
		t.Errorf("Summary reads %q", before.Summary)
	}
	if !strings.Contains(before.Summary, "1 resource that depends on it") {
		t.Errorf("Summary = %q, want the singular subject", before.Summary)
	}
}

// TestEachListIsNamedWhenItIsAStrictSubset is why the naming decision is made
// per list rather than once for both.
//
// THE FIXTURE HAS THREE DEPENDANTS OF THREE DIFFERENT KINDS, and that is what
// makes this test able to fail. With one replaced dependant and one no-op, the
// destroyed and changed lists had identical membership, so taking one
// annotation's evidence from the OTHER list was undetectable - Astra changed
// namesFor(after, reached) to namesFor(destroyed, reached) and the whole suite
// stayed green. Adding an updated dependant makes the lists {rebuilt} and
// {rebuilt, updated}: different from each other, and each still a strict
// subset of the blast radius.
func TestEachListIsNamedWhenItIsAStrictSubset(t *testing.T) {
	r := Assess(loadFixture(t, "sequence-partial.json"))
	blast, ok := annotationFor(findingFor(t, r, "terraform_data.base"), AnnBlastRadius)
	if !ok {
		t.Fatal("no blast radius annotation")
	}
	before, after := both(t, r, "terraform_data.base")
	if before == nil || after == nil {
		t.Fatal("terraform_data.base is missing an ordering annotation")
	}

	// Non-vacuity: the two lists have to differ, or a test that swaps them
	// proves nothing.
	d, c := addressesOf(before.DestroyedFirst), addressesOf(after.ChangedAfter)
	if equalStrings(d, c) {
		t.Fatalf("the destroyed list %v and the changed list %v have the same membership, "+
			"so this fixture cannot catch evidence taken from the wrong one", d, c)
	}

	for _, c := range []struct {
		name string
		a    *Annotation
		want []string
	}{
		{"destroyed", before, d},
		{"changed", after, c},
	} {
		if len(c.want) >= len(blast.Paths) {
			t.Errorf("%s list %v is not a strict subset of the blast radius %v, so this "+
				"fixture no longer covers the case", c.name, c.want, blast.Paths)
		}
		// MEMBERSHIP, NOT LENGTH. Astra replaced the evidence with an invented
		// address and the old test passed, because it compared counts.
		if !equalStrings(c.a.Paths, c.want) {
			t.Errorf("%s: Paths = %v, want exactly the ordered addresses %v",
				c.name, c.a.Paths, c.want)
		}

		// AND THE COUNT IN THE SENTENCE, INDEPENDENTLY. Astra swapped
		// len(destroyed) and len(after) between the two summary builders and
		// the whole Go suite stayed green: the evidence lists were still
		// right, so every assertion above passed while the sentences said one
		// destroyed and two changed for a finding whose lists are the other
		// way round. A sentence and its evidence disagreeing is worse than
		// either being wrong alone, because each looks like it corroborates
		// the other.
		if !strings.Contains(c.a.Summary, dependants(len(c.want))) {
			t.Errorf("%s: Summary %q does not count %d, which is what its own evidence %v holds",
				c.name, c.a.Summary, len(c.want), c.want)
		}
	}
}

// TestTheListIsNotRepeatedWhenItCoversEverything. The blast radius sits directly
// above and lists the same addresses, and printing them again teaches a reader
// that the second annotation is decoration.
func TestTheListIsNotRepeatedWhenItCoversEverything(t *testing.T) {
	r := Assess(loadFixture(t, "sequence-chain.json"))
	blast, ok := annotationFor(findingFor(t, r, "terraform_data.base"), AnnBlastRadius)
	if !ok {
		t.Fatal("no blast radius annotation")
	}
	if !equalStrings(blast.Paths, []string{"terraform_data.middle", "terraform_data.leaf"}) {
		t.Fatalf("the blast radius no longer lists both dependants: %v", blast.Paths)
	}
	before, after := both(t, r, "terraform_data.base")
	for _, a := range []*Annotation{before, after} {
		if a == nil {
			t.Fatal("terraform_data.base is missing an ordering annotation")
		}
		if len(a.Paths) != 0 {
			t.Errorf("%s carries %v, and every one of them is already listed by the blast "+
				"radius directly above", a.Code, a.Paths)
		}
	}
}

// TestAnUpdatedDependantIsOrderedAfter is the branch that says "creates or
// updates" rather than only "creates", and it exists because the first version
// dropped updates entirely.
//
// Terraform orders a dependant's UPDATE after its dependency's create exactly
// as it orders a create. testdata/_gen/sequence-update applies as base.destroy,
// base.create, follower.modify - run rather than assumed.
func TestAnUpdatedDependantIsOrderedAfter(t *testing.T) {
	before, after := both(t, Assess(loadFixture(t, "sequence-update.json")), "terraform_data.base")
	if before != nil {
		t.Errorf("a destroyed-before annotation is present, and the only dependant is "+
			"updated in place rather than destroyed: %q", before.Summary)
	}
	if after == nil {
		t.Fatal("terraform_data.base has no changed-after annotation, and an updated " +
			"resource depends on it")
	}
	if got := addressesOf(after.ChangedAfter); !equalStrings(got, []string{"terraform_data.follower"}) {
		t.Errorf("ChangedAfter = %v, want the updated dependant", got)
	}
}

// TestADeposedObjectDoesNotOverwriteItsOwnResource. A real plan rather than a
// constructed one.
//
// An object is DEPOSED when a create_before_destroy apply creates the
// replacement and then fails before destroying the old one. The next plan
// carries TWO entries for that address: the resource's own replacement, and a
// delete for the deposed object. testdata/_gen/sequence-deposed generates
// exactly that, by failing a provisioner once.
//
// The change set was keyed by address, so the second entry overwrote the first
// and the resource read as a plain delete. Which one won depended on the order
// of the array, which is worse than either answer.
func TestADeposedObjectDoesNotOverwriteItsOwnResource(t *testing.T) {
	p := loadFixture(t, "sequence-deposed.json")

	// Non-vacuity: the fixture has to carry two entries for one address.
	seen := map[string]int{}
	for _, rc := range p.ResourceChanges {
		seen[rc.Address]++
	}
	if seen["terraform_data.svc"] != 2 {
		t.Fatalf("terraform_data.svc has %d entries, want 2 - this fixture no longer carries "+
			"a deposed object and the test proves nothing", seen["terraform_data.svc"])
	}

	before, after := both(t, Assess(p), "terraform_data.base")
	if before == nil || after == nil {
		t.Fatal("terraform_data.base is missing an ordering annotation, and svc depends on it")
	}
	if got := addressesOf(before.DestroyedFirst); !equalStrings(got, []string{"terraform_data.svc"}) {
		t.Errorf("DestroyedFirst = %v, want the dependant", got)
	}
	if got := addressesOf(after.ChangedAfter); !equalStrings(got, []string{"terraform_data.svc"}) {
		t.Errorf("ChangedAfter = %v, want the dependant - it is REPLACED, so it is created "+
			"after the base as well as destroyed before it. A deposed delete at the same "+
			"address must not take the replacement's place", got)
	}
}

func findingFor(t *testing.T, r Report, addr string) Finding {
	t.Helper()
	for _, f := range r.Findings {
		if f.Address == addr {
			return f
		}
	}
	t.Fatalf("no finding for %s", addr)
	return Finding{}
}

func addressesOf(in []Reached) []string {
	out := make([]string, 0, len(in))
	for _, r := range in {
		out = append(out, r.Address)
	}
	return out
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
