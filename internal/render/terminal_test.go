package render

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/dbhq-uk/terraken/internal/assess"
	tfjson "github.com/hashicorp/terraform-json"
)

// allLevels is a report with one finding at each of the four levels, so
// a test can assert about sections without the levels interfering.
func allLevels() assess.Report {
	return assess.Report{
		TerraformVersion: "1.16.1",
		Findings: []assess.Finding{
			{Address: "example_critical.res", Kind: assess.KindDelete, Level: assess.Critical, LevelName: "critical", DataLoss: true},
			{Address: "example_high.res", Kind: assess.KindReplace, Level: assess.High, LevelName: "high", ReplacePaths: []string{"zone"}},
			{Address: "example_low.res", Kind: assess.KindUpdate, Level: assess.Low, LevelName: "low"},
			{Address: "example_info.res", Kind: assess.KindCreate, Level: assess.Info, LevelName: "info"},
		},
		CountsByName: map[string]int{"critical": 1, "high": 1, "low": 1, "info": 1},
	}
}

// visibleLines strips ANSI escapes and returns the lines as a person
// sees them. Every layout assertion measures these, never the bytes.
func visibleLines(s string) []string {
	return strings.Split(strings.TrimRight(ansiEscape.ReplaceAllString(s, ""), "\n"), "\n")
}

func renderTerminal(t *testing.T, r assess.Report, opts TerminalOptions) string {
	t.Helper()
	var b bytes.Buffer
	if err := Terminal(&b, r, opts); err != nil {
		t.Fatalf("Terminal returned error: %v", err)
	}
	return b.String()
}

// TestTerminalGroupsFindingsIntoSeveritySections is the shape of the
// whole format: one heading per level, most severe first, with the
// count of that level beside it.
func TestTerminalGroupsFindingsIntoSeveritySections(t *testing.T) {
	out := renderTerminal(t, allLevels(), TerminalOptions{Width: testWidth})

	var order []string
	for _, line := range visibleLines(out) {
		for _, name := range []string{"CRITICAL", "HIGH", "LOW", "INFO"} {
			if strings.HasPrefix(line, name+" ") {
				order = append(order, name)
			}
		}
	}
	want := []string{"CRITICAL", "HIGH", "LOW", "INFO"}
	if strings.Join(order, ",") != strings.Join(want, ",") {
		t.Errorf("sections must run most severe first: got %v, want %v\n---\n%s", order, want, out)
	}
}

// TestTerminalOmitsSectionsWithNothingInThem is the rule that keeps the
// format honest at both ends. A heading with a zero beside it is noise
// pretending to be information, and on a clean-ish plan there are three
// of them.
func TestTerminalOmitsSectionsWithNothingInThem(t *testing.T) {
	out := renderTerminal(t, oneFindingReport(), TerminalOptions{Width: testWidth})
	for _, absent := range []string{"HIGH", "LOW", "INFO"} {
		if strings.Contains(out, absent+" ") {
			t.Errorf("a level with no findings must not get a section, found %q in:\n%s", absent, out)
		}
	}
	if !strings.Contains(out, "CRITICAL") {
		t.Errorf("the level that does have a finding must get a section:\n%s", out)
	}
}

// TestTerminalSectionHeadingsFillTheWidthExactly is the test the old
// alignment test became. The fill between a level name and its
// right-aligned count is measured from the plain text and only then
// wrapped in colour; measuring it the other way round measures the
// invisible escape bytes, and every count lands in a different column -
// but only with colour on, which is why this runs both ways.
func TestTerminalSectionHeadingsFillTheWidthExactly(t *testing.T) {
	for _, colour := range []bool{false, true} {
		name := "plain"
		if colour {
			name = "colour"
		}
		t.Run(name, func(t *testing.T) {
			out := renderTerminal(t, allLevels(), TerminalOptions{Colour: colour, Width: 72})
			if colour && !strings.Contains(out, "\x1b[") {
				t.Fatal("expected ANSI escapes with colour on - without them this case proves nothing")
			}

			headings := 0
			for _, line := range visibleLines(out) {
				if !strings.HasPrefix(line, "CRITICAL ") && !strings.HasPrefix(line, "HIGH ") &&
					!strings.HasPrefix(line, "LOW ") && !strings.HasPrefix(line, "INFO ") {
					continue
				}
				headings++
				if got := utf8.RuneCountInString(line); got != 72 {
					t.Errorf("section heading must be exactly the width: got %d for %q", got, line)
				}
				if !strings.HasSuffix(line, "  1") {
					t.Errorf("the count must be right-aligned at the end of the heading: %q", line)
				}
			}
			if headings != 4 {
				t.Fatalf("expected 4 section headings, found %d", headings)
			}
		})
	}
}

