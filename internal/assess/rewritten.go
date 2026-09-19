package assess

import (
	"encoding/json"
	"fmt"
	"math"
	"math/big"
	"reflect"
	"sort"
	"strconv"
	"strings"
)

// This file finds the attributes whose before and after are the same value
// written a different way.
//
// It is the same shape of rule as the one in reorder.go, for the same
// reason. Terraform reports an attribute as changed whenever the rendered
// before and after differ, and a provider that round-trips a JSON policy
// document, re-indents a heredoc, hands a port back as a string or returns
// an empty list where state held null will produce a diff nobody wrote.
// That is the single largest source of noise in a plan.
//
// What is reported is the fact and the class it falls into, never a
// verdict. Every one of these classes has a case where the difference is
// real:
//
//   - A JSON document with its keys in a different order is the same
//     policy, but a consumer that compares the string byte for byte sees
//     a change.
//   - Whitespace is significant in a shell script, in a YAML document
//     carried as a string, and in anything hashed.
//   - 80 and "80" differ in type, and a type change can matter.
//   - null and [] are different to Terraform in some positions, where null
//     can mean inherit a default and empty means explicitly none.
//
// So each annotation names what was seen and hands the judgement back, and
// nothing here ever touches a finding's level.
//
// No value ever leaves this file. Values are compared internally, which is
// the only way to answer the question at all, and only the path is
// returned.

// rewriteClass is one way a before and an after can hold the same value
// written differently.
//
// The order of the constants is the precedence order, and precedence is
// narrowest claim first. A pair that satisfies more than one class is
// reported once, under the class that claims the least about it: a JSON
// document with a trailing newline is reported as whitespace rather than
// as JSON, because "the only difference is whitespace" is a smaller and
// more useful statement than "the keys may have moved but the data
// matches".
type rewriteClass int

const (
	notRewritten rewriteClass = iota
	whitespaceRewrite
	numberRewrite
	emptyRewrite
	jsonRewrite
)

// rewriteWording is what each class is called and what it is careful to
// say. Keeping the wording beside the rule in one table is what stops the
// two drifting apart, and it fixes the order the annotations come out in.
//
// note carries no dash of any kind, deliberately, for the same reason
// reorderNote does not: these sentences are long enough to wrap at every
// width the terminal supports, and a hyphen stranded at the start of a
// wrapped line reads as a bullet rather than as punctuation.
//
// detail is built from summary and note rather than written out again,
// which makes "Detail stays complete on its own" a property of the code
// instead of a thing to remember. Cutting a caveat out of the terminal
// footer must never cut it out of the JSON.
var rewriteWording = []struct {
	class   rewriteClass
	code    string
	summary string
	fact    string
	note    string
}{
	{
		class:   whitespaceRewrite,
		code:    AnnSameWhitespace,
		summary: "same text, different whitespace",
		fact:    "these attributes hold the same text with the whitespace set differently, such as a trailing newline, an indent or a line ending.",
		note:    "Whitespace is significant in a script, in a YAML document carried as a string and in anything hashed, so whether a whitespace change matters is yours to judge.",
	},
	{
		class:   numberRewrite,
		code:    AnnSameNumber,
		summary: "same number, written differently",
		// No quotation marks in the wording, deliberately. The markdown
		// renderer escapes everything landing in a table cell, so a pair
		// of quotes arrives there as &#34; in the raw text of a pull
		// request comment.
		fact: "both sides of these attributes are the same number written a different way, such as 80 against the text 80, or 1e3 against 1000.",
		note: "A number and its string form are different types, and a type change can matter, so whether this one does is yours to judge.",
	},
	{
		class:   emptyRewrite,
		code:    AnnNullAndEmpty,
		summary: "null on one side, empty on the other",
		fact:    "these attributes are null on one side and an empty list, object or string on the other.",
		note:    "Null and empty are not the same thing to Terraform everywhere, where null can mean inherit a default and empty means explicitly none, so whether this one matters is yours to judge.",
	},
	{
		class:   jsonRewrite,
		code:    AnnSameJSON,
		summary: "same JSON, written differently",
		fact:    "both sides of these attributes parse as JSON and hold the same data, with the keys in a different order or the whitespace set differently.",
		note:    "A consumer that compares the document byte for byte still sees a change, so whether a rewritten document matters is yours to judge.",
	},
}

