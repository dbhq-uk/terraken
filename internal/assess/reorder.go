package assess

import (
	"encoding/json"
	"fmt"
	"sort"
)

// reorderedPaths returns the attribute paths whose before and after hold
// the same elements in a different order.
//
// Terraform reports an attribute as changed whenever the rendered before
// and after differ, and a provider that returns a set as a JSON list -
// service_endpoints, address_prefixes, cidr_blocks, subnet_ids,
// availability_zones and many more - will happily return the same members
// in a different order on every read. The plan shows a diff, a reviewer
// reads a change.
//
// What is reported is the fact, never a verdict. Order is significant for
// plenty of attributes: a container's command, an ordered listener rule,
// a WAF rule list, a route table. Deciding this one does not matter is
// the reader's call to make, and the annotation says so rather than
// making it for them. Nothing here touches a finding's level.
//
// No value ever leaves this file. Values are compared internally, which
// is the only way to answer the question at all, and only the path is
// returned.
func reorderedPaths(before, after, afterUnknown interface{}) []string {
	// A resource's before and after are always objects in plan JSON.
	// Anything else - a create with no before, a delete with no after - is
	// not a pair to compare.
	bm, bok := before.(map[string]interface{})
	am, aok := after.(map[string]interface{})
	if !bok || !aok {
		return nil
	}

	var out []string
	walkReordered(bm, am, afterUnknown, "", &out)
	sort.Strings(out)
	return out
}

// What the same-elements-reordered annotation says, in the three shapes a
// renderer can use it in. They live here, beside the rule, so the wording
// and the rule cannot drift apart.
//
// reorderNote carries no dash of any kind, deliberately. It is the one
// sentence in the report long enough to wrap at every width the terminal
// supports, and a hyphen stranded at the start of a wrapped line reads as
// a bullet rather than as punctuation.
//
// reorderDetail is built from reorderNote rather than written out again,
// which is what makes "Detail stays complete on its own" a property of
// the code instead of a thing to remember. Cutting the caveat out of the
// terminal must never cut it out of the JSON.
const (
	reorderSummary = "same elements, different order"
	reorderNote    = "Order is significant for some attributes, such as a container command " +
		"or an ordered rule list, so whether a reordering matters is yours to judge."
	reorderDetail = "these lists hold the same elements in a different order. " + reorderNote
)

// listVerdict is what comparing two lists at the same path found.
type listVerdict int

const (
	// listDiffers means the elements themselves are not the same, so the
	// answer may still be inside them.
	listDiffers listVerdict = iota
	// listIdentical means element for element the same. Nothing nested
	// inside can differ either, so there is no reason to look.
	listIdentical
	// listReordered means the same elements in a different order.
	listReordered
)

// walkReordered walks before and after in step, collecting the path of
// every list that holds the same elements in a different order.
//
// afterUnknown is walked alongside them, one node at a time, so the
// unknown mark is honoured at the list it actually covers rather than
// silencing the whole subtree beneath it.
func walkReordered(before, after, afterUnknown interface{}, prefix string, out *[]string) {
	switch b := before.(type) {
	case map[string]interface{}:
		a, ok := after.(map[string]interface{})
		if !ok {
			// The attribute changed shape. That is a change of a different
			// kind, and not one this rule has anything to say about.
			return
		}
		for k, bv := range b {
			av, ok := a[k]
			if !ok {
				// Present on one side only. Terraform's omitUnknowns drops
				// computed keys from the after entirely, so this is common
				// and is not evidence of anything.
				continue
			}
			next := k
			if prefix != "" {
				next = prefix + "." + k
			}
			walkReordered(bv, av, mapChild(afterUnknown, k), next, out)
		}

	case []interface{}:
		a, ok := after.([]interface{})
		if !ok {
			return
		}
		// Different lengths cannot be the same multiset, and index i on one
		// side no longer answers to index i on the other, so there is
		// nothing safe to say at this level or below it.
		if len(b) != len(a) {
			return
		}

		switch compareList(b, a, afterUnknown) {
		case listIdentical:
			return
		case listReordered:
			// Report the list, and stop. Below this point the indices no
			// longer line up, so anything found there would be comparing
			// two elements that are not the same element.
			*out = append(*out, prefix)
			return
		}

		// Same length, different contents. Terraform renders a list diff
		// by index, so pairing by index is what the reader is looking at.
		for i := range b {
			walkReordered(b[i], a[i], listChild(afterUnknown, i), fmt.Sprintf("%s[%d]", prefix, i), out)
		}
	}
}

