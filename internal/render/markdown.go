package render

import (
	"fmt"
	"io"
	"strings"

	"github.com/dbhq-uk/terraverdict/internal/assess"
)

// Markdown writes a table suitable for a pull request comment.
func Markdown(w io.Writer, r assess.Report) error {
	if len(r.Findings) == 0 {
		_, err := fmt.Fprintln(w, "No changes. This plan does nothing.")
		return err
	}

	fmt.Fprintln(w, "| Level | Change | Resource | Notes |")
	fmt.Fprintln(w, "|---|---|---|---|")
	for _, f := range r.Findings {
		var notes []string
		if f.DataLoss {
			notes = append(notes, "holds data")
		}
		if f.Reason != "" {
			notes = append(notes, f.Reason)
		}
		for _, p := range f.ReplacePaths {
			notes = append(notes, "forces replacement: `"+p+"`")
		}
		for _, a := range f.Annotations {
			notes = append(notes, a.Code)
		}
		fmt.Fprintf(w, "| %s | %s | `%s` | %s |\n",
			strings.ToUpper(f.LevelName), verb(f.Kind), f.Address,
			strings.Join(notes, "; "))
	}
	_, err := fmt.Fprintf(w, "\n%d findings.\n", len(r.Findings))
	return err
}
