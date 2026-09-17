package assess

import (
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"

	tfjson "github.com/hashicorp/terraform-json"
)

// A team's own rules, evaluated over a plan the tool has already parsed.
//
// Teams have rules about what a plan may do - never destroy anything tagged
// production, never widen a security group to the internet. Enforcing those
// means a policy engine with its own language and runtime, or a reviewer
// remembering. This is neither: it is a small set of conditions over the
// classification already being computed, with no new runtime and nothing to
// sign up for.
//
// NOT A POLICY LANGUAGE, and it must not grow into one. There is no
// expression syntax, no arithmetic, no user-supplied code path - every
// condition is a declared field compared against something the plan already
// says. The moment this needs a parser it has become a worse version of a tool
// that already exists, and the answer is to use that one.
//
// JSON RATHER THAN HCL, which for a Terraform tool needs a reason. HCL would
// read more naturally to this audience, and it is the obvious choice until you
// notice the cost: hashicorp/hcl is currently a TEST-ONLY dependency here, and
// making it a runtime one puts a parser in the shipped binary and hands the
// module's Go floor to somebody else's release schedule. go install is this
// tool's primary install route and that floor has already broken it once. JSON
// is in the standard library, and a rule file is read by CI far more often than
// it is written by a person.

// Rule is one rule from a rule file.
type Rule struct {
	// ID is how a finding names the rule that produced it. Required: a
	// finding whose origin is "rule 3 in the file you gave me" is one the
	// reader cannot act on.
	ID string `json:"id"`

	// Message is the finding's text, in the team's words rather than the
	// tool's. Required, for the same reason.
	Message string `json:"message"`

	// Level is the severity the team assigns. Defaults to high - a rule
	// somebody bothered to write is not information.
	Level string `json:"level,omitempty"`

	When Condition `json:"when"`
}

// Condition is what a rule matches on. Every field set must hold; an unset
// field is not tested.
//
// AND, never OR, and that is deliberate rather than a missing feature. Two
// rules are how you say "or", and they read better than a nested boolean would:
// each has its own id and message, so a report says which of the two fired
// instead of naming one rule that could have matched for either of two reasons.
type Condition struct {
	// Actions matches the kind the tool assigned: create, update, delete,
	// replace, read, import, forget, no-op.
	Actions []string `json:"actions,omitempty"`

	// Types matches the resource type exactly, or with a trailing * as a
	// prefix - "aws_s3_*" is the common case and the only wildcard here.
	Types []string `json:"types,omitempty"`

	// Modules matches the module address exactly, or by prefix with a
	// trailing *. The root module is "" and may be written as such.
	Modules []string `json:"modules,omitempty"`

	// LevelAtLeast matches the tool's own classification, so a team can
	// write a rule over the risk ranking rather than restating it.
	LevelAtLeast string `json:"level_at_least,omitempty"`

	// DataLoss matches the tool's data-loss escalation.
	DataLoss *bool `json:"data_loss,omitempty"`

	// PathPresent requires every named attribute path to exist in the
	// change. PathAbsent requires every one to be missing.
	PathPresent []string `json:"path_present,omitempty"`
	PathAbsent  []string `json:"path_absent,omitempty"`

	// PathEquals tests a path's value WITHOUT EVER PRINTING IT. This is the
	// line the issue draws and it is the whole reason this type can exist at
	// all: a rule may test a value internally, and the report names only the
	// path. Rendering the value into the message would put an attribute value
	// in the output, which is the one thing this tool does not do.
	PathEquals map[string]string `json:"path_equals,omitempty"`
}

// RuleSet is a parsed rule file.
type RuleSet struct {
	Rules []Rule `json:"rules"`
}