// TestTerminalNothingExceedsTheWidth covers the rule that makes the
// format usable at all: a rule that wraps is not a rule, and an
// attribute path that runs off the edge is the thing this tool exists
// to make readable.
func TestTerminalNothingExceedsTheWidth(t *testing.T) {
	long := "azurerm_subnet.this[\"a-very-long-for-each-key-that-will-not-fit-on-one-line-at-all\"]"
	r := assess.Report{
		Findings: []assess.Finding{{
			Address: long, Kind: assess.KindReplace,
			Level: assess.Critical, LevelName: "critical",
			Reason:       "an attribute changed that cannot be updated in place",
			ReplacePaths: []string{"delegation[0].service_delegation[0].name", "network_security_group_association[0].subnet_id"},
			DataLoss:     true,
			Annotations: []assess.Annotation{{
				Code:   assess.AnnUnverifiable,
				Detail: "these values are not known until apply, so no claim about them can be checked in review",
				Paths:  []string{"private_endpoint_network_policies_enabled", "supernaturallylongattributepathwithnospacesinitatallwhatsoever"},
			}},
		}},
		CountsByName: map[string]int{"critical": 1},
	}

	for _, width := range []int{60, 72, 100} {
		for _, ascii := range []bool{false, true} {
			out := renderTerminal(t, r, TerminalOptions{Width: width, ASCII: ascii, Colour: true})
			for _, line := range visibleLines(out) {
				if got := utf8.RuneCountInString(line); got > width {
					t.Errorf("width %d ascii %v: line is %d columns wide: %q", width, ascii, got, line)
				}
			}
		}
	}
}

// TestTerminalWrapsToTheHangingIndent checks where a wrapped line
// restarts. Wrapping back to column zero would run the rest of a
// sentence under the tree column and make the tree unreadable, which is
// the failure this layout has to avoid to be worth having.
func TestTerminalWrapsToTheHangingIndent(t *testing.T) {
	r := assess.Report{
		Findings: []assess.Finding{{
			Address: "example.res", Kind: assess.KindCreate,
			Level: assess.Info, LevelName: "info",
			Annotations: []assess.Annotation{
				{
					Code:   assess.AnnUnverifiable,
					Detail: "these values are not known until apply, so no claim about them can be checked in review",
					Paths:  []string{"id"},
				},
				{
					Code:   assess.AnnSensitive,
					Detail: "these values are sensitive and are redacted in all output",
					Paths:  []string{"administrator_password"},
				},
			},
		}},
		CountsByName: map[string]int{"info": 1},
	}
	out := renderTerminal(t, r, TerminalOptions{Width: 60})

	var sawCarry, sawHang bool
	for _, line := range visibleLines(out) {
		if strings.HasPrefix(line, "  │ ") {
			// A continuation under a branch keeps the tree column and
			// hangs under the first word of the line it continues.
			sawHang = true
		}
		if strings.HasPrefix(line, "  │   ") {
			sawCarry = true
		}
		// Nothing in a finding may start at column 0.
		if line != "" && !strings.HasPrefix(line, " ") &&
			!strings.HasPrefix(line, "terraken") && !strings.HasPrefix(line, "INFO") &&
			!strings.ContainsAny(line, "━") && !strings.HasPrefix(line, "1 info") {
			t.Errorf("a finding's lines must not start at column 0: %q", line)
		}
	}
	if !sawHang {
		t.Errorf("expected a wrapped line to hang under the tree column, got:\n%s", out)
	}
	if !sawCarry {
		t.Errorf("expected an annotation path indented past the tree column, got:\n%s", out)
	}
}

