package assess

import (
	"strings"
	"testing"

	tfjson "github.com/hashicorp/terraform-json"
)

// The failure this file exists to prevent: an action, or an action
// sequence, that the classifier does not recognise falling through and
// being reported as a harmless no-op. An unknown reported as an unknown is
// the feature; an unknown quietly rendered as no change is the bug.

func TestAnUnrecognisedActionIsNotReportedAsANoOp(t *testing.T) {
	cases := []struct {
		name    string
		actions []tfjson.Action
	}{
		{"an action verb this build has never seen", []tfjson.Action{"reconcile"}},
		{"a known verb in an unknown combination", []tfjson.Action{tfjson.ActionCreate, tfjson.ActionUpdate}},
		{"three actions, which is not a shape terraform documents", []tfjson.Action{tfjson.ActionDelete, tfjson.ActionCreate, tfjson.ActionDelete}},
		{"an unknown verb beside a known one", []tfjson.Action{tfjson.ActionDelete, "quarantine"}},
		{"no actions at all", nil},
		{"the same known verb twice", []tfjson.Action{tfjson.ActionUpdate, tfjson.ActionUpdate}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r := Assess(planOf(change("azurerm_virtual_network.a", "azurerm_virtual_network", c.actions...)))
			if len(r.Findings) != 1 {
				t.Fatalf("expected 1 finding, got %d", len(r.Findings))
			}
			f := r.Findings[0]
			if f.Kind != KindUnsupported {
				t.Errorf("Kind = %q, want %q", f.Kind, KindUnsupported)
			}
			if f.Level != Unranked {
				t.Errorf("Level = %v, want Unranked - the tool cannot rank an operation it does not understand", f.Level)
			}
			if f.LevelName != "unranked" {
				t.Errorf("LevelName = %q, want %q", f.LevelName, "unranked")
			}
			if _, ok := annotationFor(f, AnnUnsupportedAction); !ok {
				t.Fatalf("expected an %s annotation, got %+v", AnnUnsupportedAction, f.Annotations)
			}
		})
	}
}

// The tool must name the vocabulary it did not recognise. Those are
// Terraform's own action strings, not attribute values, and a reader who is
// told only "unsupported" cannot look anything up.
func TestAnUnsupportedOperationNamesTheActionsItDidNotRecognise(t *testing.T) {
	r := Assess(planOf(change("azurerm_virtual_network.a", "azurerm_virtual_network",
		tfjson.ActionDelete, "quarantine")))
	a, ok := annotationFor(r.Findings[0], AnnUnsupportedAction)
	if !ok {
		t.Fatal("expected an unsupported-operation annotation")
	}
	want := []string{`"delete"`, `"quarantine"`}
	if len(a.Paths) != len(want) {
		t.Fatalf("Paths = %v, want the whole action sequence %v", a.Paths, want)
	}
	for i, p := range a.Paths {
		if p != want[i] {
			t.Errorf("Paths[%d] = %q, want %q", i, p, want[i])
		}
	}
	if !strings.Contains(a.Detail, "cannot be assessed") {
		t.Errorf("Detail must say the impact cannot be assessed, got %q", a.Detail)
	}
}

// An action string is read out of the plan file, which is untrusted input,
// and the terminal renderer has no escaping of its own. Quoting at the
// point of classification is what stops an escape sequence, a newline or a
// tab reaching any output format as itself.
func TestAnUnsupportedActionIsQuotedSoAControlCharacterCannotReachTheOutput(t *testing.T) {
	r := Assess(planOf(change("azurerm_virtual_network.a", "azurerm_virtual_network",
		tfjson.Action("\x1b[2Jwipe\nthe screen")))) //nolint:staticcheck // deliberately hostile
	a, ok := annotationFor(r.Findings[0], AnnUnsupportedAction)
	if !ok {
		t.Fatal("expected an unsupported-operation annotation")
	}
	for _, p := range a.Paths {
		if strings.ContainsAny(p, "\x1b\n\r\t") {
			t.Errorf("a raw control character survived into %q", p)
		}
	}
	if !strings.Contains(strings.Join(a.Paths, " "), `\x1b`) {
		t.Errorf("expected the escape to be shown as an escape, got %v", a.Paths)
	}
}

