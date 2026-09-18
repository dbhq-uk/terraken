# What Terraken builds next

In order, with the reason for the order. Every item is filtered on the three
contracts first - a file in, no attribute value out, deterministic - which is
why cost estimation, drift against a live cloud and policy engines do not
appear as options anywhere below.

The companion to this file is [`plan-file.md`](plan-file.md), which measures
how much of the plan the tool currently reads, and [`design.md`](design.md),
which records why the constraints are what they are.

## 1. Make the unknown say it is unknown - done

`internal/assess/assess.go` used to end action classification with an
unconditional `return KindNoOp, Info`, so an action, or action sequence, the
tool did not recognise was reported as a harmless no-op at the lowest severity.
The loader validates `format_version` and nothing validates the action
vocabulary, so a plan from a newer Terraform carrying an unfamiliar action was
presented as nothing at all - the one failure mode the fifth constraint exists
to prevent.

The classifier now ends at `KindUnsupported` at the `Unranked` level, which is
the absence of a severity rather than a fifth one: it is counted apart from the
four, `--fail-on` cannot see it, and it still sorts above critical and survives
every `--min-level`. `--format gate` carries it in its own `unsupported` array
whether or not a gate was asked for. A team that wants it to stop a pipeline
writes a rule matching `actions: ["unsupported"]`.

The recognised vocabulary is exactly the eight shapes the pinned
`terraform-json` helpers answer yes to. Everything else - an empty array, a
repeated verb, any other pair, any sequence of three - is unrecognised, and a
data resource is exempt from the ranking rather than from being recognised.

*Revisit the strictness if a real plan from a current Terraform or OpenTofu
turns out to emit a shape not in that list; loosening it is one line, and a
false unsupported is a line in a report where a false no-op is a change nobody
looked at.*

## 2. Turn the two guarantees into proofs - done

"No attribute value ever reaches the output" was a handful of hand-written
tests and a paragraph in the README. That is a strong claim held up by a small
amount of evidence, and a reader had no way to tell it apart from any other
tool's promise.

The leak fuzzing is built. `cmd/terraken/leakproof_test.go` generates plans
carrying planted credentials in every position a value can occupy, marks none
of them sensitive, and runs each one through every output the command can
produce - every `--format`, with colour and without, filtered, gated, written
to a file, and through `--moved`. Comparison strips whitespace and folds case
over a 12-character window, so a wrapped, re-indented, re-cased or half-printed
token is still caught. It is seeded, it states its own count, and CI puts that
count in the build summary.

Two parts of it are worth keeping rather than tidying: it iterates
`render.Formats`, which is what makes a new renderer impossible to add without
covering it, and every position declares whether the tool reads it today, so a
vacuous case is visible instead of silent. `prior_state` and `resource_drift`
are the two waiting, and item 5 below will make one of them live.

**The second guarantee is built too.** Everything taken from a plan is now
untrusted input in every format, not only HTML.
`internal/render/untrusted.go` neutralises escape sequences, control characters
and the Unicode format characters that reorder or hide text, once, at the entry
to every renderer; each format still escapes for its own context on the way
out. `internal/render/injection_test.go` renders every hostile fragment and
every ordered pair of them - 812 payloads - through every format, and asserts
the structure of the output rather than the absence of a character.

Both halves of this item are named in `AGENTS.md` and in the README beside each
other, which was the other half of what the issue asked for: a reader can now
tell this tool's two guarantees apart from any other tool's promise, because
both are measured on every build.

This layer came before any new capability, because every capability added to
the report is another surface the guarantees have to survive, and it is much
cheaper to have the test first. Items 3 onwards are now clear to start.

## 3. Report plan status as metadata, not as a finding - done

`errored`, `complete` and `applyable`, each true, false or not stated, carried
in the header and in the machine output but **outside** the findings list and
the severity counts.

Two of the three are not in the pinned decoder, so `internal/plan/status.go`
reads them in its own pass over the bytes - separately rather than by embedding
`tfjson.Plan`, whose `UnmarshalJSON` would be promoted to the wrapper, consume
the whole object and never look at the siblings. All three are pointers,
because absent and false are different facts and an older plan states none of
them.

