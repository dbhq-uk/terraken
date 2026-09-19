package assess

import (
	"sort"

	tfjson "github.com/hashicorp/terraform-json"
)

// What each resource actually changes.
//
// The report ranked a change and named the attributes that FORCED it, but never
// said what the change touches. A reviewer reading "update in place" with one
// reordering annotation under it had no idea whether the plan was editing a tag
// or a security rule, and that is the first question anybody asks.
//
// ATTRIBUTE NAMES ARE NOT VALUES, which is why this is allowed at all.
// AGENTS.md is explicit about what may be shown: paths, counts, levels and the
// tool's own sentences. Every name here comes from a key in the plan, and no
// value is read to produce any of them - the comparison below asks only whether
// two values differ, never what they are.
//
// TOP-LEVEL ONLY. A change to tags.owner is reported as `tags`. The roll-up in
// rewritten.go already counts top-level attributes for the same reason, which
// it states as "what a reader sees the plan show as changed"; and the nested
// paths that matter are already named by the annotations that care about them -
// what forced a replacement, what is unknown until apply, what is sensitive.
// Listing every leaf would bury those under the schema.

// changedAnnotation names what this change does to a resource's attributes, or
// returns false when there is nothing to name.
//
// THE VERB DEPENDS ON THE KIND, because the three cases are different facts. A
// create has no before, so every attribute is being set for the first time and
// "changed" would be wrong. A delete changes nothing at all - the resource goes
// - so what is useful is what it HELD, in the past tense. Only an update or a
// replacement has two sides to compare.
//
// NOTHING IS CLAIMED FOR AN OPERATION NOBODY READ. assessOne returns before
// this for KindUnsupported, and the switch below names every kind it handles
// rather than defaulting, so a kind added later arrives here as nothing rather
// than as a confident list.
func changedAnnotation(rc *tfjson.ResourceChange, kind Kind) (Annotation, bool) {
	var names []string
	var summary string

	switch kind {
	case KindCreate:
		names = attributeKeys(rc.Change.After, rc.Change.AfterUnknown)
		summary = "sets " + attributes(len(names))
	case KindDelete:
		names = attributeKeys(rc.Change.Before, nil)
		summary = "had " + attributes(len(names))
	case KindUpdate, KindReplace:
		names = differingKeys(rc.Change.Before, rc.Change.After, rc.Change.AfterUnknown)
		summary = "changes " + attributes(len(names))
	default:
		// A no-op, a read, an import, a forget. None of them changes an
		// attribute, and "changes 0 attributes" printed under every one of
		// them is how a reader is taught to skip the annotations.
		return Annotation{}, false
	}

	if len(names) == 0 {
		return Annotation{}, false
	}
	return Annotation{
		Code:    AnnChangedAttributes,
		Summary: summary,
		Detail:  summary + ". " + changedNote,
		Note:    changedNote,
		Paths:   names,
	}, true
}

// The caveat, and it is about precision rather than about trust.
//
// These are top-level attributes, so a reader who sees `tags` cannot tell
// whether one tag moved or all of them did, and a reader who sees a resource
// with one attribute changed should not conclude the change is small. Saying so
// costs one sentence and stops the count being read as a measure of size.
const changedNote = "These are the top-level attributes the plan shows as changing, so a change " +
	"inside one of them is counted once. The count is not a measure of how big the change is."

// attributes names a count of attributes, in the singular when there is one.
func attributes(n int) string {
	if n == 1 {
		return "1 attribute"
	}
	return itoa(n) + " attributes"
}

// attributeKeys is every top-level key on one side of a change.
//
// UNKNOWN ATTRIBUTES COUNT. Terraform drops an attribute whose value is not
// known until apply out of `after` entirely and marks it in `after_unknown`, so
// reading `after` alone would report a create as setting fewer attributes than
// it sets - and the ones it left out are exactly the ones nobody can check.
func attributeKeys(side, unknown interface{}) []string {
	seen := map[string]bool{}
	if m, ok := side.(map[string]interface{}); ok {
		for k := range m {
			seen[k] = true
		}
	}
	if m, ok := unknown.(map[string]interface{}); ok {
		for k, v := range m {
			// false means known, and Terraform writes it for an attribute it
			// has fully resolved. Only a true, or a nested object holding
			// one, says anything is unknown here.
			if v == nil || v == false {
				continue
			}
			seen[k] = true
		}
	}
	return sorted(seen)
}

// differingKeys is every top-level attribute whose before and after differ.
//
// It reuses jsonEqual from rewritten.go, so "differs" means what it means
// everywhere else in this package: the rendered values are not the same. An
// attribute present on one side only differs by definition - that is usually an
// unknown value, which Terraform removes from `after` rather than marking.
func differingKeys(before, after, unknown interface{}) []string {
	bm, bok := before.(map[string]interface{})
	am, aok := after.(map[string]interface{})
	if !bok || !aok {
		// One side is absent, so there is no pair to compare. Fall back to
		// whichever side exists rather than reporting nothing: a replacement
		// whose before is null still changes everything it sets.
		if bok {
			return attributeKeys(before, nil)
		}
		return attributeKeys(after, unknown)
	}

	seen := map[string]bool{}
	for _, k := range unionKeys(bm, am) {
		bv, inBefore := bm[k]
		av, inAfter := am[k]
		if inBefore && inAfter && jsonEqual(bv, av) {
			continue
		}
		seen[k] = true
	}
	// An attribute unknown until apply is dropped from `after`, so the union
	// above already has it from `before`. One that exists on neither side and
	// is only in after_unknown is new and unknown, and belongs here too.
	if m, ok := unknown.(map[string]interface{}); ok {
		for k, v := range m {
			if v == nil || v == false {
				continue
			}
			seen[k] = true
		}
	}
	return sorted(seen)
}

func sorted(set map[string]bool) []string {
	out := make([]string, 0, len(set))
	for k := range set {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
