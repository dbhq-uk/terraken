package assess

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
)

// Annotation is extra context attached to a finding. Annotations are
// reported separately from the risk level so their reasoning is always
// visible rather than folded silently into a score.
//
// Detail is a finished English sentence, written for the terminal. A
// renderer that cannot use a prose blob - a markdown table cell, a JSON
// consumer that wants the numbers - must not be left with nothing to
// show, so the evidence behind the sentence is carried as data too.
// Moved does that for the missed-moved-block annotation, the one whose
// evidence matters most.
type Annotation struct {
	Code   string   `json:"code"`
	Detail string   `json:"detail"`
	Paths  []string `json:"paths,omitempty"`

	// Moved is set only on an AnnMissedMoved annotation. It is nil on
	// every other code.
	Moved *MovedEvidence `json:"moved,omitempty"`
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
	FromModule  string `json:"from_module,omitempty"`
	ToModule    string `json:"to_module,omitempty"`
}

// Annotation codes.
const (
	AnnMissedMoved   = "possible-missed-moved-block"
	AnnUnverifiable  = "unverifiable-until-apply"
	AnnSensitive     = "sensitive"
	AnnUnknownVendor = "unrecognised-provider"
	// AnnReordered names a fact, not a verdict: the before and after of a
	// list hold the same elements in a different order. Order is
	// significant for some attributes, so this never changes a finding's
	// level and never claims the change is meaningless.
	AnnReordered = "same-elements-reordered"
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
}

// Max returns the highest level present in the report, and false if there
// are no findings at all.
func (r Report) Max() (Level, bool) {
	if len(r.Findings) == 0 {
		return Info, false
	}
	return r.Findings[0].Level, true
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
		if f.Level >= min {
			kept = append(kept, f)
		}
	}

	out := r
	out.Hidden = len(r.Findings) - len(kept)
	if out.Hidden > 0 {
		out.HiddenBelow = min.String()
	}
	out.Findings = kept
	return out
}
