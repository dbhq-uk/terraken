package assess

import tfjson "github.com/hashicorp/terraform-json"

// What this plan does before what.
//
// Blast radius (blast.go) answers "what else depends on this". It does not
// answer the question a reviewer asks next, which is WHEN - and the same graph
// plus the change set gives it, because Terraform's apply ordering follows the
// dependency graph rather than being chosen at run time.
//
// TWO CLAIMS, REPORTED SEPARATELY, AND THE TOOL MAKES NO OTHERS:
//
//   - a dependant is destroyed BEFORE this resource is destroyed
//   - a dependant is created or updated AFTER this resource is created
//
// Both were checked by running Terraform rather than by citing it. A
// three-resource chain applies as leaf.destroy, middle.destroy, base.destroy,
// base.create, middle.create, leaf.create; an updated dependant is ordered the
// same way, base.destroy, base.create, follower.modify. The roots and the
// observed output are in testdata/_gen.
//
// EACH CLAUSE IS ANCHORED TO ONE STEP OF THIS RESOURCE, and that is what makes
// them safe to state together. An earlier version welded them into one
// sentence - "destroys these before this one, and changes them again
// afterwards" - which quietly assumed this resource's destroy comes before its
// create. With create_before_destroy set along a chain it does not: the creates
// run first and the old objects go afterwards, so the welded sentence
// described the apply backwards. Anchoring each clause to "before destroying
// this one" and "after creating this one" is true whichever way round the
// replacement happens, which is why the ordering from #43 is stated separately
// on the same finding rather than folded in here.
//
// THE SECOND CLAUSE NEEDS THIS RESOURCE TO BE CREATED AT ALL. A pure delete -
// including the delete of a DEPOSED object left behind by a failed
// create_before_destroy - has no create for anything to be ordered after, so
// nothing downstream is claimed. Reporting one there put a dependant's create
// after a step that does not exist, and in a real recovery plan that create had
// already happened first.
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

// sequenceAnnotations describes what this plan orders around one destructive
// change. It returns nothing when it orders nothing.
//
// ONLY DESTRUCTIVE CHANGES, on the same terms as the blast radius: an update in
// place does not order its dependants, so annotating one would be noise dressed
// as a warning.
//
// A change that orders NOTHING is not annotated with an empty sequence. A
// resource nothing depends on, or whose dependants this plan leaves alone, has
// no order to report, and "0 resources" printed under thirty findings teaches a
// reader to skip the annotations entirely.
func sequenceAnnotations(g *graph, addr string, kind Kind, steps map[string]step) []Annotation {
	switch kind {
	case KindDelete, KindReplace:
	default:
		return nil
	}

	reached := g.reach(addr)
	var destroyed, after []Reached
	for _, r := range reached {
		st := steps[r.Address]
		if st.destroys {
			destroyed = append(destroyed, r)
		}
		if st.changes {
			after = append(after, r)
		}
		// A dependant this plan does not change occupies no step of this
		// apply, so there is nothing to order it against. It stays in the
		// blast radius, which counts what depends on this resource rather than
		// what this plan does to it.
	}

	var out []Annotation
	if len(destroyed) > 0 {
		line := "this plan destroys " + dependants(len(destroyed)) + " before destroying this one"
		out = append(out, Annotation{
			Code:           AnnDestroyedBefore,
			Summary:        line,
			Detail:         line + ". " + sequenceNote,
			Note:           sequenceNote,
			Paths:          namesFor(destroyed, reached),
			DestroyedFirst: destroyed,
		})
	}
	// ONLY WHEN THIS RESOURCE IS CREATED. See the file comment: a pure delete
	// has no create for a dependant to be ordered after.
	if kind == KindReplace && len(after) > 0 {
		line := "this plan creates or updates " + dependants(len(after)) + " after creating this one"
		out = append(out, Annotation{
			Code:         AnnChangedAfter,
			Summary:      line,
			Detail:       line + ". " + sequenceNote,
			Note:         sequenceNote,
			Paths:        namesFor(after, reached),
			ChangedAfter: after,
		})
	}
	return out
}

// namesFor is the addresses to print under one of these sentences, or nil when
// the blast-radius annotation above has already printed them.
//
// The blast radius sits directly above on the same finding and lists everything
// that depends on this resource, and an ordered list is always a subset of it.
// Printing the same addresses again teaches a reader that the second annotation
// is decoration. When the ordered list is a STRICT subset the count no longer
// answers "which ones", so then they are named.
//
// PER LIST, NOT ONCE FOR BOTH. An earlier version decided this once for the
// union of the two lists, which hid exactly the case that needs naming: a plan
// with one dependant replaced, one updated and one untouched has both lists
// differing from each other AND from the blast radius, and one decision for the
// union printed one list under a sentence about the other.
func namesFor(list, reached []Reached) []string {
	if len(list) == len(reached) {
		return nil
	}
	out := make([]string, 0, len(list))
	for _, r := range list {
		out = append(out, r.Address)
	}
	return out
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

// The caveat, and it says the graph is PARTIAL rather than listing exceptions.
//
// An earlier version named depends_on, count and for_each, which read as
// though those were the omissions. They are not. Astra built a plan whose
// dependants reach the same resource directly, through a local, through a
// module input and through that module's output; Terraform updates all four
// after creating it and terraken named one. A reference through a data source
// is dropped as well.
//
// The rule underneath all of them is the same: this reads
// `configuration.expressions[].references` and follows nothing else, so a
// dependency that travels through anything on its way is not in the graph.
// Enumerating the ways that can happen invites a reader to assume the list is
// complete, and it is not - so the sentence states the boundary and gives
// examples rather than a set. #57 is the work to widen it.
//
// The other two limits stand as they were. The graph is what the configuration
// declares, so a dependency nobody wrote down is not in it at all. And
// Terraform walks the graph in parallel, so what is stated is the order it HAS
// to respect rather than the only order it will produce - two resources with
// no dependency between them have no order, and a reader who took this for a
// timeline would be reading something the plan does not say.
const sequenceNote = "This is the order Terraform's dependency rules require, counted within this plan. " +
	"Steps with no dependency between them are not ordered against each other and may run at the same time. " +
	"It is read only from the direct references between resources in the configuration, so it is a floor " +
	"rather than the whole graph: a dependency that travels through a local, a module, a data source or " +
	"depends_on is not in it, nor is one to a resource expanded by count or for_each, nor one nobody wrote down."

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
