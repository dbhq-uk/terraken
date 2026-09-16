package render

import (
	"bytes"
	"strings"
	"testing"

	"github.com/dbhq-uk/terraken/internal/assess"
)

func renderMarkdown(t *testing.T, r assess.Report) string {
	t.Helper()
	var b bytes.Buffer
	if err := Markdown(&b, r); err != nil {
		t.Fatalf("Markdown returned error: %v", err)
	}
	return b.String()
}

// rollUpReport is one update in place carrying a per-attribute annotation
// and the roll-up that speaks for the whole resource. The two together are
// the case every assertion in this file is about: the roll-up has to be
// tellable apart from the line above it.
func rollUpReport() assess.Report {
	return assess.Report{
		TerraformVersion: "1.9.8",
		Findings: []assess.Finding{{
			Address: "aws_iam_policy.pipeline",
			Type:    "aws_iam_policy",
			Kind:    assess.KindUpdate,
			Level:   assess.Low, LevelName: "low",
			Annotations: []assess.Annotation{
				{
					Code:    assess.AnnSameJSON,
					Detail:  "both sides parse as JSON and hold the same data. A consumer that compares the document byte for byte still sees a change.",
					Summary: "same JSON, written differently",
					Note:    "A consumer that compares the document byte for byte still sees a change.",
					Paths:   []string{"policy"},
				},
				{
					Code:    assess.AnnAllRewritten,
					Detail:  "every attribute this plan shows as changed here is a difference in how the value is written. It rules on none of them.",
					Summary: "every attribute this plan shows as changed here is a difference in how the value is written",
					Note:    "It rules on none of them.",
				},
			},
		}},
		CountsByName: map[string]int{"low": 1},
	}
}

// TestTerminalSetsTheRollUpApart. The roll-up is a statement about the
// whole resource where every other line is about one attribute, so it is
// set in bold. The per-attribute label above it must not be, or the
// distinction says nothing.
func TestTerminalSetsTheRollUpApart(t *testing.T) {
	out := renderTerminal(t, rollUpReport(), TerminalOptions{Colour: true, Width: 100})

	if !strings.Contains(out, ansiBold+"every attribute this plan shows as changed here") {
		t.Errorf("the roll-up must be set apart from the lines around it:\n%s", out)
	}
	if strings.Contains(out, ansiBold+"same JSON, written differently") {
		t.Errorf("a per-attribute annotation must not be emphasised, or the roll-up is not distinct:\n%s", out)
	}
}

// TestTerminalRollUpSurvivesPlain is the other half, and the half that
// matters more. --plain turns colour off, so bold is gone and the wording
// is all the reader has: a whole sentence rather than a label, with no
// attribute paths hanging under it.
func TestTerminalRollUpSurvivesPlain(t *testing.T) {
	out := renderTerminal(t, rollUpReport(), TerminalOptions{Colour: false, ASCII: true, Width: 100})

	if strings.Contains(out, "\x1b[") {
		t.Fatalf("--plain must emit no escape at all:\n%s", out)
	}
	lines := visibleLines(out)
	var rollUp int
	for i, l := range lines {
		if strings.Contains(l, "every attribute this plan shows as changed here") {
			rollUp = i
		}
	}
	if rollUp == 0 {
		t.Fatalf("the roll-up must be readable with colour off:\n%s", out)
	}
	// It closes the finding, so it hangs off the last connector and
	// nothing is listed beneath it.
	if !strings.Contains(lines[rollUp], asciiGlyphs.last) {
		t.Errorf("the roll-up must come last on the finding, got %q", lines[rollUp])
	}
}

// TestMarkdownSetsTheRollUpApart. Every note on a finding lands in one
// table cell here, so without bold the roll-up reads as one more note in a
// run of them.
func TestMarkdownSetsTheRollUpApart(t *testing.T) {
	out := renderMarkdown(t, rollUpReport())

	if !strings.Contains(out, "**every attribute this plan shows as changed here") {
		t.Errorf("the roll-up must be set apart inside the Notes cell:\n%s", out)
	}
	if strings.Contains(out, "**both sides parse as JSON") {
		t.Errorf("a per-attribute annotation must not be emphasised, or the roll-up is not distinct:\n%s", out)
	}
	// The roll-up names no attribute, so it has no evidence block. An
	// empty <details> would be a disclosure triangle that opens on nothing.
	if strings.Contains(out, assess.AnnAllRewritten+" - <code>") {
		t.Errorf("the roll-up has no paths, so it must not open an evidence block:\n%s", out)
	}
}

// TestHTMLSetsTheRollUpApart. It is lifted out of the run of bullets
// rather than set as another one.
func TestHTMLSetsTheRollUpApart(t *testing.T) {
	out := renderHTML(t, rollUpReport())

	if !strings.Contains(out, `<li class="rollup">every attribute this plan shows as changed here`) {
		t.Errorf("the roll-up must carry its own class:\n%s", out)
	}
	if !strings.Contains(out, ".notes > li.rollup {") {
		t.Error("the inline stylesheet must style the class, or the class distinguishes nothing")
	}
	// The document stays valid: the roll-up carries no paths, so it must
	// not open an evidence list it has nothing to put in.
	if strings.Contains(out, `<li class="rollup">`) && strings.Contains(out, "<ul class=\"evidence\">\n</ul>") {
		t.Error("an empty evidence list is invalid HTML")
	}
}
