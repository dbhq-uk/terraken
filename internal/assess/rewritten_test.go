package assess

import (
	"reflect"
	"strings"
	"testing"

	tfjson "github.com/hashicorp/terraform-json"
)

// pathsFor is the paths of one annotation code on a finding, or nil when
// the finding does not carry that annotation.
func pathsFor(f Finding, code string) []string {
	a, ok := annotationFor(f, code)
	if !ok {
		return nil
	}
	return a.Paths
}

// rolledUp reports whether the finding carries the roll-up: the statement
// that every attribute the plan shows as changed on this resource is a
// difference in how the value is written.
func rolledUp(f Finding) bool {
	_, ok := annotationFor(f, AnnAllRewritten)
	return ok
}

// assessUpdate assesses a single in-place update of the two maps given, so
// a table test can say what changed and nothing else.
func assessUpdate(before, after map[string]interface{}) Finding {
	return Assess(planOf(updateOf(before, after))).Findings[0]
}

// TestEachClassIsDetected is requirement one through four, one case each.
func TestEachClassIsDetected(t *testing.T) {
	cases := []struct {
		name          string
		before, after interface{}
		code          string
	}{
		{
			name:   "json object keys came back in a different order",
			before: `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Action":"s3:GetObject"}]}`,
			after:  `{"Statement":[{"Action":"s3:GetObject","Effect":"Allow"}],"Version":"2012-10-17"}`,
			code:   AnnSameJSON,
		},
		{
			name:   "json with the whitespace set differently",
			before: `{"a":1,"b":[2,3]}`,
			after:  "{\n  \"a\": 1,\n  \"b\": [2, 3]\n}",
			code:   AnnSameJSON,
		},
		{
			name:   "a trailing newline",
			before: "#!/bin/sh\nsystemctl restart app\n",
			after:  "#!/bin/sh\nsystemctl restart app",
			code:   AnnSameWhitespace,
		},
		{
			name:   "windows line endings",
			before: "line one\r\nline two",
			after:  "line one\nline two",
			code:   AnnSameWhitespace,
		},
		{
			name:   "indentation",
			before: "  key: value",
			after:  "key: value",
			code:   AnnSameWhitespace,
		},
		{
			name:   "a number became its string form",
			before: float64(80),
			after:  "80",
			code:   AnnSameNumber,
		},
		{
			name:   "a string became a number",
			before: "443",
			after:  float64(443),
			code:   AnnSameNumber,
		},
		{
			name:   "exponent notation",
			before: "1e3",
			after:  "1000",
			code:   AnnSameNumber,
		},
		{
			name:   "a trailing zero",
			before: "1",
			after:  "1.0",
			code:   AnnSameNumber,
		},
		{
			name:   "null became an empty list",
			before: nil,
			after:  []interface{}{},
			code:   AnnNullAndEmpty,
		},
		{
			name:   "an empty object became null",
			before: map[string]interface{}{},
			after:  nil,
			code:   AnnNullAndEmpty,
		},
		{
			name:   "null became an empty string",
			before: nil,
			after:  "",
			code:   AnnNullAndEmpty,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			f := assessUpdate(
				map[string]interface{}{"attr": c.before},
				map[string]interface{}{"attr": c.after},
			)
			got := pathsFor(f, c.code)
			if !reflect.DeepEqual(got, []string{"attr"}) {
				t.Errorf("%s paths = %v, want [attr]", c.code, got)
			}
		})
	}
}

