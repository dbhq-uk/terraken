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

	// Built once for the whole plan, not per finding. The graph is the same
	// for every resource in it, and rebuilding it inside the loop would make
	// a large plan quadratic for no gain.
	g := buildGraph(p)

	for _, rc := range p.ResourceChanges {
		if rc == nil || rc.Change == nil {
			continue
		}
		f := assessOne(rc)
		if ann, ok := moves[rc.Address]; ok {
			f.Annotations = append(f.Annotations, ann)
		}
		if ann, ok := blastAnnotation(g, rc.Address, f.Kind); ok {
			f.Annotations = append(f.Annotations, ann)
		}
		r.Findings = append(r.Findings, f)
	}

	// Most severe first, then widest reach, then address.
	//
	// REACH BREAKS THE TIE WITHIN A LEVEL RATHER THAN CHANGING ONE. A
	// replacement thirty resources depend on is a different event from one
	// nothing depends on, and the report should say so - but it must not say
	// so by escalating, because critical means the resource type holds data
	// and destroying it loses that data, and that is the only escalation in
	// the tool. Widening it would make the word mean two things.
	//
	// The address is still the final tiebreak, so the ordering stays total
	// and the output stays byte-identical between runs.
	sort.SliceStable(r.Findings, func(i, j int) bool {
		a, b := r.Findings[i], r.Findings[j]
		if a.Level != b.Level {
			return a.Level > b.Level
		}
		if ra, rb := reachOf(a), reachOf(b); ra != rb {
			return ra > rb
		}
		return a.Address < b.Address
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

	// Last, over the complete finding set. Computing it earlier would miss
	// any annotation or escalation added above, and computing it after a
	// filter would count less than the plan holds.
	r.Shape = shapeOf(r.Findings)
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
				Detail: "this provider is not on terraken's curated list, so whether destroying this loses data has not been assessed",
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

	// Only a change with both a before and an after has two versions of a
	// value to compare. A create has no before; a delete has no after.
	//
	// These annotations state what was seen and stop there. They must
	// never touch f.Level: order is significant for a container's command,
	// whitespace is significant in a script, a type change can matter, and
	// a tool that scored any of them as harmless would eventually be
	// confidently wrong about one of them.
	if kind == KindUpdate || kind == KindReplace {
		reordered := reorderedPaths(rc.Change.Before, rc.Change.After, rc.Change.AfterUnknown)
		if len(reordered) > 0 {
			// The caveat is carried apart from the fact, in Note, because
			// it is the same sentence on every finding this rule fires
			// on. A terminal with thirty reshuffled sets states it once
			// in the footer; Detail keeps the whole thing for the formats
			// that have no footer to state it in. See reorder.go for the
			// wording and Annotation for the split.
			f.Annotations = append(f.Annotations, Annotation{
				Code:    AnnReordered,
				Detail:  reorderDetail,
				Summary: reorderSummary,
				Note:    reorderNote,
				Paths:   reordered,
			})
		}

		// The other classes of the same thing: a JSON document whose keys
		// moved, a heredoc that was re-indented, a port that came back as
		// a string, a null that became an empty list. The reordered paths
		// are passed in so nothing is reported twice - see rewritten.go
		// for the precedence rule - and the roll-up at the end of the list
		// counts a reordering as accounted for like any other class.
		f.Annotations = append(f.Annotations,
			rewrittenPaths(rc.Change.Before, rc.Change.After, rc.Change.AfterUnknown, reordered).annotations()...)
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

// reachOf is how many resources a finding's blast-radius annotation says
// depend on it, or 0 when it has none. Read off the annotation rather than
// recomputed, so the number that orders the report is the same number the
// report prints.
func reachOf(f Finding) int {
	for _, a := range f.Annotations {
		if a.Code == AnnBlastRadius {
			return len(a.Reached)
		}
	}
	return 0
}
