package assess

import (
	"os"
	"strings"
	"testing"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclparse"
	"github.com/hashicorp/hcl/v2/hclsyntax"
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
// for by name. create_before_destroy changes the order; it does not stop the
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

// The two sentences, pinned exactly, one per ordering.
//
// TestTheOrderingIsAnnotatedInTheToolsOwnVoice only asked that the two
// sentences exist and differ, and Astra showed what that misses: swapping the
// branch inside replaceOrderAnnotation gives a create-first replacement the
// destroy-first sentence, and the whole Go suite stays green. "Different" is
// not the contract. Which one goes with which is the contract.
var wantOrderingSentence = map[ReplaceOrder]struct{ summary, detail string }{
	ReplaceDestroyFirst: {
		summary: "destroyed before the replacement is created",
		detail:  "this plan destroys the existing object before creating its replacement",
	},
	ReplaceCreateFirst: {
		summary: "the replacement is created first",
		detail:  "this plan creates the replacement before destroying the existing object",
	},
}

func TestEachOrderingCarriesItsOwnSentence(t *testing.T) {
	for fixture, order := range map[string]ReplaceOrder{
		"replace-destroy-first.json": ReplaceDestroyFirst,
		"replace-create-first.json":  ReplaceCreateFirst,
	} {
		t.Run(fixture, func(t *testing.T) {
			f := only(t, Assess(loadFixture(t, fixture)))
			a, ok := annotationFor(f, AnnReplaceOrder)
			if !ok {
				t.Fatalf("no %s annotation", AnnReplaceOrder)
			}
			want := wantOrderingSentence[order]
			if a.Summary != want.summary {
				t.Errorf("Summary = %q, want %q", a.Summary, want.summary)
			}
			if a.Detail != want.detail {
				t.Errorf("Detail = %q, want %q", a.Detail, want.detail)
			}
		})
	}
}

// TestTheOrderingNeverClaimsTheResourceKeepsExisting is a wording ban with a
// real apply behind it.
//
// The first version of the create-first sentence said "there is no point
// during the apply at which this resource does not exist". That is not a
// cautious claim, it is a false one. A local_file with a fixed filename and
// create_before_destroy plans ["create", "delete"]: Terraform writes the file
// for the new object, then the old object's destroy removes that same path,
// and the file is GONE at the end of the apply. That was run, not reasoned
// about.
//
// The order of two operations is what the plan states. Whether the thing those
// operations act on exists throughout is a question about the provider, and
// nothing in the file answers it. So the sentence states the sequence and
// stops - which also keeps it honest for #32, where the temptation to promise
// an outage window will be strongest.
func TestTheOrderingNeverClaimsTheResourceKeepsExisting(t *testing.T) {
	banned := []string{
		"does not exist", "no point", "keeps existing", "still exists",
		"available", "unavailable", "reachable", "unreachable",
		"offline", "outage", "downtime", "window", "no gap", "uninterrupted",
	}
	seen := 0
	for _, fixture := range []string{"replace-destroy-first.json", "replace-create-first.json"} {
		f := only(t, Assess(loadFixture(t, fixture)))
		a, ok := annotationFor(f, AnnReplaceOrder)
		if !ok {
			// Fatal, not a silent pass. A wording ban that holds only while
			// the annotation exists is a test that goes green the moment the
			// feature is deleted.
			t.Fatalf("%s: no %s annotation to check the wording of", fixture, AnnReplaceOrder)
		}
		seen++
		text := strings.ToLower(a.Detail + " " + a.Summary)
		for _, w := range banned {
			if strings.Contains(text, w) {
				t.Errorf("%s: the ordering sentence says %q, which is a claim about existence "+
					"or availability that the plan does not support: %q", fixture, w, a.Detail)
			}
		}
	}
	if seen != 2 {
		t.Fatalf("checked %d sentences, want 2", seen)
	}
}

// resourceBlock returns the parsed body of one resource block from a root.
//
// THE REAL PARSER, NOT A HAND-ROLLED ONE, and the first two attempts at this
// test are why. The first compared the index of "create_before_destroy"
// against the index of each resource header, so moving the rule onto the other
// resource and putting that resource last in the file kept the test green. The
// second stripped comments and counted braces, and Astra broke it four ways:
// a block header inside a heredoc counted as a block, a dependency written as
// the STRING "terraform_data.upstream.output" counted as a reference, an
// attribute called create_before_destroy in an ordinary object counted as the
// lifecycle rule, and an unterminated comment made the whole thing pass.
//
// hclparse is already a test-only dependency of this package - see
// propose_test.go, which imports it for the same reason: a hand-rolled check
// only ever confirms the author's own idea of the grammar. It is imported from
// _test.go files alone and is not linked into the shipped binary. This is not
// the tool reading HCL, which is a separate question tracked as #44; it is a
// test reading a fixture's provenance.
func resourceBlock(t *testing.T, src, path, name string) *hclsyntax.Body {
	t.Helper()
	p := hclparse.NewParser()
	file, diags := p.ParseHCL([]byte(src), path)
	if diags.HasErrors() {
		t.Fatalf("%s does not parse as HCL, so nothing can be claimed about it: %s", path, diags.Error())
	}
	body, ok := file.Body.(*hclsyntax.Body)
	if !ok {
		t.Fatalf("%s did not parse into a native syntax body", path)
	}
	for _, b := range body.Blocks {
		if b.Type == "resource" && len(b.Labels) == 2 && b.Labels[0] == "terraform_data" && b.Labels[1] == name {
			return b.Body
		}
	}
	t.Fatalf("%s declares no terraform_data.%s", path, name)
	return nil
}

// hasInstances reports whether a resource body avoids count and for_each.
//
// A resource with `count = 0` or `for_each = {}` declares NO instances, so the
// root generates no resource changes at all and the fixture beside it shows
// something the configuration does not ask for. Every assertion about the
// lifecycle block passes on such a root, which is how Astra got one through.
// These roots are deliberately single-instance.
func hasInstances(body *hclsyntax.Body) bool {
	for _, meta := range []string{"count", "for_each"} {
		if _, ok := body.Attributes[meta]; ok {
			return false
		}
	}
	return true
}

// setsCreateBeforeDestroy reports whether a resource body carries
// `lifecycle { create_before_destroy = true }` - as a BLOCK with an ATTRIBUTE
// in it, never as text that happens to contain the words.
func setsCreateBeforeDestroy(t *testing.T, body *hclsyntax.Body) bool {
	t.Helper()
	for _, b := range body.Blocks {
		if b.Type != "lifecycle" {
			continue
		}
		attr, ok := b.Body.Attributes["create_before_destroy"]
		if !ok {
			continue
		}
		v, diags := attr.Expr.Value(nil)
		if diags.HasErrors() {
			t.Fatalf("create_before_destroy is not a constant: %s", diags.Error())
		}
		if v.True() {
			return true
		}
	}
	return false
}

// referencesResource reports whether any attribute of a resource body refers to
// another resource, through an EXPRESSION rather than through a string that
// spells its address.
func referencesResource(body *hclsyntax.Body, typ, name string) bool {
	for _, attr := range body.Attributes {
		for _, v := range attr.Expr.Variables() {
			if len(v) < 2 || v.RootName() != typ {
				continue
			}
			if step, ok := v[1].(hcl.TraverseAttr); ok && step.Name == name {
				return true
			}
		}
	}
	return false
}

// TestTheGeneratingRootsSayWhatTheFixturesClaim ties the prose to the
// configuration it is about.
//
// TestTheOrderingNeverNamesTheLifecycleRule asserts that one resource in the
// propagated fixture sets create_before_destroy and the other does not, and the
// plan file cannot show that - the lifecycle block is not in plan JSON, which
// is the whole point. The roots are committed under testdata/_gen so the claim
// rests on something in the repository, and this reads them.
func TestTheGeneratingRootsSayWhatTheFixturesClaim(t *testing.T) {
	// THE WHOLE ROOT, NOT JUST main.tf. Terraform merges override files -
	// an override.tf.json beside this one saying
	// {"resource":{"terraform_data":{"service":{"lifecycle":
	// {"create_before_destroy":false}}}}} reverses the ordering, and a test
	// that read main.tf alone would never see it. These roots are one file
	// each, so the check is that they still are.
	root := func(name string) (string, string) {
		t.Helper()
		dir := "../../testdata/_gen/" + name
		entries, err := os.ReadDir(dir)
		if err != nil {
			t.Fatalf("the root that generated %s.json is missing: %v", name, err)
		}
		var names []string
		for _, e := range entries {
			names = append(names, e.Name())
		}
		if len(names) != 1 || names[0] != "main.tf" {
			t.Fatalf("%s holds %v, want exactly [main.tf]. Terraform merges every .tf and "+
				"every override file in a directory, so a second file can say something this "+
				"test never reads", dir, names)
		}
		path := dir + "/main.tf"
		b, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("%s is missing: %v", path, err)
		}
		return string(b), path
	}

	// The pair that isolates the lifecycle block: identical roots, one rule.
	src, path := root("replace-destroy-first")
	plainService := resourceBlock(t, src, path, "service")
	if !hasInstances(plainService) {
		t.Error("replace-destroy-first's resource is behind a count or for_each, so the root " +
			"may declare no instances at all and generate no replacement")
	}
	if setsCreateBeforeDestroy(t, plainService) {
		t.Error("replace-destroy-first sets create_before_destroy, so it is not the default case")
	}
	src, path = root("replace-create-first")
	cbdService := resourceBlock(t, src, path, "service")
	if !hasInstances(cbdService) {
		t.Error("replace-create-first's resource is behind a count or for_each, so the root " +
			"may declare no instances at all and generate no replacement")
	}
	if !setsCreateBeforeDestroy(t, cbdService) {
		t.Error("replace-create-first does not set create_before_destroy on the resource it " +
			"replaces, so the fixture pair does not isolate the lifecycle block")
	}

	// The propagated case, which carries a claim the plan file cannot support
	// on its own: the rule is set ONCE, on the resource that DEPENDS on the
	// other, and both are planned create-first anyway.
	src, path = root("replace-create-first-propagated")
	up := resourceBlock(t, src, path, "upstream")
	down := resourceBlock(t, src, path, "downstream")

	for label, body := range map[string]*hclsyntax.Body{"upstream": up, "downstream": down} {
		if !hasInstances(body) {
			t.Errorf("%s is behind a count or for_each, so the root may declare no instances "+
				"and the propagated fixture shows something it does not ask for", label)
		}
	}
	if setsCreateBeforeDestroy(t, up) {
		t.Error("upstream sets create_before_destroy. The fixture's whole claim is that the " +
			"resource WITHOUT the rule is planned create-first, so with the rule on it the " +
			"plan proves nothing")
	}
	if !setsCreateBeforeDestroy(t, down) {
		t.Error("downstream does not set create_before_destroy, so nothing in this root asks " +
			"for the ordering the fixture shows on both resources")
	}
	if !referencesResource(down, "terraform_data", "upstream") {
		t.Error("downstream holds no expression referring to upstream, so there is no " +
			"dependency chain for the rule to propagate along")
	}
	if referencesResource(up, "terraform_data", "downstream") {
		t.Error("upstream refers to downstream, so the chain runs both ways and the fixture " +
			"no longer shows which direction the rule propagated in")
	}
}
