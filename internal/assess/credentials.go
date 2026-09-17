package assess

import (
	"math"
	"regexp"
	"sort"
	"strings"

	tfjson "github.com/hashicorp/terraform-json"
)

// Credentials a plan is carrying that Terraform did not mark sensitive.
//
// WHY THIS EXISTS. Terraform marks a value sensitive when a provider schema
// says the attribute is sensitive, or when a sensitive variable flows into it.
// Schemas are not exhaustive and the flow analysis has holes, so the marking is
// best effort. This project found a live API token sitting in the clear in a
// real plan - at `variables.cloudflare_api_token.value`, where a variable
// declared `sensitive = true` still lands in plaintext because the top-level
// variables block carries no sensitivity information at all.
//
// The tool's answer to that has been defensive: it prints no attribute value in
// any format, ever. That protects the report. It does nothing for the person
// who still has the file, and the file is the problem.
//
// WHAT THIS IS NOT. It is not a secret scanner for a repository and it does not
// rewrite anybody's artefacts. The scope is one plan file, read once, and the
// output is a list of paths.
//
// THE OUTPUT IS PATHS AND CLASSES. A detected value never reaches the output -
// not masked, not truncated, not as a length. Reporting "a 40-character string
// beginning ghp_" would be a leak wearing a hat.

// ExposedValue is one value in the plan that looks like a credential and that
// Terraform did not mark sensitive.
//
// It carries no value and no part of one. Looks is the tool's own sentence
// about what the thing appears to be, chosen from a fixed set, and it is
// written so that it can never be a paraphrase of the value itself.
type ExposedValue struct {
	// Address is the resource the value sits on, or one of the two
	// pseudo-addresses below for the parts of a plan that are not a resource.
	Address string `json:"address"`
	Path    string `json:"path"`
	Looks   string `json:"looks_like"`
}

// The two pseudo-addresses. Bracketed so they cannot be confused with a real
// resource address, which never starts with a bracket.
const (
	AddrRootVariables = "(root variables)"
	AddrOutputs       = "(outputs)"
)

// ExposureNote is the honesty, and it is carried on every report that has
// anything to say rather than left to a renderer to remember.
//
// The issue asks for it in those words: "Be honest about confidence. This is
// pattern matching, and it will both miss things and flag innocent ones. Say so
// in the finding, every time."
const ExposureNote = "This is pattern matching over the plan's own values: it misses credentials it does not recognise, and it names values that are not credentials. Treat it as a reason to check, never as a clean bill of health."

// ExposureConfidence is the same admission, cut short enough to repeat on every
// single entry in a machine format without the caveat costing more than the
// finding. The long form is stated once where there is somewhere to state it
// once - a terminal footer, a paragraph under a table - and a machine format
// has nowhere, because a caller lifts one entry out and leaves the rest.
const ExposureConfidence = "pattern matching: this misses credentials it does not recognise, and names values that are not credentials"

// ExposureAdvice is what to do about it, which is not what a reader's first
// instinct will be. Editing the plan does not un-expose anything: the value has
// been written to a file, and wherever that file has been is where the
// credential has been.
const ExposureAdvice = "Treat this plan file as a secret: store it accordingly, and rotate whatever it turns out to hold. Editing the file does not undo the exposure."

// Exposure is what the plan FILE is carrying, as opposed to what the change
// does. It is a property of the artefact, not of any one resource change, which
// is why it hangs off the report rather than off a finding.
//
// THAT PLACEMENT IS LOAD-BEARING. A credential in the clear is not a risk level
// and it must not be one: Level means how much damage the change can do, and
// data loss is the only thing in this tool that escalates it. Folding an
// exposure into a level would make the word mean two things, and worse, it
// would put the fact behind --min-level - so a reader filtering down to the
// serious changes would stop being told their plan file holds a token. Carried
// here, it survives every display filter untouched.
type Exposure struct {
	Values []ExposedValue `json:"values,omitempty"`
	Note   string         `json:"note,omitempty"`
	Advice string         `json:"advice,omitempty"`
}

// Any reports whether anything was detected. A plan with nothing suspicious
// produces an empty Exposure and no reassurance - a tool that says "no
// credentials found" over a detector this rough is teaching the reader to
// believe something it cannot support.
func (e Exposure) Any() bool { return len(e.Values) > 0 }

