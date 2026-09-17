package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode"
	"unicode/utf8"
)

// --moved is the output that leaves this tool and goes into somebody's
// CONFIGURATION. It is redirected into a .tf file and committed, so if
// anything here must not carry a hostile address, it is this.
//
// It is also the one output where escaping would be the wrong answer: a
// display escape changes the address the block targets, and a moved block
// pointing at the wrong resource adopts a decommissioned object's state under
// a live address. So the rule is refusal, not escaping, and the refusal is
// printed rather than left as a silent omission.
func TestMovedOutputNeverCarriesAHostileAddress(t *testing.T) {
	attrs := func() map[string]interface{} {
		return map[string]interface{}{
			"input": "same", "region": "eu-west-2", "tier": "standard",
			"label": "app", "owner": "platform",
		}
	}
	for _, tc := range []struct{ name, key string }{
		{"an escape sequence", "\x1b[2Jcleared"},
		{"a newline that opens a line of HCL", "a\"]\nmoved {\n  from = real.thing\n  to   = attacker.thing\n}\n#["},
		{"a carriage return", "a\rb"},
		{"a right-to-left override", "a\u202eb"},
		{"a nul", "a\x00b"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			old := `terraform_data.gone["` + tc.key + `"]`
			plan := map[string]interface{}{
				"format_version":    "1.2",
				"terraform_version": "1.9.8",
				"resource_changes": []interface{}{
					map[string]interface{}{
						"address": old, "mode": "managed", "type": "terraform_data",
						"name": "gone", "provider_name": "registry.terraform.io/hashicorp/terraform",
						"change": map[string]interface{}{
							"actions": []interface{}{"delete"},
							"before":  attrs(), "after": nil,
						},
					},
					map[string]interface{}{
						"address": "terraform_data.fresh", "mode": "managed", "type": "terraform_data",
						"name": "fresh", "provider_name": "registry.terraform.io/hashicorp/terraform",
						"change": map[string]interface{}{
							"actions": []interface{}{"create"},
							"before":  nil, "after": attrs(),
						},
					},
				},
			}
			doc, err := json.Marshal(plan)
			if err != nil {
				t.Fatal(err)
			}
			dir := t.TempDir()
			path := filepath.Join(dir, "plan.json")
			if err := os.WriteFile(path, doc, 0o600); err != nil {
				t.Fatal(err)
			}

			var out, errOut bytes.Buffer
			if code := run([]string{"--moved", path}, strings.NewReader(""), &out, &errOut); code != 0 {
				t.Fatalf("exit = %d, stderr: %s", code, errOut.String())
			}
			hcl := out.String()

			// Nothing that acts on a terminal, breaks a line, or hides itself.
			if !utf8.ValidString(hcl) {
				t.Error("the HCL is not valid UTF-8")
			}
			for i, r := range hcl {
				if r == '\n' || r == '\t' {
					continue
				}
				if r < 0x20 || r == 0x7f || (r >= 0x80 && r <= 0x9f) || unicode.Is(unicode.Cf, r) {
					t.Errorf("a control character reached the HCL at offset %d: %q", i, hcl)
					break
				}
			}
			// And no block targeting something the plan never named.
			if strings.Contains(hcl, "attacker.thing") {
				t.Errorf("the payload wrote its own moved block:\n%s", hcl)
			}
			// The refusal is stated, not silent.
			if !strings.Contains(hcl, "NOT PROPOSED") {
				t.Errorf("a rename was dropped without saying so:\n%s", hcl)
			}
		})
	}
}

// And the ordinary case still writes a block, so the refusal above is not
// simply "this feature stopped working".
func TestMovedOutputStillProposesAnOrdinaryRename(t *testing.T) {
	var out, errOut bytes.Buffer
	if code := run([]string{"--moved", "../../testdata/rename-no-moved.json"},
		strings.NewReader(""), &out, &errOut); code != 0 {
		t.Fatalf("exit = %d, stderr: %s", code, errOut.String())
	}
	if !strings.Contains(out.String(), "moved {") {
		t.Errorf("expected a moved block for an ordinary rename:\n%s", out.String())
	}
	if strings.Contains(out.String(), "NOT PROPOSED") {
		t.Errorf("an ordinary rename must not be refused:\n%s", out.String())
	}
}
