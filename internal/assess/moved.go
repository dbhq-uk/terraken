package assess

import (
	"encoding/json"
	"fmt"

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
	if len(deletes) == 0 || len(creates) == 0 {
		return out
	}

	// Render every candidate's attributes to a flat map[string]string
	// once, rather than re-rendering per attribute on every candidate
	// pair. This is what keeps an all-one-type plan (e.g. a for_each
	// re-key) affordable: rendering is O(n), and comparison then costs a
	// map lookup and a string equality instead of repeated
	// reflection-based formatting.
	deleteAttrs := make([]map[string]string, len(deletes))
	for i, d := range deletes {
		deleteAttrs[i] = renderAttrs(d.Change.Before)
	}
	createAttrs := make([]map[string]string, len(creates))
	createUnknown := make([]map[string]bool, len(creates))
	for i, c := range creates {
		createAttrs[i] = renderAttrs(c.Change.After)
		createUnknown[i] = unknownTopLevelKeys(c.Change.AfterUnknown)
	}

	// Bucket creates by type+module so a delete only ever compares
	// against creates it could plausibly be a rename of. This is exactly
	// the gate the per-pair check used to apply - same type, same
	// module, unchanged - it just avoids paying for cross-type and
	// cross-module comparisons in a large mixed plan.
	buckets := map[string][]int{}
	for i, c := range creates {
		k := bucketKey(c)
		buckets[k] = append(buckets[k], i)
	}

	// usedCreateIdx tracks claimed creates by their index into the
	// creates slice, not by address: address is not a unique key across
	// a plan (a deposed object shares the live resource's address), so
	// keying bookkeeping on it risks conflating two distinct resource
	// changes.
	usedCreateIdx := map[int]bool{}
	for di, d := range deletes {
		bestCi := -1
		var bestMatched, bestCompared int

		for _, ci := range buckets[bucketKey(d)] {
			if usedCreateIdx[ci] {
				continue
			}
			c := creates[ci]
			if d.Address == c.Address {
				// A deposed object shares the live resource's address.
				// Never let a resource be paired with itself.
				continue
			}

			matched, compared := compareAttrs(deleteAttrs[di], createAttrs[ci], createUnknown[ci])
			if compared < minComparableAttrs {
				continue
			}
			if float64(matched)/float64(compared) < similarityThreshold {
				continue
			}

			if bestCi == -1 || better(matched, compared, bestMatched, bestCompared, c.Address, creates[bestCi].Address) {
				bestCi, bestMatched, bestCompared = ci, matched, compared
			}
		}
		if bestCi == -1 {
			continue
		}
		usedCreateIdx[bestCi] = true
		c := creates[bestCi]

		detail := fmt.Sprintf(
			"%s is being destroyed and %s created, with %d of %d compared attributes identical. "+
				"If this is a rename, a moved block would keep the resource instead of destroying it.",
			d.Address, c.Address, bestMatched, bestCompared)

		ann := Annotation{
			Code:   AnnMissedMoved,
			Detail: detail,
			// This is a suggestion to verify, never copy-pasteable proof:
			// pasting a wrong pairing adopts a decommissioned resource's
			// state under a new address, which is worse than the
			// problem it would fix.
			Paths: []string{fmt.Sprintf(
				"if this is a rename, the moved block would be: moved { from = %s  to = %s } - verify before using",
				d.Address, c.Address)},
		}
		out[d.Address] = ann
		out[c.Address] = ann
	}
	return out
}

// bucketKey groups a resource change by the two things that must match
// before it is even considered a rename candidate: type and module.
func bucketKey(rc *tfjson.ResourceChange) string {
	return rc.Type + "\x00" + rc.ModuleAddress
}

// better reports whether candidate (matched, compared) beats the current
// best match. A higher match ratio wins outright; an exact tie is broken
// on the lexicographically smaller address so the result stays
// deterministic regardless of slice order, instead of first-fit taking
// whichever candidate happened to come first.
func better(matched, compared, bestMatched, bestCompared int, addr, bestAddr string) bool {
	ratio := float64(matched) / float64(compared)
	bestRatio := float64(bestMatched) / float64(bestCompared)
	if ratio != bestRatio {
		return ratio > bestRatio
	}
	return addr < bestAddr
}

// renderAttrs flattens a resource's top-level attribute map into a
// map[string]string, rendering each value once via JSON encoding rather
// than fmt's %v. %v is type-blind - it renders nil and the string
// "<nil>" identically, the number 15 and the string "15" identically,
// true and the string "true" identically, and two lists with the same
// flattened text but different element boundaries identically. JSON
// encoding keeps all of those distinct.
func renderAttrs(v interface{}) map[string]string {
	m, ok := v.(map[string]interface{})
	if !ok {
		return nil
	}
	out := make(map[string]string, len(m))
	for k, val := range m {
		b, err := json.Marshal(val)
		if err != nil {
			// Unreachable in practice - this value came from
			// json.Unmarshal, so it can always be marshalled again -
			// but render something that can never accidentally match
			// another attribute rather than skip it.
			out[k] = fmt.Sprintf("\x00unrenderable\x00%v", err)
			continue
		}
		out[k] = string(b)
	}
	return out
}

// unknownTopLevelKeys returns the set of top-level attribute names that
// Terraform marked wholly unknown in after_unknown.
//
// Terraform's omitUnknowns behaviour drops these keys from Change.After
// entirely rather than nulling them - the delete's Before still carries
// them with their real, once-computed values. Without this, every
// computed attribute on a genuine rename (id, and similar) would count
// as compared but never matched, permanently deflating the score.
func unknownTopLevelKeys(afterUnknown interface{}) map[string]bool {
	out := map[string]bool{}
	m, ok := afterUnknown.(map[string]interface{})
	if !ok {
		return out
	}
	for k, v := range m {
		if b, ok := v.(bool); ok && b {
			out[k] = true
		}
	}
	return out
}

// compareAttrs reports how many rendered top-level attributes matched
// between a delete's Before and a create's After, and how many were
// compared at all.
//
// A key is excluded from the comparison entirely - neither matched nor
// compared - in two cases: the create marks it wholly unknown (there is
// genuinely nothing to compare it to yet, not a difference), or it is
// null on both sides (Terraform state is full of null optional
// attributes, and counting agreement on absence as a match inflates the
// score toward pairing genuinely unrelated resources).
func compareAttrs(before, after map[string]string, unknownAfter map[string]bool) (matched, compared int) {
	keys := map[string]bool{}
	for k := range before {
		keys[k] = true
	}
	for k := range after {
		keys[k] = true
	}

	for k := range keys {
		if unknownAfter[k] {
			continue
		}
		bv, inB := before[k]
		av, inA := after[k]
		if inB && inA && bv == "null" && av == "null" {
			continue
		}
		compared++
		if inB && inA && bv == av {
			matched++
		}
	}
	return matched, compared
}
