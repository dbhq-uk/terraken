package main

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/dbhq-uk/terraken/internal/render"
)

// What changed underneath the estate, end to end.

func TestDriftIsReportedInEveryFormat(t *testing.T) {
	for _, format := range render.Formats {
		t.Run(format, func(t *testing.T) {
			var out, errOut bytes.Buffer
			if code := run([]string{"--format", format, "--no-colour", "../../testdata/drifted.json"},
				strings.NewReader(""), &out, &errOut); code != 0 {
				t.Fatalf("exit = %d: %s", code, errOut.String())
			}
			got := out.String()
			for _, want := range []string{"terraform_data.vanished", "local_file.config"} {
				if !strings.Contains(got, want) {
					t.Errorf("%s does not mention %q:\n%s", format, want, got)
				}
			}
			if !strings.Contains(strings.ToLower(got), "outside terraform") {
				t.Errorf("%s does not say the change happened outside Terraform:\n%s", format, got)
			}
		})
	}
}

// THE PAST TENSE, NOT THE PLANNED ONE. The verbs are the same words in both
// lists, and this is the confusion the separate list exists to prevent: a
// resource somebody already deleted must not appear with "destroy" beside it,
// reading as though this plan were about to destroy something already gone.
func TestDriftUsesThePastTenseNotThePlannedVerb(t *testing.T) {
	var out, errOut bytes.Buffer
	if code := run([]string{"--no-colour", "../../testdata/drifted.json"},
		strings.NewReader(""), &out, &errOut); code != 0 {
		t.Fatalf("exit = %d: %s", code, errOut.String())
	}
	body := out.String()

	drift, rest, found := strings.Cut(body, "CHANGED OUTSIDE TERRAFORM")
	_ = drift
	if !found {
		t.Fatalf("no drift section:\n%s", body)
	}
	// Everything from the drift heading to the first severity heading.
	section := rest
	for _, level := range []string{"\nCRITICAL", "\nHIGH", "\nLOW", "\nINFO"} {
		if i := strings.Index(section, level); i >= 0 {
			section = section[:i]
		}
	}
	if !strings.Contains(section, "gone") {
		t.Errorf("a destroyed resource should read as gone:\n%s", section)
	}
	if strings.Contains(section, "destroy and create") || strings.Contains(section, "\n  destroy\n") {
		t.Errorf("the drift section uses a planned-change verb:\n%s", section)
	}
}

// Drift is its own array in the machine output and does not move the verdict.
// It already happened, and blocking is what this change would do - stopping a
// deploy over something a deploy cannot fix is the wrong move.
func TestTheGateCarriesDriftWithoutMovingTheVerdict(t *testing.T) {
	var out, errOut bytes.Buffer
	// --fail-on low, NOT critical. The drifted delete ranks high, so a
	// threshold of critical would pass whether or not drift were wrongly
	// included in the severity evaluation. At low, the planned change is the
	// only thing that may block, and drift leaking in would change the answer.
	code := run([]string{"--format", "gate", "--fail-on", "low", "../../testdata/drifted.json"},
		strings.NewReader(""), &out, &errOut)
	if code != 1 {
		t.Fatalf("exit = %d, want 1 - the planned update is low:\n%s", code, out.String())
	}

	var g struct {
		Verdict  string `json:"verdict"`
		Blocking []struct {
			Address string `json:"address"`
		} `json:"blocking"`
		Drift []struct {
			Address  string   `json:"address"`
			Level    string   `json:"level"`
			Kind     string   `json:"kind"`
			DataLoss bool     `json:"data_loss"`
			Reasons  []string `json:"reasons"`
		} `json:"drift"`
	}
	if err := json.Unmarshal(out.Bytes(), &g); err != nil {
		t.Fatalf("not valid JSON: %v\n%s", err, out.String())
	}
	if g.Verdict != "fail" {
		t.Errorf("verdict = %q, want fail - the planned update is at the threshold", g.Verdict)
	}
	// EXACTLY ONE BLOCKING ENTRY, and it is the planned change. Drift already
	// happened, and blocking is what this change would do; stopping a deploy
	// over something a deploy cannot fix is the wrong move.
	if len(g.Blocking) != 1 || g.Blocking[0].Address != "local_file.config" {
		t.Errorf("blocking = %+v, want only the planned change", g.Blocking)
	}
	if len(g.Drift) != 2 {
		t.Fatalf("drift = %+v, want 2 entries", g.Drift)
	}
	// The first reason on every entry says it happened outside Terraform, so
	// an entry lifted into a log line cannot read as a planned change.
	want := map[string]struct {
		level    string
		kind     string
		dataLoss bool
	}{
		"terraform_data.vanished": {"high", "delete", false},
		"local_file.config":       {"low", "update", false},
	}
	for _, d := range g.Drift {
		w, known := want[d.Address]
		if !known {
			t.Errorf("unexpected drift entry %q", d.Address)
			continue
		}
		if d.Level != w.level || d.Kind != w.kind || d.DataLoss != w.dataLoss {
			t.Errorf("%s: %s/%s/%v, want %s/%s/%v",
				d.Address, d.Level, d.Kind, d.DataLoss, w.level, w.kind, w.dataLoss)
		}
		if len(d.Reasons) == 0 || !strings.Contains(strings.ToLower(d.Reasons[0]), "outside terraform") {
			t.Errorf("%s: first reason = %v, want it to say this happened outside Terraform",
				d.Address, d.Reasons)
		}
	}
}

