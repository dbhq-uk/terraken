package render

import (
	"bytes"
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/dbhq-uk/terraken/internal/assess"
	tfjson "github.com/hashicorp/terraform-json"
)

// reportFor assesses a committed fixture, so these tests read what the command
// would print rather than what a hand-built report would.
func reportFor(t *testing.T, fixture string) assess.Report {
	t.Helper()
	b, err := os.ReadFile("../../testdata/" + fixture)
	if err != nil {
		t.Fatalf("committed fixture %s is missing: %v", fixture, err)
	}
	var p tfjson.Plan
	if err := json.Unmarshal(b, &p); err != nil {
		t.Fatalf("fixture %s did not parse: %v", fixture, err)
	}
	return assess.Assess(&p)
}

func renderAs(t *testing.T, format string, r assess.Report) string {
	t.Helper()
	var b bytes.Buffer
	if err := Write(&b, format, r, Options{
		Terminal:  TerminalOptions{Width: testWidth},
		Threshold: "high",
	}); err != nil {
		t.Fatalf("Write(%q) returned error: %v", format, err)
	}
	return b.String()
}

// TestEveryFormatTellsTheTwoReplacementsApart is the acceptance the issue asks
// for, held against the format registry rather than against a list.
//
// The two fixtures are the same configuration planned by real Terraform, one
// with create_before_destroy and one without. They differ in the order of one
// actions array and in nothing else a reader can see, so if a format renders
// them identically that format has thrown the distinction away - which is what
// every format did before this feature existed.
//
// Keyed on Formats, so a renderer added later cannot skip it.
func TestEveryFormatTellsTheTwoReplacementsApart(t *testing.T) {
	destroyFirst := reportFor(t, "replace-destroy-first.json")
	createFirst := reportFor(t, "replace-create-first.json")

	for _, format := range Formats {
		t.Run(format, func(t *testing.T) {
			a := renderAs(t, format, destroyFirst)
			b := renderAs(t, format, createFirst)
			if a == b {
				t.Errorf("%s renders a destroy-before-create replacement and a "+
					"create-before-destroy one identically, so the distinction does not "+
					"reach the reader", format)
			}
		})
	}
}

// TestTheActionLineNamesTheOrder pins the one line a reviewer reads first.
//
// "destroy and create" beside a replacement whose new object is made first is
// not a vague line, it is a wrong one: it states an order, and the order is
// the opposite of what the plan says. Both phrasings name the sequence.
func TestTheActionLineNamesTheOrder(t *testing.T) {
	cases := []struct {
		order assess.ReplaceOrder
		want  string
	}{
		{assess.ReplaceDestroyFirst, "destroy, then create"},
		{assess.ReplaceCreateFirst, "create, then destroy"},
	}
	for _, c := range cases {
		got := verb(assess.Finding{Kind: assess.KindReplace, ReplaceOrder: c.order})
		if got != c.want {
			t.Errorf("verb for %s = %q, want %q", c.order, got, c.want)
		}
	}
}

// TestAReplacementWithNoStatedOrderStillReads covers the finding assembled by
// hand - a test, a rule, a future caller - rather than by assessOne. It has no
// ordering, and the line falls back to naming both steps without claiming a
// sequence, because claiming one would be inventing it.
func TestAReplacementWithNoStatedOrderStillReads(t *testing.T) {
	got := verb(assess.Finding{Kind: assess.KindReplace})
	if got == "" {
		t.Fatal("a replacement with no stated ordering renders no action at all")
	}
	for _, claim := range []string{"then"} {
		if strings.Contains(got, claim) {
			t.Errorf("verb = %q, which states a sequence the finding does not carry", got)
		}
	}
}

