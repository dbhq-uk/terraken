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
	}
	return "unknown"
}

// ParseLevel converts a user-supplied level name into a Level.
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
