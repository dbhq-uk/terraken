package assess

import (
	"encoding/json"
	"os"
	"reflect"
	"strings"
	"testing"

	tfjson "github.com/hashicorp/terraform-json"
)

// updateOf builds an update whose before and after are the maps given, so
// a table test can say what changed and nothing else.
func updateOf(before, after map[string]interface{}) *tfjson.ResourceChange {
	rc := change("azurerm_subnet.app", "azurerm_subnet", tfjson.ActionUpdate)
	rc.Change.Before = before
	rc.Change.After = after
	return rc
}

// reorderedFor is the paths of the same-elements-reordered annotation on
// the first finding, or nil when there is no such annotation.
func reorderedFor(f Finding) []string {
	a, ok := annotationFor(f, AnnReordered)
	if !ok {
		return nil
	}
	return a.Paths
}

func TestReorderedListIsDetected(t *testing.T) {
	r := Assess(planOf(updateOf(
		map[string]interface{}{"service_endpoints": []interface{}{"Microsoft.Storage", "Microsoft.Sql", "Microsoft.KeyVault"}},
		map[string]interface{}{"service_endpoints": []interface{}{"Microsoft.KeyVault", "Microsoft.Storage", "Microsoft.Sql"}},
	)))

	got := reorderedFor(r.Findings[0])
	want := []string{"service_endpoints"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Paths = %v, want %v", got, want)
	}
}

// TestReorderedAnnotationStatesTheFactNotAVerdict is the design
// constraint, asserted rather than left to a comment. Order is genuinely
// significant for some attributes - a container command, an ordered
// listener rule - so the annotation must say what it saw and hand the
// judgement back, never announce that the change is meaningless.
func TestReorderedAnnotationStatesTheFactNotAVerdict(t *testing.T) {
	r := Assess(planOf(updateOf(
		map[string]interface{}{"service_endpoints": []interface{}{"a", "b"}},
		map[string]interface{}{"service_endpoints": []interface{}{"b", "a"}},
	)))

	a, ok := annotationFor(r.Findings[0], AnnReordered)
	if !ok {
		t.Fatal("expected a same-elements-reordered annotation")
	}
	for _, banned := range []string{"no change", "no semantic", "meaningless", "safe", "noise", "ignore"} {
		if strings.Contains(strings.ToLower(a.Detail), banned) {
			t.Errorf("the annotation must not pass judgement, got %q containing %q", a.Detail, banned)
		}
	}
	if !strings.Contains(strings.ToLower(a.Detail), "order is significant") {
		t.Errorf("the annotation must say order is significant for some attributes, got %q", a.Detail)
	}
}

// TestReorderedAnnotationDoesNotChangeTheLevel is the other half of the
// same constraint. An update stays low and a replacement stays high: this
// annotation adds a fact to read, never a score.
func TestReorderedAnnotationDoesNotChangeTheLevel(t *testing.T) {
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
			rc.Change.Before = map[string]interface{}{"service_endpoints": []interface{}{"a", "b"}}
			rc.Change.After = map[string]interface{}{"service_endpoints": []interface{}{"b", "a"}}
			r := Assess(planOf(rc))

			if _, ok := annotationFor(r.Findings[0], AnnReordered); !ok {
				t.Fatal("expected a same-elements-reordered annotation")
			}
			if r.Findings[0].Level != c.want {
				t.Errorf("Level = %v, want %v - this annotation must never move a finding's level",
					r.Findings[0].Level, c.want)
			}
		})
	}
}

func TestIdenticalListIsNotReported(t *testing.T) {
	r := Assess(planOf(updateOf(
		map[string]interface{}{"service_endpoints": []interface{}{"a", "b", "c"}, "name": "app"},
		map[string]interface{}{"service_endpoints": []interface{}{"a", "b", "c"}, "name": "web"},
	)))

	if got := reorderedFor(r.Findings[0]); got != nil {
		t.Errorf("Paths = %v, want none - an unchanged list is not a reordering", got)
	}
}