// What the roll-up says.
//
// rollUpSummary is a whole sentence rather than a label, which is what
// makes it read as a statement about the resource instead of as one more
// attribute heading. That difference is the only one a plain, uncoloured
// terminal has to go on, so it is carried in the wording rather than left
// to a renderer.
//
// The summary stops at the difference in writing and says nothing about
// meaning. That restraint is load-bearing rather than timid.
//
// An earlier wording ended "not in what it is". That is a ruling, and for
// a reordered container command it is a false one - reordering those
// arguments changes what the container does. A caveat was attached, but in
// the wrong place: Summary is what a reader sees against the finding and
// Note is lifted into a footer, so the strong claim was prominent and the
// correction was buried. Anyone scanning the report would have taken the
// claim and never reached the retraction.
//
// So the fact is that each attribute differs in how it is written. Whether
// a different writing is a different thing is the reader's call, on every
// class this fires over: order matters in a container command, whitespace
// matters in a script, and a type change from 80 to "80" can matter
// anywhere.
const (
	rollUpSummary = "every attribute this plan shows as changed here is a difference in how the value is written"
	rollUpNote    = "Each of these classes has a case where the difference is real. Order matters in a container command, and whitespace matters in a script. This says what kind of difference each one is and rules on none of them."
	rollUpDetail  = rollUpSummary + ". " + rollUpNote
)

// rewrites is what the walk found on one resource.
type rewrites struct {
	// paths holds the attribute paths found, grouped by class.
	paths map[rewriteClass][]string

	// allWritten is the roll-up: every top-level attribute the plan shows
	// as changed was accounted for, and there was at least one.
	allWritten bool
}

// annotations turns what the walk found into annotations, one per class
// that fired plus the roll-up, in a fixed order so the report is
// deterministic.
func (r rewrites) annotations() []Annotation {
	var out []Annotation
	for _, w := range rewriteWording {
		paths := r.paths[w.class]
		if len(paths) == 0 {
			continue
		}
		sort.Strings(paths)
		out = append(out, Annotation{
			Code:    w.code,
			Detail:  w.fact + " " + w.note,
			Summary: w.summary,
			Note:    w.note,
			Paths:   paths,
		})
	}

	// The roll-up comes last, where it reads as a statement about
	// everything above it rather than as one more class.
	if r.allWritten {
		out = append(out, Annotation{
			Code:    AnnAllRewritten,
			Detail:  rollUpDetail,
			Summary: rollUpSummary,
			Note:    rollUpNote,
		})
	}
	return out
}

// rewrittenPaths walks a resource's before and after and reports every
// attribute whose two sides are the same value written differently.
//
// reordered is what the reordering rule already claimed. Precedence: a
// path that rule reported is not reported again here, and the walk does
// not descend beneath it either, because below a reordering index i on one
// side is no longer index i on the other.
func rewrittenPaths(before, after, afterUnknown interface{}, reordered []string) rewrites {
	out := rewrites{paths: map[rewriteClass][]string{}}

	// A resource's before and after are always objects in plan JSON.
	// Anything else - a create with no before, a delete with no after - is
	// not a pair to compare.
	bm, bok := before.(map[string]interface{})
	am, aok := after.(map[string]interface{})
	if !bok || !aok {
		return out
	}

	claimed := make(map[string]bool, len(reordered))
	for _, p := range reordered {
		claimed[p] = true
	}

	// The roll-up is counted over top-level attributes, because that is
	// what a reader sees the plan show as changed.
	changed, allExplained := 0, true
	for _, k := range unionKeys(bm, am) {
		bv, inBefore := bm[k]
		av, inAfter := am[k]

		if !inBefore || !inAfter {
			// Present on one side only. Terraform drops an attribute that
			// is not known until apply from the after entirely, so this is
			// usually an unknown value. Either way the plan shows the
			// attribute as changed and there is no pair to hold it
			// against, so the roll-up cannot speak for this resource.
			changed++
			allExplained = false
			continue
		}

		if jsonEqual(bv, av) {
			continue
		}
		changed++
		if !walkRewrites(bv, av, mapChild(afterUnknown, k), k, claimed, &out) {
			allExplained = false
		}
	}

	// A resource with nothing changed has nothing for the roll-up to speak
	// about, and the sentence said over no attributes at all is a sentence
	// with no subject.
	out.allWritten = changed > 0 && allExplained
	return out
}

