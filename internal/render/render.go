// Package render turns an assessment into output a person or a machine
// can read. It writes to an io.Writer and does nothing else.
package render

import (
	"fmt"
	"io"
	"strings"

	"github.com/dbhq-uk/terraken/internal/assess"
)

// errWriter writes lines and remembers the first error, so a renderer
// reads as a sequence of lines rather than as a sequence of error
// checks. Once it has failed it writes nothing more, and the caller
// returns err at the end.
type errWriter struct {
	w   io.Writer
	err error
}

func (e *errWriter) line(s string) {
	if e.err != nil {
		return
	}
	_, e.err = fmt.Fprintln(e.w, s)
}

func (e *errWriter) printf(format string, args ...interface{}) {
	if e.err != nil {
		return
	}
	_, e.err = fmt.Fprintf(e.w, format, args...)
}

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
	case assess.KindNoOp:
		return "no change"
	case assess.KindUnsupported:
		return "operation this build does not recognise"
	}
	// EVERY KIND IS NAMED ABOVE, INCLUDING NO-OP, so that this fallback is
	// reached only by a kind no renderer knows. "no change" used to live
	// here, which meant a kind added to assess and forgotten here was
	// described to a reviewer as nothing at all - the same error the
	// classifier used to make one layer down.
	return "operation this build does not recognise"
}

const (
	ansiReset = "\x1b[0m"
	ansiBold  = "\x1b[1m"
	ansiRed   = "\x1b[31;1m"
	ansiAmber = "\x1b[33;1m"
	ansiBlue  = "\x1b[34m"
	// Bright black rather than the dim attribute, which a fair number of
	// terminals ignore outright.
	ansiGrey = "\x1b[90m"
)

// annotationLabel turns an annotation code into a heading a person can
// read: possible-missed-moved-block becomes "possible missed moved
// block". The code stays the stable identifier; this is only ever used
// for display.
func annotationLabel(code string) string {
	return strings.ReplaceAll(code, "-", " ")
}

func colourFor(l assess.Level) string {
	switch l {
	case assess.Critical:
		return ansiRed
	case assess.High:
		return ansiAmber
	case assess.Low:
		return ansiBlue
	case assess.Unranked:
		// Bold, not red. Red is the severity colour and this is the absence
		// of a severity; borrowing it would say "worse than critical" in the
		// one channel a reader takes in before any of the words.
		return ansiBold
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

// levelTally is one level's share of the whole assessment.
type levelTally struct {
	Level assess.Level
	Count int
}

// tallies is the per-level breakdown as data, most severe first, with
// the levels nothing landed on left out. It counts the whole assessment,
// filter or no filter: a filter changes what you read, not what was
// found. Renderers that set the breakdown differently - colour per level
// in the terminal, a list in HTML - share this rather than each
// rebuilding it.
//
// The unranked tally is read from Report.Unassessed and not from
// CountsByName, because it is deliberately not in there: it is the absence
// of a severity rather than one more of them. It still leads the line. A
// summary that tallied only what the tool managed to rank would be a
// summary that hid how much of the plan it could not read.
func tallies(r assess.Report) []levelTally {
	var out []levelTally
	if r.Unassessed > 0 {
		out = append(out, levelTally{Level: assess.Unranked, Count: r.Unassessed})
	}
	for _, l := range []assess.Level{assess.Critical, assess.High, assess.Low, assess.Info} {
		if n := r.CountsByName[l.String()]; n > 0 {
			out = append(out, levelTally{Level: l, Count: n})
		}
	}
	return out
}