// detectExposure reads the whole plan: root variables, every resource change,
// and every output change.
//
// ROOT VARIABLES FIRST, because that is where the real incident was and because
// it is the one place with no sensitivity information to consult - the
// variables block records a value and nothing else, whatever the variable was
// declared as.
func detectExposure(p *tfjson.Plan) Exposure {
	var e Exposure
	if p == nil {
		return e
	}
	seen := map[string]bool{}
	add := func(address, path, looks string) {
		if looks == "" {
			return
		}
		key := address + "\x00" + path
		if seen[key] {
			return
		}
		seen[key] = true
		e.Values = append(e.Values, ExposedValue{Address: address, Path: path, Looks: looks})
	}

	for name, v := range p.Variables {
		if v == nil {
			continue
		}
		walkStrings(v.Value, name, func(path, s string) {
			add(AddrRootVariables, path, classifyValue(path, s))
		})
	}

	for _, rc := range p.ResourceChanges {
		if rc == nil || rc.Change == nil {
			continue
		}
		// EXCLUDED: anything Terraform DID mark. The whole point is the gap in
		// the marking, and re-reporting what is already marked would bury the
		// gap under the cases that are working correctly.
		marked := markedPrefixes(rc.Change.BeforeSensitive, rc.Change.AfterSensitive)
		scan := func(v interface{}) {
			walkStrings(v, "", func(path, s string) {
				if marked.covers(path) {
					return
				}
				add(rc.Address, path, classifyValue(path, s))
			})
		}
		scan(rc.Change.After)
		scan(rc.Change.Before)
	}

	for name, oc := range p.OutputChanges {
		if oc == nil {
			continue
		}
		marked := markedPrefixes(oc.BeforeSensitive, oc.AfterSensitive)
		scan := func(v interface{}) {
			walkStrings(v, name, func(path, s string) {
				// An output's own path starts at its name, so the mark - which
				// is relative to the output's value - is tested against the
				// remainder.
				if marked.covers(strings.TrimPrefix(strings.TrimPrefix(path, name), ".")) {
					return
				}
				add(AddrOutputs, path, classifyValue(path, s))
			})
		}
		scan(oc.After)
		scan(oc.Before)
	}

	if len(e.Values) == 0 {
		return e
	}
	sort.Slice(e.Values, func(i, j int) bool {
		a, b := e.Values[i], e.Values[j]
		if a.Address != b.Address {
			return a.Address < b.Address
		}
		return a.Path < b.Path
	})
	e.Note = ExposureNote
	e.Advice = ExposureAdvice
	return e
}

// prefixSet is the set of paths Terraform marked sensitive, tested by prefix so
// that a mark on a whole block covers everything inside it. A mark on `input`
// means every attribute under it is marked, and testing for equality alone
// would walk straight into the block and report its children.
type prefixSet struct {
	all    bool
	paths  []string
	lookup map[string]bool
}

func markedPrefixes(vs ...interface{}) prefixSet {
	ps := prefixSet{lookup: map[string]bool{}}
	for _, p := range sensitivePaths(vs...) {
		if p == "(whole resource)" {
			ps.all = true
			continue
		}
		ps.lookup[p] = true
		ps.paths = append(ps.paths, p)
	}
	return ps
}

func (ps prefixSet) covers(path string) bool {
	if ps.all {
		return true
	}
	if ps.lookup[path] {
		return true
	}
	for _, p := range ps.paths {
		if strings.HasPrefix(path, p+".") || strings.HasPrefix(path, p+"[") {
			return true
		}
	}
	return false
}

// walkStrings visits every string leaf, building the same dotted-and-indexed
// path the rest of the package uses.
func walkStrings(v interface{}, prefix string, fn func(path, s string)) {
	switch t := v.(type) {
	case string:
		if prefix == "" {
			return
		}
		fn(prefix, t)
	case map[string]interface{}:
		for k, child := range t {
			next := k
			if prefix != "" {
				next = prefix + "." + k
			}
			walkStrings(child, next, fn)
		}
	case []interface{}:
		for i, child := range t {
			walkStrings(child, prefix+"["+itoa(i)+"]", fn)
		}
	}
}

