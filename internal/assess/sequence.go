package assess

import tfjson "github.com/hashicorp/terraform-json"

// What this plan does before what.
//
// Blast radius (blast.go) answers "what else depends on this". It does not
// answer the question a reviewer asks next, which is WHEN - and the same graph
// plus the change set gives it, because Terraform's apply ordering is
// determined by the dependency graph rather than chosen at run time.
//
// TWO CLAIMS, AND THE TOOL MAKES NO OTHERS:
//
//   - a resource is DESTROYED BEFORE the things it depends on
//   - a resource is CREATED OR UPDATED AFTER the things it depends on
//
// Both are Terraform's documented ordering, and both were checked by running
// it rather than by citing it. A three-resource chain applies as
// leaf.destroy, middle.destroy, base.destroy, base.create, middle.create,
// leaf.create - the whole chain torn down before any of it is rebuilt. An
// UPDATED dependant is ordered the same way: base.destroy, base.create,
// follower.modify. The roots and the observed output are in testdata/_gen.
//
// THEY HOLD WHICHEVER WAY ROUND A REPLACEMENT HAPPENS, which is what makes
// them safe to state without reading the lifecycle rule. create_before_destroy
// moves the resource's OWN two steps relative to each other - with it set, the
// old object is destroyed last, after its dependents have been rebuilt - and
// that is #43's business, stated separately on the same finding. It does not
// move a dependent's destroy after its dependency's destroy, and it does not
// move a dependent's create before its dependency's create. Both were run.
//
// WHAT IT DELIBERATELY DOES NOT SAY. #32 offers "six resources depend on this
// and cannot be reached until it is recreated" as a fact this tool may state.
// It is not. "Cannot be reached" is a claim about the world: the plan states
// the order of operations and says nothing about whether a provider's destroy
// takes the thing away, nor whether anything was reaching it. The same
// counterexample that cut eleven such claims out of #43 - a local_file
// replacement that ends with the file deleted - cuts this one. So there is no
// window here, no outage, no duration and no availability. There is an order,
// which is what the file actually holds.

// sequenceAnnotation describes what this plan orders around one destructive
// change, or returns false when it orders nothing around it.
//
// ONLY DESTRUCTIVE CHANGES, on the same terms as the blast radius: an update
// in place neither takes its dependants down nor holds them up, so ordering
// one would be noise dressed as a warning.
//
// A change that orders NOTHING is not annotated with an empty sequence. A
// resource nothing depends on, or whose dependants this plan leaves alone, has
// no order to report, and "0 resources" printed under thirty findings teaches
// a reader to skip the annotations entirely.
func sequenceAnnotation(g *graph, addr string, kind Kind, steps map[string]step) (Annotation, bool) {
	switch kind {
	case KindDelete, KindReplace:
	default:
		return Annotation{}, false
	}

	reached := g.reach(addr)
	var destroyed, after, involved []Reached
	for _, r := range reached {
		st := steps[r.Address]
		// A dependant this plan does not change occupies no step of this
		// apply, so there is nothing to order it against. It is still in the
		// blast radius, which counts what depends on this rather than what
		// this plan does.
		if !st.destroys && !st.changes {
			continue
		}
		// NAMED ONCE, in nearest-first order, even though a replaced
		// dependant is in both lists below. Paths is what a renderer prints
		// under the sentence, and printing an address twice would read as two
		// resources.
		involved = append(involved, r)
		if st.destroys {
			destroyed = append(destroyed, r)
		}
		if st.changes {
			after = append(after, r)
		}
	}
	if len(destroyed) == 0 && len(after) == 0 {
		return Annotation{}, false
	}

	// NAMED ONLY WHEN IT IS A STRICT SUBSET. The blast-radius annotation sits
	// directly above this one on the same finding and lists everything that
	// depends on this resource, and the ordered set is always a subset of it.
	// Printing the same addresses again teaches a reader that the second
	// annotation is decoration. When some dependants are not in this plan at
	// all, the counts no longer answer "which ones", so then they are named.
	var paths []string
	switch {
	case len(destroyed) > 0 && len(after) > 0 && !sameAddresses(destroyed, after):
		// THE COUNTS DO NOT SAY WHICH IS WHICH. When different resources go
		// before this one and come after it, "destroys 1, changes 1" leaves a
		// reader unable to tell them apart, and the blast radius above lists
		// both without saying which is which either. The destroyed ones are
		// named, because "what goes before this and is not stated to come
		// back" is the half a reviewer is reading for.
		paths = addressesIn(destroyed)
	case len(involved) != len(reached):
		// A strict subset of the blast radius: some dependants are not in
		// this plan at all, so the counts no longer answer "which ones".
		paths = addressesIn(involved)
	}

	summary := sequenceSummary(destroyed, after)
	return Annotation{
		Code:           AnnSequence,
		Summary:        summary,
		Detail:         summary + ". " + sequenceNote,
		Note:           sequenceNote,
		Paths:          paths,
		DestroyedFirst: destroyed,
		ChangedAfter:   after,
	}, true
}

