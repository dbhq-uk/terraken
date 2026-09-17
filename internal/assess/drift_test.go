package assess

import (
	"strings"
	"testing"

	tfjson "github.com/hashicorp/terraform-json"
)

// What changed underneath the estate, as opposed to what this change plans to
// do. Terraform computed it during refresh and wrote it into the file; reading
// it needs no credentials, no network and no cloud API, which every other tool
// reporting drift needs all three of.

func reportWithDrift(t *testing.T, entries ...*tfjson.ResourceChange) Report {
	t.Helper()
	p := planOf(change("azurerm_virtual_network.planned", "azurerm_virtual_network", tfjson.ActionUpdate))
	p.ResourceDrift = entries
	return Assess(p)
}

// THE TWO LISTS NEVER MIX. One is what somebody did, the other is what will
// happen, and a reader who cannot tell them apart has been told neither.
func TestDriftIsNeverMixedWithPlannedChanges(t *testing.T) {
	r := reportWithDrift(t, change("azurerm_mssql_database.db", "azurerm_mssql_database", tfjson.ActionUpdate))

	if len(r.Findings) != 1 {
		t.Fatalf("got %d findings, want 1 - the drift entry must not be one", len(r.Findings))
	}
	if r.Findings[0].Address != "azurerm_virtual_network.planned" {
		t.Errorf("findings[0] = %q, want the planned change", r.Findings[0].Address)
	}
	if len(r.Drift) != 1 {
		t.Fatalf("got %d drift entries, want 1", len(r.Drift))
	}
	if r.Drift[0].Address != "azurerm_mssql_database.db" {
		t.Errorf("drift[0] = %q", r.Drift[0].Address)
	}
}

// Ranked on the same terms as everything else: drift on a resource that holds
// data is a different event from drift on a tag.
func TestDriftIsRankedOnTheSameTermsAsAChange(t *testing.T) {
	// GIVEN IN THE WRONG ORDER ON PURPOSE. Handing them over already sorted
	// meant removing the sort entirely still passed.
	r := reportWithDrift(t,
		change("azurerm_virtual_network.net", "azurerm_virtual_network", tfjson.ActionUpdate),
		change("azurerm_mssql_database.db", "azurerm_mssql_database", tfjson.ActionDelete),
	)
	if len(r.Drift) != 2 {
		t.Fatalf("got %d drift entries, want 2", len(r.Drift))
	}
	// Most severe first, like the findings.
	if r.Drift[0].Address != "azurerm_mssql_database.db" {
		t.Errorf("drift is not sorted most severe first: %+v", r.Drift)
	}
	if r.Drift[0].Level != Critical || !r.Drift[0].DataLoss {
		t.Errorf("a database gone from underneath is critical and a data loss, got %v/%v",
			r.Drift[0].Level, r.Drift[0].DataLoss)
	}
	if r.Drift[1].Level != Low {
		t.Errorf("an updated network is low, got %v", r.Drift[1].Level)
	}
}

// DRIFT IS NOT IN THE SEVERITY COUNTS AND --fail-on CANNOT SEE IT. It is not a
// change this plan makes, and `critical` in the counts means one resource
// change losing data.
//
// A team that wants to stop on drift branches on the gate's `drift` array.
// NOT a rule: rules are evaluated against resource changes and never run over
// drift, so saying "write a rule" here would be telling somebody to do
// something that silently does nothing.
func TestDriftStaysOutOfTheCountsAndTheVerdict(t *testing.T) {
	r := reportWithDrift(t, change("azurerm_mssql_database.db", "azurerm_mssql_database", tfjson.ActionDelete))

	if r.CountsByName["critical"] != 0 {
		t.Errorf("counts = %v; drift must not be counted as a planned change", r.CountsByName)
	}
	max, any := r.Max()
	if !any || max != Low {
		t.Errorf("Max() = %v, %v; want Low - only the planned update counts", max, any)
	}
}

// A display filter changes what a reader sees of the FINDINGS. Drift is not a
// finding, and hiding it because somebody turned the volume down would be the
// same mistake as hiding the exposure.
func TestAFilterDoesNotHideDrift(t *testing.T) {
	r := reportWithDrift(t, change("azurerm_virtual_network.net", "azurerm_virtual_network", tfjson.ActionUpdate))
	if len(r.Drift) == 0 {
		t.Fatal("there is no drift to hide, so this proves nothing")
	}
	filtered := r.AtLeast(Critical)
	if len(filtered.Drift) != len(r.Drift) {
		t.Errorf("a filter hid drift: %d then %d", len(r.Drift), len(filtered.Drift))
	}
}

