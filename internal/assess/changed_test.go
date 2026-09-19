package assess

import (
	"os"
	"regexp"
	"strings"
	"testing"
)

// What each resource actually changes, at every level.
//
// The report ranked a change and named the attributes that FORCED it, but
// never said what the change touches. A reviewer reading "update in place" with
// one reordering annotation under it has no idea whether the plan is editing a
// tag or a security rule.
//
// ATTRIBUTE NAMES ARE NOT VALUES, which is why this is allowed at all.
// AGENTS.md is explicit: paths, counts, levels and the tool's own sentences are
// what it may show. Every name here comes from the plan's attribute keys and no
// value is read to produce them.
//
// TOP-LEVEL ONLY. A change to tags.owner is reported as `tags`. The roll-up in
// rewritten.go already counts top-level attributes for the same reason - it is
// what a reader sees the plan show as changed - and the nested paths that
// matter are already named by the annotations that care about them: what forced
// a replacement, what is unknown until apply, what is sensitive.

func changedFor(t *testing.T, r Report, addr string) *Annotation {
	t.Helper()
	f := findingFor(t, r, addr)
	for i := range f.Annotations {
		if f.Annotations[i].Code == AnnChangedAttributes {
			return &f.Annotations[i]
		}
	}
	return nil
}

// TestAnUpdateNamesWhatItTouches is the gap this closes.
func TestAnUpdateNamesWhatItTouches(t *testing.T) {
	a := changedFor(t, Assess(loadFixture(t, "demo.json")), "azurerm_network_security_group.web")
	if a == nil {
		t.Fatal("an update in place does not say which attributes it changes")
	}
	// BOTH, and the second one is the point. This resource's report already
	// named security_rules, through the reordering annotation. It changes tags
	// as well, and nothing in the report said so - a reviewer reading "update
	// in place, same elements different order" would have concluded the plan
	// touched one attribute and been wrong.
	if !equalStrings(a.Paths, []string{"security_rules", "tags"}) {
		t.Errorf("Paths = %v, want both attributes this update touches", a.Paths)
	}
	if !strings.Contains(a.Summary, "changes 2 attributes") {
		t.Errorf("Summary = %q, want it to count what it names", a.Summary)
	}
}

// TestASingleChangedAttributeReadsAsOne keeps the grammar honest on the count
// that is easiest to get wrong.
func TestASingleChangedAttributeReadsAsOne(t *testing.T) {
	seen := false
	for _, fixture := range []string{"demo.json", "real-plan.json", "critical.json", "large-estate.json"} {
		for _, f := range Assess(loadFixture(t, fixture)).Findings {
			for _, a := range f.Annotations {
				if a.Code != AnnChangedAttributes || len(a.Paths) != 1 {
					continue
				}
				seen = true
				if strings.Contains(a.Summary, "1 attributes") {
					t.Errorf("%s/%s: Summary reads %q", fixture, f.Address, a.Summary)
				}
			}
		}
	}
	if !seen {
		t.Skip("no fixture has a change touching exactly one attribute")
	}
}

// TestACreateNamesWhatItSets. A create has no before, so every attribute is
// new - which is the whole of what the plan says it will make.
func TestACreateNamesWhatItSets(t *testing.T) {
	a := changedFor(t, Assess(loadFixture(t, "demo.json")), "azurerm_subnet.application")
	if a == nil {
		t.Fatal("a create does not say which attributes it sets")
	}
	// THE EXACT SET, including the ones Terraform does not put in `after`.
	// An attribute whose value is unknown until apply is dropped from `after`
	// entirely and marked in `after_unknown`, so reading `after` alone reports
	// a create as setting fewer attributes than it sets - and the ones left
	// out are exactly the ones nobody can check before apply. Asserting only
	// that the list is non-empty missed that: dropping the unknowns still left
	// plenty of names behind.
	want := []string{
		"address_prefixes", "etag", "id", "name", "private_endpoint_network_policies",
		"resource_group_name",
		"service_endpoints", "virtual_network_name",
	}
	if !equalStrings(a.Paths, want) {
		t.Errorf("Paths = %v, want %v - id and etag are unknown until apply and are in "+
			"after_unknown rather than after", a.Paths, want)
	}
	if !strings.HasPrefix(a.Summary, "sets ") {
		t.Errorf("Summary = %q, want it to say the attributes are being SET rather than "+
			"changed - there is nothing here to change from", a.Summary)
	}
}

// TestADeleteNamesWhatItHad. Nothing changes on a delete: the resource goes.
// Naming what it held is the useful fact, and the verb has to say so.
func TestADeleteNamesWhatItHad(t *testing.T) {
	a := changedFor(t, Assess(loadFixture(t, "demo.json")), "azurerm_subnet.app")
	if a == nil {
		t.Fatal("a delete does not say which attributes it had")
	}
	if !strings.HasPrefix(a.Summary, "had ") {
		t.Errorf("Summary = %q, want the past tense - a delete changes nothing, it goes",
			a.Summary)
	}
}

