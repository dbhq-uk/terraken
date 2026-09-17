package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
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
	if !strings.Contains(out.String(), "terraken") {
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
	if !strings.Contains(msg, "try terraken --format md ../../testdata/critical.json") {
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
// terraken -". The README is right that plan JSON is a secret, so the tool has
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
		{"terminal", "terraken"},
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
	// FORCE_COLOR set, which is the case this used to miss. It answers yes
	// before isTTY ever looks at the destination, so a file got escape codes
	// written into it and this test passed anyway - because nobody runs the
	// suite with the variable set. Set explicitly now, in both directions.
	t.Setenv("NO_COLOR", "")
	t.Setenv("FORCE_COLOR", "1")
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

// TestRunReorderCaveatIsStatedOnceNotOnEveryFinding runs the whole
// command over a plan with four reordered resources.
//
// Presentation is the only thing this tool adds over reading terraform
// plan directly, so a caveat repeated verbatim under every finding is not
// a cosmetic problem: it is the tool committing the sin it diagnoses. The
// fact belongs to each finding, the caveat belongs to the report.
func TestRunReorderCaveatIsStatedOnceNotOnEveryFinding(t *testing.T) {
	for _, args := range [][]string{
		{"../../testdata/reordered.json"},
		{"--plain", "../../testdata/reordered.json"},
	} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			var out, errOut bytes.Buffer
			if code := run(args, strings.NewReader(""), &out, &errOut); code != 0 {
				t.Fatalf("exit code = %d, want 0. stderr: %s", code, errOut.String())
			}
			got := out.String()

			if n := strings.Count(got, "Order is significant"); n != 1 {
				t.Errorf("the caveat must appear once, found it %d times:\n%s", n, got)
			}
			if n := strings.Count(got, "same elements, different order"); n != 4 {
				t.Errorf("all four findings must still say what was found, got %d:\n%s", n, got)
			}
			for _, path := range []string{"vpc_security_group_ids", "command", "ingress[0].cidr_blocks", "service_endpoints"} {
				if !strings.Contains(got, path) {
					t.Errorf("the report lost the attribute path %q:\n%s", path, got)
				}
			}
		})
	}
}

// TestRunMachineFormatsKeepTheAnnotationSelfContained is the other half of
// that change, and the reason the caveat was not simply deleted.
//
// A terminal report is read top to bottom, so a footer note is in view. A
// markdown row gets quoted into a review comment, an HTML card gets
// screenshotted, and a JSON annotation gets read on its own by something
// with no footer at all. Each of those has to stand up alone, so Detail
// stays complete in all three.
func TestRunMachineFormatsKeepTheAnnotationSelfContained(t *testing.T) {
	for _, format := range []string{"md", "json", "html"} {
		t.Run(format, func(t *testing.T) {
			var out, errOut bytes.Buffer
			code := run([]string{"--format", format, "../../testdata/reordered.json"},
				strings.NewReader(""), &out, &errOut)
			if code != 0 {
				t.Fatalf("exit code = %d, want 0. stderr: %s", code, errOut.String())
			}
			if n := strings.Count(out.String(), "Order is significant"); n != 4 {
				t.Errorf("%s must carry the whole sentence on each of the 4 findings, got %d:\n%s",
					format, n, out.String())
			}
		})
	}
}

// TestRunSuggestsAMovedBlockOncePerPair. The annotation is attached to
// both the destroy and the create, and both reads are correct, but the
// suggested block is one pair's - printing it twice invites pasting it
// twice, and a moved block pasted twice is a state file adopting a
// resource it was never meant to.
func TestRunSuggestsAMovedBlockOncePerPair(t *testing.T) {
	var out, errOut bytes.Buffer
	if code := run([]string{"../../testdata/rename-no-moved.json"}, strings.NewReader(""), &out, &errOut); code != 0 {
		t.Fatalf("exit code = %d, want 0. stderr: %s", code, errOut.String())
	}
	got := out.String()

	if n := strings.Count(got, "possible missed moved block"); n != 2 {
		t.Errorf("both halves of the pair must be told, found %d:\n%s", n, got)
	}
	for _, once := range []string{"moved { from =", "5 of 5 attributes match"} {
		if n := strings.Count(got, once); n != 1 {
			t.Errorf("%q must appear once, found %d:\n%s", once, n, got)
		}
	}
}