// TestTheGateCarriesTheOrdering is the machine half of the acceptance. A
// caller deciding whether to proceed is exactly who most needs to know whether
// the thing goes away before its replacement exists.
func TestTheGateCarriesTheOrdering(t *testing.T) {
	cases := map[string]string{
		"replace-destroy-first.json": "destroy-before-create",
		"replace-create-first.json":  "create-before-destroy",
	}
	for fixture, want := range cases {
		t.Run(fixture, func(t *testing.T) {
			var v struct {
				Schema   string `json:"schema"`
				Blocking []struct {
					Address      string `json:"address"`
					ReplaceOrder string `json:"replace_order"`
				} `json:"blocking"`
			}
			if err := json.Unmarshal([]byte(renderAs(t, "gate", reportFor(t, fixture))), &v); err != nil {
				t.Fatalf("gate output did not parse: %v", err)
			}
			if len(v.Blocking) != 1 {
				t.Fatalf("expected 1 blocking finding, got %d", len(v.Blocking))
			}
			if v.Blocking[0].ReplaceOrder != want {
				t.Errorf("replace_order = %q, want %q", v.Blocking[0].ReplaceOrder, want)
			}
			// ADDING A FIELD IS NOT A BREAKING CHANGE, so the schema does not
			// move. The v1 policy in gate.go promises fields are only added
			// within a version, and a parser reading the fields it knows keeps
			// working. Pinned here so a version bump has to be deliberate.
			if v.Schema != GateSchema || GateSchema != "terraken.gate/v1" {
				t.Errorf("schema = %q, want terraken.gate/v1 - adding a field is not a "+
					"breaking change and does not move the version", v.Schema)
			}
		})
	}
}

// TestNothingButAReplacementGetsAReplaceOrderKey checks the gate omits the key
// rather than emitting an empty one. A caller reading "replace_order": "" on a
// destroy would be reading an answer to a question nobody asked.
func TestNothingButAReplacementGetsAReplaceOrderKey(t *testing.T) {
	out := renderAs(t, "gate", reportFor(t, "real-plan.json"))
	var v struct {
		Blocking []map[string]any `json:"blocking"`
	}
	if err := json.Unmarshal([]byte(out), &v); err != nil {
		t.Fatalf("gate output did not parse: %v", err)
	}
	found := 0
	for _, f := range v.Blocking {
		if _, ok := f["replace_order"]; ok {
			found++
			if f["kind"] != "replace" {
				t.Errorf("%v carries replace_order and is a %v", f["address"], f["kind"])
			}
		}
	}
	if found == len(v.Blocking) && len(v.Blocking) > 0 {
		t.Errorf("every blocking finding in real-plan.json carries replace_order, but the "+
			"fixture holds deletes as well: %v", v.Blocking)
	}
}

// TestTheJSONReportCarriesTheOrdering is the other machine output. It is the
// human report serialised, so the field arrives from the finding itself.
func TestTheJSONReportCarriesTheOrdering(t *testing.T) {
	var v struct {
		Findings []struct {
			ReplaceOrder string `json:"replace_order"`
		} `json:"findings"`
	}
	if err := json.Unmarshal([]byte(renderAs(t, "json", reportFor(t, "replace-create-first.json"))), &v); err != nil {
		t.Fatalf("json output did not parse: %v", err)
	}
	if len(v.Findings) != 1 {
		t.Fatalf("expected 1 finding, got %d", len(v.Findings))
	}
	if v.Findings[0].ReplaceOrder != "create-before-destroy" {
		t.Errorf("replace_order = %q, want create-before-destroy", v.Findings[0].ReplaceOrder)
	}
}

// TestDriftStillReadsInThePastTense guards the seam this feature opens into
// the drift list. driftVerb is separate from verb precisely so a past-tense
// entry never borrows a future-tense word, and an ordering claim is the most
// future-tense thing in the report.
func TestDriftStillReadsInThePastTense(t *testing.T) {
	got := driftVerb(assess.KindReplace)
	if strings.Contains(got, "then") || strings.Contains(got, "destroy and create") {
		t.Errorf("driftVerb = %q, which describes an apply that is not going to happen", got)
	}
}

