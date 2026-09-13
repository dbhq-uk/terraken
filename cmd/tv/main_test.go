package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunCleanPlanExitsZero(t *testing.T) {
	var out, errOut bytes.Buffer
	code := run([]string{"../../testdata/minimal.json"}, strings.NewReader(""), &out, &errOut)
	if code != 0 {
		t.Errorf("exit code = %d, want 0. stderr: %s", code, errOut.String())
	}
	if !strings.Contains(out.String(), "No changes") {
		t.Errorf("expected the no-changes message, got: %s", out.String())
	}
}

func TestRunMissingArgExitsTwo(t *testing.T) {
	var out, errOut bytes.Buffer
	if code := run(nil, strings.NewReader(""), &out, &errOut); code != 2 {
		t.Errorf("exit code = %d, want 2", code)
	}
	if !strings.Contains(errOut.String(), "usage") {
		t.Errorf("expected usage on stderr, got: %s", errOut.String())
	}
}

func TestRunBadFileExitsTwo(t *testing.T) {
	var out, errOut bytes.Buffer
	if code := run([]string{"../../testdata/malformed.json"}, strings.NewReader(""), &out, &errOut); code != 2 {
		t.Errorf("exit code = %d, want 2", code)
	}
}

func TestRunFailOnTriggersExitOne(t *testing.T) {
	var out, errOut bytes.Buffer
	code := run([]string{"--fail-on", "critical", "../../testdata/critical.json"}, strings.NewReader(""), &out, &errOut)
	if code != 1 {
		t.Errorf("exit code = %d, want 1. stdout: %s", code, out.String())
	}
}

func TestRunFailOnNotReachedExitsZero(t *testing.T) {
	var out, errOut bytes.Buffer
	code := run([]string{"--fail-on", "critical", "../../testdata/lowrisk.json"}, strings.NewReader(""), &out, &errOut)
	if code != 0 {
		t.Errorf("exit code = %d, want 0. stdout: %s", code, out.String())
	}
}

func TestRunWithoutFailOnAlwaysExitsZero(t *testing.T) {
	var out, errOut bytes.Buffer
	if code := run([]string{"../../testdata/critical.json"}, strings.NewReader(""), &out, &errOut); code != 0 {
		t.Errorf("exit code = %d, want 0 - --fail-on is off by default", code)
	}
}

func TestRunRejectsMediumFailOn(t *testing.T) {
	var out, errOut bytes.Buffer
	if code := run([]string{"--fail-on", "medium", "../../testdata/minimal.json"}, strings.NewReader(""), &out, &errOut); code != 2 {
		t.Errorf("exit code = %d, want 2", code)
	}
}

func TestRunRejectsUnknownFormat(t *testing.T) {
	var out, errOut bytes.Buffer
	code := run([]string{"--format", "bogus", "../../testdata/minimal.json"}, strings.NewReader(""), &out, &errOut)
	if code != 2 {
		t.Errorf("exit code = %d, want 2", code)
	}
	for _, want := range []string{"terminal", "md", "json", "html"} {
		if !strings.Contains(errOut.String(), want) {
			t.Errorf("expected stderr to mention the %s format, got: %s", want, errOut.String())
		}
	}
}

func TestRunJSONFormatProducesParsableOutput(t *testing.T) {
	var out, errOut bytes.Buffer
	code := run([]string{"--format", "json", "../../testdata/critical.json"}, strings.NewReader(""), &out, &errOut)
	if code != 0 {
		t.Errorf("exit code = %d, want 0. stderr: %s", code, errOut.String())
	}
	var report map[string]interface{}
	if err := json.Unmarshal(out.Bytes(), &report); err != nil {
		t.Fatalf("output is not valid JSON: %v\noutput: %s", err, out.String())
	}
	if _, ok := report["findings"]; !ok {
		t.Errorf("expected a findings key in the JSON output, got: %s", out.String())
	}
}

