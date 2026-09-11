package main

import (
	"bytes"
	"encoding/json"
	"os"
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
	if !strings.Contains(errOut.String(), "terminal") || !strings.Contains(errOut.String(), "md") || !strings.Contains(errOut.String(), "json") {
		t.Errorf("expected stderr to mention the valid formats, got: %s", errOut.String())
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
