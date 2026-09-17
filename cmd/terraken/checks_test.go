package main

import (
	"bytes"
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/dbhq-uk/terraken/internal/render"
)

// Failed checks, end to end, against real Terraform output.

func TestAFailedCheckIsReportedInEveryFormat(t *testing.T) {
	for _, format := range render.Formats {
		t.Run(format, func(t *testing.T) {
			var out, errOut bytes.Buffer
			if code := run([]string{"--format", format, "--no-colour",
				"../../testdata/real-checks.json"}, strings.NewReader(""), &out, &errOut); code != 0 {
				t.Fatalf("exit = %d: %s", code, errOut.String())
			}
			got := strings.Join(strings.Fields(out.String()), " ")
			if !strings.Contains(got, "check.budget_is_set") {
				t.Errorf("%s does not name the failed check:\n%s", format, out.String())
			}
			// The part a reader most needs, and the reason this feature
			// exists: Terraform treats the failure as a warning, so nothing
			// downstream has to notice it.
			//
			// NOT "exit 0", which the first version of this asserted. That
			// was two claims the plan does not support - another error may
			// have failed the plan anyway, and terraform plan
			// -detailed-exitcode returns 2 for a successful plan with changes.
			if !strings.Contains(got, "as a warning") {
				t.Errorf("%s does not say Terraform treats this as a warning:\n%s",
					format, out.String())
			}
			if strings.Contains(got, "exit 0") {
				t.Errorf("%s claims an exit code the plan does not state:\n%s",
					format, out.String())
			}
			// The problem count reaches a reader too. Markdown puts it in a
			// column of its own; the terminal and HTML put it in the line.
			if format == "terminal" || format == "html" {
				if !strings.Contains(got, "1 problem recorded") {
					t.Errorf("%s does not say how many problems were recorded:\n%s",
						format, out.String())
				}
			}
			if format == "md" && !strings.Contains(got, "| Problems |") {
				t.Errorf("markdown has no problems column:\n%s", out.String())
			}
		})
	}
}

// THE MESSAGE NEVER REACHES ANY OUTPUT. error_message is author-written text
// Terraform interpolates, and a plan generated to test this carried a live
// GitHub token in one. The count is safe where the message is not.
func TestACheckMessageNeverReachesAnyFormat(t *testing.T) {
	// The message the committed fixture actually holds, read from the fixture
	// rather than copied, so it cannot drift out of step with it.
	raw, err := os.ReadFile("../../testdata/real-checks.json")
	if err != nil {
		t.Fatal(err)
	}
	var plan struct {
		Checks []struct {
			Instances []struct {
				Problems []struct {
					Message string `json:"message"`
				} `json:"problems"`
			} `json:"instances"`
		} `json:"checks"`
	}
	if err := json.Unmarshal(raw, &plan); err != nil {
		t.Fatal(err)
	}
	var messages []string
	for _, c := range plan.Checks {
		for _, i := range c.Instances {
			for _, p := range i.Problems {
				messages = append(messages, p.Message)
			}
		}
	}
	if len(messages) == 0 {
		t.Fatal("the fixture carries no check message, so this proves nothing")
	}

	for _, format := range render.Formats {
		var out, errOut bytes.Buffer
		run([]string{"--format", format, "--no-colour", "../../testdata/real-checks.json"},
			strings.NewReader(""), &out, &errOut)
		for _, m := range messages {
			if strings.Contains(out.String(), m) {
				t.Errorf("%s printed a check's error_message, which Terraform interpolates "+
					"and which can hold an attribute value:\n%s", format, out.String())
			}
		}
	}
}

// A failed check does not fail the gate on its own. Terraform treats it as a
// warning; a team that disagrees branches on the array, which is one line and
// their policy.
func TestAFailedCheckDoesNotMoveTheVerdict(t *testing.T) {
	var out, errOut bytes.Buffer
	code := run([]string{"--format", "gate", "--fail-on", "critical",
		"../../testdata/real-checks.json"}, strings.NewReader(""), &out, &errOut)
	if code != 0 {
		t.Fatalf("exit = %d, want 0 - the plan holds nothing critical:\n%s", code, out.String())
	}

	var g struct {
		Verdict string `json:"verdict"`
		Checks  []struct {
			Address  string `json:"address"`
			Kind     string `json:"kind"`
			Status   string `json:"status"`
			Problems int    `json:"problems"`
			Detail   string `json:"detail"`
		} `json:"checks"`
	}
	if err := json.Unmarshal(out.Bytes(), &g); err != nil {
		t.Fatalf("not valid JSON: %v\n%s", err, out.String())
	}
	if g.Verdict != "pass" {
		t.Errorf("verdict = %q", g.Verdict)
	}
	if len(g.Checks) != 1 {
		t.Fatalf("checks = %+v, want 1", g.Checks)
	}
	c := g.Checks[0]
	if c.Address != "check.budget_is_set" || c.Status != "fail" ||
		c.Kind != "check block" || c.Problems != 1 {
		t.Errorf("got %+v", c)
	}
	if c.Detail == "" {
		t.Error("the entry carries no explanation")
	}
}

// A plan whose ONLY content is a failed check still reports it. The "no
// changes, this plan does nothing" shortcuts have to consult the checks list,
// and the other fixture has resource changes so it never exercises that.
func TestAChecksOnlyPlanStillReportsThem(t *testing.T) {
	for _, format := range []string{"terminal", "md", "html"} {
		t.Run(format, func(t *testing.T) {
			var out, errOut bytes.Buffer
			if code := run([]string{"--format", format, "--no-colour",
				"../../testdata/checks-only.json"}, strings.NewReader(""), &out, &errOut); code != 0 {
				t.Fatalf("exit = %d: %s", code, errOut.String())
			}
			got := out.String()
			if !strings.Contains(got, "check.budget_is_set") {
				t.Errorf("%s dropped the check on a plan with nothing else in it:\n%s", format, got)
			}
			if strings.Contains(got, "No changes. This plan does nothing.") &&
				!strings.Contains(got, "check.budget_is_set") {
				t.Errorf("%s claims the plan does nothing while a check failed:\n%s", format, got)
			}
		})
	}
}

// A failed check does not fail the gate at ANY threshold, not just critical.
func TestAFailedCheckDoesNotMoveTheVerdictAtAnyThreshold(t *testing.T) {
	for _, threshold := range []string{"critical", "high", "low", "info"} {
		var out, errOut bytes.Buffer
		code := run([]string{"--format", "gate", "--fail-on", threshold,
			"../../testdata/checks-only.json"}, strings.NewReader(""), &out, &errOut)
		if code != 0 {
			t.Errorf("--fail-on %s: exit = %d, want 0 - a failed check is not a severity:\n%s",
				threshold, code, out.String())
		}
	}
}
