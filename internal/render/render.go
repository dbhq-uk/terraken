// Package render turns an assessment into output a person or a machine
// can read. It writes to an io.Writer and does nothing else.
package render

import (
	"fmt"
	"strings"

	"github.com/dbhq-uk/terraverdict/internal/assess"
)

// verb describes what the plan does to a resource, in plain words.
func verb(k assess.Kind) string {
	switch k {
	case assess.KindReplace:
		return "destroy and create"
	case assess.KindDelete:
		return "destroy"
	case assess.KindUpdate:
		return "update in place"
	case assess.KindCreate:
		return "create"
	case assess.KindImport:
		return "import, no change"
	case assess.KindRead:
		return "read"
	case assess.KindForget:
		return "remove from state, leave in place"
	}
	return "no change"
}

const (
	ansiReset = "\x1b[0m"
	ansiRed   = "\x1b[31;1m"
	ansiAmber = "\x1b[33;1m"
	ansiBlue  = "\x1b[34m"
	ansiGrey  = "\x1b[90m"
)

func colourFor(l assess.Level) string {
	switch l {
	case assess.Critical:
		return ansiRed
	case assess.High:
		return ansiAmber
	case assess.Low:
		return ansiBlue
	}
	return ansiGrey
}

// plural returns singular when n is 1, and pluralForm otherwise. Shared by
// every renderer's summary line so "1 findings" cannot come back in one
// format after being fixed in another.
func plural(n int, singular, pluralForm string) string {
	if n == 1 {
		return singular
	}
	return pluralForm
}

// total is how many findings the plan produced, which is more than the
// number displayed whenever a filter held some back. Every summary line
// counts the whole assessment: the point of a filter is to read less,
// not to be told less was found.
func total(r assess.Report) int {
	return len(r.Findings) + r.Hidden
}

// hiddenNote is the phrase every format appends when a display filter is
// holding findings back. Showing fewer lines than were found, without
// saying so, is exactly how the one that mattered gets missed, so no
// renderer is allowed to leave it out.
func hiddenNote(r assess.Report) string {
	if r.Hidden == 0 {
		return ""
	}
	return fmt.Sprintf("%d below %s not shown", r.Hidden, r.HiddenBelow)
}

// levelCounts renders the per-level breakdown, most severe first. It
// counts the whole assessment, filter or no filter.
func levelCounts(r assess.Report) string {
	var parts []string
	for _, l := range []assess.Level{assess.Critical, assess.High, assess.Low, assess.Info} {
		if n := r.CountsByName[l.String()]; n > 0 {
			parts = append(parts, fmt.Sprintf("%d %s", n, l.String()))
		}
	}
	return strings.Join(parts, ", ")
}
