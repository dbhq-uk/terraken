package render

import (
	"fmt"
	"io"
	"strings"

	"github.com/dbhq-uk/terraverdict/internal/assess"
)

// Terminal writes the human-facing report, most severe first.
func Terminal(w io.Writer, r assess.Report, colour bool) error {
	// Nothing found, and nothing held back. A report whose findings were
	// all filtered out must never claim the plan does nothing: it falls
	// through to the summary, which says what was found and how much of
	// it is not being shown.
	if len(r.Findings) == 0 && r.Hidden == 0 {
		_, err := fmt.Fprintln(w, "No changes. This plan does nothing.")
		return err
	}

	for _, f := range r.Findings {
		// Pad the plain label to a fixed width before wrapping it in
		// colour, so every address starts at the same column regardless
		// of level. Padding after wrapping would pad the invisible
		// escape bytes instead of the visible label, and break exactly
		// the alignment this is for in the mode people actually look at.
		label := fmt.Sprintf("%-8s", strings.ToUpper(f.LevelName))
		if colour {
			label = colourFor(f.Level) + label + ansiReset
		}

		if _, err := fmt.Fprintf(w, "%s  %s\n", label, f.Address); err != nil {
			return err
		}
		if _, err := fmt.Fprintf(w, "          %s\n", verb(f.Kind)); err != nil {
			return err
		}
		if f.DataLoss {
			fmt.Fprintf(w, "          this resource type holds data, so destroying it loses that data\n")
		}
		if f.Reason != "" {
			fmt.Fprintf(w, "          because %s\n", f.Reason)
		}
		for _, p := range f.ReplacePaths {
			fmt.Fprintf(w, "          forces replacement: %s\n", p)
		}
		for _, a := range f.Annotations {
			fmt.Fprintf(w, "          %s: %s\n", a.Code, a.Detail)
			for _, p := range a.Paths {
				fmt.Fprintf(w, "            %s\n", p)
			}
		}
		fmt.Fprintln(w)
	}

	n := total(r)
	line := fmt.Sprintf("%d %s: %s", n, plural(n, "finding", "findings"), levelCounts(r))
	if note := hiddenNote(r); note != "" {
		line += " (" + note + ")"
	}
	_, err := fmt.Fprintln(w, line)
	return err
}
