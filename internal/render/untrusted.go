package render

import (
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/dbhq-uk/terraken/internal/assess"
)

// Everything taken from a plan is untrusted input, in every format.
//
// THE ATTACK IS REVIEWER DECEPTION, NOT DEFACEMENT. A resource address carries
// a `for_each` key chosen by whoever wrote the Terraform, and on a fork pull
// request that is not somebody to trust. The addresses, attribute paths,
// module names and resource types in a report all come from there, and every
// one of them is printed. A report that can be made to grow a table row, close
// a stylesheet, repaint a terminal or reverse the reading order of a line is
// worse than no report, because it is the thing being trusted.
//
// TWO LAYERS, AND BOTH ARE NEEDED.
//
// First, `sanitise` below, applied to the whole report at the entry to every
// renderer. It removes what is dangerous everywhere: escape sequences,
// carriage returns, newlines, and the Unicode format characters that reorder
// or hide text. Doing it once, over the report, rather than at each of the
// forty-odd places a string is written, is what makes the property hold for a
// renderer somebody adds later without reading this file.
//
// Second, each format escapes for its own context on the way out - HTML
// escaping, a markdown table cell, a code span long enough not to be closed by
// its own contents. That layer cannot be shared, because the dangerous
// character in one destination is ordinary text in another.
//
// IT DOES NOT REWRITE THE PLAN. Terraken never edits somebody's artefacts; it
// escapes on the way out, and what it prints says plainly what the file held.

// sanitise returns the report with every string that came from a plan made
// safe to print in any destination.
//
// It copies rather than mutating. A Report is an ordinary struct a caller owns,
// and a renderer that quietly rewrote its argument would be a surprise - and
// would make the second call in a test see different input from the first.
func sanitise(r assess.Report) assess.Report {
	out := r
	out.TerraformVersion = safeText(r.TerraformVersion)
	out.FormatVersion = safeText(r.FormatVersion)
	out.HiddenBelow = safeText(r.HiddenBelow)

	// NIL STAYS NIL. A nil slice marshals as null and an empty one as [], and
	// which of those a caller gets is not this function's business to change:
	// sanitising a report must alter the characters in it and nothing else.
	if r.Findings != nil {
		out.Findings = make([]assess.Finding, len(r.Findings))
		for i, f := range r.Findings {
			out.Findings[i] = sanitiseFinding(f)
		}
	}
	if r.CountsByName != nil {
		out.CountsByName = make(map[string]int, len(r.CountsByName))
		for k, v := range r.CountsByName {
			out.CountsByName[safeText(k)] = v
		}
	}

	out.Shape = sanitiseShape(r.Shape)
	out.Exposure = sanitiseExposure(r.Exposure)
	out.Coverage = sanitiseCoverage(r.Coverage)

	// A check's address comes from the configuration, so it carries a
	// for_each key like any other address. Its detail is this package's own
	// sentence, cleaned for the same reason every other one is.
	if r.Checks != nil {
		out.Checks = make([]assess.CheckFinding, len(r.Checks))
		for i, c := range r.Checks {
			c.Address = safeText(c.Address)
			c.Kind = safeText(c.Kind)
			c.Status = safeText(c.Status)
			c.Detail = safeText(c.Detail)
			out.Checks[i] = c
		}
	}

	// Drift entries are findings built from the plan like any other, so they
	// carry the same untrusted addresses and paths.
	if r.Drift != nil {
		out.Drift = make([]assess.Finding, len(r.Drift))
		for i, f := range r.Drift {
			out.Drift[i] = sanitiseFinding(f)
		}
	}

	// Report.Status is THREE BOOLEANS AND NOTHING ELSE, so there is nothing
	// here to escape: every word printed beside them is written by
	// render/status.go, not read out of the plan. It is named rather than
	// left out, because "this field was considered" and "this field was
	// forgotten" look identical in a diff -
	// TestEveryStringOnAReportIsSanitised holds the two apart.
	return out
}

func sanitiseFinding(f assess.Finding) assess.Finding {
	out := f
	out.Address = safeText(f.Address)
	out.Type = safeText(f.Type)
	out.Module = safeText(f.Module)
	out.Provider = safeText(f.Provider)
	out.LevelName = safeText(f.LevelName)
	out.Kind = assess.Kind(safeText(string(f.Kind)))
	out.Reason = safeText(f.Reason)
	out.ReplacePaths = safeStrings(f.ReplacePaths)

	if f.Annotations != nil {
		out.Annotations = make([]assess.Annotation, len(f.Annotations))
		for i, a := range f.Annotations {
			out.Annotations[i] = sanitiseAnnotation(a)
		}
	}
	return out
}

