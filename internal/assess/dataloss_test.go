package assess

import (
	"testing"

	tfjson "github.com/hashicorp/terraform-json"
)

func TestIsDataLoss(t *testing.T) {
	stateful := []string{
		"azurerm_postgresql_flexible_server",
		"azurerm_storage_account",
		"azurerm_key_vault",
		"aws_db_instance",
		"aws_s3_bucket",
		"aws_dynamodb_table",
		"google_sql_database_instance",
		"google_storage_bucket",
	}
	for _, tp := range stateful {
		if !IsDataLoss(tp) {
			t.Errorf("IsDataLoss(%q) = false, want true", tp)
		}
	}

	stateless := []string{
		"azurerm_virtual_network",
		"aws_security_group",
		"google_compute_firewall",
	}
	for _, tp := range stateless {
		if IsDataLoss(tp) {
			t.Errorf("IsDataLoss(%q) = true, want false", tp)
		}
	}
}

func TestDeleteOfStatefulTypeIsCritical(t *testing.T) {
	r := Assess(planOf(change("azurerm_postgresql_flexible_server.main",
		"azurerm_postgresql_flexible_server", tfjson.ActionDelete)))
	f := r.Findings[0]
	if f.Level != Critical {
		t.Errorf("Level = %v, want critical", f.Level)
	}
	if !f.DataLoss {
		t.Error("DataLoss = false, want true")
	}
}

func TestReplaceOfStatefulTypeIsCritical(t *testing.T) {
	r := Assess(planOf(change("aws_db_instance.main", "aws_db_instance",
		tfjson.ActionDelete, tfjson.ActionCreate)))
	if r.Findings[0].Level != Critical {
		t.Errorf("Level = %v, want critical", r.Findings[0].Level)
	}
}

func TestUpdateOfStatefulTypeDoesNotEscalate(t *testing.T) {
	// Escalation applies to destruction only. Updating a database in
	// place does not lose data.
	r := Assess(planOf(change("aws_db_instance.main", "aws_db_instance",
		tfjson.ActionUpdate)))
	if r.Findings[0].Level != Low {
		t.Errorf("Level = %v, want low", r.Findings[0].Level)
	}
}

func TestUnrecognisedProviderIsAnnotated(t *testing.T) {
	rc := change("cloudflare_record.www", "cloudflare_record", tfjson.ActionDelete)
	rc.ProviderName = "registry.terraform.io/cloudflare/cloudflare"
	r := Assess(planOf(rc))
	f := r.Findings[0]

	if f.Level != High {
		t.Errorf("Level = %v, want high - it must not guess", f.Level)
	}
	var found bool
	for _, a := range f.Annotations {
		if a.Code == AnnUnknownVendor {
			found = true
		}
	}
	if !found {
		t.Error("an unrecognised provider must be annotated, not silently assumed safe")
	}
}

func TestRecognisedProviderIsNotAnnotated(t *testing.T) {
	r := Assess(planOf(change("azurerm_virtual_network.a", "azurerm_virtual_network",
		tfjson.ActionDelete)))
	for _, a := range r.Findings[0].Annotations {
		if a.Code == AnnUnknownVendor {
			t.Error("azurerm is a recognised provider and must not be annotated as unknown")
		}
	}
}