None of it is a finding, none of it is counted, and `--fail-on` cannot see any
of it. `errored: true` is not critical: critical means a resource type holds
data and destroying it loses that data, and a failed planning operation is not
a resource change. `applyable: false` is stated as a fact and never as risk,
because Terraform defines applyable as true only when the plan calls for a
meaningful change - so a clean no-op plan is not applyable, and anything
flagging it would penalise the plan that most deserves to pass.

The wording for `complete: false` is Terraform's own: another plan and apply
round is expected, and the plan does not say why. `-target` and deferred
changes both produce it and nothing in the file distinguishes them, so naming
a cause would be an inference the plan does not support. A test asserts the
report never says `-target`.

A plan stating none of the three prints no status block at all, so the change
is invisible to everybody it has nothing to tell.

## 4. Review coverage - done

The headline capability, and the one the tool is uniquely placed to build.

It states, for the plan as a whole, how much of it can actually be checked
before apply, and names every place the file is silent. Five separate silences
in five different fields were one fact nobody was saying:

- attribute changes that cannot be compared because they are unknown until apply
- output values unknown until apply, which are not resource changes and get
  their own denominator
- operations this build cannot read at all, which assessOne returns on before
  it ever looks for unknown values
- nothing recorded in `resource_drift`, which is indistinguishable from refresh
  having been skipped. Absent and empty say the same thing: Terraform writes
  the array only when something actually drifted
- `complete: false`, so the plan is not the whole change
- `checks` instances with status `unknown`
- `deferred_changes`, work Terraform knows it postponed

Counts with a named denominator, never a percentage. "62% reviewable" is a
verdict wearing a number and the third constraint forbids the report from
ruling; "1 of 2 changes carry values Terraform will not know until it applies
them" is a fact a reader can act on.

It is said either way. A plan with nothing hidden gets one line saying so,
because a reader who sees no coverage section cannot tell whether everything
was checkable or whether the tool did not look - and those are the two things
the whole feature exists to keep apart.

It counts the whole plan and survives `--min-level`, like the shape summary and
for the same reason, and it does not move the gate: not knowing something is
not a severity.

## 5. Drift, with the ambiguity stated rather than resolved - done

Terraform already recorded what changed underneath the estate during refresh
and wrote it into `resource_drift`. Reading it needs no credentials, no network
and no cloud API, which every other tool reporting drift needs all three of.

It is reported as its own list, never merged into the findings. One is what
somebody did and the other is what will happen, and the verbs are the same
words: "destroy" in the findings means Terraform will destroy it, and in drift
it means it is already gone. So drift has its own section in every format, its
own array in the machine ones, and past-tense verbs of its own.

It is ranked by the same rules - `driftOf` reuses `assessOne`, so a database
gone from underneath is critical exactly as a database being destroyed is - and
counted by none of them. It is outside the severity counts, outside `Max` and
invisible to `--fail-on`, because the counts describe changes this plan makes.
A caller that wants to act on drift branches on the gate's `drift` array; a
team RULE will not do it, because rules are evaluated against resource changes
and never run over drift.

Three things in it are easy to get wrong and are handled explicitly:

- **Not every entry is external change.** Terraform puts a no-op entry in the
  same array when an object moved to a different address in state without its
  values changing - a `moved` block, a renamed module. Reporting that as
  somebody editing infrastructure by hand is a false alarm about the one thing
  this section exists to raise real alarms about.
- **`relevant_attributes` says "may have", not "did".** It names the resource,
  and the attribute paths are dropped, so a plan reading one attribute and
  drift touching another still matches. Terraform's own specification says the
  field identifies external changes that MAY have affected the result.
- **`action_reason` and `replace_paths` describe a planning decision**, not a
  change. Nothing decided drift; somebody did it. Both are cleared.

Saying when drift cannot be known belongs to item 4 rather than here: an empty
or absent `resource_drift` means either nothing drifted or refresh never ran,
and the coverage report names that silence.

