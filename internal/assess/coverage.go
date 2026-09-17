package assess

import (
	"fmt"

	"github.com/dbhq-uk/terraken/internal/plan"
	tfjson "github.com/hashicorp/terraform-json"
)

// How much of this change can be checked before it is applied, and what the
// part that cannot is.
//
// THIS IS THE CAPABILITY THE TOOL IS SHAPED FOR. Terraken already said, per
// resource, that some values are not known until apply, and separately
// summarised the shape of the whole plan. It never joined the two, so it never
// answered the question a reviewer actually has before approving: how much of
// this can I check, and what is the part I cannot?
//
// Every comparable tool renders the plan in more detail. A section saying what
// the report CANNOT show you is the opposite of what a renderer is for, and
// exactly what an assessor is for. It is constraint 5 - say when something
// cannot be known - applied to the file rather than to a single resource.
//
// COUNTS WITH A NAMED DENOMINATOR, NEVER A PERCENTAGE. "62% reviewable" is a
// verdict wearing a number, and the roll-up states a fact rather than ruling.
// Two of three changes carrying values unknown until apply is a fact a reader
// can act on; 67% is a grade they cannot.

// Gap codes. Stable identifiers, so a machine consumer can branch on one
// without parsing prose, and so a reader can look one up.
const (
	// Attribute values Terraform will not know until the change runs. The
	// oldest of the five and the only one already reported per resource.
	GapUnknownUntilApply = "unknown-until-apply"

	// No drift recorded. Terraform only writes the array when something
	// actually drifted, so an absent field and an empty one say the same
	// thing: either nothing drifted or refresh never ran, and the file does
	// not distinguish them.
	GapDriftNotKnown = "drift-not-known"

	// complete: false. The plan is not expected to converge, so what is here
	// is not the whole change.
	GapIncompletePlan = "plan-not-complete"

	// A check Terraform could not determine before apply.
	GapChecksUndetermined = "checks-undetermined"

	// An operation this build could not read at all, so nothing below it was
	// assessed either. See classify: the kind is what decides, because a team
	// rule giving one a severity does not make it understood.
	GapUnreadableOperation = "operation-not-recognised"

	// An output whose value Terraform will not know until apply. Separate
	// from the resource-change gap because an output is not a resource change
	// and shares none of its denominator.
	GapUnknownOutputs = "outputs-unknown-until-apply"

	// Work Terraform has already decided to postpone.
	GapDeferred = "changes-deferred"
)

// Gap is one reason part of this plan could not be assessed.
type Gap struct {
	// Code is the stable identifier. Detail is the tool's own sentence,
	// complete on its own, because a consumer reading one gap with nothing
	// else in view gets Detail and only Detail.
	Code   string `json:"code"`
	Detail string `json:"detail"`

	// Count and Of are the count and its denominator, and they are only set
	// where there is something to count. Of == 0 means the gap is a fact
	// about the whole plan rather than a proportion of it - refresh either
	// ran or it did not, and "0 of 1" would be a worse way of saying so.
	Count int `json:"count,omitempty"`
	Of    int `json:"of,omitempty"`
}

// Coverage is what the tool could and could not assess, over the whole plan.
type Coverage struct {
	// Changes is every RESOURCE change in the plan, and Assessed is how many
	// of those the tool could read in full. The denominator is named rather
	// than left to be guessed: an output change is not a resource change and
	// is counted separately below, because folding the two together would put
	// a number in the report that matches nothing else in it.
	Changes  int `json:"changes"`
	Assessed int `json:"assessed"`

	// Outputs is how many output changes the plan holds, and UnknownOutputs
	// how many of those Terraform will not know until apply. Outputs are not
	// resource changes and have no finding to hang from, which is why they
	// need their own pair rather than a share of the one above.
	Outputs        int `json:"outputs"`
	UnknownOutputs int `json:"unknown_outputs"`

	// Drift is how many entries Terraform recorded as having changed
	// underneath the estate. It is NOT part of the coverage denominators -
	// drift is not a change this plan makes - and it is here for one reason:
	// the headline must not tell a reader there is nothing in this plan while
	// a section below it lists a database that vanished.
	Drift int `json:"drift"`

	// Gaps is every reason part of the plan could not be assessed, in a fixed
	// order so two runs of the same plan produce the same report.
	Gaps []Gap `json:"gaps"`

	// Headline is the one sentence, said either way. A plan where everything
	// is checkable says so rather than leaving a reader to infer it from an
	// absent section - being told nothing and being told there is nothing are
	// not the same.
	Headline string `json:"headline"`
}

// Complete reports whether the whole plan could be assessed.
func (c Coverage) Complete() bool { return len(c.Gaps) == 0 }

