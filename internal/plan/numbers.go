package plan

import (
	"bytes"
	"encoding/json"

	tfjson "github.com/hashicorp/terraform-json"
)

// Attribute values keep the digits the file actually holds.
//
// encoding/json decodes every JSON number into a float64, and a float64 holds
// 53 bits of significand - so 9007199254740992 and 9007199254740993 arrive as
// the same value and every comparison downstream calls them equal. Terraform
// shows the attribute changing and terraken said it did not.
//
// THAT IS THE TOOL'S OWN FAILURE MODE, not a rounding nicety. A change reported
// as no change is the one thing the report exists to prevent, and it was silent
// twice over: the changed-attribute list left the attribute out, and the
// rewritten-value rule was one step from calling a real difference "the same
// number written differently".
//
// WHY IT IS DONE HERE AND NOT WITH A DECODER OPTION. json.Decoder.UseNumber
// makes the decoder produce json.Number instead of float64, and it does not
// reach these values: tfjson.Plan has its own UnmarshalJSON, which decodes the
// document itself and does not carry the option inward. Checked against the
// pinned v0.28.0 - the values still come back as float64. So the plan is
// decoded twice: once by the library for its structure, and once here, plainly,
// for the values that are compared.
//
// ONLY before AND after. Those are the two the tool compares. after_unknown and
// the sensitive maps are booleans, replace_paths holds indices that
// flattenPath renders from a float64 and never compares, and the configuration
// is read for references rather than values.

// preserveNumbers re-reads the attribute values of every resource change and
// drift entry with their digits intact, and puts them back on the decoded plan.
//
// It is best effort by design. The plan has already been decoded and validated
// by the time this runs, so a document this cannot walk is one the library was
// happy with - and the right answer then is to leave the float64 values alone
// rather than to fail a plan that parses. What is lost in that case is exactly
// what was lost before this existed.
func preserveNumbers(p *tfjson.Plan, b []byte) {
	if p == nil {
		return
	}

	var raw struct {
		ResourceChanges []rawChange `json:"resource_changes"`
		ResourceDrift   []rawChange `json:"resource_drift"`
	}
	d := json.NewDecoder(bytes.NewReader(b))
	d.UseNumber()
	if err := d.Decode(&raw); err != nil {
		return
	}

	apply(p.ResourceChanges, raw.ResourceChanges)
	apply(p.ResourceDrift, raw.ResourceDrift)
}

// rawChange is one entry's values, decoded with the digits kept.
//
// Change IS A POINTER, matching the library. A JSON object may repeat a key,
// and encoding/json takes the last one - so `"change": null` after a
// `"change": {...}` CLEARS the field for a pointer and merely leaves the
// earlier members in place for a value struct. With a value struct here the
// second decode could hand back a `before` the library had correctly
// discarded, and apply() would write it over the top.
type rawChange struct {
	Address string `json:"address"`
	Change  *struct {
		Before interface{} `json:"before"`
		After  interface{} `json:"after"`
	} `json:"change"`
}

// repairNumbers walks the library's decoded value and the re-read one together,
// and replaces a float64 leaf with the digits it was read from.
//
// IT REPAIRS RATHER THAN REPLACES, and that is the whole shape of it. An
// earlier version assigned the re-read value over the library's, which meant
// the two decodes had to agree about the document - and on a hand-crafted plan
// that repeats a key, or repeats `resource_changes` with a shorter array in
// between, they do not. encoding/json MERGES a repeated key into what it has
// already decoded and reuses the backing array of a slice, so the second decode
// could hand back a `before` the library had correctly discarded, and assigning
// it put the discarded value back into the report.
//
// Walking both means a leaf is only ever rewritten where the library already
// has one. A value the library dropped stays dropped, because there is nothing
// here to walk into; a shape the two disagree about is left exactly as the
// library decoded it. The worst case is the float64 that was there before.
func repairNumbers(lib, raw interface{}) interface{} {
	switch l := lib.(type) {
	case map[string]interface{}:
		r, ok := raw.(map[string]interface{})
		if !ok {
			return lib
		}
		for k, v := range l {
			if rv, ok := r[k]; ok {
				l[k] = repairNumbers(v, rv)
			}
		}
		return l
	case []interface{}:
		r, ok := raw.([]interface{})
		if !ok || len(r) != len(l) {
			return lib
		}
		for i := range l {
			l[i] = repairNumbers(l[i], r[i])
		}
		return l
	case float64:
		// The only substitution. A json.Number holds the digits the file
		// actually had, which a float64 cannot always represent.
		if n, ok := raw.(json.Number); ok {
			return n
		}
		return lib
	default:
		return lib
	}
}

// apply puts the re-read values back, BY POSITION AND CHECKED BY ADDRESS.
//
// Both slices come from the same document in the same order, so position is
// the correspondence. The address is compared as well because two entries can
// share one - a deposed object and the resource it belongs to - and a mismatch
// there would mean the two decodes had disagreed about the shape of the
// document, which is a reason to leave everything alone rather than to guess.
func apply(changes []*tfjson.ResourceChange, raw []rawChange) {
	if len(changes) != len(raw) {
		return
	}
	for i, rc := range changes {
		if rc == nil || rc.Change == nil || raw[i].Change == nil {
			continue
		}
		if rc.Address != raw[i].Address {
			continue
		}
		rc.Change.Before = repairNumbers(rc.Change.Before, raw[i].Change.Before)
		rc.Change.After = repairNumbers(rc.Change.After, raw[i].Change.After)
	}
}
