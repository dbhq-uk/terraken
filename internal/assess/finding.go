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
type Report struct {
	TerraformVersion string         `json:"terraform_version,omitempty"`
	FormatVersion    string         `json:"format_version,omitempty"`
	Findings         []Finding      `json:"findings"`
	Counts           map[Level]int  `json:"-"`
	CountsByName     map[string]int `json:"counts"`
}

// Max returns the highest level present in the report, and false if there
// are no findings at all.
func (r Report) Max() (Level, bool) {
	if len(r.Findings) == 0 {
		return Info, false
	}
	return r.Findings[0].Level, true
}
