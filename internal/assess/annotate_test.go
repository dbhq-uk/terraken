package assess

import (
	"testing"

	tfjson "github.com/hashicorp/terraform-json"
)

func annotationFor(f Finding, code string) (Annotation, bool) {
	for _, a := range f.Annotations {
		if a.Code == code {
			return a, true
		}
	}
	return Annotation{}, false
}

func TestUnknownPathsAreCollected(t *testing.T) {
	got := unknownPaths(map[string]interface{}{
		"id":                    true,
		"name":                  false,
		"source_address_prefix": true,
		"delegation": []interface{}{
			map[string]interface{}{"name": true},
		},
	})
	want := map[string]bool{
		"id":                    true,
		"source_address_prefix": true,
		"delegation[0].name":    true,
	}
	if len(got) != len(want) {
		t.Fatalf("unknownPaths = %v, want %d entries", got, len(want))
	}
	for _, p := range got {
		if !want[p] {
			t.Errorf("unexpected path %q", p)
		}
	}
}

func TestUnverifiableAnnotation(t *testing.T) {
	rc := change("azurerm_network_security_rule.web", "azurerm_network_security_rule",
		tfjson.ActionCreate)
	rc.Change.AfterUnknown = map[string]interface{}{"source_address_prefix": true}
	r := Assess(planOf(rc))

	a, ok := annotationFor(r.Findings[0], AnnUnverifiable)
	if !ok {
		t.Fatal("expected an unverifiable-until-apply annotation")
	}
	if len(a.Paths) != 1 || a.Paths[0] != "source_address_prefix" {
		t.Errorf("Paths = %v, want [source_address_prefix]", a.Paths)
	}
}

func TestNoUnverifiableAnnotationWhenEverythingKnown(t *testing.T) {
	rc := change("azurerm_subnet.app", "azurerm_subnet", tfjson.ActionCreate)
	rc.Change.AfterUnknown = map[string]interface{}{"name": false}
	r := Assess(planOf(rc))
	if _, ok := annotationFor(r.Findings[0], AnnUnverifiable); ok {
		t.Error("must not annotate when nothing is unknown")
	}
}

func TestSensitiveAnnotation(t *testing.T) {
	rc := change("azurerm_key_vault_secret.db", "azurerm_key_vault_secret",
		tfjson.ActionUpdate)
	rc.Change.AfterSensitive = map[string]interface{}{"value": true}
	r := Assess(planOf(rc))

	a, ok := annotationFor(r.Findings[0], AnnSensitive)
	if !ok {
		t.Fatal("expected a sensitive annotation")
	}
	if len(a.Paths) != 1 || a.Paths[0] != "value" {
		t.Errorf("Paths = %v, want [value]", a.Paths)
	}
	if a.Detail == "" {
		t.Error("the sensitive annotation must state that values are redacted")
	}
}
