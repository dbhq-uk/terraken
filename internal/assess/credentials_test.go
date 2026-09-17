package assess

import (
	"encoding/json"
	"strings"
	"testing"
)

// The fixture is real terraform output, generated from a root built only from
// terraform_data and the local provider - no real infrastructure, no real
// provider credentials, and every value in it fabricated. It is the second
// fixture in this repository that exists to be leaked from.
const credFixture = "unmarked-credentials.json"

// The values the fixture plants. Nothing in this list is a real credential and
// none of them may ever appear in any output.
var plantedValues = []string{
	"ghp_000000000000000000000000000000000000",
	"postgres://admin:hunter2@db.internal:5432/app",
	"hunter2",
	"AKIAIOSFODNN7EXAMPLE",
	"Zx9Kq2mWv7Lp4Nd8Rt6Yb3Fh5Jc1Ag0Se7Uk2Mo9Qi4Xz",
	"v1.0-fakefakefake-NOTAREALTOKEN-0000000000000000000000000000000000000000",
	"MIIEowIBAAKCAQEAx0000000000000000000000000000000000000000000000000",
	"BEGIN RSA PRIVATE KEY",
}

// vector joins a test value that is fabricated but deliberately well-formed.
//
// GITHUB'S PUSH PROTECTION BLOCKED THE FIRST VERSION OF THIS FILE, on the Slack
// and Vault cases below, and it was not wrong to: a test vector for a
// credential-shape detector has to match the published format or it tests
// nothing, and matching the published format is exactly what a scanner looks
// for. None of these is a credential and none of them ever worked.
//
// Splitting the prefix from the body keeps them out of every OTHER scanner's
// way without weakening what they test here, because the string the detector
// sees is assembled at run time and identical either way. The alternative was
// to click GitHub's "allow this secret" link, which trains everybody involved
// that the block is a formality.
func vector(prefix, body string) string { return prefix + body }

func exposureOf(t *testing.T, fixture string) Exposure {
	t.Helper()
	return Assess(mustPlan(t, fixture)).Exposure
}

func lookedLike(e Exposure, address, path string) (string, bool) {
	for _, v := range e.Values {
		if v.Address == address && v.Path == path {
			return v.Looks, true
		}
	}
	return "", false
}

// THE INCIDENT THAT SHAPED THIS TOOL, as a test.
//
// A live Cloudflare API token was found in the clear in a real plan, at
// variables.cloudflare_api_token.value, despite the variable being declared
// sensitive = true. The reason is structural rather than a bug: the plan's
// top-level variables block records a value and carries no sensitivity
// information at all, so `sensitive = true` buys nothing there.
//
// The fixture reproduces it exactly - a sensitive variable, and its value
// sitting in plaintext in the JSON.
func TestASensitiveVariableStillLandsInTheClearAndIsCaught(t *testing.T) {
	p := mustPlan(t, credFixture)

	// First, the premise. If a future Terraform starts redacting the variables
	// block this test should fail loudly rather than quietly prove nothing.
	v, ok := p.Variables["cloudflare_api_token"]
	if !ok || v == nil {
		t.Fatal("the fixture no longer carries the variable this test is about")
	}
	if s, _ := v.Value.(string); !strings.Contains(s, "NOTAREALTOKEN") {
		t.Fatal("the variables block no longer carries the value in the clear - the premise of this test has changed")
	}

	looks, found := lookedLike(Assess(p).Exposure, AddrRootVariables, "cloudflare_api_token")
	if !found {
		t.Fatal("the token in the root variables block was not detected")
	}
	if looks == "" {
		t.Fatal("detected without saying what it appears to be")
	}
}

// THE TEST THAT MATTERS MOST HERE, and it is adversarial by design.
//
// A detector that finds credentials and then prints them has made the problem
// worse than not looking. The assertion is over the serialised exposure with
// ALL whitespace stripped, so a value broken across lines by a renderer cannot
// slip past a plain substring check.
func TestNoDetectedValueReachesTheExposure(t *testing.T) {
	e := exposureOf(t, credFixture)
	if !e.Any() {
		t.Fatal("nothing detected, so this test proves nothing")
	}

	blob, err := json.Marshal(e)
	if err != nil {
		t.Fatal(err)
	}
	haystacks := []string{string(blob), strings.Join(strings.Fields(string(blob)), "")}

	for _, planted := range plantedValues {
		stripped := strings.Join(strings.Fields(planted), "")
		for _, hay := range haystacks {
			if strings.Contains(hay, planted) {
				t.Fatalf("a detected value reached the exposure: %q", planted)
			}
			if stripped != "" && strings.Contains(hay, stripped) {
				t.Fatalf("a detected value reached the exposure with whitespace stripped: %q", planted)
			}
		}
	}

	// Not even a fragment. A prefix long enough to identify the credential is
	// a leak, and "reports the first four characters" is the shape the rule
	// against masking exists to prevent.
	for _, planted := range plantedValues {
		if len(planted) < 12 {
			continue
		}
		for _, hay := range haystacks {
			if strings.Contains(hay, planted[:12]) {
				t.Fatalf("a 12-character fragment of a detected value reached the exposure: %q", planted[:12])
			}
		}
	}
}

