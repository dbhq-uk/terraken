package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const unsupportedPlan = "../../testdata/unsupported-action.json"

// The end of the acceptance for an operation this build cannot assess: it
// reaches a person running the command, it does not decide anything on its
// own, and the exit code a team already depends on does not move underneath
// them.

func TestAnUnrecognisedOperationIsReportedAndDoesNotFailTheRun(t *testing.T) {
	var out, errOut bytes.Buffer
	code := run([]string{"--no-colour", unsupportedPlan}, strings.NewReader(""), &out, &errOut)
	if code != 0 {
		t.Fatalf("exit code = %d, want 0 - no --fail-on was given. stderr: %s", code, errOut.String())
	}
	for _, want := range []string{
		"UNRANKED",
		"terraform_data.reconciled",
		"does not recognise",
		`"reconcile"`,
		"2 unranked",
	} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("the report does not mention %q:\n%s", want, out.String())
		}
	}
}

// --fail-on is a severity threshold. An operation with no severity cannot
// clear one, so a team pinned to --fail-on critical does not start failing
// the day Terraform ships an action verb this build has never seen.
func TestFailOnDoesNotSeeAnOperationWithNoSeverity(t *testing.T) {
	// The fixture holds two unrankable operations and one ordinary delete,
	// which is high. So critical passes and high fails, exactly as it would
	// without the unrankable pair in the file.
	for _, tc := range []struct {
		threshold string
		want      int
	}{
		{"critical", 0},
		{"high", 1},
		{"low", 1},
		{"info", 1},
	} {
		var out, errOut bytes.Buffer
		code := run([]string{"--fail-on", tc.threshold, "--format", "gate", unsupportedPlan},
			strings.NewReader(""), &out, &errOut)
		if code != tc.want {
			t.Errorf("--fail-on %s: exit code = %d, want %d\n%s", tc.threshold, code, tc.want, out.String())
		}
	}
}

// A team that does want it to decide something says so in a file somebody
// committed. That is the whole escape hatch, and it is the mechanism the
// tool already has for "our policy" rather than a second one beside it.
func TestATeamsRuleCanMakeAnUnrecognisedOperationFailTheRun(t *testing.T) {
	dir := t.TempDir()
	rules := filepath.Join(dir, "rules.json")
	if err := os.WriteFile(rules, []byte(`{"rules":[{
		"id":"unreadable-plan",
		"message":"terraken could not assess this operation, so this plan has not been reviewed",
		"level":"critical",
		"when":{"actions":["unsupported"]}}]}`), 0o600); err != nil {
		t.Fatal(err)
	}

	var out, errOut bytes.Buffer
	code := run([]string{"--rules", rules, "--fail-on", "critical", "--no-colour", unsupportedPlan},
		strings.NewReader(""), &out, &errOut)
	if code != 1 {
		t.Fatalf("exit code = %d, want 1 - the rule gave it a severity. stderr: %s", code, errOut.String())
	}
	if !strings.Contains(out.String(), "so this plan has not been reviewed") {
		t.Errorf("the team's own message is not in the report:\n%s", out.String())
	}
}

// The machine output an agent parses. It is the consumer least able to
// notice something that quietly went missing, so the array is there whether
// or not a gate was asked for.
func TestTheGateCarriesUnassessedOperationsWithNoThreshold(t *testing.T) {
	var out, errOut bytes.Buffer
	code := run([]string{"--format", "gate", unsupportedPlan}, strings.NewReader(""), &out, &errOut)
	if code != 0 {
		t.Fatalf("exit code = %d, want 0. stderr: %s", code, errOut.String())
	}

	var v struct {
		Verdict     string `json:"verdict"`
		Unsupported []struct {
			Address string   `json:"address"`
			Actions []string `json:"actions"`
			Detail  string   `json:"detail"`
		} `json:"unsupported"`
	}
	if err := json.Unmarshal(out.Bytes(), &v); err != nil {
		t.Fatalf("gate output is not valid JSON: %v\n%s", err, out.String())
	}
	if v.Verdict != "pass" {
		t.Errorf("verdict = %q, want pass - no threshold was set", v.Verdict)
	}
	if len(v.Unsupported) != 2 {
		t.Fatalf("unsupported = %+v, want 2 entries", v.Unsupported)
	}
	for _, u := range v.Unsupported {
		if len(u.Actions) == 0 || u.Detail == "" {
			t.Errorf("%s: actions = %v, detail = %q; both are required", u.Address, u.Actions, u.Detail)
		}
	}
}

// --min-level turns the report's volume down. It must never be able to turn
// this off: a finding the tool could not assess is not a quiet one, it is an
// unmeasured one, and hiding it is the failure the finding exists to
// correct.
func TestNoMinLevelCanHideAnOperationTheToolCouldNotAssess(t *testing.T) {
	for _, floor := range []string{"info", "low", "high", "critical"} {
		var out, errOut bytes.Buffer
		code := run([]string{"--min-level", floor, "--no-colour", unsupportedPlan},
			strings.NewReader(""), &out, &errOut)
		if code != 0 {
			t.Fatalf("--min-level %s: exit code = %d. stderr: %s", floor, code, errOut.String())
		}
		if !strings.Contains(out.String(), "terraform_data.reconciled") {
			t.Errorf("--min-level %s hid an unrankable finding:\n%s", floor, out.String())
		}
	}
}

// "unranked" is the tool saying it has no severity to give. It is not a
// severity somebody can ask for, and a flag that quietly accepted it would
// be a gate nobody could reason about.
func TestUnrankedIsNotSomethingAFlagAccepts(t *testing.T) {
	for _, flag := range []string{"--fail-on", "--min-level"} {
		var out, errOut bytes.Buffer
		code := run([]string{flag, "unranked", unsupportedPlan}, strings.NewReader(""), &out, &errOut)
		if code != 2 {
			t.Errorf("%s unranked: exit code = %d, want 2", flag, code)
		}
		if !strings.Contains(errOut.String(), "unknown level") {
			t.Errorf("%s unranked: stderr = %q, want an unknown level error", flag, errOut.String())
		}
	}
}
