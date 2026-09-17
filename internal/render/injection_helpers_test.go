package render

import (
	"encoding/json"
	"regexp"
	"strings"
	"testing"
	"unicode"
	"unicode/utf8"
)

// Helpers for the injection tests. They are deliberately strict: a helper
// that quietly passed would make the whole file decorative.

// ourAnsi is every escape sequence this package writes. The terminal check
// asserts that the set of sequences in an output is a subset of this, which
// is the only form of "no injected escapes" that means anything in a renderer
// that emits escapes of its own.
var ourAnsi = map[string]bool{
	ansiReset: true,
	ansiBold:  true,
	ansiRed:   true,
	ansiAmber: true,
	ansiBlue:  true,
	ansiGrey:  true,
}

// anyAnsi matches a CSI sequence, an OSC sequence and a bare two-character
// escape. Wider than what this package emits on purpose: the question is what
// is IN the output, not what this package would have written.
var anyAnsi = regexp.MustCompile(`\x1b(?:\[[0-9;?]*[ -/]*[@-~]|\][^\x07\x1b]*(?:\x07|\x1b\\)|[@-Z\\-_])`)

func ansiSequences(s string) []string {
	found := anyAnsi.FindAllString(s, -1)
	seen := map[string]bool{}
	var out []string
	for _, f := range found {
		if seen[f] {
			continue
		}
		seen[f] = true
		out = append(out, f)
	}
	return out
}

// stripOurAnsi removes the sequences this package legitimately writes, so
// what is left can be checked for control characters without the renderer's
// own colour tripping it.
func stripOurAnsi(s string) string {
	for seq := range ourAnsi {
		s = strings.ReplaceAll(s, seq, "")
	}
	return s
}

// assertNoControlCharacters is the blunt check, and the list of what counts
// is the point of it.
//
// A newline and a space are the renderer's own layout and are allowed. Every
// other C0 control, DEL, the C1 range, and the Unicode format characters that
// reorder or hide text are not - a right-to-left override in an address makes
// a line read backwards to a human while comparing equal to a machine, which
// is reviewer deception with no escape sequence involved at all.
func assertNoControlCharacters(t *testing.T, where string, subjects []string) {
	t.Helper()
	for _, s := range subjects {
		for i, r := range s {
			if !isBanned(r) {
				continue
			}
			t.Errorf("%s carries %s at offset %d: %q",
				where, describeRune(r), i, excerpt(s, i))
		}
	}
}

func isBanned(r rune) bool {
	switch r {
	case '\n', '\t':
		// The renderers use both for layout. A tab from a plan is
		// neutralised before it gets here, which the tests above cover by
		// asserting the STRUCTURE rather than by banning the character.
		return false
	}
	if r == 0x7f || (r < 0x20) || (r >= 0x80 && r <= 0x9f) {
		return true
	}
	switch r {
	// Bidirectional overrides and isolates: they change what a human reads
	// without changing what a machine compares.
	case 0x200e, 0x200f, 0x202a, 0x202b, 0x202c, 0x202d, 0x202e,
		0x2066, 0x2067, 0x2068, 0x2069:
		return true
	// Zero width and the byte order mark: they hide a boundary, so two
	// different addresses can be made to look like one.
	case 0x200b, 0x200c, 0x200d, 0xfeff:
		return true
	}
	return unicode.Is(unicode.Cf, r)
}

func describeRune(r rune) string {
	switch {
	case r == 0x1b:
		return "an escape"
	case r == '\r':
		return "a carriage return"
	case r == 0x08:
		return "a backspace"
	case r == 0:
		return "a nul"
	case r >= 0x202a && r <= 0x202e:
		return "a bidirectional override"
	case r >= 0x2066 && r <= 0x2069:
		return "a bidirectional isolate"
	case r == 0x200b:
		return "a zero-width space"
	case r == 0xfeff:
		return "a byte order mark"
	}
	return "a control character"
}