// TestTerminalUsesTreeConnectors checks the last detail line closes the
// tree and the ones above it do not.
func TestTerminalUsesTreeConnectors(t *testing.T) {
	out := renderTerminal(t, sample(), TerminalOptions{Width: testWidth})
	lines := visibleLines(out)

	var branches, lasts int
	for _, line := range lines {
		if strings.HasPrefix(line, "  ├ ") {
			branches++
		}
		if strings.HasPrefix(line, "  └ ") {
			lasts++
		}
	}
	// The critical finding has three details, the info finding one.
	if branches != 2 {
		t.Errorf("expected 2 branch connectors, got %d:\n%s", branches, out)
	}
	if lasts != 2 {
		t.Errorf("expected one closing connector per finding, got %d:\n%s", lasts, out)
	}
}

// TestTerminalASCIIUsesNoGlyphOutsideASCII is what --plain promises.
// Box drawing is the first thing to break in a log viewer, a Windows
// console with the wrong code page, or a paste into a ticket.
func TestTerminalASCIIUsesNoGlyphOutsideASCII(t *testing.T) {
	reports := map[string]assess.Report{
		"all levels": allLevels(),
		"moved":      movedReport(),
		"filtered":   filteredReport(),
		"reordered":  reorderedReport(3),
	}
	for name, r := range reports {
		t.Run(name, func(t *testing.T) {
			out := renderTerminal(t, r, TerminalOptions{ASCII: true, Width: testWidth})
			for i, line := range strings.Split(out, "\n") {
				for _, ch := range line {
					if ch > 127 {
						t.Errorf("line %d has a non-ASCII rune %q: %q", i+1, ch, line)
						break
					}
				}
			}
			if strings.Contains(out, "\x1b[") {
				t.Errorf("ASCII output carried ANSI escapes it was not given:\n%s", out)
			}
		})
	}
}

// TestTerminalASCIIStillDrawsTheTree - stripping the glyphs must not
// strip the structure. The layout has to carry the report on its own,
// which is the whole test of whether it is doing any work.
func TestTerminalASCIIStillDrawsTheTree(t *testing.T) {
	out := renderTerminal(t, sample(), TerminalOptions{ASCII: true, Width: testWidth})
	for _, want := range []string{"|- ", "`- ", "====", "----", "CRITICAL", "INFO"} {
		if !strings.Contains(out, want) {
			t.Errorf("ASCII output missing %q:\n%s", want, out)
		}
	}
}

// TestTerminalWidthIsClamped stops a 400-column terminal producing a
// line of prose nobody can track back to its own left edge, and a
// 20-column one producing a heading with no room for a rule.
func TestTerminalWidthIsClamped(t *testing.T) {
	for _, tc := range []struct{ given, want int }{
		{given: 10, want: minWidth},
		{given: 400, want: maxWidth},
		{given: 72, want: 72},
	} {
		out := renderTerminal(t, allLevels(), TerminalOptions{Width: tc.given})
		for _, line := range visibleLines(out) {
			if strings.HasPrefix(line, "CRITICAL ") {
				if got := utf8.RuneCountInString(line); got != tc.want {
					t.Errorf("width %d should clamp to %d, heading came out %d wide", tc.given, tc.want, got)
				}
			}
		}
	}
}

// TestTerminalEmptyReportSaysNothingElse keeps the good-news case a
// single line. A masthead, two rules and a heading to say "nothing
// happened" would be worse than the sentence.
func TestTerminalEmptyReportSaysNothingElse(t *testing.T) {
	out := renderTerminal(t, assess.Report{CountsByName: map[string]int{}}, TerminalOptions{Width: testWidth})
	if out != "No changes. This plan does nothing.\n" {
		t.Errorf("a clean plan must be one line, got:\n%s", out)
	}
}