// An unrecognised operation sorts above everything the tool CAN rank, so a
// reviewer meets it before anything else and no display filter can hide it.
func TestAnUnsupportedOperationSortsAboveCriticalAndSurvivesEveryFilter(t *testing.T) {
	r := Assess(planOf(
		change("azurerm_mssql_database.db", "azurerm_mssql_database", tfjson.ActionDelete),
		change("azurerm_virtual_network.a", "azurerm_virtual_network", "reconcile"),
	))
	if len(r.Findings) != 2 {
		t.Fatalf("expected 2 findings, got %d", len(r.Findings))
	}
	if r.Findings[0].Kind != KindUnsupported {
		t.Errorf("the unrankable finding must come first, got %q", r.Findings[0].Kind)
	}
	if r.Findings[1].Level != Critical {
		t.Fatalf("expected the database delete to be critical, got %v", r.Findings[1].Level)
	}

	filtered := r.AtLeast(Critical)
	if len(filtered.Findings) != 2 {
		t.Fatalf("--min-level critical held back %d findings; an unrankable one must never be hidden",
			filtered.Hidden)
	}
}

// The same guarantee has to survive a team rule, and a team rule can be
// broad. "All terraform_data changes are info here" is a reasonable thing to
// write, and it must not turn --min-level high into a way of never being
// told the tool could not read the plan. Ranking something is not
// understanding it, so the filter keeps the finding on the kind rather than
// on the level it ended up at.
func TestAFilterCannotHideAnUnreadableOperationARuleRankedDown(t *testing.T) {
	rs := loadRulesFrom(t, `{"rules":[{
		"id":"terraform-data-is-noise","message":"terraform_data changes are routine here",
		"level":"info","when":{"types":["terraform_data"]}}]}`)

	r := AssessWithRules(planOf(
		change("terraform_data.a", "terraform_data", "reconcile"),
		change("azurerm_virtual_network.b", "azurerm_virtual_network", tfjson.ActionUpdate),
	), rs)

	var unsupported Finding
	for _, f := range r.Findings {
		if f.Kind == KindUnsupported {
			unsupported = f
		}
	}
	if unsupported.LevelName != "info" {
		t.Fatalf("LevelName = %q, want info - the rule set it", unsupported.LevelName)
	}

	filtered := r.AtLeast(High)
	var kept bool
	for _, f := range filtered.Findings {
		if f.Kind == KindUnsupported {
			kept = true
		}
	}
	if !kept {
		t.Error("--min-level high hid an operation the tool could not read")
	}
	if filtered.Hidden != 1 {
		t.Errorf("Hidden = %d, want 1 - only the ordinary update should have gone", filtered.Hidden)
	}
}

// design.md decides this case by name: "an action the tool does not
// recognise ... is not a data loss, so it may not be reported as critical.
// If it is not one resource change losing data, it belongs outside the
// severity counts rather than at the top of them."
//
// So it is counted apart, and --fail-on - which is a severity threshold and
// nothing else - cannot see it. A team that wants to block on one writes a
// rule, which is visible and auditable, rather than having the meaning of a
// pinned --fail-on change underneath them.
func TestAnUnsupportedOperationStaysOutOfTheSeverityCounts(t *testing.T) {
	r := Assess(planOf(
		change("azurerm_virtual_network.a", "azurerm_virtual_network", "reconcile"),
		change("azurerm_virtual_network.b", "azurerm_virtual_network", tfjson.ActionUpdate),
	))

	if r.Unassessed != 1 {
		t.Errorf("Unassessed = %d, want 1", r.Unassessed)
	}
	if got := len(r.CountsByName); got != 1 || r.CountsByName["low"] != 1 {
		t.Errorf("CountsByName = %v, want only the update's low", r.CountsByName)
	}
	if r.Counts[Unranked] != 0 {
		t.Errorf("Counts[Unranked] = %d, want 0 - it is not a severity", r.Counts[Unranked])
	}

	max, any := r.Max()
	if !any || max != Low {
		t.Errorf("Max() = %v, %v; want Low - an unrankable finding has no severity for --fail-on to compare",
			max, any)
	}
}

// The report is nothing but unrankable findings. There is no severity in it
// at all, so Max must say so rather than handing --fail-on a level it
// invented.
func TestAReportOfNothingButUnsupportedOperationsHasNoMaximum(t *testing.T) {
	r := Assess(planOf(change("azurerm_virtual_network.a", "azurerm_virtual_network", "reconcile")))
	if _, any := r.Max(); any {
		t.Error("Max() reported a severity for a report that holds none")
	}
	if r.Unassessed != 1 {
		t.Errorf("Unassessed = %d, want 1", r.Unassessed)
	}
}

