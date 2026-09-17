package main

import (
	"encoding/json"
	"fmt"
	"math/rand"
	"strings"
)

// The generator behind the no-values proof. See leakproof_test.go for the
// property it exists to serve.
//
// WHAT IT GENERATES IS COVERAGE OF POSITIONS, NOT OF VALUES. A credential is
// only interesting here for its shape, and six shapes are plenty; what a fixed
// set of fixtures can never give is coverage of the PLACES a value can hide.
// Every disclosure defect this project has seen in comparable tools was in a
// position nobody had written a fixture for - inside an array, inside a JSON
// document carried as a string, reached by a code path that did not carry the
// sensitivity metadata at all. So the axis that is enumerated exhaustively is
// the position, and every one of them is a named case.
//
// NOTHING IS MARKED SENSITIVE. That is the whole point: the guarantee cannot
// rest on Terraform's marking, because a live credential has been found in a
// real plan that Terraform had not marked. A generator that marked its plants
// would be testing the masking this tool deliberately does not do.
//
// The JSON is built by hand rather than through tfjson structs, so the loader
// is exercised too and so that a position can be planted whether or not the
// assessment currently walks it. The pinned decoder models more of the format
// than the tool reads - prior_state, resource_drift, checks and
// deferred_changes all decode today and none of them is assessed - and it is
// the gap between decoded and read that a later feature closes without anybody
// rechecking the guarantee.

// position is one place in a plan file where a value can sit.
type position struct {
	// name is what a failure reports, so "this leaked" is immediately "this
	// leaked from here".
	name string

	// plant writes the credential into the plan under construction and
	// returns nothing. Each one owns the shape of its own corner of the file.
	plant func(p *planFile, secret string)

	// live is whether the tool reads this position TODAY.
	//
	// A position the tool does not read cannot leak from it, so its cases
	// pass without proving anything - and a proof with silent vacuous cases
	// in it is worse than a smaller proof, because the count in the README
	// then overstates what was checked. Recording it makes the vacuum
	// visible, and TestEveryPlantedPositionIsReadOrSaysItIsNot fails the
	// moment a position changes state in either direction.
	//
	// Going from false to true is the good direction and is expected: the
	// roadmap has drift and prior-state arriving later, and when they do, the
	// proof is already waiting for them rather than being written afterwards
	// by somebody who has just built the feature.
	live bool
}