// reorderCaveat and reorderSummary mirror what assess attaches to a
// same-elements-reordered annotation: a short label for the finding, and
// the standing caveat that belongs to the rule rather than to any one
// finding. The render package is tested on what it does with those two,
// not on assess's exact wording, which cmd/terraken covers end to end.
const (
	reorderSummary = "same elements, different order"
	reorderCaveat  = "Order is significant for some attributes, such as a container command " +
		"or an ordered rule list, so whether a reordering matters is yours to judge."
)

// reorderedReport is n findings that all carry the same annotation - the
// estate-sized case the shared caveat exists for. At n=30 the old layout
// printed the caveat thirty times, ninety lines of identical prose, and
// the reader stopped reading.
func reorderedReport(n int) assess.Report {
	var findings []assess.Finding
	for i := 0; i < n; i++ {
		findings = append(findings, assess.Finding{
			Address:   "aws_security_group.web" + strconv.Itoa(i),
			Kind:      assess.KindUpdate,
			Level:     assess.Low,
			LevelName: "low",
			Annotations: []assess.Annotation{{
				Code:    assess.AnnReordered,
				Summary: reorderSummary,
				Note:    reorderCaveat,
				Detail:  "these lists hold the same elements in a different order. " + reorderCaveat,
				Paths:   []string{"ingress[0].cidr_blocks"},
			}},
		})
	}
	return assess.Report{
		TerraformVersion: "1.9.8",
		Findings:         findings,
		CountsByName:     map[string]int{"low": n},
	}
}

// TestTerminalStatesASharedCaveatOnceForTheWholeReport is the fix this
// layout exists for. The caveat is the same sentence on every finding
// that carries the annotation, so repeating it is repetition, not
// information - and on a real estate with thirty reshuffled sets it was
// ninety lines of it.
func TestTerminalStatesASharedCaveatOnceForTheWholeReport(t *testing.T) {
	out := renderTerminal(t, reorderedReport(30), TerminalOptions{Width: testWidth})

	if got := strings.Count(out, "Order is significant"); got != 1 {
		t.Errorf("the caveat must be stated once for the whole report, found it %d times:\n%s", got, out)
	}
	// Each finding still says what was found about it. Hoisting the
	// caveat must not take the fact with it.
	if got := strings.Count(out, reorderSummary); got != 30 {
		t.Errorf("every finding must still carry the label, found %d of 30:\n%s", got, out)
	}
	if got := strings.Count(out, "ingress[0].cidr_blocks"); got != 30 {
		t.Errorf("every finding must still name its own attribute paths, found %d of 30", got)
	}
}

// TestTerminalOmitsTheCaveatWhenNothingCarriesIt keeps the footer
// honest. A standing note about an annotation nobody's plan produced is
// furniture.
func TestTerminalOmitsTheCaveatWhenNothingCarriesIt(t *testing.T) {
	out := renderTerminal(t, sample(), TerminalOptions{Width: testWidth})
	if strings.Contains(out, "Order is significant") {
		t.Errorf("a report with no reordering must not carry the reordering caveat:\n%s", out)
	}
}

// TestTerminalCaveatSitsBetweenTheClosingRuleAndTheCounts pins where the
// note goes. Above the closing rule it would read as another finding;
// below the counts it would be past where anyone stops reading.
func TestTerminalCaveatSitsBetweenTheClosingRuleAndTheCounts(t *testing.T) {
	lines := visibleLines(renderTerminal(t, reorderedReport(3), TerminalOptions{Width: testWidth}))

	var lastRule, caveat, counts int
	for i, line := range lines {
		switch {
		case strings.HasPrefix(line, "━━"):
			lastRule = i
		case strings.HasPrefix(line, "Order is significant"):
			caveat = i
		case strings.HasPrefix(line, "3 low"):
			counts = i
		}
	}
	if caveat == 0 || counts == 0 {
		t.Fatalf("expected both a caveat line and a counts line, got:\n%s", strings.Join(lines, "\n"))
	}
	if !(lastRule < caveat && caveat < counts) {
		t.Errorf("expected rule (%d) then caveat (%d) then counts (%d):\n%s",
			lastRule, caveat, counts, strings.Join(lines, "\n"))
	}
	// A blank line between the note and the tally, so the footer reads as
	// two things rather than one run-on block.
	if lines[counts-1] != "" {
		t.Errorf("expected a blank line above the counts, got %q", lines[counts-1])
	}
}