// The escape hatch, and the only one: the team's own rule. It is visible in
// a file somebody committed, which is the difference between a policy and a
// surprise.
func TestATeamsOwnRuleCanGiveAnUnsupportedOperationASeverity(t *testing.T) {
	rs := loadRulesFrom(t, `{"rules":[{
		"id":"no-unknown-ops","message":"terraken could not assess this operation","level":"critical",
		"when":{"actions":["unsupported"]}}]}`)

	r := AssessWithRules(planOf(
		change("azurerm_virtual_network.a", "azurerm_virtual_network", "reconcile")), rs)

	f := r.Findings[0]
	if f.Level != Critical || f.LevelName != "critical" {
		t.Fatalf("Level = %v (%q), want critical - the team ranked it", f.Level, f.LevelName)
	}
	if f.Kind != KindUnsupported {
		t.Errorf("Kind = %q, want %q - a rule sets the severity, it does not make the operation understood",
			f.Kind, KindUnsupported)
	}
	if _, ok := annotationFor(f, AnnUnsupportedAction); !ok {
		t.Error("the unsupported-operation annotation must survive a rule assigning a severity")
	}
	if r.Unassessed != 0 || r.CountsByName["critical"] != 1 {
		t.Errorf("Unassessed = %d, CountsByName = %v; a ranked finding belongs in the severity counts",
			r.Unassessed, r.CountsByName)
	}
	if max, any := r.Max(); !any || max != Critical {
		t.Errorf("Max() = %v, %v; want Critical so --fail-on critical blocks", max, any)
	}
}

// Terraform emits one entry per OBJECT, not per address: a deposed object
// left behind by a failed create-before-destroy shares its address with the
// current one. A rule was matched against whichever of them happened to
// survive a lookup keyed on the address, so the other was ranked by a rule
// that never saw it - and an unsupported operation sitting beside a
// recognised change at the same address was the case where that mattered
// most, because the rule is the only way to make one stop a pipeline.
func TestARuleIsMatchedAgainstEveryObject_NotJustTheLastOneAtAnAddress(t *testing.T) {
	rs := loadRulesFrom(t, `{"rules":[{
		"id":"no-unknown-ops","message":"terraken could not assess this operation","level":"critical",
		"when":{"actions":["unsupported"]}}]}`)

	current := change("terraform_data.same", "terraform_data", "reconcile")
	deposed := change("terraform_data.same", "terraform_data", tfjson.ActionDelete)
	deposed.DeposedKey = "abc12345"

	r := AssessWithRules(planOf(current, deposed), rs)
	if len(r.Findings) != 2 {
		t.Fatalf("got %d findings, want 2 - one per object", len(r.Findings))
	}

	var ranked, untouched int
	for _, f := range r.Findings {
		_, hasRule := annotationFor(f, AnnRule)
		switch f.Kind {
		case KindUnsupported:
			if !hasRule || f.Level != Critical {
				t.Errorf("the unsupported object was not matched by the rule: level %v, rule %v",
					f.Level, hasRule)
			}
			ranked++
		case KindDelete:
			if hasRule {
				t.Error("the delete was matched by a rule written about unsupported operations")
			}
			untouched++
		default:
			t.Errorf("unexpected kind %q", f.Kind)
		}
	}
	if ranked != 1 || untouched != 1 {
		t.Errorf("ranked %d, untouched %d; want one of each", ranked, untouched)
	}
	if max, any := r.Max(); !any || max != Critical {
		t.Errorf("Max() = %v, %v; want Critical so --fail-on critical blocks", max, any)
	}
}

// Nothing else gets to make a claim about an operation nobody understood.
// The early return in assessOne covers the annotations it owns; the ones
// attached after it - a missed moved block, a blast radius - are keyed on
// the address, and an address is shared by a deposed object.
func TestNothingElseAnnotatesAnOperationTheToolCouldNotRead(t *testing.T) {
	r := Assess(planOf(change("terraform_data.a", "terraform_data", "reconcile")))
	f := r.Findings[0]
	if len(f.Annotations) != 1 || f.Annotations[0].Code != AnnUnsupportedAction {
		t.Fatalf("annotations = %+v, want exactly the unsupported-operation one", f.Annotations)
	}
	if f.Reason != "" || len(f.ReplacePaths) > 0 {
		t.Errorf("Reason = %q, ReplacePaths = %v; neither can be read off an operation that was not recognised",
			f.Reason, f.ReplacePaths)
	}
}

// level_at_least matches the tool's own ranking, and an unrankable finding
// has none. A rule written about critical changes must not start firing on
// an operation the tool never ranked just because "unranked" happens to
// sort above critical - the sort position is a reading order, not a claim.
func TestLevelAtLeastNeverMatchesAnUnrankedFinding(t *testing.T) {
	rs := loadRulesFrom(t, `{"rules":[{
		"id":"anything-serious","message":"a senior reviewer must approve this",
		"when":{"level_at_least":"critical"}}]}`)

	r := AssessWithRules(planOf(
		change("azurerm_virtual_network.a", "azurerm_virtual_network", "reconcile")), rs)

	f := r.Findings[0]
	if _, ok := annotationFor(f, AnnRule); ok {
		t.Error("level_at_least: critical matched an operation the tool could not rank at all")
	}
	if f.Level != Unranked {
		t.Errorf("Level = %v, want Unranked - the rule must not have given it one", f.Level)
	}
}

