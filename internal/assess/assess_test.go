package assess

import (
	"testing"

	tfjson "github.com/hashicorp/terraform-json"
)

// change builds a minimal ResourceChange for table tests.
func change(address, rtype string, actions ...tfjson.Action) *tfjson.ResourceChange {
	return &tfjson.ResourceChange{
		Address:      address,
		Type:         rtype,
		Name:         "x",
		Mode:         tfjson.ManagedResourceMode,
		ProviderName: "registry.terraform.io/hashicorp/azurerm",
		Change:       &tfjson.Change{Actions: actions},
	}
}

func planOf(changes ...*tfjson.ResourceChange) *tfjson.Plan {
	return &tfjson.Plan{FormatVersion: "1.2", ResourceChanges: changes}
}

func TestBaseRiskByAction(t *testing.T) {
	cases := []struct {
		name    string
		actions []tfjson.Action
		want    Level
		kind    Kind
	}{
		{"delete", []tfjson.Action{tfjson.ActionDelete}, High, KindDelete},
		{"replace", []tfjson.Action{tfjson.ActionDelete, tfjson.ActionCreate}, High, KindReplace},
		{"replace-create-before-destroy", []tfjson.Action{tfjson.ActionCreate, tfjson.ActionDelete}, High, KindReplace},
		{"update", []tfjson.Action{tfjson.ActionUpdate}, Low, KindUpdate},
		{"create", []tfjson.Action{tfjson.ActionCreate}, Info, KindCreate},
		{"noop", []tfjson.Action{tfjson.ActionNoop}, Info, KindNoOp},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			// azurerm_virtual_network is a known provider but not on the
			// data-loss list, so it never escalates.
			r := Assess(planOf(change("azurerm_virtual_network.a", "azurerm_virtual_network", c.actions...)))
			if len(r.Findings) != 1 {
				t.Fatalf("expected 1 finding, got %d", len(r.Findings))
			}
			f := r.Findings[0]
			if f.Level != c.want {
				t.Errorf("Level = %v, want %v", f.Level, c.want)
			}
			if f.Kind != c.kind {
				t.Errorf("Kind = %v, want %v", f.Kind, c.kind)
			}
		})
	}
}

func TestDataSourceReadIsInfo(t *testing.T) {
	rc := change("data.azurerm_client_config.current", "azurerm_client_config", tfjson.ActionRead)
	rc.Mode = tfjson.DataResourceMode
	r := Assess(planOf(rc))
	if r.Findings[0].Level != Info {
		t.Errorf("a data source read must be info, got %v", r.Findings[0].Level)
	}
}

func TestFindingsSortedBySeverityDescending(t *testing.T) {
	r := Assess(planOf(
		change("azurerm_virtual_network.a", "azurerm_virtual_network", tfjson.ActionCreate),
		change("azurerm_virtual_network.b", "azurerm_virtual_network", tfjson.ActionDelete),
		change("azurerm_virtual_network.c", "azurerm_virtual_network", tfjson.ActionUpdate),
	))
	if r.Findings[0].Level != High {
		t.Fatalf("most severe finding must come first, got %v", r.Findings[0].Level)
	}
	for i := 1; i < len(r.Findings); i++ {
		if r.Findings[i-1].Level < r.Findings[i].Level {
			t.Fatalf("findings not sorted by descending severity at index %d", i)
		}
	}
}

func TestCounts(t *testing.T) {
	r := Assess(planOf(
		change("azurerm_virtual_network.a", "azurerm_virtual_network", tfjson.ActionCreate),
		change("azurerm_virtual_network.b", "azurerm_virtual_network", tfjson.ActionDelete),
	))
	if r.Counts[Info] != 1 || r.Counts[High] != 1 {
		t.Errorf("Counts = %v, want 1 info and 1 high", r.Counts)
	}
}

func TestImportWithNoChangeIsInfo(t *testing.T) {
	rc := change("azurerm_resource_group.rg", "azurerm_resource_group", tfjson.ActionNoop)
	rc.Change.Importing = &tfjson.Importing{ID: "/subscriptions/x/resourceGroups/rg"}
	r := Assess(planOf(rc))
	f := r.Findings[0]
	if f.Kind != KindImport {
		t.Errorf("Kind = %v, want import", f.Kind)
	}
	if f.Level != Info {
		t.Errorf("an import with no change must be info, got %v", f.Level)
	}
}