// LoadRules reads and validates a rule file.
//
// IT FAILS LOUDLY, which the issue asks for in as many words: "a policy that
// silently does not run is worse than no policy". Every error here names the
// rule and what is wrong with it, and nothing is skipped over. A typo'd action
// is a hard error rather than a condition that quietly never matches - the
// latter is how a team believes they are covered for a year.
func LoadRules(r io.Reader) (*RuleSet, error) {
	var rs RuleSet
	dec := json.NewDecoder(r)
	// Unknown fields are an error, not something to ignore. "actons" instead
	// of "actions" would otherwise parse as an empty condition that matches
	// every resource in the plan.
	dec.DisallowUnknownFields()
	if err := dec.Decode(&rs); err != nil {
		return nil, fmt.Errorf("rule file is not valid JSON: %w", err)
	}
	if len(rs.Rules) == 0 {
		return nil, fmt.Errorf("rule file contains no rules")
	}

	seen := map[string]bool{}
	for i, rule := range rs.Rules {
		where := fmt.Sprintf("rule %d", i+1)
		if rule.ID != "" {
			where = fmt.Sprintf("rule %q", rule.ID)
		}
		if rule.ID == "" {
			return nil, fmt.Errorf("%s has no id, so a finding could not say which rule produced it", where)
		}
		if seen[rule.ID] {
			return nil, fmt.Errorf("rule id %q is used more than once", rule.ID)
		}
		seen[rule.ID] = true
		if strings.TrimSpace(rule.Message) == "" {
			return nil, fmt.Errorf("%s has no message", where)
		}
		if rule.Level != "" {
			if _, perr := ParseLevel(strings.ToLower(rule.Level)); perr != nil {
				return nil, fmt.Errorf("%s has unknown level %q: expected critical, high, low or info", where, rule.Level)
			}
		}
		if rule.When.LevelAtLeast != "" {
			if _, perr := ParseLevel(strings.ToLower(rule.When.LevelAtLeast)); perr != nil {
				return nil, fmt.Errorf("%s has unknown level_at_least %q: expected critical, high, low or info", where, rule.When.LevelAtLeast)
			}
		}
		for _, a := range rule.When.Actions {
			if !knownKind(a) {
				return nil, fmt.Errorf("%s has unknown action %q: expected create, update, delete, replace, read, import, forget or no-op", where, a)
			}
		}
		if rule.When.isEmpty() {
			return nil, fmt.Errorf("%s has no conditions, so it would match every resource in the plan", where)
		}
	}
	return &rs, nil
}

func (c Condition) isEmpty() bool {
	return len(c.Actions) == 0 && len(c.Types) == 0 && len(c.Modules) == 0 &&
		c.LevelAtLeast == "" && c.DataLoss == nil &&
		len(c.PathPresent) == 0 && len(c.PathAbsent) == 0 && len(c.PathEquals) == 0
}

func knownKind(s string) bool {
	switch Kind(s) {
	case KindCreate, KindUpdate, KindDelete, KindReplace, KindRead, KindImport, KindForget, KindNoOp:
		return true
	}
	return false
}

// applyRules evaluates every rule against every change and returns the
// annotations to attach, keyed by resource address.
//
// Deterministic and order-independent, which the issue asks for: rules are
// evaluated in file order for each resource, and the resulting annotations are
// sorted by rule id, so two rule files with the same rules in a different
// order produce identical reports.
// ruleHit is one rule matching one resource: the annotation to attach and the
// severity the team assigned, which may be higher OR lower than the tool's own
// classification.
type ruleHit struct {
	ann   Annotation
	level Level
}

func applyRules(rs *RuleSet, changes []*tfjson.ResourceChange, findings []Finding) map[string][]ruleHit {
	if rs == nil {
		return nil
	}
	byAddr := map[string]Finding{}
	for _, f := range findings {
		byAddr[f.Address] = f
	}

	out := map[string][]ruleHit{}
	for _, rc := range changes {
		if rc == nil || rc.Change == nil {
			continue
		}
		f, ok := byAddr[rc.Address]
		if !ok {
			continue
		}
		for _, rule := range rs.Rules {
			if !matches(rule.When, rc, f) {
				continue
			}
			out[rc.Address] = append(out[rc.Address], ruleHit{
				ann: Annotation{
					Code: AnnRule,
					// The team's words, marked so nobody mistakes a local
					// rule for the tool's own judgement. The issue asks for
					// exactly this separation.
					Detail:  fmt.Sprintf("%s (your rule: %s)", rule.Message, rule.ID),
					Summary: rule.Message,
					Paths:   matchedPaths(rule.When),
				},
				level: ruleLevel(rule),
			})
		}
	}
	// Sorted by the rendered detail, which begins with the team's message and
	// ends with the rule id - so two rule files holding the same rules in a
	// different order produce byte-identical reports. The issue asks for
	// order-independence and this is where it is bought.
	for addr := range out {
		hits := out[addr]
		sort.SliceStable(hits, func(i, j int) bool { return hits[i].ann.Detail < hits[j].ann.Detail })
		out[addr] = hits
	}
	return out
}