// A plan with drift and no changes is not "No changes. This plan does
// nothing." - something happened, it just is not something this plan proposes.
func TestAPlanWithOnlyDriftDoesNotClaimNothingHappened(t *testing.T) {
	var out, errOut bytes.Buffer
	if code := run([]string{"--no-colour", "../../testdata/drift-only.json"},
		strings.NewReader(""), &out, &errOut); code != 0 {
		t.Fatalf("exit = %d: %s", code, errOut.String())
	}
	got := strings.Join(strings.Fields(out.String()), " ")

	if !strings.Contains(got, "CHANGED OUTSIDE TERRAFORM") {
		t.Errorf("the drift section is missing:\n%s", out.String())
	}
	// AND NO REASSURANCE ABOVE IT. A report that says there is nothing in this
	// plan, directly above a section listing a database that vanished, is
	// worse than saying nothing at all.
	for _, lie := range []string{
		"No changes. This plan does nothing.",
		"there is nothing in it",
	} {
		if strings.Contains(got, lie) {
			t.Errorf("the report says %q while listing drift:\n%s", lie, out.String())
		}
	}
}

// A filter changes what a reader sees of the findings. Drift is not a finding.
func TestAFilterDoesNotHideTheDriftSection(t *testing.T) {
	var out, errOut bytes.Buffer
	if code := run([]string{"--no-colour", "--min-level", "critical", "../../testdata/drifted.json"},
		strings.NewReader(""), &out, &errOut); code != 0 {
		t.Fatalf("exit = %d: %s", code, errOut.String())
	}
	if !strings.Contains(out.String(), "CHANGED OUTSIDE TERRAFORM") {
		t.Errorf("a filter hid the drift section:\n%s", out.String())
	}
}

// The severity of a drift entry has to be VISIBLE, not merely implied by its
// position. Findings take their level from the section they sit under; drift
// is all in one section, so a database that vanished had no CRITICAL anywhere
// and read exactly like a changed tag.
func TestDriftShowsItsSeverityInEveryHumanFormat(t *testing.T) {
	for _, format := range []string{"terminal", "md", "html"} {
		t.Run(format, func(t *testing.T) {
			var out, errOut bytes.Buffer
			if code := run([]string{"--format", format, "--no-colour",
				"../../testdata/drift-only.json"}, strings.NewReader(""), &out, &errOut); code != 0 {
				t.Fatalf("exit = %d: %s", code, errOut.String())
			}
			if !strings.Contains(out.String(), "CRITICAL") {
				t.Errorf("%s does not show the severity of a database that vanished:\n%s",
					format, out.String())
			}
		})
	}
}
