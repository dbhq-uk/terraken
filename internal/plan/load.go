// Package plan loads Terraform and OpenTofu plan JSON, from a file or
// from a stream.
//
// It reads what it is given and nothing else. It never executes
// terraform, never reads credentials and never makes a network call.
package plan

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
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
	return parse(b, path)
}

// Read parses plan JSON from r, so a plan can be piped in rather than
// written to disk first. name appears in error messages only, so a
// stream can still say what failed under its own name instead of naming
// a file nobody opened.
func Read(r io.Reader, name string) (*tfjson.Plan, error) {
	b, err := io.ReadAll(r)
	if err != nil {
		return nil, fmt.Errorf("cannot read %s: %w", name, err)
	}
	return parse(b, name)
}

func parse(b []byte, name string) (*tfjson.Plan, error) {
	// Empty input is almost always a broken pipeline - terraform failed,
	// or nothing was piped in at all. "unexpected end of JSON input"
	// would send someone looking at the wrong thing.
	if len(bytes.TrimSpace(b)) == 0 {
		return nil, fmt.Errorf("%s is empty: expected plan JSON from: terraform show -json tfplan", name)
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
		return nil, fmt.Errorf("%s is not valid JSON: %w", name, err)
	}

	if probe.FormatVersion == "" {
		return nil, fmt.Errorf("%s does not look like a Terraform plan: no format_version. Generate one with: terraform show -json tfplan > plan.json", name)
	}

	var p tfjson.Plan
	if err := json.Unmarshal(b, &p); err != nil {
		return nil, fmt.Errorf("%s is not a valid Terraform plan: %w", name, err)
	}

	return &p, nil
}