// positions is the register. Adding a place a value can live means adding a
// case here, and the count it reports is the number the README quotes.
//
// DELIBERATELY NOT COVERED: an attribute NAME. A map key chosen in the
// configuration is printed, because attribute paths are printed - that is the
// tool's entire output. A credential used as a map key would therefore appear,
// and that is not a hole in the guarantee but the edge of what the guarantee
// is about: it covers values, and a path is not a value. Said here rather than
// left for somebody to discover as a surprise.
var positions = []position{
	{name: "top-level-attribute", live: true, plant: func(p *planFile, s string) {
		p.change("terraform_data.top", "update").after["config_blob"] = s
	}},
	{name: "nested-object", live: true, plant: func(p *planFile, s string) {
		p.change("terraform_data.nested", "update").after["settings"] =
			map[string]interface{}{"inner": map[string]interface{}{"value": s}}
	}},
	{name: "array-element", live: true, plant: func(p *planFile, s string) {
		p.change("terraform_data.array", "update").after["items"] =
			[]interface{}{"harmless", s, "also harmless"}
	}},
	{name: "deeply-nested-array-of-objects", live: true, plant: func(p *planFile, s string) {
		p.change("terraform_data.deep", "update").after["a"] =
			map[string]interface{}{"b": []interface{}{
				map[string]interface{}{"c": map[string]interface{}{"d": "fine"}},
				map[string]interface{}{"c": map[string]interface{}{"d": s}},
			}}
	}},
	{name: "json-document-carried-as-a-string", live: true, plant: func(p *planFile, s string) {
		doc, _ := json.Marshal(map[string]interface{}{
			"Version":   "2012-10-17",
			"Statement": []interface{}{map[string]interface{}{"Credential": s}},
		})
		p.change("terraform_data.policy", "update").after["policy"] = string(doc)
	}},
	{name: "the-before-side-of-a-destroy", live: true, plant: func(p *planFile, s string) {
		// A delete has no after at all, so a destroyed secret lives only in
		// before - and destroying a secret is the case most worth saying out
		// loud, which means it is the case most likely to be printed.
		c := p.change("terraform_data.destroyed", "delete")
		c.before["config_blob"] = s
		c.after = nil
	}},
	{name: "a-value-beside-an-unknown-sibling", live: true, plant: func(p *planFile, s string) {
		// The unverifiable-until-apply annotation walks after_unknown and
		// prints the paths it finds. Its sibling is a real value sitting in
		// the same block, one step from a code path whose job is to print.
		c := p.change("terraform_data.partly_unknown", "update")
		c.after["config_blob"] = s
		c.afterUnknown = map[string]interface{}{"generated_id": true}
	}},
	{name: "a-value-that-was-only-reordered", live: true, plant: func(p *planFile, s string) {
		// The reorder and rewrite detectors are the two rules that read both
		// sides of a value and compare them. They are the code most likely to
		// end up holding one in a variable it might print.
		c := p.change("terraform_data.reordered", "update")
		c.before["items"] = []interface{}{s, "b", "c"}
		c.after["items"] = []interface{}{"c", "b", s}
	}},
	{name: "a-value-rewritten-as-json", live: true, plant: func(p *planFile, s string) {
		c := p.change("terraform_data.rewritten", "update")
		before, _ := json.Marshal(map[string]interface{}{"a": 1, "secret": s})
		c.before["doc"] = string(before)
		c.after["doc"] = "{\n  \"secret\": " + quoteJSON(s) + ",\n  \"a\": 1\n}"
	}},
	{name: "inside-a-module", live: true, plant: func(p *planFile, s string) {
		c := p.change("module.app.terraform_data.inner", "update")
		c.module = "module.app"
		c.after["config_blob"] = s
	}},
	{name: "a-replacement-forced-by-the-value", live: true, plant: func(p *planFile, s string) {
		// replace_paths names the attribute that forced a replacement, and
		// the report prints that path beside the reason. The value behind it
		// sits one dereference away.
		c := p.change("terraform_data.replaced", "replace")
		c.before["config_blob"] = "old"
		c.after["config_blob"] = s
		c.replacePaths = []interface{}{[]interface{}{"config_blob"}}
		c.actionReason = "replace_because_cannot_update"
	}},
	{name: "a-root-variable", live: true, plant: func(p *planFile, s string) {
		// Where the real incident was. The top-level variables block records
		// a value and nothing else, so a variable declared sensitive = true
		// still lands here in plaintext.
		p.variables["service_token"] = map[string]interface{}{"value": s}
	}},
	{name: "an-output-value", live: true, plant: func(p *planFile, s string) {
		p.outputs["endpoint"] = map[string]interface{}{
			"actions": []interface{}{"create"},
			"before":  nil,
			"after":   s,
		}
	}},
	{name: "prior-state", live: false, plant: func(p *planFile, s string) {
		p.priorState = append(p.priorState, map[string]interface{}{
			"address":       "terraform_data.prior",
			"mode":          "managed",
			"type":          "terraform_data",
			"name":          "prior",
			"provider_name": "registry.terraform.io/hashicorp/terraform",
			"values":        map[string]interface{}{"config_blob": s},
		})
	}},
	{name: "resource-drift", live: false, plant: func(p *planFile, s string) {
		p.drift = append(p.drift, map[string]interface{}{
			"address":       "terraform_data.drifted",
			"mode":          "managed",
			"type":          "terraform_data",
			"name":          "drifted",
			"provider_name": "registry.terraform.io/hashicorp/terraform",
			"change": map[string]interface{}{
				"actions": []interface{}{"update"},
				"before":  map[string]interface{}{"config_blob": "as planned"},
				"after":   map[string]interface{}{"config_blob": s},
			},
		})
	}},
	{name: "an-output-before-value", live: true, plant: func(p *planFile, s string) {
		// The before side of an output travels a different branch from the
		// after side, and a destroy plan has nothing else.
		p.outputs["retired"] = map[string]interface{}{
			"actions": []interface{}{"delete"},
			"before":  s,
			"after":   nil,
		}
	}},
	{name: "a-compound-root-variable", live: true, plant: func(p *planFile, s string) {
		// A variable does not have to be a string. An object or a list walks
		// a branch a bare string never reaches.
		p.variables["service"] = map[string]interface{}{"value": map[string]interface{}{
			"name":  "api",
			"creds": []interface{}{map[string]interface{}{"token": s}},
		}}
	}},
	{name: "a-value-beside-one-terraform-did-mark", live: true, plant: func(p *planFile, s string) {
		// The marked sibling is what sends the assessment down the
		// sensitivity branch. An all-unmarked corpus never exercises it, and
		// the path that handles marks is exactly the path that has a mark to
		// consult and might decide the OTHER value is fine to print.
		c := p.change("terraform_data.half_marked", "update")
		c.after["config_blob"] = s
		c.after["known_secret"] = "marked-and-redacted"
		c.afterSensitive = map[string]interface{}{"known_secret": true}
		c.beforeSensitive = map[string]interface{}{"known_secret": true}
	}},
	{name: "planned-values", live: false, plant: func(p *planFile, s string) {
		p.plannedValues = append(p.plannedValues, map[string]interface{}{
			"address":       "terraform_data.planned",
			"mode":          "managed",
			"type":          "terraform_data",
			"name":          "planned",
			"provider_name": "registry.terraform.io/hashicorp/terraform",
			"values":        map[string]interface{}{"config_blob": s},
		})
	}},
	{name: "planned-values-in-a-child-module", live: false, plant: func(p *planFile, s string) {
		p.plannedChildModules = append(p.plannedChildModules, map[string]interface{}{
			"address": "module.app",
			"resources": []interface{}{map[string]interface{}{
				"address":       "module.app.terraform_data.deep",
				"mode":          "managed",
				"type":          "terraform_data",
				"name":          "deep",
				"provider_name": "registry.terraform.io/hashicorp/terraform",
				"values":        map[string]interface{}{"config_blob": s},
			}},
		})
	}},
	{name: "a-planned-output", live: false, plant: func(p *planFile, s string) {
		p.plannedOutputs["endpoint"] = map[string]interface{}{"sensitive": false, "value": s}
	}},
	{name: "a-check-result-message", live: false, plant: func(p *planFile, s string) {
		// A check's failure message is an evaluated interpolation, so it can
		// hold a value nothing marked. Roadmap item 6 will start reading
		// these, and the proof is here first on purpose.
		p.checks = append(p.checks, map[string]interface{}{
			"address": map[string]interface{}{
				"kind": "check", "to_display": "check.api_reachable", "name": "api_reachable",
			},
			"status": "fail",
			"instances": []interface{}{map[string]interface{}{
				"address":  map[string]interface{}{"to_display": "check.api_reachable"},
				"status":   "fail",
				"problems": []interface{}{map[string]interface{}{"message": "could not reach " + s}},
			}},
		})
	}},
	{name: "a-deferred-change", live: false, plant: func(p *planFile, s string) {
		p.deferred = append(p.deferred, map[string]interface{}{
			"reason": "instance_count_unknown",
			"resource_change": map[string]interface{}{
				"address":       "terraform_data.deferred",
				"mode":          "managed",
				"type":          "terraform_data",
				"name":          "deferred",
				"provider_name": "registry.terraform.io/hashicorp/terraform",
				"change": map[string]interface{}{
					"actions": []interface{}{"create"},
					"before":  nil,
					"after":   map[string]interface{}{"config_blob": s},
				},
			},
		})
	}},
	{name: "a-constant-in-the-configuration", live: false, plant: func(p *planFile, s string) {
		// The configuration block is already walked, for the blast radius -
		// it reads references and ignores constants. A constant sitting
		// beside a reference it does read is one change away from being read
		// itself.
		p.configResources = append(p.configResources, map[string]interface{}{
			"address":             "terraform_data.configured",
			"mode":                "managed",
			"type":                "terraform_data",
			"name":                "configured",
			"provider_config_key": "terraform",
			"expressions": map[string]interface{}{
				"input":      map[string]interface{}{"constant_value": s},
				"depends_on": map[string]interface{}{"references": []interface{}{"terraform_data.top"}},
			},
		})
	}},
	{name: "a-configuration-variable-default", live: false, plant: func(p *planFile, s string) {
		p.configVariables["service_token"] = map[string]interface{}{
			"default": s, "sensitive": false,
		}
	}},
	{name: "a-resource-identity", live: false, plant: func(p *planFile, s string) {
		// Identity arrived in a recent format version and is separate from
		// the attribute values. It is exactly the sort of field a later
		// feature reads without anybody rechecking the guarantee.
		c := p.change("terraform_data.identified", "update")
		c.beforeIdentity = map[string]interface{}{"id": s}
		c.afterIdentity = map[string]interface{}{"id": s}
	}},
	{name: "prior-state-in-a-child-module", live: false, plant: func(p *planFile, s string) {
		p.priorChildModules = append(p.priorChildModules, map[string]interface{}{
			"address": "module.legacy",
			"resources": []interface{}{map[string]interface{}{
				"address":       "module.legacy.terraform_data.old",
				"mode":          "managed",
				"type":          "terraform_data",
				"name":          "old",
				"provider_name": "registry.terraform.io/hashicorp/terraform",
				"values":        map[string]interface{}{"config_blob": s},
			}},
		})
	}},
	{name: "a-rename-that-forgot-a-moved-block", live: true, plant: func(p *planFile, s string) {
		// The missed-moved-block detector compares attributes across a
		// delete/create pair and reports how many matched. It is the rule
		// that holds the most values in flight at once, and --moved turns its
		// output into HCL somebody redirects into their configuration.
		attrs := func() map[string]interface{} {
			return map[string]interface{}{
				"config_blob": s, "region": "eu-west-2",
				"tier": "standard", "label": "app", "owner": "platform",
			}
		}
		gone := p.change("terraform_data.old_name", "delete")
		gone.before = attrs()
		gone.after = nil
		fresh := p.change("terraform_data.new_name", "create")
		fresh.before = nil
		fresh.after = attrs()
	}},
}

