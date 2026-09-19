package plan

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// The loader re-reads a plan's attribute values so their digits survive, and
// these are the ways that can go wrong.

func write(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "plan.json")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func inputOf(t *testing.T, v interface{}) string {
	t.Helper()
	m, ok := v.(map[string]interface{})
	if !ok {
		t.Fatalf("not an object: %#v", v)
	}
	b, _ := json.Marshal(m["input"])
	return string(b)
}

// TestDigitsSurviveInBothLists. Drift is the same shape as a change and is
// ranked by the same rules, so a number in it has to survive the same way -
// removing drift from the second decode left the whole suite green.
func TestDigitsSurviveInBothLists(t *testing.T) {
	path := write(t, `{
	  "format_version": "1.2",
	  "resource_changes": [
	    {"address":"terraform_data.a","mode":"managed","type":"terraform_data","name":"a",
	     "change":{"actions":["update"],
	       "before":{"input":9007199254740992},"after":{"input":9007199254740993}}}
	  ],
	  "resource_drift": [
	    {"address":"terraform_data.b","mode":"managed","type":"terraform_data","name":"b",
	     "change":{"actions":["update"],
	       "before":{"input":9007199254740992},"after":{"input":9007199254740993}}}
	  ]
	}`)

	p, _, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := inputOf(t, p.ResourceChanges[0].Change.Before); got != "9007199254740992" {
		t.Errorf("resource_changes before.input = %s, want the digits from the file", got)
	}
	if got := inputOf(t, p.ResourceChanges[0].Change.After); got != "9007199254740993" {
		t.Errorf("resource_changes after.input = %s, want the digits from the file", got)
	}
	if got := inputOf(t, p.ResourceDrift[0].Change.Before); got != "9007199254740992" {
		t.Errorf("resource_drift before.input = %s - drift is ranked by the same rules and "+
			"needs the same digits", got)
	}
	if got := inputOf(t, p.ResourceDrift[0].Change.After); got != "9007199254740993" {
		t.Errorf("resource_drift after.input = %s", got)
	}
}

// TestEveryEntryIsReRead, not just the first. Preserving numbers only in the
// first resource entry passed the suite, because every fixture that cared had
// one.
func TestEveryEntryIsReRead(t *testing.T) {
	path := write(t, `{
	  "format_version": "1.2",
	  "resource_changes": [
	    {"address":"terraform_data.first","mode":"managed","type":"terraform_data","name":"first",
	     "change":{"actions":["update"],"before":{"input":1},"after":{"input":2}}},
	    {"address":"terraform_data.second","mode":"managed","type":"terraform_data","name":"second",
	     "change":{"actions":["update"],
	       "before":{"input":9007199254740992},"after":{"input":9007199254740993}}}
	  ]
	}`)

	p, _, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := inputOf(t, p.ResourceChanges[1].Change.After); got != "9007199254740993" {
		t.Errorf("the second entry's after.input = %s, want the digits from the file", got)
	}
}

// TestARepeatedKeyDoesNotResurrectAValue.
//
// A JSON object may repeat a key and encoding/json takes the last one. The
// library decodes `change` into a POINTER, so a later `"change": null` clears
// everything the earlier one set. A value struct in the second decode kept the
// earlier members, and apply() wrote them back over the top - so a `before` the
// library had correctly discarded came back, and the report named fewer
// attributes than the change has.
//
// Crafted rather than generated: Terraform does not emit this, and the point is
// that the two decodes must not disagree about a document both accept.
func TestARepeatedKeyDoesNotResurrectAValue(t *testing.T) {
	path := write(t, `{
	  "format_version": "1.2",
	  "resource_changes": [
	    {"address":"terraform_data.a","mode":"managed","type":"terraform_data","name":"a",
	     "change":{"before":{"input":1}},
	     "change":null,
	     "change":{"actions":["update"],"after":{"input":1,"other":2}}}
	  ]
	}`)

	p, _, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if p.ResourceChanges[0].Change.Before != nil {
		t.Errorf("before = %#v, want nil - the library cleared it and the second decode must "+
			"not put it back", p.ResourceChanges[0].Change.Before)
	}
}
