package assess

import (
	"fmt"
	"strings"

	tfjson "github.com/hashicorp/terraform-json"
)

// humanReason turns Terraform's own machine-readable action reason into a
// sentence. Terraform already knows why it is doing this; there is no
// need to infer it.
func humanReason(r tfjson.ActionReason) string {
	switch r {
	case tfjson.ActionReasonReplaceBecauseCannotUpdate:
		return "an attribute changed that cannot be updated in place"
	case tfjson.ActionReasonReplaceBecauseTainted:
		return "the resource is tainted"
	case tfjson.ActionReasonReplaceByRequest:
		return "replacement was explicitly requested"
	case tfjson.ActionReasonReplaceByTriggers:
		return "a replace_triggered_by reference changed"
	case tfjson.ActionReasonDeleteBecauseNoResourceConfig:
		return "its configuration block was removed"
	case tfjson.ActionReasonDeleteBecauseWrongRepetition:
		return "it uses the wrong repetition argument for its module"
	case tfjson.ActionReasonDeleteBecauseCountIndex:
		return "its count index is out of range"
	case tfjson.ActionReasonDeleteBecauseEachKey:
		return "its for_each key is no longer present"
	case tfjson.ActionReasonDeleteBecauseNoModule:
		return "its module is no longer called"
	case tfjson.ActionReasonDeleteBecauseNoMoveTarget:
		return "a moved block points at a resource that does not exist"
	case tfjson.ActionReasonReadBecauseConfigUnknown:
		return "its configuration contains unknown values"
	case tfjson.ActionReasonReadBecauseDependencyPending:
		return "it depends on something not yet created"
	case tfjson.ActionReasonReadBecauseCheckNested:
		return "it is nested inside a check block"
	}
	return ""
}

// flattenPath renders one of Terraform's replace_paths entries as a
// readable attribute path: ["delegation", 0, "name"] becomes
// "delegation[0].name".
func flattenPath(raw interface{}) string {
	parts, ok := raw.([]interface{})
	if !ok {
		return ""
	}
	var b strings.Builder
	for _, p := range parts {
		switch v := p.(type) {
		case string:
			if b.Len() > 0 {
				b.WriteString(".")
			}
			b.WriteString(v)
		case float64:
			fmt.Fprintf(&b, "[%d]", int(v))
		default:
			fmt.Fprintf(&b, "[%v]", v)
		}
	}
	return b.String()
}