// shape is one credential format worth planting, named so a failure says what
// kind of thing got out as well as where from.
type shape struct {
	name string

	// make builds the value that goes in the plan, wrapped in whatever fixed
	// syntax the format carries.
	make func(core string) string

	// secretPart is the part of that value whose disclosure would be the
	// leak, given the same core.
	//
	// THIS IS SEPARATE FROM make FOR A REASON, and getting it wrong in either
	// direction ruins the proof. Checking windows across the WHOLE value
	// produces false alarms off the fixed syntax: "-----BEGIN RSA PRIVATE
	// KEY-----" flattens to a run that the tool's own harmless sentence "a
	// private key" sits inside, and "sslmode=require" appears in a connection
	// string without disclosing a byte of its password. Both would fail a
	// build over an output that had leaked nothing. Checking nothing but the
	// core would be the other error, so the fixed syntax is still planted and
	// still rendered - it is only excluded from what counts as a disclosure,
	// because a published prefix everybody knows is not the secret.
	secretPart func(core string) string
}

// Every one of these is a published format a scanner would recognise, or a
// shape that is obviously a secret on sight. core is a per-run random string,
// so no two runs plant the same bytes.
var shapes = []shape{
	{
		name:       "aws-access-key-id",
		make:       func(core string) string { return "AKIA" + strings.ToUpper(core)[:16] },
		secretPart: func(core string) string { return strings.ToUpper(core)[:16] },
	},
	{
		name:       "github-token",
		make:       func(core string) string { return "ghp_" + core },
		secretPart: func(core string) string { return core },
	},
	{
		name: "pem-private-key",
		make: func(core string) string {
			return "-----BEGIN RSA PRIVATE KEY-----\n" + core + "\n" + core + "\n-----END RSA PRIVATE KEY-----"
		},
		secretPart: func(core string) string { return core },
	},
	{
		name: "connection-string-with-a-password",
		make: func(core string) string {
			return "postgres://svc_account:" + core + "@db.internal.example:5432/app?sslmode=require"
		},
		secretPart: func(core string) string { return core },
	},
	{
		name: "json-web-token",
		make: func(core string) string {
			return "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9." + core + "." + core
		},
		secretPart: func(core string) string { return core },
	},
	{
		name:       "a-long-high-entropy-string",
		make:       func(core string) string { return core + "Zq7" + core[:8] },
		secretPart: func(core string) string { return core },
	},
}

