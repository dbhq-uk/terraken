package render

import (
	"fmt"
	"html"
	"io"

	"github.com/dbhq-uk/terraverdict/internal/assess"
)

// HTML writes the report as one self-contained document: inline CSS, no
// external stylesheet, no font, no image, no script. It opens from a
// file:// URL with nothing else on disk, which is the point - a review
// artefact that fetches anything is a review artefact that can be
// changed after you sent it.
//
// Every value that comes from the plan goes through esc. A resource
// address can carry a for_each key chosen by whoever wrote the
// Terraform, and this file gets opened by a reviewer, so the address is
// untrusted input in an HTML context. The <style> block is the other
// half of that: no plan-derived value is written anywhere inside it, so
// there is nothing there to break out of.
func HTML(w io.Writer, r assess.Report) error {
	out := &errWriter{w: w}

	out.line(`<!doctype html>`)
	out.line(`<html lang="en-GB">`)
	out.line(`<head>`)
	out.line(`<meta charset="utf-8">`)
	out.line(`<meta name="viewport" content="width=device-width, initial-scale=1">`)
	out.line(`<meta name="color-scheme" content="light dark">`)
	out.line(`<title>terraverdict report</title>`)
	out.line("<style>")
	out.line(stylesheet)
	out.line("</style>")
	out.line(`</head>`)
	out.line(`<body>`)
	out.line(`<main class="report">`)

	htmlMasthead(out, r)

	switch {
	case len(r.Findings) == 0 && r.Hidden == 0:
		out.line(`<p class="clear">No changes. This plan does nothing.</p>`)
	default:
		for _, lv := range []assess.Level{assess.Critical, assess.High, assess.Low, assess.Info} {
			group := findingsAt(r, lv)
			if len(group) == 0 {
				continue
			}
			htmlSection(out, lv, group)
		}
		htmlSummary(out, r)
	}

	out.line(`</main>`)
	out.line(`</body>`)
	out.line(`</html>`)
	return out.err
}

func htmlMasthead(out *errWriter, r assess.Report) {
	n := total(r)
	out.line(`<header class="masthead">`)
	out.line(`<h1>terraverdict</h1>`)
	line := fmt.Sprintf(`<p class="tally"><strong>%d %s</strong>`, n, plural(n, "finding", "findings"))
	if r.TerraformVersion != "" {
		line += fmt.Sprintf(` <span class="version">terraform %s</span>`, esc(r.TerraformVersion))
	}
	out.line(line + `</p>`)
	out.line(`</header>`)
}

func htmlSection(out *errWriter, lv assess.Level, group []assess.Finding) {
	cls := levelClass(lv)
	out.printf("<section class=\"level %s\" aria-labelledby=\"heading-%s\">\n", cls, cls)
	out.printf("<h2 id=\"heading-%s\"><span class=\"name\">%s</span><span class=\"track\"></span><span class=\"count\">%d</span></h2>\n",
		cls, lv.String(), len(group))
	for _, f := range group {
		htmlFinding(out, f)
	}
	out.line(`</section>`)
}

func htmlFinding(out *errWriter, f assess.Finding) {
	out.line(`<article class="finding">`)
	out.printf("<h3><code>%s</code></h3>\n", esc(f.Address))
	out.printf("<p class=\"verb\">%s</p>\n", esc(verb(f.Kind)))

	// An empty <ul> is invalid HTML, and a create with nothing else to
	// say is the commonest finding in any plan, so this case is the rule
	// rather than the exception.
	if f.DataLoss || f.Reason != "" || len(f.ReplacePaths) > 0 || len(f.Annotations) > 0 {
		out.line(`<ul class="notes">`)
		if f.DataLoss {
			out.line(`<li>holds data, so destroying it loses that data</li>`)
		}
		if f.Reason != "" {
			out.printf("<li>%s</li>\n", esc(f.Reason))
		}
		for _, p := range f.ReplacePaths {
			out.printf("<li>forces replacement <code>%s</code></li>\n", esc(p))
		}
		for _, a := range f.Annotations {
			htmlAnnotation(out, f.Address, a)
		}
		out.line(`</ul>`)
	}
	out.line(`</article>`)
}

