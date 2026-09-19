package assess

import (
	"strings"

	tfjson "github.com/hashicorp/terraform-json"
)

// Joining the configuration graph to the resources a plan actually holds.
//
// The graph in blast.go is built from `configuration`, which names a resource
// once however many copies of it exist: `terraform_data.counted`. The change
// set names every copy: `terraform_data.counted[0]`, `[1]`. Nothing joined the
// two, and the consequences were worse than a missing feature.
//
//   - The blast radius NAMED RESOURCES THAT DO NOT EXIST. On a plan with a
//     count of two and a for_each of two it reported "2 resources in this plan
//     depend on it directly" and listed `terraform_data.counted` and
//     `terraform_data.keyed`, neither of which is in the plan. A reviewer
//     looking either of them up finds nothing, and the real answer was six.
//   - The sequencing annotation was SILENT. It looks each dependant up in the
//     change set to find what this plan does to it, and a configuration address
//     is not in there.
//
// `count` and `for_each` are in most real estates, so this was the feature that
// answers "what else breaks if I destroy this" quietly answering "nothing" for
// a large share of the plans it was pointed at.
//
// DEPENDS_ON IS READ HERE TOO. It is a dependency somebody wrote down, and the
// standing caveat only ever admitted to missing the ones nobody wrote. It lives
// on the configuration resource rather than in its expressions, which is why
// walking `expressions[].references` never saw it.

// configAddress is the configuration address an instance address expands from:
// `module.app[0].terraform_data.counted["a"]` becomes
// `module.app.terraform_data.counted`.
//
// IT CANNOT BE A SPLIT ON ".". A for_each key is an arbitrary string and is
// written into the address in quotes, so `terraform_data.k["a.b"]` holds a dot
// that is not a separator and `terraform_data.k["]"]` holds a bracket that does
// not close anything. This walks the string once, skipping what is inside
// brackets and tracking quotes, which is the only way to get both right.
func configAddress(instance string) string {
	var b strings.Builder
	b.Grow(len(instance))

	depth, inQuote, escaped := 0, false, false
	for _, r := range instance {
		switch {
		case escaped:
			escaped = false
		case inQuote && r == '\\':
			escaped = true
		case inQuote:
			if r == '"' {
				inQuote = false
			}
		case r == '"' && depth > 0:
			inQuote = true
		case r == '[':
			depth++
		case r == ']':
			if depth > 0 {
				depth--
			}
		case depth == 0:
			b.WriteRune(r)
		}
	}
	return b.String()
}

// instancesByConfig maps each configuration address to the instances of it this
// plan holds.
//
// Drift entries are included: an object that changed underneath is one of a
// resource's instances like any other, and leaving them out would make the
// graph disagree with itself depending on which list a resource appeared in.
func instancesByConfig(p *tfjson.Plan) map[string][]string {
	out := map[string][]string{}
	seen := map[string]bool{}

	// AN ORPHAN IS NOT DECLARED BY THIS CONFIGURATION, so it gets none of the
	// configuration's dependencies.
	//
	// Reducing a count leaves the instances above the new one being destroyed
	// and nothing else. They are in the change set, and expanding the
	// configuration address to them handed each one every dependency the
	// resource has TODAY - so a resource that now reads something else had its
	// deleted former sibling listed as a casualty of it, and ordered against
	// it. Terraform's graph connects such a destroy to the provider and
	// nothing else.
	//
	// The test is deliberately narrow: an instance that is ONLY being deleted,
	// where another instance of the same configuration is not. A plan that
	// destroys everything - terraform destroy - has every instance
	// delete-only, and excluding those would empty the graph for the case a
	// blast radius matters most.
	orphan := orphanInstances(p)

	add := func(addr string) {
		if addr == "" || seen[addr] || orphan[addr] {
			return
		}
		seen[addr] = true
		cfg := configAddress(addr)
		out[cfg] = append(out[cfg], addr)
	}
	for _, rc := range p.ResourceChanges {
		if rc != nil {
			add(rc.Address)
		}
	}
	for _, rc := range p.ResourceDrift {
		if rc != nil {
			add(rc.Address)
		}
	}
	return out
}

// orphanInstances is every instance that is only being deleted.
//
// A delete-only instance WILL NOT EXIST after the apply, and the configuration
// describes what will exist - so it says nothing about that instance, and
// giving it the configuration's dependencies attributes to it a relationship
// it may never have had. Reducing a count leaves instances behind that were
// created under a configuration that no longer applies to them.
//
// THE DESTROY-PLAN EXEMPTION WAS WRONG AND IS GONE. It kept the edges when
// nothing in the plan was being kept, on the reasoning that `terraform
// destroy` orders its destroys in reverse dependency order and a blast radius
// is worth most there. Astra showed the test cannot tell that plan from one
// that sets every count to zero: both have nothing kept, and in the second the
// surviving instances never had the dependency the configuration now declares.
// Nothing in the file distinguishes them.
//
// So the edges go. A plan that destroys everything now reports no blast radius
// and no ordering, which is a real loss - and it is the side of the contract
// this tool has to come down on, because the alternative is a report that
// invents a dependency and states it as a fact. The radius may be short and
// may never be invented.
func orphanInstances(p *tfjson.Plan) map[string]bool {
	deleteOnly := map[string]bool{}
	for _, rc := range p.ResourceChanges {
		if rc == nil || rc.Change == nil {
			continue
		}
		// A deposed object shares an address with its resource, and one entry
		// being a plain delete does not make the address delete-only.
		if d, seen := deleteOnly[rc.Address]; seen && !d {
			continue
		}
		deleteOnly[rc.Address] = rc.Change.Actions.Delete()
	}
	out := map[string]bool{}
	for addr, only := range deleteOnly {
		if only {
			out[addr] = true
		}
	}
	return out
}

// knownInstances is every instance address this plan holds, as a set.
func knownInstances(p *tfjson.Plan) map[string]bool {
	out := map[string]bool{}
	for _, rc := range p.ResourceChanges {
		if rc != nil {
			out[rc.Address] = true
		}
	}
	for _, rc := range p.ResourceDrift {
		if rc != nil {
			out[rc.Address] = true
		}
	}
	return out
}