func TestRunVersionPrintsAndExitsZero(t *testing.T) {
	var out, errOut bytes.Buffer
	if code := run([]string{"--version"}, strings.NewReader(""), &out, &errOut); code != 0 {
		t.Errorf("exit code = %d, want 0. stderr: %s", code, errOut.String())
	}
	if !strings.Contains(out.String(), version) {
		t.Errorf("expected the version on stdout, got: %q", out.String())
	}
	if !strings.Contains(out.String(), "terraverdict") {
		t.Errorf("expected the project name on stdout, got: %q", out.String())
	}
}

func TestRunVersionNeedsNoPlanFile(t *testing.T) {
	// A version check must not require an argument it has nothing to do
	// with, so this runs with no file at all and must not print usage.
	var out, errOut bytes.Buffer
	if code := run([]string{"--version"}, strings.NewReader(""), &out, &errOut); code != 0 {
		t.Errorf("exit code = %d, want 0", code)
	}
	if strings.Contains(errOut.String(), "usage") {
		t.Errorf("--version must not print usage, got: %s", errOut.String())
	}
}

// TestRunFlagsAfterFileExplainsItself covers the trap Go's flag package
// sets for anyone whose mental model comes from Terraform, where
// "terraform apply tfplan -auto-approve" works. Parsing stops at the
// first positional, so the flags are simply ignored and the command
// exits 2 with nothing but usage. It must say what actually went wrong.
func TestRunFlagsAfterFileExplainsItself(t *testing.T) {
	var out, errOut bytes.Buffer
	code := run([]string{"../../testdata/critical.json", "--format", "md"}, strings.NewReader(""), &out, &errOut)
	if code != 2 {
		t.Errorf("exit code = %d, want 2", code)
	}
	msg := errOut.String()
	if !strings.Contains(msg, "flags must come before the file") {
		t.Errorf("expected an explanation, got: %s", msg)
	}
	if !strings.Contains(msg, "try tv --format md ../../testdata/critical.json") {
		t.Errorf("expected the corrected command, got: %s", msg)
	}
}

// TestRunTwoFilesJustShowsUsage checks the explanation is not fired at
// the wrong thing. Two plan files is a different mistake, and telling
// someone their flag order is wrong when they passed no flag would be
// worse than saying nothing.
func TestRunTwoFilesJustShowsUsage(t *testing.T) {
	var out, errOut bytes.Buffer
	code := run([]string{"../../testdata/critical.json", "../../testdata/lowrisk.json"}, strings.NewReader(""), &out, &errOut)
	if code != 2 {
		t.Errorf("exit code = %d, want 2", code)
	}
	if strings.Contains(errOut.String(), "flags must come before the file") {
		t.Errorf("must not blame flag order when no flag was passed, got: %s", errOut.String())
	}
	if !strings.Contains(errOut.String(), "usage") {
		t.Errorf("expected usage on stderr, got: %s", errOut.String())
	}
}

// TestRunReadsStdinWhenGivenDash covers "terraform show -json tfplan |
// tv -". The README is right that plan JSON is a secret, so the tool has
// to support the workflow that never writes one to disk.
func TestRunReadsStdinWhenGivenDash(t *testing.T) {
	b, err := os.ReadFile("../../testdata/critical.json")
	if err != nil {
		t.Fatalf("committed fixture is missing: %v", err)
	}

	var out, errOut bytes.Buffer
	code := run([]string{"-"}, bytes.NewReader(b), &out, &errOut)
	if code != 0 {
		t.Errorf("exit code = %d, want 0. stderr: %s", code, errOut.String())
	}
	if !strings.Contains(out.String(), "azurerm_postgresql_flexible_server.main") {
		t.Errorf("expected the report on stdout, got: %s", out.String())
	}
}

func TestRunStdinStillHonoursFailOn(t *testing.T) {
	b, err := os.ReadFile("../../testdata/critical.json")
	if err != nil {
		t.Fatalf("committed fixture is missing: %v", err)
	}
	var out, errOut bytes.Buffer
	code := run([]string{"--fail-on", "critical", "-"}, bytes.NewReader(b), &out, &errOut)
	if code != 1 {
		t.Errorf("exit code = %d, want 1. stdout: %s", code, out.String())
	}
}

