package render

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
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
		{regexp.MustCompile(`(?i)((?:zero|no|none|one|two|three|four|five|six|seven|eight|nine|ten|eleven|twelve|[\d,]+)) payloads`), count, "payload count"},
		{regexp.MustCompile(`(?i)((?:zero|no|none|one|two|three|four|five|six|seven|eight|nine|ten|eleven|twelve|[\d,]+))\s+(?:hostile\s+)?renders`), func() int { return len(payloads()) * len(Formats) }, "hostile render count"},
	}
}

func TestEveryDocumentedInjectionNumberIsTheRealOne(t *testing.T) {
	if n := len(payloads()); n < 100 {
		t.Fatalf("payloads() returned %d - the generator has broken", n)
	}
	checkClaims(t, injectionClaims(), markdownFiles(t)...)
}

// number reads a count written as digits or as a word.
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
			text := collapse(string(b))
			for _, m := range c.pattern.FindAllStringSubmatch(text, -1) {
				seen++
				if number(m[1]) != want {
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

// markdownFiles is every document in the repository, found rather than listed.
//
// A HARD-CODED LIST IS A REGISTER THAT GOES STALE, and this one did: the first
// version named README.md and AGENTS.md, so a wrong count in docs/plan-file.md
// or CONTRIBUTING.md passed, and so would one in a document added tomorrow.
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
// still reads as one phrase.
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
	s = htmlTag.ReplaceAllString(s, "")
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
	htmlTag       = regexp.MustCompile(`</?[a-zA-Z][^>]*>`)
	quoteMarker   = regexp.MustCompile(`(?m)^\s*>+`)
)
