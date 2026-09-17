package render

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/dbhq-uk/terraken/internal/assess"
	tfjson "github.com/hashicorp/terraform-json"
)

// An operation the tool could not assess has to be visible in every format.
// The machine formats matter most: an agent parsing the output is the
// consumer least able to notice a finding that quietly went missing.

func unsupportedReport(t *testing.T) assess.Report {
	t.Helper()
	return loadFixture(t, "unsupported-action.json")
}

func TestEveryFormatNamesAnOperationItCouldNotAssess(t *testing.T) {
	r := unsupportedReport(t)
	if r.Unassessed != 2 {
		t.Fatalf("fixture gave Unassessed = %d, want 2", r.Unassessed)
	}

	formats := map[string]func(*bytes.Buffer) error{
		"terminal": func(b *bytes.Buffer) error {
			return Terminal(b, r, TerminalOptions{Width: testWidth})
		},
		"markdown": func(b *bytes.Buffer) error { return Markdown(b, r) },
		"json":     func(b *bytes.Buffer) error { return JSON(b, r) },
		"html":     func(b *bytes.Buffer) error { return HTML(b, r) },
		"gate":     func(b *bytes.Buffer) error { return Gate(b, r, "") },
	}
	for name, render := range formats {
		t.Run(name, func(t *testing.T) {
			var b bytes.Buffer
			if err := render(&b); err != nil {
				t.Fatalf("%s returned error: %v", name, err)
			}
			out := b.String()
			for _, want := range []string{
				"terraform_data.reconciled",
				"terraform_data.quarantined",
				// The sentence, in every format including the machine ones.
				"does not recognise",
				// The action vocabulary itself, which is what a reader needs
				// in order to go and look it up.
				"reconcile",
				"quarantine",
			} {
				if !strings.Contains(out, want) {
					t.Errorf("%s does not mention %q:\n%s", name, want, out)
				}
			}
			// And never as a no-op, which is the bug this replaced.
			if strings.Contains(out, "no change") {
				t.Errorf("%s still describes an unrecognised operation as no change:\n%s", name, out)
			}
		})
	}
}

