package assess

import (
	"strings"
	"testing"

	"github.com/dbhq-uk/terrakit/internal/plan"
	tfjson "github.com/hashicorp/terraform-json"
)

// pair builds a delete and a create that look like a rename.
func pair(oldAddr, newAddr, rtype string, before, after map[string]interface{}) []*tfjson.ResourceChange {
	del := change(oldAddr, rtype, tfjson.ActionDelete)
	del.Change.Before = before
	crt := change(newAddr, rtype, tfjson.ActionCreate)
	crt.Change.After = after
	return []*tfjson.ResourceChange{del, crt}
}

func TestDetectsRenameWithoutMovedBlock(t *testing.T) {
	attrs := map[string]interface{}{
		"location": "uksouth", "sku": "GP_Standard_D2s_v3", "version": "15", "zone": "1",
	}
	changes := pair("azurerm_postgresql_flexible_server.main",
		"azurerm_postgresql_flexible_server.primary",
		"azurerm_postgresql_flexible_server", attrs, attrs)

	r := Assess(&tfjson.Plan{FormatVersion: "1.2", ResourceChanges: changes})

	var annotated bool
	for _, f := range r.Findings {
		if a, ok := annotationFor(f, AnnMissedMoved); ok {
			annotated = true
			if a.Detail == "" {
				t.Error("the annotation must show its reasoning")
			}
		}
	}
	if !annotated {
		t.Fatal("expected a possible-missed-moved-block annotation")
	}
}

func TestIgnoresRenameThatUsedAMovedBlock(t *testing.T) {
	// Terraform sets PreviousAddress when a moved block was used. This is
	// the correctly handled case and must never be flagged.
	rc := change("azurerm_postgresql_flexible_server.primary",
		"azurerm_postgresql_flexible_server", tfjson.ActionNoop)
	rc.PreviousAddress = "azurerm_postgresql_flexible_server.main"

	r := Assess(planOf(rc))
	for _, f := range r.Findings {
		if _, ok := annotationFor(f, AnnMissedMoved); ok {
			t.Fatal("a resource with PreviousAddress used a moved block correctly and must not be flagged")
		}
	}
}

func TestIgnoresDifferentTypes(t *testing.T) {
	changes := []*tfjson.ResourceChange{
		change("azurerm_subnet.a", "azurerm_subnet", tfjson.ActionDelete),
		change("azurerm_virtual_network.b", "azurerm_virtual_network", tfjson.ActionCreate),
	}
	r := Assess(&tfjson.Plan{FormatVersion: "1.2", ResourceChanges: changes})
	for _, f := range r.Findings {
		if _, ok := annotationFor(f, AnnMissedMoved); ok {
			t.Fatal("different resource types must not be paired")
		}
	}
}

func TestIgnoresDissimilarAttributes(t *testing.T) {
	changes := pair("azurerm_subnet.a", "azurerm_subnet.b", "azurerm_subnet",
		map[string]interface{}{"address_prefixes": "10.0.1.0/24", "name": "a", "x": "1", "y": "2"},
		map[string]interface{}{"address_prefixes": "10.9.9.0/24", "name": "b", "x": "9", "y": "8"},
	)
	r := Assess(&tfjson.Plan{FormatVersion: "1.2", ResourceChanges: changes})
	for _, f := range r.Findings {
		if _, ok := annotationFor(f, AnnMissedMoved); ok {
			t.Fatal("resources sharing no attribute values must not be paired")
		}
	}
}

// TestDetectsCrossModuleRename replaces the old TestIgnoresDifferentModules,
// which asserted that a cross-module pair was never flagged. The project
// owner ruled that behaviour wrong: moving a resource into or out of a
// module is the single most common reason anyone writes a moved block, and
// HashiCorp's own documentation leads with it. A cross-module pair must now
// be detected - it is just weaker evidence than a same-module one, and the
// annotation must say so.
func TestDetectsCrossModuleRename(t *testing.T) {
	attrs := map[string]interface{}{"input": "abc", "triggers_replace": "1", "id": "x", "z": "2"}
	changes := pair("terraform_data.bucket", "module.storage.terraform_data.bucket",
		"terraform_data", attrs, attrs)
	changes[1].ModuleAddress = "module.storage"

	r := Assess(&tfjson.Plan{FormatVersion: "1.2", ResourceChanges: changes})

	var found bool
	for _, f := range r.Findings {
		a, ok := annotationFor(f, AnnMissedMoved)
		if !ok {
			continue
		}
		found = true
		if !strings.Contains(a.Detail, "cross-module: the root module to module.storage") {
			t.Errorf("Detail must explicitly say this is a cross-module pairing, got: %s", a.Detail)
		}
		// An empty ModuleAddress means the root module. Rendering it
		// verbatim gives module "", which reads as a bug.
		if strings.Contains(a.Detail, `module ""`) {
			t.Errorf(`the root module must never be rendered as module "", got: %s`, a.Detail)
		}
		if a.Moved == nil {
			t.Fatal("a missed-moved-block annotation must carry its evidence as fields, not only as a sentence")
		}
		if !a.Moved.CrossModule {
			t.Error("Moved.CrossModule = false, want true")
		}
		if a.Moved.FromModule != "" || a.Moved.ToModule != "module.storage" {
			t.Errorf("Moved modules = %q to %q, want the root module to module.storage",
				a.Moved.FromModule, a.Moved.ToModule)
		}
	}
	if !found {
		t.Fatal("expected a cross-module rename to be detected")
	}
}

