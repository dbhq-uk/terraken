# Design notes

Why Terraken is shaped the way it is. `AGENTS.md` states the constraints that
must not be broken; this file records the reasoning behind them, so a change
that looks like an improvement can be checked against the argument it would
overturn.

## What the tool is

A CLI that reads a Terraform or OpenTofu plan and reports, ranked by how much
damage it can do, what the change actually does and what it cannot tell you.

```
terraform plan -out tfplan
terraform show -json tfplan > plan.json
terraken plan.json
```

It takes a file. It never runs `terraform`, never reads a cloud credential,
never makes a network call and never applies anything. Nothing leaves the
machine.

The product is the triage and the ranking, not the detection. `terraform plan`
already contains this information and presents it as a wall of undifferentiated
text, where the line that destroys a database looks exactly like the line that
adds a tag. Ranking that text is the whole job.

## Never printing a value

The first constraint is that no attribute value from a plan reaches any output,
in any format - not masked, not redacted, not truncated. Only addresses,
attribute paths, counts, levels and the tool's own sentences.

This is stricter than it needs to be for the tool to work, and that is
deliberate. The usual approach is to mask whatever Terraform marked
`sensitive`, which makes the guarantee only as good as the provider's marking.
It is not good enough: a live credential has been found in a real plan that
Terraform had not marked. Once a tool prints values at all, keeping secrets out
of the output becomes a problem that reopens with every provider schema.

Not printing values closes it by construction. The cost is real - the report
cannot show a before and after - and it is paid willingly, because it is what
makes the tool safe to point at a plan nobody has vetted.

A set of tests exists purely to hold this line, named individually in
[`AGENTS.md`](../AGENTS.md) so there is one register of them rather than a
count in three files that drift apart. A finding that needs a value in the
output is not a finding this tool can have.

## Four levels and no medium

`critical`, `high`, `low`, `info`. There is no middle bucket, because a middle
bucket is where findings go to be ignored, and every action sorts cleanly
without one.

Base risk comes from the action alone: a delete or a replace is high, an update
in place is low, a create or a no-change import is info. There is exactly one
escalation: if the resource being destroyed is on the data-loss list, high
becomes critical.

Critical means the resource type holds data and destroying it loses that data.
That is the only thing **the tool's own ranking** means by it, and widening it
would make the word mean two things. A plan that failed to compute, an action
the tool does not recognise - neither of these is a data loss, so neither may
be ranked critical. This is the constraint that decides where new information
goes: if it is not one resource change losing data, it belongs outside the
severity counts rather than at the top of them.

A team's own rule is the one thing that can put any level on any finding,
including critical, and that is not an exception to the rule above but the
other side of it. The constraint governs what **terraken** claims; a rule
carries the team's message, is marked as theirs rather than the tool's
judgement, and is written down in a file somebody committed. The report says
which of the two is speaking, which is the whole reason the distinction is
worth keeping.

### And one non-level

`unranked` is what a finding gets when the tool could not assess it at all - an
action verb this build has never seen, or a sequence of verbs Terraform does not
document. It is not a fifth severity. It is the absence of one, and the rule
above is why: the tool does not know what the operation does, so it has no
damage to measure, and both `info` and `critical` would be a guess dressed as a
measurement.

So it is counted apart from the four, it is not in `counts`, and `--fail-on` -
which takes a severity - cannot see it. What it does get is the things that
stop it disappearing: it sorts above critical so a reviewer meets it first, no
`--min-level` can filter it out, it has its own tally on the summary line, and
`--format gate` carries it in a separate array whether or not a gate was asked
for.

A team that wants it to stop a pipeline writes a rule. `actions:
["unsupported"]` is the one that says so directly, though any rule that
matches the resource will do it, because a rule assigns a severity - and a
rule with no `level` assigns `high`. Going through a rule is the point rather
than the particular condition: the alternative, letting an unrankable finding
satisfy any threshold on its own, would mean a pinned `--fail-on critical`
changed meaning the day Terraform shipped a new action verb, without anybody
deciding it should.

A rule assigning a severity does **not** make the operation understood, and two
places are deliberately keyed on the kind rather than the level so that ranking
one cannot quietly stop reporting it: `Report.AtLeast`, so no `--min-level` can
filter it out, and the gate's `unsupported` array, so a machine consumer is
still told the verdict is incomplete.

Internally `Unranked` is nonetheless the highest value in the `Level` enum, and
that is a safety property rather than a claim. Every comparison it is meant to
be excluded from is excluded explicitly; the ordering decides what happens at a
site somebody adds later and forgets to guard, and there it errs towards showing
the finding rather than hiding it - which is the failure this whole finding type
exists to correct.

## Degrading honestly

A resource type the data-loss list does not carry gets its base action risk and
says so, rather than guessing. Silence about what it does not know is the
failure mode this tool exists to correct, so it must not commit it.

The same rule produces the most distinctive finding type in the tool,
`unverifiable until apply`: the values in `after_unknown` are precisely what
nobody can know until the change runs, and therefore precisely which claims
about the change cannot be checked in review. An unknown reported as an unknown
is the feature. An unknown quietly rendered as no change would be the bug.

## Annotations, not silent escalations

Annotations sit orthogonal to the risk level and carry most of the signal. The
important one is the rename that forgot a `moved` block.

