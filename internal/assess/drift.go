package assess

import (
	"sort"

	tfjson "github.com/hashicorp/terraform-json"
)

// What changed underneath the estate, as opposed to what this change plans to
// do.
//
// Terraform computed this during refresh and WROTE IT INTO THE FILE.
// `resource_drift` is "a description of the changes Terraform detected when it
// compared the most recent state to the prior saved state", in the same shape
// as `resource_changes`. Reading it needs no credentials, no network and no
// cloud API - every other tool that reports drift needs all three, and this
// one needs none of them because the answer is already in the artefact.
//
// THE TWO LISTS NEVER MIX, and that is the whole shape of this file. One is
// what somebody did and the other is what will happen; a "destroy" in the
// findings means Terraform will destroy it, and a "destroy" here means it is
// already gone. Putting them in one list would make the report unreadable in
// the worst way - by looking readable.
//
// IT IS NOT IN THE SEVERITY COUNTS AND --fail-on CANNOT SEE IT, for the reason
// design.md gives: the counts describe resource changes this plan makes, and
// drift is not one. It still carries a level, because drift on a resource that
// holds data is a different event from drift on a tag and a reader needs to
// see which - it is ranked without being counted, the same way the report
// ranks without ruling.
//
// SAYING WHEN DRIFT CANNOT BE KNOWN is handled in coverage.go rather than
// here, because it is one of the silences that report exists to name. An empty
// or absent array means either nothing drifted or refresh never ran, and
// nothing in the file distinguishes them.

// Annotation codes for drift.
const (
	// AnnDrift is on every drift entry. It carries the sentence that says
	// this already happened, so an entry lifted out of the list on its own
	// cannot be mistaken for a planned change.
	AnnDrift = "changed-outside-terraform"

	// AnnDriftRelevant marks drift on a resource this plan reads, from
	// relevant_attributes. It says the plan MAY have been affected, not that
	// it was - see the comment where it is attached. Absent means no claim
	// either way.
	AnnDriftRelevant = "drift-this-plan-reads"

	// AnnDriftMoved is a drift entry that is not external change at all: the
	// object moved to a different address in state. Terraform emits those in
	// the same array.
	AnnDriftMoved = "moved-in-state"
)

// driftOf ranks what Terraform found changed underneath.
//
// It reuses assessOne, so drift is ranked by exactly the rules a planned
// change is: the data-loss escalation, the unrecognised-provider caveat, the
// unknown-value annotation. A second ranking path would drift from the first
// the day somebody changed one of them.
func driftOf(p *tfjson.Plan) []Finding {
	if p == nil || len(p.ResourceDrift) == 0 {
		return nil
	}

	relevant := relevantResources(p.RelevantAttributes)

	out := make([]Finding, 0, len(p.ResourceDrift))
	for _, rc := range p.ResourceDrift {
		if rc == nil || rc.Change == nil {
			continue
		}
		f := assessOne(rc)
		f.LevelName = f.Level.String()

		// WHAT ASSESSONE READS THAT DOES NOT APPLY HERE. It was written for a
		// planned change, and two of the things it carries are the planning
		// DECISION rather than the change:
		//
		//   - action_reason says why Terraform decided to do something -
		//     "replacement was explicitly requested", "its configuration
		//     block was removed". Nothing decided this. Somebody did it.
		//   - replace_paths names the attribute that forced a replacement,
		//     which is a planning concept; a drift entry printing "forces
		//     replacement" would be describing a decision that was never made.
		//   - replace_order says which way round an apply will carry out a
		//     replacement. Nothing here is going to be applied, so "there is a
		//     point during the apply at which this resource does not exist"
		//     would describe an apply that is not going to happen.
		//
		// Dropped here rather than guarded inside assessOne, so the ranking
		// stays one code path and the exception stays where the reason for it
		// is written down.
		f.Reason = ""
		f.ReplacePaths = nil
		f.ReplaceOrder = ""
		f.Annotations = withoutCode(f.Annotations, AnnReplaceOrder)

		// The unrecognised-provider caveat is right but worded for a plan.
		// "whether destroying this loses data" reads as a proposal; here the
		// resource is already gone.
		for i, a := range f.Annotations {
			if a.Code == AnnUnknownVendor {
				f.Annotations[i].Detail = "this provider is not on terraken's curated list, " +
					"so whether losing this has lost any data has not been assessed"
			}
		}

		// NOT EVERY ENTRY IN THIS ARRAY IS EXTERNAL CHANGE. Terraform puts a
		// no-op entry here when an object MOVED to a different address in
		// state without its values changing - a `moved` block, a renamed
		// module. Calling that "changed outside Terraform" tells a reviewer
		// somebody has been editing infrastructure by hand when nobody has,
		// and a past-tense verb does not fix the meaning of the event.
		//
		// FIRST, so it is the first thing read. Every other annotation on a
		// drift entry describes the resource; this one describes what kind of
		// event the reader is looking at.
		f.Annotations = append([]Annotation{driftAnnotation(rc, f.Kind)}, f.Annotations...)

		// Only when the plan said something about relevance at all. An absent
		// relevant_attributes says nothing about what contributed, and
		// inferring from its absence would be the same error as reading an
		// absent drift array as reassurance.
		if len(relevant) > 0 && relevant[rc.Address] {
			// "MAY HAVE", NOT "DID". relevant_attributes names the resource,
			// and this drops its attribute paths - so the plan reading
			// `resource.output` and the drift touching `resource.input` still
			// matches here. Terraform's own specification says the field
			// identifies external changes that MAY have affected the result,
			// and claiming more than that would be inventing a causal link
			// the pairing does not support.
			f.Annotations = append(f.Annotations, Annotation{
				Code: AnnDriftRelevant,
				Detail: "this plan reads values from this resource, so what changed " +
					"underneath it may have affected what the plan decided to do. " +
					"Which attributes were involved is not stated precisely enough here " +
					"to say that it did",
				Summary: "this plan reads this resource",
			})
		}
		out = append(out, f)
	}

	// Most severe first, then address, like the findings. The address tiebreak
	// is what keeps the output byte-identical between runs.
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Level != out[j].Level {
			return out[i].Level > out[j].Level
		}
		return out[i].Address < out[j].Address
	})
	return out
}