// matchedPaths names the attribute paths a rule tested, so the finding says
// which part of the resource the rule was about.
//
// PATHS ONLY. PathEquals contributes its KEYS and never its values - that is
// the line the whole feature is built around, and it is why this function
// exists rather than the message being formatted with the value inlined.
func matchedPaths(c Condition) []string {
	var out []string
	out = append(out, c.PathPresent...)
	out = append(out, c.PathAbsent...)
	for k := range c.PathEquals {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func matches(c Condition, rc *tfjson.ResourceChange, f Finding) bool {
	if len(c.Actions) > 0 && !containsStr(c.Actions, string(f.Kind)) {
		return false
	}
	if len(c.Types) > 0 && !matchesAny(c.Types, rc.Type) {
		return false
	}
	if len(c.Modules) > 0 && !matchesAny(c.Modules, rc.ModuleAddress) {
		return false
	}
	if c.LevelAtLeast != "" {
		min, _ := ParseLevel(strings.ToLower(c.LevelAtLeast))
		if f.Level < min {
			return false
		}
	}
	if c.DataLoss != nil && *c.DataLoss != f.DataLoss {
		return false
	}

	// Paths are looked up in `after` where there is one, falling back to
	// `before`. A delete has no after, and a rule about what is being
	// destroyed is exactly the rule a team most wants.
	attrs := rc.Change.After
	if attrs == nil {
		attrs = rc.Change.Before
	}
	for _, p := range c.PathPresent {
		if _, ok := lookupPath(attrs, p); !ok {
			return false
		}
	}
	for _, p := range c.PathAbsent {
		if _, ok := lookupPath(attrs, p); ok {
			return false
		}
	}
	for p, want := range c.PathEquals {
		got, ok := lookupPath(attrs, p)
		if !ok || renderScalar(got) != want {
			return false
		}
	}
	return true
}

// lookupPath walks a dotted path into a decoded JSON object.
//
// Dots only, no indexing. A rule that needs to reach into the third element of
// a list is a rule that wants an expression language, and that is the line
// this feature does not cross.
func lookupPath(v interface{}, path string) (interface{}, bool) {
	cur := v
	for _, part := range strings.Split(path, ".") {
		m, ok := cur.(map[string]interface{})
		if !ok {
			return nil, false
		}
		cur, ok = m[part]
		if !ok {
			return nil, false
		}
	}
	return cur, true
}

// renderScalar makes a value comparable to the string in a rule.
//
// JSON-encoded rather than fmt's %v, for the same reason renderAttrs is: %v is
// type-blind. It renders nil and the string "<nil>" identically, the number 15
// and the string "15" identically. A rule matching on the wrong type is a rule
// that silently does not fire.
func renderScalar(v interface{}) string {
	if s, ok := v.(string); ok {
		return s
	}
	b, err := json.Marshal(v)
	if err != nil {
		return ""
	}
	return string(b)
}

func containsStr(hay []string, needle string) bool {
	for _, h := range hay {
		if h == needle {
			return true
		}
	}
	return false
}

// matchesAny supports one wildcard form: a trailing *, meaning prefix.
// "aws_s3_*" is the case teams actually write; anything richer is a pattern
// language, which this is not.
func matchesAny(patterns []string, s string) bool {
	for _, p := range patterns {
		if strings.HasSuffix(p, "*") {
			if strings.HasPrefix(s, strings.TrimSuffix(p, "*")) {
				return true
			}
			continue
		}
		if p == s {
			return true
		}
	}
	return false
}

// ruleLevel is the severity a matched rule gives a finding, defaulting to
// high. A rule somebody bothered to write is not information.
func ruleLevel(r Rule) Level {
	if r.Level == "" {
		return High
	}
	lv, _ := ParseLevel(strings.ToLower(r.Level))
	return lv
}
