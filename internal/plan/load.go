// Package plan loads Terraform and OpenTofu plan JSON from disk.
//
// It reads a file and nothing else. It never executes terraform, never
// reads credentials and never makes a network call.
package plan

import (
	"encoding/json"
	"fmt"
	"os"

	tfjson "github.com/hashicorp/terraform-json"
)

// Load reads a plan JSON file produced by "terraform show -json" or
// "tofu show -json". OpenTofu emits the same format, so both are handled
// by the same path.
func Load(path string) (*tfjson.Plan, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("cannot read %s: %w", path, err)
	}

	// tfjson.Plan has its own UnmarshalJSON, which validates the plan
	// (including checking format_version) as part of decoding. That
	// means a file with valid JSON but no format_version would fail
	// inside json.Unmarshal below and get reported as invalid JSON,
	// which is misleading: the JSON is fine, it just is not a plan.
	// Probe for format_version first so the two failure modes get
	// their own, accurate messages.
	var probe struct {
		FormatVersion string `json:"format_version"`
	}
	if err := json.Unmarshal(b, &probe); err != nil {
		return nil, fmt.Errorf("%s is not valid JSON: %w", path, err)
	}

	if probe.FormatVersion == "" {
		return nil, fmt.Errorf("%s does not look like a Terraform plan: no format_version. Generate one with: terraform show -json tfplan > plan.json", path)
	}

	var p tfjson.Plan
	if err := json.Unmarshal(b, &p); err != nil {
		return nil, fmt.Errorf("%s is not a valid Terraform plan: %w", path, err)
	}

	return &p, nil
}
