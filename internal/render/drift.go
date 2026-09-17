package render

import (
	"fmt"
	"io"

	"github.com/dbhq-uk/terraken/internal/assess"
)

// Rendering for what changed underneath the estate. See
// internal/assess/drift.go for the ranking and for why it is a separate list.
//
// ITS OWN SECTION, ALWAYS, IN EVERY FORMAT. The one thing this must never do
// is let a drift entry be mistaken for a planned change: one already happened
// and the other has not, and the verbs are the same words. So it gets its own
// heading, its own place in the document, and its own key in the machine
// formats - never a flag on an entry in the findings list, which is exactly
// the shape that gets missed.
//
// ABOVE THE FINDINGS, because it is the context for them. A plan that updates
// a database is a different proposition when the database was already changed
// by somebody else this morning.

const driftHeading = "Changed outside Terraform"

// drift writes the terminal section.
func (s style) drift(out *errWriter, entries []assess.Finding, said *said) {
	if len(entries) == 0 {
		return
	}
	out.line("")
	out.line(s.banner(upper(driftHeading), len(entries), ansiAmber))
	for _, f := range entries {
		out.line("")
		s.driftStanza(out, f, said)
	}
}

// driftStanza is `finding` with the past tense.
//
// IT DOES NOT REUSE `finding`, and that is the whole reason this function
// exists. `finding` prints verb(f.Kind), which says what the plan WILL do -
// so a resource that somebody had already deleted appeared under
// CHANGED OUTSIDE TERRAFORM with the word "destroy" beside it, reading as
// though this plan were about to destroy something that was already gone.
// That is the exact confusion this whole list is separate to prevent, and it
// arrived from sharing one line of rendering.
func (s style) driftStanza(out *errWriter, f assess.Finding, said *said) {
	s.emit(out, findingIndent, findingIndent, f.Address, ansiBold)

	// THE LEVEL IS PRINTED ON THE ENTRY, because this section has only one
	// heading. A planned finding takes its severity from the section it sits
	// under; drift is all in one amber block, so a database that vanished had
	// no visible CRITICAL anywhere and read exactly like a changed tag.
	s.emit(out, findingIndent, findingIndent,
		upper(f.LevelName)+"   "+driftVerb(f.Kind), colourFor(f.Level))

	ds := details(f, said)
	for i, d := range ds {
		connector, carry := s.g.branch, s.g.pipe
		if i == len(ds)-1 {
			connector, carry = s.g.last, s.g.blank
		}
		hang := findingIndent + carry
		sub := hang + "  "
		s.emit(out, findingIndent+connector, hang, d.text, d.emphasis)
		for _, line := range d.sub {
			s.emit(out, sub, sub+"  ", line, ansiGrey)
		}
	}
}

func markdownDrift(w io.Writer, entries []assess.Finding) {
	if len(entries) == 0 {
		return
	}
	fmt.Fprintf(w, "**%s**\n\n", driftHeading)
	fmt.Fprintln(w, "| Level | What happened | Resource | Notes |")
	fmt.Fprintln(w, "|---|---|---|---|")
	for _, f := range entries {
		var notes []string
		if f.DataLoss {
			notes = append(notes, dataLossSentence(f))
		}
		for _, a := range f.Annotations {
			notes = append(notes, prose(a.Detail))
		}
		fmt.Fprintf(w, "| %s | %s | %s | %s |\n",
			prose(upper(f.LevelName)), prose(driftVerb(f.Kind)), codeSpan(f.Address),
			joinNotes(notes))
	}
	fmt.Fprintln(w)

	// THE EVIDENCE, under the table, exactly as the findings table does it.
	// Without this a reorder annotation says "these lists" and names no lists,
	// and a sensitive one says "these values" and names no paths - the detail
	// sentence is written assuming its paths are printed somewhere.
	for _, f := range entries {
		for _, a := range f.Annotations {
			writeEvidence(w, f.Address, a)
		}
	}
	fmt.Fprintln(w)
}

func htmlDrift(out *errWriter, entries []assess.Finding) {
	if len(entries) == 0 {
		return
	}
	out.line(`<section class="drift" aria-labelledby="heading-drift">`)
	out.printf("<h2 id=\"heading-drift\"><span class=\"name\">%s</span>"+
		"<span class=\"track\"></span><span class=\"count\">%d</span></h2>\n",
		esc(driftHeading), len(entries))
	for _, f := range entries {
		out.line(`<article class="finding">`)
		out.printf("<h3><code>%s</code></h3>\n", esc(f.Address))
		// The level, on the entry. See driftStanza: one section means the
		// heading cannot carry it.
		out.printf("<p class=\"verb\"><span class=\"%s\">%s</span> %s</p>\n",
			levelClass(f.Level), esc(upper(f.LevelName)), esc(driftVerb(f.Kind)))
		out.line(`<ul class="notes">`)
		if f.DataLoss {
			out.printf("<li>%s</li>\n", esc(dataLossSentence(f)))
		}
		for _, a := range f.Annotations {
			out.printf("<li>%s\n", esc(a.Detail))
			if len(a.Paths) > 0 {
				out.line(`<ul class="evidence">`)
				for _, p := range a.Paths {
					out.printf("<li><code>%s</code></li>\n", esc(p))
				}
				out.line(`</ul>`)
			}
			out.line(`</li>`)
		}
		out.line(`</ul>`)
		out.line(`</article>`)
	}
	out.line(`</section>`)
}

// driftVerb is the past tense, because it already happened. `verb` says what
// the plan WILL do, and using it here would print "destroy" beside a resource
// that is already gone.
func driftVerb(k assess.Kind) string {
	switch k {
	case assess.KindDelete:
		return "gone"
	case assess.KindCreate:
		return "appeared"
	case assess.KindUpdate:
		return "changed"
	case assess.KindReplace:
		return "replaced"
	case assess.KindNoOp:
		return "unchanged"
	}
	return "changed in a way this build does not recognise"
}

func joinNotes(notes []string) string {
	out := ""
	for i, n := range notes {
		if i > 0 {
			out += "; "
		}
		out += n
	}
	return out
}