// TestGenuineDifferencesAreNotReported is the negative case for every
// class, and it is the half that keeps the tool trustworthy. A rule that
// over-reports here tells a reviewer a real change is only a rewrite.
func TestGenuineDifferencesAreNotReported(t *testing.T) {
	cases := []struct {
		name          string
		before, after interface{}
	}{
		{
			name:   "json with a different value",
			before: `{"Effect":"Allow","Action":"s3:GetObject"}`,
			after:  `{"Effect":"Deny","Action":"s3:GetObject"}`,
		},
		{
			name:   "json that gained a key",
			before: `{"Effect":"Allow"}`,
			after:  `{"Effect":"Allow","Resource":"*"}`,
		},
		{
			name:   "a json array reordered, which is not a key order",
			before: `{"command":["run","--fast"]}`,
			after:  `{"command":["--fast","run"]}`,
		},
		{
			name:   "one side is not json at all",
			before: `{"a":1}`,
			after:  `not json`,
		},
		{
			name:   "text with a word changed",
			before: "systemctl restart app",
			after:  "systemctl restart api",
		},
		{
			name:   "text that gained a line",
			before: "one\ntwo",
			after:  "one\ntwo\nthree",
		},
		{
			name:   "a space that was deleted rather than resized",
			before: "a b",
			after:  "ab",
		},
		{
			name:   "different numbers",
			before: float64(80),
			after:  "443",
		},
		{
			name:   "a number that only looks close",
			before: "1.5",
			after:  "1.50001",
		},
		{
			name:   "a string that is not a number",
			before: "80",
			after:  "80/tcp",
		},
		{
			// strconv reads both of these as an infinity, which would have
			// the rule call two different strings the same number. Nothing
			// in a plan is legitimately an infinity, so they are refused.
			name:   "two spellings of infinity",
			before: "Inf",
			after:  "infinity",
		},
		{
			name:   "json against something that only looks like json",
			before: `{"a":1}`,
			after:  `{"a":1,}`,
		},
		{
			name:   "null became a list with something in it",
			before: nil,
			after:  []interface{}{"a"},
		},
		{
			name:   "an empty list became an empty object",
			before: []interface{}{},
			after:  map[string]interface{}{},
		},
		{
			name:   "an empty string became an empty list",
			before: "",
			after:  []interface{}{},
		},
		{
			name:   "false is not null",
			before: nil,
			after:  false,
		},
		{
			name:   "zero is not null",
			before: nil,
			after:  float64(0),
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			f := assessUpdate(
				map[string]interface{}{"attr": c.before},
				map[string]interface{}{"attr": c.after},
			)
			for _, code := range []string{AnnSameJSON, AnnSameWhitespace, AnnSameNumber, AnnNullAndEmpty} {
				if got := pathsFor(f, code); got != nil {
					t.Errorf("%s paths = %v, want none - this is a real difference", code, got)
				}
			}
			if rolledUp(f) {
				t.Error("the roll-up must not fire on a real difference")
			}
		})
	}
}

// TestPrecedenceIsOneClassPerAttribute pins the precedence rule. A pair
// that satisfies more than one class is reported once, under the narrowest
// class that is true of it, and never twice.
func TestPrecedenceIsOneClassPerAttribute(t *testing.T) {
	cases := []struct {
		name          string
		before, after interface{}
		want          string
	}{
		{
			// Both sides are JSON and the only difference is a trailing
			// newline, so both the whitespace class and the JSON class are
			// true of it. Whitespace is the narrower claim, so it wins:
			// "the only difference is whitespace" says more than "the keys
			// may have moved but the data matches".
			name:   "json that differs only in whitespace is whitespace, not json",
			before: "{\"a\":1,\"b\":2}\n",
			after:  `{"a":1,"b":2}`,
			want:   AnnSameWhitespace,
		},
		{
			// "1e3" and " 1000 " are the same number, and they are not the
			// same text with different whitespace, so the number class is
			// the only one that is true.
			name:   "a number wrapped in spaces is a number",
			before: "1e3",
			after:  " 1000 ",
			want:   AnnSameNumber,
		},
		{
			// An empty string on one side and a string of spaces on the
			// other is not null and empty, it is text laid out differently.
			name:   "blank against empty is whitespace, not null and empty",
			before: "",
			after:  "   ",
			want:   AnnSameWhitespace,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			f := assessUpdate(
				map[string]interface{}{"attr": c.before},
				map[string]interface{}{"attr": c.after},
			)
			var fired []string
			for _, code := range []string{AnnSameJSON, AnnSameWhitespace, AnnSameNumber, AnnNullAndEmpty} {
				if pathsFor(f, code) != nil {
					fired = append(fired, code)
				}
			}
			if !reflect.DeepEqual(fired, []string{c.want}) {
				t.Errorf("classes fired = %v, want exactly [%s]", fired, c.want)
			}
		})
	}
}

