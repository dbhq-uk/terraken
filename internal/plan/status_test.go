package plan

import (
	"strings"
	"testing"
)

// The three flags a plan carries about ITSELF, as opposed to about the
// resources in it.
//
// ABSENT AND FALSE ARE DIFFERENT FACTS and the whole of this file is about
// keeping them apart. Terraform added `errored` in v1.7 and `complete` and
// `applyable` in v1.8, so a plan from in between states one and not the
// others; an older plan, or OpenTofu, may state none of them. "This plan does not converge" and "this build cannot tell
// whether it converges" are different things to tell a reviewer, and a bool
// cannot hold both.
func TestStatusDistinguishesAbsentFromFalse(t *testing.T) {
	for _, tc := range []struct {
		name                         string
		json                         string
		errored, complete, applyable *bool
	}{
		{
			name: "a plan from before the flags existed",
			json: `{"format_version":"1.1","terraform_version":"1.5.0"}`,
		},
		{
			name:    "all three stated true",
			json:    `{"format_version":"1.2","errored":true,"complete":true,"applyable":true}`,
			errored: boolp(true), complete: boolp(true), applyable: boolp(true),
		},
		{
			name:    "all three stated false",
			json:    `{"format_version":"1.2","errored":false,"complete":false,"applyable":false}`,
			errored: boolp(false), complete: boolp(false), applyable: boolp(false),
		},
		{
			name:    "a clean no-op plan: complete but not applyable",
			json:    `{"format_version":"1.2","errored":false,"complete":true,"applyable":false}`,
			errored: boolp(false), complete: boolp(true), applyable: boolp(false),
		},
		{
			name:     "only complete, which is what the pinned decoder models",
			json:     `{"format_version":"1.2","complete":false}`,
			complete: boolp(false),
		},
		{
			name: "null is not false either",
			json: `{"format_version":"1.2","errored":null,"complete":null,"applyable":null}`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, got, err := Read(strings.NewReader(tc.json), "test")
			if err != nil {
				t.Fatalf("Read returned error: %v", err)
			}
			check(t, "errored", got.Errored, tc.errored)
			check(t, "complete", got.Complete, tc.complete)
			check(t, "applyable", got.Applyable, tc.applyable)
		})
	}
}

// The decoding is its own pass over the bytes, and it has to be: tfjson.Plan
// has its own UnmarshalJSON, so embedding it in a wrapper struct promotes that
// method, which consumes the whole object and never looks at the siblings.
// This asserts the plan still decodes correctly alongside the flags.
func TestStatusDecodingDoesNotDisturbThePlan(t *testing.T) {
	const doc = `{
		"format_version":"1.2","terraform_version":"1.9.8",
		"errored":true,"complete":false,"applyable":false,
		"resource_changes":[{
			"address":"terraform_data.a","mode":"managed","type":"terraform_data","name":"a",
			"provider_name":"registry.terraform.io/hashicorp/terraform",
			"change":{"actions":["delete"],"before":{"input":"x"},"after":null}
		}]
	}`
	p, st, err := Read(strings.NewReader(doc), "test")
	if err != nil {
		t.Fatalf("Read returned error: %v", err)
	}
	if p.TerraformVersion != "1.9.8" {
		t.Errorf("TerraformVersion = %q", p.TerraformVersion)
	}
	if len(p.ResourceChanges) != 1 {
		t.Fatalf("got %d resource changes, want 1", len(p.ResourceChanges))
	}
	if st.Errored == nil || !*st.Errored {
		t.Error("errored did not survive alongside the plan")
	}
	if st.Applyable == nil || *st.Applyable {
		t.Error("applyable did not survive alongside the plan")
	}
}

// Any() is what every renderer asks before printing a status block at all. A
// plan that states none of the three gets no block, so a report over an older
// plan looks exactly as it did before this existed.
func TestStatusAnyIsFalseWhenNothingIsStated(t *testing.T) {
	var none Status
	if none.Any() {
		t.Error("a status with nothing stated must not be reported")
	}
	if (Status{Complete: boolp(false)}).Any() != true {
		t.Error("one stated flag is enough to report")
	}
}

func boolp(b bool) *bool { return &b }

func check(t *testing.T, name string, got, want *bool) {
	t.Helper()
	switch {
	case got == nil && want == nil:
	case got == nil:
		t.Errorf("%s = not stated, want %v", name, *want)
	case want == nil:
		t.Errorf("%s = %v, want not stated", name, *got)
	case *got != *want:
		t.Errorf("%s = %v, want %v", name, *got, *want)
	}
}

// A value that is not a boolean is NOT STATED, and must never come back as a
// confident false.
//
// Unmarshalling straight into a *bool does the worst possible thing here: Go
// allocates the pointer before it meets the type mismatch, so a plan carrying
// `"errored":"true"` left a non-nil pointer to false behind. The error was
// discarded, and a caller whose entire policy is `.status.errored == true` got
// told, explicitly, that planning succeeded - by the decoder, not by the plan.
func TestAMalformedFlagIsNotStatedRatherThanFalse(t *testing.T) {
	for _, tc := range []struct{ name, value string }{
		{"a string", `"true"`},
		{"a string that says false", `"false"`},
		{"a number", `1`},
		{"zero", `0`},
		{"an object", `{}`},
		{"an array", `[]`},
		{"an empty string", `""`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// complete is left out: the pinned decoder models it and rejects a
			// non-bool outright, so the plan never reaches here. errored and
			// applyable are the two it does not model, and the two that
			// therefore need this.
			doc := `{"format_version":"1.2","errored":` + tc.value +
				`,"applyable":` + tc.value + `}`
			_, got, err := Read(strings.NewReader(doc), "test")
			if err != nil {
				t.Fatalf("Read returned error: %v", err)
			}
			if got.Errored != nil {
				t.Errorf("errored = %v, want not stated - the plan did not state a boolean", *got.Errored)
			}
			if got.Applyable != nil {
				t.Errorf("applyable = %v, want not stated", *got.Applyable)
			}
		})
	}
}

// A flag named somewhere else in the document is not this flag.
func TestOnlyTheTopLevelFlagsAreRead(t *testing.T) {
	const doc = `{
		"format_version":"1.2",
		"output_changes":{"errored":{"actions":["create"],"after":true}},
		"variables":{"complete":{"value":false}}
	}`
	_, got, err := Read(strings.NewReader(doc), "test")
	if err != nil {
		t.Fatalf("Read returned error: %v", err)
	}
	if got.Any() {
		t.Errorf("a nested field was read as plan status: %+v", got)
	}
}

// Any() has to look at all three, not at whichever one was written first.
func TestAnyLooksAtEveryFlag(t *testing.T) {
	for _, tc := range []struct {
		name string
		st   Status
	}{
		{"only errored", Status{Errored: boolp(false)}},
		{"only complete", Status{Complete: boolp(false)}},
		{"only applyable", Status{Applyable: boolp(false)}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if !tc.st.Any() {
				t.Error("one stated flag is enough to report, whichever one it is")
			}
		})
	}
}