The credential detector was extended to walk `resource_drift` at the same time.
Terraform writes the before and after of everything that changed underneath, so
a credential rotated by hand was sitting in a part of the file nothing looked
at.

## 6. Checks, but prove the representation before building - done

The fixture came first, as this said it must. `testdata/real-checks.json` and
`testdata/real-checks-undetermined.json` are genuine `terraform show -json`
output from throwaway local roots - `terraform_data` and the `local` provider,
no cloud, no credential - carrying a check block that fails at plan time, a
check block that cannot be determined until apply, and resource postconditions
expanded over `for_each`.

**Standalone check blocks do appear**, which was the open question, so this is
the full-scope version rather than the postcondition-only fallback. What the
real output established:

- A check block appears with `kind: "check"`, a resource condition with
  `kind: "resource"`, so the two are distinguishable. Terraform aggregates a
  resource's PRECONDITIONS AND POSTCONDITIONS under one object and the JSON
  does not say which failed, so the report calls it a resource condition rather
  than naming one of them.
- A check block **can fail at plan time**. Terraform reports that as a
  *Warning* and the plan still succeeds, so a failure somebody wrote down as
  mattering goes past without anything downstream having to notice. That is the
  whole reason to report it. The report says Terraform treats it as a warning
  and stops there: claiming the pipeline "sees exit 0" would be two claims the
  plan does not support, because another error may have failed the plan anyway
  and `-detailed-exitcode` returns 2 for a successful plan with changes.
- An expanded postcondition gives one instance per object, addressed
  `terraform_data.app["production"]`. An unexpanded check block gives one
  instance whose address equals its parent's.
- A condition is `unknown` when the values it needs are unknown, which is NOT
  the same as the resource being created. The first draft of this note said a
  postcondition on a new resource is always unknown, and the committed fixture
  disproves it: both `terraform_data.app` instances are being created and their
  conditions pass. The second fixture has the genuine case - a condition
  reading an attribute that is itself unknown until apply.

**The message is never reported.** `error_message` is written by whoever wrote
the configuration and Terraform interpolates it: a plan generated to test this
carried a live GitHub token in a check's failure message. Printing it would put
an attribute value in the output, so the report names the check, its kind, its
status and how many problems it had, and the reader takes the message from the
plan output where they already have it. `CheckFinding` has nowhere to put one,
and a test asserts the type has no message field.

Passing checks are not reported. Failed and undetermined ones are, and the
undetermined ones also feed the coverage report from item 4.

The specification's experimental warning belongs beside the feature and is in
the README.

## 7. Which way round a replacement happens - done

