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

The claim is proved rather than asserted, and the difference matters because
every tool in this space promises something about secrets. On every build, CI
generates plans carrying planted credentials in every position a value can
occupy in a plan file, marks none of them sensitive, runs each one through
every output the command can produce, and reports the count. A set of
hand-written tests pins specific known cases beside it. Both are named
individually in [`AGENTS.md`](../AGENTS.md), so there is one register of them
rather than a count in three files that drift apart.

Two things about the shape of that proof are decisions rather than details.

**The axis enumerated exhaustively is the position, not the value.** A handful
of credential shapes is plenty; what a fixed set of fixtures can never give is
coverage of the places a value can hide. Every disclosure defect this project
has seen in comparable tools was in a position nobody had written a fixture
for.

**A path is not a value.** Attribute paths are printed - that is most of what
the output is - so a credential used as a map key would appear. That is the
edge of what the guarantee covers rather than a hole in it, and the generator
says so where a reader will meet it rather than leaving it to be discovered.

A finding that needs a value in the output is not a finding this tool can have.

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

A rule assigning a severity does **not** make the operation understood, and
three places are deliberately keyed on the kind rather than the level so that
ranking one cannot quietly stop reporting it: `Report.AtLeast`, so no
`--min-level` can filter it out; the gate's `unsupported` array, so a machine
consumer is still told the verdict is incomplete; and the coverage report,
which counts an unreadable operation as one it could not assess however a rule
later ranked it.

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

## What the tool refuses to read

Two things, and both are recorded because "not built yet" and "will never be
built" look identical to somebody reading the code a year later.

**A check's `error_message`.** A failed check is worth reporting precisely
because Terraform treats it as a warning and lets the plan succeed, so a
pipeline steps over it. The message is the useful part - and it is written by
whoever wrote the configuration and INTERPOLATED by Terraform. A plan generated
while building that feature carried a live GitHub token in a check's failure
message, because the condition referenced a variable holding one.

Printing it would put an attribute value in the output. The carve-out was
available - it is the author's own sentence, not a value the tool went looking
for - and it was refused, because a guarantee with one exception in it is a
guarantee a reader has to hold caveats about, and this one is the reason the
tool is safe to point at a plan nobody has vetted. So the report names the
check, says what its status means, and counts the problems. The message is in
the plan output, where the reader already has it.

`CheckFinding` has no field for a message, and a test asserts the type has
none, because a field would eventually be filled in.

**A check's status, quoted back into prose.** Terraform emits a fixed
vocabulary, but `Report` is an ordinary struct built from a file this package
does not validate - so a status is plan-derived text, and interpolating an
unrecognised one into a sentence would make "the tool's own words" untrue. An
unrecognised status is reported as unrecognised.

## The configuration is not an input

