package render

import (
	"fmt"
	"os"
	"regexp"
	"strings"
	"testing"
)

// The numbers the documents quote about the injection proof, checked against
// the proof.
//
// A number a reader can check beats an adjective, and a stale one is worse than
// either: both README.md and AGENTS.md carried 1,122 payloads long after pairs
// of two non-delimiters stopped being generated.
//
// EVERY OCCURRENCE OF EVERY CLAIM, not a substring anywhere in the file. The
// first version of this used strings.Contains, and Astra walked five mutations
// past it: a second correct occurrence concealed a wrong first one, an
// unrelated "1722" satisfied "722", and AGENTS.md's render count was never
// looked at. A document that is right once and wrong twice is a document that
// is wrong.

// docClaim is one phrase the documents use to state a number.
type docClaim struct {
	// pattern captures the number. It is anchored on the words around it so a
	// number belonging to something else cannot satisfy it.
	pattern *regexp.Regexp
	want    func() int
	what    string
}

func injectionClaims() []docClaim {
	count := func() int { return len(payloads()) }
	return []docClaim{
		{regexp.MustCompile(`([\d,]+) payloads`), count, "payload count"},
		{regexp.MustCompile(`([\d,]+)\s+(?:hostile\s+)?renders`), func() int { return len(payloads()) * len(Formats) }, "hostile render count"},
	}
}

func TestEveryDocumentedInjectionNumberIsTheRealOne(t *testing.T) {
	if n := len(payloads()); n < 100 {
		t.Fatalf("payloads() returned %d - the generator has broken", n)
	}
	checkClaims(t, injectionClaims(), "../../README.md", "../../AGENTS.md",
		"../../docs/roadmap.md", "../../docs/design.md")
}

// checkClaims asserts that every number stated for a claim, in every named
// document, is the one the code produces - and that at least one document
// states it, so the test cannot pass by finding nothing.
func checkClaims(t *testing.T, claims []docClaim, docs ...string) {
	t.Helper()
	for _, c := range claims {
		want := c.want()
		seen := 0
		for _, doc := range docs {
			b, err := os.ReadFile(doc)
			if err != nil {
				t.Fatalf("cannot read %s: %v", doc, err)
			}
			text := strings.ReplaceAll(string(b), "\n", " ")
			for _, m := range c.pattern.FindAllStringSubmatch(text, -1) {
				seen++
				got := strings.ReplaceAll(m[1], ",", "")
				if got != fmt.Sprint(want) {
					t.Errorf("%s states the %s as %s, and it is %d: %q",
						doc, c.what, m[1], want, strings.TrimSpace(m[0]))
				}
			}
		}
		if seen == 0 {
			t.Errorf("no document states the %s, so this check proves nothing", c.what)
		}
	}
}