// sequenceSummary states the order and the counts, and nothing following from
// them.
//
// THE SENTENCE DOES NOT CHANGE WITH `whole`. An earlier draft said "all of
// them" when the ordered set covered the whole blast radius, and that was
// wrong as soon as the two lists differed from each other: a plan that
// replaces one dependant and updates another has every dependant in the union,
// so "destroys all of them" was said about a count that was not all of them.
// Only whether the addresses are listed depends on `whole`; the counts are
// always explicit.
func sequenceSummary(destroyed, changed []Reached) string {
	d, c := len(destroyed), len(changed)
	switch {
	// THE SAME RESOURCES, NOT THE SAME COUNT. This branch says they come
	// back, and keying it on the counts being equal made it a false
	// statement: a plan that deletes one dependant permanently and creates a
	// different one has one of each, and the report said the deleted one was
	// changed again afterwards. Sets, compared by address.
	case d > 0 && sameAddresses(destroyed, changed):
		return "this plan destroys " + dependants(d) + " before this one, " +
			"and changes " + them(d) + " again afterwards"
	case d > 0 && c > 0:
		// "OTHER", because the sets are not the same - the branch above took
		// that case. Without it the sentence reads as the destroyed resources
		// coming back, which is the error this whole comparison exists for.
		return "this plan destroys " + dependants(d) + " before this one, " +
			"and changes " + other(c) + " after it"
	case d > 0:
		return "this plan destroys " + dependants(d) + " before this one"
	default:
		return "this plan changes " + dependants(c) + " after this one"
	}
}

// sameAddresses reports whether two reached lists name the same resources.
// Both come from one pass over g.reach in its order, so a position-by-position
// comparison is enough and no sorting is needed.
func sameAddresses(a, b []Reached) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i].Address != b[i].Address {
			return false
		}
	}
	return true
}

// dependants names a count of resources that depend on this one, with the verb
// agreeing. "1 resources that depend on it" is the kind of line that makes a
// reader trust nothing else on the page, and AGENTS.md names the case.
func dependants(n int) string {
	if n == 1 {
		return "1 resource that depends on it"
	}
	return itoa(n) + " resources that depend on it"
}

// other names a count of DIFFERENT resources, so a reader cannot take the
// second clause for the first set coming back.
func other(n int) string {
	if n == 1 {
		return "1 other"
	}
	return itoa(n) + " others"
}

// them is the pronoun for that same count, so the second clause agrees with
// the first.
func them(n int) string {
	if n == 1 {
		return "it"
	}
	return "them"
}

// addressesIn is the addresses of a reached list, in the order given.
func addressesIn(in []Reached) []string {
	out := make([]string, 0, len(in))
	for _, r := range in {
		out = append(out, r.Address)
	}
	return out
}

// The caveat, and it carries two limits rather than one.
//
// The first is the graph's, word for word the same limit the blast radius
// has: this is what the configuration declares, so a dependency nobody wrote
// down orders nothing here.
//
// The second belongs to this annotation alone. Terraform walks the graph in
// parallel, so what is stated is the order it HAS to respect, not the only
// order it will produce. Two resources with no dependency between them have no
// order at all, and a reader who took this for a timeline would be reading
// something the plan does not say.
const sequenceNote = "This is the order Terraform's dependency rules require, counted within this plan. " +
	"Steps with no dependency between them are not ordered against each other and may run at the same time, " +
	"and a dependency that is not written down is not here at all."

// step is what this plan does at one address, as the two things ordering cares
// about rather than as a kind.
//
// TWO BOOLEANS AND NOT ONE KIND, because an address can carry more than one
// operation. A replacement both destroys and creates. So does an address whose
// DEPOSED object is being deleted while the resource itself is replaced -
// Terraform emits two entries there, sharing one address, and storing a kind
// per address meant the second silently replaced the first. Which one won
// depended on the order of the array, so the report moved with the plan's
// formatting rather than with the plan.
type step struct {
	destroys bool
	changes  bool
}

// stepsByAddress is what this plan does at each address, ACCUMULATED over every
// entry for it.
//
// Built once for the whole plan rather than searched per finding, like the
// graph beside it.
func stepsByAddress(p *tfjson.Plan) map[string]step {
	out := make(map[string]step, len(p.ResourceChanges))
	for _, rc := range p.ResourceChanges {
		if rc == nil || rc.Change == nil {
			continue
		}
		k, _ := classify(rc)
		st := out[rc.Address]
		switch k {
		case KindDelete:
			st.destroys = true
		case KindCreate, KindUpdate:
			// AN UPDATE COUNTS, and it was left out of the first version.
			// Terraform orders a dependant's update after its dependency's
			// create exactly as it orders a create - base.destroy,
			// base.create, follower.modify, which was run rather than
			// assumed. Dropping it lost a true fact about the apply.
			st.changes = true
		case KindReplace:
			st.destroys, st.changes = true, true
		default:
			// AN UNREADABLE OPERATION ORDERS NOTHING, and neither does a
			// no-op, a read, an import or a forget. classify ends at
			// KindUnsupported when it cannot read the action, and a resource
			// whose operation is unknown cannot be placed in a sequence -
			// putting it in one would be the confident guess that kind exists
			// to refuse. Note this never CLEARS a flag another entry at the
			// same address set: one unreadable object does not unsay what a
			// readable one at that address is doing.
			continue
		}
		out[rc.Address] = st
	}
	return out
}
