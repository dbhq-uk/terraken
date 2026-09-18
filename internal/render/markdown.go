package render

import (
	"fmt"
	"html"
	"io"
	"strings"

	"github.com/dbhq-uk/terraken/internal/assess"
)

// Markdown writes a table suitable for a pull request comment.
//
// A PR review queue is the reason this tool exists, so this format must
// carry everything the terminal carries. An annotation's reasoning goes
// in the Notes cell and its evidence goes under the table - never the
// bare annotation code on its own, which tells a reviewer nothing.
func Markdown(w io.Writer, r assess.Report) error {
	// Plan content is untrusted input. See untrusted.go.
	r = sanitise(r)

	// See the terminal renderer: the no-changes line stays, and anything
	// coverage has to say goes underneath it rather than in place of it.
	if len(r.Findings) == 0 && r.Hidden == 0 && !r.Exposure.Any() && !r.Status.Any() &&
		len(r.Drift) == 0 && len(r.Checks) == 0 {
		if _, err := fmt.Fprintln(w, "No changes. This plan does nothing."); err != nil {
			return err
		}
		if r.Coverage.Complete() {
			return nil
		}
		fmt.Fprintln(w)
		markdownCoverage(w, r.Coverage)
		return nil
	}

	// What the FILE is carrying, above the table - see exposure.go.
	markdownExposure(w, r.Exposure)
	markdownStatus(w, r.Status)
	markdownCoverage(w, r.Coverage)
	markdownDrift(w, r.Drift)
	markdownChecks(w, r.Checks)

	// An empty table is worse than no table. When a filter has hidden
	// everything, go straight to the summary, which says so.
	if len(r.Findings) > 0 {
		fmt.Fprintln(w, "| Level | Change | Resource | Notes |")
		fmt.Fprintln(w, "|---|---|---|---|")
	}
	for _, f := range r.Findings {
		var notes []string
		if f.DataLoss {
			notes = append(notes, "holds data, so destroying it loses that data")
		}
		if f.Reason != "" {
			notes = append(notes, prose(f.Reason))
		}
		for _, p := range f.ReplacePaths {
			notes = append(notes, "forces replacement: "+codeSpan(p))
		}
		for _, a := range f.Annotations {
			note := prose(a.Detail)
			// The roll-up speaks for the whole resource where every other
			// note speaks for one attribute, and in this format they all
			// land in the same cell. Bold is what keeps it from reading as
			// one more note in the list.
			if a.Code == assess.AnnAllRewritten {
				note = "**" + note + "**"
			}
			notes = append(notes, note)
		}
		fmt.Fprintf(w, "| %s | %s | %s | %s |\n",
			prose(strings.ToUpper(f.LevelName)), verb(f), codeSpan(f.Address),
			strings.Join(notes, "; "))
	}

	// A missed-moved-block annotation is attached to both halves of the
	// pair, which reads correctly in the terminal where each finding has
	// its own stanza. Here every block lands in one list under one
	// table, so the same evidence twice in a row is just noise.
	seen := map[string]bool{}
	for _, f := range r.Findings {
		for _, a := range f.Annotations {
			if m := a.Moved; m != nil {
				key := m.From + "\x00" + m.To
				if seen[key] {
					continue
				}
				seen[key] = true
			}
			writeEvidence(w, f.Address, a)
		}
	}

	n := total(r)
	line := fmt.Sprintf("%d %s", n, plural(n, "finding", "findings"))
	if note := hiddenNote(r); note != "" {
		line += ", " + note
	}

	// Blank line to separate the summary from the table above it, but
	// not when a filter hid everything and there is no table - a report
	// should not open on an empty line.
	lead := "\n"
	if len(r.Findings) == 0 {
		lead = ""
	}
	_, err := fmt.Fprintf(w, "%s%s.\n", lead, line)
	return err
}

// cell escapes a value for a GitHub-flavoured markdown table cell. A
// table row is split on pipes before any inline markup is parsed, so an
// unescaped pipe breaks the row into extra columns even inside a code
// span - and a for_each key such as resource.this["a|b"] puts one in an
// address. A table row must also be a single line: a raw newline in a
// value ends the row early and spills the rest as unstructured text
// below the table, so newlines are flattened to a space before the pipe
// is escaped.
//
// A newline should never reach here now - sanitise turns one into a visible
// \n before any renderer sees the report - but the flattening stays, because
// this function's contract is "safe in a table cell" and a caller should not
// have to know which layer already ran.
func cell(s string) string {
	s = strings.NewReplacer("\r\n", " ", "\n", " ", "\r", " ").Replace(s)
	return strings.ReplaceAll(s, "|", `\|`)
}

