// Command tv reads a Terraform or OpenTofu plan and reports, ranked by
// risk, what the change actually does and what it cannot tell you.
//
// It takes a file. It never runs terraform, never reads credentials and
// never makes a network call.
package main

import (
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/dbhq-uk/terraverdict/internal/assess"
	"github.com/dbhq-uk/terraverdict/internal/plan"
	"github.com/dbhq-uk/terraverdict/internal/render"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

// run is the whole program, with its streams injected so it can be
// tested without spawning a process.
func run(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("tv", flag.ContinueOnError)
	fs.SetOutput(stderr)

	format := fs.String("format", "terminal", "output format: terminal, md or json")
	failOn := fs.String("fail-on", "", "exit 1 if any finding reaches this level: critical, high, low or info. Off by default")
	noColour := fs.Bool("no-colour", false, "disable colour in terminal output")

	fs.Usage = func() {
		fmt.Fprintln(stderr, "usage: tv [flags] <plan.json>")
		fmt.Fprintln(stderr, "")
		fmt.Fprintln(stderr, "Generate the input with:")
		fmt.Fprintln(stderr, "  terraform show -json tfplan > plan.json")
		fmt.Fprintln(stderr, "")
		fs.PrintDefaults()
	}

	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() != 1 {
		fs.Usage()
		return 2
	}

	var threshold assess.Level
	enforcing := *failOn != ""
	if enforcing {
		var err error
		threshold, err = assess.ParseLevel(*failOn)
		if err != nil {
			fmt.Fprintf(stderr, "error: %v\n", err)
			return 2
		}
	}

	p, err := plan.Load(fs.Arg(0))
	if err != nil {
		fmt.Fprintf(stderr, "error: %v\n", err)
		return 2
	}

	report := assess.Assess(p)

	switch *format {
	case "terminal":
		err = render.Terminal(stdout, report, !*noColour && isTTY(stdout))
	case "md":
		err = render.Markdown(stdout, report)
	case "json":
		err = render.JSON(stdout, report)
	default:
		fmt.Fprintf(stderr, "error: unknown format %q: expected terminal, md or json\n", *format)
		return 2
	}
	if err != nil {
		fmt.Fprintf(stderr, "error: %v\n", err)
		return 2
	}

	if enforcing {
		if max, any := report.Max(); any && max >= threshold {
			return 1
		}
	}
	return 0
}

// isTTY reports whether w is a terminal, so colour is only emitted when
// a person is going to see it.
func isTTY(w io.Writer) bool {
	f, ok := w.(*os.File)
	if !ok {
		return false
	}
	info, err := f.Stat()
	if err != nil {
		return false
	}
	return info.Mode()&os.ModeCharDevice != 0
}
