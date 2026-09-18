package assess

import (
	"os"
	"strings"
	"testing"

	tfjson "github.com/hashicorp/terraform-json"
)

// only returns the single finding in a one-resource fixture.
func only(t *testing.T, r Report) Finding {
	t.Helper()
	if len(r.Findings) != 1 {
		t.Fatalf("expected exactly 1 finding, got %d", len(r.Findings))
	}
	return r.Findings[0]
}

// TestTheActionOrderIsTheOnlySignalForCreateBeforeDestroy is the reason this
// whole feature reads the array order rather than a lifecycle block.
//
// The two fixtures were generated from the same configuration by real
// Terraform 1.16.1, differing only in a `lifecycle { create_before_destroy =
// true }` block. Neither plan file mentions the block, the rule, or the
// argument anywhere - so the ORDER of the actions array is the whole of what
// can be known about it from a file. If Terraform ever starts writing the
// lifecycle block into plan JSON this test goes red, and reading it directly
// would then be better than inferring it.
func TestTheActionOrderIsTheOnlySignalForCreateBeforeDestroy(t *testing.T) {
	for _, name := range []string{"replace-destroy-first.json", "replace-create-first.json"} {
		b, err := os.ReadFile("../../testdata/" + name)
		if err != nil {
			t.Fatalf("committed fixture %s is missing: %v", name, err)
		}
		for _, word := range []string{"lifecycle", "create_before_destroy", "prevent_destroy", "ignore_changes"} {
			if strings.Contains(string(b), word) {
				t.Errorf("%s contains %q - the lifecycle block is in plan JSON after all, "+
					"so read it rather than inferring the ordering from the action array", name, word)
			}
		}
	}
}

// TestCreateBeforeDestroyIsReadFromTheActionOrder is the feature: the plan
// distinguishes the two replacements and so does the report.
func TestCreateBeforeDestroyIsReadFromTheActionOrder(t *testing.T) {
	cases := []struct {
		fixture string
		want    ReplaceOrder
	}{
		{"replace-destroy-first.json", ReplaceDestroyFirst},
		{"replace-create-first.json", ReplaceCreateFirst},
	}
	for _, c := range cases {
		t.Run(c.fixture, func(t *testing.T) {
			f := only(t, Assess(loadFixture(t, c.fixture)))
			if f.Kind != KindReplace {
				t.Fatalf("kind = %q, want %q", f.Kind, KindReplace)
			}
			if f.ReplaceOrder != c.want {
				t.Errorf("ReplaceOrder = %q, want %q", f.ReplaceOrder, c.want)
			}
		})
	}
}

// TestTheOrderingIsAnnotatedInTheToolsOwnVoice checks each ordering carries a
// sentence, and that the two sentences are different ones. A report that
// recorded the ordering in a field nobody renders would satisfy the machine
// consumer and tell the reviewer nothing.
func TestTheOrderingIsAnnotatedInTheToolsOwnVoice(t *testing.T) {
	details := map[ReplaceOrder]string{}
	for fixture, want := range map[string]ReplaceOrder{
		"replace-destroy-first.json": ReplaceDestroyFirst,
		"replace-create-first.json":  ReplaceCreateFirst,
	} {
		f := only(t, Assess(loadFixture(t, fixture)))
		var found *Annotation
		for i, a := range f.Annotations {
			if a.Code == AnnReplaceOrder {
				found = &f.Annotations[i]
			}
		}
		if found == nil {
			t.Fatalf("%s: no %s annotation on a replacement", fixture, AnnReplaceOrder)
		}
		if found.Detail == "" || found.Summary == "" {
			t.Errorf("%s: annotation needs both a detail and a summary, got %q and %q",
				fixture, found.Detail, found.Summary)
		}
		details[want] = found.Detail
	}
	if details[ReplaceDestroyFirst] == details[ReplaceCreateFirst] {
		t.Errorf("both orderings produced the same sentence %q - the whole point is that "+
			"they are different events", details[ReplaceDestroyFirst])
	}
}

// TestTheOrderingNeverRules checks the sentence states what happens and stops.
// The tool says what a change does; it does not say whether to approve it.
func TestTheOrderingNeverRules(t *testing.T) {
	banned := []string{"should", "must", "safe", "unsafe", "recommend", "prefer", "better", "risky", "avoid"}
	for _, fixture := range []string{"replace-destroy-first.json", "replace-create-first.json"} {
		f := only(t, Assess(loadFixture(t, fixture)))
		for _, a := range f.Annotations {
			if a.Code != AnnReplaceOrder {
				continue
			}
			text := strings.ToLower(a.Detail + " " + a.Summary)
			for _, w := range banned {
				if strings.Contains(text, w) {
					t.Errorf("%s: ordering sentence contains %q, which rules on the change "+
						"rather than stating it: %q", fixture, w, a.Detail)
				}
			}
		}
	}
}

