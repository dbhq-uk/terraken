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

	// Exposure is what the plan FILE carries: values that look like
	// credentials and that Terraform did not mark sensitive. Added in v1,
	// which the compatibility policy above allows.
	//
	// IT DOES NOT MOVE THE VERDICT, and that is a decision rather than an
	// oversight. `verdict` answers one question - is anything at or above the
	// threshold the invocation set - and the threshold is a severity level,
	// which an exposure is not. Folding a heuristic into it would mean a
	// pattern match could fail a build that nothing in the plan justified
	// failing, and the detection is explicitly admitted to be rough.
	//
	// A caller that wants to stop on this branches on the array being
	// non-empty, which is one line and is the caller's policy rather than
	// this tool's. Never empty-vs-absent as a signal: the field is omitted
	// when there is nothing, so `(.exposure // []) | length > 0` is the test.
	Exposure []GateExposed `json:"exposure,omitempty"`

	// Unsupported is every operation this build could not recognise, and so
	// could not assess at all. Added in v1, which the compatibility policy
	// above allows.
	//
	// THIS IS THE FIELD A MACHINE CONSUMER MOST NEEDS, because it is the one
	// that says the verdict above is incomplete. `verdict` answers whether
	// anything reached the severity threshold; these findings have no
	// severity, so they cannot reach it, and a caller reading `pass` without
	// reading this has been told the plan is clear when part of it was never
	// read. Like the exposure it is carried whether or not a gate was asked
	// for, and like the exposure it does not move the verdict - a severity
	// threshold is a question about severities.
	//
	// Two ways to act on it, both the caller's own policy rather than this
	// tool's: branch on `(.unsupported // []) | length > 0`, or write a rule
	// matching actions: ["unsupported"], which gives the finding a real
	// severity and brings it into `blocking` like anything else.
	Unsupported []GateUnsupported `json:"unsupported,omitempty"`
}

// GateUnsupported is one operation this build does not recognise.
//
// It carries no attribute value and no part of one. Actions is Terraform's
// own vocabulary, taken from the plan verbatim and quoted - see
// internal/assess, which quotes it at the point it enters the report so
// that no format can be made to print a raw control character.
type GateUnsupported struct {
	Address string `json:"address"`
	Type    string `json:"type,omitempty"`

	// Actions is the whole ordered sequence the plan named, including verbs
	// that would be recognised on their own. [delete, quarantine] is not a
	// delete with a footnote; it is one operation this build cannot read,
	// and handing over half of it would suggest otherwise.
	Actions []string `json:"actions"`

	// Detail is the tool's own sentence, repeated on every entry rather than
	// stated once at the top, so a caller that lifts one entry into a log
	// line cannot lift the address without the reason.
	Detail string `json:"detail"`
}

// GateExposed is one value that looks like a credential, as paths and a class.
// It carries no value and no part of one - see internal/assess/credentials.go.
type GateExposed struct {
	Address   string `json:"address"`
	Path      string `json:"path"`
	LooksLike string `json:"looks_like"`

	// Confidence is the standing caveat, repeated on every entry rather than
	// stated once at the top. A caller that lifts one entry into a log line or
	// a comment must not be able to lift the claim without the caveat.
	Confidence string `json:"confidence"`
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

	// Before the threshold check, and outside it. An exposure is a fact about
	// the file rather than a finding at a level, so it is reported whether or
	// not a gate was asked for - a caller running `--format gate` with no
	// --fail-on, purely to see what is in the plan, still gets told.
	for _, e := range r.Exposure.Values {
		v.Exposure = append(v.Exposure, GateExposed{
			Address:    e.Address,
			Path:       e.Path,
			LooksLike:  e.Looks,
			Confidence: assess.ExposureConfidence,
		})
	}

	// Also before the threshold check, and for the same reason as the
	// exposure above: this is what says the verdict is incomplete, so a
	// caller running --format gate with no --fail-on still gets it.
	//
	// KEYED ON THE KIND, NOT ON THE LEVEL, and that distinction is the whole
	// point of the array. A team rule may give one of these a severity - and
	// a rule can be broad, "everything of this type is info" being a
	// perfectly reasonable thing to write - which takes the finding off the
	// Unranked level. It does not make the operation understood. A team
	// ranking something is not a team declaring the tool able to read it, so
	// the coverage gap is reported either way.
	for _, f := range r.Findings {
		if f.Kind != assess.KindUnsupported {
			continue
		}
		u := GateUnsupported{Address: f.Address, Type: f.Type}
		for _, a := range f.Annotations {
			if a.Code != assess.AnnUnsupportedAction {
				continue
			}
			u.Actions = append(u.Actions, a.Paths...)
			u.Detail = a.Detail
		}
		// Never nil. A nil slice marshals as null and a caller iterating it
		// fails on an entry it was told to expect.
		if u.Actions == nil {
			u.Actions = []string{}
		}
		v.Unsupported = append(v.Unsupported, u)
	}

	min, perr := assess.ParseLevel(threshold)
	if threshold == "" || perr != nil {
		return encodeGate(w, v)
	}
	v.Threshold = min.String()

	for _, f := range r.Findings {
		// UNRANKED IS NOT A SEVERITY, so it cannot clear a severity
		// threshold. It sorts above critical so a reader meets it first, and
		// that position must not be allowed to leak into a verdict: a team
		// pinned to --fail-on critical would otherwise start failing the day
		// Terraform ships an action verb this build has never seen. It is
		// reported in Unsupported above, at every threshold.
		if f.Level == assess.Unranked || f.Level < min {
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
			// The unrecognised action names are not attribute paths either,
			// and they have their own array. Putting them in Paths would
			// hand a caller "\"quarantine\"" where it expected "tags.Name",
			// and the dedupe below sorts, so they would not even arrive in
			// the order the plan named them. The reason still goes in
			// Reasons; only the vocabulary is held back, because
			// Unsupported above carries it in full.
			if a.Code == assess.AnnUnsupportedAction {
				if a.Summary != "" && !seen[a.Summary] {
					seen[a.Summary] = true
					g.Reasons = append(g.Reasons, a.Summary)
				}
				continue
			}
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