// The recognised shapes. Each is a published, documented credential format, so
// a match is high confidence - which is why these run before anything
// heuristic and why each gets to name what it is.
var signatures = []struct {
	re    *regexp.Regexp
	looks string
}{
	{regexp.MustCompile(`\b(?:AKIA|ASIA|AIDA|AGPA|ANPA|ANVA|ABIA|ACCA)[0-9A-Z]{16}\b`), "an AWS access key id"},
	{regexp.MustCompile(`\bgh[pousr]_[A-Za-z0-9]{36,}`), "a GitHub token"},
	{regexp.MustCompile(`\bgithub_pat_[A-Za-z0-9_]{20,}`), "a GitHub fine-grained token"},
	{regexp.MustCompile(`\bglpat-[A-Za-z0-9_-]{20,}`), "a GitLab token"},
	{regexp.MustCompile(`\bxox[abprs]-[A-Za-z0-9-]{10,}`), "a Slack token"},
	{regexp.MustCompile(`\b[rs]k_(?:live|test)_[A-Za-z0-9]{20,}`), "a Stripe key"},
	{regexp.MustCompile(`\bAIza[0-9A-Za-z_-]{35}\b`), "a Google API key"},
	{regexp.MustCompile(`\bSG\.[A-Za-z0-9_-]{16,}\.[A-Za-z0-9_-]{16,}`), "a SendGrid API key"},
	{regexp.MustCompile(`\bnpm_[A-Za-z0-9]{36}\b`), "an npm token"},
	{regexp.MustCompile(`\bdo[oprt]_v1_[a-f0-9]{64}\b`), "a DigitalOcean token"},
	{regexp.MustCompile(`\bsk-ant-[A-Za-z0-9_-]{20,}`), "an Anthropic API key"},
	{regexp.MustCompile(`\bhv[sb]\.[A-Za-z0-9_-]{20,}`), "a HashiCorp Vault token"},
	{regexp.MustCompile(`\bEAACEdEose0cBA[0-9A-Za-z]+`), "a Facebook access token"},
	{regexp.MustCompile(`\bSK[0-9a-fA-F]{32}\b`), "a Twilio key"},
}

// A URL with a password in the userinfo. The password group forbids `/` so a
// path cannot be mistaken for one, and requires at least one character so
// `scheme://user:@host` - which carries nothing - stays silent.
var connectionString = regexp.MustCompile(`^[a-zA-Z][a-zA-Z0-9+.-]*://[^\s:/@]+:[^\s/@]+@`)

// Three base64url segments. A JWT is not always a credential, but a bearer
// token in a plan is worth a line either way, and the header segment pins the
// shape tightly enough that ordinary text does not match.
var jsonWebToken = regexp.MustCompile(`^eyJ[A-Za-z0-9_-]{4,}\.[A-Za-z0-9_-]{4,}\.[A-Za-z0-9_-]*$`)

// Attribute names that say what the value is. The word has to stand alone
// between separators, so `token` matches `api_token` and not `tokenizer`.
var secretName = regexp.MustCompile(`(?i)(^|[._-])(passwords?|passwd|pwd|secrets?|tokens?|credentials?|passphrase|private_?keys?|client_?secret|connection_?string|api_?keys?|access_?keys?|secret_?keys?|auth_?tokens?|sas_?token|bearer)($|[._-])`)

// ...and the names that merely refer to one. A secret's ARN, its version, the
// rotation schedule and the algorithm it uses are all safe to hold in the
// clear, and flagging them is how a reader learns to skip this section.
var notItself = regexp.MustCompile(`(?i)(public|_id$|_ids$|name|arn|path|file|algorithm|length|rotation|enabled|disabled|version|expiry|expires|ttl|policy|description|type|count|format|url|uri|endpoint|hash|digest|fingerprint|thumbprint|recovery|reference)`)

// Attribute names whose value is DERIVED from something else, so however
// random it looks it is not a credential.
//
// This exists because the entropy heuristic could not tell the difference on
// its own, and provably cannot: `content_base64sha256` on a local_file is a
// base64-encoded 32-byte digest, and a base64-encoded 32-byte random secret is
// the same 44 characters with the same distribution. Nothing in the value
// distinguishes them - only the name does. Without this, one ordinary
// eight-file module produced sixteen reports and the section became unreadable.
var derivedName = regexp.MustCompile(`(?i)(sha\d|md5|crc|checksum|hash|digest|etag|fingerprint|thumbprint|signature|base64|revision|serial|uuid|guid|version|^id$|_id$|arn)`)