// unionKeys is every key on either side, sorted, so the walk is
// deterministic and an attribute set only on the after is seen too.
func unionKeys(before, after map[string]interface{}) []string {
	seen := make(map[string]bool, len(before)+len(after))
	keys := make([]string, 0, len(before)+len(after))
	for _, m := range []map[string]interface{}{before, after} {
		for k := range m {
			if seen[k] {
				continue
			}
			seen[k] = true
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)
	return keys
}

// walkRewrites classifies one differing before and after, descending into
// objects and lists when the difference is not a class in itself.
//
// It returns whether everything that differs at or beneath this point was
// accounted for, which is what the roll-up is built from. Reporting a
// class and accounting for a subtree are the same answer here on purpose:
// the roll-up may only say what it says if nothing was left unexplained.
func walkRewrites(before, after, afterUnknown interface{}, path string, claimed map[string]bool, out *rewrites) bool {
	// The reordering rule already reported this path. It is accounted for,
	// and claiming it again under a second class would tell the reader the
	// same attribute twice.
	if claimed[path] {
		return true
	}

	// true at this node means the whole value is unknown until apply.
	// There is no after to hold the before against, here or anywhere
	// beneath it.
	if whole, ok := afterUnknown.(bool); ok && whole {
		return false
	}

	// A mark deeper in the structure covers part of this value only. The
	// pair cannot be classified as a whole, but the walk carries on so the
	// mark stops the child it actually covers and leaves its siblings
	// comparable.
	if !anyUnknown(afterUnknown) {
		if c := rewriteOf(before, after); c != notRewritten {
			out.paths[c] = append(out.paths[c], path)
			return true
		}
	}

	switch b := before.(type) {
	case map[string]interface{}:
		a, ok := after.(map[string]interface{})
		if !ok {
			// The attribute changed shape, which is a change of a different
			// kind and not one this rule has anything to say about.
			return false
		}
		// Every key on both sides is walked; a key on one side only is a
		// difference this rule cannot account for, so it leaves the subtree
		// unexplained without silencing the siblings it can explain.
		explained := len(b) == len(a)
		for _, k := range unionKeys(b, a) {
			bv, inBefore := b[k]
			av, inAfter := a[k]
			if !inBefore || !inAfter {
				explained = false
				continue
			}
			if jsonEqual(bv, av) {
				continue
			}
			if !walkRewrites(bv, av, mapChild(afterUnknown, k), path+"."+k, claimed, out) {
				explained = false
			}
		}
		return explained

	case []interface{}:
		a, ok := after.([]interface{})
		if !ok {
			return false
		}
		// Different lengths mean index i on one side no longer answers to
		// index i on the other, so there is nothing safe to say below here.
		if len(b) != len(a) {
			return false
		}
		explained := true
		for i := range b {
			if jsonEqual(b[i], a[i]) {
				continue
			}
			if !walkRewrites(b[i], a[i], listChild(afterUnknown, i), fmt.Sprintf("%s[%d]", path, i), claimed, out) {
				explained = false
			}
		}
		return explained
	}

	// A leaf that is not one of the classes. Two different values.
	return false
}

// rewriteOf names the class a differing pair falls into, or notRewritten
// when the two sides are genuinely different values.
//
// The order of the checks is the precedence rule, narrowest claim first,
// and it is the reason an attribute is only ever reported under one class.
func rewriteOf(before, after interface{}) rewriteClass {
	// Identical values are not a difference at all. The walk skips them
	// before it reaches here; this is what keeps the classes honest if
	// anything else ever calls in.
	if jsonEqual(before, after) {
		return notRewritten
	}
	switch {
	case sameTextDifferentWhitespace(before, after):
		return whitespaceRewrite
	case sameNumber(before, after):
		return numberRewrite
	case nullAndEmpty(before, after):
		return emptyRewrite
	case sameJSON(before, after):
		return jsonRewrite
	}
	return notRewritten
}

// sameTextDifferentWhitespace reports whether both sides are strings that
// are equal once the whitespace is normalised.
func sameTextDifferentWhitespace(before, after interface{}) bool {
	b, bok := before.(string)
	a, aok := after.(string)
	if !bok || !aok {
		return false
	}
	return normaliseWhitespace(b) == normaliseWhitespace(a)
}

// normaliseWhitespace puts a string into the form that answers "is the
// only difference how this is laid out": every run of whitespace becomes
// one space, and the ends are trimmed.
//
// That covers the differences this rule is for - a trailing newline, an
// indent, a CRLF against an LF - and it is careful about the one it is
// not. A run is replaced with a single space rather than removed, so "a b"
// and "ab" stay different strings: one of them has a space in it and the
// other does not, and that is not a difference in how a value is written.
func normaliseWhitespace(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

// sameNumber reports whether both sides are the same number written a
// different way: 80 and "80", 1 and "1.0", "1e3" and "1000".
//
// EXACTLY, NOT AS A float64. This rule tells a reviewer that a difference is
// cosmetic, so getting it wrong says a real change is nothing - the worst
// sentence in the tool. Two integers a float64 cannot tell apart,
// 9007199254740992 and 9007199254740993, compared equal here and the rule
// called the change a rewrite. big.Rat parses a decimal string exactly and
// still makes 1e3 and 1000 the same number, which is the case this exists for.
func sameNumber(before, after interface{}) bool {
	b, bok := numberValue(before)
	a, aok := numberValue(after)
	return bok && aok && b.Cmp(a) == 0
}

// numberValue reads a value as an exact number, from a JSON number or from a
// string holding nothing but a number.
//
// Values that are not finite are refused. Nothing in a plan is legitimately an
// infinity or a NaN, and big.Rat cannot hold one anyway - accepting the strings
// would make "Inf" and "infinity" compare as the same number, which is a claim
// about text rather than about a value.
//
// json.Number arrives from the loader, which re-reads a plan's attribute
// numbers so their digits survive - see internal/plan/numbers.go. float64 is
// still accepted, because a caller can build a Change by hand and because
// nothing else in the plan goes through that path.
func numberValue(v interface{}) (*big.Rat, bool) {
	var text string
	switch t := v.(type) {
	case json.Number:
		text = t.String()
	case float64:
		if math.IsNaN(t) || math.IsInf(t, 0) {
			return nil, false
		}
		// -1 is the shortest representation that round-trips, so a float64
		// that came from a JSON number renders as the digits it was read
		// from wherever that is possible.
		text = strconv.FormatFloat(t, 'g', -1, 64)
	case string:
		// strconv does not trim, and a value wrapped in spaces is still the
		// number it holds.
		text = strings.TrimSpace(t)
	default:
		return nil, false
	}

	r, ok := new(big.Rat).SetString(text)
	if !ok {
		return nil, false
	}
	return r, true
}

// nullAndEmpty reports whether one side is null and the other is an empty
// list, an empty object or an empty string.
func nullAndEmpty(before, after interface{}) bool {
	return (before == nil && isEmptyValue(after)) || (after == nil && isEmptyValue(before))
}

// isEmptyValue reports whether a value is an empty list, an empty object
// or an empty string. Nothing else counts: false and 0 are values, not
// absences, and a rule that treated them as empty would call a real change
// a rewrite.
func isEmptyValue(v interface{}) bool {
	switch t := v.(type) {
	case string:
		return t == ""
	case []interface{}:
		return len(t) == 0
	case map[string]interface{}:
		return len(t) == 0
	}
	return false
}

// sameJSON reports whether both sides are strings holding a JSON object or
// array that parse to the same data.
//
// Object key order and insignificant whitespace disappear in the parse,
// which is the whole point: a provider that round-trips an IAM policy, a
// bucket policy or a container definition hands the keys back in its own
// order and Terraform renders a diff. Array order does not disappear, and
// must not: a reordered array is a different document.
func sameJSON(before, after interface{}) bool {
	b, bok := jsonDocument(before)
	a, aok := jsonDocument(after)
	return bok && aok && reflect.DeepEqual(b, a)
}

// jsonDocument parses a string holding a JSON object or array.
//
// Only an object or an array counts. A string holding a bare JSON scalar -
// "80", "true", "null" - is a scalar written as text, and calling that a
// JSON document would let this rule claim more than it saw. The number
// class is the one that speaks for "80".
func jsonDocument(v interface{}) (interface{}, bool) {
	s, ok := v.(string)
	if !ok {
		return nil, false
	}
	t := strings.TrimSpace(s)
	if t == "" || (t[0] != '{' && t[0] != '[') {
		return nil, false
	}
	var out interface{}
	if err := json.Unmarshal([]byte(t), &out); err != nil {
		return nil, false
	}
	return out, true
}

// jsonEqual reports whether two values are the same value, compared by
// their JSON encoding rather than by fmt's %v.
//
// The encoding is what makes the comparison exact and type-aware, and it
// is the lesson this codebase learned once already in renderAttrs: %v
// renders the number 15 and the string "15" identically, which is exactly
// the difference the number class exists to find deliberately rather than
// by accident. Marshalling an object sorts its keys, so two objects that
// hold the same data compare equal whatever order they arrived in.
//
// A value that will not encode is not equal to anything. A comparison that
// cannot be made falls through to the walk, which cannot classify it
// either, so it ends as an attribute nobody claimed - silence rather than
// a guess.
func jsonEqual(before, after interface{}) bool {
	b, err := json.Marshal(before)
	if err != nil {
		return false
	}
	a, err := json.Marshal(after)
	if err != nil {
		return false
	}
	return string(b) == string(a)
}
