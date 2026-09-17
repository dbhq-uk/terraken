package main

import (
	"bytes"
	"encoding/json"
	"regexp"
	"strings"
	"testing"

	"github.com/dbhq-uk/terraken/internal/render"
)

// How much of this could be checked, end to end.

// percentage catches any number followed by a per-cent sign. A list of banned
// phrasings caught "62% reviewable" and missed "62% assessed", which is the
// same verdict wearing different words.
var percentage = regexp.MustCompile(`[0-9]+\s*%`)

func TestCoverageIsReportedInEveryFormat(t *testing.T) {
	for _, format := range render.Formats {
		t.Run(format, func(t *testing.T) {
			var out, errOut bytes.Buffer
			if code := run([]string{"--format", format, "--no-colour",
				"../../testdata/partly-unknowable.json"}, strings.NewReader(""), &out, &errOut); code != 0 {
				t.Fatalf("exit = %d: %s", code, errOut.String())
			}
			got := strings.Join(strings.Fields(out.String()), " ")

			// All five silences, each named separately. A reader who cannot
			// tell "unknown until apply" from "refresh was skipped" has been
			// given one fact where there were two.
			for _, want := range []string{
				"will not know until it applies",
				"records nothing that changed underneath the estate",
				"not expect the state to match",
				"could not be determined",
				"postponed",
			} {
				if !strings.Contains(got, want) {
					t.Errorf("%s does not say %q:\n%s", format, want, out.String())
				}
			}
		})
	}
}

// NO PERCENTAGE ANYWHERE. "62% reviewable" is a verdict wearing a number, and
// the roll-up states a fact rather than ruling.
func TestCoverageNeverReportsAPercentageOrAScore(t *testing.T) {
	for _, fixture := range []string{"partly-unknowable.json", "demo.json", "minimal.json"} {
		for _, format := range render.Formats {
			var out, errOut bytes.Buffer
			if code := run([]string{"--format", format, "--no-colour", "../../testdata/" + fixture},
				strings.NewReader(""), &out, &errOut); code != 0 {
				t.Fatalf("%s/%s exited %d: %s", fixture, format, code, errOut.String())
			}
			// The stylesheet is full of legitimate percentages - width: 100%
			// and the rest - and none of them is a verdict about a plan. The
			// question is about the REPORT, so the style block comes out
			// before it is asked.
			body := out.String()
			if i := strings.Index(body, "<style>"); i >= 0 {
				if j := strings.Index(body, "</style>"); j > i {
					body = body[:i] + body[j:]
				}
			}
			got := strings.ToLower(body)
			// Coverage has to be there at all, or this passes by absence.
			if !strings.Contains(got, "hidden from review") && !strings.Contains(got, "read in full") {
				t.Errorf("%s/%s reports no coverage, so this proves nothing:\n%s",
					fixture, format, out.String())
			}
			// ANY percentage, not a list of phrasings. "62% reviewable" and
			// "62% assessed" are the same verdict wearing different words.
			if percentage.MatchString(got) {
				t.Errorf("%s/%s carries a percentage, which is a verdict rather than a fact:\n%s",
					fixture, format, out.String())
			}
			for _, banned := range []string{"score", "grade", "rating"} {
				if strings.Contains(got, banned) {
					t.Errorf("%s/%s says %q, which is a verdict rather than a fact",
						fixture, format, banned)
				}
			}
		}
	}
}

// A plan with nothing hidden says so, in one line. Silence would leave a
// reader to infer it from an absent section, and "nothing was hidden" and
// "the tool did not look" are the two things this exists to keep apart.
func TestAFullyCheckablePlanSaysSoOutLoud(t *testing.T) {
	var out, errOut bytes.Buffer
	if code := run([]string{"--no-colour", "../../testdata/fully-checkable.json"},
		strings.NewReader(""), &out, &errOut); code != 0 {
		t.Fatalf("exit = %d: %s", code, errOut.String())
	}
	got := strings.Join(strings.Fields(out.String()), " ")
	// The affirmative sentence, which shares no phrase with the negative one -
	// they both used to contain "could be assessed", so this passed on a plan
	// that was only half readable and would have gone on passing if the
	// affirmative path were deleted outright.
	if !strings.Contains(got, "Nothing here is hidden from review") {
		t.Errorf("a plan with nothing hidden must say so:\n%s", out.String())
	}
}