// TestAHostileReplaceOrderIsSanitisedLikeEverythingElse holds the ordering to
// the standard untrusted.go sets for itself: a string this package did not
// choose gets no exemption for looking like one it did.
//
// assessOne only ever sets one of two constants, so the plan cannot reach this
// field today. A Report is a struct a caller fills in, though, and the field is
// rendered - so the exemption would be "this is safe because of what fills it
// in", which is exactly the reasoning the second guarantee exists to refuse.
func TestAHostileReplaceOrderIsSanitisedLikeEverythingElse(t *testing.T) {
	r := assess.Report{
		Findings: []assess.Finding{{
			Address:      "terraform_data.a",
			Kind:         assess.KindReplace,
			LevelName:    "high",
			ReplaceOrder: assess.ReplaceOrder("create-before-destroy\x1b[2J\r\u202e"),
		}},
		CountsByName: map[string]int{"high": 1},
	}
	got := string(sanitise(r).Findings[0].ReplaceOrder)
	for _, raw := range []string{"\x1b", "\r", "\u202e"} {
		if strings.Contains(got, raw) {
			t.Errorf("ReplaceOrder = %q, which still carries a raw %q", got, raw)
		}
	}
	if !strings.Contains(got, `\x1b`) || !strings.Contains(got, `\u202e`) {
		t.Errorf("ReplaceOrder = %q - the escapes must be made VISIBLE rather than dropped, "+
			"because dropping them makes two different strings render identically", got)
	}
}

// orderInOutput is the exact text each format must carry for each ordering,
// keyed on format name and held against Formats.
//
// TestEveryFormatTellsTheTwoReplacementsApart compares whole reports, and
// Astra showed what that misses: rendering verb(assess.Finding{Kind: f.Kind})
// in the terminal, markdown and HTML sends every replacement's action line
// back to "destroy and create", and the reports still differ because the
// ANNOTATION differs. "Something differs" is not the contract. The line the
// reviewer reads first is the contract.
var orderInOutput = map[string]struct{ destroyFirst, createFirst string }{
	"terminal": {"destroy, then create", "create, then destroy"},
	"md":       {"| HIGH | destroy, then create |", "| HIGH | create, then destroy |"},
	"html":     {`<p class="verb">destroy, then create</p>`, `<p class="verb">create, then destroy</p>`},
	// No action line in either machine format: the ordering is a field, which
	// is the better shape for a parser and is asserted as such.
	"json": {`"replace_order": "destroy-before-create"`, `"replace_order": "create-before-destroy"`},
	"gate": {`"replace_order": "destroy-before-create"`, `"replace_order": "create-before-destroy"`},
}

func TestEveryFormatNamesTheOrderItself(t *testing.T) {
	for _, format := range Formats {
		want, ok := orderInOutput[format]
		if !ok {
			t.Errorf("%s is a format with no entry in orderInOutput, so nobody has decided "+
				"how it says which way round a replacement happens", format)
			continue
		}
		t.Run(format, func(t *testing.T) {
			cases := []struct{ fixture, want, wrong string }{
				{"replace-destroy-first.json", want.destroyFirst, want.createFirst},
				{"replace-create-first.json", want.createFirst, want.destroyFirst},
			}
			for _, c := range cases {
				out := renderAs(t, format, reportFor(t, c.fixture))
				if !strings.Contains(out, c.want) {
					t.Errorf("%s of %s does not contain %q", format, c.fixture, c.want)
				}
				if strings.Contains(out, c.wrong) {
					t.Errorf("%s of %s contains %q, which is the OTHER ordering", format, c.fixture, c.wrong)
				}
			}
		})
	}
}