// The count has to be stated too. Two findings in the list and a summary
// that tallies one is a report disagreeing with itself.
func TestTheSummaryStatesHowManyFindingsCouldNotBeAssessed(t *testing.T) {
	r := unsupportedReport(t)

	var term bytes.Buffer
	if err := Terminal(&term, r, TerminalOptions{Width: testWidth}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(term.String(), "2 unranked") {
		t.Errorf("the terminal summary does not tally the unranked findings:\n%s", term.String())
	}

	var h bytes.Buffer
	if err := HTML(&h, r); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(h.String(), "2 unranked") {
		t.Errorf("the HTML summary does not tally the unranked findings:\n%s", h.String())
	}
}

// The gate carries them whether or not a gate was asked for - the same
// treatment the exposure already gets, and for the same reason: a caller
// running --format gate with no --fail-on, purely to see what is in the
// plan, still has to be told the tool could not read part of it.
func TestTheGateReportsUnassessedOperationsWithoutAThreshold(t *testing.T) {
	var b bytes.Buffer
	if err := Gate(&b, unsupportedReport(t), ""); err != nil {
		t.Fatal(err)
	}
	var v GateVerdict
	if err := json.Unmarshal(b.Bytes(), &v); err != nil {
		t.Fatalf("gate output is not valid JSON: %v\n%s", err, b.String())
	}
	if len(v.Unsupported) != 2 {
		t.Fatalf("Unsupported = %+v, want 2 entries", v.Unsupported)
	}
	byAddr := map[string][]string{}
	for _, u := range v.Unsupported {
		byAddr[u.Address] = u.Actions
		if u.Detail == "" {
			t.Errorf("%s carries no explanation", u.Address)
		}
	}
	if got := byAddr["terraform_data.quarantined"]; len(got) != 2 ||
		got[0] != `"delete"` || got[1] != `"quarantine"` {
		t.Errorf("actions = %v, want the whole ordered sequence", got)
	}
}

// The verdict is a severity question and an unassessable operation has no
// severity, so it must not move it. That is the promise --fail-on already
// makes, and a caller that wants to stop on this branches on the array -
// one line, and their policy rather than this tool's.
func TestAnUnassessedOperationDoesNotMoveTheVerdict(t *testing.T) {
	r := unsupportedReport(t)
	for _, threshold := range []string{"critical", "high", "low", "info"} {
		var b bytes.Buffer
		if err := Gate(&b, r, threshold); err != nil {
			t.Fatal(err)
		}
		var v GateVerdict
		if err := json.Unmarshal(b.Bytes(), &v); err != nil {
			t.Fatal(err)
		}
		if len(v.Unsupported) != 2 {
			t.Errorf("at %q: the unsupported array must be carried at every threshold", threshold)
		}
		for _, g := range v.Blocking {
			if g.Level == "unranked" {
				t.Errorf("at %q: %s blocked the gate on a severity it does not have", threshold, g.Address)
			}
		}
	}

	// local_file.ordinary is a delete, which is high, so high still fails
	// and critical still passes. The unrankable findings changed neither.
	for _, tc := range []struct{ threshold, want string }{
		{"critical", "pass"},
		{"high", "fail"},
	} {
		var b bytes.Buffer
		if err := Gate(&b, r, tc.threshold); err != nil {
			t.Fatal(err)
		}
		var v GateVerdict
		if err := json.Unmarshal(b.Bytes(), &v); err != nil {
			t.Fatal(err)
		}
		if v.Verdict != tc.want {
			t.Errorf("at %q: verdict %q, want %q", tc.threshold, v.Verdict, tc.want)
		}
	}
}

// A rule can give an unrecognised operation a severity, and a rule can be
// broad - "everything of this type is info" is a reasonable thing for a
// team to write. What it must never do is erase the fact that the tool
// could not read the operation, because the gate's unsupported array is the
// field that says the verdict is incomplete. A team ranking something is
// not a team declaring it understood.
func TestARuleGivingASeverityDoesNotEraseTheCoverageGap(t *testing.T) {
	rs, err := assess.LoadRules(strings.NewReader(`{"rules":[{
		"id":"terraform-data-is-noise","message":"terraform_data changes are routine here",
		"level":"info","when":{"types":["terraform_data"]}}]}`))
	if err != nil {
		t.Fatal(err)
	}

	p := &tfjson.Plan{
		FormatVersion: "1.2",
		ResourceChanges: []*tfjson.ResourceChange{{
			Address:      "terraform_data.reconciled",
			Mode:         tfjson.ManagedResourceMode,
			Type:         "terraform_data",
			Name:         "reconciled",
			ProviderName: "registry.terraform.io/hashicorp/terraform",
			Change:       &tfjson.Change{Actions: tfjson.Actions{"reconcile"}},
		}},
	}
	r := assess.AssessWithRules(p, rs)
	if r.Findings[0].LevelName != "info" {
		t.Fatalf("LevelName = %q, want info - the rule set it", r.Findings[0].LevelName)
	}

	var b bytes.Buffer
	if err := Gate(&b, r, "critical"); err != nil {
		t.Fatal(err)
	}
	var v GateVerdict
	if err := json.Unmarshal(b.Bytes(), &v); err != nil {
		t.Fatal(err)
	}
	if len(v.Unsupported) != 1 {
		t.Fatalf("unsupported = %+v, want the operation still listed", v.Unsupported)
	}
	if got := v.Unsupported[0].Actions; len(got) != 1 || got[0] != `"reconcile"` {
		t.Errorf("actions = %v, want the sequence the plan named", got)
	}
}

// blocking[].paths is documented as attribute paths only, so a caller can
// treat every entry as one. An unrecognised action verb is not an attribute
// path, and the dedupe sorts, so it would arrive both mislabelled and out of
// order. Its own array carries it.
func TestUnrecognisedActionsDoNotLandInTheGatesAttributePaths(t *testing.T) {
	rs, err := assess.LoadRules(strings.NewReader(`{"rules":[{
		"id":"no-unknown-ops","message":"terraken could not assess this operation","level":"critical",
		"when":{"actions":["unsupported"]}}]}`))
	if err != nil {
		t.Fatal(err)
	}
	p := &tfjson.Plan{
		FormatVersion: "1.2",
		ResourceChanges: []*tfjson.ResourceChange{{
			Address:      "terraform_data.quarantined",
			Mode:         tfjson.ManagedResourceMode,
			Type:         "terraform_data",
			Name:         "quarantined",
			ProviderName: "registry.terraform.io/hashicorp/terraform",
			Change:       &tfjson.Change{Actions: tfjson.Actions{"delete", "quarantine"}},
		}},
	}

	var b bytes.Buffer
	if err := Gate(&b, assess.AssessWithRules(p, rs), "critical"); err != nil {
		t.Fatal(err)
	}
	var v GateVerdict
	if err := json.Unmarshal(b.Bytes(), &v); err != nil {
		t.Fatal(err)
	}
	if len(v.Blocking) != 1 {
		t.Fatalf("blocking = %+v, want the rule's finding", v.Blocking)
	}
	if len(v.Blocking[0].Paths) != 0 {
		t.Errorf("paths = %v, want none - these are action names, not attribute paths", v.Blocking[0].Paths)
	}
	if len(v.Blocking[0].Reasons) == 0 {
		t.Error("the reason must still be given")
	}
	if len(v.Unsupported) != 1 || len(v.Unsupported[0].Actions) != 2 {
		t.Errorf("unsupported = %+v, want the action sequence in its own array", v.Unsupported)
	}
}

// An action string is read out of the plan file, which is untrusted input:
// whoever wrote the Terraform chose it, and on a fork pull request that is
// not somebody you trust. This runs the real pipeline - a plan with a
// hostile action in it, through Assess, into every format - rather than
// handing the renderers an already-safe string and proving nothing.
func TestAHostileActionStringCannotReachAnyFormatRaw(t *testing.T) {
	// An ANSI sequence that would erase the screen and repaint the rest of
	// the report, a newline that would end a markdown table row early, and a
	// fence that would close the evidence block it is printed inside.
	const hostile = "\x1b[2J\x1b[31mwipe\nthe|report```"

	p := &tfjson.Plan{
		FormatVersion: "1.2",
		ResourceChanges: []*tfjson.ResourceChange{{
			Address:      "terraform_data.hostile",
			Mode:         tfjson.ManagedResourceMode,
			Type:         "terraform_data",
			Name:         "hostile",
			ProviderName: "registry.terraform.io/hashicorp/terraform",
			Change:       &tfjson.Change{Actions: tfjson.Actions{tfjson.Action(hostile)}},
		}},
	}
	r := assess.Assess(p)
	if r.Unassessed != 1 {
		t.Fatalf("Unassessed = %d, want 1", r.Unassessed)
	}

	// The two JSON formats are listed for completeness, and they prove less
	// than the other three: encoding/json escapes a control character on its
	// own, so those cases would pass with the assess-layer quoting removed.
	// The terminal and markdown cases are the ones that hold the line, and
	// the HTML one covers a destination where an escape is inert but a
	// newline still reflows the document.
	render := map[string]func(*bytes.Buffer) error{
		"terminal": func(b *bytes.Buffer) error {
			return Terminal(b, r, TerminalOptions{Width: testWidth})
		},
		"terminal-colour": func(b *bytes.Buffer) error {
			return Terminal(b, r, TerminalOptions{Width: testWidth, Colour: true})
		},
		"markdown": func(b *bytes.Buffer) error { return Markdown(b, r) },
		"json":     func(b *bytes.Buffer) error { return JSON(b, r) },
		"html":     func(b *bytes.Buffer) error { return HTML(b, r) },
		"gate":     func(b *bytes.Buffer) error { return Gate(b, r, "critical") },
	}
	for name, run := range render {
		t.Run(name, func(t *testing.T) {
			var b bytes.Buffer
			if err := run(&b); err != nil {
				t.Fatalf("%s returned error: %v", name, err)
			}
			out := b.String()

			// The screen-erase and the colour change, neither of which any
			// format has a legitimate reason to carry out of a plan file.
			if strings.Contains(out, "\x1b[2J") || strings.Contains(out, "\x1b[31mwipe") {
				t.Errorf("a raw ANSI sequence from the plan reached %s", name)
			}
			// The newline, which ends a markdown table row and a terminal
			// line early and spills the rest somewhere it was not meant to
			// go.
			if strings.Contains(out, "wipe\nthe") {
				t.Errorf("a raw newline from the plan reached %s", name)
			}
			// It is still reported. Dropping the hostile string would be a
			// different way of saying nothing.
			if !strings.Contains(out, "wipe") {
				t.Errorf("%s dropped the unrecognised action instead of quoting it:\n%s", name, out)
			}
		})
	}
}

// The backtick run is markdown's problem alone, and quoting does not solve
// it: the evidence block has to show the text exactly as the file holds it,
// so the fence has to outrun whatever is inside it. A fenced block is closed
// by a run of at least as many backticks as opened it, and three of them in
// an attribute path - a for_each key is chosen by whoever wrote the
// Terraform - would otherwise end the block and drop the rest of the report
// back into live markdown.
func TestFenceForOutrunsTheLongestBacktickRunInItsContents(t *testing.T) {
	for _, tc := range []struct {
		name  string
		lines []string
		want  string
	}{
		{"ordinary evidence", []string{"tags.Name", "zone"}, "```"},
		{"a single backtick", []string{"tags.`Name`"}, "```"},
		{"exactly a fence", []string{"x[\"```\"]"}, "````"},
		{"longer than a fence", []string{"x[\"``````\"]"}, "```````"},
		{"the run is split across lines", []string{"a``", "b````", "c`"}, "`````"},
		{"backticks are not adjacent", []string{"``a``"}, "```"},
		{"nothing at all", nil, "```"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := fenceFor(tc.lines); got != tc.want {
				t.Errorf("fenceFor(%q) = %q (%d backticks), want %q (%d)",
					tc.lines, got, len(got), tc.want, len(tc.want))
			}
		})
	}
}