// TestTheCountAlwaysMatchesTheNames. A sentence and its own evidence
// disagreeing is worse than either being wrong alone, because each looks like
// it corroborates the other. This session found that class four times.
func TestTheCountAlwaysMatchesTheNames(t *testing.T) {
	checked := 0
	for _, fixture := range []string{"demo.json", "real-plan.json", "critical.json", "large-estate.json"} {
		for _, f := range Assess(loadFixture(t, fixture)).Findings {
			for _, a := range f.Annotations {
				if a.Code != AnnChangedAttributes {
					continue
				}
				checked++
				if !strings.Contains(a.Summary, itoa(len(a.Paths))+" attribute") {
					t.Errorf("%s/%s: Summary %q does not count the %d attributes it names: %v",
						fixture, f.Address, a.Summary, len(a.Paths), a.Paths)
				}
			}
		}
	}
	if checked == 0 {
		t.Fatal("no changed-attribute annotation was checked, so this test proves nothing")
	}
}

// TestNothingIsClaimedForAnOperationNobodyRead. classify ends at
// KindUnsupported when it cannot read the action, and assessOne returns before
// anything else is claimed. A list of attributes under an operation nobody
// understood would be exactly the confident guess that kind exists to refuse.
func TestNothingIsClaimedForAnOperationNobodyRead(t *testing.T) {
	for _, f := range Assess(loadFixture(t, "unsupported-action.json")).Findings {
		if f.Kind != KindUnsupported {
			continue
		}
		for _, a := range f.Annotations {
			if a.Code == AnnChangedAttributes {
				t.Errorf("%s: an unreadable operation claims %q", f.Address, a.Summary)
			}
		}
	}
}

// TestANoOpNamesNothing. A resource this plan does not change has no attributes
// to report, and "changes 0 attributes" under every no-op is how a reader is
// taught to skip the annotations.
func TestANoOpNamesNothing(t *testing.T) {
	for _, f := range Assess(loadFixture(t, "sequence-chain.json")).Findings {
		if f.Kind != KindNoOp {
			continue
		}
		for _, a := range f.Annotations {
			if a.Code == AnnChangedAttributes {
				t.Errorf("%s is a no-op and claims %q", f.Address, a.Summary)
			}
		}
	}
}

// TestEveryRankedChangeSaysWhatItTouches is the acceptance: at every level, a
// finding that does something to a resource says what.
func TestEveryRankedChangeSaysWhatItTouches(t *testing.T) {
	for _, fixture := range []string{"demo.json", "real-plan.json"} {
		r := Assess(loadFixture(t, fixture))
		for _, f := range r.Findings {
			switch f.Kind {
			case KindCreate, KindUpdate, KindDelete, KindReplace:
			default:
				continue
			}
			if changedFor(t, r, f.Address) == nil {
				t.Errorf("%s/%s is a %s at %s and does not say which attributes it touches",
					fixture, f.Address, f.Kind, f.Level)
			}
		}
	}
}

// TestNoValueReachesTheAttributeList is constraint 1 on this annotation. The
// names come from the plan's keys; nothing here may carry what was in them.
func TestNoValueReachesTheAttributeList(t *testing.T) {
	r := Assess(loadFixture(t, "unmarked-credentials.json"))
	secrets := []string{
		"AKIA", "-----BEGIN", "ghp_", "xoxb-", "cloudflare", "eyJ",
	}
	checked := 0
	for _, f := range r.Findings {
		for _, a := range f.Annotations {
			if a.Code != AnnChangedAttributes {
				continue
			}
			checked++
			hay := strings.ToLower(strings.Join(append(a.Paths, a.Summary, a.Detail), " "))
			for _, s := range secrets {
				if strings.Contains(hay, strings.ToLower(s)) {
					t.Errorf("%s: the attribute list carries %q, which came out of a value",
						f.Address, s)
				}
			}
		}
	}
	if checked == 0 {
		t.Fatal("no changed-attribute annotation was checked against the credentials fixture")
	}
}

// TestEveryReportedNameLooksLikeASchemaAttribute is the structural argument for
// why this annotation cannot leak, rather than the case-by-case one.
//
// A resource's before and after are a flat object of SCHEMA attributes, and a
// provider schema names its attributes in lower snake case. Taking top-level
// keys only means every name reported here came from a schema rather than from
// data - a map with attacker-chosen keys is the VALUE of an attribute called
// `tags`, and its keys are one level down where this never looks.
//
// So a name that could not be a schema attribute is evidence that something
// from a value has reached the list, whatever the route. That is a stronger
// guard than checking for particular secrets, because it does not depend on
// recognising what leaked.
func TestEveryReportedNameLooksLikeASchemaAttribute(t *testing.T) {
	schemaName := regexp.MustCompile(`^[a-z][a-z0-9_]*$`)

	checked := 0
	for _, fixture := range fixtureFiles(t) {
		for _, f := range Assess(loadFixture(t, fixture)).Findings {
			for _, a := range f.Annotations {
				if a.Code != AnnChangedAttributes {
					continue
				}
				for _, name := range a.Paths {
					checked++
					if !schemaName.MatchString(name) {
						t.Errorf("%s/%s: %q is not a shape a provider schema gives an "+
							"attribute, so it did not come from one - something from a "+
							"value has reached the list", fixture, f.Address, name)
					}
				}
			}
		}
	}
	if checked == 0 {
		t.Fatal("no attribute name was checked, so this test proves nothing")
	}
	t.Logf("checked %d attribute names across %d fixtures", checked, len(fixtureFiles(t)))
}