// TestTheFallbackActionLineIsExact pins the wording for a replacement carrying
// no ordering, rather than only banning one word in it. A test that accepts
// any string without "then" accepts the empty string.
func TestTheFallbackActionLineIsExact(t *testing.T) {
	if got := verb(assess.Finding{Kind: assess.KindReplace}); got != "destroy and create" {
		t.Errorf("verb with no ordering = %q, want %q - both steps named, no sequence claimed",
			got, "destroy and create")
	}
}

// TestDriftReplacementVerbIsExact pins driftVerb for the same reason. Banning
// "then" in it passed on the empty string too.
func TestDriftReplacementVerbIsExact(t *testing.T) {
	if got := driftVerb(assess.KindReplace); got != "replaced" {
		t.Errorf("driftVerb(replace) = %q, want %q - past tense, no sequence, because nothing "+
			"here is about to be applied", got, "replaced")
	}
}

// TestTheGateCarriesTheSequence. A caller deciding whether to proceed is
// exactly who needs to know that this change takes four other things down
// before it goes and brings them back after.
func TestTheGateCarriesTheSequence(t *testing.T) {
	var v struct {
		Blocking []struct {
			Address        string   `json:"address"`
			Paths          []string `json:"paths"`
			Depends        []string `json:"depends"`
			DestroyedFirst []string `json:"destroyed_first"`
			ChangedAfter   []string `json:"changed_after"`
		} `json:"blocking"`
	}
	out := renderAs(t, "gate", reportFor(t, "sequence-chain.json"))
	if err := json.Unmarshal([]byte(out), &v); err != nil {
		t.Fatalf("gate output did not parse: %v", err)
	}
	var base *struct {
		Address        string   `json:"address"`
		Paths          []string `json:"paths"`
		Depends        []string `json:"depends"`
		DestroyedFirst []string `json:"destroyed_first"`
		ChangedAfter   []string `json:"changed_after"`
	}
	for i := range v.Blocking {
		if v.Blocking[i].Address == "terraform_data.base" {
			base = &v.Blocking[i]
		}
	}
	if base == nil {
		t.Fatal("terraform_data.base is not in the gate's blocking list")
	}
	want := []string{"terraform_data.middle", "terraform_data.leaf"}
	if !equalStringSlices(base.DestroyedFirst, want) {
		t.Errorf("destroyed_first = %v, want %v", base.DestroyedFirst, want)
	}
	if !equalStringSlices(base.ChangedAfter, want) {
		t.Errorf("changed_after = %v, want %v", base.ChangedAfter, want)
	}
	// RESOURCE ADDRESSES ARE NOT ATTRIBUTE PATHS. The blast radius already has
	// this trap and the gate already avoids it there: a caller parsing `paths`
	// as attribute paths would be handed "terraform_data.middle" and have no
	// way to tell it from "tags.Name".
	for _, p := range base.Paths {
		if strings.HasPrefix(p, "terraform_data.") {
			t.Errorf("paths holds the resource address %q, which is not an attribute path", p)
		}
	}

	// AND THE SUBSET CASE, which is where the trap actually springs. The
	// sequence annotation carries no Paths when it covers the whole blast
	// radius, so a test using only sequence-chain.json would pass with the
	// addresses going straight into `paths`.
	out = renderAs(t, "gate", reportFor(t, "sequence-partial.json"))
	if err := json.Unmarshal([]byte(out), &v); err != nil {
		t.Fatalf("gate output did not parse: %v", err)
	}
	found := false
	for _, b := range v.Blocking {
		if b.Address != "terraform_data.base" {
			continue
		}
		found = true
		for _, p := range b.Paths {
			if strings.HasPrefix(p, "terraform_data.") {
				t.Errorf("paths holds the resource address %q on the subset case", p)
			}
		}
		if !equalStringSlices(b.ChangedAfter, []string{"terraform_data.rebuilt"}) {
			t.Errorf("changed_after = %v, want the one dependant this plan changes", b.ChangedAfter)
		}
	}
	if !found {
		t.Fatal("terraform_data.base is not in the subset fixture's blocking list")
	}
}

func equalStringSlices(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