func TestEachRecognisedShapeIsNamedForWhatItIs(t *testing.T) {
	e := exposureOf(t, credFixture)

	for _, tc := range []struct{ address, path, want string }{
		{"terraform_data.app", "input.api_token", "a GitHub token"},
		{"terraform_data.app", "input.database_url", "a connection string with an embedded password"},
		{"terraform_data.aws", "input.access_key_id", "an AWS access key id"},
		{"terraform_data.signing", "input.material", "a private key"},
		{"terraform_data.cdn", "input.edge_handle", "a long, high-entropy string"},
		{AddrRootVariables, "cloudflare_api_token", "an attribute named as a secret, not marked sensitive"},
	} {
		got, found := lookedLike(e, tc.address, tc.path)
		if !found {
			t.Errorf("%s %s was not detected at all", tc.address, tc.path)
			continue
		}
		if got != tc.want {
			t.Errorf("%s %s: looks like %q, want %q", tc.address, tc.path, got, tc.want)
		}
	}
}

func TestTheOrdinaryAttributesAreLeftAlone(t *testing.T) {
	// A section full of bucket names and regions is a section nobody reads, and
	// then the one line that mattered goes with it.
	e := exposureOf(t, credFixture)
	for _, tc := range []struct{ address, path string }{
		{"terraform_data.app", "input.region"},
		{"terraform_data.app", "input.replicas"},
		{"terraform_data.aws", "input.bucket"},
		{"terraform_data.cdn", "input.ttl"},
		{"local_file.readme", "content"},
		{"local_file.readme", "file_permission"},
		{AddrRootVariables, "region"},
	} {
		if looks, found := lookedLike(e, tc.address, tc.path); found {
			t.Errorf("%s %s is innocuous but was reported as %q", tc.address, tc.path, looks)
		}
	}
}

func TestAPlanWithNothingSuspiciousSaysNothingAtAll(t *testing.T) {
	// "A plan with nothing suspicious produces no finding rather than a
	// reassurance." A detector this rough must not be allowed to tell anybody
	// their plan is clean.
	//
	// large-estate.json and real-plan.json are the regression guard for the
	// digest noise: both are ordinary plans full of computed base64 hashes, and
	// an eight-file module in large-estate once produced sixteen reports on its
	// own. A section that long is a section nobody reads.
	for _, fx := range []string{
		"minimal.json", "blast-radius.json", "lowrisk.json",
		"large-estate.json", "real-plan.json", "demo.json",
		"written-differently.json", "rename-no-moved.json", "critical.json",
	} {
		e := exposureOf(t, fx)
		if e.Any() {
			t.Errorf("%s: expected nothing, got %d value(s): %+v", fx, len(e.Values), e.Values)
		}
		if e.Note != "" || e.Advice != "" {
			t.Errorf("%s: a clean plan should carry no note and no advice", fx)
		}
	}
}

func TestWhatTerraformAlreadyMarkedIsNotReported(t *testing.T) {
	// The subject is the GAP in Terraform's marking. Re-reporting values it
	// marked correctly would bury the gap under the cases that are working, and
	// the sensitive annotation already names those paths.
	//
	// The mark is tested by prefix: a mark on a whole block covers everything
	// inside it, and equality alone would walk into the block and report its
	// children.
	marked := markedPrefixes(map[string]interface{}{"input": true}, nil)
	if !marked.covers("input.api_token") {
		t.Fatal("a mark on the parent block does not cover the child - a marked secret would be re-reported")
	}

	whole := markedPrefixes(true, nil)
	if !whole.covers("anything.at.all") {
		t.Fatal("a mark on the whole resource does not cover its attributes")
	}

	none := markedPrefixes(map[string]interface{}{"other": true}, nil)
	if none.covers("input.api_token") {
		t.Fatal("an unrelated mark is covering an unmarked path")
	}
}

func TestTheCaveatAndTheAdviceRideWithEveryDetection(t *testing.T) {
	// "Be honest about confidence... Say so in the finding, every time." The
	// caveat is carried on the report rather than left to each renderer to
	// remember, because a renderer that forgets it ships a detector that reads
	// as certain.
	e := exposureOf(t, credFixture)
	if !strings.Contains(e.Note, "misses credentials") || !strings.Contains(e.Note, "names values that are not credentials") {
		t.Fatalf("the note does not admit both failure directions: %q", e.Note)
	}
	// And the advice is about the FILE, not about editing the plan.
	if !strings.Contains(e.Advice, "rotate") {
		t.Fatalf("the advice does not say to rotate: %q", e.Advice)
	}
	if !strings.Contains(e.Advice, "does not undo the exposure") {
		t.Fatalf("the advice does not warn that editing the file changes nothing: %q", e.Advice)
	}
}