// TestMissedMovedAnnotationCarriesStructuredEvidence pins the fields a
// renderer needs in order to phrase this annotation itself. Detail is
// written for the terminal, and a renderer that cannot print a paragraph
// used to be left with nothing but the code - which is how the markdown
// output, the one a reviewer actually reads on a pull request, came to
// show the bare slug and no working at all.
func TestMissedMovedAnnotationCarriesStructuredEvidence(t *testing.T) {
	attrs := map[string]interface{}{
		"location": "uksouth", "sku": "GP_Standard_D2s_v3", "version": "15", "zone": "1",
	}
	changes := pair("azurerm_postgresql_flexible_server.main",
		"azurerm_postgresql_flexible_server.primary",
		"azurerm_postgresql_flexible_server", attrs, attrs)

	r := Assess(&tfjson.Plan{FormatVersion: "1.2", ResourceChanges: changes})

	var got *MovedEvidence
	for _, f := range r.Findings {
		if a, ok := annotationFor(f, AnnMissedMoved); ok {
			got = a.Moved
		}
	}
	if got == nil {
		t.Fatal("expected a missed-moved-block annotation carrying MovedEvidence")
	}
	if got.From != "azurerm_postgresql_flexible_server.main" {
		t.Errorf("From = %q", got.From)
	}
	if got.To != "azurerm_postgresql_flexible_server.primary" {
		t.Errorf("To = %q", got.To)
	}
	if got.Matched != 4 || got.Compared != 4 {
		t.Errorf("Matched/Compared = %d/%d, want 4/4", got.Matched, got.Compared)
	}
	if got.CrossModule {
		t.Error("CrossModule = true, want false - both resources are in the root module")
	}
}

// TestSameModuleCandidateWinsOverEquallyGoodCrossModuleOne checks the
// ranking, not just the detection: given two candidates that match the
// delete's attributes equally well - one same-module, one cross-module -
// the same-module one must be picked, because it is the stronger signal.
// The cross-module candidate's address is deliberately chosen to sort
// before the same-module one, so a bare address tie-break (with no module
// preference at all) would pick the wrong candidate by coincidence and
// this test would still pass. Only the module-preference bonus makes it
// pass for the right reason.
func TestSameModuleCandidateWinsOverEquallyGoodCrossModuleOne(t *testing.T) {
	attrs := map[string]interface{}{"location": "uksouth", "sku": "x", "v": "1", "z": "2"}

	del := change("random_string.old", "random_string", tfjson.ActionDelete)
	del.Change.Before = attrs

	sameModule := change("random_string.new", "random_string", tfjson.ActionCreate)
	sameModule.Change.After = attrs

	// "module.aaa..." sorts before "random_string..." lexically, so this
	// address would win a plain address tie-break despite being the
	// weaker, cross-module match.
	crossModule := change("module.aaa.random_string.new", "random_string", tfjson.ActionCreate)
	crossModule.Change.After = attrs
	crossModule.ModuleAddress = "module.aaa"

	changes := []*tfjson.ResourceChange{del, crossModule, sameModule}
	r := Assess(&tfjson.Plan{FormatVersion: "1.2", ResourceChanges: changes})

	var detail string
	for _, f := range r.Findings {
		if f.Address == "random_string.old" {
			if a, ok := annotationFor(f, AnnMissedMoved); ok {
				detail = a.Detail
			}
		}
	}
	if detail == "" {
		t.Fatal("expected the delete to be annotated")
	}
	if !strings.Contains(detail, "random_string.new") {
		t.Errorf("expected the same-module candidate to win the pairing, got: %s", detail)
	}
	if strings.Contains(detail, "cross-module") {
		t.Errorf("the winning pair is same-module and must not be labelled cross-module, got: %s", detail)
	}
}

// TestDetectsRealisticRenameWithOmittedComputedAttributes is the fixture
// that actually proves the feature. The pair() helper above assigns the
// SAME map to Before and After, which Terraform can never emit - a real
// delete's before always carries id and every other computed attribute; a
// real create's after never does, because omitUnknowns drops those keys
// entirely rather than nulling them. A test built on pair()'s shape could
// not have caught that, so this loads a hand-built fixture shaped like
// real "terraform show -json" output instead.
func TestDetectsRealisticRenameWithOmittedComputedAttributes(t *testing.T) {
	p, err := plan.Load("../../testdata/rename-no-moved.json")
	if err != nil {
		t.Fatalf("failed to load fixture: %v", err)
	}

	r := Assess(p)
	var annotated bool
	for _, f := range r.Findings {
		if a, ok := annotationFor(f, AnnMissedMoved); ok {
			annotated = true
			if a.Detail == "" {
				t.Error("the annotation must show its reasoning")
			}
		}
	}
	if !annotated {
		t.Fatal("expected a possible-missed-moved-block annotation for a realistic rename with omitted computed attributes")
	}
}