// TestReorderingTakesPrecedence is the other half of "do not double
// report". The reordering rule claims a list and stops at it, so nothing
// in this file may claim the same path or descend beneath it.
func TestReorderingTakesPrecedence(t *testing.T) {
	f := assessUpdate(
		map[string]interface{}{"blocks": []interface{}{
			map[string]interface{}{"doc": `{"a":1,"b":2}`},
			map[string]interface{}{"doc": `{"b":2,"a":1}`},
		}},
		map[string]interface{}{"blocks": []interface{}{
			map[string]interface{}{"doc": `{"b":2,"a":1}`},
			map[string]interface{}{"doc": `{"a":1,"b":2}`},
		}},
	)

	if got := pathsFor(f, AnnReordered); !reflect.DeepEqual(got, []string{"blocks"}) {
		t.Fatalf("reordered paths = %v, want [blocks]", got)
	}
	for _, code := range []string{AnnSameJSON, AnnSameWhitespace, AnnSameNumber, AnnNullAndEmpty} {
		if got := pathsFor(f, code); got != nil {
			t.Errorf("%s paths = %v, want none - the reordering rule already reported this attribute", code, got)
		}
	}
}

// TestRollUpFiresWhenEveryChangedAttributeIsARewrite is the feature's
// point: an update in place that is really nothing, said out loud.
func TestRollUpFiresWhenEveryChangedAttributeIsARewrite(t *testing.T) {
	f := assessUpdate(
		map[string]interface{}{
			"name":        "app",
			"policy":      `{"Version":"2012-10-17","Statement":[]}`,
			"user_data":   "#!/bin/sh\nrun\n",
			"port":        float64(80),
			"subnet_ids":  nil,
			"zones":       []interface{}{"a", "b"},
			"description": "unchanged",
		},
		map[string]interface{}{
			"name":        "app",
			"policy":      `{"Statement":[],"Version":"2012-10-17"}`,
			"user_data":   "#!/bin/sh\nrun",
			"port":        "80",
			"subnet_ids":  []interface{}{},
			"zones":       []interface{}{"b", "a"},
			"description": "unchanged",
		},
	)

	want := map[string][]string{
		AnnSameJSON:       {"policy"},
		AnnSameWhitespace: {"user_data"},
		AnnSameNumber:     {"port"},
		AnnNullAndEmpty:   {"subnet_ids"},
		AnnReordered:      {"zones"},
	}
	for code, paths := range want {
		if got := pathsFor(f, code); !reflect.DeepEqual(got, paths) {
			t.Errorf("%s paths = %v, want %v", code, got, paths)
		}
	}
	if !rolledUp(f) {
		t.Error("every changed attribute is a rewrite, so the roll-up must fire")
	}
}

// TestRollUpStaysQuietWhenOneRealChangeIsPresent is the most important
// test in the set.
//
// Four of the five changed attributes are rewrites and one is a real
// change. The per-attribute annotations must still be reported - they are
// each true - and the roll-up must not fire, because the sentence it says
// would be false about instance_type.
func TestRollUpStaysQuietWhenOneRealChangeIsPresent(t *testing.T) {
	f := assessUpdate(
		map[string]interface{}{
			"policy":        `{"Version":"2012-10-17","Statement":[]}`,
			"user_data":     "#!/bin/sh\nrun\n",
			"port":          float64(80),
			"subnet_ids":    nil,
			"instance_type": "t3.medium",
		},
		map[string]interface{}{
			"policy":        `{"Statement":[],"Version":"2012-10-17"}`,
			"user_data":     "#!/bin/sh\nrun",
			"port":          "80",
			"subnet_ids":    []interface{}{},
			"instance_type": "t3.large",
		},
	)

	for code, paths := range map[string][]string{
		AnnSameJSON:       {"policy"},
		AnnSameWhitespace: {"user_data"},
		AnnSameNumber:     {"port"},
		AnnNullAndEmpty:   {"subnet_ids"},
	} {
		if got := pathsFor(f, code); !reflect.DeepEqual(got, paths) {
			t.Errorf("%s paths = %v, want %v - each of these is still true", code, got, paths)
		}
	}
	if rolledUp(f) {
		t.Fatal("instance_type genuinely changed, so the roll-up must stay quiet")
	}
}