// TestCreateBeforeDestroyDoesNotChangeTheLevel is the decision the issue asks
// for by name. create_before_destroy narrows a window; it does not stop the
// old object being destroyed, so a replacement is high either way and a
// data-holding replacement stays critical either way.
func TestCreateBeforeDestroyDoesNotChangeTheLevel(t *testing.T) {
	a := only(t, Assess(loadFixture(t, "replace-destroy-first.json")))
	b := only(t, Assess(loadFixture(t, "replace-create-first.json")))
	if a.Level != b.Level {
		t.Errorf("levels differ: destroy-first %s, create-first %s - the ordering must not "+
			"move the severity", a.Level, b.Level)
	}
	if a.Level != High {
		t.Errorf("a replacement is high, got %s", a.Level)
	}
	if a.Kind != b.Kind {
		t.Errorf("kinds differ: %q and %q - both are replacements and the kind stays whole",
			a.Kind, b.Kind)
	}
}

// TestOnlyAReplacementCarriesAnOrdering checks nothing else grows the field.
// A create has no destroy to order against, and a claim about the sequencing
// of a single operation would be a claim about nothing.
func TestOnlyAReplacementCarriesAnOrdering(t *testing.T) {
	for _, fixture := range []string{"real-plan.json", "demo.json", "critical.json", "unsupported-action.json"} {
		r := Assess(loadFixture(t, fixture))
		for _, f := range r.Findings {
			if f.Kind == KindReplace {
				if f.ReplaceOrder == "" {
					t.Errorf("%s: %s is a replacement with no ordering - the plan always "+
						"states one", fixture, f.Address)
				}
				continue
			}
			if f.ReplaceOrder != "" {
				t.Errorf("%s: %s is a %s and carries ordering %q", fixture, f.Address, f.Kind, f.ReplaceOrder)
			}
			for _, a := range f.Annotations {
				if a.Code == AnnReplaceOrder {
					t.Errorf("%s: %s is a %s and carries an ordering annotation",
						fixture, f.Address, f.Kind)
				}
			}
		}
	}
}

// TestTheOrderingNeverNamesTheLifecycleRule is the honesty limit on this
// feature, and it is held up by a fixture rather than by an argument.
//
// create_before_destroy PROPAGATES DOWN THE DEPENDENCY CHAIN. In
// replace-create-first-propagated.json, generated by real Terraform 1.16.1,
// terraform_data.downstream sets the rule and terraform_data.upstream does
// not - and both plan as ["create", "delete"]. So the ordering is a fact about
// the apply and the rule is an inference about the configuration, and on that
// fixture the inference is wrong for one of the two resources.
//
// terraken reports the order. It never reports the cause, because the plan
// does not carry the cause: the lifecycle block is not in plan JSON at all.
func TestTheOrderingNeverNamesTheLifecycleRule(t *testing.T) {
	r := Assess(loadFixture(t, "replace-create-first-propagated.json"))
	if len(r.Findings) != 2 {
		t.Fatalf("expected 2 findings, got %d", len(r.Findings))
	}
	for _, f := range r.Findings {
		if f.ReplaceOrder != ReplaceCreateFirst {
			t.Errorf("%s: ReplaceOrder = %q, want %q - Terraform plans both this way",
				f.Address, f.ReplaceOrder, ReplaceCreateFirst)
		}
		for _, a := range f.Annotations {
			if a.Code != AnnReplaceOrder {
				continue
			}
			text := strings.ToLower(a.Detail + " " + a.Summary)
			for _, claim := range []string{"create_before_destroy", "lifecycle"} {
				if strings.Contains(text, claim) {
					t.Errorf("%s: ordering sentence names %q. The plan does not carry the "+
						"lifecycle block, and this fixture has one resource planned this "+
						"way that does not set the rule: %q", f.Address, claim, a.Detail)
				}
			}
		}
	}
}

// TestDriftNeverCarriesAnOrderingClaim keeps the planning decision out of the
// past-tense list, on the same terms as action_reason and replace_paths.
//
// create_before_destroy is a lifecycle rule - a decision about how Terraform
// will carry out a change it is about to make. Nothing about drift is about to
// happen, so "the replacement is created before the old object is destroyed"
// describes an apply that is not going to occur.
func TestDriftNeverCarriesAnOrderingClaim(t *testing.T) {
	p := &tfjson.Plan{
		FormatVersion: "1.2",
		ResourceDrift: []*tfjson.ResourceChange{{
			Address:      "terraform_data.moved_underneath",
			Type:         "terraform_data",
			Mode:         tfjson.ManagedResourceMode,
			ProviderName: "terraform.io/builtin/terraform",
			Change: &tfjson.Change{
				Actions: tfjson.Actions{tfjson.ActionCreate, tfjson.ActionDelete},
			},
		}},
	}
	r := Assess(p)
	if len(r.Drift) != 1 {
		t.Fatalf("expected 1 drift entry, got %d", len(r.Drift))
	}
	if r.Drift[0].ReplaceOrder != "" {
		t.Errorf("drift carries ordering %q - a lifecycle rule is a decision about an "+
			"apply, and nothing here is about to be applied", r.Drift[0].ReplaceOrder)
	}
	for _, a := range r.Drift[0].Annotations {
		if a.Code == AnnReplaceOrder {
			t.Errorf("drift carries an ordering annotation: %q", a.Detail)
		}
	}
}