// TestNoAttributeValueEverReachesAnyFormat is the product's core promise,
// checked end to end against a plan built to break it.
//
// testdata/reordered-secrets.json holds values that are obvious secrets -
// an access key, a bearer token, connection strings with passwords in
// them, a vault token inside a bootstrap script, a role ARN inside an IAM
// policy document - none of which Terraform has marked sensitive, which is
// exactly the case the README warns about: a plan file can carry anything
// state can carry, in the clear, whether or not anything marked it.
//
// Every rule that compares two values is made to read every one of them.
// The lists are reordered, so the same-elements-reordered rule walks them;
// aws_iam_role_policy.deploy carries a reordered JSON policy, a bootstrap
// script that differs only in its line endings, an account id that became
// a string and a null that became an empty list, so all four rewrite
// classes and the roll-up read theirs too.
//
// Reading them is necessary and fine. Printing one is not. This runs the
// whole command over all four formats and asserts that not one secret
// appears in any of them.
func TestNoAttributeValueEverReachesAnyFormat(t *testing.T) {
	// This one is about CONTENT, so the colour environment is pinned off
	// rather than inherited: with FORCE_COLOR set, escape codes land between
	// the words of the roll-up and its substring assertions stop matching a
	// sentence that is perfectly well there.
	t.Setenv("NO_COLOR", "")
	t.Setenv("FORCE_COLOR", "")
	secrets := []string{
		"AKIAIOSFODNN7EXAMPLE-LEAKED-ACCESS-KEY",
		"wJalrXUtnFEMI-LEAKED-SECRET-KEY",
		"eyJhbGciOiJIUzI1NiJ9.LEAKED-BEARER-TOKEN",
		"LEAKED-DB-PASSWORD",
		"LEAKED-REDIS-AUTH",
		"LEAKED-CLIENT-SECRET-NOT-MARKED-SENSITIVE",
		"hvs.LEAKED-VAULT-TOKEN",
		"LEAKED-MYSQL-PASSWORD",
		"LEAKED-ROLE-INSIDE-A-POLICY",
		// Fragments too. A renderer that truncated or escaped a value
		// would still have leaked it.
		"postgres://admin",
		"redis://",
		"cache.internal",
		"db.internal",
		"sql.internal",
		"mysql://root",
		"sts:AssumeRole",
		"987654321098",
	}

	for _, format := range []string{"terminal", "md", "json", "html"} {
		t.Run(format, func(t *testing.T) {
			var out, errOut bytes.Buffer
			code := run([]string{"--format", format, "../../testdata/reordered-secrets.json"},
				strings.NewReader(""), &out, &errOut)
			if code != 0 {
				t.Fatalf("exit code = %d, want 0. stderr: %s", code, errOut.String())
			}
			got := out.String()
			// The terminal wraps, so a sentence and even a single long
			// token can arrive split across lines. Every check below runs
			// against the output with its whitespace collapsed to one
			// space and against the output with its whitespace removed
			// altogether: the first finds a wrapped sentence, the second
			// finds a secret that was cut mid-word at the right margin,
			// which a check on the raw text would read straight past.
			spaced := strings.Join(strings.Fields(got), " ")
			squashed := strings.Join(strings.Fields(got), "")

			// The report has to have done its job, or this test passes by
			// rendering nothing at all.
			for _, address := range []string{"aws_ssm_parameter.pipeline", "aws_iam_role_policy.deploy"} {
				if !strings.Contains(spaced, address) {
					t.Fatalf("the %s report is missing %s entirely:\n%s", format, address, got)
				}
			}
			// One path per rule, so a rule that silently stopped firing
			// cannot make this test pass by having nothing to leak.
			for _, path := range []string{
				"values", "connection.hosts", // same elements, different order
				"policy",       // same JSON, written differently
				"bootstrap",    // same text, different whitespace
				"account_id",   // same number, written differently
				"session_tags", // null on one side, empty on the other
			} {
				if !strings.Contains(spaced, path) {
					t.Fatalf("the %s report is missing the %s attribute path:\n%s", format, path, got)
				}
			}
			if !strings.Contains(spaced, "difference in how the value is written") {
				t.Fatalf("the %s report is missing the roll-up:\n%s", format, got)
			}

			for _, s := range secrets {
				for _, haystack := range []string{got, spaced, squashed} {
					if strings.Contains(haystack, s) {
						t.Errorf("an attribute value reached the %s output: %q\n%s", format, s, got)
						break
					}
				}
			}
		})
	}
}

