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

	add := func(addr string) {
		if addr == "" || seen[addr] {
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