// TestTerminalCaveatWrapsInsideTheWidth covers the one sentence in the
// report long enough to wrap at every width the terminal supports, and
// the one that must never strand a hyphen at column zero where it reads
// as a bullet.
func TestTerminalCaveatWrapsInsideTheWidth(t *testing.T) {
	for width := minWidth; width <= maxWidth; width++ {
		out := renderTerminal(t, reorderedReport(2), TerminalOptions{Width: width})
		for _, line := range visibleLines(out) {
			if got := utf8.RuneCountInString(line); got > width {
				t.Errorf("width %d: line is %d columns wide: %q", width, got, line)
			}
			if strings.HasPrefix(line, "- ") {
				t.Errorf("width %d: a wrapped line started with a stranded hyphen: %q", width, line)
			}
		}
	}
}

// TestTerminalMovedEvidencePrintsOncePerPair. The annotation is attached
// to both halves of the pair and both halves genuinely are affected, so
// both are told - but the suggested block and the attribute count are one
// pair's evidence, and printing the same block twice invites someone to
// paste it twice.
func TestTerminalMovedEvidencePrintsOncePerPair(t *testing.T) {
	pair := &assess.MovedEvidence{
		From: "azurerm_subnet.app", To: "azurerm_subnet.application",
		Matched: 5, Compared: 5,
	}
	r := assess.Report{
		Findings: []assess.Finding{
			{
				Address: "azurerm_subnet.app", Kind: assess.KindDelete,
				Level: assess.High, LevelName: "high",
				Annotations: []assess.Annotation{{Code: assess.AnnMissedMoved, Moved: pair}},
			},
			{
				Address: "azurerm_subnet.application", Kind: assess.KindCreate,
				Level: assess.Info, LevelName: "info",
				Annotations: []assess.Annotation{{Code: assess.AnnMissedMoved, Moved: pair}},
			},
		},
		CountsByName: map[string]int{"high": 1, "info": 1},
	}
	out := renderTerminal(t, r, TerminalOptions{Width: testWidth})

	if got := strings.Count(out, "possible missed moved block"); got != 2 {
		t.Errorf("both halves of the pair are affected and both must say so, found %d:\n%s", got, out)
	}
	for _, once := range []string{"moved { from =", "5 of 5 attributes match", "verify the pairing"} {
		if got := strings.Count(out, once); got != 1 {
			t.Errorf("the pair's evidence must appear once, found %q %d times:\n%s", once, got, out)
		}
	}
	// The second half still names the resource it is paired with, and
	// says where the evidence is.
	if !strings.Contains(out, "paired with azurerm_subnet.app, shown above") {
		t.Errorf("the second half must name the other resource and point at the evidence:\n%s", out)
	}
}

// TestTerminalMovedEvidenceGoesToWhicheverHalfIsShownFirst. A filter can
// hide one half of a pair, and the half that survives must get the
// evidence rather than a pointer to a stanza that was never printed.
func TestTerminalMovedEvidenceGoesToWhicheverHalfIsShownFirst(t *testing.T) {
	pair := &assess.MovedEvidence{
		From: "azurerm_subnet.app", To: "azurerm_subnet.application",
		Matched: 5, Compared: 5,
	}
	r := assess.Report{
		Findings: []assess.Finding{{
			Address: "azurerm_subnet.application", Kind: assess.KindCreate,
			Level: assess.Info, LevelName: "info",
			Annotations: []assess.Annotation{{Code: assess.AnnMissedMoved, Moved: pair}},
		}},
		CountsByName: map[string]int{"high": 1, "info": 1},
		Hidden:       1,
		HiddenBelow:  "info",
	}
	out := renderTerminal(t, r, TerminalOptions{Width: testWidth})

	if !strings.Contains(out, "moved { from =") {
		t.Errorf("the only half on show must carry the evidence, got:\n%s", out)
	}
	if strings.Contains(out, "shown above") {
		t.Errorf("nothing was shown above, so nothing may point there:\n%s", out)
	}
}

