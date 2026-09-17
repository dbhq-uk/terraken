package assess

import "github.com/dbhq-uk/terraken/internal/plan"

// Kind is what the plan does to a resource.
type Kind string

const (
	KindCreate  Kind = "create"
	KindUpdate  Kind = "update"
	KindDelete  Kind = "delete"
	KindReplace Kind = "replace"
	KindImport  Kind = "import"
	KindRead    Kind = "read"
	KindForget  Kind = "forget"
	KindNoOp    Kind = "no-op"

	// KindUnsupported is an operation this build does not recognise: an
	// action verb it has never seen, or a sequence of verbs Terraform does
	// not document. It is the one kind that says nothing about what the plan
	// does, because nothing is known about what the plan does.
	//
	// It exists because the alternative was worse. The classifier used to
	// end in an unconditional no-op, so a plan from a newer Terraform
	// carrying an action this build had never seen was presented to a
	// reviewer as nothing at all - which is the exact failure this tool is
	// for. An unknown reported as an unknown is the feature.
	KindUnsupported Kind = "unsupported"
)

// Annotation is extra context attached to a finding. Annotations are
// reported separately from the risk level so their reasoning is always
// visible rather than folded silently into a score.
//
// Detail is a finished English sentence, and it is always complete on
// its own. A consumer that reads one annotation with nothing else in
// view - a JSON client, a markdown row quoted into a review comment -
// gets Detail and only Detail, so it can never be the half of a sentence
// that needs a footer to make sense.
//
// A renderer that cannot use a prose blob must not be left with nothing
// to show either, so the sentence is also carried apart, as data:
//
//   - Moved is the evidence behind the missed-moved-block annotation,
//     as fields rather than as prose. It is nil on every other code.
//   - Summary is the fact alone, cut to a label a few words long, with
//     no caveat welded onto it.
//   - Note is the caveat, which belongs to the rule and not to this
//     finding: every annotation sharing a Code carries the same Note
//     word for word.
//
// Summary and Note together are what let a format with somewhere to put
// a standing caveat - a terminal footer - say the fact once per finding
// and the caveat once per report. Repeating a caveat verbatim under
// thirty findings is repetition, not information, and it is how a reader
// is trained to skip the annotations entirely. Both are optional: a
// renderer falls back to Detail when they are empty, which is what every
// annotation without a shared caveat leaves them.
type Annotation struct {
	Code    string `json:"code"`
	Detail  string `json:"detail"`
	Summary string `json:"summary,omitempty"`

	// Note is not serialised. Detail already ends with this same
	// sentence, so emitting both would hand a JSON consumer the caveat
	// twice for every finding that carries it. Note exists only so a
	// renderer with a footer can lift the caveat out of the body; a
	// format without one reads Detail and has lost nothing.
	Note string `json:"-"`

	Paths []string `json:"paths,omitempty"`

	// Moved is set only on an AnnMissedMoved annotation. It is nil on
	// every other code.
	Moved *MovedEvidence `json:"moved,omitempty"`

	// Reached is set only on an AnnBlastRadius annotation, and carries
	// the same addresses as Paths with their depth attached. Paths keeps
	// the flat list every renderer already knows how to print; Reached is
	// for a consumer that wants to rank or group by distance without
	// re-deriving it. Both are the same set, shallowest first.
	Reached []Reached `json:"reached,omitempty"`
}

// MovedEvidence is the evidence behind a possible-missed-moved-block
// annotation, as fields rather than as a sentence: which two addresses
// were paired, how many compared attributes matched, and whether the
// pairing crossed a module boundary.
//
// Matched and Compared are the true, unadjusted attribute counts. The
// same-module preference used when ranking candidates never reaches
// here, so what is reported is always what was actually compared.
type MovedEvidence struct {
	From        string `json:"from"`
	To          string `json:"to"`
	Matched     int    `json:"matched"`
	Compared    int    `json:"compared"`
	CrossModule bool   `json:"cross_module"`

	// Rivals names every OTHER create that matched the deleted resource
	// exactly as well as the chosen one did. Empty is the normal case and
	// means the evidence picked a winner; non-empty means the tiebreak did,
	// and a proposal must refuse rather than let the alphabet decide which
	// object's state gets adopted under which address.
	Rivals     []string `json:"rivals,omitempty"`
	FromModule string   `json:"from_module,omitempty"`
	ToModule   string   `json:"to_module,omitempty"`
}

