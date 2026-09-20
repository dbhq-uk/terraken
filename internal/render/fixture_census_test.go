package render

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// The fixture census in docs/plan-file.md, counted rather than remembered.
//
// THIS EXACT CLAIM HAS GONE STALE TWICE. It was written by hand, corrected by
// hand, and was wrong again nine fixtures later - every one of the six counts
// understated by the same nine, because nine fixtures were added and nobody
// recounted. A number in a document that nothing checks is a number that
// decays, and this document opens by saying so: "re-run the census before
// quoting the numbers".
//
// So the census is now checked the same way the proof numbers are: the
// sentence states a count, the test counts the files, and a new fixture that
// moves the number fails the build until the sentence catches up.

// censusFields are the plan fields the document counts fixtures for. The
// underscores are stripped to match, because collapse removes them along with
// markdown's other emphasis marker.
var censusFields = []string{
	"complete", "timestamp", "planned_values",
	"variables", "prior_state", "relevant_attributes",
}

func fixtureCensusClaims() []docClaim {
	var out []docClaim
	for _, field := range censusFields {
		field := field
		word := strings.ReplaceAll(field, "_", "")
		out = append(out, docClaim{
			pattern: regexp.MustCompile(`(?i)((?:zero|no|none|one|two|three|four|five|six|seven|` +
				`eight|nine|ten|eleven|twelve|[\d,]+)) carry ` + regexp.QuoteMeta(word) + `\b`),
			want: func() int { return fixturesCarrying(field) },
			what: "fixture count for " + field,
		})
	}
	return out
}

func TestTheFixtureCensusIsCounted(t *testing.T) {
	if n := len(fixtures(t)); n < 20 {
		t.Fatalf("found %d fixtures - the walk has broken", n)
	}
	checkClaims(t, fixtureCensusClaims(), markdownFiles(t)...)
}

// fixturesCarrying counts the fixtures whose top-level object holds this
// field.
//
// A file that does not parse is not counted and is not an error: testdata
// holds malformed.json on purpose, and it is a fixture for the loader rather
// than a plan carrying fields.
func fixturesCarrying(field string) int {
	var n int
	for _, path := range fixturePaths() {
		b, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		var top map[string]json.RawMessage
		if json.Unmarshal(b, &top) != nil {
			continue
		}
		if _, ok := top[field]; ok {
			n++
		}
	}
	return n
}

// fixtures is fixturePaths with a fatal on failure, for the sanity check.
func fixtures(t *testing.T) []string {
	t.Helper()
	paths := fixturePaths()
	if paths == nil {
		t.Fatal("cannot list testdata")
	}
	return paths
}

// fixturePaths is every plan fixture, at the top level of testdata only -
// which is what the document says it counted. testdata/_gen holds the
// Terraform roots the fixtures were generated from, not fixtures.
func fixturePaths() []string {
	paths, err := filepath.Glob("../../testdata/*.json")
	if err != nil {
		return nil
	}
	return paths
}