func TestDifferentElementsAreNotReported(t *testing.T) {
	r := Assess(planOf(updateOf(
		map[string]interface{}{"address_prefixes": []interface{}{"10.0.1.0/24"}},
		map[string]interface{}{"address_prefixes": []interface{}{"10.0.2.0/24"}},
	)))

	if got := reorderedFor(r.Findings[0]); got != nil {
		t.Errorf("Paths = %v, want none - the elements themselves changed", got)
	}
}

// TestDifferentMultiplicityIsNotReported is why the comparison counts
// elements instead of collecting them into a set. ["a","a","b"] and
// ["a","b","b"] hold the same distinct values and are not the same list.
func TestDifferentMultiplicityIsNotReported(t *testing.T) {
	r := Assess(planOf(updateOf(
		map[string]interface{}{"tags": []interface{}{"a", "a", "b"}},
		map[string]interface{}{"tags": []interface{}{"a", "b", "b"}},
	)))

	if got := reorderedFor(r.Findings[0]); got != nil {
		t.Errorf("Paths = %v, want none - duplicates count, so these are different lists", got)
	}
}

func TestDifferentLengthIsNotReported(t *testing.T) {
	r := Assess(planOf(updateOf(
		map[string]interface{}{"subnet_ids": []interface{}{"a", "b"}},
		map[string]interface{}{"subnet_ids": []interface{}{"b", "a", "c"}},
	)))

	if got := reorderedFor(r.Findings[0]); got != nil {
		t.Errorf("Paths = %v, want none - a list that grew is not a reordering", got)
	}
}

// TestNestedListInsideAnObjectIsDetected checks the walk goes into nested
// blocks and names the full path the same way every other annotation in
// this package does.
func TestNestedListInsideAnObjectIsDetected(t *testing.T) {
	r := Assess(planOf(updateOf(
		map[string]interface{}{"delegation": []interface{}{
			map[string]interface{}{
				"name":    "dlg",
				"actions": []interface{}{"Microsoft.Network/virtualNetworks/subnets/join/action", "Microsoft.Network/virtualNetworks/subnets/action"},
			},
		}},
		map[string]interface{}{"delegation": []interface{}{
			map[string]interface{}{
				"name":    "dlg",
				"actions": []interface{}{"Microsoft.Network/virtualNetworks/subnets/action", "Microsoft.Network/virtualNetworks/subnets/join/action"},
			},
		}},
	)))

	got := reorderedFor(r.Findings[0])
	want := []string{"delegation[0].actions"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Paths = %v, want %v", got, want)
	}
}

// TestReorderedListOfObjectsIsDetected checks the elements can be objects
// rather than strings, and that a reordering is reported at the list
// itself rather than as a difference inside each element.
func TestReorderedListOfObjectsIsDetected(t *testing.T) {
	rule := func(port float64, cidr string) interface{} {
		return map[string]interface{}{"from_port": port, "cidr_blocks": []interface{}{cidr}}
	}
	r := Assess(planOf(updateOf(
		map[string]interface{}{"ingress": []interface{}{rule(80, "10.0.0.0/8"), rule(443, "10.0.0.0/8")}},
		map[string]interface{}{"ingress": []interface{}{rule(443, "10.0.0.0/8"), rule(80, "10.0.0.0/8")}},
	)))

	got := reorderedFor(r.Findings[0])
	want := []string{"ingress"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Paths = %v, want %v", got, want)
	}
}

func TestNonListAttributesAreIgnored(t *testing.T) {
	r := Assess(planOf(updateOf(
		map[string]interface{}{"name": "app", "enabled": true, "count": float64(2), "tags": map[string]interface{}{"env": "dev"}},
		map[string]interface{}{"name": "web", "enabled": false, "count": float64(3), "tags": map[string]interface{}{"env": "prod"}},
	)))

	if got := reorderedFor(r.Findings[0]); got != nil {
		t.Errorf("Paths = %v, want none - only lists can be reordered", got)
	}
}

// TestUnknownUntilApplyIsSkipped covers an attribute Terraform has marked
// wholly unknown. There is no after to compare, so any apparent ordering
// is an artefact of the placeholder, not evidence.
func TestUnknownUntilApplyIsSkipped(t *testing.T) {
	rc := updateOf(
		map[string]interface{}{"subnet_ids": []interface{}{"a", "b"}},
		map[string]interface{}{"subnet_ids": []interface{}{"b", "a"}},
	)
	rc.Change.AfterUnknown = map[string]interface{}{"subnet_ids": true}
	r := Assess(planOf(rc))

	if got := reorderedFor(r.Findings[0]); got != nil {
		t.Errorf("Paths = %v, want none - the attribute is not known until apply", got)
	}
}

