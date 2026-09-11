// Command tv reads a Terraform or OpenTofu plan and reports, ranked by
// risk, what the change actually does and what it cannot tell you.
//
// It takes a file, or "-" for standard input. It never runs terraform,
// never reads credentials and never makes a network call.
package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/dbhq-uk/terraverdict/internal/assess"
	"github.com/dbhq-uk/terraverdict/internal/plan"
	"github.com/dbhq-uk/terraverdict/internal/render"
	tfjson "github.com/hashicorp/terraform-json"
)

// version is the released version, set at link time by goreleaser with
// -X main.version=<tag>. It must stay a package-level var in package
// main: the Go linker silently ignores an -X flag naming a symbol that
// does not exist, so without this declaration every release binary
// builds cleanly and then reports nothing about itself.
var version = "dev"

func main() {
	os.Exit(run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr))
}

// run is the whole program, with its streams injected so it can be
// tested without spawning a process.
func run(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("tv", flag.ContinueOnError)
	fs.SetOutput(stderr)

	format := fs.String("format", "terminal", "output format: terminal, md or json")
	failOn := fs.String("fail-on", "", "exit 1 if any finding reaches this level: critical, high, low or info. Off by default")
	minLevel := fs.String("min-level", "", "only show findings at this level or above: critical, high, low or info. Shows everything by default")
	noColour := fs.Bool("no-colour", false, "disable colour in terminal output")
	// The British spelling is canonical here, for a UK project. The
	// American one is an alias because half the tools people already
	// have in their shell history use it, and guessing wrong should not
	// cost them a failed run.
	noColor := fs.Bool("no-color", false, "alias for --no-colour")
	showVersion := fs.Bool("version", false, "print the version and exit")

	fs.Usage = func() {
		fmt.Fprintln(stderr, "usage: tv [flags] <plan.json>")
		fmt.Fprintln(stderr, "")
		fmt.Fprintln(stderr, "Use - as the file to read the plan from standard input.")
		fmt.Fprintln(stderr, "")
		fmt.Fprintln(stderr, "Generate the input with:")
		fmt.Fprintln(stderr, "  terraform show -json tfplan > plan.json")
		fmt.Fprintln(stderr, "")
		fs.PrintDefaults()
	}

	if err := fs.Parse(args); err != nil {
		return 2
	}

	// Before the argument check: asking a binary what it is must work
	// without also handing it a plan.
	if *showVersion {
		fmt.Fprintf(stdout, "terraverdict %s\n", version)
		return 0
	}

	if fs.NArg() != 1 {
		// Go's flag package stops parsing at the first positional, so
		// "tv plan.json --format md" leaves the flags sitting in the
		// argument list and exits with bare usage. A Terraform user's
		// mental model is flags anywhere, because "terraform apply
		// tfplan -auto-approve" works. Say what happened rather than
		// leaving them to infer it from a usage block - and say it
		// without reordering argv, which would hide the rule instead of
		// teaching it.
		if fs.NArg() > 1 && strings.HasPrefix(fs.Arg(1), "-") {
			fixed := append(append([]string{}, fs.Args()[1:]...), fs.Arg(0))
			fmt.Fprintf(stderr, "error: flags must come before the file: try tv %s\n",
				strings.Join(fixed, " "))
		}
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

	var floor assess.Level
	filtering := *minLevel != ""
	if filtering {
		var err error
		floor, err = assess.ParseLevel(*minLevel)
		if err != nil {
			fmt.Fprintf(stderr, "error: %v\n", err)
			return 2
		}
	}

	// "-" means standard input, so a plan can go straight from
	// "terraform show -json" into this without ever being written to
	// disk. That matters: plan JSON can hold credentials in the clear,
	// and a file on a runner is one more place for it to be left.
	var p *tfjson.Plan
	var err error
	if fs.Arg(0) == "-" {
		p, err = plan.Read(stdin, "standard input")
	} else {
		p, err = plan.Load(fs.Arg(0))
	}
	if err != nil {
		fmt.Fprintf(stderr, "error: %v\n", err)
		return 2
	}

	report := assess.Assess(p)

	// --min-level filters what is displayed, and nothing else. The
	// unfiltered report is what --fail-on is measured against below: a
	// build must not start passing because someone turned the volume
	// down.
	shown := report
	if filtering {
		shown = report.AtLeast(floor)
	}

	switch *format {
	case "terminal":
		err = render.Terminal(stdout, shown, !*noColour && !*noColor && isTTY(stdout))
	case "md":
		err = render.Markdown(stdout, shown)
	case "json":
		err = render.JSON(stdout, shown)
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
//
// It also answers no when NO_COLOR is set, because that is the same
// question asked from the environment rather than from the stream, and
// every caller of this function is asking it to decide about colour.
func isTTY(w io.Writer) bool {
	// no-color.org: the variable being present and non-empty disables
	// colour, whatever its value. An empty value means nothing, so it
	// must not be treated as opting in either.
	if v, ok := os.LookupEnv("NO_COLOR"); ok && v != "" {
		return false
	}
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