// codeSpan wraps a value in backticks, using a delimiter long enough that
// nothing inside can close it.
//
// A SINGLE BACKTICK IS NOT ENOUGH, and this is the hole it fills. A code span
// opened with one backtick ends at the next one, so an address such as
// resource.this["`**approved**`"] closed the span and left its own markdown
// live in the middle of a table cell - which renders as bold text a reviewer
// reads as the tool's own. The fix is the rule the format already has: a span
// opened with n backticks is closed by the next run of exactly n, so counting
// the longest run inside and going one better cannot be beaten.
//
// The padding spaces are the second half of the rule. A span whose content
// begins or ends with a backtick needs one space inside each delimiter, and
// the renderer strips a single leading and trailing space when it displays it,
// so the value still reads exactly as it was.
func codeSpan(s string) string {
	s = cell(s)
	longest, run := 0, 0
	for _, r := range s {
		if r != '`' {
			run = 0
			continue
		}
		run++
		if run > longest {
			longest = run
		}
	}
	ticks := strings.Repeat("`", longest+1)

	// CommonMark strips ONE leading and ONE trailing space from a code span
	// when both are present and the content is not all spaces. So padding is
	// needed whenever the content starts or ends with a backtick - otherwise
	// the delimiter runs on - and ALSO whenever it starts or ends with a
	// space, or that space is the one the renderer eats and the value no
	// longer reads as the file holds it.
	//
	// Content that is all spaces is the documented exception: it is preserved
	// as it is, and padding it would add two more. Empty content gets no
	// padding either, for the same reason - a span of two spaces is not empty.
	pad := ""
	allSpaces := s != "" && strings.TrimLeft(s, " ") == ""
	if s != "" && !allSpaces {
		if strings.HasPrefix(s, "`") || strings.HasSuffix(s, "`") ||
			strings.HasPrefix(s, " ") || strings.HasSuffix(s, " ") {
			pad = " "
		}
	}
	return ticks + pad + s + pad + ticks
}

