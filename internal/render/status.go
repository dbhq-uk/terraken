package render

import (
	"fmt"
	"io"
	"strings"

	"github.com/dbhq-uk/terraken/internal/plan"
)

// Rendering for what the plan says about ITSELF. See internal/plan/status.go
// for the three flags and why each is a pointer.
//
// IT IS METADATA, NOT A FINDING, and every decision here follows from that.
// It sits in the header rather than in the list, it is not counted, it is not
// coloured by severity, and no display filter touches it. A failed planning
// operation is not a resource change, and `critical` in this tool means one
// thing only - a resource type that holds data being destroyed.
//
// THE WORDING IS TERRAFORM'S, NOT THIS TOOL'S OPINION. `complete: false` is
// reported as "another plan and apply round is expected", which is what the
// specification says it means. It is NOT reported as "-target was used":
// `-target` and deferred changes both produce it and nothing in the file says
// which, so naming a cause would be an inference the plan does not support.
//
// `applyable: false` is stated as a fact and never as a problem. Terraform
// defines applyable as true only when planning succeeded AND the plan calls
// for a meaningful change, so a perfectly clean no-op plan is not applyable -
// and anything that read the flag as risk would penalise the plan that most
// deserves to pass.

const statusHeading = "What this plan says about itself"

// statusLine is one flag, as a label and the sentence that goes with it.
type statusLine struct {
	name string
	// state is "yes", "no" or "not stated", which is the whole reason the
	// fields are pointers.
	state string
	// meaning is what that state means, in Terraform's own terms, or empty
	// where the state carries no consequence worth a sentence.
	meaning string
}

// statusLines turns a Status into what every format prints, so the three
// renderers cannot drift apart on the wording.
//
// ALL THREE ARE ALWAYS LISTED once the block renders at all, including the
// ones the plan did not state. Listing only what was stated looked tidier and
// was worse: a plan carrying `complete: true` and nothing else showed one line
// saying complete, and a reader had no way to tell whether planning succeeded
// or whether this build simply could not tell. "Not stated" is a third state
// the issue asks for by name, and it is the one that says which of those two
// it is.
//
// The block as a whole is still suppressed when NOTHING is stated - see
// Status.Any - so a plan from before these flags existed produces no status
// block, and the change is invisible to everybody it has nothing to tell.
func statusLines(s plan.Status) []statusLine {
	return []statusLine{
		erroredLine(s.Errored),
		completeLine(s.Complete),
		applyableLine(s.Applyable),
	}
}

func erroredLine(v *bool) statusLine {
	l := statusLine{name: "errored", state: state(v)}
	switch {
	case v == nil:
		l.meaning = notStated
	case *v:
		l.meaning = "planning failed, so this plan cannot be applied. " +
			"The changes below are what Terraform worked out before it stopped, " +
			"which is less than the whole change"
	}
	return l
}

func completeLine(v *bool) statusLine {
	l := statusLine{name: "complete", state: state(v)}
	switch {
	case v == nil:
		l.meaning = notStated
	case !*v:
		l.meaning = "Terraform does not expect the state to match the configuration " +
			"after this is applied, so at least one more plan and apply round is expected. " +
			"The plan does not say why"
	}
	return l
}

func applyableLine(v *bool) statusLine {
	l := statusLine{name: "applyable", state: state(v)}
	switch {
	case v == nil:
		l.meaning = notStated
	case !*v:
		l.meaning = "Terraform would not expect an automation to apply this. " +
			"That is not a fault: a plan that changes nothing is not applyable either"
	}
	return l
}

// notStated is the same sentence on every flag that carries it, because the
// reason is the same every time and it is about this build rather than about
// this plan.
const notStated = "this plan does not say, which older versions of Terraform " +
	"and OpenTofu do not. It is not the same as no"

func state(v *bool) string {
	if v == nil {
		return "not stated"
	}
	return yesNo(*v)
}

func yesNo(b bool) string {
	if b {
		return "yes"
	}
	return "no"
}

// status writes the terminal block, above the findings and below the
// exposure, in the same shape as every other heading in the report.
func (s style) status(out *errWriter, st plan.Status) {
	if !st.Any() {
		return
	}
	lines := statusLines(st)
	out.line("")
	out.line(s.banner(strings.ToUpper(statusHeading), len(lines), ansiBlue))
	for _, l := range lines {
		out.line("")
		s.emit(out, findingIndent, findingIndent, l.name+"   "+l.state, ansiBold)
		if l.meaning != "" {
			s.emit(out, findingIndent, findingIndent, l.meaning, ansiGrey)
		}
	}
}

func markdownStatus(w io.Writer, st plan.Status) {
	if !st.Any() {
		return
	}
	fmt.Fprintf(w, "**%s**\n\n", statusHeading)
	fmt.Fprintln(w, "| | |")
	fmt.Fprintln(w, "|---|---|")
	for _, l := range statusLines(st) {
		cell := prose(l.state)
		if l.meaning != "" {
			cell += " - " + prose(l.meaning)
		}
		fmt.Fprintf(w, "| %s | %s |\n", prose(l.name), cell)
	}
	fmt.Fprintln(w)
}

func htmlStatus(out *errWriter, st plan.Status) {
	if !st.Any() {
		return
	}
	lines := statusLines(st)
	out.line(`<section class="plan-status" aria-labelledby="heading-status">`)
	out.printf("<h2 id=\"heading-status\"><span class=\"name\">%s</span>"+
		"<span class=\"track\"></span><span class=\"count\">%d</span></h2>\n",
		esc(statusHeading), len(lines))
	out.line(`<ul class="status">`)
	for _, l := range lines {
		out.printf("<li><strong>%s</strong> %s", esc(l.name), esc(l.state))
		if l.meaning != "" {
			out.printf("<span class=\"meaning\">%s</span>", esc(l.meaning))
		}
		out.line(`</li>`)
	}
	out.line(`</ul>`)
	out.line(`</section>`)
}
