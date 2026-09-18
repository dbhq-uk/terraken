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
// So the report names ORDER and stops. Two claims, both from Terraform's
// documented ordering and both verified by running it:
//
//   - a resource is destroyed BEFORE the things it depends on
//   - a resource is created AFTER the things it depends on
//
// Both hold whichever way round a replacement happens, which is what makes
// them safe to state without branching on the lifecycle rule. See
// testdata/_gen/sequence-chain for the roots and the observed apply order.

// TestAChainIsOrderedAroundTheResourceItDependsOn is the feature, on a real
// plan. terraform_data.middle and .leaf both depend on .base, all three are
// replaced, and .unrelated is a no-op that must stay out of it.
func TestAChainIsOrderedAroundTheResourceItDependsOn(t *testing.T) {
	r := Assess(loadFixture(t, "sequence-chain.json"))

	f := findingFor(t, r, "terraform_data.base")
	a, ok := annotationFor(f, AnnSequence)
	if !ok {
		t.Fatalf("terraform_data.base has no %s annotation, and two resources in this plan "+
			"depend on it", AnnSequence)
	}

	if got := addressesOf(a.DestroyedFirst); !equalStrings(got, []string{
		"terraform_data.middle", "terraform_data.leaf",
	}) {
		t.Errorf("DestroyedFirst = %v, want middle then leaf - nearest first, and a resource "+
			"is destroyed before the thing it depends on", got)
	}
	if got := addressesOf(a.ChangedAfter); !equalStrings(got, []string{
		"terraform_data.middle", "terraform_data.leaf",
	}) {
		t.Errorf("ChangedAfter = %v, want middle then leaf", got)
	}
}

// TestALeafOrdersNothing checks the far end of the chain says nothing. Nothing
// depends on terraform_data.leaf, so there is no order to state, and an
// annotation saying "0 resources" under every finding is how a reader learns to
// skip the annotations - the same rule blast.go already follows.
func TestALeafOrdersNothing(t *testing.T) {
	r := Assess(loadFixture(t, "sequence-chain.json"))
	f := findingFor(t, r, "terraform_data.leaf")
	if _, ok := annotationFor(f, AnnSequence); ok {
		t.Error("terraform_data.leaf carries a sequence annotation and nothing depends on it")
	}
}