func excerpt(s string, at int) string {
	lo, hi := at-20, at+20
	if lo < 0 {
		lo = 0
	}
	if hi > len(s) {
		hi = len(s)
	}
	return s[lo:hi]
}

// decodedStrings pulls every string out of a JSON document, which is what a
// consumer gets after decoding. The encoder escapes a control character into
//  and the document is valid; the string handed back to the caller
// still holds the escape, and the caller prints it somewhere.
func decodedStrings(t *testing.T, doc string) []string {
	t.Helper()
	var v interface{}
	if err := json.Unmarshal([]byte(doc), &v); err != nil {
		t.Fatalf("not valid JSON: %v", err)
	}
	var out []string
	var walk func(interface{})
	walk = func(v interface{}) {
		switch t := v.(type) {
		case string:
			out = append(out, t)
		case map[string]interface{}:
			for k, child := range t {
				out = append(out, k)
				walk(child)
			}
		case []interface{}:
			for _, child := range t {
				walk(child)
			}
		}
	}
	walk(v)
	return out
}

func countLinesStarting(s, prefix string) int {
	var n int
	for _, line := range strings.Split(s, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), prefix) {
			n++
		}
	}
	return n
}

// countBanners counts the severity headings in a terminal report. A banner is
// the level name in capitals at the start of a line, which is the one piece of
// furniture that says "a group of findings begins here".
func countBanners(s string) int {
	var n int
	for _, line := range strings.Split(stripOurAnsi(s), "\n") {
		for _, name := range []string{"UNRANKED", "CRITICAL", "HIGH", "LOW", "INFO"} {
			if strings.HasPrefix(line, name+" ") {
				n++
				break
			}
		}
	}
	return n
}

// liveMarkdown returns only the parts of a document that markdown will
// INTERPRET: fenced code blocks and inline code spans removed.
//
// This distinction is the whole basis of the markdown checks, and getting it
// wrong in either direction makes them useless. Counting raw occurrences
// reports the format working correctly as a breach - an attribute path shown
// verbatim inside a fence is what a fence is for. Ignoring the question
// entirely means never noticing that a payload landed somewhere live. What
// matters is whether the markup can act, and it can only act out here.
func liveMarkdown(md string) string {
	var out []string
	fence := ""
	inHTMLBlock := false
	for _, line := range strings.Split(md, "\n") {
		trimmed := strings.TrimSpace(line)

		// CommonMark HTML block, type 6: it opens on a line starting with a
		// known block tag and runs until a blank line, and markdown inside it
		// is NOT parsed. The <details>/<summary> pair this renderer writes is
		// exactly that, which is why the addresses in a summary are
		// HTML-escaped and not markdown-escaped - ** there is two asterisks a
		// reader sees, not emphasis. Escaping it anyway would put visible
		// backslashes in front of every bracket in an address.
		if inHTMLBlock {
			if trimmed == "" {
				inHTMLBlock = false
			}
			continue
		}
		if fence == "" && startsHTMLBlock(trimmed) {
			inHTMLBlock = true
			continue
		}

		if fence == "" {
			if open := fenceOpening(trimmed); open != "" {
				fence = open
				continue
			}
			out = append(out, stripCodeSpans(line))
			continue
		}
		// A fence is closed by a run of at least as many backticks as opened
		// it, and nothing else on the line.
		if closesFence(trimmed, len(fence)) {
			fence = ""
		}
	}
	return strings.Join(out, "\n")
}

// fenceOpening returns the backtick run a line opens a fenced block with, or
// "" if it does not open one. A fence may carry an info string - ```terraform
// is the one this package writes - so the run is measured rather than the
// whole line being compared.
func fenceOpening(s string) string {
	n := 0
	for n < len(s) && s[n] == '`' {
		n++
	}
	if n < 3 {
		return ""
	}
	// An info string may not itself contain a backtick, which is what stops
	// an inline span being read as a fence.
	if strings.Contains(s[n:], "`") {
		return ""
	}
	return strings.Repeat("`", n)
}

