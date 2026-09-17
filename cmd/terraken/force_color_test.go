package main

import (
	"bytes"
	"strings"
	"testing"
)

// EVERY TEST IN HERE SETS BOTH VARIABLES, always, whatever it is measuring.
//
// Colour is decided by NO_COLOR and FORCE_COLOR together, and NO_COLOR
// deliberately wins, so a test that sets only FORCE_COLOR is not testing
// FORCE_COLOR - it is testing whether whoever ran the suite happens to have
// NO_COLOR in their shell. With NO_COLOR=1 inherited, the first test below
// failed and the rest passed for the wrong reason.
//
// colourEnv sets the pair explicitly so the environment cannot decide the
// answer. An empty value is the same as unset to the tool, which is what
// no-color.org specifies and what isTTY implements.
func colourEnv(t *testing.T, noColor, forceColor string) {
	t.Helper()
	t.Setenv("NO_COLOR", noColor)
	t.Setenv("FORCE_COLOR", forceColor)
}

func TestForceColorMakesColourSurviveAPipe(t *testing.T) {
	colourEnv(t, "", "1")
	var out, errOut bytes.Buffer
	if code := run([]string{"../../testdata/critical.json"}, strings.NewReader(""), &out, &errOut); code != 0 {
		t.Fatalf("exit = %d, stderr: %s", code, errOut.String())
	}
	if !strings.Contains(out.String(), "\x1b[") {
		t.Error("FORCE_COLOR must produce colour even though the writer is not a terminal")
	}
}

func TestNoColorBeatsForceColor(t *testing.T) {
	colourEnv(t, "1", "1")
	var out, errOut bytes.Buffer
	run([]string{"../../testdata/critical.json"}, strings.NewReader(""), &out, &errOut)
	if strings.Contains(out.String(), "\x1b[") {
		t.Error("NO_COLOR must win: turning colour off is never the setting that loses")
	}
}

func TestForceColorZeroIsNotOptingIn(t *testing.T) {
	colourEnv(t, "", "0")
	var out, errOut bytes.Buffer
	run([]string{"../../testdata/critical.json"}, strings.NewReader(""), &out, &errOut)
	if strings.Contains(out.String(), "\x1b[") {
		t.Error("FORCE_COLOR=0 must not enable colour")
	}
}

// An empty NO_COLOR is not opting out. no-color.org says the variable must be
// present AND non-empty, and reading "" as "off" would mean anybody who had
// ever exported it blank could never get a coloured report again.
func TestEmptyNoColorIsNotOptingOut(t *testing.T) {
	colourEnv(t, "", "1")
	var out, errOut bytes.Buffer
	run([]string{"../../testdata/critical.json"}, strings.NewReader(""), &out, &errOut)
	if !strings.Contains(out.String(), "\x1b[") {
		t.Error("an empty NO_COLOR must not disable colour")
	}
}

// And the flags beat the environment, both spellings.
func TestTheFlagsBeatForceColor(t *testing.T) {
	for _, flag := range []string{"--no-colour", "--no-color", "--plain"} {
		t.Run(flag, func(t *testing.T) {
			colourEnv(t, "", "1")
			var out, errOut bytes.Buffer
			run([]string{flag, "../../testdata/critical.json"}, strings.NewReader(""), &out, &errOut)
			if strings.Contains(out.String(), "\x1b[") {
				t.Errorf("%s must turn colour off even with FORCE_COLOR set", flag)
			}
		})
	}
}