// TestUnknownElementIsSkippedButSiblingsAreStillWalked checks the unknown
// mark is honoured at the list it actually covers, without silencing a
// nested list that is perfectly comparable.
func TestUnknownElementIsSkippedButSiblingsAreStillWalked(t *testing.T) {
	rc := updateOf(
		map[string]interface{}{"block": []interface{}{map[string]interface{}{
			"ids":     []interface{}{"a", "b"},
			"regions": []interface{}{"uksouth", "ukwest"},
		}}},
		map[string]interface{}{"block": []interface{}{map[string]interface{}{
			"ids":     []interface{}{"b", "a"},
			"regions": []interface{}{"ukwest", "uksouth"},
		}}},
	)
	rc.Change.AfterUnknown = map[string]interface{}{
		"block": []interface{}{map[string]interface{}{"ids": true}},
	}
	r := Assess(planOf(rc))

	got := reorderedFor(r.Findings[0])
	want := []string{"block[0].regions"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Paths = %v, want %v - the unknown list is skipped, the known one is not", got, want)
	}
}

// TestMixedTypesAreComparedExactly is the lesson this codebase already
// learned once in renderAttrs: fmt's %v renders the number 15 and the
// string "15" identically, so comparing rendered text that way invents
// matches that are not there. Encoding each element as JSON keeps them
// apart.
func TestMixedTypesAreComparedExactly(t *testing.T) {
	cases := []struct {
		name   string
		before []interface{}
		after  []interface{}
		want   []string
	}{
		{
			name:   "a number and its string form swap places",
			before: []interface{}{float64(15), "15"},
			after:  []interface{}{"15", float64(15)},
			want:   []string{"mixed"},
		},
		{
			name:   "a number became a string",
			before: []interface{}{float64(15), float64(15)},
			after:  []interface{}{"15", float64(15)},
			want:   nil,
		},
		{
			name:   "a bool became its string form",
			before: []interface{}{true, "x"},
			after:  []interface{}{"true", "x"},
			want:   nil,
		},
		{
			name:   "null is not the string nil",
			before: []interface{}{nil, "x"},
			after:  []interface{}{"<nil>", "x"},
			want:   nil,
		},
		{
			name:   "null and a value swap places",
			before: []interface{}{nil, "x"},
			after:  []interface{}{"x", nil},
			want:   []string{"mixed"},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r := Assess(planOf(updateOf(
				map[string]interface{}{"mixed": c.before},
				map[string]interface{}{"mixed": c.after},
			)))
			if got := reorderedFor(r.Findings[0]); !reflect.DeepEqual(got, c.want) {
				t.Errorf("Paths = %v, want %v", got, c.want)
			}
		})
	}
}

// TestCreateAndDeleteAreNotAnnotated covers requirement one from the other
// side: a create has no before and a delete has no after, so there is
// nothing to compare and nothing to say.
func TestCreateAndDeleteAreNotAnnotated(t *testing.T) {
	cases := []struct {
		name    string
		actions []tfjson.Action
	}{
		{"create", []tfjson.Action{tfjson.ActionCreate}},
		{"delete", []tfjson.Action{tfjson.ActionDelete}},
		{"no-op", []tfjson.Action{tfjson.ActionNoop}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			rc := change("azurerm_subnet.app", "azurerm_subnet", c.actions...)
			// Deliberately populated on both sides. Even given something
			// to compare, a change that is not an update or a replacement
			// must not be annotated.
			rc.Change.Before = map[string]interface{}{"service_endpoints": []interface{}{"a", "b"}}
			rc.Change.After = map[string]interface{}{"service_endpoints": []interface{}{"b", "a"}}
			r := Assess(planOf(rc))

			if got := reorderedFor(r.Findings[0]); got != nil {
				t.Errorf("Paths = %v, want none on a %s", got, c.name)
			}
		})
	}
}