// planFile is a plan JSON document under construction.
type planFile struct {
	changes   []*changeEntry
	byAddress map[string]*changeEntry
	variables map[string]interface{}
	outputs   map[string]interface{}

	priorState        []interface{}
	priorChildModules []interface{}
	drift             []interface{}

	plannedValues       []interface{}
	plannedChildModules []interface{}
	plannedOutputs      map[string]interface{}

	checks          []interface{}
	deferred        []interface{}
	configResources []interface{}
	configVariables map[string]interface{}
}

type changeEntry struct {
	address         string
	module          string
	action          string
	before          map[string]interface{}
	after           map[string]interface{}
	afterUnknown    map[string]interface{}
	beforeSensitive map[string]interface{}
	afterSensitive  map[string]interface{}
	beforeIdentity  map[string]interface{}
	afterIdentity   map[string]interface{}
	replacePaths    []interface{}
	actionReason    string
}

func newPlanFile() *planFile {
	return &planFile{
		byAddress:       map[string]*changeEntry{},
		variables:       map[string]interface{}{},
		outputs:         map[string]interface{}{},
		plannedOutputs:  map[string]interface{}{},
		configVariables: map[string]interface{}{},
	}
}

// change returns the entry for an address, creating it on first use so a
// position can name the resource it wants without caring whether another
// position already made one.
func (p *planFile) change(address, action string) *changeEntry {
	if c, ok := p.byAddress[address]; ok {
		return c
	}
	c := &changeEntry{
		address: address,
		action:  action,
		before:  map[string]interface{}{"region": "eu-west-2"},
		after:   map[string]interface{}{"region": "eu-west-2"},
	}
	p.byAddress[address] = c
	p.changes = append(p.changes, c)
	return c
}

