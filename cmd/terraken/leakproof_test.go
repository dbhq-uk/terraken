package main

import (
	"bytes"
	"fmt"
	"io"
	"math/rand"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"unicode"

	"github.com/dbhq-uk/terraken/internal/render"
)

// The proof behind the one guarantee this tool makes.
//
// "No attribute value ever reaches the output, in any format" was four
// hand-written tests and a paragraph in the README. That is a strong claim held
// up by a small amount of evidence, and a reader had no way to tell it apart
// from any other tool's promise. What follows is meant to be the most heavily
// tested thing in the repository.
//
// IT TESTS THE COMMAND, NOT THE RENDERERS. Every case goes through run() with
// real flags, so what is proved is what a person actually gets: every --format,
// with and without colour, with --min-level on, with --fail-on set, through
// --out to a file, and through --moved, which writes HCL somebody redirects
// into their own configuration.
//
// WHAT WOULD MAKE IT FAIL. A renderer that printed a value. A finding type that
// quoted one into a sentence. A credential class that interpolated a prefix or
// a length. The detector in leaks() is proved against a deliberately leaky
// output below, so a green run here is evidence rather than an absence of
// evidence.

// leakWindow is the shortest run of a planted value that counts as a leak.
//
// Twelve characters, compared after whitespace is stripped and case folded. A
// tool that printed less than that has still printed part of a credential, but
// twelve is where a window stops being able to collide with the tool's own
// prose by accident - and every planted value carries a random core, so a
// window of one is not a string anything else could produce.
const leakWindow = 12

// leakSeed makes the run reproducible. A failure is replayable exactly by
// putting the seed the test reports back here.
const leakSeed = 20260917

func TestNoPlantedValueReachesAnyOutput(t *testing.T) {
	rng := rand.New(rand.NewSource(leakSeed)) //nolint:gosec // reproducibility, not secrecy

	var plans, checks int
	for _, pos := range positions {
		for _, sh := range shapes {
			c := core(rng)
			secret := sh.make(c)

			// What counts as a disclosure. The fixed syntax a credential
			// format carries - a PEM header, "sslmode=require" - is planted
			// and rendered like the rest of it, but it is published and
			// everybody knows it, so it is not the secret. Checking windows
			// across it would fail builds over outputs that leaked nothing.
			sensitive := sh.secretPart(c)
			if !strings.Contains(secret, sensitive) {
				t.Fatalf("%s: secretPart is not part of the value it claims to describe", sh.name)
			}

			p := newPlanFile()
			pos.plant(p, secret)
			doc := p.JSON()
			plans++

			// The plant has to have landed. A generator whose position
			// silently wrote nothing would produce a green run proving
			// nothing at all, which is the one way this test could lie.
			if !bytes.Contains(doc, []byte(sensitive[:leakWindow])) &&
				!bytes.Contains(doc, []byte(escapeForJSON(sensitive)[:leakWindow])) {
				t.Fatalf("%s: the generator did not plant the value in the plan it produced:\n%s",
					describe(pos, sh), doc)
			}

			t.Run(describe(pos, sh), func(t *testing.T) {
				for _, inv := range invocations() {
					out := runAgainst(t, doc, inv)
					checks++
					if where, leaked := leaks(out, sensitive); leaked {
						t.Errorf("%s leaked %d characters of the planted %s:\n  %q\n\nfull output:\n%s",
							inv.name, leakWindow, sh.name, where, out)
					}
				}
			})
		}
	}

	// The number the README quotes, stated on every run.
	//
	// Printed rather than logged, so it reaches a CI log without -v. A claim
	// this size should be re-earned on every build and visible where the
	// build is, not asserted once in a paragraph somebody wrote by hand.
	//
	// ONLY WHEN IT PASSED. A failing run used to print the success sentence
	// underneath its own failures, because the line was unconditional - and a
	// summary that congratulates itself on a red build is worse than no
	// summary. The wording is the property actually checked, too: runs of
	// leakWindow characters, not "zero bytes", which this cannot support.
	if t.Failed() {
		return
	}
	var live int
	for _, p := range positions {
		if p.live {
			live++
		}
	}
	fmt.Printf("leak proof: %d generated plans, %d positions (%d of them read by this build), "+
		"%d credential shapes, %d rendered outputs, seed %d - "+
		"no run of %d or more characters of any planted secret reached any of them\n",
		plans, len(positions), live, len(shapes), checks, leakSeed, leakWindow)
}

