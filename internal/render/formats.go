package render

import (
	"fmt"
	"io"
	"strings"

	"github.com/dbhq-uk/terraken/internal/assess"
)

// The formats, in ONE registry.
//
// THIS EXISTS SO THAT ADDING A RENDERER CANNOT QUIETLY SKIP THE PROOF. The one
// guarantee this tool makes is that no attribute value reaches the output, in
// any format, and that is held by a generative test which runs every format
// this package will write against plans carrying planted credentials. A format
// the test does not know about is a format the guarantee has not been checked
// against.
//
// It is a map rather than a list beside a switch, and that is the whole point
// of the type. A list and a switch are two registries wearing one name: a
// renderer added to the switch and forgotten in the list leaves the build
// green, and the proof iterates the list. Here there is nowhere to add a
// renderer that the proof does not immediately see.
//
// Options carries everything a format might need beyond the report. Only two
// read any of it; they all take it so that one signature fits the map, which
// is what lets a caller - and the proof - iterate without a switch of its own.
var writers = map[string]func(io.Writer, assess.Report, Options) error{
	"terminal": func(w io.Writer, r assess.Report, o Options) error { return Terminal(w, r, o.Terminal) },
	"md":       func(w io.Writer, r assess.Report, _ Options) error { return Markdown(w, r) },
	"json":     func(w io.Writer, r assess.Report, _ Options) error { return JSON(w, r) },
	"html":     func(w io.Writer, r assess.Report, _ Options) error { return HTML(w, r) },
	"gate":     func(w io.Writer, r assess.Report, o Options) error { return Gate(w, r, o.Threshold) },
}

// order is how the formats are presented to a person: the human ones first,
// the machine ones after. It decides presentation only - `writers` above
// decides what exists - and TestFormatOrderCoversEveryWriter holds the two
// together, so a renderer added to the map without a place in this line is a
// failing build rather than a format missing from the help text.
var order = []string{"terminal", "md", "json", "html", "gate"}

// Formats is every format this package can write, in the order a reader meets
// them. The command validates --format against it and the leak proof iterates
// it, so the two cannot come apart.
var Formats = order

// DefaultFormat is what the command uses when --format is not given.
const DefaultFormat = "terminal"

// Valid reports whether name is a format this package can write.
func Valid(name string) bool {
	_, ok := writers[name]
	return ok
}

// FormatList is the formats as the command shows them to a person, so the
// flag's help and its error message cannot drift apart from each other or from
// what the package can actually write.
func FormatList() string {
	if len(Formats) < 2 {
		return strings.Join(Formats, "")
	}
	return strings.Join(Formats[:len(Formats)-1], ", ") + " or " + Formats[len(Formats)-1]
}

// Options are everything a format might need beyond the report itself.
type Options struct {
	Terminal TerminalOptions

	// Threshold is the level that fails the gate, taken from the invocation.
	// Empty means no gate was asked for. Only the gate format reads it.
	Threshold string
}

// Write renders the report in the named format.
//
// It is the single door every format goes through. The command used to hold
// its own switch over format names, which meant the list of formats existed in
// three places - the flag's help, the validation, and the switch - and a test
// wanting to cover "every format" had to keep a fourth.
func Write(w io.Writer, name string, r assess.Report, opts Options) error {
	write, ok := writers[name]
	if !ok {
		return fmt.Errorf("unknown format %q: expected %s", name, FormatList())
	}
	return write(w, r, opts)
}