// TestRollUpStaysQuietWhenNothingChanged covers the empty denominator. A
// resource with no changed attribute at all has nothing for the roll-up to
// speak about, and "every changed attribute here is a rewrite" said over
// none of them is a sentence with no subject.
func TestRollUpStaysQuietWhenNothingChanged(t *testing.T) {
	f := assessUpdate(
		map[string]interface{}{"name": "app", "port": float64(80)},
		map[string]interface{}{"name": "app", "port": float64(80)},
	)
	if rolledUp(f) {
		t.Error("nothing changed, so the roll-up has nothing to say")
	}
}

// TestRollUpStaysQuietWhenAnAttributeCannotBeCompared covers the honest
// limit. An attribute that is not known until apply is shown as changed
// and cannot be held against its before, so the roll-up cannot speak for
// the resource even though every attribute it could compare is a rewrite.
func TestRollUpStaysQuietWhenAnAttributeCannotBeCompared(t *testing.T) {
	cases := []struct {
		name string
		rc   *tfjson.ResourceChange
	}{
		{
			name: "marked unknown on both sides",
			rc: func() *tfjson.ResourceChange {
				rc := updateOf(
					map[string]interface{}{"port": float64(80), "arn": "arn:aws:old"},
					map[string]interface{}{"port": "80", "arn": "arn:aws:new"},
				)
				rc.Change.AfterUnknown = map[string]interface{}{"arn": true}
				return rc
			}(),
		},
		{
			name: "dropped from the after entirely",
			rc: updateOf(
				map[string]interface{}{"port": float64(80), "endpoint": "old.example"},
				map[string]interface{}{"port": "80"},
			),
		},
		{
			name: "set on the after only",
			rc: updateOf(
				map[string]interface{}{"port": float64(80)},
				map[string]interface{}{"port": "80", "new_attribute": "set"},
			),
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			f := Assess(planOf(c.rc)).Findings[0]
			if got := pathsFor(f, AnnSameNumber); !reflect.DeepEqual(got, []string{"port"}) {
				t.Errorf("same-number paths = %v, want [port] - the comparable attribute is still reported", got)
			}
			if rolledUp(f) {
				t.Error("an attribute the plan shows as changed could not be compared, so the roll-up must stay quiet")
			}
		})
	}
}

// TestUnknownAttributeIsSkipped is requirement two. There is nothing to
// compare, so nothing is claimed - and a sibling that is comparable is
// still walked.
func TestUnknownAttributeIsSkipped(t *testing.T) {
	rc := updateOf(
		map[string]interface{}{"block": []interface{}{map[string]interface{}{
			"secret": float64(80),
			"port":   float64(443),
		}}},
		map[string]interface{}{"block": []interface{}{map[string]interface{}{
			"secret": "80",
			"port":   "443",
		}}},
	)
	rc.Change.AfterUnknown = map[string]interface{}{
		"block": []interface{}{map[string]interface{}{"secret": true}},
	}

	f := Assess(planOf(rc)).Findings[0]
	got := pathsFor(f, AnnSameNumber)
	if !reflect.DeepEqual(got, []string{"block[0].port"}) {
		t.Errorf("same-number paths = %v, want [block[0].port] - the unknown attribute is skipped, the known one is not", got)
	}
	if rolledUp(f) {
		t.Error("an unknown attribute cannot be accounted for, so the roll-up must stay quiet")
	}
}

// TestWholeAttributeUnknownIsSkipped covers the mark landing on a whole
// block rather than on a leaf inside it. Nothing beneath it is comparable
// either.
func TestWholeAttributeUnknownIsSkipped(t *testing.T) {
	rc := updateOf(
		map[string]interface{}{"block": []interface{}{map[string]interface{}{"port": float64(80)}}},
		map[string]interface{}{"block": []interface{}{map[string]interface{}{"port": "80"}}},
	)
	rc.Change.AfterUnknown = map[string]interface{}{"block": true}

	f := Assess(planOf(rc)).Findings[0]
	if got := pathsFor(f, AnnSameNumber); got != nil {
		t.Errorf("same-number paths = %v, want none - the whole block is unknown until apply", got)
	}
	if rolledUp(f) {
		t.Error("the only changed attribute is unknown until apply, so the roll-up must stay quiet")
	}
}

