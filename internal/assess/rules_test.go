package assess

import (
	"strings"
	"testing"
)

func loadRulesFrom(t *testing.T, src string) *RuleSet {
	t.Helper()
	rs, err := LoadRules(strings.NewReader(src))
	if err != nil {
		t.Fatalf("expected valid rules, got: %v", err)
	}
	return rs
}

func TestAMalformedRuleFileFailsLoudly(t *testing.T) {
	// "A policy that silently does not run is worse than no policy." Every
	// one of these must be an error naming what is wrong, never a rule that
	// quietly never matches.
	cases := []struct {
		name string
		src  string
		want string
	}{
		{"not json", `{oh dear`, "not valid JSON"},
		{"no rules", `{"rules": []}`, "no rules"},
		{"no id", `{"rules":[{"message":"m","when":{"actions":["delete"]}}]}`, "no id"},
		{"no message", `{"rules":[{"id":"a","when":{"actions":["delete"]}}]}`, "no message"},
		{"duplicate id", `{"rules":[
			{"id":"a","message":"m","when":{"actions":["delete"]}},
			{"id":"a","message":"n","when":{"actions":["create"]}}]}`, "more than once"},
		{"bad level", `{"rules":[{"id":"a","message":"m","level":"urgent","when":{"actions":["delete"]}}]}`, "unknown level"},
		{"bad action", `{"rules":[{"id":"a","message":"m","when":{"actions":["destroy"]}}]}`, "unknown action"},
		{"bad level_at_least", `{"rules":[{"id":"a","message":"m","when":{"level_at_least":"severe"}}]}`, "unknown level_at_least"},
		// "unranked" is the tool saying it has no severity to give, not a
		// severity a team gets to hand out. A rule assigning it would be
		// claiming the tool could not assess something it assessed fine.
		{"assigning unranked", `{"rules":[{"id":"a","message":"m","level":"unranked","when":{"actions":["delete"]}}]}`, "unknown level"},
		// The one that matters most: a typo'd FIELD name would otherwise
		// parse as an empty condition matching every resource in the plan.
		{"typo'd field", `{"rules":[{"id":"a","message":"m","when":{"actons":["delete"]}}]}`, "not valid JSON"},
		{"no conditions", `{"rules":[{"id":"a","message":"m","when":{}}]}`, "match every resource"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := LoadRules(strings.NewReader(tc.src))
			if err == nil {
				t.Fatal("expected an error, got none")
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error %q does not mention %q", err, tc.want)
			}
		})
	}
}

func TestARuleMatchesOnActionAndType(t *testing.T) {
	rs := loadRulesFrom(t, `{"rules":[{
		"id":"no-file-replace","message":"files must not be replaced","level":"critical",
		"when":{"actions":["replace"],"types":["local_file"]}}]}`)

	r := AssessWithRules(mustPlan(t, "large-estate.json"), rs)

	var hit int
	for _, f := range r.Findings {
		for _, a := range f.Annotations {
			if a.Code == AnnRule {
				hit++
				if f.Level != Critical {
					t.Fatalf("%s matched a critical rule but is %s", f.Address, f.LevelName)
				}
			}
		}
	}
	if hit != 12 {
		t.Fatalf("expected 12 local_file replacements to match, got %d", hit)
	}
}

func TestTheHighestSeverityWinsWhenSeveralRulesMatch(t *testing.T) {
	// Assigning each matched rule's level in turn let the last one seen
	// overwrite the rest, so a critical rule lost to a high one purely on
	// where its message sorted. A team with both matching one resource means
	// that resource is critical.
	rs := loadRulesFrom(t, `{"rules":[
		{"id":"zzz-low","message":"zzz something minor","level":"low",
		 "when":{"actions":["replace"],"types":["local_file"]}},
		{"id":"aaa-critical","message":"aaa something serious","level":"critical",
		 "when":{"actions":["replace"],"types":["local_file"]}}]}`)

	r := AssessWithRules(mustPlan(t, "large-estate.json"), rs)
	for _, f := range r.Findings {
		if f.Type == "local_file" && f.Kind == KindReplace {
			if f.Level != Critical {
				t.Fatalf("%s should be critical, the higher of the two matching rules, got %s", f.Address, f.LevelName)
			}
			return
		}
	}
	t.Fatal("no local_file replacement in the fixture")
}

