// Package assess turns a parsed plan into ranked findings.
//
// Everything in this package is pure: plan in, report out. No clock, no
// network, no filesystem. That is what makes every rule testable as a
// table, and it is deliberate.
package assess

import (
	"sort"

	tfjson "github.com/hashicorp/terraform-json"
)

// Assess evaluates every resource change in the plan.
func Assess(p *tfjson.Plan) Report {
	r := Report{
		TerraformVersion: p.TerraformVersion,
		FormatVersion:    p.FormatVersion,
		Counts:           map[Level]int{},
		CountsByName:     map[string]int{},
	}

	for _, rc := range p.ResourceChanges {
		if rc == nil || rc.Change == nil {
			continue
		}
		r.Findings = append(r.Findings, assessOne(rc))
	}

	// Most severe first. Ties broken by address so output is deterministic.
	sort.SliceStable(r.Findings, func(i, j int) bool {
		if r.Findings[i].Level != r.Findings[j].Level {
			return r.Findings[i].Level > r.Findings[j].Level
		}
		return r.Findings[i].Address < r.Findings[j].Address
	})

	for _, f := range r.Findings {
		r.Counts[f.Level]++
		r.CountsByName[f.Level.String()]++
	}
	return r
}

func assessOne(rc *tfjson.ResourceChange) Finding {
	kind, level := classify(rc)

	f := Finding{
		Address:  rc.Address,
		Type:     rc.Type,
		Module:   rc.ModuleAddress,
		Provider: rc.ProviderName,
		Kind:     kind,
		Level:    level,
	}
	f.LevelName = f.Level.String()
	return f
}

// classify maps the plan's actions onto a kind and a base risk level.
func classify(rc *tfjson.ResourceChange) (Kind, Level) {
	a := rc.Change.Actions

	// A data source read carries no risk to managed infrastructure.
	if rc.Mode == tfjson.DataResourceMode {
		return KindRead, Info
	}

	switch {
	case a.Replace():
		return KindReplace, High
	case a.Delete():
		return KindDelete, High
	case a.Update():
		return KindUpdate, Low
	case a.Create():
		return KindCreate, Info
	case a.Forget():
		// Removed from state but left in place. Not destructive to the
		// resource, but it stops being managed.
		return KindForget, Low
	case a.Read():
		return KindRead, Info
	case a.NoOp():
		if rc.Change.Importing != nil {
			return KindImport, Info
		}
		return KindNoOp, Info
	}
	return KindNoOp, Info
}
