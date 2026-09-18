// Package assess turns a parsed plan into ranked findings.
//
// Everything in this package is pure: plan in, report out. No clock, no
// network, no filesystem. That is what makes every rule testable as a
// table, and it is deliberate.
package assess

import (
	"sort"

	"github.com/dbhq-uk/terraken/internal/plan"
	"strconv"

	tfjson "github.com/hashicorp/terraform-json"
)

// Assess evaluates every resource change in the plan.
func Assess(p *tfjson.Plan) Report { return AssessWithRules(p, nil) }

// AssessWithRules is Assess, plus a team's own rules over the same evaluation.
//
// IT DOES NOT DISCARD `complete`. The pinned decoder models that one flag, so
// it is sitting on the plan already and there is no reason for a caller
// without a loader status to lose it - passing a wholly zero status here meant
// Assess and AssessWithRules missed the plan-not-complete gap on a plan that
// stated it, while the command found it. Only `errored` and `applyable` need
// the loader, because only they are absent from the decoder.
func AssessWithRules(p *tfjson.Plan, rules *RuleSet) Report {
	var st plan.Status
	if p != nil {
		st.Complete = p.Complete
	}
	return AssessWithStatus(p, rules, st)
}

// AssessWithStatus is Assess, plus a team's own rules and what the plan says
// about itself.
//
// A nil rule set is exactly Assess, so the rules path costs nothing when it is
// not used - and every existing caller keeps working unchanged.
//
// THE STATUS IS TAKEN RATHER THAN READ, because two of its three flags are not
// in the pinned decoder and only the loader has them - see
// internal/plan/status.go. Coverage needs one of them: a plan that is not
// complete is a plan that is not the whole change, which is one of the five
// silences this report exists to name. A zero Status states nothing, which is
// what every caller that does not have one should pass.
func AssessWithStatus(p *tfjson.Plan, rules *RuleSet, status plan.Status) Report {
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

	// The change set by address, built once beside the graph and for the same
	// reason. Sequencing needs to know what this plan does to each resource
	// the graph reaches, and re-scanning ResourceChanges per finding would
	// make a large plan quadratic.
	kinds := kindsByAddress(p)

	for _, rc := range p.ResourceChanges {
		if rc == nil || rc.Change == nil {
			continue
		}
		f := assessOne(rc)

		// NOTHING ELSE CLAIMS ANYTHING ABOUT AN OPERATION NOBODY UNDERSTOOD.
		// assessOne stops at its own annotations; these two are attached
		// from outside it and are looked up by address, which a deposed
		// object shares with the current one - so without this guard an
		// unreadable operation could inherit a rename proposal belonging to
		// the recognised change beside it.
		if f.Kind != KindUnsupported {
			if ann, ok := moves[rc.Address]; ok {
				f.Annotations = append(f.Annotations, ann)
			}
			if ann, ok := blastAnnotation(g, rc.Address, f.Kind); ok {
				f.Annotations = append(f.Annotations, ann)
			}
			// AFTER the blast radius, because it is the answer to the
			// question the blast radius raises. "Six things depend on this"
			// comes first; "and this plan destroys four of them before it"
			// follows from it and reads as nonsense on its own.
			if ann, ok := sequenceAnnotation(g, rc.Address, f.Kind, kinds); ok {
				f.Annotations = append(f.Annotations, ann)
			}
		}

		// THE READER'S OWN RULES, over the finding the tool has already made
		// from THIS change - which is what lets a rule say
		// "level_at_least: high" and mean the tool's own ranking rather than
		// restating it.
		//
		// Evaluated here, with the change and its finding in the same hand,
		// rather than afterwards over the sorted list. An address does not
		// identify an object - a deposed instance carries the same one as
		// the current instance - so matching by address let a rule rank the
		// wrong object, or miss one entirely. See rulesFor.
		//
		// A rule may raise OR lower a level. Lowering is not a mistake to
		// guard against: a team that knows a particular destroy is routine
		// in their estate is better served by saying so than by learning to
		// ignore a critical, which is how a real one gets missed.
		if hits := rulesFor(rules, rc, f); len(hits) > 0 {
			// THE HIGHEST SEVERITY WINS WHEN SEVERAL RULES MATCH. Assigning
			// each in turn let the last one seen overwrite the rest, so a
			// critical rule lost to a high one purely on where its message
			// sorted - which is the worst possible way to decide a severity.
			//
			// A team with a critical rule and a high rule both matching one
			// resource means that resource is critical. Taking the maximum
			// is the only reading that cannot quietly downgrade something
			// somebody deliberately marked.
			level := hits[0].level
			for _, h := range hits {
				f.Annotations = append(f.Annotations, h.ann)
				if h.level > level {
					level = h.level
				}
			}
			f.Level = level
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

		// UNRANKED IS COUNTED APART, never in the severity tallies. It is
		// not a fifth severity, it is the absence of one, and design.md
		// decides this case by name: an action the tool does not recognise
		// is not one resource change losing data, so it belongs outside the
		// counts rather than at the top of them.
		//
		// This is measured AFTER the rules above, which is what makes the
		// escape hatch work. A team rule matching actions: ["unsupported"]
		// gives the finding a real severity, and from that moment it counts
		// as that severity and Max can see it - so --fail-on blocks on it,
		// because somebody wrote down that it should.
		if r.Findings[i].Level == Unranked {
			r.Unassessed++
			continue
		}
		r.Counts[r.Findings[i].Level]++
		r.CountsByName[r.Findings[i].Level.String()]++
	}

	// Last, over the complete finding set. Computing it earlier would miss
	// any annotation or escalation added above, and computing it after a
	// filter would count less than the plan holds.
	r.Shape = shapeOf(r.Findings)

	// LAST, AND OVER THE WHOLE PLAN. Coverage joins what the findings say
	// about themselves to what the plan says about itself, so it has to come
	// after every finding is final - and it counts the whole plan rather than
	// a filtered view, which Report.AtLeast carries across untouched.
	// WHAT CHANGED UNDERNEATH, in its own list. Ranked by the same rules as a
	// planned change and counted by none of them - see drift.go.
	r.Drift = driftOf(p)

	// Checks Terraform could not confirm. See checks.go, and note what it
	// deliberately does not carry.
	r.Checks = checksOf(p)

	r.Status = status
	r.Coverage = coverageOf(p, status, r.Findings, len(r.Drift))

	// Read off the WHOLE plan, not off the findings: the root variables block
	// and the output changes are not resource changes and have no finding to
	// hang from, and the root variables block is where the real incident was.
	r.Exposure = detectExposure(p)
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

	// FIRST, AND THEN NOTHING ELSE IS CLAIMED. Every annotation below this
	// reads the change on the assumption that the operation was understood -
	// what it destroys, what forces it, what cannot be verified. None of
	// those readings is safe when the operation itself was not recognised,
	// and a report that ranked the attributes of an operation it could not
	// name would be exactly as confident and exactly as wrong as the no-op
	// it replaced.
	if kind == KindUnsupported {
		f.Annotations = append(f.Annotations, unsupportedAnnotation(rc.Change.Actions))
		return f
	}

	f.Reason = humanReason(rc.ActionReason)
	for _, raw := range rc.Change.ReplacePaths {
		if p := flattenPath(raw); p != "" {
			f.ReplacePaths = append(f.ReplacePaths, p)
		}
	}

	// WHICH WAY ROUND. The plan carries it in the order of the actions array
	// and terraken used to throw it away, so two replacements the plan says
	// happen in opposite orders read identically in the report - and the
	// action line stated one of the two orders as fact, which made it wrong
	// for half of them.
	//
	// It sits beside the kind rather than splitting it, and it never touches
	// the level - see ReplaceOrder for both reasons.
	if kind == KindReplace {
		f.ReplaceOrder = ReplaceDestroyFirst
		if rc.Change.Actions.CreateBeforeDestroy() {
			f.ReplaceOrder = ReplaceCreateFirst
		}
		f.Annotations = append(f.Annotations, replaceOrderAnnotation(f.ReplaceOrder))
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
//
// THE FALLBACK SAYS SO. An action verb this build has never seen, or a
// sequence of verbs Terraform does not document, ends here as
// KindUnsupported at Unranked - never as a no-op at Info. The loader
// validates format_version and nothing validates the action vocabulary, so
// before this a plan from a newer Terraform carrying an unfamiliar action
// was presented to a reviewer as nothing at all.
//
// The recognised shapes are exactly the eight the pinned terraform-json
// helpers answer yes to: each of the six verbs alone, plus [delete, create]
// and [create, delete]. Everything else - an empty array, a repeated verb,
// any other pair, any sequence of three - is unrecognised. That is stricter
// than it needs to be today and deliberately so: a false unsupported is a
// line in a report, a false no-op is a change nobody looked at.
func classify(rc *tfjson.ResourceChange) (Kind, Level) {
	a := rc.Change.Actions

	// A DATA SOURCE IS EXEMPT FROM THE RANKING, NOT FROM BEING RECOGNISED.
	// The shortcut used to be unconditional, so a data resource carrying any
	// action at all came back as a harmless read. Terraform emits read or
	// no-op for one; anything else here is as unknown as it is anywhere.
	if rc.Mode == tfjson.DataResourceMode {
		if a.Read() || a.NoOp() {
			return KindRead, Info
		}
		return KindUnsupported, Unranked
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
		// Import metadata names a no-op that is really an adoption. It does
		// not rescue a sequence that was not recognised in the first place,
		// which is why it is tested inside this case and not beside it.
		if rc.Change.Importing != nil {
			return KindImport, Info
		}
		return KindNoOp, Info
	}
	return KindUnsupported, Unranked
}

// unsupportedAnnotation names the vocabulary the classifier did not
// recognise, and says the impact was not assessed.
//
// THE ACTION STRINGS ARE QUOTED HERE, at the point they enter the report,
// rather than at each of the five places it leaves. They are read out of a
// plan file, which is untrusted input, and the terminal renderer has no
// escaping of its own - an action carrying an ANSI sequence could otherwise
// recolour or erase the report it appears in. strconv.Quote turns every
// control character, newline and non-printable rune into a visible escape,
// so what a reader sees is what the file actually held. The per-format
// escaping still applies on top: markdown and HTML each have characters
// that survive quoting and matter to them.
//
// The whole sequence is named, in order, including verbs that ARE
// recognised on their own. [delete, quarantine] is not a delete with a
// footnote; it is one operation this build cannot read, and printing half
// of it would suggest otherwise.
func unsupportedAnnotation(a tfjson.Actions) Annotation {
	quoted := make([]string, 0, len(a))
	for _, act := range a {
		quoted = append(quoted, strconv.Quote(string(act)))
	}
	detail := "this build does not recognise the operation this plan asks for, so its impact cannot be assessed and nothing below it was ranked"
	if len(quoted) == 0 {
		detail += ". The plan names no action at all for this resource"
	}
	return Annotation{
		Code:    AnnUnsupportedAction,
		Detail:  detail,
		Summary: "terraken cannot assess this operation",
		Paths:   quoted,
	}
}

// replaceOrderAnnotation says what the ordering means, in one clause, and
// stops.
//
// IT STATES THE ORDER AND NAMES NO CAUSE. The obvious sentence for the second
// case is "create_before_destroy is set", and it would be wrong: the lifecycle
// block is not in plan JSON at all, and the rule propagates down the
// dependency chain, so a resource that never sets it is planned this way when
// something downstream of it does. See ReplaceOrder.
//
// IT DOES NOT RULE. Neither ordering is presented as the right one.
//
// IT STATES THE SEQUENCE AND NOTHING FOLLOWING FROM IT. The first version of
// the create-first sentence said "there is no point during the apply at which
// this resource does not exist", and that is false rather than merely
// optimistic. A local_file with a fixed filename and create_before_destroy
// plans ["create", "delete"]: Terraform writes the file for the new object,
// then the old object's destroy removes that same path, and the file is gone
// when the apply finishes. That was run, not argued about.
//
// The order of two operations is what the plan states. Whether the thing those
// operations act on survives is a question about the provider, and the plan
// does not answer it - a destroy can be a no-op on one resource type and the
// end of a database on another. So each sentence names the sequence and stops,
// which is also the discipline the outage-window work will need most.
func replaceOrderAnnotation(o ReplaceOrder) Annotation {
	if o == ReplaceCreateFirst {
		return Annotation{
			Code:    AnnReplaceOrder,
			Detail:  "this plan creates the replacement before destroying the existing object",
			Summary: "the replacement is created first",
		}
	}
	return Annotation{
		Code:    AnnReplaceOrder,
		Detail:  "this plan destroys the existing object before creating its replacement",
		Summary: "destroyed before the replacement is created",
	}
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