// TestNoDetectedCredentialReachesAnyFormat is the same promise held against
// the feature most likely to break it.
//
// The credential detector reads every string value in a plan and decides which
// of them look like secrets. That is the one part of this tool that knows
// where the secrets are, so a leak here would be a leak of exactly the values
// that matter most - and it would be worse than not looking, because the
// output names the ones worth stealing.
//
// testdata/unmarked-credentials.json is real terraform output from a root of
// terraform_data and local_file. Every value in it is fabricated. It reproduces
// the incident that shaped this tool: a variable declared `sensitive = true`
// whose value still sits in the clear in the plan's top-level variables block,
// because that block carries no sensitivity information at all.
func TestNoDetectedCredentialReachesAnyFormat(t *testing.T) {
	planted := []string{
		"ghp_000000000000000000000000000000000000",
		"postgres://admin:hunter2@db.internal:5432/app",
		"hunter2",
		"AKIAIOSFODNN7EXAMPLE",
		"Zx9Kq2mWv7Lp4Nd8Rt6Yb3Fh5Jc1Ag0Se7Uk2Mo9Qi4Xz",
		"v1.0-fakefakefake-NOTAREALTOKEN-0000000000000000000000000000000000000000",
		"MIIEowIBAAKCAQEAx0000000000000000000000000000000000000000000000000",
		"BEGIN RSA PRIVATE KEY",
		// Fragments. A renderer that truncated a value to "show a bit of it"
		// would still have leaked enough to identify the credential.
		"NOTAREALTOKEN",
		"ghp_0000",
		"AKIAIOSFO",
		"admin:hunter2",
	}

	// gate is in the list because it is the format most likely to be piped
	// into something that keeps it - an agent's context window, a CI log, a
	// job summary - which makes it the worst place for a value to appear.
	for _, format := range []string{"terminal", "md", "json", "html", "gate"} {
		t.Run(format, func(t *testing.T) {
			var out, errOut bytes.Buffer
			code := run([]string{"--format", format, "../../testdata/unmarked-credentials.json"},
				strings.NewReader(""), &out, &errOut)
			if code != 0 {
				t.Fatalf("exit code = %d, want 0. stderr: %s", code, errOut.String())
			}
			got := out.String()
			spaced := strings.Join(strings.Fields(got), " ")
			squashed := strings.Join(strings.Fields(got), "")

			// The detector has to have done its job, or this test passes by
			// finding nothing to leak.
			for _, path := range []string{
				"cloudflare_api_token", // the incident, in the variables block
				"input.api_token",      // a recognised token prefix
				"input.database_url",   // a password in a connection string
				"input.access_key_id",  // a recognised key format
				"input.material",       // a private key header
				"input.edge_handle",    // entropy alone, under a neutral name
			} {
				if !strings.Contains(spaced, path) {
					t.Fatalf("the %s output never names %s, so it has nothing to leak:\n%s", format, path, got)
				}
			}

			for _, s := range planted {
				for _, haystack := range []string{got, spaced, squashed} {
					if strings.Contains(haystack, s) {
						t.Errorf("a detected value reached the %s output: %q\n%s", format, s, got)
						break
					}
				}
			}
		})
	}
}

