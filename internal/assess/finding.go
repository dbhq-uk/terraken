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
type Annotation struct {
	Code   string   `json:"code"`
	Detail string   `json:"detail"`
	Paths  []string `json:"paths,omitempty"`
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