// invocation is one way of running the command, named for a failure message.
type invocation struct {
	name string
	args []string

	// outFile means the report is written to a path rather than to stdout,
	// and the file is what gets checked. The report is the same report; the
	// point is that --out is a separate write path and a separate chance to
	// get it wrong.
	outFile bool
}

// invocations is every way the report can leave the process.
//
// THE FORMAT LIST COMES FROM THE RENDER PACKAGE, not from a copy here. That is
// what makes "adding a renderer without extending this fails the build" true
// rather than aspirational: a new format has to be added to render.Formats
// before the command will accept it, and the moment it is, every case below
// starts running against it.
func invocations() []invocation {
	var out []invocation
	for _, f := range render.Formats {
		out = append(out,
			invocation{name: "--format " + f, args: []string{"--format", f}},
			// With a gate set. The gate's blocking array carries reasons and
			// paths lifted off annotations, which is a second assembly step
			// and a second chance to lift a value with them.
			invocation{name: "--format " + f + " --fail-on info",
				args: []string{"--format", f, "--fail-on", "info"}},
			// Written to a file rather than to a stream.
			invocation{name: "--format " + f + " --out", args: []string{"--format", f}, outFile: true},
		)
	}
	return append(out,
		// Colour on. Every painted string passes through a second assembly
		// that a plain run does not exercise.
		invocation{name: "--format terminal, colour", args: []string{"--format", "terminal"}},
		invocation{name: "--plain", args: []string{"--plain"}},
		// A filter on. Findings are dropped and counts are rewritten, and a
		// renderer that rebuilt a summary from the wrong side of the filter
		// would be assembling strings again.
		invocation{name: "--min-level high", args: []string{"--min-level", "high"}},
		// HCL somebody redirects into their own configuration. If anything in
		// this tool must not carry a credential, it is the output whose whole
		// purpose is to be pasted into a file and committed.
		invocation{name: "--moved", args: []string{"--moved"}},
		invocation{name: "--moved --out", args: []string{"--moved"}, outFile: true},
	)
}

// runAgainst writes the plan to a temporary file, runs the command over it,
// and returns everything the command produced - stdout, stderr and, when the
// invocation used --out, the file it wrote.
//
// STDERR IS CHECKED TOO. An error message that quoted the value it choked on
// would be a leak like any other, and the stream it went to is not a defence.
func runAgainst(t *testing.T, doc []byte, inv invocation) string {
	t.Helper()

	dir := t.TempDir()
	planPath := filepath.Join(dir, "plan.json")
	if err := os.WriteFile(planPath, doc, 0o600); err != nil {
		t.Fatal(err)
	}

	args := append([]string{}, inv.args...)
	var outPath string
	if inv.outFile {
		outPath = filepath.Join(dir, "report.out")
		args = append(args, "--out", outPath)
	}

	// BOTH COLOUR VARIABLES ARE SET EXPLICITLY, EVERY TIME. Colour is decided
	// by whether the destination is a terminal, and a bytes.Buffer never is,
	// so FORCE_COLOR is the supported way to ask for it - the path a CI log
	// that renders ANSI already uses. NO_COLOR deliberately beats it, which
	// means an inherited NO_COLOR=1 in whoever's shell is running the suite
	// silently turns the colour case into a second plain case. That happened:
	// a whole review pass ran with the colour path never once exercised.
	// Neither variable is left to the environment now.
	wantColour := strings.Contains(inv.name, "colour")
	t.Setenv("NO_COLOR", "")
	if wantColour {
		t.Setenv("FORCE_COLOR", "1")
	} else {
		t.Setenv("FORCE_COLOR", "")
	}
	args = append(args, planPath)

	// EVERYTHING THE PROCESS PRINTS, not only what it was handed a writer
	// for. run() writes to the streams it is given, but a debugging
	// fmt.Printf, a stray os.Stdout.Write or a default-logger line inside
	// assess or render would go straight past them - and a credential
	// printed that way leaks exactly as well as one printed properly, while
	// a harness watching only the injected writers stays green.
	realOut, realErr := os.Stdout, os.Stderr
	rOut, wOut, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	rErr, wErr, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout, os.Stderr = wOut, wErr

	strayOut := make(chan []byte, 1)
	strayErr := make(chan []byte, 1)
	go func() { b, _ := io.ReadAll(rOut); strayOut <- b }()
	go func() { b, _ := io.ReadAll(rErr); strayErr <- b }()

	var stdout, stderr bytes.Buffer
	code := run(args, strings.NewReader(""), &stdout, &stderr)

	wOut.Close()
	wErr.Close()
	os.Stdout, os.Stderr = realOut, realErr
	stray := append(<-strayOut, <-strayErr...)

	// Exit 2 is "the tool could not do its job", which for a generated plan
	// means the generator produced something invalid rather than the tool
	// finding something. Either way the test is not measuring what it thinks.
	if code == 2 {
		t.Fatalf("%s exited 2: %s", inv.name, stderr.String())
	}

	var b strings.Builder
	b.Write(stdout.Bytes())
	b.Write(stderr.Bytes())
	b.Write(stray)
	if outPath != "" {
		written, err := os.ReadFile(outPath)
		if err != nil {
			t.Fatalf("%s: --out named %s and it could not be read: %v", inv.name, outPath, err)
		}
		b.Write(written)
	}

	// The colour case has to have actually produced colour, or it is a second
	// copy of the plain case wearing a different name. A plan the tool finds
	// nothing in is the exception and not a failure: "No changes. This plan
	// does nothing." is one unpainted line by design, and the positions this
	// build does not read produce exactly that.
	out := b.String()
	if wantColour && !strings.Contains(out, "\x1b[") && !strings.Contains(out, "No changes") {
		t.Fatalf("%s produced no ANSI at all, so the colour path was not exercised:\n%s", inv.name, out)
	}
	return out
}

