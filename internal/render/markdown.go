package render

import (
	"fmt"
	"html"
	"io"
	"strings"

	"github.com/dbhq-uk/terraverdict/internal/assess"
)

// Markdown writes a table suitable for a pull request comment.
//
// A PR review queue is the reason this tool exists, so this format must
// carry everything the terminal carries. An annotation's reasoning goes
// in the Notes cell and its evidence goes under the table - never the
// bare annotation code on its own, which tells a reviewer nothing.
func Markdown(w io.Writer, r assess.Report) error {
	if len(r.Findings) == 0 && r.Hidden == 0 {
		_, err := fmt.Fprintln(w, "No changes. This plan does nothing.")
		return err
	}

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
			notes = append(notes, cell(f.Reason))
		}
		for _, p := range f.ReplacePaths {
			notes = append(notes, "forces replacement: `"+cell(p)+"`")
		}
		for _, a := range f.Annotations {
			notes = append(notes, cell(a.Detail))
		}
		fmt.Fprintf(w, "| %s | %s | `%s` | %s |\n",
			strings.ToUpper(f.LevelName), verb(f.Kind), cell(f.Address),
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
func cell(s string) string {
	s = strings.NewReplacer("\r\n", " ", "\n", " ", "\r", " ").Replace(s)
	return strings.ReplaceAll(s, "|", `\|`)
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
// The moved-block code fence below is markdown, not raw HTML, so From and
// To are written there unescaped on purpose.
func writeEvidence(w io.Writer, address string, a assess.Annotation) {
	if a.Moved == nil && len(a.Paths) == 0 {
		return
	}

	subject := "<code>" + html.EscapeString(address) + "</code>"
	if m := a.Moved; m != nil {
		subject = "<code>" + html.EscapeString(m.From) + "</code> and <code>" + html.EscapeString(m.To) + "</code>"
	}
	fmt.Fprintf(w, "\n<details>\n<summary>%s - %s</summary>\n\n", a.Code, subject)

	if m := a.Moved; m != nil {
		// Built from the structured evidence rather than reprinted from
		// Paths, so the block arrives as something a reader can check
		// line by line instead of as one run-on sentence.
		fmt.Fprintf(w, "If this is a rename, the moved block would be:\n\n"+
			"```terraform\nmoved {\n  from = %s\n  to   = %s\n}\n```\n\n", m.From, m.To)
		fmt.Fprintf(w, "%d of %d compared attributes are identical", m.Matched, m.Compared)
		if m.CrossModule {
			fmt.Fprintf(w, ", and the pair crosses a module boundary (%s to %s)",
				moduleName(m.FromModule), moduleName(m.ToModule))
		}
		fmt.Fprint(w, ". Verify the pairing before using this block: pasting the wrong one "+
			"adopts a decommissioned resource's state under a new address, which is worse "+
			"than the problem it would fix.\n")
	} else {
		fmt.Fprintln(w, "```")
		for _, p := range a.Paths {
			fmt.Fprintln(w, p)
		}
		fmt.Fprintln(w, "```")
	}

	fmt.Fprint(w, "\n</details>\n")
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