func TestExposureIsDeterministic(t *testing.T) {
	// Map iteration over variables, outputs and attributes is the obvious way
	// to make this output shuffle between runs.
	var first string
	for i := 0; i < 40; i++ {
		b, err := json.Marshal(exposureOf(t, credFixture))
		if err != nil {
			t.Fatal(err)
		}
		if i == 0 {
			first = string(b)
			continue
		}
		if string(b) != first {
			t.Fatalf("run %d differs from the first:\n%s\n%s", i, first, b)
		}
	}
}

func TestADisplayFilterCannotHideTheExposure(t *testing.T) {
	// Filtering to the serious changes must not quietly stop telling somebody
	// their plan file holds a token. Exposure is not a level and --min-level
	// has no business touching it.
	full := Assess(mustPlan(t, credFixture))
	if !full.Exposure.Any() {
		t.Fatal("nothing detected, so this test proves nothing")
	}
	filtered := full.AtLeast(Critical)
	if len(filtered.Findings) >= len(full.Findings) {
		t.Fatal("the filter held nothing back, so this test proves nothing")
	}
	if len(filtered.Exposure.Values) != len(full.Exposure.Values) {
		t.Fatalf("the filter changed the exposure: %d values became %d",
			len(full.Exposure.Values), len(filtered.Exposure.Values))
	}
}

func TestClassifyValueRules(t *testing.T) {
	for _, tc := range []struct {
		name, path, value, want string
	}{
		{"short values are never anything", "password", "abc", ""},
		{"a bare word under a secret name", "db.password", "hunter2", "an attribute named as a secret, not marked sensitive"},
		{"a secret's arn is not the secret", "secret_arn", "arn:aws:secretsmanager:eu-west-2:1:secret:db-AbCdEf", ""},
		{"a secret's name is not the secret", "secret_name", "production-db-password", ""},
		{"rotation settings are not the secret", "password_rotation_enabled", "true", ""},
		{"a public key is not a private one", "public_key", "ssh-rsa AAAAB3NzaC1yc2EAAAADAQABAAABgQDZ1x", ""},
		{"tokenizer is not a token", "tokenizer", "standard", ""},
		{"a uuid is an id", "handle", "3f2504e0-4f89-11d3-9a0c-0305e82c3301", ""},
		{"a sha256 digest is not a secret", "digest", strings.Repeat("a1b2c3d4", 8), ""},
		{"an empty password in a url carries nothing", "url", "postgres://admin:@db.internal:5432/app", ""},
		{"a url with no userinfo", "url", "https://db.internal:5432/app", ""},
		{"a jwt", "header", "eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiIxIn0.abcdefghijkl", "a JSON Web Token"},
		{"a slack token", "hook", vector("xox", "b-0000000000-0000000000-AbCdEfGhIjKlMnOpQrSt"), "a Slack token"},
		{"a vault token", "vault", vector("hvs", ".AbCdEfGhIjKlMnOpQrStUvWx"), "a HashiCorp Vault token"},
		{"prose is not high entropy", "note", "This file is a fixture and it holds no secrets at all", ""},
		{"a long lowercase word run is not random enough", "slug", strings.Repeat("abcde", 10), ""},
		// A base64 digest and a base64 32-byte secret are the same 44
		// characters with the same distribution. Only the name tells them
		// apart, so the name has to be consulted.
		{"a base64 digest is derived, not secret", "content_base64sha256", "Zx9Kq2mWv7Lp4Nd8Rt6Yb3Fh5Jc1Ag0Se7Uk2Mo9Qi4Xz", ""},
		{"the same string under a neutral name is reported", "edge_handle", "Zx9Kq2mWv7Lp4Nd8Rt6Yb3Fh5Jc1Ag0Se7Uk2Mo9Qi4Xz", "a long, high-entropy string"},
		{"an etag is derived", "etag", "Zx9Kq2mWv7Lp4Nd8Rt6Yb3Fh5Jc1Ag0Se7Uk2Mo9Qi4Xz", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := classifyValue(tc.path, tc.value); got != tc.want {
				t.Fatalf("classifyValue(%q, ...) = %q, want %q", tc.path, got, tc.want)
			}
		})
	}
}

func TestClassifyValueNeverReturnsAnythingDerivedFromTheValue(t *testing.T) {
	// Every class is a fixed sentence chosen from a closed set. A class that
	// interpolated any part of the value - a prefix, a length, a character
	// count - would be a leak wearing a hat, and this is what stops one being
	// added later without anybody noticing.
	allowed := map[string]bool{
		"a private key": true,
		"a connection string with an embedded password": true,
		"a JSON Web Token": true,
		"an attribute named as a secret, not marked sensitive": true,
		"a long, high-entropy string":                          true,
	}
	for _, s := range signatures {
		allowed[s.looks] = true
	}
	e := exposureOf(t, credFixture)
	for _, v := range e.Values {
		if !allowed[v.Looks] {
			t.Fatalf("%q is not one of the fixed classes - where did it come from?", v.Looks)
		}
	}
}