// leaks reports whether any run of leakWindow characters from secret appears
// in output, and returns the offending run.
//
// BOTH SIDES ARE FLATTENED FIRST, and each step of the flattening is there
// because it is a way a credential stays usable while a naive substring check
// stops seeing it:
//
//   - whitespace removed, so a token wrapped across two terminal lines or
//     re-indented inside a JSON document is still one run
//   - case folded, so one upper-cased into a heading is still the same token
//   - ANSI escape sequences removed, so a value with a colour change through
//     the middle of it does not read as two harmless halves
//   - HTML tags removed, for the same reason in the HTML report
//   - backslashes removed, so a value carrying JSON escapes matches the
//     unescaped form somebody would copy out of it
//
// WHAT IT CANNOT SEE, stated because the alternative is a claim larger than
// the evidence: a disclosure shorter than the window, a length or a character
// count derived from a value, and a value split by something this does not
// strip. The property held here is "no run of leakWindow or more characters of
// a planted secret reaches any output" - which is what the README says, rather
// than "no byte", which this could not support.
func leaks(output, secret string) (string, bool) {
	haystack := flatten(output)
	needle := flatten(secret)
	if len(needle) < leakWindow {
		// Nothing to test against. The generator never produces one this
		// short, and a silent pass here would be the test lying.
		panic("a planted value shorter than the leak window: " + secret)
	}
	for i := 0; i+leakWindow <= len(needle); i++ {
		window := needle[i : i+leakWindow]
		if strings.Contains(haystack, window) {
			return window, true
		}
	}
	return "", false
}

var (
	ansiSequence = regexp.MustCompile(`\x1b\[[0-9;?]*[ -/]*[@-~]`)
	htmlTag      = regexp.MustCompile(`<[^<>]*>`)
)