// relevant_attributes lists the sources of all values contributing to changes
// in the plan, so it says which drift actually affected this plan's result.
// The rest is real drift that this change simply does not depend on, and
// saying so is more useful than a flat list.
func TestDriftSaysWhichEntriesAffectedThisPlan(t *testing.T) {
	p := planOf(change("azurerm_virtual_network.planned", "azurerm_virtual_network", tfjson.ActionUpdate))
	p.ResourceDrift = []*tfjson.ResourceChange{
		change("azurerm_mssql_database.db", "azurerm_mssql_database", tfjson.ActionUpdate),
		change("azurerm_virtual_network.unrelated", "azurerm_virtual_network", tfjson.ActionUpdate),
	}
	p.RelevantAttributes = []tfjson.ResourceAttribute{
		{Resource: "azurerm_mssql_database.db"},
	}

	r := Assess(p)
	byAddress := map[string]Finding{}
	for _, d := range r.Drift {
		byAddress[d.Address] = d
	}
	if !hasAnnotation(byAddress["azurerm_mssql_database.db"], AnnDriftRelevant) {
		t.Error("drift this plan depends on must say so")
	}
	if hasAnnotation(byAddress["azurerm_virtual_network.unrelated"], AnnDriftRelevant) {
		t.Error("drift nothing in the plan reads must not be marked as affecting it")
	}
}

// With no relevant_attributes at all, nothing is marked either way. The field
// is optional, and an absent one says nothing about what contributed - guessing
// from its absence would be the same error as reading an absent drift array as
// reassurance.
func TestNoRelevantAttributesMeansNoClaimEitherWay(t *testing.T) {
	r := reportWithDrift(t, change("azurerm_mssql_database.db", "azurerm_mssql_database", tfjson.ActionUpdate))
	if hasAnnotation(r.Drift[0], AnnDriftRelevant) {
		t.Error("with no relevant_attributes, no claim about relevance can be made")
	}
}

// Drift has its own sentence, and it says what happened rather than what will
// happen. "destroy" on a planned change means terraform will destroy it;
// "destroy" in drift means it is already gone.
func TestDriftCarriesItsOwnExplanation(t *testing.T) {
	r := reportWithDrift(t, change("azurerm_mssql_database.db", "azurerm_mssql_database", tfjson.ActionDelete))
	a, ok := annotationOf(r.Drift[0], AnnDrift)
	if !ok {
		t.Fatalf("no drift annotation: %+v", r.Drift[0].Annotations)
	}
	for _, want := range []string{"already", "does not propose"} {
		if !strings.Contains(a.Detail, want) {
			t.Errorf("Detail = %q, want it to mention %q", a.Detail, want)
		}
	}
	// AGAINST THE PRIOR SAVED STATE, NOT SINCE THE LAST APPLY. The plan
	// records a comparison, not a timeline: no apply timestamp, no attribution
	// of who caused the difference. Saying "since the last apply" puts both in
	// the reader's head from a file that states neither.
	if strings.Contains(a.Detail, "since the last apply") {
		t.Errorf("Detail claims a timeline the plan does not state: %q", a.Detail)
	}
	if !strings.Contains(a.Detail, "state Terraform last recorded") {
		t.Errorf("Detail = %q, want it to name what the comparison is against", a.Detail)
	}
}

// NOT EVERY ENTRY IN resource_drift IS EXTERNAL CHANGE. Terraform puts a no-op
// entry there when an object moved to a different address in state without its
// values changing - a moved block, a renamed module, doing exactly what they
// were asked. Reporting that as somebody editing infrastructure by hand is a
// false alarm about the one thing this section exists to raise real alarms
// about, and a past-tense verb does not fix the meaning of the event.
func TestAnAddressMoveIsNotReportedAsExternalChange(t *testing.T) {
	moved := change("terraform_data.new_name", "terraform_data", tfjson.ActionNoop)
	moved.PreviousAddress = "terraform_data.old_name"

	r := reportWithDrift(t, moved)
	if len(r.Drift) != 1 {
		t.Fatalf("got %d drift entries, want 1", len(r.Drift))
	}
	if hasAnnotation(r.Drift[0], AnnDrift) {
		t.Error("an address move must not be reported as changed outside Terraform")
	}
	a, ok := annotationOf(r.Drift[0], AnnDriftMoved)
	if !ok {
		t.Fatalf("no moved-in-state annotation: %+v", r.Drift[0].Annotations)
	}
	if !strings.Contains(a.Detail, "terraform_data.old_name") {
		t.Errorf("Detail = %q, want it to name where the object moved from", a.Detail)
	}
	if !strings.Contains(a.Detail, "bookkeeping") {
		t.Errorf("Detail = %q, want it to say this is Terraform's own doing", a.Detail)
	}
}

