package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/dbhq-uk/terraken/internal/render"
)

// The three flags reach a reader in every format, and decide nothing.

func TestPlanStatusIsReportedInEveryFormat(t *testing.T) {
	for _, format := range render.Formats {
		t.Run(format, func(t *testing.T) {
			var out, errOut bytes.Buffer
			code := run([]string{"--format", format, "--no-colour", "../../testdata/errored-plan.json"},
				strings.NewReader(""), &out, &errOut)
			if code != 0 {
				t.Fatalf("exit = %d, stderr: %s", code, errOut.String())
			}
			for _, want := range []string{"errored", "complete", "applyable"} {
				if !strings.Contains(out.String(), want) {
					t.Errorf("%s does not mention %q:\n%s", format, want, out.String())
				}
			}
			// The NAMES are not enough: a renderer that printed three labels
			// and no values would pass that. The fixture states errored true,
			// complete false and applyable false, and the human formats have
			// to say which is which.
			if format == "terminal" || format == "md" || format == "html" {
				flat := strings.Join(strings.Fields(out.String()), " ")
				for _, want := range []string{"errored yes", "complete no", "applyable no"} {
					name, state, _ := strings.Cut(want, " ")
					// Each format wraps the pair differently: the terminal
					// puts them on one line, markdown in two table cells,
					// HTML in a <strong> and a text node. What has to be true
					// everywhere is that the state appears next to its name.
					forms := []string{
						want,
						"| " + name + " | " + state,
						"<strong>" + name + "</strong> " + state,
					}
					var found bool
					for _, f := range forms {
						if strings.Contains(flat, f) {
							found = true
						}
					}
					if !found {
						t.Errorf("%s does not state %q:\n%s", format, want, out.String())
					}
				}
			}
		})
	}
}

// NOT A FINDING, NOT A LEVEL, NOT A COUNT. design.md decides where new
// information goes: if it is not one resource change losing data, it belongs
// outside the severity counts rather than at the top of them. An errored plan
// is not a resource change at all.
func TestPlanStatusIsNotAFindingAndNotCounted(t *testing.T) {
	var out, errOut bytes.Buffer
	if code := run([]string{"--format", "json", "../../testdata/errored-plan.json"},
		strings.NewReader(""), &out, &errOut); code != 0 {
		t.Fatalf("exit = %d, stderr: %s", code, errOut.String())
	}

	var report struct {
		Findings []struct {
			Address string `json:"address"`
			Level   string `json:"level"`
		} `json:"findings"`
		Counts map[string]int `json:"counts"`
		Status struct {
			Errored   *bool `json:"errored"`
			Complete  *bool `json:"complete"`
			Applyable *bool `json:"applyable"`
		} `json:"status"`
	}
	if err := json.Unmarshal(out.Bytes(), &report); err != nil {
		t.Fatalf("not valid JSON: %v\n%s", err, out.String())
	}

	// The fixture holds exactly one resource change, and the plan status is
	// not a second one.
	if len(report.Findings) != 1 {
		t.Errorf("got %d findings, want 1 - the plan status must not be one", len(report.Findings))
	}
	for _, f := range report.Findings {
		if f.Level == "critical" {
			t.Errorf("%s came out critical; critical means a resource type that holds data "+
				"is being destroyed, and nothing else", f.Address)
		}
	}
	if total := report.Counts["critical"] + report.Counts["high"] +
		report.Counts["low"] + report.Counts["info"]; total != 1 {
		t.Errorf("counts total %d, want 1 - the plan status must not be counted: %v",
			total, report.Counts)
	}
	if report.Status.Errored == nil || !*report.Status.Errored {
		t.Errorf("errored = %v, want true", report.Status.Errored)
	}
}

