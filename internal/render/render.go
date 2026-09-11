// Package render turns an assessment into output a person or a machine
// can read. It writes to an io.Writer and does nothing else.
package render

import "github.com/dbhq-uk/terraverdict/internal/assess"

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