func htmlAnnotation(out *errWriter, address string, a assess.Annotation) {
	if m := a.Moved; m != nil {
		other := m.To
		if address == m.To {
			other = m.From
		}
		out.printf("<li>%s\n", esc(annotationLabel(a.Code)))
		out.line(`<ul class="evidence">`)
		out.printf("<li>%d of %d attributes match <code>%s</code></li>\n",
			m.Matched, m.Compared, esc(other))
		if m.CrossModule {
			out.printf("<li>crosses a module boundary, %s to %s</li>\n",
				esc(moduleName(m.FromModule)), esc(moduleName(m.ToModule)))
		}
		out.line(`</ul>`)
		out.printf("<pre class=\"moved\"><code>moved {\n  from = %s\n  to   = %s\n}</code></pre>\n",
			esc(m.From), esc(m.To))
		out.line(`<p class="caution">Verify the pairing before using this block. Pasting the wrong ` +
			`one adopts a decommissioned resource's state under a new address, which is worse ` +
			`than the problem it would fix.</p>`)
		out.line(`</li>`)
		return
	}

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

func htmlSummary(out *errWriter, r assess.Report) {
	out.line(`<footer class="summary">`)
	out.line(`<ul class="tallies">`)
	for _, t := range tallies(r) {
		out.printf("<li class=\"%s\">%d %s</li>\n", levelClass(t.Level), t.Count, t.Level)
	}
	out.line(`</ul>`)
	if note := hiddenNote(r); note != "" {
		out.printf("<p class=\"held-back\">%s</p>\n", esc(note))
	}
	out.line(`</footer>`)
}

// esc is the only way a value from a plan is allowed to reach the HTML.
// It escapes for both a text node and a quoted attribute, so there is
// one rule to follow rather than two to get wrong.
func esc(s string) string {
	return html.EscapeString(s)
}

// levelClass maps a level onto its CSS class through a switch on the
// enum, never through Level.String or Finding.LevelName.
//
// That is deliberate. Report is an ordinary struct that a caller fills
// in, so LevelName is a string this package did not choose, and a string
// this package did not choose has no business landing in a class
// attribute. The switch can only ever produce one of five fixed values.
func levelClass(lv assess.Level) string {
	switch lv {
	case assess.Critical:
		return "level-critical"
	case assess.High:
		return "level-high"
	case assess.Low:
		return "level-low"
	case assess.Info:
		return "level-info"
	}
	return "level-unknown"
}

// stylesheet is the whole design, inlined. It holds no value from the
// plan and never will: keeping it a constant is what makes "an address
// cannot break out of the style block" a property of the code rather
// than a thing to remember.
const stylesheet = `*, *::before, *::after { box-sizing: border-box; }

:root {
  color-scheme: light dark;
  --bg: #f6f6f4;
  --panel: #ffffff;
  --fg: #14171c;
  --muted: #5b626f;
  --line: #e1e4e9;
  --code-bg: #eef0f3;
  --critical: #ad1f18;
  --high: #8a5200;
  --low: #1a5ea6;
  --info: #5b626f;
}

@media (prefers-color-scheme: dark) {
  :root {
    --bg: #0e1014;
    --panel: #161a21;
    --fg: #e6e9ef;
    --muted: #98a0ae;
    --line: #242a33;
    --code-bg: #1c212a;
    --critical: #ff7b6f;
    --high: #e2a63c;
    --low: #7cb0f5;
    --info: #98a0ae;
  }
}

html { -webkit-text-size-adjust: 100%; }

body {
  margin: 0;
  background: var(--bg);
  color: var(--fg);
  font: 16px/1.6 ui-sans-serif, system-ui, -apple-system, Segoe UI, Roboto, Helvetica, Arial, sans-serif;
}

code, pre {
  font-family: ui-monospace, SFMono-Regular, Menlo, Consolas, Liberation Mono, monospace;
}

.report { max-width: 50rem; margin: 0 auto; padding: 3rem 1.25rem 4rem; }

.masthead {
  border-bottom: 2px solid var(--fg);
  padding-bottom: 0.6rem;
  margin-bottom: 2.25rem;
}
.masthead h1 {
  margin: 0;
  font-size: 1.15rem;
  font-weight: 650;
  letter-spacing: 0.01em;
}
.masthead .tally { margin: 0.15rem 0 0; font-size: 0.9rem; color: var(--muted); font-weight: 400; }
.masthead .tally strong { color: var(--fg); font-weight: 600; }
.masthead .version { margin-left: 0.5rem; }

.level { margin: 0 0 2.25rem; }
.level-critical { --accent: var(--critical); }
.level-high { --accent: var(--high); }
.level-low { --accent: var(--low); }
.level-info { --accent: var(--info); }
.level-unknown { --accent: var(--muted); }

.level h2 {
  display: flex;
  align-items: center;
  gap: 0.75rem;
  margin: 0 0 0.8rem;
  font-size: 0.75rem;
  font-weight: 650;
  letter-spacing: 0.16em;
  text-transform: uppercase;
  color: var(--accent);
}
.level h2 .track { flex: 1; height: 1px; background: var(--line); }
.level h2 .count { font-variant-numeric: tabular-nums; letter-spacing: 0; }

.finding {
  background: var(--panel);
  border: 1px solid var(--line);
  border-left: 3px solid var(--accent);
  border-radius: 5px;
  padding: 0.8rem 1rem 0.9rem;
  margin: 0 0 0.6rem;
}
.finding h3 { margin: 0; font-size: 0.95rem; font-weight: 600; }
/* The address is the headline of the card, so it is set as one -
   monospaced for legibility, but without the chip that would demote it
   to an inline snippet. */
.finding h3 code {
  background: none;
  padding: 0;
  font-size: 0.95rem;
  font-weight: 650;
  overflow-wrap: anywhere;
}
.verb { margin: 0.1rem 0 0.6rem; font-size: 0.85rem; color: var(--muted); }

.notes { margin: 0; padding-left: 1.1rem; }
.notes > li { margin: 0.2rem 0; }
.notes > li::marker { color: var(--accent); }

code {
  background: var(--code-bg);
  border-radius: 3px;
  padding: 0.05em 0.35em;
  font-size: 0.88em;
  overflow-wrap: anywhere;
}

.evidence {
  list-style: none;
  margin: 0.35rem 0 0;
  padding: 0 0 0 0.85rem;
  border-left: 1px solid var(--line);
  font-size: 0.88rem;
  color: var(--muted);
}
.evidence li { margin: 0.15rem 0; }

pre.moved {
  margin: 0.5rem 0 0;
  padding: 0.6rem 0.8rem;
  background: var(--code-bg);
  border-radius: 4px;
  font-size: 0.82rem;
  line-height: 1.5;
  overflow-x: auto;
}
pre.moved code { background: none; padding: 0; font-size: inherit; }

.caution { margin: 0.45rem 0 0; font-size: 0.82rem; color: var(--muted); }

.clear { margin: 0; font-size: 1rem; }

.summary {
  display: flex;
  flex-wrap: wrap;
  align-items: baseline;
  justify-content: space-between;
  gap: 0.75rem 1.5rem;
  margin-top: 2.5rem;
  padding-top: 0.7rem;
  border-top: 2px solid var(--fg);
}
.tallies {
  display: flex;
  flex-wrap: wrap;
  gap: 0.35rem 1.4rem;
  list-style: none;
  margin: 0;
  padding: 0;
  font-size: 0.88rem;
  font-variant-numeric: tabular-nums;
}
.tallies .level-critical { color: var(--critical); }
.tallies .level-high { color: var(--high); }
.tallies .level-low { color: var(--low); }
.tallies .level-info { color: var(--info); }
.held-back { margin: 0; font-size: 0.82rem; color: var(--muted); }

@media print {
  body { background: #fff; }
  .finding { break-inside: avoid; }
}`
