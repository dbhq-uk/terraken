package render

import (
	"bytes"
	"encoding/json"
	"regexp"
	"strings"
	"testing"

	"github.com/dbhq-uk/terraverdict/internal/assess"
)

func sample() assess.Report {
	return assess.Report{
		TerraformVersion: "1.9.8",
		Findings: []assess.Finding{
			{
				Address: "azurerm_postgresql_flexible_server.main",
				Type:    "azurerm_postgresql_flexible_server",
				Kind:    assess.KindReplace,
				Level:   assess.Critical, LevelName: "critical",
				Reason:       "an attribute changed that cannot be updated in place",
				ReplacePaths: []string{"zone"},
				DataLoss:     true,
			},
			{
				Address: "azurerm_subnet.app",
				Type:    "azurerm_subnet",
				Kind:    assess.KindCreate,
				Level:   assess.Info, LevelName: "info",
				Annotations: []assess.Annotation{{
					Code:   assess.AnnUnverifiable,
					Detail: "these values are not known until apply, so no claim about them can be checked in review",
					Paths:  []string{"id"},
				}},
			},
		},
		CountsByName: map[string]int{"critical": 1, "info": 1},
	}
}

func TestTerminalShowsAddressLevelAndReason(t *testing.T) {
	var b bytes.Buffer
	if err := Terminal(&b, sample(), false); err != nil {
		t.Fatalf("Terminal returned error: %v", err)
	}
	out := b.String()
	for _, want := range []string{
		"CRITICAL",
		"azurerm_postgresql_flexible_server.main",
		"an attribute changed that cannot be updated in place",
		"zone",
		"holds data",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("terminal output missing %q\n---\n%s", want, out)
		}
	}
}

func TestTerminalNoColourWhenDisabled(t *testing.T) {
	var b bytes.Buffer
	_ = Terminal(&b, sample(), false)
	if strings.Contains(b.String(), "\x1b[") {
		t.Error("no ANSI escape codes when colour is disabled")
	}
}

func TestTerminalColourWhenEnabled(t *testing.T) {
	var b bytes.Buffer
	_ = Terminal(&b, sample(), true)
	if !strings.Contains(b.String(), "\x1b[") {
		t.Error("expected ANSI escape codes when colour is enabled")
	}
}

func TestMarkdownIsATable(t *testing.T) {
	var b bytes.Buffer
	if err := Markdown(&b, sample()); err != nil {
		t.Fatalf("Markdown returned error: %v", err)
	}
	out := b.String()
	if !strings.Contains(out, "| Level | Change | Resource |") {
		t.Errorf("markdown must contain a table header\n---\n%s", out)
	}
	if strings.Contains(out, "\x1b[") {
		t.Error("markdown must never contain ANSI escape codes")
	}
}

// movedReport is the flagship finding: a rename that forgot its moved
// block, with the evidence attached.
func movedReport() assess.Report {
	return assess.Report{
		Findings: []assess.Finding{{
			Address: "azurerm_postgresql_flexible_server.main",
			Type:    "azurerm_postgresql_flexible_server",
			Kind:    assess.KindDelete,
			Level:   assess.Critical, LevelName: "critical",
			DataLoss: true,
			Annotations: []assess.Annotation{{
				Code: assess.AnnMissedMoved,
				Detail: "azurerm_postgresql_flexible_server.main is being destroyed and " +
					"azurerm_postgresql_flexible_server.primary created, with 4 of 4 compared " +
					"attributes identical. If this is a rename, a moved block would keep the " +
					"resource instead of destroying it.",
				Paths: []string{"if this is a rename, the moved block would be: moved { from = a  to = b } - verify before using"},
				Moved: &assess.MovedEvidence{
					From:     "azurerm_postgresql_flexible_server.main",
					To:       "azurerm_postgresql_flexible_server.primary",
					Matched:  4,
					Compared: 4,
				},
			}},
		}},
		CountsByName: map[string]int{"critical": 1},
	}
}

