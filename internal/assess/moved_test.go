package assess

import (
	"testing"

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

func TestIgnoresDifferentModules(t *testing.T) {
	attrs := map[string]interface{}{"location": "uksouth", "sku": "x", "v": "1", "z": "2"}
	changes := pair("module.a.azurerm_subnet.x", "module.b.azurerm_subnet.x",
		"azurerm_subnet", attrs, attrs)
	changes[0].ModuleAddress = "module.a"
	changes[1].ModuleAddress = "module.b"

	r := Assess(&tfjson.Plan{FormatVersion: "1.2", ResourceChanges: changes})
	for _, f := range r.Findings {
		if _, ok := annotationFor(f, AnnMissedMoved); ok {
			t.Fatal("resources in different modules must not be paired")
		}
	}
}