Terraform represents two superficially similar cases quite differently, and the
difference is the point:

- A **forced replacement** is one entry with actions `[delete, create]`, and
  Terraform supplies `replace_paths` - the exact attribute that forced it.
  That can be reported directly.
- A **rename without a `moved` block** is two separate entries, a delete of the
  old address and a create of the new one. Nothing in the plan links them.

The second is the one that matters, because it is how a refactor destroys a
database it meant to keep. The heuristic pairs a delete with a create where the
type matches, the module matches and the non-computed attributes are
near-identical.

It is the only rule with meaningful false-positive risk, so it is an
annotation that always shows its evidence - which attributes matched, which did
not - and never a silent escalation. `--moved` prints the blocks it would
propose as HCL, on their own, because that output is meant to be redirected
into a `.tf` file and anything else on the stream would land in somebody's
configuration.

## A pure assessment boundary

```
internal/plan      parse plan JSON via hashicorp/terraform-json
internal/assess    changes -> findings, levels, annotations
internal/render    findings -> terminal, markdown, json, html, gate
cmd/terraken       flags, file in, exit code out
```

`internal/assess` is pure: plan in, report out, no clock, no network, no
filesystem, no logging. That is what makes the entire risk model testable as a
table, and it is the boundary that keeps the rules honest - a rule that needs
to look something up is a rule that has left the contract.

`hashicorp/terraform-json` is the official library for this format, so the
parser is a solved problem rather than something hand-rolled. OpenTofu emits
the same plan JSON, so both engines are served from one code path. The library
has a known cost, recorded here because it constrains what can be built: it
drops attributes it does not know about, and it lags behind Terraform's own
emitter. Reaching a field the library has not added yet needs its own decoding,
not a version bump.

## Stating a fact, not ruling

The report says what the change destroys and what cannot be verified until
apply. It does not say whether to approve.

`--fail-on` is the only thing that turns a finding into a decision, and it is
off by default. A tool that breaks the build on day one gets removed on day
one, so a team can adopt it read-only in CI first and turn enforcement on once
they trust it. Exit codes are 0 for clean or below the threshold, 1 for
threshold met, 2 for a usage or parse error.

`--min-level` filters what is displayed and never what the gate sees, so
turning down the noise cannot quietly turn off the gate.

When a team needs an exception, the answer is the rules file rather than a
baseline or a suppression list. A rule can lower a level as well as raise it,
per rule and visibly: a team that knows a particular destroy is routine in
their estate is better served by saying so than by learning to ignore a
critical, which is how a real one gets missed.

`--format gate` emits a versioned machine verdict under the schema identifier
`terraken.gate/v1`, so a consumer can depend on the shape rather than on
parsing a report.

## Plan content is untrusted input

A resource address can carry a `for_each` key chosen by whoever wrote the
Terraform, and on a fork pull request that is not somebody to trust. Everything
taken from the plan is escaped before it reaches HTML or a live-markdown
context, and the HTML style block is asserted to contain nothing from the plan
at all.

The HTML report stays a single self-contained document - inline CSS, no
external stylesheet, font, image or script - so it can be attached to a build
without the reader's browser fetching anything.

## What it deliberately does not do

- **Cost estimation, policy enforcement, drift detection against a live cloud.**
  Each needs credentials, a network call or both, which ends the contract that
  makes the tool safe.
- **Provider schema validation.** `terraform validate` already does this, and
  doing it needs a provider plugin, which means `terraform init`.
- **HCL parsing.** The plan is the layer that matters, and the plan's
  `configuration` object already carries the parsed configuration tree,
  including an `expressions` map with references unwrapped - which is what
  makes blast radius computable from the file alone.
- **Per-resource-type semantic rendering.** Teaching the tool what any one
  provider's resource means is an obligation that never ends and covers one
  cloud at a time. Every finding is about the shape of a change, not about what
  a particular resource type means.
- **A model in the loop.** The same plan gives the same verdict. There is
  nothing to talk round, and nothing to send a plan to.

## Testing

Golden-file fixtures over the `assess` package, with every risk rule and every
annotation as a table entry. The pure boundary is what makes this cheap.

Two fixture sources, and both are needed. Plans generated by Terraform against
a real estate exercise the parser against genuine output rather than against
something hand-written to pass. Hand-built fixtures cover the destructive paths
that cannot safely be produced from a real estate, and the deliberately nasty
ones - `testdata/reordered-secrets.json` exists only to be leaked from.

A new finding type needs a golden test, because that is how a report's exact
shape is pinned and how a rendering change that alters meaning shows up as a
diff rather than as nothing.

## Known weak points

Recorded so they are not rediscovered as surprises.

- **The rename heuristic produces false positives.** Mitigated by making it an
  annotation that shows its evidence. If it proves noisy in practice it can go
  behind a flag without disturbing the risk model.
- **Missed-`moved` evidence prints twice**, once under each half of the pair,
  which reads as noise.
- **The low section is still long on a large plan.** Grouping makes it
  skippable, not short.
- **The annotation map in `moved.go` is keyed by address**, which is not unique
  when a deposed object is present.
- **`isTTY` uses `ModeCharDevice`**, so `/dev/null` reads as a terminal.
  Harmless.
- **The `0600` mode on `--out`** is a Unix claim, and the Windows matrix proved
  it needed saying that way.