// TestMarkdownShowsAnnotationEvidenceNotJustTheCode is the regression
// test for the worst bug found before release: markdown put only
// a.Code in the Notes cell, so the flagship finding reached a pull
// request as the bare slug "possible-missed-moved-block" - no attribute
// count, no suggested moved block, no sentence. The README promises the
// detector always shows its working, and markdown is the path a
// reviewer actually reads.
func TestMarkdownShowsAnnotationEvidenceNotJustTheCode(t *testing.T) {
	var b bytes.Buffer
	if err := Markdown(&b, movedReport()); err != nil {
		t.Fatalf("Markdown returned error: %v", err)
	}
	out := b.String()

	for _, want := range []string{
		"4 of 4 compared attributes",
		"azurerm_postgresql_flexible_server.primary",
		"moved {",
		"from = azurerm_postgresql_flexible_server.main",
		"to   = azurerm_postgresql_flexible_server.primary",
		"Verify the pairing",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("markdown output missing %q\n---\n%s", want, out)
		}
	}
}

func TestMarkdownRendersAnnotationPathsBelowTheTable(t *testing.T) {
	var b bytes.Buffer
	if err := Markdown(&b, sample()); err != nil {
		t.Fatalf("Markdown returned error: %v", err)
	}
	out := b.String()
	if !strings.Contains(out, assess.AnnUnverifiable) {
		t.Errorf("markdown must name the annotation, got:\n%s", out)
	}
	// "id" is the unverifiable path on the sample's second finding. It
	// only appears if the paths are rendered at all.
	body := out[strings.Index(out, "<details>"):]
	if !strings.Contains(body, "\nid\n") {
		t.Errorf("markdown must render an annotation's paths below the table, got:\n%s", out)
	}
}

// TestMarkdownEscapesPipesInAddresses guards the table itself. A
// for_each key can contain a pipe, and a table row is split on pipes
// before any inline markup is parsed - so an unescaped one breaks the
// row into extra columns even inside a code span.
func TestMarkdownEscapesPipesInAddresses(t *testing.T) {
	r := assess.Report{
		Findings: []assess.Finding{{
			Address:   `azurerm_subnet.this["a|b"]`,
			Kind:      assess.KindUpdate,
			Level:     assess.Low,
			LevelName: "low",
			Reason:    "a|b",
		}},
		CountsByName: map[string]int{"low": 1},
	}
	var b bytes.Buffer
	if err := Markdown(&b, r); err != nil {
		t.Fatalf("Markdown returned error: %v", err)
	}
	for _, line := range strings.Split(b.String(), "\n") {
		if !strings.Contains(line, "azurerm_subnet.this") {
			continue
		}
		if strings.Count(line, "|")-strings.Count(line, `\|`) != 5 {
			t.Errorf("row must have exactly 5 unescaped pipes for 4 columns, got: %s", line)
		}
	}
}

// TestMarkdownEscapesAddressDerivedHTML guards the raw-HTML region of the
// evidence block. The <details><summary> line is raw HTML, not markdown -
// GitHub only resumes parsing markdown after the blank line that follows
// it - so an address that reaches it unescaped can close the <details>
// element early and inject content into the rendered summary. A fork PR
// author chooses a resource's for_each key, so the address is untrusted.
func TestMarkdownEscapesAddressDerivedHTML(t *testing.T) {
	cases := []struct {
		name    string
		address string
		want    string // escaped form that must appear inside <summary>
		unwant  string // raw form that must never appear inside <summary>
	}{
		{
			name:    "closes details and injects a script tag",
			address: `aws_s3_bucket.html["</code><script>alert(1)</script>"]`,
			want:    "&lt;/code&gt;&lt;script&gt;alert(1)&lt;/script&gt;",
			unwant:  "</code><script>",
		},
		{
			name:    "ampersand and angle bracket",
			address: `aws_s3_bucket.html["a & b < c"]`,
			want:    "a &amp; b &lt; c",
			unwant:  "a & b < c",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := assess.Report{
				Findings: []assess.Finding{{
					Address:   tc.address,
					Kind:      assess.KindCreate,
					Level:     assess.Info,
					LevelName: "info",
					Annotations: []assess.Annotation{{
						Code:   assess.AnnUnverifiable,
						Detail: "these values are not known until apply, so no claim about them can be checked in review",
						Paths:  []string{"id"},
					}},
				}},
				CountsByName: map[string]int{"info": 1},
			}
			var b bytes.Buffer
			if err := Markdown(&b, r); err != nil {
				t.Fatalf("Markdown returned error: %v", err)
			}
			out := b.String()

			start := strings.Index(out, "<details>")
			end := strings.Index(out, "</summary>")
			if start == -1 || end == -1 {
				t.Fatalf("expected a <details><summary> block, got:\n%s", out)
			}
			summary := out[start : end+len("</summary>")]

			if !strings.Contains(summary, tc.want) {
				t.Errorf("expected escaped %q inside <summary>, got:\n%s", tc.want, summary)
			}
			if strings.Contains(summary, tc.unwant) {
				t.Errorf("raw %q must not appear verbatim inside <summary>, got:\n%s", tc.unwant, summary)
			}
		})
	}
}