// TestRunEmptyStdinSaysSo checks the message for the commonest piped
// failure. "unexpected end of JSON input" would point at the wrong
// thing when what actually happened is that terraform produced nothing.
func TestRunEmptyStdinSaysSo(t *testing.T) {
	var out, errOut bytes.Buffer
	if code := run([]string{"-"}, strings.NewReader(""), &out, &errOut); code != 2 {
		t.Errorf("exit code = %d, want 2", code)
	}
	if !strings.Contains(errOut.String(), "standard input is empty") {
		t.Errorf("expected an empty-input message naming standard input, got: %s", errOut.String())
	}
}

// TestRunCleanPlanJSONHasAnArrayNotNull covers the shape a consumer
// parses, not just its validity. A nil slice marshals as null, so
// "findings": null made jq '.findings[]' fail with "Cannot iterate over
// null" - on the one plan whose answer is good news.
func TestRunCleanPlanJSONHasAnArrayNotNull(t *testing.T) {
	var out, errOut bytes.Buffer
	code := run([]string{"--format", "json", "../../testdata/minimal.json"}, strings.NewReader(""), &out, &errOut)
	if code != 0 {
		t.Fatalf("exit code = %d, want 0. stderr: %s", code, errOut.String())
	}
	if strings.Contains(out.String(), `"findings": null`) {
		t.Errorf("findings must marshal as [], never null:\n%s", out.String())
	}

	var report struct {
		Findings []map[string]interface{} `json:"findings"`
	}
	if err := json.Unmarshal(out.Bytes(), &report); err != nil {
		t.Fatalf("output is not valid JSON: %v", err)
	}
	if report.Findings == nil {
		t.Error("findings decoded as nil - it must be an empty array")
	}
}

func TestRunMinLevelHidesLowerFindings(t *testing.T) {
	var all, errOut bytes.Buffer
	if code := run([]string{"../../testdata/real-plan.json"}, strings.NewReader(""), &all, &errOut); code != 0 {
		t.Fatalf("exit code = %d, want 0. stderr: %s", code, errOut.String())
	}

	var filtered bytes.Buffer
	errOut.Reset()
	if code := run([]string{"--min-level", "high", "../../testdata/real-plan.json"}, strings.NewReader(""), &filtered, &errOut); code != 0 {
		t.Fatalf("exit code = %d, want 0. stderr: %s", code, errOut.String())
	}

	if filtered.Len() >= all.Len() {
		t.Error("--min-level high must show less than the unfiltered report")
	}
	if !strings.Contains(filtered.String(), "below high not shown") {
		t.Errorf("the summary must say findings were hidden, got: %s", filtered.String())
	}
}

// TestRunMinLevelDoesNotChangeTheExitCode is the one that matters. A
// volume control must not become a way to turn a gate off: --fail-on is
// measured against everything found, not against what was displayed.
func TestRunMinLevelDoesNotChangeTheExitCode(t *testing.T) {
	var out, errOut bytes.Buffer
	code := run([]string{"--min-level", "critical", "--fail-on", "high", "../../testdata/real-plan.json"},
		strings.NewReader(""), &out, &errOut)
	if code != 1 {
		t.Errorf("exit code = %d, want 1 - the high findings are hidden but still found. stdout: %s", code, out.String())
	}
}

func TestRunRejectsUnknownMinLevel(t *testing.T) {
	var out, errOut bytes.Buffer
	if code := run([]string{"--min-level", "medium", "../../testdata/minimal.json"}, strings.NewReader(""), &out, &errOut); code != 2 {
		t.Errorf("exit code = %d, want 2", code)
	}
}

func TestRunWithoutMinLevelShowsEverything(t *testing.T) {
	var out, errOut bytes.Buffer
	if code := run([]string{"../../testdata/real-plan.json"}, strings.NewReader(""), &out, &errOut); code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	if strings.Contains(out.String(), "not shown") {
		t.Errorf("nothing may be hidden by default, got: %s", out.String())
	}
}

