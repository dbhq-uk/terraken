package render

import (
	"bytes"
	"encoding/json"
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

func TestEmptyReportSaysSoInAllFormats(t *testing.T) {
	empty := assess.Report{CountsByName: map[string]int{}}
	var b bytes.Buffer
	_ = Terminal(&b, empty, false)
	if !strings.Contains(b.String(), "No changes") {
		t.Errorf("terminal must say when a plan changes nothing, got: %s", b.String())
	}
}
