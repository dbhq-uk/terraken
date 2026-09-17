package assess

import "fmt"

// Level is the risk level of a finding. There are exactly four, ordered
// least to most severe. There is deliberately no medium: a middle bucket
// is where findings go to be ignored.
type Level int

const (
	Info Level = iota
	Low
	High
	Critical

	// Unranked is the ABSENCE of a severity, not a fifth one. It belongs to
	// a finding the tool could not assess at all - see classify - and it
	// means the ranking does not apply rather than that the ranking came out
	// high.
	//
	// IT IS OUTSIDE THE SEVERITY COUNTS, which design.md decides by name: an
	// action the tool does not recognise is not one resource change losing
	// data, so it belongs outside the counts rather than at the top of them.
	// Report.Unassessed counts it instead, Report.Max steps over it, and
	// --fail-on - a severity threshold and nothing else - therefore never
	// sees it. A team that wants to block on one writes a rule.
	//
	// IT STILL SORTS ABOVE CRITICAL, and that is the only thing its position
	// in this enum buys. Two things fall out of it for free: no --min-level
	// can hide it, and a comparison somebody adds later and forgets to guard
	// errs towards showing it rather than hiding it - which is the failure
	// this whole finding type exists to correct.
	Unranked
)

func (l Level) String() string {
	switch l {
	case Info:
		return "info"
	case Low:
		return "low"
	case High:
		return "high"
	case Critical:
		return "critical"
	case Unranked:
		return "unranked"
	}
	return "unknown"
}

// ParseLevel converts a user-supplied level name into a Level.
//
// "unranked" is deliberately not accepted. It is the tool saying it has no
// severity to give, so --fail-on unranked, --min-level unranked and a rule
// assigning it are all mistakes, and each is better met with an error than
// with a reading somebody did not intend.
func ParseLevel(s string) (Level, error) {
	switch s {
	case "info":
		return Info, nil
	case "low":
		return Low, nil
	case "high":
		return High, nil
	case "critical":
		return Critical, nil
	}
	return Info, fmt.Errorf("unknown level %q: expected one of critical, high, low, info", s)
}
