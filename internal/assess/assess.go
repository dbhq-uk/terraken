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
		// Initialised, not left nil. A nil slice marshals as null, so a
		// clean plan produced "findings": null and jq '.findings[]'
		// failed with "Cannot iterate over null" - on the one plan whose
		// answer is good news.
		Findings:     []Finding{},
		Counts:       map[Level]int{},
		CountsByName: map[string]int{},
	}

	moves := detectMissedMoves(p.ResourceChanges)

	for _, rc := range p.ResourceChanges {
		if rc == nil || rc.Change == nil {
			continue
		}
		f := assessOne(rc)
		if ann, ok := moves[rc.Address]; ok {
			f.Annotations = append(f.Annotations, ann)
		}
		r.Findings = append(r.Findings, f)
	}

	// Most severe first. Ties broken by address so output is deterministic.
	sort.SliceStable(r.Findings, func(i, j int) bool {
		if r.Findings[i].Level != r.Findings[j].Level {
			return r.Findings[i].Level > r.Findings[j].Level
		}
		return r.Findings[i].Address < r.Findings[j].Address
	})

	// LevelName is derived from Level here, after sorting and after every
	// task's mutation of a finding is complete - never inside assessOne.
	// That is what stops a later escalation step from changing Level
	// without LevelName following it, which would ship a finding whose
	// JSON disagrees with its own sort position.
	for i := range r.Findings {
		r.Findings[i].LevelName = r.Findings[i].Level.String()
		r.Counts[r.Findings[i].Level]++
		r.CountsByName[r.Findings[i].Level.String()]++
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

	f.Reason = humanReason(rc.ActionReason)
	for _, raw := range rc.Change.ReplacePaths {
		if p := flattenPath(raw); p != "" {
			f.ReplacePaths = append(f.ReplacePaths, p)
		}
	}

	// Escalation applies to destruction only. Updating a database in
	// place does not lose data.
	destructive := kind == KindDelete || kind == KindReplace
	if destructive {
		if IsDataLoss(rc.Type) {
			f.DataLoss = true
			f.Level = Critical
		} else if !knownProvider(rc.Type) {
			// Do not assume an unrecognised type is safe. Say so.
			f.Annotations = append(f.Annotations, Annotation{
				Code:   AnnUnknownVendor,
				Detail: "this provider is not on terraverdict's curated list, so whether destroying this loses data has not been assessed",
			})
		}
	}

	if paths := unknownPaths(rc.Change.AfterUnknown); len(paths) > 0 {
		f.Annotations = append(f.Annotations, Annotation{
			Code:   AnnUnverifiable,
			Detail: "these values are not known until apply, so no claim about them can be checked in review",
			Paths:  paths,
		})
	}

	// Both sides. A delete has no "after", so a destroyed secret is
	// marked only in before_sensitive - and destroying a secret is the
	// case that most needs saying out loud.
	if paths := sensitivePaths(rc.Change.BeforeSensitive, rc.Change.AfterSensitive); len(paths) > 0 {
		f.Annotations = append(f.Annotations, Annotation{
			Code:   AnnSensitive,
			Detail: "these values are sensitive and are redacted in all output",
			Paths:  paths,
		})
	}

	// Only a change with both a before and an after has two orderings to
	// compare. A create has no before; a delete has no after.
	//
	// This annotation states what was seen and stops there. It must never
	// touch f.Level: order is significant for a container's command, an
	// ordered listener rule, a route table, and a tool that scored this as
	// harmless would eventually be confidently wrong about one of them.
	if kind == KindUpdate || kind == KindReplace {
		if paths := reorderedPaths(rc.Change.Before, rc.Change.After, rc.Change.AfterUnknown); len(paths) > 0 {
			f.Annotations = append(f.Annotations, Annotation{
				Code: AnnReordered,
				// Worded so no hyphen can be stranded at the start of a
				// wrapped line, where it reads as a bullet rather than as
				// punctuation.
				Detail: "these lists hold the same elements in a different order. Order is significant " +
					"for some attributes, such as a container command or an ordered rule list, so " +
					"whether this one matters is yours to judge",
				Paths: paths,
			})
		}
	}

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