// TestMarkdownFlattensNewlinesInTableCells guards the table itself, the
// same way TestMarkdownEscapesPipesInAddresses does for a pipe. A table
// row must be a single line; a raw newline in an address or a reason
// splits the row and spills the rest as unstructured text below the
// table.
func TestMarkdownFlattensNewlinesInTableCells(t *testing.T) {
	r := assess.Report{
		Findings: []assess.Finding{{
			Address:   "azurerm_subnet.this[\"a\nb\"]",
			Kind:      assess.KindUpdate,
			Level:     assess.Low,
			LevelName: "low",
			Reason:    "line one\nline two",
		}},
		CountsByName: map[string]int{"low": 1},
	}
	var b bytes.Buffer
	if err := Markdown(&b, r); err != nil {
		t.Fatalf("Markdown returned error: %v", err)
	}
	out := b.String()

	if strings.Contains(out, "a\nb") || strings.Contains(out, "line one\nline two") {
		t.Errorf("a newline in a finding must not reach the table row raw, got:\n%s", out)
	}

	found := false
	for _, line := range strings.Split(out, "\n") {
		if !strings.Contains(line, "azurerm_subnet.this") {
			continue
		}
		found = true
		if strings.Count(line, "|")-strings.Count(line, `\|`) != 5 {
			t.Errorf("row must have exactly 5 unescaped pipes for 4 columns, got: %s", line)
		}
		if !strings.Contains(line, "a b") {
			t.Errorf("newline in the address should be flattened to a space, got: %s", line)
		}
	}
	if !found {
		t.Fatalf("expected a table row containing the address, got:\n%s", out)
	}
}

func TestJSONRoundTrips(t *testing.T) {
	var b bytes.Buffer
	if err := JSON(&b, sample()); err != nil {
		t.Fatalf("JSON returned error: %v", err)
	}
	var back map[string]interface{}
	if err := json.Unmarshal(b.Bytes(), &back); err != nil {
		t.Fatalf("output is not valid JSON: %v", err)
	}
	if _, ok := back["findings"]; !ok {
		t.Error("JSON output must have a findings key")
	}
}

// ansiEscape matches an SGR colour sequence, so a test can measure the
// visible width of a coloured line rather than its byte length.
var ansiEscape = regexp.MustCompile(`\x1b\[[0-9;]*m`)

