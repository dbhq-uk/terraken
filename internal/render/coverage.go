package render

import (
	"fmt"
	"io"

	"github.com/dbhq-uk/terraken/internal/assess"
)

// Rendering for how much of the plan could be checked before apply. See
// internal/assess/coverage.go for the measurement.
//
// IT GOES NEAR THE TOP, with the shape summary, because it frames every
// finding below it. A reviewer who reads a ranked list and only afterwards
// learns that a third of the change could not be examined has read the list
// under the wrong impression.
//
// IT IS SAID EITHER WAY. A plan where everything is checkable gets one line
// saying so. Silence there would leave a reader to infer coverage from an
// absent section, and "nothing was hidden" and "the tool did not look" are
// exactly the two things this exists to keep apart.

const coverageHeading = "How much of this could be checked"

// coverage writes the terminal block.
func (s style) coverage(out *errWriter, c assess.Coverage) {
	out.line("")
	if c.Complete() {
		// One line, no banner. A clean answer should not take up as much room
		// as a list of problems, or it reads as one.
		s.emit(out, findingIndent, findingIndent, c.Headline, ansiGrey)
		return
	}

	out.line(s.banner(upper(coverageHeading), len(c.Gaps), ansiBlue))
	out.line("")
	s.emit(out, findingIndent, findingIndent, c.Headline, ansiBold)
	for _, g := range c.Gaps {
		out.line("")
		s.emit(out, findingIndent, findingIndent, g.Detail, ansiGrey)
	}
}

func markdownCoverage(w io.Writer, c assess.Coverage) {
	if c.Complete() {
		fmt.Fprintf(w, "%s\n\n", prose(c.Headline))
		return
	}
	fmt.Fprintf(w, "**%s**\n\n%s\n\n", coverageHeading, prose(c.Headline))
	for _, g := range c.Gaps {
		fmt.Fprintf(w, "- %s\n", prose(g.Detail))
	}
	fmt.Fprintln(w)
}

func htmlCoverage(out *errWriter, c assess.Coverage) {
	if c.Complete() {
		out.printf("<p class=\"coverage-clear\">%s</p>\n", esc(c.Headline))
		return
	}
	out.line(`<section class="coverage" aria-labelledby="heading-coverage">`)
	out.printf("<h2 id=\"heading-coverage\"><span class=\"name\">%s</span>"+
		"<span class=\"track\"></span><span class=\"count\">%d</span></h2>\n",
		esc(coverageHeading), len(c.Gaps))
	out.printf("<p class=\"headline\">%s</p>\n", esc(c.Headline))
	out.line(`<ul class="gaps">`)
	for _, g := range c.Gaps {
		out.printf("<li>%s</li>\n", esc(g.Detail))
	}
	out.line(`</ul>`)
	out.line(`</section>`)
}
