package assess

import (
	"fmt"
	"sort"
)

// unknownPaths walks Terraform's after_unknown structure and returns the
// attribute paths whose values cannot be known until apply.
//
// after_unknown mirrors the shape of the resource, with true at any leaf
// that is unknown.
func unknownPaths(v interface{}) []string {
	var out []string
	walkTrue(v, "", &out)
	sort.Strings(out)
	return out
}

// sensitivePaths walks every sensitivity structure it is given - they
// have the same shape as after_unknown - and returns the union of the
// marked paths, deduplicated and sorted.
//
// It takes more than one because both before_sensitive and
// after_sensitive have to be read. A delete has no "after", so its only
// marker is in before_sensitive: reading after_sensitive alone meant
// destroying azurerm_key_vault_secret.db_password produced no sensitive
// annotation while the matching create produced one, leaving the
// higher-risk half of the pair as the silent one.
//
// Nothing leaks either way. No attribute value is ever printed, marked
// or not - only the path is named.
func sensitivePaths(vs ...interface{}) []string {
	seen := map[string]bool{}
	var out []string
	for _, v := range vs {
		var found []string
		walkTrue(v, "", &found)
		for _, p := range found {
			if seen[p] {
				continue
			}
			seen[p] = true
			out = append(out, p)
		}
	}
	sort.Strings(out)
	return out
}

func walkTrue(v interface{}, prefix string, out *[]string) {
	switch t := v.(type) {
	case bool:
		if !t {
			return
		}
		if prefix == "" {
			// A bare true at the root means the entire object is unknown
			// or sensitive, not any one attribute. Reporting nothing here
			// would be worse than reporting it imprecisely.
			*out = append(*out, "(whole resource)")
			return
		}
		*out = append(*out, prefix)
	case map[string]interface{}:
		for k, child := range t {
			next := k
			if prefix != "" {
				next = prefix + "." + k
			}
			walkTrue(child, next, out)
		}
	case []interface{}:
		for i, child := range t {
			walkTrue(child, fmt.Sprintf("%s[%d]", prefix, i), out)
		}
	}
}