var (
	uuid     = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)
	hexOnly  = regexp.MustCompile(`^[0-9a-fA-F]+$`)
	tokenish = regexp.MustCompile(`^[A-Za-z0-9+/=_.:-]+$`)
)

// classifyValue names what a value appears to be, or returns "" for no.
//
// The order is confidence order, and it matters: a published format gets to
// name itself before a heuristic gets to call it "a long, high-entropy string",
// because the specific answer is the useful one.
func classifyValue(path, v string) string {
	if len(v) < 6 {
		return ""
	}

	// A private key, wherever it sits and whatever the attribute is called.
	// This is the one class with no false-positive story worth worrying about.
	if strings.Contains(v, "-----BEGIN") && strings.Contains(v, "PRIVATE KEY") {
		return "a private key"
	}
	for _, s := range signatures {
		if s.re.MatchString(v) {
			return s.looks
		}
	}
	if connectionString.MatchString(v) {
		return "a connection string with an embedded password"
	}
	if jsonWebToken.MatchString(v) {
		return "a JSON Web Token"
	}

	// Name-led. The attribute says what it holds and Terraform did not mark it,
	// which is the plain form of the gap this whole file is about.
	if seg := lastSegment(path); secretName.MatchString(seg) && !notItself.MatchString(seg) {
		if !strings.ContainsAny(v, " \t\r\n") && !strings.Contains(v, "://") {
			return "an attribute named as a secret, not marked sensitive"
		}
	}

	if !derivedName.MatchString(lastSegment(path)) && looksRandom(v) {
		return "a long, high-entropy string"
	}
	return ""
}

// lastSegment is the attribute's own name, without its parents and without a
// list index. `input.api_token` is `api_token`; `users[2].password` is
// `password`.
func lastSegment(path string) string {
	if i := strings.LastIndex(path, "."); i >= 0 {
		path = path[i+1:]
	}
	if i := strings.Index(path, "["); i >= 0 {
		path = path[:i]
	}
	return path
}

// looksRandom is the last resort, and it is deliberately hard to satisfy.
//
// A plan is full of long opaque strings that are not credentials: resource ids,
// ARNs, digests, certificate bodies, base64 user data. Every exclusion below
// exists because one of those would otherwise be reported, and a section full
// of resource ids is a section nobody reads. The cost is misses, which is
// exactly what ExposureNote says out loud.
func looksRandom(v string) bool {
	// Long enough to be a generated secret, short enough not to be a document.
	// A key file or a certificate is a different shape and is caught above by
	// its header, or not at all.
	if len(v) < 40 || len(v) > 200 {
		return false
	}
	if strings.ContainsAny(v, " \t\r\n") {
		return false
	}
	if strings.Contains(v, "://") || strings.HasPrefix(v, "arn:") {
		return false
	}
	if uuid.MatchString(v) {
		return false
	}
	// Pure hex is overwhelmingly a digest, an id or a fingerprint. A hex
	// secret is a real miss and an accepted one: reporting every sha256 in a
	// plan would drown the cases that matter.
	if hexOnly.MatchString(v) {
		return false
	}
	if !tokenish.MatchString(v) {
		return false
	}
	var upper, lower, digit bool
	for _, r := range v {
		switch {
		case r >= 'A' && r <= 'Z':
			upper = true
		case r >= 'a' && r <= 'z':
			lower = true
		case r >= '0' && r <= '9':
			digit = true
		}
	}
	if !(upper && lower && digit) {
		return false
	}
	return shannon(v) >= 4.0
}

// shannon is entropy in bits per character, over the string's own alphabet.
func shannon(v string) float64 {
	if v == "" {
		return 0
	}
	counts := map[rune]int{}
	n := 0
	for _, r := range v {
		counts[r]++
		n++
	}
	var h float64
	for _, c := range counts {
		p := float64(c) / float64(n)
		h -= p * math.Log2(p)
	}
	return h
}
