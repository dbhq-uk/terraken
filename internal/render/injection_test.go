package render

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/dbhq-uk/terraken/internal/assess"
)

// The second guarantee: a report cannot be made to say something other than
// what the plan does.
//
// THE ATTACK IS NOT DEFACEMENT, IT IS REVIEWER DECEPTION. A resource address
// carries a for_each key chosen by whoever wrote the Terraform, and on a fork
// pull request that is not somebody to trust. A report that can be made to
// grow a row, close a table, repaint a terminal or reverse the reading order
// of a line is worse than no report, because it is trusted.
//
// So this tests the STRUCTURE of the output, not the absence of a character.
// Counting escapes proves a payload did not arrive in one particular shape;
// counting rows, sections and findings proves the report still says what the
// report was given.

// fragments are the pieces an attacker has to work with. They are combined
// rather than listed, so the test covers shapes nobody sat down and thought
// of - which is the same reason the leak proof generates positions.
var fragments = []struct {
	name string
	text string
}{
	{"erase the screen", "\x1b[2J"},
	{"repaint in red", "\x1b[31m"},
	{"move the cursor home", "\x1b[1;1H"},
	{"set the window title", "\x1b]0;owned\x07"},
	{"carriage return over the line", "\rall clear"},
	{"a newline", "\nno findings"},
	{"a backspace", "safe\x08\x08\x08\x08"},
	{"a tab", "\tindented"},
	{"a nul", "\x00"},
	{"right-to-left override", "\u202edecaf"},
	{"an isolate that is never popped", "\u2066never popped"},
	{"a zero-width space", "a\u200bb"},
	{"a byte order mark in the middle", "a\ufeffb"},
	{"a table pipe", "| destroy | nothing |"},
	// A LONE BACKTICK, not a matched pair. Two of them pair off around
	// whatever sits between and leave it inside a span, which is inert - so
	// the paired fragment below cannot demonstrate a breakout on its own, and
	// a sabotage run proved it: reverting code spans to a single delimiter
	// left every test green until this fragment existed.
	{"a lone backtick", "`"},
	{"a code span", "`inline`"},
	{"a code fence", "```"},
	{"markdown emphasis", "**approved**"},
	{"a markdown link", "[click](https://example.invalid)"},
	{"a markdown image", "![x](https://example.invalid/x.png)"},
	{"a details element", "<details><summary>clean</summary>"},
	{"closing a details element", "</details>"},
	{"closing the stylesheet", "</style>"},
	{"a script tag", "<script>alert(1)</script>"},
	{"breaking out of an attribute", `" onload="alert(1)`},
	{"closing a code element", "</code><b>spoof</b>"},
	{"a JSON string terminator", `","injected":"`},
	{"a backslash", `\`},
	{"a table row", "\n| CLEAN | create | `x` | nothing |"},
	// Bytes that are not UTF-8 at all. A document is supposed to be UTF-8 and
	// a byte that is not leaves the destination to guess, which is the whole
	// class of problem this guards. The loader cannot produce these today -
	// encoding/json replaces them with U+FFFD while decoding - but Report is a
	// struct a caller fills in, and this package does not get to assume where
	// its input came from.
	{"a lone continuation byte", "\x80"},
	{"a truncated sequence", "\xe2\x80"},
	{"an overlong encoding", "\xc0\xaf"},
	{"a surrogate half", "\xed\xa0\x80"},
}

// hostile wraps fragments in the shape a real address takes, so what is
// tested is a string the tool could genuinely be handed rather than an
// abstract payload.
func hostile(parts ...string) string {
	var b strings.Builder
	b.WriteString(`terraform_data.app["`)
	for _, p := range parts {
		b.WriteString(p)
	}
	b.WriteString(`"]`)
	return b.String()
}

// payloads is every fragment alone, and every ORDERED PAIR of them.
//
// EXHAUSTIVE RATHER THAN RANDOM, and that is a correction rather than a
// preference. This drew a few fragments at random to begin with, and a
// sabotage run found the cost: reverting code spans to a single backtick left
// the tests green, because breaking out of a span needs a backtick AND
// something worth injecting after it, and forty random draws never once put
// those two fragments in the same payload. The interesting attacks are nearly
// all pairs - a delimiter that ends the context, then a payload that acts in
// the one it lands in - so the pairs are the thing to enumerate.
func payloads() []string {
	out := make([]string, 0, len(fragments)*len(fragments)+len(fragments))
	for _, a := range fragments {
		out = append(out, hostile(a.text))
		for _, b := range fragments {
			out = append(out, hostile(a.text, b.text))
		}
	}
	return out
}

// hostileReport fills EVERY plan-derived string on a report with the same
// payload. Each one is a separate sink and any of them is enough.
func hostileReport(payload string) assess.Report {
	return assess.Report{
		TerraformVersion: payload,
		FormatVersion:    payload,
		Findings: []assess.Finding{
			{
				Address: payload, Type: payload, Module: payload, Provider: payload,
				Kind: assess.KindReplace, Level: assess.Critical, LevelName: "critical",
				Reason: payload, ReplacePaths: []string{payload}, DataLoss: true,
				Annotations: []assess.Annotation{
					{Code: assess.AnnUnverifiable, Detail: "not known until apply: " + payload,
						Summary: "unverifiable " + payload, Paths: []string{payload}},
					{Code: assess.AnnMissedMoved, Detail: "a rename, maybe: " + payload,
						Moved: &assess.MovedEvidence{
							From: payload, To: payload + "-b", Matched: 4, Compared: 4,
							CrossModule: true, FromModule: payload, ToModule: payload,
							Rivals: []string{payload},
						}},
					{Code: assess.AnnBlastRadius, Detail: "reaches things: " + payload,
						Summary: "blast radius " + payload, Paths: []string{payload},
						Reached: []assess.Reached{{Address: payload, Depth: 1}}},
					// The CODE itself. It is a package constant in every real
					// report, and that is exactly why it is worth planting in:
					// Report is an ordinary struct a caller fills in, so a
					// string this package did not choose gets no exemption for
					// looking like one it did. The markdown renderer writes it
					// into a raw <summary> line.
					{Code: payload, Detail: "a code from nowhere: " + payload,
						Paths: []string{payload}},
				},
			},
			{
				Address: payload + "-second", Type: payload,
				Kind: assess.KindCreate, Level: assess.Info, LevelName: "info",
			},
			{
				Address: payload + "-third", Type: payload,
				Kind: assess.KindUnsupported, Level: assess.Unranked, LevelName: "unranked",
				Annotations: []assess.Annotation{{
					Code: assess.AnnUnsupportedAction, Detail: "cannot assess: " + payload,
					Summary: "terraken cannot assess this operation", Paths: []string{payload},
				}},
			},
		},
		// UNRANKED as well, so the kind added for an operation this build
		// cannot read is covered by the guarantee too.
		Counts:       map[assess.Level]int{assess.Critical: 1, assess.Info: 1},
		CountsByName: map[string]int{"critical": 1, "info": 1},
		Unassessed:   1,

		// A FILTER IS ON. Hidden and HiddenBelow drive a second summary line
		// and a note in the shape block, both of which are assembled from
		// strings rather than copied.
		Hidden:      3,
		HiddenBelow: "high",

		// TOTAL IS OVER THE SUMMARY FLOOR ON PURPOSE. Shape.Worth() is false
		// below nine findings, so the whole shape block - headline, module
		// list, filter note - was never rendered at all while this said 2, and
		// every sanitised string in it went untested. Two busiest modules for
		// the same reason: one is not enough to make the list print.
		Shape: assess.Shape{
			Total: 12, Headline: "12 changes, mixed - " + payload,
			ByAction: map[assess.Kind]int{assess.KindReplace: 1, assess.KindCreate: 1},
			ByType:   map[string]int{payload: 2},
			ByModule: map[string]int{payload: 2, payload + "-other": 1},
			BusiestModules: []assess.ModuleChurn{
				{Name: payload, Count: 8},
				{Name: payload + "-other", Count: 3},
				{Name: payload + "-third", Count: 1},
			},
		},
		Exposure: assess.Exposure{
			Values: []assess.ExposedValue{{Address: payload, Path: payload, Looks: payload}},
			Note:   assess.ExposureNote,
			Advice: assess.ExposureAdvice,
		},
	}
}

// structureCheck asserts that one format's output still has the shape the
// report implies. clean is the same report rendered with a harmless payload,
// so each check compares a hostile render against a known-good one rather
// than against a hard-coded count somebody has to keep current.
type structureCheck func(t *testing.T, hostileOut, cleanOut string)

// checks is keyed by format name, and TestEveryFormatHasAnInjectionTest
// holds it against render.Formats. That is what makes "adding a renderer
// without an injection test fails the build" true rather than aspirational.
var checks = map[string]structureCheck{
	"terminal": func(t *testing.T, got, clean string) {
		// EVERY ANSI SEQUENCE IN THE OUTPUT IS ONE THIS PACKAGE EMITTED.
		// The renderer paints its own text, so "no escapes at all" would be
		// the wrong assertion; the right one is that the set of sequences
		// present is the set this package knows how to write.
		for _, seq := range ansiSequences(got) {
			if !ourAnsi[seq] {
				t.Errorf("an escape sequence from the plan reached the terminal: %q", seq)
			}
		}
		// THE FURNITURE, NOT THE LINE COUNT. A longer address wraps onto more
		// lines and that is the layout working, so counting lines would fail
		// on a payload that is merely long. What must not change is the
		// number of pieces of report structure: the rules that open and close
		// it, the severity banners, and the tree connector that starts every
		// detail line. A payload that could produce any of those could make
		// the report appear to hold a finding, or a whole section, that the
		// plan does not.
		for _, furniture := range []struct {
			name string
			of   func(string) int
		}{
			{"heavy rules", func(s string) int { return countLinesStarting(s, "━") }},
			{"severity banners", countBanners},
			{"tree branches", func(s string) int { return strings.Count(s, "├ ") }},
			{"tree ends", func(s string) int { return strings.Count(s, "└ ") }},
		} {
			if g, c := furniture.of(got), furniture.of(clean); g != c {
				t.Errorf("got %d %s, want %d - the payload produced report structure",
					g, furniture.name, c)
			}
		}
	},
	"md": func(t *testing.T, got, clean string) {
		// TWO VIEWS, BECAUSE THERE ARE TWO QUESTIONS.
		//
		// `elements` is the document with code fences and inline spans taken
		// out: markup is inert inside those by definition, and the evidence
		// blocks show attribute paths verbatim in a fence on purpose, so
		// counting raw occurrences would report the format working as a
		// breach. Everything from a plan is escaped before it reaches a line
		// out here, so an element counted in this view is one the renderer
		// wrote.
		//
		// `live` takes the HTML blocks out as well, and answers the different
		// question of what markdown will actually interpret.
		elements, cleanElements := markupOutsideCode(got), markupOutsideCode(clean)
		live := liveMarkdown(got)

		// A table row is a line beginning with a pipe. A payload that carried
		// one would grow the table, and a reviewer reading the summary would
		// see a finding the plan does not contain.
		if g, c := countLinesStarting(elements, "|"), countLinesStarting(cleanElements, "|"); g != c {
			t.Errorf("the table has %d rows and should have %d - the payload grew it", g, c)
		}
		for _, tag := range []string{"<details>", "</details>", "<summary>", "</summary>"} {
			g, c := strings.Count(elements, tag), strings.Count(cleanElements, tag)
			if g != c {
				t.Errorf("got %d %s, want %d - the payload changed the structure", g, tag, c)
			}
			if c == 0 {
				t.Errorf("the harmless report has no %s at all, so this check proves nothing", tag)
			}
		}
		// A fence that its own contents can close is a fence that drops the
		// rest of the report back into live markdown.
		assertFencesHold(t, got)
		// And a row that grew a COLUMN loses everything past the header's
		// width, which counting rows cannot see.
		assertTableShapeHolds(t, got)

		// Markup that could be interpreted where it lands. GitHub strips
		// scripts, but it renders emphasis, links and images perfectly well,
		// and any of those is enough to make the report say something else -
		// a bold APPROVED in a Notes cell reads as the tool's own word.
		for _, banned := range []string{
			"<script", "<b>", "<summary>clean", "<style", "<details><summary>",
			"**approved**", "[click](", "![x](",
		} {
			if strings.Contains(live, banned) {
				t.Errorf("%q from the plan is live markup in the markdown", banned)
			}
		}
	},
	"json": func(t *testing.T, got, clean string) {
		var v interface{}
		if err := json.Unmarshal([]byte(got), &v); err != nil {
			t.Fatalf("the payload produced invalid JSON: %v", err)
		}
		// The document being valid is not enough: a consumer decodes it and
		// prints the strings somewhere. What comes back out must be inert.
		assertNoControlCharacters(t, "json", decodedStrings(t, got))
	},
	"html": func(t *testing.T, got, clean string) {
		if n := strings.Count(got, "</style>"); n != 1 {
			t.Errorf("got %d </style>, want 1 - the payload closed the stylesheet", n)
		}
		if strings.Contains(got, "<script") {
			t.Error("a script tag from the plan reached the document")
		}
		for _, tag := range []string{"<article", "<section", "<h2", "<h3", "<li"} {
			if g, c := strings.Count(got, tag), strings.Count(clean, tag); g != c {
				t.Errorf("got %d %s elements, want %d - the payload changed the structure", g, tag, c)
			}
		}
	},
	"gate": func(t *testing.T, got, clean string) {
		var v GateVerdict
		if err := json.Unmarshal([]byte(got), &v); err != nil {
			t.Fatalf("the payload produced invalid gate JSON: %v", err)
		}
		// NOT MERELY "ONE OF THE TWO ALLOWED VALUES". The report holds a
		// critical finding and the threshold is info, so the verdict is fail -
		// and a payload that could flip it to pass would be talking a gate
		// into letting a change through, which is the worst thing on this
		// page. Compared against the harmless render rather than hard-coded.
		var want GateVerdict
		if err := json.Unmarshal([]byte(clean), &want); err != nil {
			t.Fatalf("the harmless gate output is not valid JSON: %v", err)
		}
		if v.Verdict != want.Verdict {
			t.Errorf("verdict = %q, want %q - the payload moved the gate", v.Verdict, want.Verdict)
		}
		if len(v.Blocking) != len(want.Blocking) {
			t.Errorf("blocking has %d entries, want %d", len(v.Blocking), len(want.Blocking))
		}
		if len(v.Unsupported) != len(want.Unsupported) {
			t.Errorf("unsupported has %d entries, want %d", len(v.Unsupported), len(want.Unsupported))
		}
		assertNoControlCharacters(t, "gate", decodedStrings(t, got))
	},
}

func TestNoPlanContentCanChangeTheShapeOfAnyReport(t *testing.T) {
	cleanR := hostileReport(hostile("ordinary"))
	clean := map[string]string{}
	for _, format := range Formats {
		clean[format] = renderTo(t, format, cleanR)
	}

	for i, payload := range payloads() {
		hostileR := hostileReport(payload)
		for _, format := range Formats {
			check, ok := checks[format]
			if !ok {
				t.Fatalf("format %q has no injection check - add one to `checks`", format)
			}
			t.Run(fmt.Sprintf("%s/%d", format, i), func(t *testing.T) {
				got := renderTo(t, format, hostileR)
				check(t, got, clean[format])
				assertInert(t, format, got)
				if t.Failed() {
					t.Logf("payload was %q\n\noutput:\n%s", payload, got)
				}
			})
		}
	}
}

// Every format, including any added later. The map above is keyed by format
// name and this holds it against the registry, so a renderer cannot arrive
// without somebody deciding what "the structure is intact" means for it.
func TestEveryFormatHasAnInjectionTest(t *testing.T) {
	for _, f := range Formats {
		if _, ok := checks[f]; !ok {
			t.Errorf("format %q has no entry in `checks`, so nothing holds its structure "+
				"against hostile plan content", f)
		}
	}
	for name := range checks {
		if !Valid(name) {
			t.Errorf("`checks` has an entry for %q, which is not a format this package writes", name)
		}
	}
}

// assertInert is the blunt half, run beside every structural check rather
// than as a second pass over the same payloads - rendering 800 reports twice
// cost more than it proved.
//
// Nothing this tool writes carries a control character out of a plan. The
// terminal is the sharpest case, being the one destination that ACTS on them,
// but a value that reverses the reading order of a line misleads a reviewer in
// HTML just as well, and a JSON consumer decodes the document before printing
// it somewhere else.
func assertInert(t *testing.T, format, out string) {
	t.Helper()
	assertValidUTF8(t, format, out)
	if format == "json" || format == "gate" {
		assertNoControlCharacters(t, format, decodedStrings(t, out))
		return
	}
	assertNoControlCharacters(t, format, []string{stripOurAnsi(out)})
}

func renderTo(t *testing.T, format string, r assess.Report) string {
	t.Helper()
	var b bytes.Buffer
	if err := Write(&b, format, r, Options{
		Terminal:  TerminalOptions{Width: testWidth, Colour: true},
		Threshold: "info",
	}); err != nil {
		t.Fatalf("%s returned error: %v", format, err)
	}
	return b.String()
}