// TestIsTTYHonoursNoColor uses /dev/null, which is a character device
// and so counts as a terminal, because os.Stdout under "go test" is a
// pipe and would answer no for the wrong reason - leaving the NO_COLOR
// assertion proving nothing.
func TestIsTTYHonoursNoColor(t *testing.T) {
	dev, err := os.OpenFile(os.DevNull, os.O_WRONLY, 0)
	if err != nil {
		t.Fatalf("cannot open %s: %v", os.DevNull, err)
	}
	defer dev.Close()

	// Present but empty means nothing, per no-color.org, so colour
	// stays on. This also proves the case below can fail.
	t.Setenv("NO_COLOR", "")
	if !isTTY(dev) {
		t.Fatal("a character device must count as a terminal - without this the NO_COLOR case proves nothing")
	}

	t.Setenv("NO_COLOR", "1")
	if isTTY(dev) {
		t.Error("NO_COLOR set and non-empty must switch colour off")
	}
}

func TestRunAcceptsTheAmericanNoColorSpelling(t *testing.T) {
	var out, errOut bytes.Buffer
	if code := run([]string{"--no-color", "../../testdata/critical.json"}, strings.NewReader(""), &out, &errOut); code != 0 {
		t.Errorf("exit code = %d, want 0. stderr: %s", code, errOut.String())
	}
	if strings.Contains(out.String(), "\x1b[") {
		t.Error("--no-color must produce no ANSI escape codes")
	}
}

// TestRunPlainIsASCIIOnly is what the flag is for. Box drawing is the
// first thing to break in a log viewer, a Windows console on the wrong
// code page, or a paste into a ticket - so --plain promises there is
// none, on any plan.
func TestRunPlainIsASCIIOnly(t *testing.T) {
	for _, fixture := range []string{"critical.json", "rename-no-moved.json", "real-plan.json"} {
		t.Run(fixture, func(t *testing.T) {
			var out, errOut bytes.Buffer
			if code := run([]string{"--plain", "../../testdata/" + fixture}, strings.NewReader(""), &out, &errOut); code != 0 {
				t.Fatalf("exit code = %d, want 0. stderr: %s", code, errOut.String())
			}
			for _, ch := range out.String() {
				if ch > 127 {
					t.Fatalf("--plain emitted the non-ASCII rune %q:\n%s", ch, out.String())
				}
			}
			if strings.Contains(out.String(), "\x1b[") {
				t.Errorf("--plain must produce no ANSI escape codes:\n%s", out.String())
			}
		})
	}
}

// TestRunPlainStillReportsEverything checks the flag only changes how
// the report is drawn, never what it says.
func TestRunPlainStillReportsEverything(t *testing.T) {
	var plain, normal, errOut bytes.Buffer
	if code := run([]string{"--plain", "../../testdata/rename-no-moved.json"}, strings.NewReader(""), &plain, &errOut); code != 0 {
		t.Fatalf("exit code = %d, want 0. stderr: %s", code, errOut.String())
	}
	if code := run([]string{"../../testdata/rename-no-moved.json"}, strings.NewReader(""), &normal, &errOut); code != 0 {
		t.Fatalf("exit code = %d, want 0. stderr: %s", code, errOut.String())
	}
	for _, want := range []string{
		"azurerm_subnet.app",
		"possible missed moved block",
		"5 of 5 attributes match",
		"2 findings",
	} {
		if !strings.Contains(plain.String(), want) {
			t.Errorf("--plain lost %q:\n%s", want, plain.String())
		}
	}
	if strings.Count(plain.String(), "\n") != strings.Count(normal.String(), "\n") {
		t.Errorf("--plain changed the line count, so it changed the layout, not just the glyphs")
	}
}

func TestRunHTMLFormatIsSelfContained(t *testing.T) {
	var out, errOut bytes.Buffer
	if code := run([]string{"--format", "html", "../../testdata/critical.json"}, strings.NewReader(""), &out, &errOut); code != 0 {
		t.Fatalf("exit code = %d, want 0. stderr: %s", code, errOut.String())
	}
	got := out.String()
	if !strings.HasPrefix(got, "<!doctype html>") {
		t.Errorf("expected an HTML document, got: %.60q", got)
	}
	if !strings.Contains(got, "azurerm_postgresql_flexible_server.main") {
		t.Errorf("expected the finding in the document:\n%s", got)
	}
	for _, banned := range []string{"<script", "<link", "src=", "href="} {
		if strings.Contains(got, banned) {
			t.Errorf("the document must pull in nothing external, found %q", banned)
		}
	}
}