func closesFence(s string, openedWith int) bool {
	n := 0
	for n < len(s) && s[n] == '`' {
		n++
	}
	return n >= openedWith && strings.TrimSpace(s[n:]) == ""
}

// stripCodeSpans removes inline code spans from one line. A span opened with
// n backticks is closed by the next run of exactly n, which is the rule
// codeSpan() in markdown.go relies on to be unbreakable.
//
// A BACKSLASH-ESCAPED BACKTICK DOES NOT OPEN A SPAN, and missing that made
// this helper report the renderer as broken when it was not. prose() escapes a
// backtick from a plan to \`, which markdown renders as a literal character;
// a scanner that read it as a delimiter went out of step with the real spans
// on the same line and left live-looking markup in its output. The rule here
// has to match the one the format uses, or the helper invents breaches.
func stripCodeSpans(line string) string {
	var out strings.Builder
	i := 0
	for i < len(line) {
		if line[i] == '\\' && i+1 < len(line) {
			out.WriteString(line[i : i+2])
			i += 2
			continue
		}
		if line[i] != '`' {
			out.WriteByte(line[i])
			i++
			continue
		}
		n := 0
		for i+n < len(line) && line[i+n] == '`' {
			n++
		}
		closing := indexRun(line[i+n:], n)
		if closing < 0 {
			// An unclosed span is not a span; the backticks are literal.
			out.WriteString(line[i : i+n])
			i += n
			continue
		}
		i += n + closing + n
	}
	return out.String()
}

// assertFencesHold checks that no line inside a fenced block could close it
// early, and that every fence opened is closed. A fence its own contents can
// end drops the rest of the report back into live markdown.
func assertFencesHold(t *testing.T, md string) {
	t.Helper()
	fence := ""
	openedAt := 0
	for i, line := range strings.Split(md, "\n") {
		trimmed := strings.TrimSpace(line)
		if fence == "" {
			if open := fenceOpening(trimmed); open != "" {
				fence, openedAt = open, i+1
			}
			continue
		}
		if closesFence(trimmed, len(fence)) {
			fence = ""
			continue
		}
		// Inside the block: nothing may hold a run long enough to end it.
		if strings.Contains(line, fence) {
			t.Errorf("line %d can close the %d-backtick fence opened on line %d: %q",
				i+1, len(fence), openedAt, line)
		}
	}
	if fence != "" {
		t.Errorf("the fence opened on line %d with %q was never closed", openedAt, fence)
	}
}

// startsHTMLBlock reports whether a line opens a CommonMark type 6 HTML block.
// Only the tags this renderer actually writes are listed: a wider list would
// be guessing at what the renderer might do rather than describing what it
// does, and a payload cannot open a block of its own because everything from
// the plan is HTML-escaped before it reaches a line like this.
func startsHTMLBlock(s string) bool {
	for _, tag := range []string{"<details>", "</details>", "<summary>"} {
		if strings.HasPrefix(s, tag) {
			return true
		}
	}
	return false
}