// coverageOf measures the plan against the findings already made from it.
//
// It takes the findings rather than re-reading the changes for the unknowns,
// so the count it reports is the same count the report shows - a summary that
// disagreed with the list under it would be worse than no summary.
func coverageOf(p *tfjson.Plan, st plan.Status, findings []Finding, drift int) Coverage {
	c := Coverage{Changes: len(findings), Gaps: []Gap{}}

	// ORDER IS FIXED, not derived from the plan. Ranging a map is
	// deliberately unordered in Go and a plan's own ordering is not ours to
	// depend on, so the gaps are appended in the order below and nothing
	// sorts them afterwards.
	// TWO WAYS A CHANGE FAILS TO BE ASSESSED, and only one of them was
	// counted. A value unknown until apply is the obvious one. The other is an
	// operation the tool could not read at all - assessOne returns before it
	// reaches the unknown-value check, so an unsupported change carried no
	// unverifiable annotation and was counted as fully assessed. A plan whose
	// single change terraken explicitly could not understand reported
	// "everything could be assessed", which is the exact failure constraint 5
	// exists to prevent, arriving from the feature that exists to report it.
	//
	// The kind is what decides, not the level: a team rule giving an
	// unsupported operation a severity does not make the operation understood.
	var withUnknowns, unreadable int
	for _, f := range findings {
		if f.Kind == KindUnsupported {
			unreadable++
			continue
		}
		for _, a := range f.Annotations {
			if a.Code == AnnUnverifiable {
				withUnknowns++
				break
			}
		}
	}
	c.Assessed = c.Changes - withUnknowns - unreadable
	if withUnknowns > 0 {
		c.Gaps = append(c.Gaps, Gap{
			Code:  GapUnknownUntilApply,
			Count: withUnknowns, Of: c.Changes,
			Detail: fmt.Sprintf("%d of %d changes carry values Terraform will not know "+
				"until it applies them, so no claim about those values can be checked now",
				withUnknowns, c.Changes),
		})
	}

	if unreadable > 0 {
		c.Gaps = append(c.Gaps, Gap{
			Code:  GapUnreadableOperation,
			Count: unreadable, Of: c.Changes,
			Detail: fmt.Sprintf("%d of %d changes ask for an operation this build does not "+
				"recognise, so nothing about what they do has been assessed",
				unreadable, c.Changes),
		})
	}

	// NIL AND EMPTY ARE THE SAME SILENCE, and the first version of this got
	// that wrong. It treated an empty array as proof that refresh had run and
	// found nothing, which would have been a useful distinction if it existed.
	// It does not: Terraform writes the array only when something actually
	// drifted, so a plan from a clean refresh and a plan from -refresh=false
	// are byte-identical here. docs/roadmap.md said so before any of this was
	// written - "-refresh=false and nothing drifted produce the same empty
	// array" - and the code contradicted it.
	//
	// So this fires whenever no drift is recorded. The sentence says the file
	// cannot tell the two apart, which is the honest reading and the same move
	// the tool already makes for a value that is unknown until apply.
	if p != nil && len(p.ResourceDrift) == 0 {
		c.Gaps = append(c.Gaps, Gap{
			Code: GapDriftNotKnown,
			Detail: "this plan records nothing that changed underneath the estate, and that " +
				"means one of two things it does not distinguish: either nothing drifted, " +
				"or refresh never ran and nobody looked",
		})
	}

	if st.Complete != nil && !*st.Complete {
		c.Gaps = append(c.Gaps, Gap{
			Code: GapIncompletePlan,
			Detail: "Terraform does not expect the state to match the configuration after " +
				"applying this, so what is below is not the whole change. The plan does not say why",
		})
	}

	if p != nil {
		// OUTPUTS ARE NOT RESOURCE CHANGES, and they were missed entirely.
		// Terraform marks an output unknown until apply in exactly the same
		// way it marks an attribute, and an output-only plan therefore
		// reported that there was nothing to assess and nothing unknown -
		// while carrying a value nobody could check.
		c.Outputs = len(p.OutputChanges)
		for _, oc := range p.OutputChanges {
			if oc == nil {
				continue
			}
			if unknownPaths(oc.AfterUnknown) != nil || isTrue(oc.AfterUnknown) {
				c.UnknownOutputs++
			}
		}
		if c.UnknownOutputs > 0 {
			c.Gaps = append(c.Gaps, Gap{
				Code:  GapUnknownOutputs,
				Count: c.UnknownOutputs, Of: c.Outputs,
				Detail: fmt.Sprintf("%d of %d outputs hold a value Terraform will not know "+
					"until it applies this, so what they will contain cannot be checked now",
					c.UnknownOutputs, c.Outputs),
			})
		}

		if undetermined, total, unexpanded := undeterminedChecks(p.Checks); undetermined > 0 || unexpanded > 0 {
			detail := fmt.Sprintf("%d of %d checked objects could not be determined "+
				"before apply, so whether they hold is not known yet", undetermined, total)
			if unexpanded > 0 {
				// A DIFFERENT SILENCE INSIDE THE SAME FIELD. A check whose
				// objects Terraform could not even work out has no instances
				// to count, and counting it as one invents a cardinality the
				// plan does not state. Said on its own terms instead.
				detail = fmt.Sprintf("%s. A further %s could not be expanded at all, so "+
					"how many objects %s covers is not known either",
					detail, pick(unexpanded == 1, "1 check", fmt.Sprintf("%d checks", unexpanded)),
					pick(unexpanded == 1, "it", "they"))
				if undetermined == 0 {
					detail = fmt.Sprintf("%s could not be expanded at all, so how many "+
						"objects %s covers is not known, nor whether they hold",
						pick(unexpanded == 1, "1 check", fmt.Sprintf("%d checks", unexpanded)),
						pick(unexpanded == 1, "it", "they"))
				}
			}
			c.Gaps = append(c.Gaps, Gap{
				Code:  GapChecksUndetermined,
				Count: undetermined, Of: total,
				Detail: detail,
			})
		}
		if n := len(p.DeferredChanges); n > 0 {
			c.Gaps = append(c.Gaps, Gap{
				Code:  GapDeferred,
				Count: n,
				Detail: fmt.Sprintf("Terraform postponed %s, which %s not assessed here "+
					"and %s not in the counts below",
					pick(n == 1, "1 change", fmt.Sprintf("%d changes", n)),
					pick(n == 1, "is", "are"), pick(n == 1, "is", "are")),
			})
		}
	}

	c.Drift = drift
	c.Headline = coverageHeadline(c)
	return c
}