// AN ERRORED PLAN DOES NOT FAIL A GATE BY ITSELF, and a clean no-op plan that
// is deliberately not applyable does not either. --fail-on takes a severity,
// and none of these is one.
func TestPlanStatusDoesNotMoveTheExitCode(t *testing.T) {
	for _, tc := range []struct {
		fixture, threshold string
		want               int
		why                string
	}{
		// errored: true, complete: false, applyable: false, and one high
		// finding. critical passes, high fails - exactly as it would without
		// the flags.
		{"errored-plan.json", "critical", 0, "an errored plan is not a data loss"},
		{"errored-plan.json", "high", 1, "the delete in it is still high"},
		// The trap: a perfectly clean plan is not applyable, and anything
		// treating that as risk would fail the plan that most deserves to pass.
		{"clean-noop-plan.json", "critical", 0, "a no-op plan is clean"},
		// low, not info: the no-op in this fixture is legitimately an info
		// finding, and --fail-on info blocking on it is the gate working. The
		// point here is that applyable: false adds nothing to that.
		{"clean-noop-plan.json", "low", 0, "applyable: false is not a finding"},
	} {
		t.Run(tc.fixture+"/"+tc.threshold, func(t *testing.T) {
			var out, errOut bytes.Buffer
			code := run([]string{"--fail-on", tc.threshold, "--format", "gate",
				"../../testdata/" + tc.fixture}, strings.NewReader(""), &out, &errOut)
			if code != tc.want {
				t.Errorf("exit = %d, want %d - %s\n%s", code, tc.want, tc.why, out.String())
			}
		})
	}
}

// The gate carries all three under a stable key, with null as a third state:
// "not stated" is a different fact from "false" and a caller has to be able to
// tell them apart.
func TestTheGateCarriesAllThreeStates(t *testing.T) {
	type gate struct {
		Verdict string `json:"status_probe"`
		Status  struct {
			Errored   *bool `json:"errored"`
			Complete  *bool `json:"complete"`
			Applyable *bool `json:"applyable"`
		} `json:"status"`
	}

	// A plan that states none of them: every key present, every value null.
	var out, errOut bytes.Buffer
	if code := run([]string{"--format", "gate", "../../testdata/critical.json"},
		strings.NewReader(""), &out, &errOut); code != 0 {
		t.Fatalf("exit = %d: %s", code, errOut.String())
	}
	// EVERY KEY PRESENT, not just the object. A missing key decodes to nil
	// exactly like an explicit null, so {"status":{}} would satisfy the
	// assertions below while telling a caller nothing at all.
	for _, key := range []string{`"errored"`, `"complete"`, `"applyable"`} {
		if !strings.Contains(out.String(), key) {
			t.Errorf("the gate must always carry %s, even when the plan does not state it:\n%s",
				key, out.String())
		}
	}
	var g gate
	if err := json.Unmarshal(out.Bytes(), &g); err != nil {
		t.Fatal(err)
	}
	if g.Status.Errored != nil || g.Status.Complete != nil || g.Status.Applyable != nil {
		t.Errorf("a plan stating nothing must report null, not false: %+v", g.Status)
	}

	// And one that states all three.
	out.Reset()
	errOut.Reset()
	if code := run([]string{"--format", "gate", "../../testdata/errored-plan.json"},
		strings.NewReader(""), &out, &errOut); code != 0 {
		t.Fatalf("exit = %d: %s", code, errOut.String())
	}
	if err := json.Unmarshal(out.Bytes(), &g); err != nil {
		t.Fatal(err)
	}
	if g.Status.Errored == nil || !*g.Status.Errored {
		t.Error("errored should be true")
	}
	if g.Status.Complete == nil || *g.Status.Complete {
		t.Error("complete should be false, not absent")
	}
	if g.Status.Applyable == nil || *g.Status.Applyable {
		t.Error("applyable should be false, not absent")
	}

	// And a true value survives as true, which none of the above proves.
	out.Reset()
	errOut.Reset()
	if code := run([]string{"--format", "gate", "../../testdata/clean-noop-plan.json"},
		strings.NewReader(""), &out, &errOut); code != 0 {
		t.Fatalf("exit = %d: %s", code, errOut.String())
	}
	if err := json.Unmarshal(out.Bytes(), &g); err != nil {
		t.Fatal(err)
	}
	if g.Status.Complete == nil || !*g.Status.Complete {
		t.Error("complete should be true")
	}
	if g.Status.Errored == nil || *g.Status.Errored {
		t.Error("errored should be false")
	}
}