// Annotation codes.
const (
	AnnMissedMoved   = "possible-missed-moved-block"
	AnnUnverifiable  = "unverifiable-until-apply"
	AnnSensitive     = "sensitive"
	AnnUnknownVendor = "unrecognised-provider"

	// The operation itself was not recognised, so nothing below it was
	// assessed either. Its Paths carry the plan's whole action sequence,
	// quoted - Terraform's own vocabulary, not attribute values.
	AnnUnsupportedAction = "unsupported-operation"

	// What else in the plan depends on a resource being destroyed or
	// replaced. Set on destructive changes only, and only when something
	// is actually reached - see blast.go for why an empty radius is not
	// reported rather than reported as zero.
	AnnBlastRadius = "blast-radius"

	// A finding produced by one of the reader's OWN rules rather than by
	// the tool's judgement. Kept as its own code so a consumer can tell
	// the two apart without parsing prose - see rules.go.
	AnnRule = "your-rule"
	// AnnReordered names a fact, not a verdict: the before and after of a
	// list hold the same elements in a different order. Order is
	// significant for some attributes, so this never changes a finding's
	// level and never claims the change is meaningless.
	AnnReordered = "same-elements-reordered"

	// The classes of attribute whose before and after are the same value
	// written a different way. Each names a fact and the class it falls
	// into, on the same terms as AnnReordered: every one of them has a
	// case where the difference is real, so none of them changes a
	// finding's level and none of them claims the change is meaningless.
	// See rewritten.go for the rules and the wording.
	AnnSameWhitespace = "same-text-different-whitespace"
	AnnSameNumber     = "same-number-written-differently"
	AnnNullAndEmpty   = "null-on-one-side-empty-on-the-other"
	AnnSameJSON       = "same-json-written-differently"

	// AnnAllRewritten is the roll-up: every attribute this plan shows as
	// changed on the resource is one of the classes above, including a
	// reordering. It is still a statement of fact rather than a verdict,
	// and it is the most useful thing this tool can say about an update in
	// place that is really nothing.
	AnnAllRewritten = "every-changed-attribute-written-differently"
)

// Finding is one resource change, assessed.
type Finding struct {
	Address      string       `json:"address"`
	Type         string       `json:"type"`
	Module       string       `json:"module,omitempty"`
	Provider     string       `json:"provider,omitempty"`
	Kind         Kind         `json:"kind"`
	Level        Level        `json:"-"`
	LevelName    string       `json:"level"`
	Reason       string       `json:"reason,omitempty"`
	ReplacePaths []string     `json:"replace_paths,omitempty"`
	DataLoss     bool         `json:"data_loss"`
	Annotations  []Annotation `json:"annotations,omitempty"`
}