// flatten reduces a string to the characters that carry the secret, so the
// comparison is about the bytes of a credential rather than about how they
// happened to be laid out on the way out of the process.
func flatten(s string) string {
	s = ansiSequence.ReplaceAllString(s, "")
	s = htmlTag.ReplaceAllString(s, "")
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range strings.ToLower(s) {
		if unicode.IsSpace(r) || r == '\\' {
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}

// escapeForJSON is how a value looks once it has been through an encoder, so
// the generator's own sanity check can find a planted value in the document it
// just built even when the value held a quote or a newline.
func escapeForJSON(s string) string {
	q := quoteJSON(s)
	return q[1 : len(q)-1]
}

// TestTheLeakDetectorCatchesALeak is the test for the test.
//
// A green proof is only worth something if the detector could have gone red,
// and "we ran a lot of cases and nothing failed" is exactly the claim that
// needs its own evidence. Each case below is a way a value could plausibly
// reach an output while still passing a naive substring check.
func TestTheLeakDetectorCatchesALeak(t *testing.T) {
	const secret = "ghp_R7tQm2xLvB9nKpZa4WcYeD6sJhF1gU3oNi0T"

	for _, tc := range []struct {
		name   string
		output string
		want   bool
	}{
		{"printed outright", "terraform_data.x\n  config_blob = " + secret, true},
		{"wrapped across two terminal lines",
			"  config_blob  ghp_R7tQm2xLvB9nKpZa4Wc\n  YeD6sJhF1gU3oNi0T", true},
		{"re-indented inside a JSON document",
			"{\n    \"token\":\n      \"ghp_R7tQm2xLvB9nKpZa4WcYeD6sJhF1gU3oNi0T\"\n}", true},
		{"upper-cased into a heading", strings.ToUpper(secret), true},
		{"only the first half", secret[:20], true},
		{"only the last half", secret[20:], true},
		{"exactly the window", secret[4 : 4+leakWindow], true},
		{"one character short of the window", secret[4 : 4+leakWindow-1], false},
		{"the path but not the value", "config_blob\ntoken\napi_key", false},
		{"the tool's own sentence about it",
			"a GitHub token\nan attribute named as a secret, not marked sensitive", false},
		{"a clean report", "terraken  3 findings\nCRITICAL\n  terraform_data.x\n  destroy", false},
		// Split by things that are not whitespace. Each of these is a
		// perfectly recoverable credential that a flatten which only removed
		// spaces would have walked straight past.
		{"split by a colour change",
			"\x1b[31mghp_R7tQm2xLvB9n\x1b[0mKpZa4WcYeD6sJhF1gU3oNi0T", true},
		{"split by an HTML element",
			"<code>ghp_R7tQm2xLvB9n</code><span>KpZa4WcYeD6sJhF1gU3oNi0T</span>", true},
		{"serialised into a JSON document",
			`{"token":"ghp_R7tQm2xLvB9nKpZa4WcYeD6sJhF1gU3oNi0T"}`, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			where, got := leaks(tc.output, secret)
			if got != tc.want {
				t.Fatalf("leaks() = %v (%q), want %v", got, where, tc.want)
			}
		})
	}

	// A value carrying characters JSON has to escape. The escapes are the
	// encoder's, not the value's, so a check that counted them would stop
	// seeing a credential the moment it held a quote - and an attacker
	// copying it out of the document gets the unescaped form back.
	const quoted = `pw"th"tneedsesc"ping-8Kd2mQ`
	if where, got := leaks(quoteJSON(quoted), quoted); !got {
		t.Errorf("a value whose JSON encoding escapes it was missed: %q", where)
	}
}

// The other half of the detector's honesty: it must not cry leak over the
// published syntax a credential format wraps around its secret.
//
// A PEM header and "sslmode=require" are printed in documentation, in this
// repository, and in the tool's own sentences. Checking windows across the
// whole planted value flagged them - "-----BEGIN RSA PRIVATE KEY-----"
// flattens to a run that the harmless class name "a private key" sits inside -
// and a build failed over an output that had disclosed nothing. Each shape
// declares which part is actually the secret, and this holds both halves of
// that: the secret is caught, and the wrapper alone is not.
func TestTheDetectorIgnoresTheSyntaxAroundASecret(t *testing.T) {
	const c = "Rq7TmXLvB9nKpZa4WcYeD6sJhF1gU3oNi0TbVxEw"

	for _, sh := range shapes {
		t.Run(sh.name, func(t *testing.T) {
			value := sh.make(c)
			sensitive := sh.secretPart(c)

			// The secret itself, printed, is a leak.
			if _, got := leaks(value, sensitive); !got {
				t.Errorf("the planted value does not contain its own secret part")
			}

			// The value with its secret cut out is the fixed syntax alone.
			// Nothing in there is a disclosure.
			wrapper := strings.ReplaceAll(value, sensitive, "")
			if wrapper == value {
				t.Fatalf("secretPart is not a substring of the value")
			}
			if where, got := leaks(wrapper, sensitive); got {
				t.Errorf("the published syntax alone reads as a leak: %q", where)
			}

			// And the tool's own class name for this shape is not a leak
			// either, which is the collision that started this.
			for _, sentence := range []string{
				"a private key", "an AWS access key id", "a GitHub token",
				"a connection string with an embedded password", "a JSON Web Token",
				"a long, high-entropy string",
			} {
				if where, got := leaks(sentence, sensitive); got {
					t.Errorf("the class name %q reads as a leak: %q", sentence, where)
				}
			}
		})
	}
}

// TestEveryFormatIsCoveredByTheLeakProof is the load-bearing half of "adding a
// renderer without extending the proof fails the build".
//
// The proof iterates render.Formats. This asserts that the command actually
// accepts every name in it, so the two cannot come apart: a format added to
// the list but not wired into the command fails here, and a format wired into
// the command but not added to the list cannot be reached at all, because the
// command validates against the same list.
func TestEveryFormatIsCoveredByTheLeakProof(t *testing.T) {
	if len(render.Formats) == 0 {
		t.Fatal("render.Formats is empty, so the leak proof is running against nothing")
	}
	for _, f := range render.Formats {
		t.Run(f, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			code := run([]string{"--format", f, "../../testdata/demo.json"},
				strings.NewReader(""), &stdout, &stderr)
			if code == 2 {
				t.Fatalf("the command rejected %q, which render.Formats says it can write: %s",
					f, stderr.String())
			}
			if stdout.Len() == 0 {
				t.Errorf("%q produced no output", f)
			}
		})
	}

	// And the inverse: a name that is not in the list is refused, so the list
	// really is the whole surface rather than a hint.
	var stdout, stderr bytes.Buffer
	if code := run([]string{"--format", "yaml", "../../testdata/demo.json"},
		strings.NewReader(""), &stdout, &stderr); code != 2 {
		t.Errorf("a format outside render.Formats was accepted, exit code %d", code)
	}
	if !strings.Contains(stderr.String(), render.FormatList()) {
		t.Errorf("the error does not list the formats that do work: %q", stderr.String())
	}
}

