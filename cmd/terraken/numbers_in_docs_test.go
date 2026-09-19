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
		{regexp.MustCompile(`(?i)((?:zero|no|none|one|two|three|four|five|six|seven|eight|nine|ten|eleven|twelve|[\d,]+)) generated plans`), len(positions) * len(shapes), "generated plan count"},
		// The leak line reads "28 positions (18 of them read...)", and the
		// bracket is what tells it from "ten positions are waiting".
		{regexp.MustCompile(`(?i)((?:zero|no|none|one|two|three|four|five|six|seven|eight|nine|ten|eleven|twelve|[\d,]+)) positions \(`), len(positions), "position count"},
		{regexp.MustCompile(`(?i)\(((?:zero|no|none|one|two|three|four|five|six|seven|eight|nine|ten|eleven|twelve|[\d,]+)) of them read by this build\)`), live, "read position count"},
		{regexp.MustCompile(`(?i)((?:zero|no|none|one|two|three|four|five|six|seven|eight|nine|ten|eleven|twelve|[\d,]+)) credential shapes`), len(shapes), "credential shape count"},
		{regexp.MustCompile(`(?i)((?:zero|no|none|one|two|three|four|five|six|seven|eight|nine|ten|eleven|twelve|[\d,]+)) rendered outputs`), len(positions) * len(shapes) * len(invocations()), "rendered output count"},
		{regexp.MustCompile(`(?i)((?:zero|no|none|one|two|three|four|five|six|seven|eight|nine|ten|eleven|twelve|[\d,]+)) of the [\d,]+ positions`), waiting, "unread position count"},
		{regexp.MustCompile(`(?i)((?:zero|no|none|one|two|three|four|five|six|seven|eight|nine|ten|eleven|twelve|[\d,]+)) positions are waiting`), waiting, "unread position count"},
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
	words := map[string]int{
		"zero": 0, "no": 0, "none": 0, "one": 1, "two": 2, "three": 3, "four": 4, "five": 5, "six": 6,
		"seven": 7, "eight": 8, "nine": 9, "ten": 10, "eleven": 11, "twelve": 12,
	}
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
			// ONLY WHAT CANNOT HOLD A DOCUMENT. testdata and dist were
			// skipped, and a false claim under either passed - a directory is
			// skipped because walking it is pointless, never because its
			// contents are assumed right.
			if d.Name() == ".git" {
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
	// Markdown emphasis and link syntax, so `**722**` and
	// `[722](https://example)` both read as the number they state. A claim
	// wearing a link was walking past the pattern entirely.
	// Link syntax in both forms, inline and reference, reduced to its text -
	// a claim wearing a link walked past the pattern entirely. Balanced
	// parentheses inside an inline target are why the target is matched
	// lazily up to the last one on the line rather than the first.
	s = inlineLink.ReplaceAllString(s, "$1")
	s = referenceLink.ReplaceAllString(s, "$1")
	// HTML the document may use for emphasis, and blockquote markers, which
	// otherwise sat between a number and its noun once the lines were joined.
	s = docHTMLTag.ReplaceAllString(s, "")
	s = quoteMarker.ReplaceAllString(s, " ")
	// Emphasis, code ticks, and the two ways a non-breaking space arrives.
	s = strings.NewReplacer(
		"**", "", "*", "", "`", "", "_", "",
		"\u00a0", " ", "&nbsp;", " ", "&#160;", " ", "&#xA0;", " ", "&#xa0;", " ",
		// A shortcut reference link is `[999]` with its definition elsewhere,
		// so the brackets themselves have to go. Inline and full reference
		// links are already reduced to their text above; what is left is a
		// bare pair, and a link definition line becomes harmless text.
		"[", "", "]", "",
	).Replace(s)
	return strings.Join(strings.Fields(s), " ")
}

// The two markdown link forms, reduced to their text.
var (
	inlineLink    = regexp.MustCompile(`\[([^\]]*)\]\((?:[^()\n]|\((?:[^()\n]|\([^()\n]*\))*\))*\)`)
	referenceLink = regexp.MustCompile(`\[([^\]]*)\]\[[^\]]*\]`)
	docHTMLTag    = regexp.MustCompile(`</?[a-zA-Z][^>]*>`)
	quoteMarker   = regexp.MustCompile(`(?m)^\s*>+`)
)
