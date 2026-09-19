package main

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
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

	docs := markdownFiles(t)
	for _, c := range claims {
		seen := 0
		for _, doc := range docs {
			b, err := os.ReadFile(doc)
			if err != nil {
				t.Fatalf("cannot read %s: %v", doc, err)
			}
			for _, m := range c.pattern.FindAllStringSubmatch(collapse(string(b)), -1) {
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

// markdownFiles is every document in the repository, found rather than listed.
//
// A HARD-CODED LIST IS A REGISTER THAT GOES STALE, and this one did: the first
// version named README.md and AGENTS.md, so a wrong count in docs/plan-file.md
// or CONTRIBUTING.md passed, and so would one in a document added tomorrow.
// The point of these tests is that a number cannot drift; a list of places to
// look drifts the same way.
func markdownFiles(t *testing.T) []string {
	t.Helper()
	var out []string
	err := filepath.WalkDir("../..", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			switch d.Name() {
			case ".git", "node_modules", "dist", "testdata":
				return filepath.SkipDir
			}
			return nil
		}
		if strings.HasSuffix(path, ".md") {
			out = append(out, path)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("cannot walk the repository: %v", err)
	}
	if len(out) < 4 {
		t.Fatalf("found %d markdown files - the walk has broken", len(out))
	}
	return out
}

// collapse turns every run of whitespace into one space and removes markdown
// emphasis, so a claim broken across an indented line or written as **722**
// still reads as one phrase. Replacing newlines alone left the indent behind,
// and the emphasis markers hid a number from the pattern entirely.
func collapse(s string) string {
	s = strings.NewReplacer("**", "", "*", "", "`", "", "_", "").Replace(s)
	return strings.Join(strings.Fields(s), " ")
}
