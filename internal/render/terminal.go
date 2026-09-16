package render

import (
	"fmt"
	"io"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/dbhq-uk/terrakit/internal/assess"
)

// Width bounds. Below about 60 columns the tree connectors and the
// right-aligned counts stop earning their space; above about 100 a line
// of prose is too long to track back to its own left edge.
const (
	minWidth     = 60
	maxWidth     = 100
	defaultWidth = 80
)

// findingIndent is the left margin every finding sits at, and the column
// the tree connectors hang from.
const findingIndent = "  "

// TerminalOptions are the presentation choices the terminal renderer
// takes from the command line.
//
// Colour and ASCII are separate questions on purpose. A terminal that
// renders box drawing badly may still do colour perfectly well, and a
// pipeline that wants neither sets both. --plain is the flag that sets
// both; nothing in here assumes one implies the other.
type TerminalOptions struct {
	// Colour allows ANSI colour. Off means the layout has to carry the
	// report on its own, which is the test of whether the layout works.
	Colour bool

	// ASCII restricts every glyph to ASCII, for terminals and log
	// viewers that mangle box drawing.
	ASCII bool

	// Width is the column count to set the report to. Zero means ask the
	// stream being written to, which is what the command does; a test
	// pins it so the layout is deterministic.
	Width int
}

// glyphs is the drawing kit. There are two: one that uses box drawing,
// and one that uses nothing outside ASCII. Every connector carries its
// own trailing space so the two sets can differ in width - the ASCII
// connectors are three columns wide, the box-drawing ones two - without
// any caller having to know which is in use.
type glyphs struct {
	heavy  string // one column of the rule that opens and closes the report
	light  string // one column of a section rule
	branch string // a detail line with siblings below it
	last   string // the final detail line
	pipe   string // continuation below a branch, keeping the tree column
	blank  string // continuation below the last line, where the tree ends
}

var boxGlyphs = glyphs{
	heavy:  "━", // heavy horizontal
	light:  "─", // light horizontal
	branch: "├ ",
	last:   "└ ",
	pipe:   "│ ",
	blank:  "  ",
}

var asciiGlyphs = glyphs{
	heavy:  "=",
	light:  "-",
	branch: "|- ",
	last:   "`- ",
	pipe:   "|  ",
	blank:  "   ",
}

// style is everything a line needs to know about how it is being set.
type style struct {
	colour bool
	g      glyphs
	width  int
}

// paint wraps text in an ANSI code, or returns it untouched when colour
// is off or there is nothing to colour. Every colour in the terminal
// output goes through here, so switching colour off cannot leave a stray
// escape behind.
func (s style) paint(code, text string) string {
	if !s.colour || code == "" || text == "" {
		return text
	}
	return code + text + ansiReset
}

// rule returns a horizontal rule n columns wide.
func (s style) rule(unit string, n int) string {
	if n < 1 {
		return ""
	}
	return strings.Repeat(unit, n)
}

// Terminal writes the human-facing report: one section per severity
// present, most severe first, each finding a short stanza under it.
//
// The presentation is the product. terraform plan already contains every
// fact in here; the only thing this adds is being able to see which of
// them matters.
func Terminal(w io.Writer, r assess.Report, opts TerminalOptions) error {
	// Nothing found, and nothing held back. A report whose findings were
	// all filtered out must never claim the plan does nothing: it falls
	// through to the summary, which says what was found and how much of
	// it is not being shown.
	if len(r.Findings) == 0 && r.Hidden == 0 {
		_, err := fmt.Fprintln(w, "No changes. This plan does nothing.")
		return err
	}

	width := opts.Width
	if width <= 0 {
		width = detectWidth(w)
	}
	if width < minWidth {
		width = minWidth
	}
	if width > maxWidth {
		width = maxWidth
	}

	s := style{colour: opts.Colour, g: boxGlyphs, width: width}
	if opts.ASCII {
		s.g = asciiGlyphs
	}

	out := &errWriter{w: w}
	said := newSaid()

	out.line(s.masthead(r))
	out.line(s.paint(ansiGrey, s.rule(s.g.heavy, width)))

	for _, lv := range []assess.Level{assess.Critical, assess.High, assess.Low, assess.Info} {
		group := findingsAt(r, lv)
		// An empty section is noise pretending to be information. Omit
		// it entirely rather than printing a heading and a zero.
		if len(group) == 0 {
			continue
		}
		out.line("")
		out.line(s.section(lv, len(group)))
		for _, f := range group {
			out.line("")
			s.finding(out, f, said)
		}
	}

	out.line("")
	// The closing rule pairs with a body. When a filter hid everything
	// there is no body, and two rules with nothing between them would
	// read as a mistake.
	if len(r.Findings) > 0 {
		out.line(s.paint(ansiGrey, s.rule(s.g.heavy, width)))
	}
	s.notes(out, said.notes)
	out.line(s.summary(r))

	return out.err
}

