package assess

import (
	"testing"

	tfjson "github.com/hashicorp/terraform-json"
)

func TestReplacePathsAreReported(t *testing.T) {
	rc := change("azurerm_subnet.app", "azurerm_subnet",
		tfjson.ActionDelete, tfjson.ActionCreate)
	rc.Change.ReplacePaths = []interface{}{
		[]interface{}{"address_prefixes"},
		[]interface{}{"delegation", float64(0), "name"},
	}
	r := Assess(planOf(rc))
	got := r.Findings[0].ReplacePaths

	want := []string{"address_prefixes", "delegation[0].name"}
	if len(got) != len(want) {
		t.Fatalf("ReplacePaths = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("ReplacePaths[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestActionReasonIsHumanised(t *testing.T) {
	cases := []struct {
		reason tfjson.ActionReason
		want   string
	}{
		{tfjson.ActionReasonReplaceBecauseCannotUpdate, "an attribute changed that cannot be updated in place"},
		{tfjson.ActionReasonReplaceBecauseTainted, "the resource is tainted"},
		{tfjson.ActionReasonReplaceByRequest, "replacement was explicitly requested"},
		{tfjson.ActionReasonDeleteBecauseNoResourceConfig, "its configuration block was removed"},
		{tfjson.ActionReasonDeleteBecauseNoMoveTarget, "a moved block points at a resource that does not exist"},
	}
	for _, c := range cases {
		rc := change("azurerm_subnet.app", "azurerm_subnet", tfjson.ActionDelete)
		rc.ActionReason = c.reason
		r := Assess(planOf(rc))
		if got := r.Findings[0].Reason; got != c.want {
			t.Errorf("reason %q humanised to %q, want %q", c.reason, got, c.want)
		}
	}
}

func TestNoReasonWhenTerraformGivesNone(t *testing.T) {
	r := Assess(planOf(change("azurerm_subnet.app", "azurerm_subnet", tfjson.ActionDelete)))
	if r.Findings[0].Reason != "" {
		t.Errorf("Reason = %q, want empty when Terraform supplies none", r.Findings[0].Reason)
	}
}
