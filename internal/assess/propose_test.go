package assess

import (
	"strings"
	"testing"

	"github.com/hashicorp/hcl/v2/hclparse"
)

// THE HCL PARSER IS A TEST-ONLY IMPORT and must stay one. The issue asks that
// emitted HCL be asserted to parse rather than assumed to, and the only way to
// assert that honestly is with the parser terraform itself uses - a hand-rolled
// check would only confirm my own idea of the grammar.
//
// It is imported from _test.go files alone, so it is not linked into the
// shipped binary. `go list -deps ./cmd/terraken` is the check if that is ever
// in doubt.
func mustParseHCL(t *testing.T, src string) {
	t.Helper()
	p := hclparse.NewParser()
	_, diags := p.ParseHCL([]byte(src), "proposed.tf")
	if diags.HasErrors() {
		t.Fatalf("emitted HCL does not parse: %s\n---\n%s", diags.Error(), src)
	}
}

func TestProposedBlocksParseAsHCL(t *testing.T) {
	r := Assess(loadConfigured(t, "rename-no-moved.json"))
	out := RenderMoved(Proposals(r))

	mustParseHCL(t, out)
	if !strings.Contains(out, "moved {") {
		t.Fatalf("expected a moved block for a plan with a missed rename:\n%s", out)
	}
}

func TestAnEmptyProposalStillParses(t *testing.T) {
	// The no-findings case is all comments. It must still be valid HCL,
	// because a reader redirecting the output into a file does not check
	// first, and an unparseable .tf breaks their whole configuration rather
	// than just this feature.
	r := Assess(loadConfigured(t, "critical.json"))
	out := RenderMoved(Proposals(r))

	mustParseHCL(t, out)
	if strings.Contains(out, "moved {") {
		t.Fatalf("no rename in this plan, so no block should be proposed:\n%s", out)
	}
}

func TestTheProposalCarriesItsConfidence(t *testing.T) {
	ps := Proposals(Assess(loadConfigured(t, "rename-no-moved.json")))
	if len(ps) == 0 {
		t.Fatal("expected a proposal")
	}
	p := ps[0]
	if p.Compared == 0 {
		t.Fatal("proposal carries no evidence - Compared is zero")
	}
	// The issue: "A pairing the detector is sure about and one it is guessing
	// at must not look identical in the output."
	out := RenderMoved(ps)
	if !strings.Contains(out, "compared attributes identical") {
		t.Fatalf("the rendered block does not state its evidence:\n%s", out)
	}
}

func TestAnAmbiguousPairingIsRefusedRatherThanGuessed(t *testing.T) {
	// local_file.old matches local_file.new_a and local_file.new_b equally -
	// 4 of 4 attributes each. The detector's tiebreak picks new_a because "a"
	// sorts first, and the alphabet is not evidence.
	r := Assess(loadConfigured(t, "ambiguous-move.json"))
	ps := Proposals(r)

	if len(ps) != 1 {
		t.Fatalf("expected one pairing, got %d: %+v", len(ps), ps)
	}
	p := ps[0]
	if !p.Refused() {
		t.Fatalf("an equally good rival exists, so this must refuse: %+v", p)
	}
	if len(p.Rivals) != 1 || p.Rivals[0] != "local_file.new_b" {
		t.Fatalf("expected new_b named as the rival, got %v", p.Rivals)
	}

	out := RenderMoved(ps)
	mustParseHCL(t, out)

	// A refusal is STATED, not silent - and it is not a moved block.
	if strings.Contains(out, "moved {") {
		t.Fatalf("refused pairing must not emit a block:\n%s", out)
	}
	for _, want := range []string{"NOT PROPOSED", "local_file.new_a", "local_file.new_b", "More than one candidate fits"} {
		if !strings.Contains(out, want) {
			t.Fatalf("refusal does not mention %q:\n%s", want, out)
		}
	}
}

func TestEachPairingIsProposedOnceNotTwice(t *testing.T) {
	// The detector files the same annotation under BOTH addresses - the
	// delete's and the create's - so a naive walk of the findings proposes
	// every block twice, and a reader redirecting that into a .tf file gets
	// duplicate moved blocks, which terraform rejects.
	ps := Proposals(Assess(loadConfigured(t, "rename-no-moved.json")))
	seen := map[string]int{}
	for _, p := range ps {
		seen[p.From+"->"+p.To]++
	}
	for k, n := range seen {
		if n != 1 {
			t.Fatalf("pairing %s proposed %d times", k, n)
		}
	}
}

func TestNoAttributeValueReachesTheProposal(t *testing.T) {
	// The tool's one unbreakable guarantee, applied to the new output. A
	// moved block contains addresses, which is permitted; the fixture's
	// values must not appear anywhere in it.
	for _, fx := range []string{"rename-no-moved.json", "ambiguous-move.json", "reordered-secrets.json"} {
		out := RenderMoved(Proposals(Assess(loadConfigured(t, fx))))
		for _, forbidden := range []string{"same everywhere", "0644", "0755", "hunter2", "AKIA"} {
			if strings.Contains(out, forbidden) {
				t.Fatalf("%s: a value reached the proposal output (%q):\n%s", fx, forbidden, out)
			}
		}
	}
}

func TestProposalsAreDeterministic(t *testing.T) {
	r := Assess(loadConfigured(t, "rename-no-moved.json"))
	first := RenderMoved(Proposals(r))
	for i := 0; i < 30; i++ {
		if got := RenderMoved(Proposals(r)); got != first {
			t.Fatalf("run %d differs from the first:\n%s\n---\n%s", i, got, first)
		}
	}
}
