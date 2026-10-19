package main

import (
	"bytes"
	"strings"
	"testing"

	"github.com/dbhq-uk/terraken/internal/assess"
	"github.com/dbhq-uk/terraken/internal/render"
)

func explainRun(t *testing.T, args ...string) (int, string, string) {
	t.Helper()
	var out, errb bytes.Buffer
	code := run(args, strings.NewReader(""), &out, &errb)
	return code, out.String(), errb.String()
}

// TestExplainNeedsNoPlan is the point of the feature. A reader meeting a code
// in a report is looking at a terminal, and asking what it means must not
// require having a plan to hand - nor must it read one.
func TestExplainNeedsNoPlan(t *testing.T) {
	code, out, errb := explainRun(t, "--explain", "blast-radius")
	if code != 0 {
		t.Fatalf("exit %d, stderr: %s", code, errb)
	}
	if !strings.Contains(out, "blast-radius") {
		t.Errorf("the output does not name the code: %q", out)
	}
	if !strings.Contains(out, "floor") {
		t.Errorf("the output does not carry the explanation: %q", out)
	}
}

// TestExplainCoversEveryCode walks the registry rather than a list, so a code
// added later is covered here too.
func TestExplainCoversEveryCode(t *testing.T) {
	for _, c := range assess.ExplainableCodes() {
		code, out, errb := explainRun(t, "--explain", c)
		if code != 0 {
			t.Errorf("--explain %s exited %d: %s", c, code, errb)
			continue
		}
		if !strings.Contains(out, c) {
			t.Errorf("--explain %s does not print the code back: %q", c, out)
		}
	}
}

// TestExplainWithNoValueListsTheCodes, which is how a reader finds the one they
// want without knowing it already.
func TestExplainWithNoValueListsTheCodes(t *testing.T) {
	code, out, errb := explainRun(t, "--explain")
	if code != 0 {
		t.Fatalf("exit %d, stderr: %s", code, errb)
	}
	for _, c := range assess.ExplainableCodes() {
		if !strings.Contains(out, c) {
			t.Errorf("the list omits %q", c)
		}
	}
}

// TestAnUnknownCodeExitsTwo. Two is what this command uses for "the tool could
// not do its job", and a misspelled code is a question it cannot answer -
// exiting 0 would let a script think it had an explanation.
func TestAnUnknownCodeExitsTwo(t *testing.T) {
	code, out, errb := explainRun(t, "--explain", "same-json-written-diferently")
	if code != 2 {
		t.Errorf("exit %d, want 2", code)
	}
	if out != "" {
		t.Errorf("an unknown code wrote to stdout: %q", out)
	}
	if !strings.Contains(errb, "same-json-written-diferently") {
		t.Errorf("the error does not name what it could not find: %q", errb)
	}
}

// TestExplainDoesNotNeedAFileArgument. Every other invocation requires a plan;
// this one must not, and the argument check runs after it.
func TestExplainDoesNotNeedAFileArgument(t *testing.T) {
	if code, _, errb := explainRun(t, "--explain", "sensitive"); code != 0 {
		t.Errorf("exit %d with no file argument: %s", code, errb)
	}
}

// TestCompletionsCoverEveryFlag. The script is generated from the flag set, so
// a flag added later completes without anybody remembering - which is the point
// of generating it rather than writing it.
func TestCompletionsCoverEveryFlag(t *testing.T) {
	for _, shell := range []string{"bash", "zsh", "fish"} {
		code, out, errb := explainRun(t, "--completion", shell)
		if code != 0 {
			t.Fatalf("%s: exit %d: %s", shell, code, errb)
		}
		for _, flag := range []string{"format", "fail-on", "min-level", "explain", "out", "rules", "moved", "version"} {
			if !strings.Contains(out, flag) {
				t.Errorf("%s completion omits --%s", shell, flag)
			}
		}
	}
}

// TestCompletionsOfferEveryAcceptedValue. A value the tool accepts is a value
// it completes: the script is built from the same registries the command
// validates against, so a format added to render.Formats appears here without a
// second list being edited.
func TestCompletionsOfferEveryAcceptedValue(t *testing.T) {
	_, out, _ := explainRun(t, "--completion", "fish")
	for _, f := range render.Formats {
		if !strings.Contains(out, f) {
			t.Errorf("the completion does not offer the format %q", f)
		}
	}
	for _, l := range assess.Levels() {
		if !strings.Contains(out, l.String()) {
			t.Errorf("the completion does not offer the level %q", l)
		}
	}
	for _, c := range assess.ExplainableCodes() {
		if !strings.Contains(out, c) {
			t.Errorf("the completion does not offer the code %q", c)
		}
	}
	// Unranked is not a level a flag accepts, and ParseLevel rejects it, so
	// completing it would offer a value the tool refuses.
	if strings.Contains(out, `-a "critical high low info unranked"`) {
		t.Error("the level completion offers unranked, which --fail-on rejects")
	}
}

// TestAnUnknownShellExitsTwo, for the reason an unknown code does.
func TestAnUnknownShellExitsTwo(t *testing.T) {
	code, out, errb := explainRun(t, "--completion", "powershell")
	if code != 2 {
		t.Errorf("exit %d, want 2", code)
	}
	if out != "" {
		t.Errorf("wrote a script for a shell it does not know: %q", out)
	}
	if !strings.Contains(errb, "powershell") {
		t.Errorf("the error does not name the shell: %q", errb)
	}
}

// TestNeitherFeatureReadsAPlan is the acceptance stated as a property: both
// work with no file argument and no readable plan anywhere.
func TestNeitherFeatureReadsAPlan(t *testing.T) {
	for _, args := range [][]string{
		{"--explain"},
		{"--explain", "blast-radius"},
		{"--completion", "bash"},
	} {
		if code, _, errb := explainRun(t, args...); code != 0 {
			t.Errorf("%v exited %d: %s", args, code, errb)
		}
	}
}
