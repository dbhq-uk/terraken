package render

import (
	"bytes"
	"strings"
	"testing"

	"github.com/dbhq-uk/terraverdict/internal/assess"
)

func renderHTML(t *testing.T, r assess.Report) string {
	t.Helper()
	var b bytes.Buffer
	if err := HTML(&b, r); err != nil {
		t.Fatalf("HTML returned error: %v", err)
	}
	return b.String()
}

// TestHTMLIsOneSelfContainedDocument is the format's whole promise. A
// review artefact that fetches a stylesheet, a font or a script is a
// review artefact that can be changed after it was sent, and one that
// does not render at all from a file:// URL on a laptop with no network.
func TestHTMLIsOneSelfContainedDocument(t *testing.T) {
	out := renderHTML(t, sample())

	if !strings.HasPrefix(out, "<!doctype html>\n") {
		t.Errorf("expected a doctype on the first line, got: %.40q", out)
	}
	for _, banned := range []string{"<script", "<link", "<img", "src=", "href=", "@import", "url("} {
		if strings.Contains(out, banned) {
			t.Errorf("the document must pull in nothing external, found %q", banned)
		}
	}
	if strings.Count(out, "<style>") != 1 || strings.Count(out, "</style>") != 1 {
		t.Errorf("expected exactly one inline stylesheet")
	}
	if !strings.Contains(out, "prefers-color-scheme: dark") {
		t.Error("the document must work in dark mode as well as light")
	}
	if strings.Contains(out, "\x1b[") {
		t.Error("HTML must never contain ANSI escape codes")
	}
}

func TestHTMLIsSemanticNotAWallOfDivs(t *testing.T) {
	out := renderHTML(t, sample())
	for _, want := range []string{"<main", "<header", "<h1>", "<section", "<h2", "<article", "<h3>", "<footer"} {
		if !strings.Contains(out, want) {
			t.Errorf("expected %q in the document:\n%s", want, out)
		}
	}
	if strings.Contains(out, "<div") {
		t.Error("there is a real element for every part of this report; use it")
	}
}

// TestHTMLEscapesEverythingThatCameFromThePlan is the security test.
//
// A resource address carries a for_each key chosen by whoever wrote the
// Terraform - on a fork pull request, that is not someone you trust -
// and this file gets opened by a reviewer. So every value from the plan
// is untrusted input in an HTML context. The </style> payload is the
// sharpest form of it: an unescaped one would close the stylesheet and
// turn the rest of the document into script.
func TestHTMLEscapesEverythingThatCameFromThePlan(t *testing.T) {
	evil := `aws_s3_bucket.x["</style><script>alert(1)</script>"]`
	r := assess.Report{
		TerraformVersion: evil,
		Findings: []assess.Finding{{
			Address: evil, Type: evil,
			Kind:  assess.KindReplace,
			Level: assess.Critical, LevelName: "critical",
			Reason:       evil,
			ReplacePaths: []string{evil},
			DataLoss:     true,
			Annotations: []assess.Annotation{
				{Code: assess.AnnUnverifiable, Detail: "something about " + evil, Paths: []string{evil}},
				{
					Code:   assess.AnnMissedMoved,
					Detail: "something about " + evil,
					Moved: &assess.MovedEvidence{
						From: evil, To: evil + "-b", Matched: 4, Compared: 4,
						CrossModule: true, FromModule: evil, ToModule: evil,
					},
				},
			},
		}},
		CountsByName: map[string]int{"critical": 1},
	}
	out := renderHTML(t, r)

	if !strings.Contains(out, "&lt;/style&gt;&lt;script&gt;alert(1)&lt;/script&gt;") {
		t.Errorf("expected the payload to appear escaped:\n%s", out)
	}
	if strings.Contains(out, "<script") {
		t.Errorf("a script tag from the plan reached the document:\n%s", out)
	}
	// One </style>, and it is the tool's own. Anything else means the
	// payload closed the stylesheet.
	if got := strings.Count(out, "</style>"); got != 1 {
		t.Errorf("expected exactly 1 </style>, got %d - the payload broke out", got)
	}
	// And nothing from the plan is inside the stylesheet in the first
	// place, which is what makes the line above hold by construction.
	style := out[strings.Index(out, "<style>"):strings.Index(out, "</style>")]
	if strings.Contains(style, "aws_s3_bucket") {
		t.Errorf("no value from a plan may be written inside the style block:\n%s", style)
	}
}