// TestRunOutWritesTheFileAndSaysNothingElse is the contract --out has
// to keep to stay usable in a script: the report goes to the path, and
// stdout carries one line naming it.
func TestRunOutWritesTheFileAndSaysNothingElse(t *testing.T) {
	path := filepath.Join(t.TempDir(), "report.txt")
	var out, errOut bytes.Buffer
	if code := run([]string{"--out", path, "../../testdata/critical.json"}, strings.NewReader(""), &out, &errOut); code != 0 {
		t.Fatalf("exit code = %d, want 0. stderr: %s", code, errOut.String())
	}
	if out.String() != "wrote "+path+"\n" {
		t.Errorf("stdout must be exactly the confirmation line, got: %q", out.String())
	}

	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("--out did not write the file: %v", err)
	}
	if !strings.Contains(string(b), "azurerm_postgresql_flexible_server.main") {
		t.Errorf("the report did not reach the file:\n%s", b)
	}
}

// TestRunOutWorksForEveryFormat - --out is about where the report goes,
// not about which one it is.
func TestRunOutWorksForEveryFormat(t *testing.T) {
	for _, tc := range []struct{ format, want string }{
		{"terminal", "terraverdict"},
		{"md", "| Level | Change |"},
		{"json", `"findings"`},
		{"html", "<!doctype html>"},
	} {
		t.Run(tc.format, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "report")
			var out, errOut bytes.Buffer
			code := run([]string{"--format", tc.format, "--out", path, "../../testdata/critical.json"},
				strings.NewReader(""), &out, &errOut)
			if code != 0 {
				t.Fatalf("exit code = %d, want 0. stderr: %s", code, errOut.String())
			}
			if out.String() != "wrote "+path+"\n" {
				t.Errorf("stdout must be exactly the confirmation line, got: %q", out.String())
			}
			b, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("--out did not write the file: %v", err)
			}
			if !strings.Contains(string(b), tc.want) {
				t.Errorf("expected %q in the %s report:\n%s", tc.want, tc.format, b)
			}
		})
	}
}

// TestRunOutNeverColoursAFile. Colour is a question about the
// destination, not about the process: a report redirected into a file
// must come out plain even when it was launched from a colour-capable
// terminal.
func TestRunOutNeverColoursAFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "report.txt")
	var out, errOut bytes.Buffer
	if code := run([]string{"--out", path, "../../testdata/critical.json"}, strings.NewReader(""), &out, &errOut); code != 0 {
		t.Fatalf("exit code = %d, want 0. stderr: %s", code, errOut.String())
	}
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("--out did not write the file: %v", err)
	}
	if strings.Contains(string(b), "\x1b[") {
		t.Errorf("a report written to a file must carry no ANSI escapes:\n%s", b)
	}
}

func TestRunOutUnwritablePathExitsTwo(t *testing.T) {
	path := filepath.Join(t.TempDir(), "no-such-directory", "report.txt")
	var out, errOut bytes.Buffer
	if code := run([]string{"--out", path, "../../testdata/critical.json"}, strings.NewReader(""), &out, &errOut); code != 2 {
		t.Errorf("exit code = %d, want 2", code)
	}
	if !strings.Contains(errOut.String(), "error:") {
		t.Errorf("expected an error on stderr, got: %s", errOut.String())
	}
	if out.Len() != 0 {
		t.Errorf("nothing may be written to stdout when the file could not be opened, got: %q", out.String())
	}
}

// TestRunOutStillHonoursFailOn - where the report goes has nothing to
// do with the gate.
func TestRunOutStillHonoursFailOn(t *testing.T) {
	path := filepath.Join(t.TempDir(), "report.txt")
	var out, errOut bytes.Buffer
	code := run([]string{"--fail-on", "critical", "--out", path, "../../testdata/critical.json"},
		strings.NewReader(""), &out, &errOut)
	if code != 1 {
		t.Errorf("exit code = %d, want 1. stdout: %s", code, out.String())
	}
}
