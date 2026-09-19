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
	Info.String(): "Nothing here destroys anything: a create, a data source read, or a change " +
		"this build has nothing to say about. Info is the floor, not a judgement that the " +
		"change is correct.",
	Low.String(): "An update in place, or a resource being forgotten from state and left " +
		"running. Nothing is destroyed. An update can still be the change that breaks " +
		"something - low is about what the operation can DESTROY, not about how much it matters.",
	High.String(): "A destroy, or a replacement. The existing object goes, whatever is or is " +
		"not inside it. Every replacement is here whichever way round it happens, because " +
		"create_before_destroy changes the order and not the destruction.",
	Critical.String(): "A destroy or replacement of a resource type that HOLDS DATA, so " +
		"destroying it loses that data. This is the only escalation in the tool and it means " +
		"one thing. The type list is curated and incomplete: a type terraken does not " +
		"recognise is reported as unassessed rather than assumed safe.",
	Unranked.String(): "The absence of a severity, not a fifth one. The tool could not read " +
		"the operation, so it has nothing to rank - saying info would be a guess dressed as a " +
		"measurement. It sorts above critical, no --min-level hides it, and --fail-on cannot " +
		"see it, because --fail-on takes a severity and this has none.",

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
	AnnSameJSON: "Both sides parse as JSON and hold the same data with the keys in a different " +
		"order - a policy document, or anything a provider round-trips as a JSON blob. It does " +
		"NOT say the change is harmless: a consumer comparing the string byte for byte still " +
		"sees a change, and the numbers inside are compared exactly rather than as floats.",
	AnnAllRewritten: "The roll-up: every attribute the plan shows as changed on this resource " +
		"fell into one of the written-differently classes. It is a strict claim, so one real " +
		"change anywhere on the resource silences it. It is the most useful thing the tool can " +
		"say about an update in place that is really nothing.",
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
