package assess

import (
	"fmt"
	"sort"

	tfjson "github.com/hashicorp/terraform-json"
)

// similarityThreshold is the share of compared attributes that must match
// for a delete and a create to be treated as a probable rename.
const similarityThreshold = 0.8

// minComparableAttrs guards against pairing two nearly-empty resources,
// where a high match ratio means nothing.
const minComparableAttrs = 3

// detectMissedMoves finds deletes and creates that look like the same
// resource renamed without a moved block, and returns an annotation keyed
// by resource address.
//
// Terraform sets PreviousAddress when a moved block was used, so those
// are already handled correctly and are skipped entirely.
func detectMissedMoves(changes []*tfjson.ResourceChange) map[string]Annotation {
	out := map[string]Annotation{}

	var deletes, creates []*tfjson.ResourceChange
	for _, rc := range changes {
		if rc == nil || rc.Change == nil || rc.PreviousAddress != "" {
			continue
		}
		if rc.Mode != tfjson.ManagedResourceMode {
			continue
		}
		switch {
		case rc.Change.Actions.Delete():
			deletes = append(deletes, rc)
		case rc.Change.Actions.Create():
			creates = append(creates, rc)
		}
	}

	used := map[string]bool{}
	for _, d := range deletes {
		for _, c := range creates {
			if used[c.Address] {
				continue
			}
			if d.Type != c.Type || d.ModuleAddress != c.ModuleAddress {
				continue
			}

			matched, compared := similarity(d.Change.Before, c.Change.After)
			if compared < minComparableAttrs {
				continue
			}
			if float64(matched)/float64(compared) < similarityThreshold {
				continue
			}

			used[c.Address] = true
			detail := fmt.Sprintf(
				"%s is being destroyed and %s created, with %d of %d compared attributes identical. "+
					"If this is a rename, a moved block would keep the resource instead of destroying it.",
				d.Address, c.Address, matched, compared)

			ann := Annotation{
				Code:   AnnMissedMoved,
				Detail: detail,
				Paths:  []string{fmt.Sprintf("moved { from = %s  to = %s }", d.Address, c.Address)},
			}
			out[d.Address] = ann
			out[c.Address] = ann
			break
		}
	}
	return out
}

// similarity compares two attribute maps and reports how many top-level
// values matched, and how many were compared at all.
func similarity(before, after interface{}) (matched, compared int) {
	b, okB := before.(map[string]interface{})
	a, okA := after.(map[string]interface{})
	if !okB || !okA {
		return 0, 0
	}

	keys := map[string]bool{}
	for k := range b {
		keys[k] = true
	}
	for k := range a {
		keys[k] = true
	}

	sorted := make([]string, 0, len(keys))
	for k := range keys {
		sorted = append(sorted, k)
	}
	sort.Strings(sorted)

	for _, k := range sorted {
		bv, inB := b[k]
		av, inA := a[k]
		if !inB || !inA {
			compared++
			continue
		}
		compared++
		if fmt.Sprintf("%v", bv) == fmt.Sprintf("%v", av) {
			matched++
		}
	}
	return matched, compared
}