// markupOutsideCode is the document with fenced blocks and inline code spans
// removed, but HTML BLOCKS KEPT.
//
// It is the view for counting elements, where liveMarkdown is the view for
// asking what markdown will interpret. The two are different questions and
// using one for the other makes a check vacuous: liveMarkdown drops the
// <details> elements, so counting them there would compare zero against zero
// and pass whatever happened.
//
// Everything from a plan is HTML-escaped before it reaches a line out here, so
// an unescaped <details> in this view is one the renderer wrote.
func markupOutsideCode(md string) string {
	var out []string
	fence := ""
	inHTMLBlock := false
	for _, line := range strings.Split(md, "\n") {
		trimmed := strings.TrimSpace(line)

		// INSIDE A RAW HTML BLOCK, A BACKTICK IS A BACKTICK. Markdown syntax is
		// inactive there but HTML syntax is very much alive, so the line is
		// KEPT - this view is for counting elements - and simply not searched
		// for code spans. Stripping them would have deleted `</code><b>spoof</b>`
		// out of a <summary> on the grounds that it sat between two backticks,
		// which is exactly the content this view exists to inspect.
		if inHTMLBlock {
			out = append(out, line)
			if trimmed == "" {
				inHTMLBlock = false
			}
			continue
		}
		if fence == "" && startsHTMLBlock(trimmed) {
			inHTMLBlock = true
			out = append(out, line)
			continue
		}

		if fence == "" {
			if open := fenceOpening(trimmed); open != "" {
				fence = open
				continue
			}
			out = append(out, stripCodeSpans(line))
			continue
		}
		if closesFence(trimmed, len(fence)) {
			fence = ""
		}
	}
	return strings.Join(out, "\n")
}

// indexRun finds the first run of exactly n backticks in s - the closing
// delimiter of a code span opened with n.
//
// IT DOES NOT HONOUR BACKSLASH ESCAPES, and that is the rule rather than an
// omission. A backslash is literal INSIDE a code span, so `x\`**bold**` is a
// span holding `x\` followed by live emphasis. A scanner that skipped the
// escaped backtick ran past the real closing delimiter and swallowed the
// emphasis, which would have hidden a genuine breakout. Escapes are honoured
// when looking for the OPENING delimiter, where they do apply.
func indexRun(s string, n int) int {
	i := 0
	for i < len(s) {
		if s[i] != '`' {
			i++
			continue
		}
		run := 0
		for i+run < len(s) && s[i+run] == '`' {
			run++
		}
		if run == n {
			return i
		}
		i += run
	}
	return -1
}

// assertValidUTF8 is the other half of "inert". A byte that is not valid UTF-8
// in a document that claims to be leaves the destination guessing, and what a
// terminal or a browser does with it is not this package's to predict.
func assertValidUTF8(t *testing.T, where, out string) {
	t.Helper()
	if !utf8.ValidString(out) {
		t.Errorf("%s is not valid UTF-8", where)
	}
}

// tableColumns counts the cells in a markdown table row.
//
// A ROW IS SPLIT ON UNESCAPED PIPES, BEFORE ANY INLINE MARKUP IS PARSED, which
// is why counting rows is not enough on its own. A payload that adds a pipe
// does not add a row; it adds a COLUMN, and GitHub discards the cells past the
// header's width - so the visible effect is that the end of a row silently
// disappears. That is how a rename proposal and its caveat vanished out of a
// Notes column while every row count still matched.
//
// A backslash escapes the pipe after it, and a doubled backslash escapes
// itself and leaves the pipe live, so the parity of the run is what decides.
func tableColumns(line string) int {
	n := 0
	slashes := 0
	for i := 0; i < len(line); i++ {
		switch line[i] {
		case '\\':
			slashes++
		case '|':
			if slashes%2 == 0 {
				n++
			}
			slashes = 0
		default:
			slashes = 0
		}
	}
	return n
}

// assertTableShapeHolds checks that every row of every table has the same
// number of cells as the header that opens it.
func assertTableShapeHolds(t *testing.T, md string) {
	t.Helper()
	want := 0
	for i, line := range strings.Split(markupOutsideCode(md), "\n") {
		trimmed := strings.TrimSpace(line)
		if !strings.HasPrefix(trimmed, "|") {
			want = 0
			continue
		}
		got := tableColumns(trimmed)
		if want == 0 {
			want = got
			continue
		}
		if got != want {
			t.Errorf("table row on line %d has %d cell boundaries, the header has %d - "+
				"the payload changed the columns, so GitHub drops what is past them: %q",
				i+1, got, want, line)
		}
	}
}