// undeterminedChecks counts the check INSTANCES whose status is unknown, how
// many instances there are in total, and how many static declarations
// Terraform could not expand into instances at all.
//
// THE DENOMINATOR IS INSTANTIATED OBJECTS, and a static entry with no
// instances is not one. Terraform emits a static result with no instances in
// two cases, and they mean opposite things: `pass` with none means expansion
// established there are zero objects to check, and `unknown` with none means
// it could not establish them. Counting either as one object invents a
// cardinality the plan does not state - the first has none, and the second has
// an unknown number. So neither joins the denominator, and the second is
// returned separately to be said in its own words.
func undeterminedChecks(checks []tfjson.CheckResultStatic) (undetermined, total, unexpanded int) {
	for _, c := range checks {
		if len(c.Instances) == 0 {
			if c.Status == tfjson.CheckStatusUnknown {
				unexpanded++
			}
			continue
		}
		for _, i := range c.Instances {
			total++
			if i.Status == tfjson.CheckStatusUnknown {
				undetermined++
			}
		}
	}
	return undetermined, total, unexpanded
}

// isTrue reports whether an after_unknown value is the bare `true` Terraform
// uses when a whole value is unknown, as opposed to a structure with unknown
// leaves inside it.
func isTrue(v interface{}) bool {
	b, ok := v.(bool)
	return ok && b
}

// coverageHeadline is the one sentence, and it is said either way.
//
// The positive case is not decoration. A reader who sees no coverage section
// cannot tell whether everything was checkable or whether the tool did not
// look, and those are the two things this whole type exists to keep apart.
func coverageHeadline(c Coverage) string {
	// THE TWO SENTENCES SHARE NO PHRASE, deliberately. They used to both
	// contain "could be assessed", so a test looking for the affirmative one
	// passed on the negative one and would have gone on passing if the
	// affirmative path were deleted outright.
	if len(c.Gaps) == 0 {
		if c.Changes == 0 && c.Outputs == 0 && c.Drift == 0 {
			return "Nothing in this plan is hidden from review, because there is nothing in it"
		}
		if c.Changes == 0 && c.Outputs == 0 {
			// Drift and nothing else. "There is nothing in it" would sit
			// directly above a section listing what changed underneath.
			return "This plan proposes no changes, and everything it does record " +
				"could be read in full"
		}
		return fmt.Sprintf("Nothing here is hidden from review: all %d %s and %d %s in this "+
			"plan can be checked before it is applied",
			c.Changes, pick(c.Changes == 1, "change", "changes"),
			c.Outputs, pick(c.Outputs == 1, "output", "outputs"))
	}
	return fmt.Sprintf("%d of %d %s were read in full, and %s below %s what the rest of this "+
		"plan does not say",
		c.Assessed, c.Changes, pick(c.Changes == 1, "change", "changes"),
		pick(len(c.Gaps) == 1, "the note", fmt.Sprintf("the %d notes", len(c.Gaps))),
		pick(len(c.Gaps) == 1, "is", "are"))
}