// TestEveryPlantedPositionIsReadOrSaysItIsNot keeps the proof honest about
// its own size.
//
// A position the tool does not read cannot leak from it, so its cases pass
// without proving anything. That is fine - the value of planting there is that
// the proof is already waiting when a later feature starts reading it - but it
// is only fine if it is stated, because the count in the README would otherwise
// overstate what was actually checked.
//
// So every position declares whether it is live, and this measures it. Two
// positions are not read today, both on the roadmap: prior_state, and
// resource_drift, which item 5 will start reading. When either does, this test
// fails and tells whoever built it to flip the flag - by which time the
// guarantee has already been proved against the position they just opened.
func TestEveryPlantedPositionIsReadOrSaysItIsNot(t *testing.T) {
	rng := rand.New(rand.NewSource(leakSeed)) //nolint:gosec // reproducibility, not secrecy

	for _, pos := range positions {
		t.Run(pos.name, func(t *testing.T) {
			p := newPlanFile()
			// An AWS access key id, which the credential detector recognises
			// on sight. That is what makes this measure VALUE TRAVERSAL
			// rather than the tool merely having noticed the resource: the
			// detector only reports a path if something walked to the value
			// at the end of it and read it.
			pos.plant(p, shapes[0].make(core(rng)))

			out := runAgainst(t, p.JSON(), invocation{
				name: "--format json", args: []string{"--format", "json"},
			})

			// AN EXPOSURE ENTRY, NOT JUST AN ADDRESS ANYWHERE IN THE OUTPUT.
			// Testing for `"address"` was the weaker version and it passed
			// for the wrong reason: every finding carries an address because
			// assessOne copies it off the resource change, whether or not
			// anything looked at a single value. Removing array traversal
			// from walkStrings would have left the array and deeply-nested
			// cases reading as live. `looks_like` only appears when the
			// detector reached the planted value itself.
			read := strings.Contains(out, `"looks_like"`)
			if read != pos.live {
				verb := "is read by the tool, but the generator says it is not"
				if pos.live {
					verb = "is NOT read by the tool, so its cases in the leak proof prove nothing"
				}
				t.Errorf("position %q %s - set live: %v in leakgen_test.go\n\n%s",
					pos.name, verb, read, out)
			}
		})
	}
}

// A guard on the generator rather than on the tool: every position has to have
// a distinct name, or a failure names a case somebody cannot find.
func TestEveryPlantedPositionIsNamedOnce(t *testing.T) {
	seen := map[string]bool{}
	for _, p := range positions {
		if p.name == "" {
			t.Error("a position has no name")
		}
		if seen[p.name] {
			t.Errorf("two positions are both called %q", p.name)
		}
		seen[p.name] = true
	}
	for _, s := range shapes {
		if got := s.make(strings.Repeat("a", 40)); len(flatten(got)) < leakWindow {
			t.Errorf("shape %q produces %q, which is shorter than the leak window", s.name, got)
		}
	}
	fmt.Printf("leak proof covers %d positions x %d credential shapes\n", len(positions), len(shapes))
}
