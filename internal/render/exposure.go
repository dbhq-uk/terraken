package render

import (
	"fmt"
	"io"
	"strings"

	"github.com/dbhq-uk/terraken/internal/assess"
)

// Rendering for what the plan FILE is carrying - values that look like
// credentials and that Terraform did not mark sensitive. See
// internal/assess/credentials.go for the detection and for why this is a
// property of the report rather than of a finding.
//
// EVERY FORMAT PRINTS IT FIRST, above the findings. The findings are about
// whether to approve a change; this is about a file that is already on
// somebody's disk and already in somebody's CI log, and it needs acting on
// whichever way the review goes. A reader who has to scroll past twenty
// findings to reach it will reach it after they have decided.
//
// AND EVERY FORMAT PRINTS THE PATH AND THE CLASS, NEVER THE VALUE. The same
// rule as everywhere else in this package, and here it is the whole point: a
// detector that finds a credential and then prints it has made things worse
// than not looking.
const exposureHeading = "Credentials in the plan file"

// exposure writes the terminal block: the heading, one stanza per value, and
// the advice.
//
// The caveat goes to the footer instead, through said, because it is a
// standing caveat about the rule rather than a fact about any one value - the
// same treatment the reorder and rewrite notes get, for the same reason.
func (s style) exposure(out *errWriter, e assess.Exposure, said *said) {
	if !e.Any() {
		return
	}
	out.line("")
	out.line(s.banner(strings.ToUpper(exposureHeading), len(e.Values), ansiRed))
	for _, v := range e.Values {
		out.line("")
		s.emit(out, findingIndent, findingIndent, v.Address+"  "+v.Path, ansiBold)
		s.emit(out, findingIndent, findingIndent, v.Looks, ansiGrey)
	}
	out.line("")
	s.emit(out, findingIndent, findingIndent, e.Advice, "")
	said.note(e.Note)
}

func markdownExposure(w io.Writer, e assess.Exposure) {
	if !e.Any() {
		return
	}
	fmt.Fprintf(w, "**%s**\n\n", exposureHeading)
	fmt.Fprintln(w, "| Resource | Path | Looks like |")
	fmt.Fprintln(w, "|---|---|---|")
	for _, v := range e.Values {
		fmt.Fprintf(w, "| `%s` | `%s` | %s |\n", cell(v.Address), cell(v.Path), cell(v.Looks))
	}
	fmt.Fprintf(w, "\n%s\n\n%s\n\n", prose(e.Advice), prose(e.Note))
}

func htmlExposure(out *errWriter, e assess.Exposure) {
	if !e.Any() {
		return
	}
	out.line(`<section class="exposure" aria-labelledby="heading-exposure">`)
	out.printf("<h2 id=\"heading-exposure\"><span class=\"name\">%s</span><span class=\"track\"></span><span class=\"count\">%d</span></h2>\n",
		esc(exposureHeading), len(e.Values))
	out.line(`<ul class="exposed">`)
	for _, v := range e.Values {
		out.printf("<li><code>%s</code> <code>%s</code><span class=\"looks\">%s</span></li>\n",
			esc(v.Address), esc(v.Path), esc(v.Looks))
	}
	out.line(`</ul>`)
	out.printf("<p class=\"advice\">%s</p>\n", esc(e.Advice))
	out.printf("<p class=\"caution\">%s</p>\n", esc(e.Note))
	out.line(`</section>`)
}