// And a no-op with no previous address is Terraform recording that it looked
// and found nothing - also not external change.
func TestANoOpDriftEntryIsNotReportedAsExternalChange(t *testing.T) {
	r := reportWithDrift(t, change("terraform_data.same", "terraform_data", tfjson.ActionNoop))
	if hasAnnotation(r.Drift[0], AnnDrift) {
		t.Error("a no-op entry must not be reported as changed outside Terraform")
	}
	if !hasAnnotation(r.Drift[0], AnnDriftMoved) {
		t.Errorf("expected the unchanged annotation: %+v", r.Drift[0].Annotations)
	}
}

// relevant_attributes says which external changes MAY have affected the plan.
// It names the resource, and this drops its attribute paths - so the plan
// reading one attribute and the drift touching another still matches. Claiming
// more than "may have" would invent a causal link the pairing cannot support.
func TestTheRelevanceClaimIsNotCausal(t *testing.T) {
	p := planOf(change("azurerm_virtual_network.planned", "azurerm_virtual_network", tfjson.ActionUpdate))
	p.ResourceDrift = []*tfjson.ResourceChange{
		change("azurerm_mssql_database.db", "azurerm_mssql_database", tfjson.ActionUpdate),
	}
	p.RelevantAttributes = []tfjson.ResourceAttribute{{Resource: "azurerm_mssql_database.db"}}

	a, ok := annotationOf(Assess(p).Drift[0], AnnDriftRelevant)
	if !ok {
		t.Fatal("no relevance annotation")
	}
	if !strings.Contains(a.Detail, "may have affected") {
		t.Errorf("Detail = %q, want it to say the plan MAY have been affected", a.Detail)
	}
	for _, overclaim := range []string{"has already fed", "depends on", "changes above"} {
		if strings.Contains(a.Detail, overclaim) {
			t.Errorf("Detail claims more than the pairing supports (%q): %s", overclaim, a.Detail)
		}
	}
}

func hasAnnotation(f Finding, code string) bool {
	_, ok := annotationOf(f, code)
	return ok
}

func annotationOf(f Finding, code string) (Annotation, bool) {
	for _, a := range f.Annotations {
		if a.Code == code {
			return a, true
		}
	}
	return Annotation{}, false
}

// assessOne was written for a planned change and carries two things that are
// the planning DECISION rather than the change itself. Nothing decided drift.
// Somebody did it.
func TestDriftDropsThePlanningDecisionFields(t *testing.T) {
	rc := change("azurerm_mssql_database.db", "azurerm_mssql_database",
		tfjson.ActionDelete, tfjson.ActionCreate)
	rc.ActionReason = tfjson.ActionReasonReplaceByRequest
	rc.Change.ReplacePaths = []interface{}{[]interface{}{"zone"}}

	r := reportWithDrift(t, rc)
	d := r.Drift[0]

	if d.Reason != "" {
		t.Errorf("Reason = %q; an action_reason explains a decision Terraform made, "+
			"and nothing decided this", d.Reason)
	}
	if len(d.ReplacePaths) != 0 {
		t.Errorf("ReplacePaths = %v; nothing forced a replacement that was never planned",
			d.ReplacePaths)
	}
	// And the planned-change equivalent still carries both, so this is a
	// difference between the two lists rather than a feature being removed.
	p := planOf(rc)
	if f := Assess(p).Findings[0]; f.Reason == "" || len(f.ReplacePaths) == 0 {
		t.Errorf("a planned change must still carry its reason and replace paths: %+v", f)
	}
}

// The unrecognised-provider caveat is right but was worded for a plan.
func TestDriftRewordsTheUnrecognisedProviderCaveat(t *testing.T) {
	r := reportWithDrift(t, change("terraform_data.gone", "terraform_data", tfjson.ActionDelete))
	a, ok := annotationOf(r.Drift[0], AnnUnknownVendor)
	if !ok {
		t.Fatalf("no unrecognised-provider annotation: %+v", r.Drift[0].Annotations)
	}
	if strings.Contains(a.Detail, "destroying this") {
		t.Errorf("Detail reads as a proposal, but the resource is already gone: %q", a.Detail)
	}
	if !strings.Contains(a.Detail, "losing this") {
		t.Errorf("Detail = %q, want the past tense", a.Detail)
	}
}