func TestNeverPairsAResourceWithItself(t *testing.T) {
	// A deposed object shares the live resource's address (DeposedKey
	// distinguishes it from the live change, but Address is identical).
	// If a delete of the deposed object and a create of the live
	// resource scored well, a naive matcher would produce a
	// self-referencing moved block - nonsense that refutes itself.
	attrs := map[string]interface{}{"location": "uksouth", "sku": "x", "v": "1", "z": "2"}
	del := change("azurerm_virtual_network.x", "azurerm_virtual_network", tfjson.ActionDelete)
	del.DeposedKey = "12345678"
	del.Change.Before = attrs
	crt := change("azurerm_virtual_network.x", "azurerm_virtual_network", tfjson.ActionCreate)
	crt.Change.After = attrs

	r := Assess(planOf(del, crt))
	for _, f := range r.Findings {
		if a, ok := annotationFor(f, AnnMissedMoved); ok {
			t.Fatalf("a resource must never be paired with itself, got annotation: %s", a.Detail)
		}
	}
}

func TestExcludesBothNullAttributesFromComparison(t *testing.T) {
	// Terraform state is full of null optional attributes. If matching
	// nulls on both sides counted as agreement, two genuinely different
	// resources that both happen to leave the same optional attributes
	// unset would look like a rename. They must not: excluding the eight
	// shared nulls here leaves only name and address_prefixes, both of
	// which differ, so nothing should clear the threshold - and the two
	// keys is fewer than minComparableAttrs regardless.
	before := map[string]interface{}{
		"name": "mgmt", "address_prefixes": "10.0.1.0/24",
		"opt1": nil, "opt2": nil, "opt3": nil, "opt4": nil,
		"opt5": nil, "opt6": nil, "opt7": nil, "opt8": nil,
	}
	after := map[string]interface{}{
		"name": "data", "address_prefixes": "10.0.2.0/24",
		"opt1": nil, "opt2": nil, "opt3": nil, "opt4": nil,
		"opt5": nil, "opt6": nil, "opt7": nil, "opt8": nil,
	}
	changes := pair("azurerm_subnet.mgmt", "azurerm_subnet.data", "azurerm_subnet", before, after)
	r := Assess(&tfjson.Plan{FormatVersion: "1.2", ResourceChanges: changes})
	for _, f := range r.Findings {
		if _, ok := annotationFor(f, AnnMissedMoved); ok {
			t.Fatal("two resources differing in name and address, sharing only null optionals, must not be paired")
		}
	}
}

func TestPicksBestMatchNotFirstFit(t *testing.T) {
	// a's perfect match is alpha (1.0), but beta is also a weaker match
	// for a (0.8, clearing the threshold) and is listed first in the
	// plan. A first-fit scan would grab beta for a, leaving b to wrongly
	// claim alpha by elimination - both pairs inverted. The best-scoring
	// candidate must win regardless of slice order.
	a := map[string]interface{}{"location": "uksouth", "sku": "S1", "version": "1", "zone": "1", "tier": "basic"}
	b := map[string]interface{}{"location": "uksouth", "sku": "S1", "version": "1", "zone": "1", "tier": "premium"}

	del := func(addr string, before map[string]interface{}) *tfjson.ResourceChange {
		rc := change(addr, "azurerm_subnet", tfjson.ActionDelete)
		rc.Change.Before = before
		return rc
	}
	crt := func(addr string, after map[string]interface{}) *tfjson.ResourceChange {
		rc := change(addr, "azurerm_subnet", tfjson.ActionCreate)
		rc.Change.After = after
		return rc
	}

	changes := []*tfjson.ResourceChange{
		del("azurerm_subnet.a", a),
		del("azurerm_subnet.b", b),
		// beta (a's weaker 0.8 match) is listed before alpha (a's
		// perfect 1.0 match), deliberately, so a first-fit scan would
		// find it first.
		crt("azurerm_subnet.beta", b),
		crt("azurerm_subnet.alpha", a),
	}

	r := Assess(&tfjson.Plan{FormatVersion: "1.2", ResourceChanges: changes})

	var gotA, gotB string
	for _, f := range r.Findings {
		ann, ok := annotationFor(f, AnnMissedMoved)
		if !ok {
			continue
		}
		switch f.Address {
		case "azurerm_subnet.a":
			gotA = ann.Detail
		case "azurerm_subnet.b":
			gotB = ann.Detail
		}
	}
	if gotA == "" || gotB == "" {
		t.Fatal("expected both a and b to be annotated")
	}
	if !containsAll(gotA, "azurerm_subnet.a", "azurerm_subnet.alpha") {
		t.Errorf("a should pair with its perfect match alpha, got: %s", gotA)
	}
	if !containsAll(gotB, "azurerm_subnet.b", "azurerm_subnet.beta") {
		t.Errorf("b should pair with its perfect match beta, got: %s", gotB)
	}
}

func containsAll(s string, subs ...string) bool {
	for _, sub := range subs {
		if !strings.Contains(s, sub) {
			return false
		}
	}
	return true
}