// driftAnnotation says what kind of event this entry is, and it is not always
// the same kind.
//
// THE WORDING IS "AGAINST THE PRIOR SAVED STATE", NOT "SINCE THE LAST APPLY".
// The plan records a comparison, not a timeline: it does not carry an apply
// timestamp, and it does not say who caused the difference. Saying "since the
// last apply" would put both in the reader's head from a file that states
// neither.
func driftAnnotation(rc *tfjson.ResourceChange, k Kind) Annotation {
	// AN ADDRESS MOVE IS NOT EXTERNAL CHANGE. Terraform puts a no-op entry in
	// this array when an object moved to a different address in state and its
	// values did not change - which is a `moved` block or a renamed module
	// doing exactly what it was asked to. Reporting that as somebody editing
	// infrastructure by hand is a false alarm about the one thing this
	// section exists to raise real alarms about.
	if rc.PreviousAddress != "" && rc.PreviousAddress != rc.Address {
		return Annotation{
			Code: AnnDriftMoved,
			Detail: "this moved to a different address in state, from " + rc.PreviousAddress +
				". Its values did not change, so this is Terraform's own bookkeeping " +
				"rather than anything having been altered outside it",
			Summary: "moved in state, not changed",
		}
	}
	if k == KindNoOp {
		return Annotation{
			Code: AnnDriftMoved,
			Detail: "Terraform recorded this while refreshing but found no difference in " +
				"its values, so nothing about it was altered outside Terraform",
			Summary: "recorded, but unchanged",
		}
	}

	detail := "this differs from the state Terraform last recorded, and the difference is " +
		"already there - it is not something this plan proposes to do"
	summary := "changed outside Terraform"

	switch k {
	case KindDelete:
		detail = "this no longer exists. It is gone from the infrastructure but still in " +
			"the state Terraform last recorded, and this plan does not propose to " +
			"destroy it - it is already destroyed"
		summary = "destroyed outside Terraform"
	case KindCreate:
		detail = "this exists in the infrastructure but not in the state Terraform last " +
			"recorded, and this plan does not propose to create it - it is already there"
		summary = "created outside Terraform"
	}
	return Annotation{Code: AnnDrift, Detail: detail, Summary: summary}
}

// withoutCode drops every annotation carrying one code, and returns a new
// slice rather than filtering in place. assessOne's result is the drift
// entry's own, but the habit matters: a filter that reused the backing array
// would be one aliasing bug away from editing a planned finding.
func withoutCode(in []Annotation, code string) []Annotation {
	out := make([]Annotation, 0, len(in))
	for _, a := range in {
		if a.Code != code {
			out = append(out, a)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// relevantResources is the set of resource addresses named in
// relevant_attributes, which "lists the sources of all values contributing to
// changes in the plan".
//
// The ATTRIBUTE paths are deliberately dropped. Drift is reported per
// resource, and an attribute-level match would claim more precision than the
// pairing supports: a resource can appear in relevant_attributes for one
// attribute and have drifted in another.
func relevantResources(attrs []tfjson.ResourceAttribute) map[string]bool {
	if len(attrs) == 0 {
		return nil
	}
	out := make(map[string]bool, len(attrs))
	for _, a := range attrs {
		out[a.Resource] = true
	}
	return out
}
