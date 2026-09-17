# What Terraken builds next

In order, with the reason for the order. Every item is filtered on the three
contracts first - a file in, no attribute value out, deterministic - which is
why cost estimation, drift against a live cloud and policy engines do not
appear as options anywhere below.

The companion to this file is [`plan-file.md`](plan-file.md), which measures
how much of the plan the tool currently reads, and [`design.md`](design.md),
which records why the constraints are what they are.

## 1. Make the unknown say it is unknown

`internal/assess/assess.go` ends action classification with an unconditional
`return KindNoOp, Info`. An action, or action sequence, the tool does not
recognise therefore falls through and is reported as a harmless no-op at the
lowest severity. The loader validates `format_version` but nothing validates
the action vocabulary, so a plan from a newer Terraform carrying an action
Terraken has never seen is presented as nothing at all.

That is the one failure mode the fifth constraint exists to prevent - an
unknown reported as an unknown is the feature, an unknown quietly rendered as
no change is the bug - and the tool currently commits it.

The fix is to make the fallback say the operation is unsupported and its impact
cannot be assessed, in every format including the machine one, without
inventing a damage severity for it.

This is the smallest item on the list and the only one that repairs something
already wrong. Nothing else should ship first, and no claim about the tool's
honesty should be made out loud while it is open.

*Reverse this only if the classifier turns out to be provably total over the
action vocabulary, which the loader does not currently check.*

## 2. Turn the no-values guarantee into a proof

"No attribute value ever reaches the output" is currently four hand-written
tests and a paragraph in the README. That is a strong claim held up by a small
amount of evidence, and a reader has no way to tell it apart from any other
tool's promise.

Make the guarantee the most heavily tested thing in the repository:

1. **Leak fuzzing as a property test.** Generate plans with planted adversarial
   values - high-entropy tokens, PEM private key headers, connection strings
   with embedded passwords, cloud access key shapes - in every position a value
   can occupy: top-level attribute, nested object, array element, `body` blob,
   output value, root variable, `prior_state`, drift entry. Mark **none** of
   them sensitive. Then assert that no substring of any planted value above a
   minimum length appears in any output format, compared with whitespace
   stripped and case folded, so a wrapped or re-cased token cannot slip past a
   naive substring check.
2. **Cover positions, not values.** The generator's job is coverage of the
   *places a value can hide*, which is exactly what a handful of fixtures
   cannot give.
3. **Publish the result.** CI states the count on every run and the README
   carries it: this build ran N generated plans carrying planted credentials in
   M positions, and zero bytes of any of them reached any output format.
4. **Extend the same discipline to the second guarantee** - that nothing from
   the plan reaches the HTML style block, already asserted in `html_test.go` -
   so injection is a named, tested property rather than an implementation
   detail.

This comes before any new capability, because every capability added to the
report is another surface the guarantee has to survive, and it is much cheaper
to have the test first.

## 3. Report plan status as metadata, not as a finding

`errored`, `complete` and `applyable`, each with a true, false or unknown
state, carried prominently in the header and in the machine output but
**outside** the findings list and the severity counts.

`errored: true` must not be a critical finding. Critical is reserved for a
data-holding resource being destroyed, and widening it would make the word mean
two things; a failed planning operation is neither a resource change nor a loss
of data. `applyable: false` must not read as risk either, because Terraform's
own definition makes a clean no-op plan not applyable.

State the facts, leave the gate's meaning untouched.

Two of the three flags are not in the pinned decoder, so this needs its own
decoding rather than a dependency bump. See [`plan-file.md`](plan-file.md).

*Reverse this if a real plan from a current Terraform turns out not to emit the
flags as the specification describes.*

## 4. Review coverage

The headline capability, and the one the tool is uniquely placed to build.

State, for the plan as a whole, how much of it can actually be checked before
apply, and name every place the file is silent. Today the tool annotates
`unverifiable-until-apply` per resource and summarises plan shape separately,
and never joins the two. Five separate silences are one fact:

- attribute changes that cannot be compared because they are unknown until apply
- `resource_drift` empty because refresh was skipped, which is indistinguishable
  from nothing having drifted
- `complete: false`, so the plan is not the whole change
- `checks` entries with status `unknown`
- `deferred_changes`, work Terraform knows it postponed

Counts with a named denominator, never a single percentage. A bare "62%
reviewable" would itself be a verdict, and the third constraint forbids the
report from ruling.

## 5. Drift, with the ambiguity stated rather than resolved

Terraform already recorded what changed underneath the estate, and it is in the
file. Report it attributed as drift rather than as a planned change.

The part that matters is saying plainly when drift cannot be known, because
`-refresh=false` and "nothing drifted" produce the same empty array. A section
that renders an empty drift array as reassurance is quietly wrong, and saying
so is the same move the tool already makes for values that are unknown until
apply.

## 6. Checks, but prove the representation before building

Generate a plan from a configuration carrying both a `check` block and a
resource postcondition, and look at what actually lands in the array.

If standalone check blocks appear, this is the most valuable thing in the
layer: a failure Terraform reports as a warning and CI passes over with exit 0.
If they do not, scope it to postconditions and say so. Either way the
specification's experimental warning belongs in the documentation beside the
feature.

**Do not build this before the fixture exists.**

## 7. Sequencing and the outage window

`configuration.expressions[].references` is already parsed for blast radius.
The same graph plus the change set gives *order*: this destroys the subnet
before the replacement exists, and these six things depend on it.

An offline claim about the outage window, computed from a file, with no
credentials involved.

## 8. Interaction detection

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