**Terraken reads a plan file and nothing else.** It does not parse HCL, and the
`needs-hcl` label exists so that a capability which would need it is marked
rather than quietly built. This is [#44](https://github.com/dbhq-uk/terraken/issues/44),
recorded here so it is answered once rather than re-argued every time.

**What HCL would genuinely add**, checked against a real Terraform 1.16.1 plan
generated from a root that sets each one:

- **`ignore_changes`**, which explains why a diff is *absent*. Configured and
  absent from the plan; changing the ignored attribute produced a no-op with
  nothing to say why.
- **`prevent_destroy`**. Also absent, and the reason it was dismissed was
  wrong. `terraform plan -destroy` exits 1 on a protected resource, but the
  plan file it wrote is still readable: `terraform show -json` exits 0 and
  produces `errored: true`, `applyable: false`, `complete: false` and the
  delete itself. Terraken reads that today and reports the destroy with the
  three status flags beside it. The distinction is **not applyable**, not
  **not readable** - so knowing about `prevent_destroy` would let the report
  say why, which it currently cannot.
- **A source file and line**, which is the one thing that would make SARIF
  output useful rather than merely possible.
- **Comments**, which is how an inline suppression would work.

**Declared version constraints were on that list and should not have been.**
`provider_config` carries `version_constraint` beside `full_name`, so a
provider's declared constraint is in the plan already. The claim came from the
issue rather than from a plan, which is exactly the failure the working
practice in `AGENTS.md` exists to prevent, and it was caught by somebody
generating one. Terraform's own `required_version` is genuinely absent.

`create_before_destroy` is **not** on that list, and it was the strongest
argument until it was checked. The plan carries it as the order of the
`actions` array - see "which way round a replacement happens". A dependency
that travels through a module is not on the list either: the call's inputs and
the module's outputs are both in `configuration`, so following them needs no
second input - which is what
[#57](https://github.com/dbhq-uk/terraken/issues/57) does.

**The answer is no, for four reasons, and the first is the one that decides
it.**

**The two inputs can disagree and nothing would detect it.** A plan is a sealed
artefact from one moment; a working tree moves. Somebody edits `main.tf` after
planning, or CI checks out a different ref, and every finding becomes suspect.
There is no hash of the configuration in the plan to check against, so terraken
could not even tell the reader it had happened. A tool whose product is
trustworthiness must not be able to be confidently wrong.

**A remote module has to have been fetched.** Following a `module` block to its
source means reading `.terraform/modules` for anything that is not a local
path, and that directory is populated by `terraform init` or `terraform get` -
either way by running Terraform, which the second constraint forbids terraken
from doing. It would have to require somebody else to have run it and say so
when they had not. A `source = "./child"` is readable without any of that, so
this reason covers remote modules rather than all of them.

**HCL is a weaker view than the plan.** A configuration scanner has to be given
variable values or fall back to defaults, and the plan has them already
resolved - this plan states `release = "v2"` while the configuration declares
the default `"v1"`. It would give terraken a second, less certain view of
something it already sees settled.

**A second input is a second failure surface**, permanently.

### The one exception, and its limits

`hashicorp/hcl/v2` IS a dependency of this repository, and only from `_test.go`
files. `internal/assess/propose_test.go` asserts that emitted `moved` blocks
parse, because the only honest way to check that is with the parser Terraform
uses - a hand-rolled check confirms the author's own idea of the grammar.
`internal/assess/replace_order_test.go` reads the generating roots in
`testdata/_gen` to verify what a fixture claims about the configuration it came
from.

Neither is the tool reading configuration. `go list -deps ./cmd/terraken` does
not include it, and that is the check if it is ever in doubt. **HCL may be used
to verify evidence about a fixture. It may not become an input to the report.**

### If `ignore_changes` ever becomes load-bearing

The case is real - an absent diff is exactly the kind of silence this tool
exists to name - and the shape would not be a general parser. An optional
`--config <dir>`, off by default, read for lifecycle blocks and nothing else;
the report states when it was not supplied, so a reader knows the tool could
not account for `ignore_changes` rather than assuming there was none, which is
constraint 5 applied to the tool's own inputs. It would never resolve a
variable, follow a module or read an expression. If it grew past lifecycle
blocks, this decision is reopened rather than stretched.

Building it at all means reopening this section and `AGENTS.md` together,
because both currently say the configuration is not an input full stop. The
mismatch between a plan and a working tree would still be unsolved, and the
`--config` shape does not solve it - it only narrows what can be wrong.

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
Terraform, and on a fork pull request that is not somebody to trust.

**This is the second guarantee, and it is the same size as the first.** No plan
can make a report say something other than what the plan does. The attack is
not defacement, it is reviewer deception: a table that grew a row, a terminal
repainted to say nothing is wrong, a line whose reading order was reversed. A
report that can be made to lie is worse than no report, because it is the thing
being trusted.

Two decisions about its shape are worth recording.

**Sanitising happens once, over the whole report, at the entry to every
renderer.** The alternative was escaping at each of the forty-odd places a
string is written, which is correct exactly until somebody adds the
forty-first. Each renderer still calls it itself - `render.Write` does not do
it on their behalf - so the property is one line per renderer rather than free,
and a test holds every registered format to having an injection check. Context-specific escaping - HTML, a markdown cell, a code span -
still happens per format on the way out, because it has to: the dangerous
character in one destination is ordinary text in another.

**The escaping is injective.** A backslash is escaped along with everything
else, and that is load-bearing rather than tidy: without it a real newline and
the two characters `\n` both render as `\n`, and since `Shape.ByModule` is a
map keyed by module name, two different modules collided into one entry. One
count silently overwrote the other and the same plan produced different output
between runs, which breaks determinism as well as accuracy.

**A dangerous character is shown, not dropped.** An escape sequence becomes
`\x1b` and a right-to-left override becomes `\u202e`. Removing them would
make `app["a"]` and `app["a\u202e"]` render identically, which is the
deception rather than the cure - a reviewer comparing two addresses has to be
able to see that they differ. It also keeps the report honest about what the
file actually held.

The proof enumerates every ordered PAIR of hostile fragments rather than
sampling them, because nearly every real attack is a pair: a delimiter that
ends the context, then a payload that acts in the one it lands in. It asserts
the structure of the output - rows, elements, banners, connectors - rather than
the absence of a character, because counting characters proves a payload did
not arrive in one particular shape, and counting structure proves the report
still says what it was given.

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

Two fixture sources, and both are needed.

**Real Terraform output, from a throwaway local root.** `terraform_data` and
the `local` provider, nothing else - no cloud, no credential, no estate. It
exercises the parser against genuine output rather than against something
hand-written to pass, and it is the only thing that can settle a question about
what the format actually contains. `testdata/real-checks.json` exists because
the roadmap refused to let the checks feature be built before somebody looked
at real output, and looking corrected four separate beliefs about the shape.

**Never from a real estate.** AGENTS.md states this as a constraint and this
file used to contradict it, which is how a document gets trusted for the wrong
sentence. Generating a fixture from a live estate is how a Cloudflare token
ended up in a plan file in the first place, and a local root answers every
question a real one would.

Hand-built fixtures cover the destructive paths that cannot be produced locally
at all, and the deliberately nasty ones - `testdata/reordered-secrets.json`
exists only to be leaked from.

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