// actions turns this generator's shorthand into the plan format's array.
func (c *changeEntry) actions() []interface{} {
	if c.action == "replace" {
		return []interface{}{"delete", "create"}
	}
	return []interface{}{c.action}
}

// JSON renders the document, in the shape "terraform show -json" emits.
func (p *planFile) JSON() []byte {
	doc := map[string]interface{}{
		"format_version":    "1.2",
		"terraform_version": "1.9.8",
	}

	var changes []interface{}
	for _, c := range p.changes {
		change := map[string]interface{}{"actions": c.actions()}
		if c.before != nil {
			change["before"] = c.before
		} else {
			change["before"] = nil
		}
		if c.after != nil {
			change["after"] = c.after
		} else {
			change["after"] = nil
		}
		if c.afterUnknown != nil {
			change["after_unknown"] = c.afterUnknown
		}
		if c.replacePaths != nil {
			change["replace_paths"] = c.replacePaths
		}
		if c.beforeSensitive != nil {
			change["before_sensitive"] = c.beforeSensitive
		}
		if c.afterSensitive != nil {
			change["after_sensitive"] = c.afterSensitive
		}
		if c.beforeIdentity != nil {
			change["before_identity"] = c.beforeIdentity
		}
		if c.afterIdentity != nil {
			change["after_identity"] = c.afterIdentity
		}

		entry := map[string]interface{}{
			"address":       c.address,
			"mode":          "managed",
			"type":          "terraform_data",
			"name":          lastName(c.address),
			"provider_name": "registry.terraform.io/hashicorp/terraform",
			"change":        change,
		}
		if c.module != "" {
			entry["module_address"] = c.module
		}
		if c.actionReason != "" {
			entry["action_reason"] = c.actionReason
		}
		changes = append(changes, entry)
	}
	if changes != nil {
		doc["resource_changes"] = changes
	}
	if len(p.variables) > 0 {
		doc["variables"] = p.variables
	}
	if len(p.outputs) > 0 {
		doc["output_changes"] = p.outputs
	}
	if len(p.drift) > 0 {
		doc["resource_drift"] = p.drift
	}
	if len(p.priorState) > 0 || len(p.priorChildModules) > 0 {
		root := map[string]interface{}{}
		if len(p.priorState) > 0 {
			root["resources"] = p.priorState
		}
		if len(p.priorChildModules) > 0 {
			root["child_modules"] = p.priorChildModules
		}
		doc["prior_state"] = map[string]interface{}{
			"format_version":    "1.0",
			"terraform_version": "1.9.8",
			"values":            map[string]interface{}{"root_module": root},
		}
	}
	if len(p.plannedValues) > 0 || len(p.plannedChildModules) > 0 || len(p.plannedOutputs) > 0 {
		root := map[string]interface{}{}
		if len(p.plannedValues) > 0 {
			root["resources"] = p.plannedValues
		}
		if len(p.plannedChildModules) > 0 {
			root["child_modules"] = p.plannedChildModules
		}
		planned := map[string]interface{}{"root_module": root}
		if len(p.plannedOutputs) > 0 {
			planned["outputs"] = p.plannedOutputs
		}
		doc["planned_values"] = planned
	}
	if len(p.checks) > 0 {
		doc["checks"] = p.checks
	}
	if len(p.deferred) > 0 {
		doc["deferred_changes"] = p.deferred
	}
	if len(p.configResources) > 0 || len(p.configVariables) > 0 {
		root := map[string]interface{}{}
		if len(p.configResources) > 0 {
			root["resources"] = p.configResources
		}
		if len(p.configVariables) > 0 {
			root["variables"] = p.configVariables
		}
		doc["configuration"] = map[string]interface{}{"root_module": root}
	}

	b, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		panic("the generator produced something that will not marshal: " + err.Error())
	}
	return b
}

// core is a random alphanumeric run long enough to be a credential body and
// distinctive enough that a window of it cannot appear in the tool's own
// sentences by chance.
func core(rng *rand.Rand) string {
	const alphabet = "abcdefghijkmnopqrstuvwxyzABCDEFGHJKLMNPQRSTUVWXYZ23456789"
	b := make([]byte, 40)
	for i := range b {
		b[i] = alphabet[rng.Intn(len(alphabet))]
	}
	return string(b)
}

func quoteJSON(s string) string {
	b, err := json.Marshal(s)
	if err != nil {
		panic(err)
	}
	return string(b)
}

func lastName(address string) string {
	if i := strings.LastIndex(address, "."); i >= 0 {
		return address[i+1:]
	}
	return address
}

// describe names a generated case for a subtest and for a failure message.
func describe(pos position, sh shape) string {
	return fmt.Sprintf("%s/%s", pos.name, sh.name)
}
