package main

import (
	"fmt"
	"os"
	"regexp"
	"strings"
	"testing"
)

// The numbers the documents quote about the leak proof, checked against the
// proof - every occurrence of every claim, in every document that states one.
//
// The README quoted the line the proof prints and said 17 positions read where
// it reports 18. A substring check found the corrected one and walked past the
// stale copy two hundred lines below, which is how the first version of this
// test passed while the document was still wrong.
func TestEveryDocumentedLeakNumberIsTheRealOne(t *testing.T) {
	var live int
	for _, p := range positions {
		if p.live {
			live++
		}
	}
	waiting := len(positions) - live

	claims := []struct {
		pattern *regexp.Regexp
		want    int
		what    string
	}{
		{regexp.MustCompile(`([\d,]+) generated plans`), len(positions) * len(shapes), "generated plan count"},
		{regexp.MustCompile(`([\d,]+) positions`), len(positions), "position count"},
		{regexp.MustCompile(`\(([\d,]+) of them read by this build\)`), live, "read position count"},
		{regexp.MustCompile(`([\d,]+) credential shapes`), len(shapes), "credential shape count"},
		{regexp.MustCompile(`([\d,]+) rendered outputs`), len(positions) * len(shapes) * len(invocations()), "rendered output count"},
		{regexp.MustCompile(`([Ee]leven|[Tt]en|[Nn]ine|\d+) of the [\d,]+ positions`), waiting, "unread position count"},
		{regexp.MustCompile(`([Ee]leven|[Tt]en|[Nn]ine|\d+) positions are waiting`), waiting, "unread position count"},
	}

	docs := []string{"../../README.md", "../../AGENTS.md", "../../docs/roadmap.md", "../../docs/plan-file.md"}
	for _, c := range claims {
		seen := 0
		for _, doc := range docs {
			b, err := os.ReadFile(doc)
			if err != nil {
				t.Fatalf("cannot read %s: %v", doc, err)
			}
			for _, m := range c.pattern.FindAllStringSubmatch(strings.ReplaceAll(string(b), "\n", " "), -1) {
				seen++
				if number(m[1]) != c.want {
					t.Errorf("%s states the %s as %q, and it is %d: %q",
						doc, c.what, m[1], c.want, strings.TrimSpace(m[0]))
				}
			}
		}
		if seen == 0 {
			t.Errorf("no document states the %s, so this check proves nothing", c.what)
		}
	}
}

// number reads a count written as digits or as one of the words the documents
// use for a small one.
func number(s string) int {
	words := map[string]int{"nine": 9, "ten": 10, "eleven": 11}
	if n, ok := words[strings.ToLower(s)]; ok {
		return n
	}
	var n int
	if _, err := fmt.Sscanf(strings.ReplaceAll(s, ",", ""), "%d", &n); err != nil {
		return -1
	}
	return n
}