// TestNestedPathsAreNamedInFull checks the walk goes into blocks and names
// the path the way every other annotation in this package does.
func TestNestedPathsAreNamedInFull(t *testing.T) {
	f := assessUpdate(
		map[string]interface{}{"delegation": []interface{}{map[string]interface{}{
			"name":   "dlg",
			"policy": `{"a":1,"b":2}`,
			"weight": float64(1),
		}}},
		map[string]interface{}{"delegation": []interface{}{map[string]interface{}{
			"name":   "dlg",
			"policy": `{"b":2,"a":1}`,
			"weight": "1",
		}}},
	)

	if got := pathsFor(f, AnnSameJSON); !reflect.DeepEqual(got, []string{"delegation[0].policy"}) {
		t.Errorf("same-json paths = %v, want [delegation[0].policy]", got)
	}
	if got := pathsFor(f, AnnSameNumber); !reflect.DeepEqual(got, []string{"delegation[0].weight"}) {
		t.Errorf("same-number paths = %v, want [delegation[0].weight]", got)
	}
	if !rolledUp(f) {
		t.Error("both changed leaves are rewrites, so the roll-up must fire")
	}
}

// TestStructuralChangesAreNotRewrites covers the shapes the walk cannot
// pair up. A block that gained an element, a block that gained a key, and
// an attribute that changed type are each a change of shape, and none of
// them is a difference in how a value is written.
func TestStructuralChangesAreNotRewrites(t *testing.T) {
	cases := []struct {
		name          string
		before, after interface{}
	}{
		{
			name:   "a block gained an element",
			before: []interface{}{map[string]interface{}{"port": float64(80)}},
			after:  []interface{}{map[string]interface{}{"port": "80"}, map[string]interface{}{"port": "443"}},
		},
		{
			name:   "a block gained a key",
			before: map[string]interface{}{"port": float64(80)},
			after:  map[string]interface{}{"port": "80", "extra": "new"},
		},
		{
			name:   "a block lost a key",
			before: map[string]interface{}{"port": float64(80), "extra": "gone"},
			after:  map[string]interface{}{"port": "80"},
		},
		{
			name:   "a list became an object",
			before: []interface{}{"a"},
			after:  map[string]interface{}{"0": "a"},
		},
		{
			name:   "an object became a string",
			before: map[string]interface{}{"port": float64(80)},
			after:  "port=80",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			f := assessUpdate(
				map[string]interface{}{"attr": c.before},
				map[string]interface{}{"attr": c.after},
			)
			if rolledUp(f) {
				t.Error("the shape changed, so the roll-up must stay quiet")
			}
		})
	}
}

// TestRewritesStateTheFactNotAVerdict is the design constraint, asserted
// rather than left to a comment. Every one of these classes has a case
// where the difference is real, so no annotation may announce that the
// change is meaningless.
func TestRewritesStateTheFactNotAVerdict(t *testing.T) {
	f := assessUpdate(
		map[string]interface{}{
			"policy":     `{"a":1,"b":2}`,
			"user_data":  "run\n",
			"port":       float64(80),
			"subnet_ids": nil,
		},
		map[string]interface{}{
			"policy":     `{"b":2,"a":1}`,
			"user_data":  "run",
			"port":       "80",
			"subnet_ids": []interface{}{},
		},
	)

	codes := []string{AnnSameJSON, AnnSameWhitespace, AnnSameNumber, AnnNullAndEmpty, AnnAllRewritten}
	for _, code := range codes {
		a, ok := annotationFor(f, code)
		if !ok {
			t.Fatalf("expected a %s annotation", code)
		}
		for _, banned := range []string{"no change", "no semantic", "meaningless", "harmless", "safe to", "noise", "ignore", "nothing really"} {
			if strings.Contains(strings.ToLower(a.Detail), banned) {
				t.Errorf("%s must not pass judgement, got %q containing %q", code, a.Detail, banned)
			}
		}
		if !strings.Contains(strings.ToLower(a.Detail), "yours to judge") &&
			!strings.Contains(strings.ToLower(a.Detail), "rules on none") {
			t.Errorf("%s must hand the judgement back, got %q", code, a.Detail)
		}
	}
}

