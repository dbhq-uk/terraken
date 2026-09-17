package assess

import tfjson "github.com/hashicorp/terraform-json"

// Checks Terraform could not confirm: the ones that failed, and the ones it
// could not determine before apply.
//
// WHY THIS IS WORTH REPORTING AT ALL. A check block that fails at plan time is
// reported by Terraform as a WARNING, and the plan still succeeds - so a
// pipeline running `terraform plan` sees exit 0 and carries on. That is a
// failure a team wrote down as mattering, passing silently through the one
// gate they have. The roadmap called it the most valuable thing in this layer
// if standalone check blocks turned out to appear in plan JSON, and a real
// plan confirmed they do.
//
// THE MESSAGE IS NEVER REPORTED, and that is the decision this type is built
// around. `error_message` is written by whoever wrote the configuration and
// Terraform INTERPOLATES it - a plan generated to test this carried a live
// GitHub token in a check's failure message. Printing it would put an
// attribute value in the output, which is the one thing this tool does not do,
// and no carve-out for "but the author wrote it deliberately" survives contact
// with a reader who has to trust the guarantee absolutely. So the report says
// which check failed, what kind of check it is, and how many problems it had.
// The message is in the plan output, where the reader already has it.
//
// PASSING CHECKS ARE NOT REPORTED. A report listing everything that went right
// is one nobody reads to the end.

// What kind of checkable object this is. Terraform distinguishes them in the
// address, and a reader needs to: a check block is a standalone assertion
// somebody wrote about the estate, and a postcondition belongs to a resource.
const (
	CheckKindCheckBlock = "check block"

	// RESOURCE CONDITION, NOT POSTCONDITION. Terraform aggregates a
	// resource's preconditions AND postconditions under one checkable object,
	// and the JSON does not say which of them failed. Calling it a
	// postcondition was a claim the file cannot substantiate.
	CheckKindResourceCondition = "resource condition"

	CheckKindOutput   = "output condition"
	CheckKindVariable = "variable validation"
)

// CheckFinding is one checkable object Terraform could not confirm.
//
// IT HAS NOWHERE TO PUT A MESSAGE, deliberately. A field for one would be
// filled in by somebody eventually, and the test that asserts this type has no
// message field is what stops that being a quiet afternoon's work.
type CheckFinding struct {
	// Address is the instance address where there is one, so an expanded
	// postcondition names the object a reader has to go and look at:
	// terraform_data.app["production"], not terraform_data.app.
	Address string `json:"address"`

	// Kind is check block, resource postcondition, and so on.
	Kind string `json:"kind"`

	// Status is Terraform's own word: "fail" or "unknown". "error" is
	// possible too and passed through as it comes.
	Status string `json:"status"`

	// Problems is how many problem ENTRIES Terraform recorded on this object,
	// which is not quite the same as how many assertions failed: it omits an
	// empty failure message, and an evaluation error is reported as a
	// diagnostic elsewhere rather than as a problem here. The count is safe
	// where the messages are not.
	Problems int `json:"problems"`

	// Detail is the tool's own sentence about what this status means, which
	// is the part a reader most needs for a check block: that Terraform
	// reported it as a warning and let the plan succeed.
	Detail string `json:"detail"`
}

