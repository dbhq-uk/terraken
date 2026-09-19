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
		{regexp.MustCompile(`([\d,]+) payloads`), count, "payload count"},
		{regexp.MustCompile(`([\d,]+)\s+(?:hostile\s+)?renders`), func() int { return len(payloads()) * len(Formats) }, "hostile render count"},
	}
}

func TestEveryDocumentedInjectionNumberIsTheRealOne(t *testing.T) {
	if n := len(payloads()); n < 100 {
		t.Fatalf("payloads() returned %d - the generator has broken", n)
	}
	checkClaims(t, injectionClaims(), markdownFiles(t)...)
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
			switch d.Name() {
			case ".git", "node_modules":
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
	s = markdownLink.ReplaceAllString(s, "$1")
	s = strings.NewReplacer("**", "", "*", "", "`", "", "_", "", "\u00a0", " ").Replace(s)
	return strings.Join(strings.Fields(s), " ")
}

// markdownLink is `[text](target)`, reduced to its text.
var markdownLink = regexp.MustCompile(`\[([^\]]*)\]\([^)]*\)`)