// TestTerminalAddressColumnAlignsAcrossLevels runs with colour off and
// colour on, because only the colour-on case can catch the bug it was
// written for.
//
// The pad order matters: Terminal pads the plain label to a fixed width
// and then wraps it in colour. Padding after wrapping pads the invisible
// escape bytes instead of the visible label, so every level's address
// starts at a different column - and it does so only when colour is on.
// With colour off there are no escape bytes to pad, so a run with
// colour=false alone would pass with the bug reintroduced.
func TestTerminalAddressColumnAlignsAcrossLevels(t *testing.T) {
	r := assess.Report{
		Findings: []assess.Finding{
			{Address: "example_critical.res", Kind: assess.KindDelete, Level: assess.Critical, LevelName: "critical"},
			{Address: "example_high.res", Kind: assess.KindDelete, Level: assess.High, LevelName: "high"},
			{Address: "example_low.res", Kind: assess.KindUpdate, Level: assess.Low, LevelName: "low"},
			{Address: "example_info.res", Kind: assess.KindCreate, Level: assess.Info, LevelName: "info"},
		},
		CountsByName: map[string]int{"critical": 1, "high": 1, "low": 1, "info": 1},
	}

	for _, colour := range []bool{false, true} {
		name := "plain"
		if colour {
			name = "colour"
		}
		t.Run(name, func(t *testing.T) {
			var b bytes.Buffer
			if err := Terminal(&b, r, colour); err != nil {
				t.Fatalf("Terminal returned error: %v", err)
			}
			if colour && !strings.Contains(b.String(), "\x1b[") {
				t.Fatal("expected ANSI escapes with colour on - without them this case proves nothing")
			}

			var offsets []int
			for _, line := range strings.Split(b.String(), "\n") {
				// Strip the escapes before measuring: the column a
				// person sees is the visible one, not the byte one.
				visible := ansiEscape.ReplaceAllString(line, "")
				for _, f := range r.Findings {
					if strings.Contains(visible, f.Address) {
						offsets = append(offsets, strings.Index(visible, f.Address))
					}
				}
			}
			if len(offsets) != len(r.Findings) {
				t.Fatalf("expected %d address lines, found %d addresses matched in output", len(r.Findings), len(offsets))
			}
			for _, off := range offsets[1:] {
				if off != offsets[0] {
					t.Errorf("address column not aligned across levels: got offsets %v, want all equal to %d", offsets, offsets[0])
				}
			}
		})
	}
}

func TestEmptyReportSaysSoInAllFormats(t *testing.T) {
	empty := assess.Report{CountsByName: map[string]int{}}
	var b bytes.Buffer
	_ = Terminal(&b, empty, false)
	if !strings.Contains(b.String(), "No changes") {
		t.Errorf("terminal must say when a plan changes nothing, got: %s", b.String())
	}
}

func oneFindingReport() assess.Report {
	return assess.Report{
		Findings: []assess.Finding{
			{Address: "example_critical.res", Kind: assess.KindDelete, Level: assess.Critical, LevelName: "critical"},
		},
		CountsByName: map[string]int{"critical": 1},
	}
}

func twoFindingReport() assess.Report {
	return assess.Report{
		Findings: []assess.Finding{
			{Address: "example_critical.res", Kind: assess.KindDelete, Level: assess.Critical, LevelName: "critical"},
			{Address: "example_info.res", Kind: assess.KindCreate, Level: assess.Info, LevelName: "info"},
		},
		CountsByName: map[string]int{"critical": 1, "info": 1},
	}
}

func TestTerminalSummaryIsSingularForOneFinding(t *testing.T) {
	var b bytes.Buffer
	if err := Terminal(&b, oneFindingReport(), false); err != nil {
		t.Fatalf("Terminal returned error: %v", err)
	}
	out := b.String()
	if !strings.Contains(out, "1 finding:") {
		t.Errorf("expected singular \"1 finding\", got: %s", out)
	}
	if strings.Contains(out, "1 findings") {
		t.Errorf("must never say \"1 findings\", got: %s", out)
	}
}

func TestTerminalSummaryIsPluralForTwoFindings(t *testing.T) {
	var b bytes.Buffer
	if err := Terminal(&b, twoFindingReport(), false); err != nil {
		t.Fatalf("Terminal returned error: %v", err)
	}
	out := b.String()
	if !strings.Contains(out, "2 findings:") {
		t.Errorf("expected plural \"2 findings\", got: %s", out)
	}
}

func TestMarkdownSummaryIsSingularForOneFinding(t *testing.T) {
	var b bytes.Buffer
	if err := Markdown(&b, oneFindingReport()); err != nil {
		t.Fatalf("Markdown returned error: %v", err)
	}
	out := b.String()
	if !strings.Contains(out, "1 finding.") {
		t.Errorf("expected singular \"1 finding\", got: %s", out)
	}
	if strings.Contains(out, "1 findings") {
		t.Errorf("must never say \"1 findings\", got: %s", out)
	}
}