// Coverage counts the WHOLE plan. --min-level changes what a reader sees and
// must not change what the tool says it could check, for the same reason it
// does not change the shape summary.
func TestAFilterDoesNotChangeReportedCoverage(t *testing.T) {
	read := func(args ...string) string {
		var out, errOut bytes.Buffer
		if code := run(append(args, "../../testdata/partly-unknowable.json"),
			strings.NewReader(""), &out, &errOut); code != 0 {
			t.Fatalf("exit = %d: %s", code, errOut.String())
		}
		// The WHOLE coverage object, re-marshalled from the raw message, so
		// nothing is dropped before the comparison - and so a run where
		// coverage disappeared entirely cannot match one where it did not.
		var r struct {
			Coverage json.RawMessage `json:"coverage"`
		}
		if err := json.Unmarshal(out.Bytes(), &r); err != nil {
			t.Fatalf("not valid JSON: %v", err)
		}
		if len(r.Coverage) == 0 {
			t.Fatal("the report carries no coverage at all")
		}
		return string(r.Coverage)
	}

	full := read("--format", "json")
	filtered := read("--format", "json", "--min-level", "critical")
	if full != filtered {
		t.Errorf("coverage changed under a filter:\nfull:     %s\nfiltered: %s", full, filtered)
	}
}

// The machine formats carry it under a stable key, and it does not move the
// verdict - not knowing something is not a severity.
func TestTheGateCarriesCoverageWithoutMovingTheVerdict(t *testing.T) {
	var out, errOut bytes.Buffer
	code := run([]string{"--format", "gate", "--fail-on", "critical",
		"../../testdata/partly-unknowable.json"}, strings.NewReader(""), &out, &errOut)
	if code != 0 {
		t.Fatalf("exit = %d, want 0 - the fixture holds nothing critical: %s", code, out.String())
	}

	var g struct {
		Verdict  string `json:"verdict"`
		Coverage struct {
			Changes int `json:"changes"`
			Gaps    []struct {
				Code   string `json:"code"`
				Detail string `json:"detail"`
				Count  int    `json:"count"`
				Of     int    `json:"of"`
			} `json:"gaps"`
		} `json:"coverage"`
	}
	if err := json.Unmarshal(out.Bytes(), &g); err != nil {
		t.Fatalf("not valid JSON: %v\n%s", err, out.String())
	}
	if g.Verdict != "pass" {
		t.Errorf("verdict = %q; an incomplete assessment is not a severity", g.Verdict)
	}
	if len(g.Coverage.Gaps) != 5 {
		t.Fatalf("got %d gaps, want 5: %+v", len(g.Coverage.Gaps), g.Coverage.Gaps)
	}

	// Every gap carries a stable code and a finished sentence, and the ones
	// that are counts carry a denominator.
	// EXACTLY THESE, EACH ONCE. Membership alone passed on five copies of one
	// code, and on a gap whose count was zero.
	want := map[string]struct{ count, of int }{
		"unknown-until-apply": {1, 2},
		"drift-not-known":     {0, 0},
		"plan-not-complete":   {0, 0},
		"checks-undetermined": {1, 2},
		"changes-deferred":    {1, 0},
	}
	seen := map[string]int{}
	for _, gap := range g.Coverage.Gaps {
		w, known := want[gap.Code]
		if !known {
			t.Errorf("unexpected gap code %q", gap.Code)
			continue
		}
		seen[gap.Code]++
		if gap.Detail == "" {
			t.Errorf("gap %q has no sentence", gap.Code)
		}
		if gap.Count != w.count || gap.Of != w.of {
			t.Errorf("gap %q reports %d of %d, want %d of %d",
				gap.Code, gap.Count, gap.Of, w.count, w.of)
		}
		// A COUNT NEEDS A NAMED DENOMINATOR. Of == 0 is allowed only where
		// the gap is a fact about the whole plan rather than a proportion.
		if gap.Count > 0 && gap.Of == 0 && gap.Code != "changes-deferred" {
			t.Errorf("gap %q counts %d of nothing", gap.Code, gap.Count)
		}
	}
	for code := range want {
		if seen[code] != 1 {
			t.Errorf("gap %q appeared %d times, want exactly once", code, seen[code])
		}
	}
}