func TestTheCredentialCaveatIsNeverSeparatedFromTheClaim(t *testing.T) {
	// A reader who takes away "terraken says there is a GitHub token at
	// input.api_token" and not "this is pattern matching" has been told
	// something the tool cannot support. Every format carries both.
	for _, format := range []string{"terminal", "md", "json", "html", "gate"} {
		var out, errOut bytes.Buffer
		if code := run([]string{"--format", format, "../../testdata/unmarked-credentials.json"},
			strings.NewReader(""), &out, &errOut); code != 0 {
			t.Fatalf("%s: exit %d, stderr: %s", format, code, errOut.String())
		}
		spaced := strings.Join(strings.Fields(out.String()), " ")
		if !strings.Contains(spaced, "pattern matching") {
			t.Errorf("the %s output states the finding without the caveat:\n%s", format, out.String())
		}
	}
}

func TestADisplayFilterCannotHideTheCredentials(t *testing.T) {
	// --min-level turns the volume down on the findings. The plan file holding
	// a token is not a finding and not a level, and no filter has any business
	// touching it.
	var out, errOut bytes.Buffer
	if code := run([]string{"--min-level", "critical", "--no-colour",
		"../../testdata/unmarked-credentials.json"}, strings.NewReader(""), &out, &errOut); code != 0 {
		t.Fatalf("exit %d, stderr: %s", code, errOut.String())
	}
	spaced := strings.Join(strings.Fields(out.String()), " ")
	if !strings.Contains(spaced, "cloudflare_api_token") {
		t.Fatalf("a display filter hid the credentials block:\n%s", out.String())
	}
}

func TestMovedPrintsHCLAndNothingElse(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run([]string{"--moved", "../../testdata/rename-no-moved.json"}, nil, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit %d, stderr: %s", code, stderr.String())
	}
	out := stdout.String()
	if !strings.Contains(out, "moved {") {
		t.Fatalf("no moved block in output:\n%s", out)
	}
	// THE REPORT MUST NOT BE IN THERE. A reader doing
	// `terraken --moved plan.json >> main.tf` would otherwise get a risk
	// report in their configuration.
	for _, leak := range []string{"CRITICAL", "HIGH", "findings", "terraform 1."} {
		if strings.Contains(out, leak) {
			t.Fatalf("the report leaked into the HCL output (%q):\n%s", leak, out)
		}
	}
	if stderr.Len() != 0 {
		t.Fatalf("expected a silent stderr, got: %s", stderr.String())
	}
}

func TestMovedExitsZeroEvenWhenThePlanIsCritical(t *testing.T) {
	// --moved answers a different question from the report, so it is not a
	// gate. A pipeline asking for the blocks should not fail because the plan
	// it read them from was alarming.
	var stdout, stderr bytes.Buffer
	if code := run([]string{"--moved", "../../testdata/critical.json"}, nil, &stdout, &stderr); code != 0 {
		t.Fatalf("exit %d, stderr: %s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "No missed moved blocks") {
		t.Fatalf("expected the empty case to say so:\n%s", stdout.String())
	}
}

func TestMovedWritesOnlyWhereTold(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "moved.tf")

	var stdout, stderr bytes.Buffer
	if code := run([]string{"--moved", "--out", path, "../../testdata/rename-no-moved.json"}, nil, &stdout, &stderr); code != 0 {
		t.Fatalf("exit %d, stderr: %s", code, stderr.String())
	}
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("nothing written to the named path: %v", err)
	}
	if !strings.Contains(string(b), "moved {") {
		t.Fatalf("named file has no moved block:\n%s", b)
	}
	// 0600, like the report, and for the same reason: the proposal names
	// renamed resources across the estate.
	//
	// UNIX ONLY, AND THAT IS A REAL LIMIT RATHER THAN A TEST QUIRK. NTFS has
	// no Unix mode bits, so Go ignores the perm argument on Windows and
	// os.Stat reports 0666 whatever was asked for. The file is created with
	// default ACLs there, which on a shared runner is weaker than 0600.
	//
	// The windows matrix job added in #17 caught this on its first real run,
	// which is the whole reason that job exists.
	if runtime.GOOS != "windows" {
		fi, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		if perm := fi.Mode().Perm(); perm != 0o600 {
			t.Fatalf("expected mode 0600, got %o", perm)
		}
	}
	// Nothing anywhere else. The directory holds exactly the file asked for.
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		var names []string
		for _, e := range entries {
			names = append(names, e.Name())
		}
		t.Fatalf("expected one file, found %v", names)
	}
}