// fixtureFiles is every committed plan fixture, so a guard like the one above
// runs against everything in the repository rather than a list somebody has to
// remember to extend.
func fixtureFiles(t *testing.T) []string {
	t.Helper()
	entries, err := os.ReadDir("../../testdata")
	if err != nil {
		t.Fatalf("cannot read testdata: %v", err)
	}
	var out []string
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		// Not plans: two are deliberately broken and one is a rules file
		// that happens to live here.
		switch e.Name() {
		case "malformed.json", "notaplan.json", "rules-example.json":
			continue
		}
		out = append(out, e.Name())
	}
	return out
}

// TestEveryReportedNameIsATopLevelKeyOfThisChange is the exact version of the
// guard above, and it exists because the loose one was not enough.
//
// The shape check catches a name that could not have come from a schema. It
// does NOT catch a name that came from one level too deep and happens to look
// like an attribute - descending into an attribute's value and reporting the
// keys of a `tags` map passed it, because tag names are lower snake case too.
//
// This compares against the actual top-level keys of THIS resource's change.
// Any descent, from any route, reports a name that is not among them.
func TestEveryReportedNameIsATopLevelKeyOfThisChange(t *testing.T) {
	checked := 0
	for _, fixture := range fixtureFiles(t) {
		p := loadFixture(t, fixture)

		// Every entry for an address, because a deposed object shares one with
		// its resource and the finding could have come from either.
		top := map[string]map[string]bool{}
		for _, rc := range p.ResourceChanges {
			if rc == nil || rc.Change == nil {
				continue
			}
			if top[rc.Address] == nil {
				top[rc.Address] = map[string]bool{}
			}
			for _, side := range []interface{}{rc.Change.Before, rc.Change.After, rc.Change.AfterUnknown} {
				if m, ok := side.(map[string]interface{}); ok {
					for k := range m {
						top[rc.Address][k] = true
					}
				}
			}
		}

		for _, f := range Assess(p).Findings {
			for _, a := range f.Annotations {
				if a.Code != AnnChangedAttributes {
					continue
				}
				for _, name := range a.Paths {
					checked++
					if !top[f.Address][name] {
						t.Errorf("%s/%s reports %q, which is not a top-level key of that "+
							"resource's before, after or after_unknown - so it came from "+
							"inside a value", fixture, f.Address, name)
					}
				}
			}
		}
	}
	if checked == 0 {
		t.Fatal("no attribute name was checked, so this test proves nothing")
	}
}

// TestAnEmptyUnknownContainerIsNotAChange. Astra's first correctness finding,
// on a plan from real Terraform.
//
// after_unknown mirrors the shape of the resource, and Terraform writes an
// EMPTY container for an attribute that is fully known - "input": {} means
// there are no unknowns inside input, not that input changed. Treating any
// non-null, non-false entry as evidence of change reported an attribute whose
// before and after are identical.
//
// In testdata/unknown-containers.json, terraform_data.probe has
// "triggers_replace": {} and the same value on both sides. Terraform's own
// display shows input and output changing and nothing else.
func TestAnEmptyUnknownContainerIsNotAChange(t *testing.T) {
	a := changedFor(t, Assess(loadFixture(t, "unknown-containers.json")), "terraform_data.probe")
	if a == nil {
		t.Fatal("the update says nothing about what it changes")
	}
	want := []string{"input", "output"}
	if !equalStrings(a.Paths, want) {
		t.Errorf("Paths = %v, want %v - triggers_replace is identical on both sides and its "+
			"after_unknown entry is an empty container, which means no unknowns inside rather "+
			"than a change", a.Paths, want)
	}
}

// TestAnUnsetAttributeIsNotSet. Astra's second correctness finding, also on a
// plan from real Terraform.
//
// terraform_data.empty sets nothing. Terraform emits every optional attribute
// as a known null in `after` and displays only `id` being created. Counting the
// nulls made the number depend on how many optional attributes the provider
// happens to expose, which is a fact about the schema rather than about the
// change - and "sets 5 attributes" about a resource that sets none is simply
// false.
func TestAnUnsetAttributeIsNotSet(t *testing.T) {
	a := changedFor(t, Assess(loadFixture(t, "all-null-create.json")), "terraform_data.empty")
	if a == nil {
		t.Fatal("the create says nothing about what it sets")
	}
	if !equalStrings(a.Paths, []string{"id"}) {
		t.Errorf("Paths = %v, want only id - every other attribute is a known null, which is "+
			"an unset schema slot rather than something being set", a.Paths)
	}
	if !strings.Contains(a.Summary, "sets 1 attribute") {
		t.Errorf("Summary = %q, want the singular count of what it actually sets", a.Summary)
	}
}