// TestANoOpIsNotInTheSequence keeps a resource this plan does not change out of
// an ordering claim about this plan. terraform_data.unrelated is a no-op, so
// there is no step of the apply it occupies.
func TestANoOpIsNotInTheSequence(t *testing.T) {
	r := Assess(loadFixture(t, "sequence-chain.json"))
	for _, f := range r.Findings {
		a, ok := annotationFor(f, AnnSequence)
		if !ok {
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

// TestOnlyADestructiveChangeIsSequenced. An update in place does not take its
// dependants down with it, on the same terms as the blast radius: the ordering
// is reported where destruction makes it matter.
//
// THE FIRST FIXTURE IS THE ONE THAT MAKES THIS TEST MEAN ANYTHING. Removing
// the destructive guard from sequenceAnnotation left the whole suite green,
// because in every fixture this test read, no non-destructive change HAD a
// dependant - so there was nothing for the guard to hold back. In
// sequence-update-with-dependant.json terraform_data.base is updated in place
// and terraform_data.follower is replaced because of it, which is exactly the
// shape the guard exists for.
func TestOnlyADestructiveChangeIsSequenced(t *testing.T) {
	// Non-vacuity first: the fixture has to hold the case, or the loop below
	// proves nothing.
	base := findingFor(t, Assess(loadFixture(t, "sequence-update-with-dependant.json")), "terraform_data.base")
	if base.Kind != KindUpdate {
		t.Fatalf("terraform_data.base is a %s, so this fixture no longer carries a "+
			"non-destructive change with a dependant", base.Kind)
	}

	for _, fixture := range []string{
		"sequence-update-with-dependant.json", "sequence-chain.json", "demo.json", "real-plan.json",
	} {
		r := Assess(loadFixture(t, fixture))
		for _, f := range r.Findings {
			if _, ok := annotationFor(f, AnnSequence); !ok {
				continue
			}
			if f.Kind != KindDelete && f.Kind != KindReplace {
				t.Errorf("%s: %s is a %s and carries a sequence annotation", fixture, f.Address, f.Kind)
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
	r := Assess(loadFixture(t, "sequence-chain.json"))
	checked := 0
	for _, f := range r.Findings {
		a, ok := annotationFor(f, AnnSequence)
		if !ok {
			continue
		}
		checked++
		text := strings.ToLower(a.Detail + " " + a.Summary + " " + a.Note)
		for _, w := range banned {
			if strings.Contains(text, w) {
				t.Errorf("%s: the sequence sentence says %q, which is a claim about "+
					"availability or duration that the plan does not support: %q",
					f.Address, w, a.Detail)
			}
		}
	}
	if checked == 0 {
		// Fatal, not a silent pass. A wording ban that holds only while the
		// annotation exists goes green the moment the feature is deleted.
		t.Fatal("no sequence annotation was checked, so this test proves nothing")
	}
}

// TestTheSequenceSaysItIsOnlyWhatIsWrittenDown carries the same floor-not-
// measurement caveat the blast radius does, because it is the same graph. An
// implicit dependency nobody wrote down orders nothing here.
func TestTheSequenceSaysItIsOnlyWhatIsWrittenDown(t *testing.T) {
	r := Assess(loadFixture(t, "sequence-chain.json"))
	f := findingFor(t, r, "terraform_data.base")
	a, ok := annotationFor(f, AnnSequence)
	if !ok {
		t.Fatal("no sequence annotation")
	}
	if a.Note == "" {
		t.Fatal("the sequence annotation carries no standing caveat")
	}
	if !strings.Contains(a.Detail, a.Note) {
		t.Error("Detail does not end with the caveat, so a consumer reading Detail alone - a " +
			"JSON client, a markdown row - gets the claim without the limit on it")
	}
	// The caveat carries TWO limits and both are load-bearing, so both are
	// checked rather than the sentence being pinned word for word.
	//
	// The first is the graph's, shared with the blast radius: a dependency
	// nobody wrote down orders nothing here. The second belongs to this
	// annotation alone - Terraform walks independent steps at the same time,
	// so what is stated is the order it HAS to respect rather than the order
	// it will produce, and a reader who took it for a timeline would be
	// reading something the plan does not say.
	note := strings.ToLower(a.Note)
	if !strings.Contains(note, "not written down") {
		t.Errorf("the caveat does not say the graph is only what the configuration "+
			"declares: %q", a.Note)
	}
	if !strings.Contains(note, "not ordered") {
		t.Errorf("the caveat does not say that steps with no dependency between them are "+
			"unordered: %q", a.Note)
	}
	if !strings.Contains(note, "same time") && !strings.Contains(note, "parallel") {
		t.Errorf("the caveat does not say those steps may run together, so a reader can "+
			"still take the list for a timeline: %q", a.Note)
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

// TestTheSequenceCountsReadAsEnglish. terraform_data.middle has exactly one
// dependant, and the first version of this said "1 resources that depend on
// it". AGENTS.md names this case: "1 findings" is the kind of line that makes
// a reader trust nothing else on the page.
func TestTheSequenceCountsReadAsEnglish(t *testing.T) {
	r := Assess(loadFixture(t, "sequence-chain.json"))
	f := findingFor(t, r, "terraform_data.middle")
	a, ok := annotationFor(f, AnnSequence)
	if !ok {
		t.Fatal("terraform_data.middle has no sequence annotation, and leaf depends on it")
	}
	if len(a.DestroyedFirst) != 1 {
		t.Fatalf("expected exactly 1 ordered dependant, got %d", len(a.DestroyedFirst))
	}
	for _, wrong := range []string{"1 resources", "resources that depends", "changes them again"} {
		if strings.Contains(a.Summary, wrong) {
			t.Errorf("Summary reads %q, which contains %q", a.Summary, wrong)
		}
	}
	if !strings.Contains(a.Summary, "1 resource that depends on it") {
		t.Errorf("Summary = %q, want the singular subject", a.Summary)
	}
	if !strings.Contains(a.Summary, "changes it again") {
		t.Errorf("Summary = %q, want the pronoun to agree with the singular subject", a.Summary)
	}
}

// TestTheSequenceDoesNotRepeatTheBlastRadius. The two annotations sit next to
// each other on the same finding and read off the same graph, and the ordered
// set is always a subset of what the blast radius already listed. Printing the
// same two addresses twice in a row is how a reader is taught that the second
// annotation is decoration.
//
// So the names are carried only when the ordered set is a STRICT subset -
// which is the case where the counts alone leave "which ones" unanswered. The
// machine fields carry them either way.
func TestTheSequenceDoesNotRepeatTheBlastRadius(t *testing.T) {
	r := Assess(loadFixture(t, "sequence-chain.json"))
	f := findingFor(t, r, "terraform_data.base")

	blast, ok := annotationFor(f, AnnBlastRadius)
	if !ok {
		t.Fatal("no blast radius annotation")
	}
	seq, ok := annotationFor(f, AnnSequence)
	if !ok {
		t.Fatal("no sequence annotation")
	}
	if !equalStrings(blast.Paths, []string{"terraform_data.middle", "terraform_data.leaf"}) {
		t.Fatalf("the blast radius no longer lists both dependants: %v", blast.Paths)
	}
	if len(seq.Paths) != 0 {
		t.Errorf("Paths = %v, but every one of them is already listed by the blast radius "+
			"directly above", seq.Paths)
	}
	// The machine fields still carry the whole thing, because a consumer has
	// no annotation above to read.
	if !equalStrings(addressesOf(seq.DestroyedFirst), []string{"terraform_data.middle", "terraform_data.leaf"}) {
		t.Errorf("DestroyedFirst = %v, want both", addressesOf(seq.DestroyedFirst))
	}
	// The counts are explicit whether or not the addresses are listed. An
	// earlier draft said "all of them" in this branch, and it was wrong the
	// moment the two lists differed from each other: a plan that replaces one
	// dependant and updates another has every dependant in the union, so "all
	// of them" got said about a count that was not all of them.
	if !strings.Contains(seq.Summary, "2 resources that depend on it") {
		t.Errorf("Summary = %q, want an explicit count", seq.Summary)
	}
	if strings.Contains(seq.Summary, "all of them") {
		t.Errorf("Summary = %q - \"all\" is a claim about a set, and these two counts can "+
			"differ from each other and from the blast radius", seq.Summary)
	}
}

// TestTheSequenceNamesASubset is the other half. When this plan changes only
// some of what depends on a resource, the counts no longer answer "which
// ones", so the ordered ones are named.
func TestTheSequenceNamesASubset(t *testing.T) {
	r := Assess(loadFixture(t, "sequence-partial.json"))
	f := findingFor(t, r, "terraform_data.base")

	blast, ok := annotationFor(f, AnnBlastRadius)
	if !ok {
		t.Fatal("no blast radius annotation")
	}
	seq, ok := annotationFor(f, AnnSequence)
	if !ok {
		t.Fatal("no sequence annotation")
	}
	if len(seq.Paths) == 0 {
		t.Fatal("the ordered set is a strict subset of the blast radius and is not named")
	}
	if len(seq.Paths) >= len(blast.Paths) {
		t.Errorf("seq.Paths %v is not a strict subset of blast.Paths %v, so this fixture no "+
			"longer covers the case", seq.Paths, blast.Paths)
	}
	if strings.Contains(seq.Summary, "all of them") {
		t.Errorf("Summary = %q, but the ordering covers only some of what depends on it",
			seq.Summary)
	}
}

// TestAnUpdatedDependantIsOrderedAfter is the branch that says "changed" and
// not "destroyed", and it exists because the first version of this feature
// dropped updates entirely.
//
// Terraform orders a dependant's UPDATE after its dependency's create exactly
// as it orders a create. testdata/_gen/sequence-update applies as
// base.destroy, base.create, follower.modify - run rather than assumed - so a
// report that named only the destroyed and created dependants was leaving out
// a step of the apply it was claiming to describe.
func TestAnUpdatedDependantIsOrderedAfter(t *testing.T) {
	r := Assess(loadFixture(t, "sequence-update.json"))
	f := findingFor(t, r, "terraform_data.base")
	a, ok := annotationFor(f, AnnSequence)
	if !ok {
		t.Fatal("terraform_data.base has no sequence annotation, and an updated resource " +
			"depends on it")
	}
	if len(a.DestroyedFirst) != 0 {
		t.Errorf("DestroyedFirst = %v, want none - the dependant is updated in place, not "+
			"destroyed", addressesOf(a.DestroyedFirst))
	}
	if got := addressesOf(a.ChangedAfter); !equalStrings(got, []string{"terraform_data.follower"}) {
		t.Errorf("ChangedAfter = %v, want the updated dependant", got)
	}
	if !strings.Contains(a.Summary, "changes 1 resource that depends on it after this one") {
		t.Errorf("Summary = %q", a.Summary)
	}
}

// TestEqualCountsAreNotTheSameResources is a false statement the first version
// made, found by Astra before it had finished reading the branch.
//
// The summary had a branch for "the same resources go down before this one and
// come back after it", and it keyed on the two COUNTS being equal. Counts being
// equal does not make the sets the same. In this fixture terraform_data.base is
// replaced, terraform_data.gone is deleted and never returns, and a DIFFERENT
// resource, terraform_data.fresh, is created - so destroyed and changed are
// both 1 and the sets are disjoint. The report said the destroyed one was
// "changed again afterwards", which tells a reviewer that a resource this plan
// deletes permanently comes back.
func TestEqualCountsAreNotTheSameResources(t *testing.T) {
	r := Assess(loadFixture(t, "sequence-disjoint.json"))
	f := findingFor(t, r, "terraform_data.base")
	a, ok := annotationFor(f, AnnSequence)
	if !ok {
		t.Fatal("no sequence annotation")
	}

	if got := addressesOf(a.DestroyedFirst); !equalStrings(got, []string{"terraform_data.gone"}) {
		t.Fatalf("DestroyedFirst = %v, want only the deleted dependant", got)
	}
	if got := addressesOf(a.ChangedAfter); !equalStrings(got, []string{"terraform_data.fresh"}) {
		t.Fatalf("ChangedAfter = %v, want only the created dependant", got)
	}

	for _, wrong := range []string{"again", "them again", "changes it again"} {
		if strings.Contains(a.Summary, wrong) {
			t.Errorf("Summary = %q, which says the destroyed resource comes back. It does "+
				"not: %v is deleted and %v is a different resource being created",
				a.Summary, addressesOf(a.DestroyedFirst), addressesOf(a.ChangedAfter))
		}
	}

	// AND THE COUNTS ALONE ARE NOT ENOUGH HERE. Two resources depend on the
	// base, one is destroyed before it and a different one is changed after
	// it, so "destroys 1, changes 1" leaves a reader unable to tell which is
	// which. The destroyed list is named, and the other count says "other" so
	// it cannot be read as the same resource coming back.
	if !equalStrings(a.Paths, []string{"terraform_data.gone"}) {
		t.Errorf("Paths = %v, want the destroyed dependant named - the counts do not say "+
			"which of the two it is", a.Paths)
	}
	if !strings.Contains(a.Summary, "other") {
		t.Errorf("Summary = %q, want it to say the resources changed after are OTHERS, "+
			"since they are not the ones destroyed before", a.Summary)
	}
}

// TestTheSameResourcesComingBackStillSaysSo is the other side, so the fix above
// cannot be "never say it". On the chain fixture every dependant really is
// destroyed before and created after, and that is worth one sentence rather
// than two counts.
func TestTheSameResourcesComingBackStillSaysSo(t *testing.T) {
	r := Assess(loadFixture(t, "sequence-chain.json"))
	a, ok := annotationFor(findingFor(t, r, "terraform_data.base"), AnnSequence)
	if !ok {
		t.Fatal("no sequence annotation")
	}
	if !strings.Contains(a.Summary, "again") {
		t.Errorf("Summary = %q - here the destroyed and changed sets ARE the same two "+
			"resources, so the sentence should say they come back", a.Summary)
	}
}

// TestADeposedObjectDoesNotOverwriteItsOwnResource. Astra's second lead, and a
// real plan rather than a constructed one.
//
// An object is DEPOSED when a create_before_destroy apply creates the
// replacement and then fails before destroying the old one. The next plan
// carries TWO entries for that address: the resource's own replacement, and a
// delete for the deposed object. testdata/_gen/sequence-deposed generates
// exactly that, by failing a provisioner once.
//
// The change set was keyed by address, so the second entry overwrote the
// first and the resource read as a plain delete. Which one won depended on the
// order of the array, which is worse than either answer: terraform_data.svc is
// replaced, so it is destroyed before the base AND created after it, and the
// report dropped the second half.
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

	a, ok := annotationFor(findingFor(t, Assess(p), "terraform_data.base"), AnnSequence)
	if !ok {
		t.Fatal("terraform_data.base has no sequence annotation, and svc depends on it")
	}
	if got := addressesOf(a.DestroyedFirst); !equalStrings(got, []string{"terraform_data.svc"}) {
		t.Errorf("DestroyedFirst = %v, want the dependant", got)
	}
	if got := addressesOf(a.ChangedAfter); !equalStrings(got, []string{"terraform_data.svc"}) {
		t.Errorf("ChangedAfter = %v, want the dependant - it is REPLACED, so it is created "+
			"after the base as well as destroyed before it. A deposed delete at the same "+
			"address must not take the replacement's place", got)
	}
}