// said is what the report has already told the reader, carried across
// findings so it is not told again.
//
// The terminal is the one format read as a single stream from top to
// bottom, which is what makes a shared footer work here and nowhere else:
// a markdown row gets quoted into a review comment and an HTML card gets
// screenshotted, so those keep every sentence where it was found.
type said struct {
	// notes are the standing caveats collected in the order they were
	// first met, to be stated once at the foot of the report.
	notes    []string
	seenNote map[string]bool

	// movedPairs are the from/to pairs whose evidence has already been
	// printed. The annotation hangs on both halves of a rename, and both
	// halves genuinely are affected, so both are told - but the suggested
	// moved block is the pair's, not each resource's, and printing it
	// twice invites pasting it twice.
	movedPairs map[string]bool
}

func newSaid() *said {
	return &said{seenNote: map[string]bool{}, movedPairs: map[string]bool{}}
}

// note records a standing caveat, and ignores one it has already got.
func (s *said) note(text string) {
	if text == "" || s.seenNote[text] {
		return
	}
	s.seenNote[text] = true
	s.notes = append(s.notes, text)
}

// firstMoved reports whether this is the first time a pair has been seen,
// and records it. It is called as the report is written, in the order the
// findings are printed, so the evidence always lands on whichever half
// the reader meets first - including when a filter hid the other one.
func (s *said) firstMoved(m *assess.MovedEvidence) bool {
	key := m.From + "\x00" + m.To
	if s.movedPairs[key] {
		return false
	}
	s.movedPairs[key] = true
	return true
}

// notes writes the standing caveats between the closing rule and the
// summary: part of the footer rather than part of the body, and in view
// of the counts a reader stops on.
//
// Nothing is written when no displayed finding carried one. A standing
// note about a rule that did not fire is furniture.
func (s style) notes(out *errWriter, notes []string) {
	for _, n := range notes {
		s.emit(out, "", "", n, ansiGrey)
		out.line("")
	}
}

// masthead is the first line: what this is, how much it found, and which
// Terraform wrote the plan.
func (s style) masthead(r assess.Report) string {
	n := total(r)
	line := s.paint(ansiBold, "terrakit") +
		"  " + fmt.Sprintf("%d %s", n, plural(n, "finding", "findings"))
	if r.TerraformVersion != "" {
		line += "  " + s.paint(ansiGrey, "terraform "+r.TerraformVersion)
	}
	return line
}

// section is a severity heading: the level name, a rule, and the count
// of findings in it right-aligned to the full width.
//
// The fill is measured from the plain name and the plain count and only
// then wrapped in colour. Measuring after wrapping would measure the
// invisible escape bytes instead of the visible text, and every
// section's count would land in a different column - and only when
// colour is on, which is the mode nobody tests in.
func (s style) section(lv assess.Level, n int) string {
	name := strings.ToUpper(lv.String())
	count := strconv.Itoa(n)
	fill := s.width - utf8.RuneCountInString(name) - len(count) - 3
	return s.paint(colourFor(lv), name) + " " +
		s.paint(ansiGrey, s.rule(s.g.light, fill)) + "  " +
		s.paint(colourFor(lv), count)
}