// TestHTMLTakesItsClassNamesFromTheEnumNotTheString. Report is an
// ordinary struct a caller fills in, so LevelName is a string this
// package did not choose - and a string this package did not choose has
// no business landing in a class attribute.
func TestHTMLTakesItsClassNamesFromTheEnumNotTheString(t *testing.T) {
	r := assess.Report{
		Findings: []assess.Finding{{
			Address: "example.res", Kind: assess.KindDelete,
			Level:     assess.Critical,
			LevelName: `x" onload="alert(1)`,
		}},
		CountsByName: map[string]int{"critical": 1},
	}
	out := renderHTML(t, r)
	if strings.Contains(out, "onload") {
		t.Errorf("LevelName reached the markup:\n%s", out)
	}
	if !strings.Contains(out, `class="level level-critical"`) {
		t.Errorf("expected the class derived from Level, got:\n%s", out)
	}
}

func TestHTMLGroupsBySeverityAndOmitsEmptySections(t *testing.T) {
	out := renderHTML(t, oneFindingReport())
	if !strings.Contains(out, "level-critical") {
		t.Errorf("expected a critical section:\n%s", out)
	}
	for _, absent := range []string{"level-high", "level-low", "level-info"} {
		if strings.Contains(out, "class=\"level "+absent+"\"") {
			t.Errorf("a level with no findings must not get a section, found %q", absent)
		}
	}
}

func TestHTMLEmptyReportSaysSo(t *testing.T) {
	out := renderHTML(t, assess.Report{CountsByName: map[string]int{}})
	if !strings.Contains(out, "No changes. This plan does nothing.") {
		t.Errorf("HTML must say when a plan changes nothing, got:\n%s", out)
	}
}

// TestHTMLFilteredToNothingIsNotAClearPlan is the dangerous case, the
// same one the terminal and markdown have. If a filter hides every
// finding, "This plan does nothing" is a flat lie about a plan that
// might destroy a database.
func TestHTMLFilteredToNothingIsNotAClearPlan(t *testing.T) {
	out := renderHTML(t, assess.Report{
		Findings:     []assess.Finding{},
		CountsByName: map[string]int{"low": 3},
		Hidden:       3,
		HiddenBelow:  "critical",
	})
	if strings.Contains(out, "No changes") {
		t.Errorf("a filtered-out report must never claim the plan does nothing:\n%s", out)
	}
	if !strings.Contains(out, "3 below critical not shown") {
		t.Errorf("HTML must say what is hidden:\n%s", out)
	}
	if !strings.Contains(out, "3 findings") {
		t.Errorf("HTML must count the whole plan, not the nothing it is showing:\n%s", out)
	}
}

func TestHTMLSummaryIsSingularForOneFinding(t *testing.T) {
	out := renderHTML(t, oneFindingReport())
	if !strings.Contains(out, "1 finding<") {
		t.Errorf("expected singular \"1 finding\", got:\n%s", out)
	}
	if strings.Contains(out, "1 findings") {
		t.Errorf("must never say \"1 findings\", got:\n%s", out)
	}
}

// TestHTMLCarriesTheMovedEvidence holds the README's promise that the
// detector always shows its working, in every format.
func TestHTMLCarriesTheMovedEvidence(t *testing.T) {
	out := renderHTML(t, movedReport())
	for _, want := range []string{
		"possible missed moved block",
		"4 of 4 attributes match",
		"azurerm_postgresql_flexible_server.primary",
		"moved {",
		"from = azurerm_postgresql_flexible_server.main",
		"to   = azurerm_postgresql_flexible_server.primary",
		"Verify the pairing",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("HTML output missing %q:\n%s", want, out)
		}
	}
}

// TestHTMLHasNoEmptyList guards valid markup on the commonest finding
// in any plan: a create with nothing else to say.
func TestHTMLHasNoEmptyList(t *testing.T) {
	out := renderHTML(t, assess.Report{
		Findings: []assess.Finding{{
			Address: "example.res", Kind: assess.KindCreate,
			Level: assess.Info, LevelName: "info",
		}},
		CountsByName: map[string]int{"info": 1},
	})
	if strings.Contains(out, "<ul class=\"notes\">\n</ul>") {
		t.Errorf("an empty <ul> is invalid HTML:\n%s", out)
	}
}
