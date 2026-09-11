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

// sensitivePaths walks a sensitivity structure, which has the same shape.
func sensitivePaths(v interface{}) []string {
	var out []string
	walkTrue(v, "", &out)
	sort.Strings(out)
	return out
}

func walkTrue(v interface{}, prefix string, out *[]string) {
	switch t := v.(type) {
	case bool:
		if t && prefix != "" {
			*out = append(*out, prefix)
		}
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