// finding writes one stanza: the address, what the plan does to it, and
// the reasons, hung off a tree.
func (s style) finding(out *errWriter, f assess.Finding, said *said) {
	s.emit(out, findingIndent, findingIndent, f.Address, ansiBold)
	s.emit(out, findingIndent, findingIndent, verb(f.Kind), ansiGrey)

	ds := details(f, said)
	for i, d := range ds {
		connector, carry := s.g.branch, s.g.pipe
		if i == len(ds)-1 {
			connector, carry = s.g.last, s.g.blank
		}
		// A wrapped detail line hangs under its own first word, so the
		// rest of the sentence cannot be mistaken for the evidence
		// below it - and the tree column carries on down the left
		// either way.
		hang := findingIndent + carry
		sub := hang + "  "
		s.emit(out, findingIndent+connector, hang, d.text, d.emphasis)
		for _, line := range d.sub {
			s.emit(out, sub, sub+"  ", line, ansiGrey)
		}
	}
}

// summary is the closing line: the per-level counts for the whole plan,
// and what a filter is holding back.
func (s style) summary(r assess.Report) string {
	var parts, plainParts []string
	for _, t := range tallies(r) {
		token := fmt.Sprintf("%d %s", t.Count, t.Level)
		plainParts = append(plainParts, token)
		parts = append(parts, s.paint(colourFor(t.Level), token))
	}
	counts := strings.Join(parts, "  ")
	plain := strings.Join(plainParts, "  ")

	note := hiddenNote(r)
	if note == "" {
		return counts
	}
	// Right-align the note against the counts when the width allows it,
	// and drop it to its own line when it does not. It is never omitted:
	// showing fewer findings than were found, without saying so, is how
	// the one that mattered gets missed.
	gap := s.width - utf8.RuneCountInString(plain) - utf8.RuneCountInString(note)
	if gap >= 2 {
		return counts + strings.Repeat(" ", gap) + s.paint(ansiGrey, note)
	}
	return counts + "\n" + s.paint(ansiGrey, note)
}

// emit writes text wrapped to the style's width, prefixed with first on
// the first line and rest on every line after it.
//
// The prefix is measured plain and painted separately from the text, so
// a connector's colour never changes where a line breaks.
func (s style) emit(out *errWriter, first, rest, text, code string) {
	lines := wrap(text,
		s.width-utf8.RuneCountInString(first),
		s.width-utf8.RuneCountInString(rest))
	for i, l := range lines {
		prefix := rest
		if i == 0 {
			prefix = first
		}
		if strings.TrimSpace(prefix) != "" {
			prefix = s.paint(ansiGrey, prefix)
		}
		out.line(prefix + s.paint(code, l))
	}
}

// detail is one line hung off a finding's tree, with whatever evidence
// belongs under it.
type detail struct {
	text string
	sub  []string

	// emphasis is an ANSI code for a line that has to stand apart from the
	// ones around it, and empty for the rest. It is presentation only: it
	// never survives --plain, so nothing may depend on it to be
	// understood. The roll-up is set as a whole sentence rather than as a
	// label for exactly that reason.
	emphasis string
}

// details turns a finding into the lines under it, in the order they
// are worth reading: what is lost, why, what forced it, then whatever
// the assessment could not settle.
//
// said is the report's memory. Every line here is about this finding, so
// anything that is really about the whole report - a rule's standing
// caveat, a rename pair's suggested moved block - is handed to said and
// written once instead.
func details(f assess.Finding, said *said) []detail {
	var ds []detail
	if f.DataLoss {
		ds = append(ds, detail{text: "holds data, so destroying it loses that data"})
	}
	if f.Reason != "" {
		ds = append(ds, detail{text: f.Reason})
	}
	switch len(f.ReplacePaths) {
	case 0:
	case 1:
		// One path reads better as a label and a value on one line than
		// as a heading with a single item under it.
		ds = append(ds, detail{text: "forces replacement   " + f.ReplacePaths[0]})
	default:
		ds = append(ds, detail{text: "forces replacement", sub: f.ReplacePaths})
	}
	for _, a := range f.Annotations {
		// The missed-moved-block annotation carries its evidence as
		// fields, so it can be set as a label with the facts under it.
		// Its Detail is a paragraph saying the same thing at four times
		// the length, which is the wrong shape for this layout.
		if m := a.Moved; m != nil {
			ds = append(ds, detail{
				text: annotationLabel(a.Code),
				sub:  movedLines(f.Address, m, said.firstMoved(m)),
			})
			continue
		}

		// Summary and Note are the same annotation taken apart: the fact
		// belongs to this finding, the caveat belongs to the report. An
		// annotation that left them empty has no caveat to hoist, so its
		// Detail is used whole.
		said.note(a.Note)
		text := a.Detail
		if a.Summary != "" {
			text = a.Summary
		}
		ds = append(ds, detail{text: text, sub: a.Paths, emphasis: emphasisFor(a.Code)})
	}
	return ds
}