// TestRewritesCarryTheirLabelAndCaveatAsFields is the shape a renderer
// needs to stop repeating itself, the same shape the reordering rule
// carries. The caveat is the same sentence on every finding a class fires
// on, so a format with a footer has to be able to lift it out and state it
// once.
func TestRewritesCarryTheirLabelAndCaveatAsFields(t *testing.T) {
	f := assessUpdate(
		map[string]interface{}{
			"policy":     `{"a":1,"b":2}`,
			"user_data":  "run\n",
			"port":       float64(80),
			"subnet_ids": nil,
		},
		map[string]interface{}{
			"policy":     `{"b":2,"a":1}`,
			"user_data":  "run",
			"port":       "80",
			"subnet_ids": []interface{}{},
		},
	)

	for _, code := range []string{AnnSameJSON, AnnSameWhitespace, AnnSameNumber, AnnNullAndEmpty, AnnAllRewritten} {
		a, ok := annotationFor(f, code)
		if !ok {
			t.Fatalf("expected a %s annotation", code)
		}
		if a.Summary == "" {
			t.Errorf("%s: Summary is empty, so a terminal has no short label to set", code)
		}
		if a.Note == "" {
			t.Errorf("%s: Note is empty, so a renderer has no caveat to state once", code)
		}
		// Detail is what a JSON consumer reads with no footer in view, so
		// it has to carry the caveat itself.
		if !strings.Contains(a.Detail, a.Note) {
			t.Errorf("%s: Detail = %q must contain the caveat %q", code, a.Detail, a.Note)
		}
		// The caveat belongs to the rule, not to the label. A caveat welded
		// onto a label is what wrapped to three lines on every finding.
		if strings.Contains(a.Summary, a.Note) {
			t.Errorf("%s: Summary = %q, want the caveat kept out of the label", code, a.Summary)
		}
	}
}

// TestRewriteCaveatsCarryNoDash guards a wording choice that is easy to
// undo by accident. A hyphen stranded at the start of a wrapped terminal
// line reads as a bullet rather than as punctuation, and these caveats are
// long enough to wrap at every width the terminal supports.
func TestRewriteCaveatsCarryNoDash(t *testing.T) {
	f := assessUpdate(
		map[string]interface{}{
			"policy":     `{"a":1,"b":2}`,
			"user_data":  "run\n",
			"port":       float64(80),
			"subnet_ids": nil,
		},
		map[string]interface{}{
			"policy":     `{"b":2,"a":1}`,
			"user_data":  "run",
			"port":       "80",
			"subnet_ids": []interface{}{},
		},
	)

	for _, a := range f.Annotations {
		for _, dash := range []string{" - ", "–", "—"} {
			if strings.Contains(a.Note, dash) {
				t.Errorf("%s: Note = %q must not use %q - a wrapped line can strand it at column zero",
					a.Code, a.Note, dash)
			}
		}
	}
}

// TestRewritesNeverChangeTheLevel is the other half of the same
// constraint. An update stays low and a replacement stays high: these
// annotations add facts to read, never a score.
func TestRewritesNeverChangeTheLevel(t *testing.T) {
	cases := []struct {
		name    string
		actions []tfjson.Action
		want    Level
	}{
		{"update", []tfjson.Action{tfjson.ActionUpdate}, Low},
		{"replace", []tfjson.Action{tfjson.ActionDelete, tfjson.ActionCreate}, High},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			rc := change("azurerm_subnet.app", "azurerm_subnet", c.actions...)
			rc.Change.Before = map[string]interface{}{"policy": `{"a":1,"b":2}`}
			rc.Change.After = map[string]interface{}{"policy": `{"b":2,"a":1}`}
			f := Assess(planOf(rc)).Findings[0]

			if _, ok := annotationFor(f, AnnSameJSON); !ok {
				t.Fatal("expected a same-json annotation")
			}
			if !rolledUp(f) {
				t.Fatal("expected the roll-up")
			}
			if f.Level != c.want {
				t.Errorf("Level = %v, want %v - these annotations must never move a finding's level",
					f.Level, c.want)
			}
		})
	}
}