// Report is the whole assessment of one plan.
//
// Counts always describes the whole assessment. Findings may hold less
// than that if a display filter was applied, in which case Hidden says
// how many were held back and HiddenBelow says what the bar was. A
// renderer must use those to say so out loud: showing less without
// saying so is how a critical finding gets missed.
type Report struct {
	TerraformVersion string         `json:"terraform_version,omitempty"`
	FormatVersion    string         `json:"format_version,omitempty"`
	Findings         []Finding      `json:"findings"`
	Counts           map[Level]int  `json:"-"`
	CountsByName     map[string]int `json:"counts"`
	Hidden           int            `json:"hidden,omitempty"`
	HiddenBelow      string         `json:"hidden_below,omitempty"`

	// Unassessed is how many findings carry no severity at all, because the
	// tool could not read the operation and so had nothing to rank. They are
	// in Findings like anything else and they are NOT in Counts, which is
	// what design.md asks for: if it is not one resource change losing data,
	// it belongs outside the severity counts rather than at the top of them.
	//
	// A renderer must state it. Counting it nowhere and printing it nowhere
	// would be the original bug wearing a different hat.
	//
	// IT IS A TALLY, NOT A COVERAGE FIGURE, and the two come apart in
	// exactly one case. A team rule may give an unreadable operation a
	// severity, at which point it counts as that severity here and leaves
	// this number - the totals have to add up to len(Findings), and a
	// finding cannot be both info and unranked. The coverage fact survives
	// on the finding itself, as Kind == KindUnsupported, which is what the
	// gate's unsupported array is keyed on. Ranking something is not
	// understanding it.
	Unassessed int `json:"unassessed,omitempty"`

	// Shape summarises the WHOLE plan, and keeps doing so after a display
	// filter is applied - see AtLeast. A summary that shrank with
	// --min-level would tell a reviewer the change is smaller than it is.
	Shape Shape `json:"shape"`

	// Status is what the plan says about ITSELF - errored, complete,
	// applyable - each true, false or not stated.
	//
	// IT IS NOT A FINDING AND IT IS NOT IN THE COUNTS. A finding is one
	// resource change, assessed, and a failed planning operation is not a
	// resource change. docs/design.md decides where new information goes: if
	// it is not one resource change losing data, it belongs outside the
	// severity counts rather than at the top of them. So --fail-on cannot see
	// it and --min-level cannot hide it; it is carried beside the findings
	// like Shape and Exposure, for the same reason.
	//
	// Set by the command from what the loader read, because the three flags
	// are decoded separately from the plan - see internal/plan/status.go.
	// Assess does not fill it in, and a zero Status renders as nothing at all,
	// so a caller that does not set it gets exactly the report it got before
	// this field existed.
	Status plan.Status `json:"status"`

	// Exposure is what the plan FILE is carrying - values that look like
	// credentials and that Terraform did not mark sensitive. It describes the
	// artefact rather than the change, which is why it is here rather than on
	// a finding, and like Shape it survives a display filter: a reader who
	// filters down to the serious changes must not stop being told their plan
	// file holds a token. See credentials.go.
	Exposure Exposure `json:"exposure"`
}

// Max returns the highest SEVERITY present in the report, and false when
// the report holds none.
//
// It steps over Unranked rather than reading Findings[0] straight off the
// top, and that is the whole of --fail-on's contract with this type. An
// unrankable finding sorts first so a reader meets it first, but it has no
// severity, so handing it to a severity threshold would silently change
// what a pinned --fail-on critical means the day Terraform ships a new
// action verb. It is reported everywhere and it decides nothing; a team
// that wants it to decide something writes a rule, which gives it a real
// severity and brings it back into this answer.
func (r Report) Max() (Level, bool) {
	max, any := Info, false
	for _, f := range r.Findings {
		if f.Level == Unranked {
			continue
		}
		if !any || f.Level > max {
			max, any = f.Level, true
		}
	}
	return max, any
}

// AtLeast returns the report with only the findings at min or above kept
// for display.
//
// Counts is deliberately left alone. A filter changes what you read, not
// what was found, and a summary that quietly recounted itself around the
// filter would be the worst of both: fewer lines and a number that
// agrees with them. Call Max on the unfiltered report, never on this
// one - what fails a build must not depend on what was displayed.
func (r Report) AtLeast(min Level) Report {
	kept := make([]Finding, 0, len(r.Findings))
	for _, f := range r.Findings {
		// AN OPERATION THE TOOL COULD NOT READ IS NEVER FILTERED OUT, and
		// this is tested on the KIND rather than on the level for a reason.
		// A finding still at Unranked would survive the comparison below on
		// its own, because Unranked is above every threshold. A team rule
		// can take it off that level, though, and a team rule can be broad -
		// "all terraform_data changes are info here" is a reasonable thing
		// to write - so without this a filter could quietly stop reporting
		// that part of the plan was never read. Ranking something is not
		// understanding it.
		if f.Kind == KindUnsupported || f.Level >= min {
			kept = append(kept, f)
		}
	}

	// SHAPE AND EXPOSURE ARE CARRIED ACROSS UNCHANGED, and that is
	// load-bearing rather than incidental. Shape summarises the whole plan;
	// recomputing it from `kept` would make the summary shrink with the filter
	// and tell a reviewer the change is smaller than it is. Exposure describes
	// the plan file and has nothing to do with any finding's level, so a filter
	// must not be able to hide it at all. The struct copy below is what
	// preserves both - do not "tidy" this into a fresh Report.
	out := r
	out.Hidden = len(r.Findings) - len(kept)
	if out.Hidden > 0 {
		out.HiddenBelow = min.String()
	}
	out.Findings = kept
	return out
}