func TestSeveralReorderedPathsAreSorted(t *testing.T) {
	r := Assess(planOf(updateOf(
		map[string]interface{}{
			"zones":             []interface{}{"1", "2"},
			"service_endpoints": []interface{}{"a", "b"},
			"address_prefixes":  []interface{}{"x", "y"},
		},
		map[string]interface{}{
			"zones":             []interface{}{"2", "1"},
			"service_endpoints": []interface{}{"b", "a"},
			"address_prefixes":  []interface{}{"y", "x"},
		},
	)))

	got := reorderedFor(r.Findings[0])
	want := []string{"address_prefixes", "service_endpoints", "zones"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Paths = %v, want %v in sorted order", got, want)
	}
}

// TestReorderedNeverPrintsAValue is the standing guarantee, the same one
// the sensitive annotation carries. Comparing values is necessary and
// happens internally; only the path is ever named.
func TestReorderedNeverPrintsAValue(t *testing.T) {
	const secret = "AKIAIOSFODNN7EXAMPLE-do-not-print-this"
	r := Assess(planOf(updateOf(
		map[string]interface{}{"access_keys": []interface{}{secret, "second-element"}},
		map[string]interface{}{"access_keys": []interface{}{"second-element", secret}},
	)))

	a, ok := annotationFor(r.Findings[0], AnnReordered)
	if !ok {
		t.Fatal("expected a same-elements-reordered annotation")
	}
	if strings.Contains(a.Detail, secret) {
		t.Errorf("a value must never reach the output, got detail %q", a.Detail)
	}
	for _, p := range a.Paths {
		if strings.Contains(p, secret) {
			t.Errorf("a value must never reach the output, got path %q", p)
		}
	}
	if a.Code != AnnReordered {
		t.Errorf("Code = %q, want %q", a.Code, AnnReordered)
	}
}

// TestAbsentSidesAreIgnored covers the shapes that are not a pair of
// objects at all. A resource's before and after are always objects in real
// plan JSON, so these are guards rather than cases to reason about.
func TestAbsentSidesAreIgnored(t *testing.T) {
	cases := []struct {
		name          string
		before, after interface{}
	}{
		{"both nil", nil, nil},
		{"before nil", nil, map[string]interface{}{"a": []interface{}{"x", "y"}}},
		{"after nil", map[string]interface{}{"a": []interface{}{"x", "y"}}, nil},
		{"not objects", []interface{}{"a", "b"}, []interface{}{"b", "a"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := reorderedPaths(c.before, c.after, nil); got != nil {
				t.Errorf("reorderedPaths = %v, want none", got)
			}
		})
	}
}

// TestAttributeMissingFromOneSideIsIgnored covers a key present in the
// before and gone from the after, which Terraform's omitUnknowns produces
// for a computed attribute. There is no pair, so there is nothing to
// compare.
func TestAttributeMissingFromOneSideIsIgnored(t *testing.T) {
	got := reorderedPaths(
		map[string]interface{}{"gone": []interface{}{"a", "b"}},
		map[string]interface{}{"other": []interface{}{"b", "a"}},
		nil,
	)
	if got != nil {
		t.Errorf("reorderedPaths = %v, want none", got)
	}
}

// TestTypeChangedBetweenSidesIsIgnored covers an attribute that is a list
// on one side and something else on the other. That is a change of shape,
// not a reordering.
func TestTypeChangedBetweenSidesIsIgnored(t *testing.T) {
	got := reorderedPaths(
		map[string]interface{}{"a": []interface{}{"x", "y"}, "b": map[string]interface{}{"k": "v"}},
		map[string]interface{}{"a": "x,y", "b": []interface{}{"k", "v"}},
		nil,
	)
	if got != nil {
		t.Errorf("reorderedPaths = %v, want none", got)
	}
}