// TestCreateAndDeleteAreNotRewritten is requirement one from the other
// side. A create has no before and a delete has no after, so there is
// nothing to compare and nothing to say.
func TestCreateAndDeleteAreNotRewritten(t *testing.T) {
	for _, c := range []struct {
		name    string
		actions []tfjson.Action
	}{
		{"create", []tfjson.Action{tfjson.ActionCreate}},
		{"delete", []tfjson.Action{tfjson.ActionDelete}},
		{"no-op", []tfjson.Action{tfjson.ActionNoop}},
	} {
		t.Run(c.name, func(t *testing.T) {
			rc := change("azurerm_subnet.app", "azurerm_subnet", c.actions...)
			// Deliberately populated on both sides. Even given something to
			// compare, a change that is not an update or a replacement must
			// not be annotated.
			rc.Change.Before = map[string]interface{}{"policy": `{"a":1,"b":2}`}
			rc.Change.After = map[string]interface{}{"policy": `{"b":2,"a":1}`}
			f := Assess(planOf(rc)).Findings[0]

			if got := pathsFor(f, AnnSameJSON); got != nil {
				t.Errorf("same-json paths = %v, want none on a %s", got, c.name)
			}
			if rolledUp(f) {
				t.Errorf("the roll-up must not fire on a %s", c.name)
			}
		})
	}
}

func TestSeveralPathsInOneClassAreSorted(t *testing.T) {
	f := assessUpdate(
		map[string]interface{}{
			"zone_policy":   `{"a":1,"b":2}`,
			"access_policy": `{"a":1,"b":2}`,
			"bucket_policy": `{"a":1,"b":2}`,
		},
		map[string]interface{}{
			"zone_policy":   `{"b":2,"a":1}`,
			"access_policy": `{"b":2,"a":1}`,
			"bucket_policy": `{"b":2,"a":1}`,
		},
	)

	got := pathsFor(f, AnnSameJSON)
	want := []string{"access_policy", "bucket_policy", "zone_policy"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("same-json paths = %v, want %v in sorted order", got, want)
	}
}

// TestAbsentSidesAreIgnoredByRewrites covers the shapes that are not a
// pair of objects at all. A resource's before and after are always objects
// in real plan JSON, so these are guards rather than cases to reason about.
func TestAbsentSidesAreIgnoredByRewrites(t *testing.T) {
	cases := []struct {
		name          string
		before, after interface{}
	}{
		{"both nil", nil, nil},
		{"before nil", nil, map[string]interface{}{"a": ""}},
		{"after nil", map[string]interface{}{"a": ""}, nil},
		{"not objects", []interface{}{"a"}, []interface{}{"b"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := rewrittenPaths(c.before, c.after, nil, nil)
			if len(got.annotations()) != 0 {
				t.Errorf("annotations = %v, want none", got.annotations())
			}
		})
	}
}

// TestUnencodableValueIsNotARewrite covers the guard on json.Marshal, on
// each side in turn. A value that came out of json.Unmarshal can always be
// marshalled again, so this is unreachable through the loader, but a
// comparison that cannot be made must produce silence, never a guess.
func TestUnencodableValueIsNotARewrite(t *testing.T) {
	ch := make(chan int)
	cases := []struct {
		name          string
		before, after interface{}
	}{
		{"in the before", ch, "x"},
		{"in the after", "x", ch},
		{"on both sides", ch, ch},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := rewrittenPaths(
				map[string]interface{}{"a": c.before},
				map[string]interface{}{"a": c.after},
				nil, nil,
			)
			if got.allWritten {
				t.Error("a value that cannot be encoded cannot be compared, so the roll-up must stay quiet")
			}
			if len(got.annotations()) != 0 {
				t.Errorf("annotations = %v, want none", got.annotations())
			}
		})
	}
}

// TestRewriteOfRefusesIdenticalValues is the guard at the top of the
// classifier. The walk skips a pair that is already equal before it gets
// there, so this is only reachable by calling in directly - but a class is
// a statement about a difference, and there is no difference here to be a
// class of.
func TestRewriteOfRefusesIdenticalValues(t *testing.T) {
	for _, v := range []interface{}{
		"same",
		float64(80),
		nil,
		[]interface{}{},
		map[string]interface{}{"a": float64(1)},
		`{"a":1,"b":2}`,
	} {
		if got := rewriteOf(v, v); got != notRewritten {
			t.Errorf("rewriteOf(%v, %v) = %v, want notRewritten", v, v, got)
		}
	}
}