// compareList compares two lists of the same length.
//
// It returns listDiffers whenever it cannot answer - an unknown mark
// anywhere in the list, an element that will not encode - because a
// comparison that cannot be made must fall through to looking inside, not
// produce a claim.
func compareList(before, after []interface{}, afterUnknown interface{}) listVerdict {
	// Something in here is not known until apply, so the elements cannot
	// be compared as they stand. Terraform's placeholder is not a value,
	// and any order it appears to have is an artefact of the placeholder.
	if anyUnknown(afterUnknown) {
		return listDiffers
	}

	be, ok := canonical(before)
	if !ok {
		return listDiffers
	}
	ae, ok := canonical(after)
	if !ok {
		return listDiffers
	}

	for i := range be {
		if be[i] != ae[i] {
			if sameMultiset(be, ae) {
				return listReordered
			}
			return listDiffers
		}
	}
	return listIdentical
}

// canonical encodes each element as JSON, which is what makes the
// comparison exact and type-aware.
//
// fmt's %v would not be. It renders the number 15 and the string "15"
// identically, true and "true" identically, nil and "<nil>" identically -
// the same trap renderAttrs already avoids in moved.go. Two elements that
// differ only by type are different elements, and a rule that missed that
// would report a genuine type change as a harmless reshuffle.
//
// The false return is the guard on an element that will not encode. A
// value that came out of json.Unmarshal can always be marshalled again,
// so it is unreachable through the loader, but silence is the right
// answer to a comparison that cannot be made.
func canonical(elems []interface{}) ([]string, bool) {
	out := make([]string, len(elems))
	for i, e := range elems {
		b, err := json.Marshal(e)
		if err != nil {
			return nil, false
		}
		out[i] = string(b)
	}
	return out, true
}

// sameMultiset reports whether two equal-length encodings hold the same
// elements, counting duplicates.
//
// Counting is the whole point: ["a","a","b"] and ["a","b","b"] hold the
// same distinct values and are not the same list. Collecting them into a
// set would call that pair a reordering, which is wrong and would be
// wrong quietly.
func sameMultiset(before, after []string) bool {
	counts := make(map[string]int, len(before))
	for _, s := range before {
		counts[s]++
	}
	for _, s := range after {
		counts[s]--
		if counts[s] < 0 {
			return false
		}
	}
	// Equal lengths and nothing overdrawn means every count landed on zero.
	return true
}

// anyUnknown reports whether anything in an after_unknown subtree is
// marked unknown until apply. It reuses the walk the unknown and
// sensitive annotations already share, so the two rules cannot drift on
// what "unknown" means.
func anyUnknown(v interface{}) bool {
	var found []string
	walkTrue(v, "", &found)
	return len(found) > 0
}

// mapChild returns the after_unknown node for one key of an object, or nil
// when there is nothing there. after_unknown mirrors the shape of the
// resource, but only as far as it needs to.
func mapChild(afterUnknown interface{}, key string) interface{} {
	m, ok := afterUnknown.(map[string]interface{})
	if !ok {
		return nil
	}
	return m[key]
}

// listChild returns the after_unknown node for one index of a list, or nil
// when there is nothing there.
func listChild(afterUnknown interface{}, i int) interface{} {
	l, ok := afterUnknown.([]interface{})
	if !ok || i >= len(l) {
		return nil
	}
	return l[i]
}
