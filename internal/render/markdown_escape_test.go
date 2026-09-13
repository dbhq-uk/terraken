package render

import (
	"bytes"
	"strings"
	"testing"

	"github.com/dbhq-uk/terraverdict/internal/assess"
)

// A resource address can carry a for_each key chosen by whoever wrote the
// Terraform, and an annotation quotes that address back into the Notes
// column, which is live markdown. Escape it there.
func TestMarkdownEscapesHTMLInNotesButNotInCodeSpans(t *testing.T) {
	evil := `aws_s3_bucket.x["</code><b>spoof</b>"]`
	r := assess.Report{
		Findings: []assess.Finding{{
			Address: evil, Kind: assess.KindDelete,
			Level: assess.High, LevelName: "high",
			Annotations: []assess.Annotation{{
				Code:   assess.AnnMissedMoved,
				Detail: "something about " + evil + " being destroyed",
			}},
		}},
		CountsByName: map[string]int{"high": 1},
	}
	var b bytes.Buffer
	if err := Markdown(&b, r); err != nil {
		t.Fatalf("Markdown returned error: %v", err)
	}
	out := b.String()

	// Split the row at the backticked Resource cell. Inside it is a code
	// span, where HTML is inert and must stay literal. Everything after
	// it is live markdown and must be escaped.
	_, notes, found := strings.Cut(out, "`"+evil+"`")
	if !found {
		t.Fatalf("the backticked address must NOT be escaped:\n%s", out)
	}
	if strings.Contains(notes, "<b>spoof</b>") {
		t.Errorf("raw HTML survived into the Notes column:\n%s", notes)
	}
	if !strings.Contains(notes, "&lt;b&gt;spoof&lt;/b&gt;") {
		t.Errorf("expected the Notes cell to be HTML-escaped:\n%s", notes)
	}
}