// checksOf reports every checkable object Terraform did not confirm.
func checksOf(p *tfjson.Plan) []CheckFinding {
	if p == nil || len(p.Checks) == 0 {
		return nil
	}

	var out []CheckFinding
	for _, c := range p.Checks {
		kind := checkKind(c.Address.Kind)

		// INSTANCES FIRST, because the instance is where the answer is and
		// where the address a reader can act on is. The static entry above
		// them carries an aggregate status, and Terraform gives fail and
		// error precedence over unknown when aggregating - so reporting the
		// parent instead would collapse three objects into one and lose which
		// of them was the problem.
		if len(c.Instances) > 0 {
			for _, i := range c.Instances {
				if !worthReporting(string(i.Status)) {
					continue
				}
				out = append(out, CheckFinding{
					Address:  displayAddress(i.Address.ToDisplay, c.Address.ToDisplay),
					Kind:     kind,
					Status:   string(i.Status),
					Problems: len(i.Problems),
					Detail:   checkDetail(kind, string(i.Status)),
				})
			}
			continue
		}

		// No instances at all. Terraform emits that in two cases that mean
		// opposite things: `pass` means expansion established there are zero
		// objects, and `unknown` means it could not establish them. Only the
		// second is worth a line, and it says its own thing.
		if !worthReporting(string(c.Status)) {
			continue
		}
		// ONLY `unknown` MEANS THE EXPANSION FAILED. Terraform's aggregation
		// produces `pass` with no instances when expansion established there
		// are zero objects, and `unknown` when it could not establish them -
		// but the published representation also permits a zero-instance
		// `error`, and reporting that as unknown cardinality would be
		// inventing a cause. Anything else falls through to the ordinary
		// sentence for its status.
		detail := checkDetail(kind, string(c.Status))
		if c.Status == tfjson.CheckStatusUnknown {
			detail = "Terraform could not work out which objects this covers, so neither " +
				"how many there are nor whether they hold is known"
		}
		out = append(out, CheckFinding{
			Address: c.Address.ToDisplay,
			Kind:    kind,
			Status:  string(c.Status),
			Detail:  detail,
		})
	}
	return out
}

// worthReporting is the filter, and it is the whole editorial decision in this
// file. A passing check is not news. A failing one is news CI is currently
// stepping over, and an undetermined one is the tool's favourite kind of fact.
func worthReporting(status string) bool {
	return status != string(tfjson.CheckStatusPass)
}

func checkKind(k tfjson.CheckKind) string {
	switch k {
	case "check":
		return CheckKindCheckBlock
	case "resource", "data":
		return CheckKindResourceCondition
	case "output_value":
		return CheckKindOutput
	case "var":
		return CheckKindVariable
	}
	// Named rather than guessed. A kind this build has not seen is reported
	// as itself, which is the same honesty the action vocabulary gets.
	return string(k)
}

// checkDetail is what the status means, in the tool's own words.
//
// The sentence for a FAILED CHECK BLOCK is the point of this whole feature:
// Terraform reports it as a warning and the plan still succeeds, so a
// pipeline sees exit 0 and carries on. A reader who does not know that reads
// "fail" and assumes something stopped.
func checkDetail(kind, status string) string {
	switch status {
	case string(tfjson.CheckStatusFail):
		if kind == CheckKindCheckBlock {
			// WHAT CAN BE SAID FROM THE FILE, AND NOTHING MORE. The first
			// version of this said the pipeline sees exit 0, which is two
			// claims the plan does not support: another error may have failed
			// the plan anyway, and `terraform plan -detailed-exitcode` returns
			// 2 for a successful plan with changes in it. What is true is that
			// this failure on its own does not stop anything.
			return "this check failed while planning. Terraform treats an assertion " +
				"failure as a warning, so it does not fail the plan by itself and " +
				"nothing downstream has to notice it. The message is in the plan output"
		}
		return "this condition failed while planning. Terraform treats it as a warning, " +
			"so it does not fail the plan by itself. The message is in the plan output"
	case string(tfjson.CheckStatusUnknown):
		return "this could not be determined before apply, so whether it holds is not " +
			"known yet"
	case string(tfjson.CheckStatusError):
		// AT LEAST ONE, not all of them. An aggregate error means one
		// condition could not be evaluated; the others may have passed or
		// failed, and the file does not break it down.
		return "at least one condition here could not be evaluated. The error is in the " +
			"plan output"
	}
	// THE STATUS IS NOT INTERPOLATED INTO THIS SENTENCE. Terraform emits a
	// fixed vocabulary, but Report is built from a file this package does not
	// validate, and a status read straight out of one is plan-derived text -
	// putting it in a sentence this file calls "the tool's own" would make
	// that description untrue.
	return "Terraform reported a status for this that this build does not recognise, so " +
		"what it means has not been assessed"
}

// displayAddress prefers the instance's own address and falls back to the
// static one. An expanded postcondition gives terraform_data.app["production"],
// which is what a reader needs; an unexpanded check block gives the same
// address at both levels.
func displayAddress(instance, static string) string {
	if instance != "" {
		return instance
	}
	return static
}