// TestTerminalMovedEvidenceNamesTheOtherHalf checks the annotation is
// set as a label with facts under it rather than as the paragraph that
// carries the same facts four times as long - and that it points at the
// resource the reader is not already looking at.
func TestTerminalMovedEvidenceNamesTheOtherHalf(t *testing.T) {
	out := renderTerminal(t, movedReport(), TerminalOptions{Width: testWidth})
	for _, want := range []string{
		"possible missed moved block",
		"4 of 4 attributes match azurerm_postgresql_flexible_server.primary",
		"moved { from = azurerm_postgresql_flexible_server.main",
		"verify the pairing",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("terminal output missing %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, assess.AnnMissedMoved) {
		t.Errorf("the raw annotation code is an identifier, not a heading:\n%s", out)
	}
}

func TestWrapBreaksAWordTooLongForTheLine(t *testing.T) {
	got := wrap(strings.Repeat("x", 25), 10, 10)
	want := []string{strings.Repeat("x", 10), strings.Repeat("x", 10), strings.Repeat("x", 5)}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Errorf("wrap = %q, want %q", got, want)
	}
}

// TestWrapLeavesTextThatFitsAlone is what keeps the deliberate double
// space inside "forces replacement   zone" and inside a moved block.
func TestWrapLeavesTextThatFitsAlone(t *testing.T) {
	in := "forces replacement   zone"
	got := wrap(in, 40, 40)
	if len(got) != 1 || got[0] != in {
		t.Errorf("wrap reformatted text that already fits: %q", got)
	}
}

func TestWrapUsesTheNarrowerWidthAfterTheFirstLine(t *testing.T) {
	got := wrap("aaa bbb ccc ddd", 11, 7)
	want := []string{"aaa bbb ccc", "ddd"}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Errorf("wrap = %q, want %q", got, want)
	}
}

// TestDetectWidthFallsBackToColumnsThenEighty covers the two answers
// available when the stream cannot be asked - which is every stream in
// a test, and every stream in a pipeline.
func TestDetectWidthFallsBackToColumnsThenEighty(t *testing.T) {
	var b bytes.Buffer

	t.Setenv("COLUMNS", "")
	if got := detectWidth(&b); got != defaultWidth {
		t.Errorf("with no COLUMNS, detectWidth = %d, want %d", got, defaultWidth)
	}

	t.Setenv("COLUMNS", "93")
	if got := detectWidth(&b); got != 93 {
		t.Errorf("COLUMNS should be used when the stream cannot answer, got %d", got)
	}

	t.Setenv("COLUMNS", "not a number")
	if got := detectWidth(&b); got != defaultWidth {
		t.Errorf("a junk COLUMNS must fall through to %d, got %d", defaultWidth, got)
	}
}

// TestTerminalMovedEvidenceSaysWhenItCrossedAModule. Moving a resource
// into or out of a module is the commonest reason anyone writes a moved
// block, and it is also weaker evidence than a same-module match, so the
// reader has to be told which one this is.
func TestTerminalMovedEvidenceSaysWhenItCrossedAModule(t *testing.T) {
	r := assess.Report{
		Findings: []assess.Finding{{
			Address: "azurerm_subnet.app", Kind: assess.KindDelete,
			Level: assess.High, LevelName: "high",
			Annotations: []assess.Annotation{{
				Code:   assess.AnnMissedMoved,
				Detail: "a paragraph the terminal does not use",
				Moved: &assess.MovedEvidence{
					From: "azurerm_subnet.app", To: "module.network.azurerm_subnet.app",
					Matched: 5, Compared: 5,
					CrossModule: true, FromModule: "", ToModule: "module.network",
				},
			}},
		}},
		CountsByName: map[string]int{"high": 1},
	}
	out := renderTerminal(t, r, TerminalOptions{Width: testWidth})
	if !strings.Contains(out, "crosses a module boundary, the root module to module.network") {
		t.Errorf("expected the module boundary named, and the root module named as such:\n%s", out)
	}
}

