package main

import (
	"fmt"
	"os"
	"strings"
	"testing"
)

// TestTheDocumentedLeakProofLineIsTheRealOne.
//
// The README quotes the line the leak proof prints, because quoting something
// a reader can reproduce beats describing it. It had gone stale: the proof
// reads 18 positions and the README said 17, which is the smallest possible
// way to be wrong about evidence and still exactly the wrong kind.
//
// The line is rebuilt here from the same values the proof uses, so the README
// has to carry today's numbers rather than the day they were written.
func TestTheDocumentedLeakProofLineIsTheRealOne(t *testing.T) {
	var live int
	for _, p := range positions {
		if p.live {
			live++
		}
	}

	b, err := os.ReadFile("../../README.md")
	if err != nil {
		t.Fatalf("cannot read the README: %v", err)
	}
	text := string(b)

	for _, want := range []string{
		fmt.Sprintf("%d positions", len(positions)),
		fmt.Sprintf("%d of them read by this build", live),
		fmt.Sprintf("%d credential shapes", len(shapes)),
	} {
		if !strings.Contains(text, want) {
			t.Errorf("the README does not say %q. It quotes the line the proof prints, so a "+
				"number that has moved makes the quote a fabrication.", want)
		}
	}

	// And the count of what is NOT read, which AGENTS.md states in words.
	a, err := os.ReadFile("../../AGENTS.md")
	if err != nil {
		t.Fatalf("cannot read AGENTS.md: %v", err)
	}
	waiting := len(positions) - live
	if !strings.Contains(string(a), fmt.Sprintf("%d positions are waiting", waiting)) &&
		!strings.Contains(strings.ToLower(string(a)), fmt.Sprintf("%s positions are waiting", spell(waiting))) {
		t.Errorf("AGENTS.md does not say %d positions are waiting", waiting)
	}
}

// spell is the small numbers the documents write in words.
func spell(n int) string {
	words := map[int]string{
		8: "eight", 9: "nine", 10: "ten", 11: "eleven", 12: "twelve",
	}
	if w, ok := words[n]; ok {
		return w
	}
	return fmt.Sprint(n)
}