// TestUnencodableElementIsSkipped covers the guard on json.Marshal. A
// value that came out of json.Unmarshal can always be marshalled again, so
// this is unreachable through the loader - but a comparison that cannot be
// made must produce silence, never a guess.
func TestUnencodableElementIsSkipped(t *testing.T) {
	ch := make(chan int)
	cases := []struct {
		name          string
		before, after []interface{}
	}{
		{"in the before", []interface{}{ch, "x"}, []interface{}{"x", "y"}},
		{"in the after", []interface{}{"x", "y"}, []interface{}{"y", ch}},
		{"on both sides", []interface{}{ch, "x"}, []interface{}{"x", ch}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := reorderedPaths(
				map[string]interface{}{"a": c.before},
				map[string]interface{}{"a": c.after},
				nil,
			)
			if got != nil {
				t.Errorf("reorderedPaths = %v, want none - an element that cannot be encoded cannot be compared", got)
			}
		})
	}
}

// loadFixture reads a committed plan fixture off disk.
func loadFixture(t *testing.T, name string) *tfjson.Plan {
	t.Helper()
	path := "../../testdata/" + name
	b, err := os.ReadFile(path)
	if err != nil {
		// Fatal, not Skip. The fixture is committed, so it is never
		// legitimately absent, and skipping on a missing file means
		// deleting it turns the tests green instead of red.
		t.Fatalf("committed fixture %s is missing: %v", path, err)
	}
	var p tfjson.Plan
	if err := json.Unmarshal(b, &p); err != nil {
		t.Fatalf("fixture %s did not parse: %v", path, err)
	}
	return &p
}

// TestReorderedFixture runs the rule over real-shaped plan JSON rather
// than over Go maps built by hand, which is the only way to catch an
// assumption the hand-written cases share with the code - a number
// arriving as float64, a nested block arriving as a list of one.
//
// The fixture also carries the cases that must stay silent:
// address_prefixes gained an element, so it is a change and not a
// reordering, and aws_ecs_task_definition.worker's command is reordered
// and is annotated exactly like the rest, because a reordered container
// command is a real behaviour change and this rule does not pretend to
// know which is which.
func TestReorderedFixture(t *testing.T) {
	r := Assess(loadFixture(t, "reordered.json"))

	want := map[string][]string{
		"aws_db_instance.main":           {"vpc_security_group_ids"},
		"azurerm_subnet.app":             {"delegation[0].service_delegation[0].actions", "service_endpoints"},
		"aws_security_group.web":         {"ingress[0].cidr_blocks"},
		"aws_ecs_task_definition.worker": {"command"},
	}
	if len(r.Findings) != len(want) {
		t.Fatalf("got %d findings, want %d", len(r.Findings), len(want))
	}
	for _, f := range r.Findings {
		got := reorderedFor(f)
		if !reflect.DeepEqual(got, want[f.Address]) {
			t.Errorf("%s: Paths = %v, want %v", f.Address, got, want[f.Address])
		}
	}
}

// TestReorderedFixtureLevelsAreUntouched pins the other half. The
// annotation is attached to a critical replacement and to three low
// updates, and it moves neither.
func TestReorderedFixtureLevelsAreUntouched(t *testing.T) {
	r := Assess(loadFixture(t, "reordered.json"))

	want := map[string]Level{
		"aws_db_instance.main":           Critical,
		"azurerm_subnet.app":             Low,
		"aws_security_group.web":         Low,
		"aws_ecs_task_definition.worker": Low,
	}
	for _, f := range r.Findings {
		if f.Level != want[f.Address] {
			t.Errorf("%s: Level = %v, want %v", f.Address, f.Level, want[f.Address])
		}
	}
}

// TestSecretsFixtureNeverLeaksAValue is the assess-side half of the
// no-values guarantee. The fixture's list elements are obvious secrets
// that Terraform has not marked sensitive, which is the case the README
// warns about. The rule reads every one of them to answer the question
// and must carry none of them out.
func TestSecretsFixtureNeverLeaksAValue(t *testing.T) {
	r := Assess(loadFixture(t, "reordered-secrets.json"))

	var annotated int
	for _, f := range r.Findings {
		for _, a := range f.Annotations {
			for _, s := range append(a.Paths, a.Detail, a.Code) {
				if strings.Contains(s, "LEAKED") {
					t.Errorf("%s: an attribute value reached the annotation: %q", f.Address, s)
				}
			}
		}
		if len(reorderedFor(f)) > 0 {
			annotated++
		}
	}
	if annotated != 2 {
		t.Errorf("%d findings annotated, want 2 - the fixture must exercise the rule, not sidestep it", annotated)
	}
}
