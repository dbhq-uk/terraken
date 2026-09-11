package plan

import (
	"strings"
	"testing"
)

func TestLoadMinimal(t *testing.T) {
	p, err := Load("../../testdata/minimal.json")
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	if p.TerraformVersion != "1.9.8" {
		t.Errorf("TerraformVersion = %q, want %q", p.TerraformVersion, "1.9.8")
	}
	if len(p.ResourceChanges) != 0 {
		t.Errorf("expected no resource changes, got %d", len(p.ResourceChanges))
	}
}

func TestLoadMalformed(t *testing.T) {
	_, err := Load("../../testdata/malformed.json")
	if err == nil {
		t.Fatal("expected an error for malformed JSON")
	}
	if !strings.Contains(err.Error(), "not valid JSON") {
		t.Errorf("error should explain the file is not valid JSON, got: %v", err)
	}
}

func TestLoadMissingFile(t *testing.T) {
	_, err := Load("../../testdata/does-not-exist.json")
	if err == nil {
		t.Fatal("expected an error for a missing file")
	}
}

func TestLoadRejectsNonPlan(t *testing.T) {
	// A valid JSON document that is not a plan has no format_version.
	_, err := Load("../../testdata/notaplan.json")
	if err == nil {
		t.Fatal("expected an error for a JSON file that is not a plan")
	}
	if !strings.Contains(err.Error(), "does not look like a Terraform plan") {
		t.Errorf("error should say it is not a plan, got: %v", err)
	}
}