// mdSpecials are the characters that change what markdown MEANS in an inline
// context, as opposed to how it looks.
//
// The backslash comes first in the replacer for the usual reason: escaping it
// after the others would escape the backslashes just added. Characters that
// only have meaning at the start of a line - #, >, a list bullet - are absent
// because a table cell has no line of its own to start, newlines having been
// flattened out of it.
// THE PIPE IS NOT IN HERE, and leaving it out is the point. It is escaped
// afterwards, by cell(), because a table row is split on pipes BEFORE any
// inline markup is parsed - so the escaping has to be the last thing applied.
// Running it first cost a real defect: cell() produced \| , this replacer then
// escaped that backslash to \\| , and the doubled backslash no longer
// protected the pipe. The row split at that point and GitHub discarded every
// cell after the fourth, so a Notes column carrying a rename proposal and its
// caveat rendered as three characters and the rest of the explanation vanished.
// A report that quietly loses its own content is the failure this whole file
// exists to prevent, arriving from the defence rather than the attack.
var mdSpecials = strings.NewReplacer(
	`\`, `\\`,
	"`", "\\`",
	`*`, `\*`,
	`_`, `\_`,
	`[`, `\[`,
	`]`, `\]`,
	`~`, `\~`,
)

// prose is cell for a table cell rendered as ordinary markdown rather than
// inside a code span, so raw HTML in it would be interpreted.
//
// The distinction matters and is not cosmetic. A resource address can
// carry a for_each key chosen by whoever wrote the Terraform, and an
// annotation's detail quotes that address back. Addresses printed inside
// backticks are inert and must NOT be escaped, or a reader sees &lt;
// instead of <. Anything landing in the Notes column is live markdown and
// must be escaped, or a fork PR author can style or spoof the summary
// this tool exists to be trusted for.
//
// HTML-ESCAPING ALONE WAS NOT ENOUGH, which is the bug this fixes. It closed
// the tag route and left the markdown one wide open: **approved** still came
// out bold, [click](http://...) still came out a link, and an image reference
// still fetched from wherever it named. None of those needs a tag. The tool's
// own sentences contain none of these characters, so the escaping is invisible
// on every legitimate string and fires only on a payload.
// GFM AUTOLINKS ARE A STATED LIMIT, not an oversight. Escaping the bracket
// syntax stops a link whose LABEL lies about its target, which is the
// deception that matters. It does not stop GitHub turning a bare
// https://... in an address into a clickable link, because that is the
// renderer's extension and there is no escape for it. What a reader sees there
// is the URL itself, exactly as the plan holds it, so it cannot claim to be
// something else - and the alternative, inserting an invisible character to
// break it up, is the very thing sanitise() exists to remove.
func prose(s string) string {
	// Newlines flattened, HTML escaped, markdown escaped, and ONLY THEN the
	// pipe, because the table parser gets there first. See mdSpecials.
	out := strings.NewReplacer("\r\n", " ", "\n", " ", "\r", " ").Replace(s)
	out = mdSpecials.Replace(html.EscapeString(out))
	return strings.ReplaceAll(out, "|", `\|`)
}

// writeEvidence writes what a table cell cannot hold: the attribute
// paths behind an annotation, and the suggested moved block where there
// is one. It is collapsed in a <details> block so a long list does not
// bury the table above it.
//
// The <summary> line is raw HTML, not markdown - GitHub only starts
// re-parsing markdown after the blank line that follows it. Every
// address-derived string written there must go through html.EscapeString,
// or a for_each key crafted by whoever wrote the Terraform can close the
// <details> element early and inject content into the rendered summary.
//
// EVERYTHING BELOW IT IS MARKDOWN AGAIN, and that caught this function out
// twice. The moved block's fence was a fixed three backticks with the two
// addresses written into it raw, so a for_each key carrying three of its own
// closed the block and dropped the rest of the report into live markdown. The
// sentence after it interpolated two module names straight into prose, where
// an image reference fetches from wherever it names and bold text reads as the
// tool's own word. The fence is now sized to its contents and the sentence
// goes through prose(), like every other live-markdown context here.
func writeEvidence(w io.Writer, address string, a assess.Annotation) {
	if a.Moved == nil && len(a.Paths) == 0 {
		return
	}

	subject := "<code>" + html.EscapeString(address) + "</code>"
	if m := a.Moved; m != nil {
		subject = "<code>" + html.EscapeString(m.From) + "</code> and <code>" + html.EscapeString(m.To) + "</code>"
	}
	fmt.Fprintf(w, "\n<details>\n<summary>%s - %s</summary>\n\n",
		html.EscapeString(a.Code), subject)

	if m := a.Moved; m != nil {
		// Built from the structured evidence rather than reprinted from
		// Paths, so the block arrives as something a reader can check
		// line by line instead of as one run-on sentence.
		//
		// The fence outruns whatever the two addresses hold, and the info
		// string stays on the opening line so the block is still highlighted
		// as Terraform.
		fence := fenceFor([]string{m.From, m.To})
		fmt.Fprintf(w, "If this is a rename, the moved block would be:\n\n"+
			"%sterraform\nmoved {\n  from = %s\n  to   = %s\n}\n%s\n\n",
			fence, m.From, m.To, fence)
		fmt.Fprintf(w, "%d of %d compared attributes are identical", m.Matched, m.Compared)
		if m.CrossModule {
			fmt.Fprintf(w, ", and the pair crosses a module boundary (%s to %s)",
				prose(moduleName(m.FromModule)), prose(moduleName(m.ToModule)))
		}
		fmt.Fprint(w, ". Verify the pairing before using this block: pasting the wrong one "+
			"adopts a decommissioned resource's state under a new address, which is worse "+
			"than the problem it would fix.\n")
	} else {
		fence := fenceFor(a.Paths)
		fmt.Fprintln(w, fence)
		for _, p := range a.Paths {
			fmt.Fprintln(w, p)
		}
		fmt.Fprintln(w, fence)
	}

	fmt.Fprint(w, "\n</details>\n")
}

// fenceFor returns a code fence long enough that nothing in lines can close
// it early.
//
// A fenced block is closed by a run of at least as many backticks as opened
// it, so three backticks inside the evidence would end the block and drop
// everything after it back into live markdown - where it is styled,
// interpreted, and able to say something the plan does not. The content is
// untrusted: an attribute path carries a for_each key chosen by whoever
// wrote the Terraform, and an unrecognised action verb is a string read
// straight out of the plan file.
//
// Escaping is not an option here, because the point of the block is to show
// the text exactly as the file holds it. Outrunning the longest run is, and
// it is correct for every input rather than for the ones somebody thought
// of. Three is the floor so ordinary evidence looks like ordinary evidence.
func fenceFor(lines []string) string {
	longest := 0
	for _, l := range lines {
		run := 0
		for _, r := range l {
			if r != '`' {
				run = 0
				continue
			}
			run++
			if run > longest {
				longest = run
			}
		}
	}
	n := 3
	if longest >= n {
		n = longest + 1
	}
	return strings.Repeat("`", n)
}

// moduleName names a module for display, matching how the terminal
// names it. An empty module address is the root module, not a module
// with no name.
func moduleName(addr string) string {
	if addr == "" {
		return "the root module"
	}
	return addr
}