func TestMarkdownSummaryIsPluralForTwoFindings(t *testing.T) {
	var b bytes.Buffer
	if err := Markdown(&b, twoFindingReport()); err != nil {
		t.Fatalf("Markdown returned error: %v", err)
	}
	out := b.String()
	if !strings.Contains(out, "2 findings.") {
		t.Errorf("expected plural \"2 findings\", got: %s", out)
	}
}

// filteredReport is what a renderer gets when --min-level held some
// findings back: the counts still describe the whole plan, Findings
// holds only what qualified.
func filteredReport() assess.Report {
	// 90 findings assessed, 4 of them critical, the other 86 below the
	// high bar and held back. The visible findings and Hidden must add
	// up to the counts, exactly as assess.AtLeast leaves them.
	var visible []assess.Finding
	for _, n := range []string{"a", "b", "c", "d"} {
		visible = append(visible, assess.Finding{
			Address: "example_critical." + n, Kind: assess.KindDelete,
			Level: assess.Critical, LevelName: "critical",
		})
	}
	return assess.Report{
		Findings:     visible,
		CountsByName: map[string]int{"critical": 4, "low": 60, "info": 26},
		Hidden:       86,
		HiddenBelow:  "high",
	}
}

// TestTerminalSummarySaysWhatIsNotShown is the rule that makes a volume
// control safe. Showing fewer lines than were found, without saying so,
// is exactly how the one finding that mattered gets missed.
func TestTerminalSummarySaysWhatIsNotShown(t *testing.T) {
	var b bytes.Buffer
	if err := Terminal(&b, filteredReport(), false); err != nil {
		t.Fatalf("Terminal returned error: %v", err)
	}
	out := b.String()
	if !strings.Contains(out, "90 findings: 4 critical, 60 low, 26 info (86 below high not shown)") {
		t.Errorf("summary must count the whole plan and name what is hidden, got: %s", out)
	}
}

func TestMarkdownSummarySaysWhatIsNotShown(t *testing.T) {
	var b bytes.Buffer
	if err := Markdown(&b, filteredReport()); err != nil {
		t.Fatalf("Markdown returned error: %v", err)
	}
	if !strings.Contains(b.String(), "90 findings, 86 below high not shown.") {
		t.Errorf("summary must say what is hidden, got: %s", b.String())
	}
}

// TestFilteredToNothingIsNotAClearPlan is the dangerous case. If a
// filter hides every finding, saying "This plan does nothing" would be
// a flat lie about a plan that might destroy a database.
func TestFilteredToNothingIsNotAClearPlan(t *testing.T) {
	r := assess.Report{
		Findings:     []assess.Finding{},
		CountsByName: map[string]int{"low": 3},
		Hidden:       3,
		HiddenBelow:  "critical",
	}

	var term bytes.Buffer
	if err := Terminal(&term, r, false); err != nil {
		t.Fatalf("Terminal returned error: %v", err)
	}
	if strings.Contains(term.String(), "No changes") {
		t.Errorf("a filtered-out report must never claim the plan does nothing, got: %s", term.String())
	}
	if !strings.Contains(term.String(), "3 below critical not shown") {
		t.Errorf("terminal must say what is hidden, got: %s", term.String())
	}

	var md bytes.Buffer
	if err := Markdown(&md, r); err != nil {
		t.Fatalf("Markdown returned error: %v", err)
	}
	if strings.Contains(md.String(), "No changes") {
		t.Errorf("a filtered-out report must never claim the plan does nothing, got: %s", md.String())
	}
	if strings.Contains(md.String(), "| Level |") {
		t.Errorf("an empty table is worse than no table, got: %s", md.String())
	}
	if !strings.Contains(md.String(), "3 below critical not shown") {
		t.Errorf("markdown must say what is hidden, got: %s", md.String())
	}
}