// And the same property through the renderer, on the path that actually
// carries an untrusted string today: an attribute path.
func TestAMarkdownEvidenceFenceCannotBeClosedByItsOwnContents(t *testing.T) {
	r := assess.Report{
		Findings: []assess.Finding{{
			Address: "terraform_data.hostile",
			Type:    "terraform_data",
			Kind:    assess.KindUpdate,
			Level:   assess.Low, LevelName: "low",
			Annotations: []assess.Annotation{{
				Code:   assess.AnnUnverifiable,
				Detail: "these values are not known until apply",
				Paths:  []string{"settings[\"```\"]", "# a heading the plan does not justify"},
			}},
		}},
		CountsByName: map[string]int{"low": 1},
	}

	var b bytes.Buffer
	if err := Markdown(&b, r); err != nil {
		t.Fatal(err)
	}
	out := b.String()

	// Walk the evidence block: the opening fence, then every line up to the
	// matching close, none of which may be able to close it.
	lines := strings.Split(out, "\n")
	var fence string
	var closed bool
	for i, l := range lines {
		if fence == "" {
			if strings.HasPrefix(l, "```") {
				fence = l
			}
			continue
		}
		if l == fence {
			closed = true
			break
		}
		if strings.HasPrefix(l, fence) {
			t.Fatalf("line %d closes the evidence fence early:\n%s", i, out)
		}
	}
	if fence == "" {
		t.Fatalf("no evidence fence found in:\n%s", out)
	}
	if !closed {
		t.Fatalf("the evidence fence was never closed:\n%s", out)
	}
	if len(fence) <= 3 {
		t.Errorf("fence = %q; the contents hold three backticks, so it must be longer", fence)
	}
}
