package assess

import "sort"

// What each code in a report means, in the tool's own words.
//
// The report names annotations by code, and each has a careful explanation
// somewhere - what it means, what it does NOT mean, and where the heuristic
// stops. A reader looking at a terminal had to leave it and find the right
// section of the README.
//
// ONE SOURCE, AND A CODE WITHOUT TEXT FAILS THE BUILD.
// TestEveryCodeIsExplained reads the constants out of finding.go and level.go
// and checks each one is here, so a code added later arrives with its
// explanation or not at all.
//
// NOTHING HERE READS A PLAN. These are the tool's own sentences, looked up by
// name. `--explain` opens no file, so it is the one thing the command does that
// cannot leak anything, and it says so by construction rather than by testing.
//
// WHAT EACH ENTRY OWES THE READER. The first sentence says what the finding is.
// The rest says where it stops - what it does not claim, and what a reader
// should not conclude from it. That second half is the reason this exists: a
// code a reader half-remembers is more dangerous than one they look up.

// explanations is every code the report can print, and what it means.
var explanations = map[string]string{
	// THE BUILT-IN RANKING. A team rule can set a finding's level to anything
	// it likes - that is what rules are for - so these describe what terraken
	// assigns and not what a level always means in a report somebody has
	// configured. The your-rule annotation is what tells the two apart.
	Info.String(): "Terraken's own ranking for a change that destroys nothing: a create, a data source read, or a change " +
		"this build has nothing to say about. Info is the floor, not a judgement that the " +
		"change is correct.",
	Low.String(): "Terraken's own ranking for an update in place, or a resource being " +
		"forgotten from state and left running. Nothing is destroyed. An update can still be " +
		"the change that breaks something: low is about what the operation can DESTROY, not " +
		"about how much it matters. One of your own rules can set any level it likes, and a " +
		"finding it set carries the your-rule annotation.",
	High.String(): "Terraken's own ranking for a destroy, or a replacement. The existing object goes, whatever is or is " +
		"not inside it. Every replacement is here whichever way round it happens, because " +
		"create_before_destroy changes the order and not the destruction.",
	Critical.String(): "A destroy or replacement of a resource type that HOLDS DATA, so " +
		"destroying it loses that data. That is the only escalation terraken makes on its own, " +
		"and it means that one thing - though one of your own rules can set this level for any " +
		"reason it likes, and a finding it set carries the your-rule annotation. " +
		"The data-holding list is curated and incomplete, and recognition works on the " +
		"PROVIDER PREFIX: an unfamiliar type from a provider terraken knows is ranked high " +
		"with no caveat, and only a type from a provider it does not know carries the " +
		"unrecognised-provider annotation.",
	Unranked.String(): "The absence of a severity, not a fifth one. The tool could not read " +
		"the operation, so it has nothing to rank - saying info would be a guess dressed as a " +
		"measurement. It sorts above critical, no --min-level hides it, and --fail-on cannot " +
		"see it, because --fail-on takes a severity and this has none.",

	// THE COVERAGE GAPS. A report prints these beside the findings, under "how
	// much of this could be checked", and they are exactly the codes a reader
	// meets when the tool is telling them what it could NOT do - so a reader
	// looking one up is the reader most in need of an answer.
	GapUnknownUntilApply: "Some changed attributes hold values Terraform will not know until it " +
		"applies, so no claim about those values can be checked now. It is a count of changes " +
		"affected, not of attributes, and it does not say the change is wrong - it says this " +
		"part of it cannot be reviewed in advance.",
	GapUnreadableOperation: "Some operations in this plan are ones this build cannot read at " +
		"all, so nothing about them was assessed. This is the gap that makes every other number " +
		"in the report a partial answer, which is why it is named rather than folded into a " +
		"percentage.",
	GapDriftNotKnown: "This plan records nothing that changed underneath the estate, and that " +
		"is TWO facts it does not distinguish: either nothing drifted, or refresh never ran and " +
		"nobody looked. Terraform writes the drift array only when there is drift, so an absent " +
		"one is silence rather than reassurance.",
	GapIncompletePlan: "The plan says it is not complete, which is Terraform's own word for " +
		"expecting another plan and apply round. It does not say why, and nothing in the file " +
		"does - -target and deferred changes both produce it, so naming a cause would be an " +
		"inference the plan does not support.",
	GapUnknownOutputs: "Some output values are not known until apply. They are not resource " +
		"changes, so they have their own count rather than being folded into the changes - an " +
		"output nobody can predict is a different kind of unknown from an attribute.",
	GapChecksUndetermined: "Some checkable objects could not be determined before apply, so " +
		"whether the conditions somebody wrote down hold is not known yet. A failed check is " +
		"reported separately; this is the ones with no answer either way.",
	GapDeferred: "The plan carries deferred changes: work Terraform knows about and has not " +
		"planned in detail. Their contents are not read here, so the count says how much is " +
		"waiting rather than what it will do.",

	AnnMissedMoved: "A destroy and a create that look like one resource renamed without a " +
		"moved block, which is how a refactor destroys something it meant to keep. It is a " +
		"HEURISTIC over attribute similarity: it can pair the wrong two resources, and it " +
		"refuses to propose anything when two creates match a delete equally well. Verify the " +
		"pairing before using the block it suggests.",
	AnnUnverifiable: "Attributes Terraform will not know until it applies, named by path. No " +
		"claim about those values can be checked in review. This is the finding, not a gap in " +
		"the output - an unknown reported as an unknown is the point.",
	AnnSensitive: "Attributes Terraform marked sensitive, named by path. The values are not " +
		"printed, and neither is anything else: marking is what Terraform did, and terraken " +
		"prints no value whether or not it was marked.",
	AnnUnknownVendor: "A resource being destroyed whose provider is not on terraken's curated " +
		"list, so whether destroying it loses data has NOT been assessed. An unrecognised type " +
		"is never assumed safe, and this is the tool saying so rather than staying quiet.",
	AnnUnsupportedAction: "An operation this build cannot read at all - an action verb it has " +
		"never seen, or a sequence Terraform does not document. Nothing below it was assessed, " +
		"because nothing is known about what it does. It carries the plan's whole action " +
		"sequence, quoted.",
	AnnBlastRadius: "What else in this plan depends on a resource being destroyed or replaced, " +
		"transitively, with the nearest distance to each. It is what the CONFIGURATION " +
		"declares, counted within this plan, so it is a floor rather than a measurement: a " +
		"dependency through a local or a data source is not in it, nor is one to a resource " +
		"outside this plan.",
	AnnReplaceOrder: "Which way round a replacement happens: whether the plan destroys the " +
		"existing object before creating its replacement, or the other way about. It states " +
		"the order and nothing following from it - create-before-destroy is not a promise the " +
		"resource is there throughout, because what a provider does with two objects that " +
		"collide is not in the plan.",
	AnnDestroyedBefore: "Which resources depending on this one are destroyed before it is. " +
		"Order only, from Terraform's dependency rules: no window, no duration, and steps with " +
		"no dependency between them are not ordered against each other at all.",
	AnnChangedAfter: "Which resources depending on this one are created or updated after it is " +
		"created. Reported only where this resource is itself created, because a plan that " +
		"only destroys something has no create for anything to follow.",
	AnnDrift: "Something about this resource differs from the state Terraform last recorded, " +
		"and the difference is ALREADY THERE - it is not something this plan proposes to do. " +
		"Terraform found it while refreshing and wrote it into the file. It is ranked like a " +
		"planned change and counted like none of them, because the counts describe what this " +
		"plan does.",
	AnnDriftRelevant: "This plan reads values from a resource that changed underneath, so what " +
		"changed MAY have affected what the plan decided to do. Not that it did: " +
		"relevant_attributes names the resource and the attribute paths are dropped, so the " +
		"plan reading one attribute and the drift touching another still matches here.",
	AnnDriftMoved: "Terraform recorded this while refreshing and it is not external change at " +
		"all: either the object moved to a different address in state, which is a moved block " +
		"or a renamed module doing what it was asked, or its values did not differ. Reporting " +
		"it as somebody editing infrastructure by hand would be a false alarm about the one " +
		"thing the drift list exists to raise real alarms about.",
	AnnRule: "A finding from one of YOUR rules rather than from the tool's judgement, carrying " +
		"your message and your severity. Its own code so a consumer can tell the two apart " +
		"without reading prose.",
	AnnReordered: "A changed list holds the same elements in a different order. A fact, not a " +
		"verdict: order is significant for a container command or an ordered rule list, so " +
		"whether it matters is yours to judge. It never changes a finding's level.",
	AnnSameWhitespace: "Both sides are the same text laid out differently - a trailing " +
		"newline, an indent, a CRLF against an LF. Whitespace is significant in a script and " +
		"in anything hashed, so this names what was seen and stops.",
	AnnSameNumber: "Both sides are the same number written another way, such as 80 against the " +
		"string form of 80. Compared exactly rather than as a float, so two integers that " +
		"differ beyond what a float64 can hold are NOT called the same number.",
	AnnNullAndEmpty: "One side is null and the other is an empty list, object or string. This " +
		"is a fact and NOT a claim the two are interchangeable: to Terraform they differ in " +
		"some positions, where null can mean inherit a default and empty means explicitly " +
		"none. It never changes a finding's level.",
	AnnSameJSON: "Both sides parse as JSON and hold the same data - a policy document, or " +
		"anything a provider round-trips as a JSON blob. The keys MAY have moved and need not " +
		"have: re-indenting or pretty-printing with the order untouched lands here too, because " +
		"what is compared is the data rather than the text. It does NOT say the change is " +
		"harmless: a consumer comparing the string byte for byte still sees a change, and the " +
		"numbers inside are compared exactly rather than as floats.",
	AnnAllRewritten: "The roll-up: every attribute the plan shows as changed on this resource " +
		"fell into one of the written-differently classes. It is strict - an attribute the tool " +
		"cannot account for silences it - but it is NOT a verdict that the change is harmless. " +
		"Each of those classes has a case where the difference is real, and a reordered list is " +
		"one of them: a container's command means something different in another order.",
	AnnChangedAttributes: "Which top-level attributes this change touches, and how many. A " +
		"create sets them, a delete had them, an update or a replacement changes them. Names " +
		"only, never values. A change inside one attribute is counted once, so the number says " +
		"what is touched and not how big the change is.",
}

// Explain returns what a code means, and false when nothing knows it.
func Explain(code string) (string, bool) {
	text, ok := explanations[code]
	return text, ok
}

// ExplainableCodes is every code Explain answers for, sorted, so the command
// can list them and a test can walk them.
func ExplainableCodes() []string {
	out := make([]string, 0, len(explanations))
	for code := range explanations {
		out = append(out, code)
	}
	sort.Strings(out)
	return out
}
