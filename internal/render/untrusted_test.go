package render

import (
	"reflect"
	"strings"
	"testing"

	"github.com/dbhq-uk/terraken/internal/assess"
)

// A field added to Report later must not skip sanitising by being forgotten.
//
// This walks the type rather than the value, so it fails when somebody ADDS a
// string-bearing field, not when a particular report happens to carry one. The
// list below is what has been looked at and dealt with; anything outside it is
// new, and the failure says so.
//
// Report.Status is the case that prompted this. It is three booleans and there
// is genuinely nothing to escape, but "considered and safe" and "forgotten"
// look identical in a diff, and only one of them stays true when a string field
// is added to it later.
func TestEveryStringOnAReportIsSanitised(t *testing.T) {
	handled := map[string]bool{
		// Sanitised directly.
		"TerraformVersion": true, "FormatVersion": true, "HiddenBelow": true,
		"Findings": true, "CountsByName": true, "Shape": true, "Exposure": true,
		"Coverage": true, "Drift": true, "Checks": true,
		// No strings in it, and no judgement either: three booleans read off
		// the plan, printed with words this package owns.
		"Status": true,
		// Not strings.
		"Counts": true, "Hidden": true, "Unassessed": true,
	}

	rt := reflect.TypeOf(assess.Report{})
	for i := 0; i < rt.NumField(); i++ {
		f := rt.Field(i)
		if !handled[f.Name] {
			t.Errorf("Report.%s (%s) is not in the sanitising register in untrusted.go. "+
				"If it can carry a string that came from a plan, sanitise it; if it cannot, "+
				"say so there and add it here.", f.Name, f.Type)
		}
	}

	// And the claim about Status specifically: no string fields on it.
	st := reflect.TypeOf(assess.Report{}.Status)
	for i := 0; i < st.NumField(); i++ {
		f := st.Field(i)
		if strings.Contains(f.Type.String(), "string") {
			t.Errorf("Status.%s is a string now, so it has to be sanitised like everything "+
				"else taken from a plan", f.Name)
		}
	}
}

// TestEveryStringOnAFindingIsSanitised is the same register one level down,
// and it exists because the Report-level one above did not catch a new field.
//
// ReplaceOrder was added to Finding, is rendered, and went through no
// sanitising at all - with the whole suite green, because nothing checked the
// fields of a Finding. A Report gains a field rarely; a Finding gains one every
// time the tool learns to say something new, which makes this the register more
// likely to be needed and the one that was missing.
//
// ONE REGISTER PER TYPE, not one shared map. A shared map would let a field
// name handled on Finding excuse an unhandled field of the same name on
// Annotation, which is how a register stops being one.
func TestEveryStringOnAFindingIsSanitised(t *testing.T) {
	registers := []struct {
		of      reflect.Type
		handled map[string]bool
	}{
		{reflect.TypeOf(assess.Finding{}), map[string]bool{
			// Sanitised directly in sanitiseFinding.
			"Address": true, "Type": true, "Module": true, "Provider": true,
			"LevelName": true, "Kind": true, "Reason": true, "ReplacePaths": true,
			"ReplaceOrder": true, "Annotations": true,
			// Not strings.
			"Level": true, "DataLoss": true,
		}},
		{reflect.TypeOf(assess.Annotation{}), map[string]bool{
			// Sanitised directly in sanitiseAnnotation.
			"Code": true, "Detail": true, "Summary": true, "Note": true,
			"Paths": true, "Moved": true, "Reached": true,
		}},
		{reflect.TypeOf(assess.MovedEvidence{}), map[string]bool{
			"From": true, "To": true, "FromModule": true, "ToModule": true,
			"Rivals": true,
			// Not strings.
			"Matched": true, "Compared": true, "CrossModule": true,
		}},
		{reflect.TypeOf(assess.Reached{}), map[string]bool{
			"Address": true,
			// Not a string.
			"Depth": true,
		}},
	}
	for _, r := range registers {
		for i := 0; i < r.of.NumField(); i++ {
			f := r.of.Field(i)
			if !r.handled[f.Name] {
				t.Errorf("%s.%s (%s) is not in the sanitising register in untrusted.go. "+
					"If it can carry a string that came from a plan, sanitise it; if it "+
					"cannot, say so there and add it here.", r.of.Name(), f.Name, f.Type)
			}
		}
	}
}
