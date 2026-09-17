package render

import (
	"fmt"
	"io"

	"github.com/dbhq-uk/terraken/internal/assess"
)

// Rendering for checks Terraform could not confirm. See
// internal/assess/checks.go for what is reported and, more importantly, what
// is not.
//
// NO MESSAGE REACHES HERE, because CheckFinding has nowhere to hold one. The
// `error_message` on a check is author-written text that Terraform
// interpolates, and it can carry an attribute value - so the report names the
// check and says what its status means, and the reader gets the message from
// the plan output where they already have it.

const checksHeading = "Checks Terraform could not confirm"

func (s style) checks(out *errWriter, entries []assess.CheckFinding) {
	if len(entries) == 0 {
		return
	}
	out.line("")
	out.line(s.banner(upper(checksHeading), len(entries), ansiAmber))
	for _, c := range entries {
		out.line("")
		s.emit(out, findingIndent, findingIndent, c.Address, ansiBold)
		s.emit(out, findingIndent, findingIndent, c.Kind+"   "+c.Status+problemCount(c), ansiGrey)
		s.emit(out, findingIndent+s.g.last, findingIndent+s.g.blank, c.Detail, "")
	}
}

func markdownChecks(w io.Writer, entries []assess.CheckFinding) {
	if len(entries) == 0 {
		return
	}
	fmt.Fprintf(w, "**%s**\n\n", checksHeading)
	fmt.Fprintln(w, "| Status | What | Object | Problems | What that means |")
	fmt.Fprintln(w, "|---|---|---|---|---|")
	for _, c := range entries {
		fmt.Fprintf(w, "| %s | %s | %s | %d | %s |\n",
			prose(upper(c.Status)), prose(c.Kind), codeSpan(c.Address), c.Problems, prose(c.Detail))
	}
	fmt.Fprintln(w)
}

func htmlChecks(out *errWriter, entries []assess.CheckFinding) {
	if len(entries) == 0 {
		return
	}
	out.line(`<section class="checks" aria-labelledby="heading-checks">`)
	out.printf("<h2 id=\"heading-checks\"><span class=\"name\">%s</span>"+
		"<span class=\"track\"></span><span class=\"count\">%d</span></h2>\n",
		esc(checksHeading), len(entries))
	out.line(`<ul class="check-list">`)
	for _, c := range entries {
		out.printf("<li><code>%s</code> <strong>%s</strong> %s%s<span class=\"meaning\">%s</span></li>\n",
			esc(c.Address), esc(upper(c.Status)), esc(c.Kind), esc(problemCount(c)), esc(c.Detail))
	}
	out.line(`</ul>`)
	out.line(`</section>`)
}

// problemCount is the recorded problem entries, where there are any. It is a
// COUNT and never a message: error_message is author-written text Terraform
// interpolates, and a plan generated while building this carried a live
// GitHub token in one.
//
// Not quite "how many assertions failed" either - Terraform omits an empty
// failure message, and an evaluation error goes out as a diagnostic rather
// than as a problem here.
func problemCount(c assess.CheckFinding) string {
	if c.Problems == 0 {
		return ""
	}
	return fmt.Sprintf("   %d %s recorded", c.Problems,
		plural(c.Problems, "problem", "problems"))
}
