package render

import (
	"encoding/json"
	"io"
	"sort"

	"github.com/dbhq-uk/terraken/internal/assess"
)

// The gate: a machine-first verdict for something deciding whether to proceed.
//
// Agents write Terraform now and nothing independently checks what they
// produced. The tools aimed at agents either teach one to write Terraform or
// hand one the power to apply it; a check an agent cannot argue with is the
// missing piece. This is the fence, not the animal - there is no model here, no
// MCP server, and nothing that talks to one.
//
// WHY THIS IS NOT --format json. That format is the human report serialised, so
// it changes shape whenever the report gains a field or an annotation - which
// is often, and is fine, because a person reading it absorbs the change. A
// caller parsing a verdict cannot. This is a separate, smaller surface with its
// own version, and the compatibility policy below is part of the contract
// rather than a note about it.
//
// THE THRESHOLD COMES FROM THE INVOCATION AND NOTHING ELSE. It is passed in
// from the --fail-on flag, and nothing read out of the plan can reach it. That
// is the property the whole feature rests on: a plan being examined must not be
// able to negotiate the bar it is being held to.

// GateSchema is the version of this output's shape.
//
// THE COMPATIBILITY POLICY, which is a promise rather than a description:
//
//   - Within v1, fields are only ever ADDED. A parser that reads the fields it
//     knows and ignores the rest will keep working.
//   - `verdict` will only ever be "pass" or "fail". New states get a new field,
//     never a third value in this one, because a caller comparing against
//     "fail" must not start silently passing.
//   - Removing or repurposing a field is a v2, and v1 keeps being emitted for
//     at least one minor release after v2 appears.
//   - The human report's shape is NOT part of this contract and may change
//     freely. That separation is the entire point.
const GateSchema = "terraken.gate/v1"

// GateVerdict is the whole machine-readable output.
type GateVerdict struct {
	Schema string `json:"schema"`

	// Verdict is "pass" or "fail". Never a third value - see the policy above.
	Verdict string `json:"verdict"`

	// Threshold is the level that fails the gate, echoed back so a caller can
	// see the bar it was actually held to rather than the one it believes it
	// set. Empty means no gate was requested, in which case the verdict is
	// always "pass" and Blocking is empty.
	Threshold string `json:"threshold,omitempty"`

	// Counts is every level in the WHOLE plan, not only the blocking ones.
	Counts map[string]int `json:"counts"`

	// Blocking is every finding at or above the threshold, which is what a
	// caller has to act on. Ordered most severe first, then by address.
	Blocking []GateFinding `json:"blocking"`
}

// GateFinding is one blocking finding, cut to what a decision needs.
type GateFinding struct {
	Address string `json:"address"`
	Type    string `json:"type"`
	Level   string `json:"level"`
	Kind    string `json:"kind"`

	// DataLoss is carried on its own rather than left implicit in the level,
	// because it is the single fact most worth branching on.
	DataLoss bool `json:"data_loss"`

	// Reasons are the tool's own sentences about why this finding is what it
	// is. They name what failed and where. THEY NEVER PROPOSE A CHANGE: the
	// issue asks for guidance that is actionable without being suggestive,
	// and a gate that tells an agent how to get past it is a gate that has
	// been talked past.
	Reasons []string `json:"reasons,omitempty"`

	// Paths are the attribute paths at fault - paths only, never values.
	//
	// ATTRIBUTE PATHS ONLY. The blast-radius annotation carries RESOURCE
	// ADDRESSES in its Paths, which is right for the human renderer and wrong
	// here: a caller parsing this as attribute paths would be handed
	// "terraform_data.app" and have no way to tell it apart from "tags.Name".
	// Those go in Depends instead.
	Paths []string `json:"paths,omitempty"`

	// Depends is what this change reaches - the resources that depend on it,
	// from the blast radius. Its own field because it answers a different
	// question from Paths, and because "this destroys something four other
	// resources depend on" is exactly the fact a caller should branch on.
	Depends []string `json:"depends,omitempty"`
}

// Gate writes the machine verdict.
//
// threshold is the level at or above which a finding blocks, taken from the
// invocation. An empty threshold means no gate was asked for: the verdict is
// "pass" and nothing blocks, which lets a caller run this unconditionally and
// decide later whether it cared.
func Gate(w io.Writer, r assess.Report, threshold string) error {
	v := GateVerdict{
		Schema:  GateSchema,
		Verdict: "pass",
		Counts:  map[string]int{},
		// Initialised, never nil. A nil slice marshals as null and a caller
		// iterating it fails on the one plan whose answer is good news.
		Blocking: []GateFinding{},
	}
	for name, n := range r.CountsByName {
		v.Counts[name] = n
	}

	min, perr := assess.ParseLevel(threshold)
	if threshold == "" || perr != nil {
		return encodeGate(w, v)
	}
	v.Threshold = min.String()

	for _, f := range r.Findings {
		if f.Level < min {
			continue
		}
		g := GateFinding{
			Address:  f.Address,
			Type:     f.Type,
			Level:    f.Level.String(),
			Kind:     string(f.Kind),
			DataLoss: f.DataLoss,
		}
		if f.Reason != "" {
			g.Reasons = append(g.Reasons, f.Reason)
		}
		seen := map[string]bool{}
		for _, a := range f.Annotations {
			// Blast radius contributes its reach to Depends and nothing to
			// Paths - see the field comments above.
			if a.Code == assess.AnnBlastRadius {
				if a.Summary != "" && !seen[a.Summary] {
					seen[a.Summary] = true
					g.Reasons = append(g.Reasons, a.Summary)
				}
				for _, rch := range a.Reached {
					g.Depends = append(g.Depends, rch.Address)
				}
				continue
			}
			// Summary where there is one, Detail otherwise - Summary is the
			// fact without the standing caveat welded on, which is what a
			// caller wants and what a terminal footer exists to separate.
			line := a.Summary
			if line == "" {
				line = a.Detail
			}
			if line != "" && !seen[line] {
				seen[line] = true
				g.Reasons = append(g.Reasons, line)
			}
			for _, p := range a.Paths {
				g.Paths = append(g.Paths, p)
			}
		}
		g.Paths = append(g.Paths, f.ReplacePaths...)
		g.Paths = dedupe(g.Paths)
		g.Depends = dedupe(g.Depends)
		v.Blocking = append(v.Blocking, g)
	}

	if len(v.Blocking) > 0 {
		v.Verdict = "fail"
	}
	return encodeGate(w, v)
}

func encodeGate(w io.Writer, v GateVerdict) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

func dedupe(in []string) []string {
	if len(in) == 0 {
		return nil
	}
	sort.Strings(in)
	out := in[:1]
	for _, s := range in[1:] {
		if s != out[len(out)-1] {
			out = append(out, s)
		}
	}
	return out
}