func TestARuleCanTestAValueWithoutPrintingIt(t *testing.T) {
	// THE LINE THE WHOLE FEATURE IS BUILT AROUND. A rule may test a value
	// internally; the report names the path and never the value.
	rs := loadRulesFrom(t, `{"rules":[{
		"id":"perm-check","message":"this file has the standard permission and is being replaced",
		"when":{"actions":["replace"],"path_equals":{"file_permission":"0644"}}}]}`)

	r := AssessWithRules(mustPlan(t, "large-estate.json"), rs)

	var matched bool
	for _, f := range r.Findings {
		for _, a := range f.Annotations {
			if a.Code != AnnRule {
				continue
			}
			matched = true
			// The path is named...
			if len(a.Paths) != 1 || a.Paths[0] != "file_permission" {
				t.Fatalf("expected the tested path to be named, got %v", a.Paths)
			}
			// ...and the value is nowhere in the output.
			blob := a.Detail + " " + a.Summary + " " + strings.Join(a.Paths, " ")
			if strings.Contains(blob, "0644") {
				t.Fatalf("the tested VALUE reached the output: %q", blob)
			}
		}
	}
	if !matched {
		t.Fatal("the path_equals rule never matched, so this proves nothing")
	}
}

func TestRulesAreOrderIndependent(t *testing.T) {
	// The issue asks for it: the same plan and rules produce the same findings
	// in the same order, whichever order the rules were written in.
	a := loadRulesFrom(t, `{"rules":[
		{"id":"one","message":"alpha","when":{"actions":["replace"],"types":["local_file"]}},
		{"id":"two","message":"beta","when":{"actions":["update"],"types":["terraform_data"]}}]}`)
	b := loadRulesFrom(t, `{"rules":[
		{"id":"two","message":"beta","when":{"actions":["update"],"types":["terraform_data"]}},
		{"id":"one","message":"alpha","when":{"actions":["replace"],"types":["local_file"]}}]}`)

	ra := AssessWithRules(mustPlan(t, "large-estate.json"), a)
	rb := AssessWithRules(mustPlan(t, "large-estate.json"), b)

	if len(ra.Findings) != len(rb.Findings) {
		t.Fatalf("different finding counts: %d and %d", len(ra.Findings), len(rb.Findings))
	}
	for i := range ra.Findings {
		if ra.Findings[i].Address != rb.Findings[i].Address {
			t.Fatalf("order differs at %d: %s vs %s", i, ra.Findings[i].Address, rb.Findings[i].Address)
		}
		if len(ra.Findings[i].Annotations) != len(rb.Findings[i].Annotations) {
			t.Fatalf("%s: different annotation counts", ra.Findings[i].Address)
		}
		for j := range ra.Findings[i].Annotations {
			if ra.Findings[i].Annotations[j].Detail != rb.Findings[i].Annotations[j].Detail {
				t.Fatalf("%s: annotation %d differs", ra.Findings[i].Address, j)
			}
		}
	}
}

func TestARuleFindingIsMarkedAsYours(t *testing.T) {
	// "Clearly marked as coming from the user's rules rather than from the
	// tool's own judgement."
	rs := loadRulesFrom(t, `{"rules":[{
		"id":"mine","message":"my own rule fired","when":{"actions":["replace"]}}]}`)
	r := AssessWithRules(mustPlan(t, "large-estate.json"), rs)

	for _, f := range r.Findings {
		for _, a := range f.Annotations {
			if a.Code == AnnRule {
				if !strings.Contains(a.Detail, "your rule: mine") {
					t.Fatalf("a rule finding does not name its rule: %q", a.Detail)
				}
				return
			}
		}
	}
	t.Fatal("no rule annotation produced")
}

func TestNoRulesIsExactlyAssess(t *testing.T) {
	// A nil rule set must cost nothing and change nothing, or every existing
	// caller has quietly changed behaviour.
	p := mustPlan(t, "large-estate.json")
	plain := Assess(p)
	withNil := AssessWithRules(p, nil)

	if len(plain.Findings) != len(withNil.Findings) {
		t.Fatal("a nil rule set changed the finding count")
	}
	for i := range plain.Findings {
		if plain.Findings[i].Address != withNil.Findings[i].Address ||
			plain.Findings[i].Level != withNil.Findings[i].Level ||
			len(plain.Findings[i].Annotations) != len(withNil.Findings[i].Annotations) {
			t.Fatalf("a nil rule set changed finding %d", i)
		}
	}
}