// emphasisFor is the one annotation set apart from the rest.
//
// The roll-up is a statement about the whole resource where every other
// line is a statement about one attribute, and it is the most useful thing
// this report can say about an update in place that is really nothing. It
// is set in bold so it does not read as one more attribute heading.
//
// Bold is the smaller half of that distinction. --plain turns colour off
// and the line still has to be unmistakable, which is why the roll-up is
// worded as a whole sentence, carries no attribute paths under it, and
// always comes last.
func emphasisFor(code string) string {
	if code == assess.AnnAllRewritten {
		return ansiBold
	}
	return ""
}

// movedLines is the missed-moved-block evidence, one fact per line.
//
// It names the other half of the pair rather than repeating the address
// the reader is already looking at, and it always carries the caution:
// pasting the wrong moved block adopts a decommissioned resource's state
// under a new address, which is worse than the problem it would fix.
//
// full is false on the second half of a pair, whose evidence is the same
// evidence printed a few lines up. It still names the resource it is
// paired with, because that is the fact the reader needs at this address;
// what it drops is the count and the block, which belong to the pair and
// not to either end of it. One suggested moved block printed twice is an
// invitation to paste it twice.
func movedLines(address string, m *assess.MovedEvidence, full bool) []string {
	other := m.To
	if address == m.To {
		other = m.From
	}
	if !full {
		return []string{fmt.Sprintf("paired with %s, shown above", other)}
	}
	lines := []string{
		fmt.Sprintf("%d of %d attributes match %s", m.Matched, m.Compared, other),
		fmt.Sprintf("moved { from = %s  to = %s }", m.From, m.To),
	}
	if m.CrossModule {
		lines = append(lines, fmt.Sprintf("crosses a module boundary, %s to %s",
			moduleName(m.FromModule), moduleName(m.ToModule)))
	}
	return append(lines, "verify the pairing before using that block")
}

// wrap breaks text into lines that fit the width available on the first
// line and the width available on every line after it.
//
// Text that already fits is returned untouched, which keeps deliberate
// spacing - the gap in "forces replacement   zone", the one inside a
// moved block - exactly as it was written.
func wrap(text string, firstWidth, restWidth int) []string {
	if firstWidth < 1 {
		firstWidth = 1
	}
	if restWidth < 1 {
		restWidth = 1
	}
	if utf8.RuneCountInString(text) <= firstWidth {
		return []string{text}
	}

	words := strings.Fields(text)
	if len(words) == 0 {
		return []string{""}
	}

	var lines []string
	cur := ""
	limit := firstWidth
	flush := func() {
		lines = append(lines, cur)
		cur = ""
		limit = restWidth
	}
	for _, word := range words {
		// A single word wider than the line - a long attribute path, a
		// for_each key - is cut rather than allowed off the edge.
		for utf8.RuneCountInString(word) > limit {
			if cur != "" {
				flush()
				continue
			}
			r := []rune(word)
			lines = append(lines, string(r[:limit]))
			word = string(r[limit:])
			limit = restWidth
		}
		switch {
		case cur == "":
			cur = word
		case utf8.RuneCountInString(cur)+1+utf8.RuneCountInString(word) <= limit:
			cur += " " + word
		default:
			flush()
			cur = word
		}
	}
	if cur != "" {
		lines = append(lines, cur)
	}
	return lines
}

// findingsAt returns the displayed findings at one level. Report.Findings
// is already sorted most severe first, so this preserves that order
// within the group.
func findingsAt(r assess.Report, lv assess.Level) []assess.Finding {
	var out []assess.Finding
	for _, f := range r.Findings {
		if f.Level == lv {
			out = append(out, f)
		}
	}
	return out
}
