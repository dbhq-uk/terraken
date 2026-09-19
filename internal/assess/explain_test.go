package assess

import (
	"os"
	"regexp"
	"strings"
	"testing"
)

// declaredCodes is every annotation code this package declares, read from
// EVERY FILE IN IT rather than from finding.go alone.
//
// Reading one file was a register with a hole in it: Astra declared
// AnnReviewProbe in another file, emitted it, and both tests passed. Annotation
// codes live in finding.go by convention and a convention is not a check -
// the sequencing and changed-attribute codes could as easily have gone in their
// own files, and one of them nearly did.
func declaredCodes(t *testing.T) []string {
	t.Helper()
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("cannot read the package directory: %v", err)
	}
	pattern := regexp.MustCompile(`Ann[A-Za-z]+\s*=\s*"([a-z-]+)"`)
	var out []string
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".go") || strings.HasSuffix(e.Name(), "_test.go") {
			continue
		}
		b, err := os.ReadFile(e.Name())
		if err != nil {
			t.Fatalf("cannot read %s: %v", e.Name(), err)
		}
		for _, m := range pattern.FindAllStringSubmatch(string(b), -1) {
			out = append(out, m[1])
		}
	}
	return out
}

// TestEveryCodeIsExplained is the "one source" the issue asks for, enforced.
//
// It reads the constants out of the source rather than listing them here,
// because a list here is a second register and two registers wearing one name
// is how they drift. A code added to finding.go without an explanation fails
// the build, which is the acceptance the issue states.
func TestEveryCodeIsExplained(t *testing.T) {
	declared := declaredCodes(t)
	if len(declared) < 10 {
		t.Fatalf("found %d annotation codes - the parse has broken", len(declared))
	}
	for _, m := range declared {
		if _, ok := Explain(m); !ok {
			t.Errorf("the annotation code %q has no explanation. Add one to explain.go: a "+
				"reader meeting it in a report has nowhere else to look.", m)
		}
	}

	// Every level too, including the one that is the absence of a level.
	for _, l := range []Level{Info, Low, High, Critical, Unranked} {
		if _, ok := Explain(l.String()); !ok {
			t.Errorf("the level %q has no explanation", l)
		}
	}
}

// TestNothingIsExplainedThatCannotBePrinted is the other direction. An
// explanation for a code the tool cannot emit is a lie in the documentation,
// and it is the failure the site's own test guards against for the same list.
func TestNothingIsExplainedThatCannotBePrinted(t *testing.T) {
	known := map[string]bool{}
	for _, c := range declaredCodes(t) {
		known[c] = true
	}
	for _, l := range []Level{Info, Low, High, Critical, Unranked} {
		known[l.String()] = true
	}
	for _, code := range ExplainableCodes() {
		if !known[code] {
			t.Errorf("%q is explained and nothing can emit it", code)
		}
	}
}

// TestAnExplanationSaysWhereItStops. The first sentence says what the finding
// is; the rest says what it does NOT claim. That second half is the reason the
// feature exists - a code a reader half-remembers is more dangerous than one
// they look up - so an explanation that is only a definition is not finished.
func TestAnExplanationSaysWhereItStops(t *testing.T) {
	limits := []string{
		"not", "never", "rather than", "cannot", "stops", "floor", "only",
		"incomplete", "heuristic", "yours to judge",
	}
	for _, code := range ExplainableCodes() {
		text, _ := Explain(code)
		if len(text) < 80 {
			t.Errorf("%q: the explanation is %d characters. It has to say what the finding is "+
				"AND where it stops.", code, len(text))
		}
		low := strings.ToLower(text)
		found := false
		for _, w := range limits {
			if strings.Contains(low, w) {
				found = true
			}
		}
		if !found {
			t.Errorf("%q: the explanation states what the finding is and never what it does "+
				"not claim: %q", code, text)
		}
	}
}

// TestNoExplanationPrintsAValue. These are the tool's own sentences and
// --explain opens no file, so there is nothing a plan could have put here - but
// a future explanation quoting a fixture would be exactly the kind of example
// that reads well and breaks the guarantee.
func TestNoExplanationPrintsAValue(t *testing.T) {
	for _, code := range ExplainableCodes() {
		text, _ := Explain(code)
		for _, shape := range []string{"AKIA", "-----BEGIN", "ghp_", "xoxb-", "eyJ"} {
			if strings.Contains(text, shape) {
				t.Errorf("%q: the explanation contains %q", code, shape)
			}
		}
	}
}
