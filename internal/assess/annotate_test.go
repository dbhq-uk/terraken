package assess

import (
	"reflect"
	"strings"
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

func TestUnknownPathsRootTrueIsWholeResource(t *testing.T) {
	got := unknownPaths(true)
	want := []string{"(whole resource)"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("unknownPaths(true) = %v, want %v", got, want)
	}
}

func TestUnknownPathsRootFalseIsEmpty(t *testing.T) {
	got := unknownPaths(false)
	if len(got) != 0 {
		t.Errorf("unknownPaths(false) = %v, want empty", got)
	}
}

func TestUnknownPathsRootListRendersWithoutLeadingDot(t *testing.T) {
	got := unknownPaths([]interface{}{true, false, true})
	want := []string{"[0]", "[2]"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("unknownPaths([true,false,true]) = %v, want %v", got, want)
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

// TestSensitiveAnnotationOnDelete is the case that used to be silent.
// Only after_sensitive was read, and a delete has no "after", so
// destroying azurerm_key_vault_secret.db_password produced no sensitive
// annotation at all while the matching create produced one. The
// highest-risk half of the pair was the unflagged half.
func TestSensitiveAnnotationOnDelete(t *testing.T) {
	rc := change("azurerm_key_vault_secret.db_password", "azurerm_key_vault_secret",
		tfjson.ActionDelete)
	rc.Change.BeforeSensitive = map[string]interface{}{"value": true}
	r := Assess(planOf(rc))

	a, ok := annotationFor(r.Findings[0], AnnSensitive)
	if !ok {
		t.Fatal("destroying a sensitive value must be annotated")
	}
	if len(a.Paths) != 1 || a.Paths[0] != "value" {
		t.Errorf("Paths = %v, want [value]", a.Paths)
	}
}

// TestSensitiveAnnotationUnionsBothSides checks a replace, where a path
// can be marked on one side, the other, or both. Both sides must be
// reported, and a path marked on both must be reported once.
func TestSensitiveAnnotationUnionsBothSides(t *testing.T) {
	rc := change("azurerm_key_vault_secret.rotating", "azurerm_key_vault_secret",
		tfjson.ActionDelete, tfjson.ActionCreate)
	rc.Change.BeforeSensitive = map[string]interface{}{"value": true, "old_only": true}
	rc.Change.AfterSensitive = map[string]interface{}{"value": true, "new_only": true}
	r := Assess(planOf(rc))

	a, ok := annotationFor(r.Findings[0], AnnSensitive)
	if !ok {
		t.Fatal("expected a sensitive annotation")
	}
	want := []string{"new_only", "old_only", "value"}
	if !reflect.DeepEqual(a.Paths, want) {
		t.Errorf("Paths = %v, want %v - the union of both sides, deduplicated and sorted", a.Paths, want)
	}
}

// TestSensitiveAnnotationNeverPrintsAValue is the standing guarantee.
// Only the path is named, never what is at it.
func TestSensitiveAnnotationNeverPrintsAValue(t *testing.T) {
	const secret = "hunter2-do-not-print-this"
	rc := change("azurerm_key_vault_secret.db", "azurerm_key_vault_secret", tfjson.ActionDelete)
	rc.Change.Before = map[string]interface{}{"value": secret}
	rc.Change.BeforeSensitive = map[string]interface{}{"value": true}
	r := Assess(planOf(rc))

	a, ok := annotationFor(r.Findings[0], AnnSensitive)
	if !ok {
		t.Fatal("expected a sensitive annotation")
	}
	for _, p := range a.Paths {
		if strings.Contains(p, secret) {
			t.Errorf("a value must never reach the output, got path %q", p)
		}
	}
	if strings.Contains(a.Detail, secret) {
		t.Errorf("a value must never reach the output, got detail %q", a.Detail)
	}
}