// TestRewritesNeverPrintAValue is the standing guarantee, the same one the
// sensitive and reordering annotations carry. Comparing values is
// necessary and happens internally; only the path is ever named.
func TestRewritesNeverPrintAValue(t *testing.T) {
	const secret = "AKIAIOSFODNN7EXAMPLE-do-not-print-this"
	f := assessUpdate(
		map[string]interface{}{
			"policy":    `{"key":"` + secret + `","Version":"2012-10-17"}`,
			"user_data": "export TOKEN=" + secret + "\n",
		},
		map[string]interface{}{
			"policy":    `{"Version":"2012-10-17","key":"` + secret + `"}`,
			"user_data": "export TOKEN=" + secret,
		},
	)

	for _, a := range f.Annotations {
		for _, s := range append(append([]string{}, a.Paths...), a.Detail, a.Summary, a.Note, a.Code) {
			if strings.Contains(s, secret) {
				t.Errorf("%s: an attribute value reached the output: %q", a.Code, s)
			}
		}
	}
	if !rolledUp(f) {
		t.Error("the fixture must exercise the rule, not sidestep it")
	}
}

// TestRewrittenFixture runs the rules over real-shaped plan JSON rather
// than over Go maps built by hand, which is the only way to catch an
// assumption the hand-written cases share with the code.
func TestRewrittenFixture(t *testing.T) {
	r := Assess(loadFixture(t, "written-differently.json"))

	want := map[string]map[string][]string{
		"aws_iam_policy.pipeline": {
			AnnSameJSON: {"policy"},
		},
		"aws_ecs_task_definition.api": {
			AnnSameWhitespace: {"container_definitions"},
		},
		"aws_security_group.web": {
			AnnSameNumber: {"ingress[0].from_port", "ingress[0].to_port"},
			AnnReordered:  {"ingress[0].cidr_blocks"},
		},
		"aws_lb_target_group.app": {
			AnnNullAndEmpty: {"load_balancing_anomaly_mitigation", "tags"},
		},
		"aws_instance.bastion": {
			AnnSameWhitespace: {"user_data"},
			AnnSameJSON:       {"metadata"},
		},
	}

	if len(r.Findings) != len(want) {
		t.Fatalf("got %d findings, want %d", len(r.Findings), len(want))
	}
	for _, f := range r.Findings {
		w, ok := want[f.Address]
		if !ok {
			t.Errorf("unexpected finding %s", f.Address)
			continue
		}
		for _, code := range []string{AnnSameJSON, AnnSameWhitespace, AnnSameNumber, AnnNullAndEmpty, AnnReordered} {
			got := pathsFor(f, code)
			if !reflect.DeepEqual(got, w[code]) {
				t.Errorf("%s: %s paths = %v, want %v", f.Address, code, got, w[code])
			}
		}
	}
}

// TestRewrittenFixtureRollUp is the fixture's point. Four resources are
// rewrites from end to end and say so; aws_instance.bastion has a genuine
// instance_type change sitting alongside two rewrites, and it must stay
// quiet.
func TestRewrittenFixtureRollUp(t *testing.T) {
	r := Assess(loadFixture(t, "written-differently.json"))

	want := map[string]bool{
		"aws_iam_policy.pipeline":     true,
		"aws_ecs_task_definition.api": true,
		"aws_security_group.web":      true,
		"aws_lb_target_group.app":     true,
		"aws_instance.bastion":        false,
	}
	for _, f := range r.Findings {
		if got := rolledUp(f); got != want[f.Address] {
			t.Errorf("%s: roll-up = %v, want %v", f.Address, got, want[f.Address])
		}
	}
}

// TestRewrittenFixtureLevelsAreUntouched pins the other half. These
// annotations are attached to five low updates and move none of them.
func TestRewrittenFixtureLevelsAreUntouched(t *testing.T) {
	for _, f := range Assess(loadFixture(t, "written-differently.json")).Findings {
		if f.Level != Low {
			t.Errorf("%s: Level = %v, want low - no rewrite annotation may move a level", f.Address, f.Level)
		}
	}
}
