package assess

import (
	"strings"
	"testing"
)

func TestShapeCountsByActionTypeAndModule(t *testing.T) {
	s := Assess(loadConfigured(t, "large-estate.json")).Shape

	// 20 changes: 12 replacements, 8 in-place updates.
	if s.Total != 20 {
		t.Fatalf("total: got %d, want 20", s.Total)
	}
	if got := s.ByAction[KindReplace]; got != 12 {
		t.Fatalf("replacements: got %d, want 12", got)
	}
	if got := s.ByAction[KindUpdate]; got != 8 {
		t.Fatalf("updates: got %d, want 8", got)
	}
	if got := s.ByType["local_file"]; got != 12 {
		t.Fatalf("local_file: got %d, want 12", got)
	}
	if got := s.ByModule["module.billing"]; got != 10 {
		t.Fatalf("module.billing: got %d, want 10", got)
	}
}

func TestBusiestModulesAreRankedAndNamed(t *testing.T) {
	// "Name the modules with the most churn, so a reader knows where to look
	// before reading anything." The fixture is deliberately uneven - billing
	// 10, identity 5, edge 3, root 2 - because an even spread would look the
	// same whether the ranking worked or not.
	s := Assess(loadConfigured(t, "large-estate.json")).Shape

	if len(s.BusiestModules) == 0 {
		t.Fatal("no modules ranked")
	}
	if got := s.BusiestModules[0]; got.Name != "module.billing" || got.Count != 10 {
		t.Fatalf("busiest: got %+v, want module.billing with 10", got)
	}
	for i := 1; i < len(s.BusiestModules); i++ {
		a, b := s.BusiestModules[i-1], s.BusiestModules[i]
		if a.Count < b.Count || (a.Count == b.Count && a.Name >= b.Name) {
			t.Fatalf("not ranked by count then name at %d: %+v then %+v", i, a, b)
		}
	}
	// The root module is named as something a reader recognises. Terraform
	// leaves ModuleAddress empty there, and printing that gives a blank cell
	// that reads as a bug.
	var sawRoot bool
	for _, m := range s.BusiestModules {
		if m.Name == "the root module" {
			sawRoot = true
		}
	}
	if !sawRoot {
		t.Fatalf("the root module is not named in %+v", s.BusiestModules)
	}
}

func TestTheHeadlineSaysWhatKindOfChangeThisIs(t *testing.T) {
	h := Assess(loadConfigured(t, "large-estate.json")).Shape.Headline
	if h == "" {
		t.Fatal("no headline")
	}
	// It should lead with the dominant action and say where the churn is.
	for _, want := range []string{"replace", "module.billing"} {
		if !strings.Contains(h, want) {
			t.Fatalf("headline %q does not mention %q", h, want)
		}
	}
}

func TestASmallPlanGetsNoSummary(t *testing.T) {
	// "A three-resource plan should not get a summary longer than its
	// findings." Below the threshold the shape is computed but not worth
	// showing, and Worth() is how a renderer asks.
	s := Assess(loadConfigured(t, "critical.json")).Shape
	if s.Worth() {
		t.Fatalf("a 1-finding plan should not earn a summary: %+v", s)
	}
	// The counts are still there for a JSON consumer that wants them.
	if s.Total != 1 {
		t.Fatalf("the shape should still be computed: total %d", s.Total)
	}

	big := Assess(loadConfigured(t, "large-estate.json")).Shape
	if !big.Worth() {
		t.Fatal("a 20-finding plan should earn a summary")
	}
}

func TestTheSummaryCountsTheWholePlanEvenWhenFindingsAreHidden(t *testing.T) {
	// The acceptance criterion, and the one most likely to go wrong: --min-level
	// filters what is DISPLAYED. A summary that shrank with the filter would
	// tell a reviewer the plan is smaller than it is, which is worse than no
	// summary at all.
	full := Assess(loadConfigured(t, "large-estate.json"))
	filtered := full.AtLeast(High)

	if filtered.Shape.Total != full.Shape.Total {
		t.Fatalf("filtered summary counts %d, whole plan is %d", filtered.Shape.Total, full.Shape.Total)
	}
	if filtered.Shape.ByAction[KindUpdate] != full.Shape.ByAction[KindUpdate] {
		t.Fatal("the filter changed the summary's action counts")
	}
	if len(filtered.Findings) >= len(full.Findings) {
		t.Fatal("the fixture does not actually hide anything at high - pick another")
	}
}

func TestShapeIsDeterministic(t *testing.T) {
	first := Assess(loadConfigured(t, "large-estate.json")).Shape
	for i := 0; i < 30; i++ {
		got := Assess(loadConfigured(t, "large-estate.json")).Shape
		if got.Headline != first.Headline {
			t.Fatalf("run %d headline differs:\n got %q\nwant %q", i, got.Headline, first.Headline)
		}
		if len(got.BusiestModules) != len(first.BusiestModules) {
			t.Fatalf("run %d module count differs", i)
		}
		for j := range got.BusiestModules {
			if got.BusiestModules[j] != first.BusiestModules[j] {
				t.Fatalf("run %d module %d differs: %+v vs %+v", i, j, got.BusiestModules[j], first.BusiestModules[j])
			}
		}
	}
}

func TestTheHeadlineNeverPrintsAValue(t *testing.T) {
	for _, fx := range []string{"large-estate.json", "reordered-secrets.json", "written-differently.json"} {
		s := Assess(loadConfigured(t, fx)).Shape
		blob := s.Headline
		for _, m := range s.BusiestModules {
			blob += " " + m.Name
		}
		for _, forbidden := range []string{"v1", "v2", "hunter2", "AKIA", "0644"} {
			if strings.Contains(blob, forbidden) {
				t.Fatalf("%s: a value reached the summary (%q): %q", fx, forbidden, blob)
			}
		}
	}
}
