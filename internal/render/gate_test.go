package render

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/dbhq-uk/terraken/internal/assess"
)

func gateOf(t *testing.T, fixture, threshold string) GateVerdict {
	t.Helper()
	var buf bytes.Buffer
	if err := Gate(&buf, loadFixture(t, fixture), threshold); err != nil {
		t.Fatal(err)
	}
	var v GateVerdict
	if err := json.Unmarshal(buf.Bytes(), &v); err != nil {
		t.Fatalf("gate output is not valid JSON: %v\n%s", err, buf.String())
	}
	return v
}

func TestTheVerdictIsOnlyEverPassOrFail(t *testing.T) {
	// The compatibility promise, asserted rather than described. A caller
	// comparing against "fail" must never start silently passing because a
	// third state appeared in this field.
	for _, tc := range []struct {
		fixture, threshold, want string
	}{
		{"critical.json", "critical", "fail"},
		{"critical.json", "high", "fail"},
		{"blast-radius.json", "critical", "pass"},
		{"blast-radius.json", "high", "fail"},
		{"minimal.json", "critical", "pass"},
		// No threshold at all: a caller can run this unconditionally and
		// decide later whether it cared.
		{"critical.json", "", "pass"},
	} {
		got := gateOf(t, tc.fixture, tc.threshold)
		if got.Verdict != tc.want {
			t.Fatalf("%s at %q: verdict %q, want %q", tc.fixture, tc.threshold, got.Verdict, tc.want)
		}
		if got.Verdict != "pass" && got.Verdict != "fail" {
			t.Fatalf("verdict %q is neither pass nor fail", got.Verdict)
		}
	}
}

func TestTheSchemaIsStatedAndVersioned(t *testing.T) {
	v := gateOf(t, "critical.json", "critical")
	if v.Schema != "terraken.gate/v1" {
		t.Fatalf("schema is %q - changing it is a breaking change for every caller parsing this", v.Schema)
	}
}

func TestTheThresholdIsEchoedBackAndComesFromTheInvocation(t *testing.T) {
	// "The threshold is set by the invoking configuration and cannot be
	// altered by anything in the plan being examined."
	//
	// Every fixture is held to the same bar and must report that bar, whatever
	// the plan contains - including large-estate.json, which carries rules,
	// modules and twenty changes and still cannot move it.
	for _, fx := range []string{"critical.json", "large-estate.json", "blast-radius.json", "reordered-secrets.json"} {
		v := gateOf(t, fx, "high")
		if v.Threshold != "high" {
			t.Fatalf("%s: threshold came back %q, not the requested high", fx, v.Threshold)
		}
	}
}

func TestBlockingIsEveryFindingAtOrAboveTheThresholdAndNothingElse(t *testing.T) {
	v := gateOf(t, "large-estate.json", "high")
	if len(v.Blocking) == 0 {
		t.Fatal("expected blocking findings")
	}
	for _, b := range v.Blocking {
		if b.Level != "high" && b.Level != "critical" {
			t.Fatalf("%s is %q, below the high threshold, but is listed as blocking", b.Address, b.Level)
		}
	}
	// And the counts still describe the WHOLE plan, not just the blocking
	// part - a caller needs to know what it is not being stopped for.
	total := 0
	for _, n := range v.Counts {
		total += n
	}
	if total <= len(v.Blocking) {
		t.Fatalf("counts (%d) should cover the whole plan, blocking is %d", total, len(v.Blocking))
	}
}

func TestBlockingIsNeverNull(t *testing.T) {
	// A nil slice marshals as null, and a caller iterating it fails on the one
	// plan whose answer is good news.
	var buf bytes.Buffer
	if err := Gate(&buf, loadFixture(t, "minimal.json"), "critical"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), `"blocking": []`) {
		t.Fatalf("a passing gate should emit an empty array, not null:\n%s", buf.String())
	}
}

func TestPathsAreAttributePathsAndDependsAreAddresses(t *testing.T) {
	// The blast-radius annotation carries RESOURCE ADDRESSES in its Paths,
	// which is right for the human renderer and wrong for a contract: a caller
	// parsing paths as attribute paths would be handed "terraform_data.app"
	// with no way to tell it from "tags.Name".
	v := gateOf(t, "blast-radius.json", "high")
	if len(v.Blocking) == 0 {
		t.Fatal("expected a blocking finding")
	}
	b := v.Blocking[0]
	if len(b.Depends) == 0 {
		t.Fatal("expected the blast radius in depends")
	}
	for _, p := range b.Paths {
		if strings.HasPrefix(p, "terraform_data.") {
			t.Fatalf("a resource address leaked into paths: %q", p)
		}
	}
	for _, d := range b.Depends {
		if !strings.Contains(d, ".") {
			t.Fatalf("depends should hold resource addresses, got %q", d)
		}
	}
}

func TestTheGateNeverProposesAChange(t *testing.T) {
	// "Guidance in the output that is actionable without being suggestive:
	// name what failed and where, never propose the change that would make the
	// gate pass." A gate that tells an agent how to get past it has been
	// talked past.
	for _, fx := range []string{"critical.json", "rename-no-moved.json", "large-estate.json"} {
		v := gateOf(t, fx, "low")
		for _, b := range v.Blocking {
			for _, r := range b.Reasons {
				low := strings.ToLower(r)
				for _, suggestive := range []string{"you should", "try ", "consider ", "instead, ", "to fix", "add a moved block", "moved {"} {
					if strings.Contains(low, suggestive) {
						t.Fatalf("%s: a reason proposes a change: %q", fx, r)
					}
				}
			}
		}
	}
}

func TestNoAttributeValueReachesTheGate(t *testing.T) {
	// An agent's context window is not a safe place for a credential, and the
	// machine mode is the one most likely to be piped somewhere it is kept.
	for _, fx := range []string{"reordered-secrets.json", "large-estate.json", "written-differently.json"} {
		var buf bytes.Buffer
		if err := Gate(&buf, loadFixture(t, fx), "info"); err != nil {
			t.Fatal(err)
		}
		out := buf.String()
		for _, forbidden := range []string{"hunter2", "AKIA", "same everywhere", "0644", "10.0.0.0/16"} {
			if strings.Contains(out, forbidden) {
				t.Fatalf("%s: a value reached the gate output (%q)", fx, forbidden)
			}
		}
	}
}

func TestTheGateIsDeterministic(t *testing.T) {
	var first string
	for i := 0; i < 30; i++ {
		var buf bytes.Buffer
		if err := Gate(&buf, loadFixture(t, "large-estate.json"), "high"); err != nil {
			t.Fatal(err)
		}
		if i == 0 {
			first = buf.String()
			continue
		}
		if buf.String() != first {
			t.Fatalf("run %d differs from the first", i)
		}
	}
}

func TestAnUnparseableThresholdDoesNotSilentlyBlockEverything(t *testing.T) {
	// A typo'd threshold must not be read as "block on everything" nor as a
	// crash. The CLI validates --fail-on before reaching here; this is the
	// renderer's own floor, and it reports no gate rather than inventing one.
	v := gateOf(t, "critical.json", "sever")
	if v.Verdict != "pass" || v.Threshold != "" || len(v.Blocking) != 0 {
		t.Fatalf("an unknown threshold should report no gate, got %+v", v)
	}
	_ = assess.Critical
}