// A plan stating none of the three keeps its whole report, unchanged.
//
// THE CLAIM IS NARROWED TO WHAT IS ACTUALLY TRUE, because the looser version
// was not. The issue asks for the severity counts, the ordering and the
// --fail-on verdict to be byte-identical, and they are. The HTML document also
// gains the stylesheet rules for a block it will never render, and the machine
// formats gain a `status` key whose three values are all null - both permitted,
// the gate explicitly so by its additive v1 policy, and neither one worth
// claiming otherwise about.
//
// What must not happen is a status block appearing, or a finding moving.
func TestAPlanStatingNothingKeepsItsReport(t *testing.T) {
	// THE HUMAN FORMATS ONLY. json and gate carry the status key always, with
	// all three values null, and that is deliberate: a stable key with a third
	// state is what lets a machine consumer tell "does not converge" from
	// "could not tell". A person reading a report about a plan that says
	// nothing should simply not see the block.
	for _, format := range []string{"terminal", "md", "html"} {
		t.Run(format, func(t *testing.T) {
			var out, errOut bytes.Buffer
			if code := run([]string{"--format", format, "--no-colour", "--fail-on", "high",
				"../../testdata/demo.json"}, strings.NewReader(""), &out, &errOut); code != 1 {
				t.Fatalf("exit = %d, want 1 - demo.json has a high finding: %s", code, errOut.String())
			}
			got := out.String()
			for _, absent := range []string{"says about itself", "applyable", "not stated"} {
				if strings.Contains(got, absent) {
					t.Errorf("a plan stating nothing must print no status block, found %q:\n%s",
						absent, got)
				}
			}
		})
	}
}

// And the part the issue actually asks to be byte-identical: the findings, the
// counts, the ordering and the verdict. Compared field by field against the
// same plan with all three flags added, which must change none of them.
func TestStatingTheFlagsChangesNoFindingCountOrVerdict(t *testing.T) {
	withFlags := filepath.Join(t.TempDir(), "plan.json")
	raw, err := os.ReadFile("../../testdata/demo.json")
	if err != nil {
		t.Fatal(err)
	}
	var doc map[string]interface{}
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	doc["errored"], doc["complete"], doc["applyable"] = true, false, false
	patched, err := json.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(withFlags, patched, 0o600); err != nil {
		t.Fatal(err)
	}

	type gateShape struct {
		Verdict  string         `json:"verdict"`
		Counts   map[string]int `json:"counts"`
		Blocking []struct {
			Address string `json:"address"`
			Level   string `json:"level"`
		} `json:"blocking"`
	}
	read := func(path string) (gateShape, int) {
		var out, errOut bytes.Buffer
		code := run([]string{"--format", "gate", "--fail-on", "high", path},
			strings.NewReader(""), &out, &errOut)
		var g gateShape
		if err := json.Unmarshal(out.Bytes(), &g); err != nil {
			t.Fatalf("not valid JSON: %v\n%s", err, out.String())
		}
		return g, code
	}

	before, codeBefore := read("../../testdata/demo.json")
	after, codeAfter := read(withFlags)

	if codeBefore != codeAfter {
		t.Errorf("exit code changed from %d to %d when the flags were stated", codeBefore, codeAfter)
	}
	if !reflect.DeepEqual(before, after) {
		t.Errorf("the verdict, counts or blocking list changed when the flags were stated:\n"+
			"before: %+v\nafter:  %+v", before, after)
	}
	// An errored plan is the sharpest case: it must not have moved anything.
	if after.Verdict != "fail" || len(after.Blocking) == 0 {
		t.Errorf("the fixture should still fail on its own high finding: %+v", after)
	}
}

// The wording is Terraform's, and the two traps docs/plan-file.md records are
// the two things this must not say.
func TestTheStatusWordingDoesNotInferACause(t *testing.T) {
	var out, errOut bytes.Buffer
	if code := run([]string{"--no-colour", "../../testdata/errored-plan.json"},
		strings.NewReader(""), &out, &errOut); code != 0 {
		t.Fatalf("exit = %d: %s", code, errOut.String())
	}
	// Whitespace collapsed: the terminal wraps to the report width, so a
	// sentence this is looking for is split across lines as often as not.
	got := strings.Join(strings.Fields(out.String()), " ")

	// -target and deferred changes both make a plan incomplete and nothing in
	// the file says which. Naming either would be an inference the plan does
	// not support.
	for _, inference := range []string{"-target", "deferred"} {
		if strings.Contains(got, inference) {
			t.Errorf("the report infers a cause for complete: false (%q):\n%s", inference, got)
		}
	}
	// Both halves. Deleting the sentence that says what incomplete MEANS,
	// while keeping the caveat, would have passed the check below on its own.
	for _, want := range []string{
		"at least one more plan and apply round is expected",
		"The plan does not say why",
		"planning failed, so this plan cannot be applied",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("the report does not say %q:\n%s", want, got)
		}
	}
	// And applyable is never dressed up as a problem.
	if !strings.Contains(got, "not a fault") {
		t.Errorf("applyable: false must be stated as a fact, not as risk:\n%s", got)
	}
}
