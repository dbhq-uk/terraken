package render

import (
	"os"
	"strings"
	"testing"
)

// TestTheDocumentedInjectionCountIsTheRealOne.
//
// The README and AGENTS.md both state how many payloads the injection proof
// runs, because a number a reader can check beats an adjective - and a number
// that has gone stale is worse than either. Both said 1,122, which was the
// count before pairs of two non-delimiters were excluded, and the README also
// said "over a thousand payloads" when the answer is 722.
//
// So the documents are checked against the generator rather than against my
// memory of it. A change to the fragments changes the count, and this fails
// until both files say the new one.
func TestTheDocumentedInjectionCountIsTheRealOne(t *testing.T) {
	count := len(payloads())
	if count < 100 {
		t.Fatalf("payloads() returned %d - the generator has broken", count)
	}
	renders := count * len(Formats)

	for _, doc := range []string{"../../README.md", "../../AGENTS.md"} {
		b, err := os.ReadFile(doc)
		if err != nil {
			t.Fatalf("cannot read %s: %v", doc, err)
		}
		text := string(b)

		if !strings.Contains(text, commas(count)) {
			t.Errorf("%s does not state the payload count %s. The proof runs that many, and "+
				"a stale number in a document about evidence is worse than no number.",
				doc, commas(count))
		}
		// Any number that used to be right and is not any more.
		for _, stale := range []string{"1,122", "1122"} {
			if strings.Contains(text, stale) {
				t.Errorf("%s still says %s payloads", doc, stale)
			}
		}
	}

	b, _ := os.ReadFile("../../README.md")
	if !strings.Contains(string(b), commas(renders)) {
		t.Errorf("the README does not state the render count %s (%d payloads x %d formats)",
			commas(renders), count, len(Formats))
	}
}

// commas renders a count the way the documents write it.
func commas(n int) string {
	s := itoa(n)
	if len(s) <= 3 {
		return s
	}
	var out []byte
	for i, c := range []byte(s) {
		if i > 0 && (len(s)-i)%3 == 0 {
			out = append(out, ',')
		}
		out = append(out, c)
	}
	return string(out)
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	return string(b)
}