// The tool does not know what the operation does, so it must not claim the
// operation loses data - even on a resource type that holds some.
func TestAnUnsupportedOperationIsNotEscalatedToDataLoss(t *testing.T) {
	rc := change("azurerm_mssql_database.db", "azurerm_mssql_database",
		tfjson.ActionDelete, "quarantine")
	// Everything assessOne would read off a recognised change, present and
	// deliberately ignored. Without the early return each of these produces
	// an annotation making a claim about an operation nobody understood.
	rc.ActionReason = tfjson.ActionReasonReplaceBecauseCannotUpdate
	rc.Change.ReplacePaths = []interface{}{[]interface{}{"zone"}}
	rc.Change.AfterUnknown = map[string]interface{}{"id": true}
	rc.Change.BeforeSensitive = map[string]interface{}{"password": true}

	f := Assess(planOf(rc)).Findings[0]
	if f.DataLoss {
		t.Error("DataLoss must stay false: the operation was not recognised, so nothing is known about what it destroys")
	}
	if f.Level != Unranked {
		t.Errorf("Level = %v, want Unranked", f.Level)
	}
	if len(f.Annotations) != 1 || f.Annotations[0].Code != AnnUnsupportedAction {
		t.Errorf("annotations = %+v, want exactly the unsupported-operation one", f.Annotations)
	}
	if f.Reason != "" {
		t.Errorf("Reason = %q; a replacement reason cannot be read off an operation that was not recognised", f.Reason)
	}
	if len(f.ReplacePaths) > 0 {
		t.Errorf("ReplacePaths = %v; nothing forced a replacement the tool never identified", f.ReplacePaths)
	}
}

// A data resource is exempt from the managed-resource ranking, not from
// being recognised. Terraform emits read or no-op for one; anything else is
// as unknown here as it is anywhere.
func TestADataSourceWithAnUnrecognisedActionIsAlsoUnsupported(t *testing.T) {
	for _, a := range []tfjson.Action{tfjson.ActionRead, tfjson.ActionNoop} {
		rc := change("data.azurerm_client_config.current", "azurerm_client_config", a)
		rc.Mode = tfjson.DataResourceMode
		f := Assess(planOf(rc)).Findings[0]
		if f.Kind != KindRead || f.Level != Info {
			t.Errorf("data source with action %q: Kind = %q, Level = %v, want read and info", a, f.Kind, f.Level)
		}
	}

	rc := change("data.azurerm_client_config.current", "azurerm_client_config", "exfiltrate")
	rc.Mode = tfjson.DataResourceMode
	f := Assess(planOf(rc)).Findings[0]
	if f.Kind != KindUnsupported {
		t.Errorf("Kind = %q, want %q", f.Kind, KindUnsupported)
	}
	if f.Level != Unranked {
		t.Errorf("Level = %v, want Unranked", f.Level)
	}
}

// The whole of the existing vocabulary keeps its exact classification. This
// is the other half of the acceptance: the fallback changed, nothing else did.
func TestTheRecognisedActionsAreUnchanged(t *testing.T) {
	cases := []struct {
		actions []tfjson.Action
		kind    Kind
		level   Level
	}{
		{[]tfjson.Action{tfjson.ActionCreate}, KindCreate, Info},
		{[]tfjson.Action{tfjson.ActionUpdate}, KindUpdate, Low},
		{[]tfjson.Action{tfjson.ActionDelete}, KindDelete, High},
		{[]tfjson.Action{tfjson.ActionDelete, tfjson.ActionCreate}, KindReplace, High},
		{[]tfjson.Action{tfjson.ActionCreate, tfjson.ActionDelete}, KindReplace, High},
		{[]tfjson.Action{tfjson.ActionForget}, KindForget, Low},
		{[]tfjson.Action{tfjson.ActionRead}, KindRead, Info},
		{[]tfjson.Action{tfjson.ActionNoop}, KindNoOp, Info},
	}
	for _, c := range cases {
		t.Run(string(c.kind), func(t *testing.T) {
			f := Assess(planOf(change("azurerm_virtual_network.a", "azurerm_virtual_network", c.actions...))).Findings[0]
			if f.Kind != c.kind || f.Level != c.level {
				t.Errorf("Kind = %q, Level = %v; want %q and %v", f.Kind, f.Level, c.kind, c.level)
			}
			if _, ok := annotationFor(f, AnnUnsupportedAction); ok {
				t.Error("a recognised action must not carry an unsupported-operation annotation")
			}
		})
	}
}
