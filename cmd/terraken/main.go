// Command terraken reads a Terraform or OpenTofu plan and reports, ranked by
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
	"runtime/debug"
	"strings"

	"github.com/dbhq-uk/terraken/internal/assess"
	"github.com/dbhq-uk/terraken/internal/plan"
	"github.com/dbhq-uk/terraken/internal/render"
	tfjson "github.com/hashicorp/terraform-json"
)

// version is the released version, set at link time by goreleaser with
// -X main.version=<tag>. It must stay a package-level var in package
// main: the Go linker silently ignores an -X flag naming a symbol that
// does not exist, so without this declaration every release binary
// builds cleanly and then reports nothing about itself.
var version = "dev"

// buildVersion is what --version prints.
//
// The ldflag only reaches a binary goreleaser built. It does not reach
// one built by "go install github.com/dbhq-uk/terraken/cmd/terraken@v0.1.0",
// which is the install route the README leads with - so the commonest way
// to get this tool produced a binary that could not say which version it
// was. Go records the module version it resolved, so read that when the
// ldflag is absent.
func buildVersion() string {
	if version != "dev" {
		return version
	}
	bi, ok := debug.ReadBuildInfo()
	if !ok || bi.Main.Version == "" || bi.Main.Version == "(devel)" {
		return version
	}
	return strings.TrimPrefix(bi.Main.Version, "v")
}

func main() {
	os.Exit(run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr))
}

// run is the whole program, with its streams injected so it can be
// tested without spawning a process.
func run(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("terraken", flag.ContinueOnError)
	fs.SetOutput(stderr)

	format := fs.String("format", "terminal", "output format: terminal, md, json or html")
	out := fs.String("out", "", "write the report to this file instead of standard output")
	failOn := fs.String("fail-on", "", "exit 1 if any finding reaches this level: critical, high, low or info. Off by default")
	minLevel := fs.String("min-level", "", "only show findings at this level or above: critical, high, low or info. Shows everything by default")
	noColour := fs.Bool("no-colour", false, "disable colour in terminal output")
	// The British spelling is canonical here, for a UK project. The
	// American one is an alias because half the tools people already
	// have in their shell history use it, and guessing wrong should not
	// cost them a failed run.
	noColor := fs.Bool("no-color", false, "alias for --no-colour")
	plain := fs.Bool("plain", false, "no colour and ASCII only, for pipelines and terminals that render box drawing badly")
	showVersion := fs.Bool("version", false, "print the version and exit")
	// --moved replaces the report entirely rather than adding to it. The
	// output is meant to be redirected into a .tf file, so anything else on
	// the stream would land in the reader's configuration.
	moved := fs.Bool("moved", false, "instead of the report, print the moved blocks this plan looks like it forgot, as HCL")

	fs.Usage = func() {
		fmt.Fprintln(stderr, "usage: terraken [flags] <plan.json>")
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
		fmt.Fprintf(stdout, "terraken %s\n", buildVersion())
		return 0
	}

	if fs.NArg() != 1 {
		// Go's flag package stops parsing at the first positional, so
		// "terraken plan.json --format md" leaves the flags sitting in the
		// argument list and exits with bare usage. A Terraform user's
		// mental model is flags anywhere, because "terraform apply
		// tfplan -auto-approve" works. Say what happened rather than
		// leaving them to infer it from a usage block - and say it
		// without reordering argv, which would hide the rule instead of
		// teaching it.
		if fs.NArg() > 1 && strings.HasPrefix(fs.Arg(1), "-") {
			fixed := append(append([]string{}, fs.Args()[1:]...), fs.Arg(0))
			fmt.Fprintf(stderr, "error: flags must come before the file: try terraken %s\n",
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

	// --moved short-circuits every format. It writes HCL and nothing else:
	// a reader doing `terraken --moved plan.json >> main.tf` must not get a
	// risk report in their configuration, and --format has no meaning here
	// because HCL is the only thing a moved block can be written as.
	//
	// It honours --out for the same reason the report does, and with the same
	// 0600: the proposal names every renamed resource in the estate.
	if *moved {
		hcl := assess.RenderMoved(assess.Proposals(report))
		if *out == "" {
			fmt.Fprint(stdout, hcl)
			return 0
		}
		if werr := os.WriteFile(*out, []byte(hcl), 0o600); werr != nil {
			fmt.Fprintf(stderr, "error: %v\n", werr)
			return 2
		}
		fmt.Fprintf(stdout, "wrote %s\n", *out)
		return 0
	}

	if *format != "terminal" && *format != "md" && *format != "json" && *format != "html" {
		fmt.Fprintf(stderr, "error: unknown format %q: expected terminal, md, json or html\n", *format)
		return 2
	}

	// Work out where the report is going before deciding how to set it.
	// Colour and width are both questions about the destination, not
	// about the process: a terminal report written to a file with --out
	// must come out plain and 80 columns wide even when it was launched
	// from a wide, colour-capable terminal.
	dest := stdout
	closeDest := func() error { return nil }
	if *out != "" {
		// 0600 rather than the usual 0644. The report names every
		// resource in the estate, which on a shared CI runner is a map
		// of the infrastructure handed to everyone else with an account
		// on the box. Widen it deliberately if you want to.
		f, ferr := os.OpenFile(*out, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
		if ferr != nil {
			fmt.Fprintf(stderr, "error: %v\n", ferr)
			return 2
		}
		dest, closeDest = f, f.Close
	}

	// --plain is the stronger switch: no colour, and nothing outside
	// ASCII either.
	colour := !*plain && !*noColour && !*noColor && isTTY(dest)

	switch *format {
	case "terminal":
		err = render.Terminal(dest, shown, render.TerminalOptions{Colour: colour, ASCII: *plain})
	case "md":
		err = render.Markdown(dest, shown)
	case "json":
		err = render.JSON(dest, shown)
	case "html":
		err = render.HTML(dest, shown)
	}
	// Close whatever the report went to before reporting success. A
	// write that only fails on close - a full disk is the usual one -
	// must not be announced as a file that was written.
	if cerr := closeDest(); err == nil {
		err = cerr
	}
	if err != nil {
		fmt.Fprintf(stderr, "error: %v\n", err)
		return 2
	}

	// With --out, stdout carries one line and nothing else, so the
	// command stays usable in a script that is doing something with the
	// path afterwards.
	if *out != "" {
		fmt.Fprintf(stdout, "wrote %s\n", *out)
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
	// FORCE_COLOR is the counterpart, and honouring one without the other
	// leaves no way to get a coloured report out of a pipe at all - which
	// a CI log that renders ANSI, a pager held open with less -R, and the
	// script that records this tool's own README demo all need. NO_COLOR
	// still wins above, because turning colour off must never be the
	// setting that loses.
	if v, ok := os.LookupEnv("FORCE_COLOR"); ok && v != "" && v != "0" {
		return true
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