func sanitiseAnnotation(a assess.Annotation) assess.Annotation {
	out := a
	// The code is this package's own constant and never comes from a plan,
	// but it is cleaned anyway: a Report is a struct a caller fills in, and a
	// string this package did not choose gets no exemption for looking like
	// one it did.
	out.Code = safeText(a.Code)
	out.Detail = safeText(a.Detail)
	out.Summary = safeText(a.Summary)
	out.Note = safeText(a.Note)
	out.Paths = safeStrings(a.Paths)

	if a.Moved != nil {
		m := *a.Moved
		m.From = safeText(m.From)
		m.To = safeText(m.To)
		m.FromModule = safeText(m.FromModule)
		m.ToModule = safeText(m.ToModule)
		m.Rivals = safeStrings(m.Rivals)
		out.Moved = &m
	}
	if a.Reached != nil {
		reached := make([]assess.Reached, len(a.Reached))
		for i, rch := range a.Reached {
			rch.Address = safeText(rch.Address)
			reached[i] = rch
		}
		out.Reached = reached
	}
	return out
}

func sanitiseShape(s assess.Shape) assess.Shape {
	out := s
	out.Headline = safeText(s.Headline)

	if s.ByAction != nil {
		out.ByAction = make(map[assess.Kind]int, len(s.ByAction))
		for k, v := range s.ByAction {
			out.ByAction[assess.Kind(safeText(string(k)))] = v
		}
	}
	if s.ByType != nil {
		out.ByType = make(map[string]int, len(s.ByType))
		for k, v := range s.ByType {
			out.ByType[safeText(k)] = v
		}
	}
	if s.ByModule != nil {
		out.ByModule = make(map[string]int, len(s.ByModule))
		for k, v := range s.ByModule {
			out.ByModule[safeText(k)] = v
		}
	}
	if s.BusiestModules != nil {
		out.BusiestModules = make([]assess.ModuleChurn, len(s.BusiestModules))
		for i, m := range s.BusiestModules {
			m.Name = safeText(m.Name)
			out.BusiestModules[i] = m
		}
	}
	return out
}

// Coverage's sentences are written by internal/assess, not read out of a
// plan - but they are built with fmt and a future one could interpolate a
// module name or an address, which is exactly the kind of change nobody
// remembers to re-check. Cleaning it costs one pass over a handful of short
// strings and removes the question.
func sanitiseCoverage(c assess.Coverage) assess.Coverage {
	out := c
	out.Headline = safeText(c.Headline)
	if c.Gaps != nil {
		out.Gaps = make([]assess.Gap, len(c.Gaps))
		for i, g := range c.Gaps {
			g.Code = safeText(g.Code)
			g.Detail = safeText(g.Detail)
			out.Gaps[i] = g
		}
	}
	return out
}

func sanitiseExposure(e assess.Exposure) assess.Exposure {
	out := e
	out.Note = safeText(e.Note)
	out.Advice = safeText(e.Advice)
	if e.Values != nil {
		out.Values = make([]assess.ExposedValue, len(e.Values))
		for i, v := range e.Values {
			v.Address = safeText(v.Address)
			v.Path = safeText(v.Path)
			v.Looks = safeText(v.Looks)
			out.Values[i] = v
		}
	}
	return out
}

func safeStrings(in []string) []string {
	if in == nil {
		return nil
	}
	out := make([]string, len(in))
	for i, s := range in {
		out[i] = safeText(s)
	}
	return out
}