func TestABrokenRuleFileStopsTheRunRatherThanBeingSkipped(t *testing.T) {
	// The issue: "a policy that silently does not run is worse than no
	// policy". Exit 2 is "the tool could not do its job", distinct from
	// exit 1, "the plan failed your gate" - a pipeline must be able to tell
	// a broken policy from a caught one.
	dir := t.TempDir()
	bad := filepath.Join(dir, "rules.json")
	if err := os.WriteFile(bad, []byte(`{"rules":[{"id":"a","when":{"actions":["delete"]}}]}`), 0o600); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	code := run([]string{"--rules", bad, "../../testdata/critical.json"}, nil, &stdout, &stderr)
	if code != 2 {
		t.Fatalf("expected exit 2 for a broken rule file, got %d", code)
	}
	if !strings.Contains(stderr.String(), "no message") {
		t.Fatalf("stderr does not say what is wrong: %s", stderr.String())
	}
	// And nothing that looks like a report went to stdout.
	if strings.Contains(stdout.String(), "CRITICAL") {
		t.Fatalf("a report was produced despite the broken policy:\n%s", stdout.String())
	}
}

func TestAMissingRuleFileStopsTheRun(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run([]string{"--rules", "/nonexistent/rules.json", "../../testdata/critical.json"}, nil, &stdout, &stderr)
	if code != 2 {
		t.Fatalf("expected exit 2 for a missing rule file, got %d", code)
	}
}

func TestARuleCanDriveTheExistingGate(t *testing.T) {
	// "Exit code integrates with the existing gate." A rule that raises a
	// finding to critical must make --fail-on critical fire, on a plan whose
	// built-in classification would not have.
	dir := t.TempDir()
	rules := filepath.Join(dir, "rules.json")
	src := `{"rules":[{"id":"no-updates","message":"updates are not allowed here","level":"critical",` +
		`"when":{"actions":["update"]}}]}`
	if err := os.WriteFile(rules, []byte(src), 0o600); err != nil {
		t.Fatal(err)
	}

	// Without the rules, this plan has nothing critical and the gate passes.
	var a, ae bytes.Buffer
	if code := run([]string{"--fail-on", "critical", "../../testdata/large-estate.json"}, nil, &a, &ae); code != 0 {
		t.Fatalf("expected the ungated plan to pass, got %d", code)
	}
	// With them, it fails.
	var b, be bytes.Buffer
	if code := run([]string{"--rules", rules, "--fail-on", "critical", "../../testdata/large-estate.json"}, nil, &b, &be); code != 1 {
		t.Fatalf("expected the rule to fire the gate, got %d", code)
	}
}

func TestTheGateIgnoresMinLevel(t *testing.T) {
	// THE SUBTLE ONE. --min-level turns the human report's volume down. A
	// verdict computed from the quieter version would let a plan pass a gate
	// because somebody was not looking, which is precisely the negotiation
	// this mode exists to prevent.
	var quiet, qerr bytes.Buffer
	code := run([]string{"--format", "gate", "--fail-on", "high", "--min-level", "critical",
		"../../testdata/large-estate.json"}, nil, &quiet, &qerr)
	if code != 1 {
		t.Fatalf("expected the gate to fail regardless of --min-level, got %d", code)
	}
	if !strings.Contains(quiet.String(), `"verdict": "fail"`) {
		t.Fatalf("--min-level talked the gate into passing:\n%s", quiet.String())
	}
}

func TestTheGateFormatIsAccepted(t *testing.T) {
	var out, errOut bytes.Buffer
	if code := run([]string{"--format", "gate", "../../testdata/minimal.json"}, nil, &out, &errOut); code != 0 {
		t.Fatalf("exit %d: %s", code, errOut.String())
	}
	if !strings.Contains(out.String(), "terraken.gate/v1") {
		t.Fatalf("no schema in the output:\n%s", out.String())
	}
}