A precondition for item 8 rather than a line of its own, and it came first for
the reason [#43](https://github.com/dbhq-uk/terraken/issues/43) gives: anything
built on sequencing has to start from the order the plan actually states, and
terraken was describing both orders with one sentence that named only one of
them.

The plan distinguishes the two replacements in the order of the `actions`
array - `["delete", "create"]` against `["create", "delete"]` - and terraken
collapsed both into `KindReplace` and threw the order away. Findings now carry
`ReplaceOrder`, the action line names the sequence rather than stating the
wrong one, and the gate carries it under `replace_order`. It is added inside
`terraken.gate/v1`, which the compatibility policy allows: a field is added,
none is removed or repurposed.

Three decisions are worth keeping:

- **The kind stays whole and the level does not move.**
  `create_before_destroy` changes the order; it does not stop the old object
  being destroyed, and on a resource that holds data the data still goes.
  Splitting `KindReplace` in two would change a vocabulary every rule file and
  consumer already matches on, to carry a fact that fits beside it.
- **The sentence names the sequence and stops.** The first draft said a
  create-first replacement leaves "no point during the apply at which this
  resource does not exist", and that is false rather than cautious: a
  `local_file` with a fixed filename planned that way ends the apply with the
  file deleted, because the old object's destroy removes the path the new one
  just wrote. It was run. Whether the thing two operations act on survives is a
  question about the provider, and the plan does not answer it.
- **The order is reported and the cause never is.** "`create_before_destroy` is
  set" is the obvious sentence and it is a claim the plan does not support. The
  `lifecycle` block is not in plan JSON at any point, and the rule propagates
  down the dependency chain - `testdata/replace-create-first-propagated.json`
  is a real plan in which the resource that does *not* set the rule is planned
  this way because the one depending on it does.
- **Drift never carries it.** It is a decision about an apply, and nothing in
  the drift list is about to be applied, so it is dropped there beside
  `action_reason` and `replace_paths` for the same reason.

The two fixtures are the same root planned twice by real Terraform 1.16.1,
differing only in a `lifecycle` block, and the files that come out differ only
in the order of one array. That is the evidence for the paragraph above and it
is committed.

## 8. Sequencing and the outage window

`configuration.expressions[].references` is already parsed for blast radius.
The same graph plus the change set gives *order*: this destroys the subnet
before the replacement exists, and these six things depend on it.

An offline claim about the outage window, computed from a file, with no
credentials involved.

Item 7 is what this stands on. The graph gives reach, the change set gives what
happens, and the ordering gives which of a replacement's two steps comes first.

**It will need item 7's discipline more than item 7 did.** The word "window"
is the whole appeal of this item and it is also the trap: a `local_file`
replacement planned create-first ends the apply with the file deleted, because
the old object's destroy removes the path the new one just wrote. The plan
states an order. It does not state what a provider does with two objects that
collide, and a claim about reachability or duration is not a claim a file
supports. Whatever this reports has to survive that counterexample.

## 9. Interaction detection

Findings that are individually survivable and jointly catastrophic.

Last, because it needs the earlier items to have something to interact with.

## Release provenance

Separate from the capability order, and it blocks positioning rather than
capability: the release carries a `checksums.txt` and nothing else - no
signature, no SBOM, no build provenance. A claim about trust should not be made
while the binary carrying it cannot be traced back to the source that built it.
Tracked as [#39](https://github.com/dbhq-uk/terraken/issues/39).

## Deferred

**SARIF output.** The obvious-looking integration, and it is deferred rather
than ruled out. GitHub requires at least one location for code scanning to
display a result, and a byte offset genuinely measured in the input
`plan.json` is a real location rather than a fabricated one - so the objection
is usefulness, not honesty. A pull request annotation is only useful against a
changed source line, and the plan JSON is not one. GitHub also only accepts
SARIF uploads to private or internal repositories with Code Security enabled,
which is where most Terraform estates live.

Build it when a named consumer asks for it, and never embed a plan snippet in
it.

## Deliberately not doing

- **A baseline or suppression file.** Established gates generally ship one - a
  written baseline of existing findings, an ignore list with per-entry expiry,
  comment annotations in source, a since-this-revision filter. All of them
  baseline against a codebase, which persists between runs. A plan does not: a
  finding lives until the change is
  applied and then disappears by itself, so the accumulated noise a baseline
  exists to suppress largely does not accumulate. The rules file is already the
  better mechanism, because it lowers a level per rule, visibly and auditably,
  rather than hiding a class of finding wholesale. If exceptions are ever
  needed, extend the rules file; do not add a second configuration mechanism
  beside it.
- **Per-resource-type semantic depth for one cloud.** Teaching the tool what a
  particular provider's resource means is rendering work that must be
  maintained forever, one resource type at a time, and it also means printing
  values - which forfeits the only guarantee the tool has. Every finding stays
  about the shape of a change.
- **Anything that needs a credential, a network call or a model.** Ends the
  contract. See [`design.md`](design.md).

## How to choose what comes after this list

Two tests, applied in order.

**Does it reuse understanding the tool already has, or acquire a new
responsibility?** Proposing a `moved` block reuses what the rename heuristic
already worked out. Cloud discovery, pricing and orchestration each acquire a
provider, a credential and a version matrix that has to be kept current
forever.

**Does it close a case where the tool is currently silent about, or wrong
about, something already in the file?** That is what puts items 1 and 3 ahead
of everything else, and it is a better reason than novelty.