// safeText replaces every character that can act on a destination rather than
// be read by one, with a visible escape saying what was there.
//
// SHOWN, NOT DROPPED. Removing the character would make two different
// addresses render identically, which is the deception rather than the cure:
// a reviewer comparing `app["a"]` against `app["a\u200b"]` has to be able to
// see that they differ. Writing `\u200b` is the honest rendering, and it is
// inert in a terminal, in markdown, in HTML and in JSON alike.
//
// What it catches, and why each one is here:
//
//   - C0 controls, DEL and the C1 range. An escape starts a sequence that
//     repaints or clears a terminal; a carriage return writes over the line
//     just printed; a backspace deletes it a character at a time. A newline is
//     the sharpest of all, because it ends a markdown table row and starts a
//     line the reader takes for the tool's own.
//   - Bidirectional overrides and isolates. No escape sequence involved: they
//     reverse what a human reads while leaving what a machine compares
//     untouched, which is exactly a report saying something other than what
//     the plan does.
//   - Zero-width characters and the byte order mark. They hide a boundary, so
//     two different resources can be made to look like one.
//   - Every other Unicode format character, by category, so the list above is
//     a set of examples rather than the whole defence.
//
// A tab and a newline are NOT exempt here even though the renderers use both.
// The renderers write their own layout; this is for text arriving from a file.
func safeText(s string) string {
	if !needsEscaping(s) {
		return s
	}
	var b strings.Builder
	b.Grow(len(s) + 8)
	for i := 0; i < len(s); {
		r, size := utf8.DecodeRuneInString(s[i:])
		// INVALID UTF-8 IS ESCAPED BYTE BY BYTE. Ranging over a string hands
		// back U+FFFD for a bad byte and hides which byte it was, so the
		// decode is explicit: a one-byte RuneError means the input was not
		// valid UTF-8 there, and the raw byte is written as \xNN rather than
		// passed through. A document is supposed to be UTF-8, and a byte that
		// is not leaves the destination to guess - which is the whole class of
		// problem this file exists to close.
		//
		// The loader cannot produce this today, because encoding/json replaces
		// bad bytes with U+FFFD while decoding. Report is an ordinary struct a
		// caller fills in, and this package does not get to assume where its
		// input came from.
		if r == utf8.RuneError && size == 1 {
			b.WriteString(escapeByte(s[i]))
			i++
			continue
		}
		if dangerous(r) {
			b.WriteString(escapeRune(r))
		} else {
			b.WriteString(s[i : i+size])
		}
		i += size
	}
	return b.String()
}

// needsEscaping is the fast path. Almost every string in almost every report
// is an ordinary resource address, and walking one twice is cheaper than
// building a copy of it.
func needsEscaping(s string) bool {
	if !utf8.ValidString(s) {
		return true
	}
	for _, r := range s {
		if dangerous(r) {
			return true
		}
	}
	return false
}

func dangerous(r rune) bool {
	// THE BACKSLASH IS ESCAPED TOO, AND THAT IS NOT COSMETIC. Without it this
	// function is not injective: a real newline becomes the two characters \n,
	// and a module genuinely named `a\n` already is those two characters, so
	// the two collide. They collide in a map key as well as on screen, and
	// Shape.ByModule is a map - one count silently overwrote the other and the
	// report came out different between runs of the same plan, which breaks
	// determinism as well as accuracy. Doubling the backslash keeps every
	// distinct input distinct, which is the same reason Go and JSON do it.
	if r == '\\' {
		return true
	}
	if r < 0x20 || r == 0x7f || (r >= 0x80 && r <= 0x9f) {
		return true
	}
	// Format characters: bidirectional controls, zero-width joiners, the byte
	// order mark and everything else Unicode puts in the category.
	return unicode.Is(unicode.Cf, r)
}

// escapeRune writes one dangerous character the way Go source would, which is
// a notation a reader is likely to recognise and no destination acts on.
func escapeRune(r rune) string {
	switch r {
	case '\\':
		return `\\\\`
	case '\n':
		return `\n`
	case '\r':
		return `\r`
	case '\t':
		return `\t`
	case 0x1b:
		return `\x1b`
	case 0:
		return `\x00`
	}
	if r < 0x100 {
		return escapeByte(byte(r))
	}
	const hex = "0123456789abcdef"
	// FOUR DIGITS IS NOT ENOUGH ABOVE THE BASIC PLANE. \u takes four and
	// silently dropped the high bits, so U+E0001 - a tag character, and
	// invisible - came out as \u0001. The escape then named a different
	// character from the one in the file, and two distinct inputs could share
	// one rendering again. Go's own notation has a wider form; use it.
	if r > 0xffff {
		return `\U` + string([]byte{
			hex[(r>>28)&0xf], hex[(r>>24)&0xf], hex[(r>>20)&0xf], hex[(r>>16)&0xf],
			hex[(r>>12)&0xf], hex[(r>>8)&0xf], hex[(r>>4)&0xf], hex[r&0xf],
		})
	}
	return `\u` + string([]byte{
		hex[(r>>12)&0xf], hex[(r>>8)&0xf], hex[(r>>4)&0xf], hex[r&0xf],
	})
}

// escapeByte writes one raw byte as \xNN, for a C0 control and for a byte that
// was not valid UTF-8 in the first place.
func escapeByte(b byte) string {
	const hex = "0123456789abcdef"
	return `\x` + string([]byte{hex[b>>4], hex[b&0xf]})
}