// TestTerminalHiddenNoteDropsToItsOwnLineWhenItCannotFit. The note is
// set beside the counts when there is room and under them when there is
// not, but it is never dropped - showing fewer findings than were found,
// without saying so, is how the one that mattered gets missed.
func TestTerminalHiddenNoteDropsToItsOwnLineWhenItCannotFit(t *testing.T) {
	r := assess.Report{
		Findings:     []assess.Finding{},
		CountsByName: map[string]int{"critical": 12, "high": 34, "low": 567, "info": 890},
		Hidden:       1503,
		HiddenBelow:  "critical",
	}
	out := renderTerminal(t, r, TerminalOptions{Width: minWidth})
	lines := visibleLines(out)
	last := lines[len(lines)-1]
	if last != "1503 below critical not shown" {
		t.Errorf("expected the note on its own line when it cannot fit beside the counts, got %q\n---\n%s", last, out)
	}
	for _, line := range lines {
		if utf8.RuneCountInString(line) > minWidth {
			t.Errorf("line is wider than the width: %q", line)
		}
	}
}

// TestTerminalFilteredToNothingDoesNotPrintTwoRules. A closing rule
// pairs with a body; two of them with nothing between reads as a bug.
func TestTerminalFilteredToNothingDoesNotPrintTwoRules(t *testing.T) {
	out := renderTerminal(t, assess.Report{
		Findings:     []assess.Finding{},
		CountsByName: map[string]int{"low": 3},
		Hidden:       3,
		HiddenBelow:  "critical",
	}, TerminalOptions{Width: testWidth})

	rules := 0
	for _, line := range visibleLines(out) {
		if strings.HasPrefix(line, "━━") {
			rules++
		}
	}
	if rules != 1 {
		t.Errorf("expected one rule when there is no body, got %d:\n%s", rules, out)
	}
	if !strings.Contains(out, "3 below critical not shown") {
		t.Errorf("terminal must say what is hidden:\n%s", out)
	}
}

// loadFixture assesses a real plan file. The other tests here build reports in
// code, which is right for exercising one renderer rule at a time - but the
// summary is about the shape of a WHOLE plan, and a hand-built report would be
// testing my idea of a large estate rather than one terraform produced.
func loadFixture(t *testing.T, name string) assess.Report {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("..", "..", "testdata", name))
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	var p tfjson.Plan
	if err := json.Unmarshal(b, &p); err != nil {
		t.Fatalf("parse fixture: %v", err)
	}
	return assess.Assess(&p)
}

func TestTerminalShowsTheShapeOnlyWhenItEarnsIt(t *testing.T) {
	big := loadFixture(t, "large-estate.json")
	var buf bytes.Buffer
	if err := Terminal(&buf, big, TerminalOptions{Width: 78}); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if !strings.Contains(out, "module.billing") {
		t.Fatalf("a 20-finding plan should be summarised:\n%s", out[:400])
	}
	// The summary sits ABOVE the first section heading. Below it, it is a
	// footnote rather than an orientation.
	if strings.Index(out, "module.billing 10") > strings.Index(out, "HIGH") {
		t.Fatal("the summary appears after the findings begin")
	}

	small := loadFixture(t, "critical.json")
	buf.Reset()
	if err := Terminal(&buf, small, TerminalOptions{Width: 78}); err != nil {
		t.Fatal(err)
	}
	// A one-finding plan gets no summary: the finding IS the summary.
	if strings.Contains(buf.String(), "changes are") {
		t.Fatalf("a 1-finding plan should not be summarised:\n%s", buf.String())
	}
}

func TestTheSummarySaysWhenFindingsAreHidden(t *testing.T) {
	// The footer already reports the filter, but a reader who takes the top
	// summary as the whole picture and then sees a short list has been misled
	// by the gap between them.
	r := loadFixture(t, "large-estate.json").AtLeast(assess.High)
	var buf bytes.Buffer
	if err := Terminal(&buf, r, TerminalOptions{Width: 78}); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if !strings.Contains(out, "counts cover the whole plan") {
		t.Fatalf("the summary does not state its scope under a filter:\n%s", out[:500])
	}
	if !strings.Contains(out, "20 findings") {
		t.Fatalf("the masthead should still count the whole plan:\n%s", out[:200])
	}
}
