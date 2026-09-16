<p align="center">
  <picture>
    <source media="(prefers-color-scheme: dark)" srcset="assets/banner-dark.svg">
    <img src="assets/banner-light.svg" alt="Terraken" width="620">
  </picture>
</p>

<p align="center">
  <a href="https://github.com/dbhq-uk/terraken/releases"><img src="https://img.shields.io/github/v/release/dbhq-uk/terraken?color=2B6BF3&label=release" alt="Release"></a>
  <a href="https://github.com/dbhq-uk/terraken/actions/workflows/ci.yml"><img src="https://github.com/dbhq-uk/terraken/actions/workflows/ci.yml/badge.svg" alt="CI"></a>
  <a href="https://goreportcard.com/report/github.com/dbhq-uk/terraken"><img src="https://goreportcard.com/badge/github.com/dbhq-uk/terraken" alt="Go Report Card"></a>
  <a href="LICENSE"><img src="https://img.shields.io/badge/licence-MIT-2AD4C5" alt="MIT licence"></a>
</p>

`terraform plan` already knows the blast radius of your change. It just prints
it as several hundred lines of undifferentiated text, and the one line that
destroys your database looks exactly like the one that adds a tag.

Terraken ranks it.

<p align="center">
  <img src="assets/demo.svg" alt="Terraken ranking a plan by risk" width="800">
</p>

It reads a plan file and nothing else. No credentials, no network, no `apply`,
and it never prints an attribute's value - not even one Terraform forgot to
mark sensitive. It is deterministic: the same plan gives the same verdict, and
there is no model in the loop to talk you round.

## What it tells you

- **What this change destroys**, ranked, with the ones that lose data first
- **Why** a resource is being replaced, using Terraform's own stated reason
- **Which attribute** forced the replacement
- **Renames that forgot a `moved` block** - a destroy and a create that look
  like the same resource, which is how an agent refactor quietly destroys a
  database it meant to keep
- **Attributes whose before and after are the same value written
  differently** - a reshuffled list, a JSON policy whose keys moved, a
  re-indented heredoc, a port that came back as a string, a null that became
  an empty list - each named by attribute path and by class, so you can tell
  a rewrite from a change at a glance and decide for yourself which it is
- **When that is the whole of a resource's change**, said out loud, which is
  the fastest way to clear an `update in place` that is really nothing
- **What cannot be known until apply**, so you can see which claims about this
  change are unverifiable in review

## Install

    go install github.com/dbhq-uk/terraken/cmd/terraken@latest

Or download a binary from the releases page. The release ships both `terraken`
and `tken`; they are the same program, so use whichever name is free on your
machine.

**Every platform terraform runs on**, which is the rule rather than a list that
grew by accident: this tool reads what `terraform show -json` emits, so it has
no business claiming a platform terraform does not support and no excuse for
missing one it does.

| OS | Architectures |
|---|---|
| Linux | `386` `amd64` `arm` `arm64` `s390x` |
| macOS | `amd64` `arm64` |
| Windows | `386` `amd64` `arm64` |
| FreeBSD | `386` `amd64` `arm` |
| OpenBSD | `386` `amd64` |
| Solaris | `amd64` |

Windows archives are `.zip`; the rest are `.tar.gz`. The test suite runs on
Linux, macOS and Windows, which is every operating system GitHub hosts a runner
for. The other three are compile-checked on every push and nothing more - that
is a real limit, and it is stated rather than implied.

`tken` rather than anything shorter, because the three obvious candidates all
belong to somebody else: `tk` is Tcl/Tk, `tkn` is the Tekton CLI, and `kn` is
the Knative CLI, which even has a Homebrew formula of that name. Both of the
latter are CI/CD tools, so they share this one's audience. Installing us must
not shadow a command you already rely on.

## Use

    terraform plan -out tfplan
    terraform show -json tfplan > plan.json
    terraken plan.json

Or pipe it, and never write the plan to disk at all:

    terraform show -json tfplan | terraken -

For example, a plan that replaces a database because of an attribute that
cannot be updated in place:

    terraken  1 finding  terraform 1.9.8
    ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

    CRITICAL ───────────────────────────────────────────────────────────  1

      azurerm_postgresql_flexible_server.main
      destroy and create
      ├ holds data, so destroying it loses that data
      ├ an attribute changed that cannot be updated in place
      └ forces replacement   zone

    ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
    1 critical

One section per severity present, most severe first, and none at all for a
severity with nothing in it. The report is set to the width of your
terminal, between 60 and 100 columns.

## In CI

    terraken --format md plan.json >> "$GITHUB_STEP_SUMMARY"
    terraken --fail-on critical plan.json

`--fail-on` is off by default. Adopt it read-only first.

There is a GitHub Action in this repository that does both in one step:

    - uses: dbhq-uk/terraken@v0.5.0
      with:
        plan: plan.json
        fail-on: critical

It writes the markdown report to the job summary and uses the same run's
exit code as the gate, so the summary and the verdict cannot disagree.

## Proposing the moved blocks

When a plan destroys one resource and creates another that looks like the same
thing, `--moved` writes the block you needed:

    terraken --moved plan.json

    # Proposed by terraken from a plan file.
    #
    # VERIFY EACH PAIRING BEFORE APPLYING. These are inferred from attribute
    # similarity, not from any record of what you intended. A moved block naming
    # the wrong resource adopts a decommissioned object's state under a live
    # address, which is worse than the missing block it fixes.

    # 5 of 5 compared attributes identical.
    moved {
      from = azurerm_subnet.app
      to   = azurerm_subnet.application
    }

Redirect it into a `.tf` file. Everything that is not a block is a comment, so
the whole output is valid HCL - verified in the tests with the same parser
terraform uses, and the sample above passes `terraform validate` and
`terraform fmt -check`.

**It refuses when it cannot tell.** If two creates match the deleted resource
equally well, no block is emitted for that pairing. Instead it says so and
names both candidates, because a block is copy-pasteable in a way a warning is
not, and picking by alphabetical order is not evidence.

It writes to stdout, or to `--out <path>`. It never edits a file you did not
name.

## How it compares

There are good tools either side of this one, and it is worth being plain
about where the line falls.

**[tfautomv](https://github.com/busser/tfautomv)** (900 stars) inspects a plan,
finds create/delete pairs left by a refactor, and *writes the `moved` blocks
into your configuration for you*. If you are the person doing the rename, use
it - it fixes the problem rather than reporting it.

`--moved` narrows that gap but does not close it, and the remaining difference
is the point. Terraken emits the block to stdout or to a path you name, and
never edits a file you did not ask for; tfautomv is a refactoring tool and this
is a review tool that happens to be able to show you its working. It also
**refuses where tfautomv chooses**: two creates matching the deleted resource
equally well produce a stated refusal naming both, rather than a block picked
by the tiebreak. That is the right call for something read in a pull request
and the wrong one for something run by the author, who knows which they meant.

**[tfmv](https://github.com/suzuki-shunsuke/tfmv)** renames resources and
generates `moved` blocks, so the same distinction applies.

**[tfplan2md](https://github.com/oocx/tfplan2md)** turns a plan into a readable
markdown report for pull request review, groups by module, shows semantic
diffs on lists and masks sensitive values. It overlaps with this tool and it
is good at what it does. The difference is what the output is *for*:
tfplan2md makes a plan **readable**, and Terraken makes it **ranked** -
it sorts by how much damage a change can do, escalates to critical when the
resource holds data, and gives you `--fail-on` so a pipeline can stop on it.

There is also a wide field of "AI-powered Terraform plan risk" projects. This
is not one of them. There is no model in the loop, nothing is sent anywhere,
and the same plan always produces the same verdict.

**The one guarantee none of the above makes:** Terraken never prints an
attribute's value, in any format. Not a masked one, not a redacted one - it
does not put values in its output at all. Masking relies on Terraform having
marked the value sensitive, and the section below on plan files is there
because a live credential was found in a real plan that Terraform had not
marked. Paths, counts and its own sentences are all this tool will ever show
you.

## Flags

| Flag | What it does |
|---|---|
| `--format terminal\|md\|json\|html` | Output format. Default `terminal`. |
| `--out <path>` | Write the report to a file instead of standard output. Works for every format. |
| `--fail-on critical\|high\|low\|info` | Exit 1 if any finding reaches this level. Off by default. |
| `--min-level critical\|high\|low\|info` | Only show findings at this level or above. Shows everything by default. |
| `--plain` | No colour, and ASCII only - no box drawing anywhere in the output. |
| `--no-colour`, `--no-color` | Never colour terminal output. |
| `--moved` | Instead of the report, print the `moved` blocks this plan looks like it forgot, as HCL. |
| `--version` | Print the version and exit. |

Colour is only used when output is going to a terminal. Setting
[`NO_COLOR`](https://no-color.org) to anything non-empty switches it off
too, and `FORCE_COLOR` turns it back on when the destination is a pipe -
a CI log that renders ANSI, or a pager held open with `less -R`. `NO_COLOR`
wins if both are set, because turning colour off should never be the
setting that loses. `--plain` goes further and also drops the box-drawing characters, for
a pipeline, a log viewer, or a console that renders them badly:

    terraken --plain plan.json

    terraken  1 finding  terraform 1.9.8
    ========================================================================

    CRITICAL ------------------------------------------------------------  1

      azurerm_postgresql_flexible_server.main
      destroy and create
      |- holds data, so destroying it loses that data
      |- an attribute changed that cannot be updated in place
      `- forces replacement   zone

Flags go before the file: `terraken --format md plan.json`.

### Writing the report to a file

`--out` sends the report to a path and prints one line naming it, so
nothing else lands on standard output:

    terraken --format html --out report.html plan.json
    wrote report.html

The file is created mode 0600. The report names every resource in the
plan, which on a shared runner is a map of the estate; widen it yourself
if you want to. Colour is never written to a file, whatever terminal the
command was launched from.

### The HTML report

`--format html` writes one self-contained document: inline CSS, no
external stylesheet, no font, no image, no script. It opens from a
`file://` URL with nothing else on disk, and it follows
`prefers-color-scheme`, so it reads the same way in a dark editor as in a
light browser.

It is meant for a review you want to send to someone, or keep beside a
change. It carries exactly what the terminal carries - no attribute value
has ever reached any of these formats, and none reaches this one.

### Turning the volume down

A 90-resource plan runs to a few hundred lines, most of it `update in
place` stanzas that say nothing else. Ranking sorts that problem; it does
not remove it. `--min-level` does:

    terraken --min-level high plan.json

    ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
    4 critical  60 low  26 info               86 below high not shown

The summary always counts the whole plan and always says how much is
hidden. `--min-level` changes what you read, never what was found, and
never the exit code - `--fail-on` is measured against every finding, so
turning the volume down cannot turn a gate off.

The same filtering applies to `--format json`: `findings` holds only what
qualified, `counts` always stays complete and unfiltered, and `hidden` /
`hidden_below` are omitted entirely on an unfiltered run, so a consumer
should read `.hidden // 0` rather than assume the key exists.

### Same elements, different order

A lot of what a plan shows as changed is a list that came back in a
different order. Providers return sets as JSON lists, and
`service_endpoints`, `address_prefixes`, `cidr_blocks`, `subnet_ids`,
`vpc_security_group_ids` and `availability_zones` all reshuffle themselves
without anyone touching the configuration. Terraform renders a diff, and a
reviewer reads a change.

For an update or a replacement, terraken says when a changed list holds
the same elements in a different order, and names the attribute:

    aws_db_instance.main
    destroy and create
    ├ holds data, so destroying it loses that data
    ├ an attribute changed that cannot be updated in place
    ├ forces replacement   instance_class
    └ these lists hold the same elements in a different order. Order is
      significant for some attributes, such as a container command or an
      ordered rule list, so whether this one matters is yours to judge
        vpc_security_group_ids

**It is a fact, not a verdict.** It does not say the change is harmless and
it does not move the finding's level. Order is significant for plenty of
attributes - a container's `command` or `entry_point`, an
`aws_lb_listener_rule`'s actions, a route table, a WAF rule list - and
reordering any of those changes what the infrastructure does. terraken
has no way to know which attribute you are looking at, so it reports what
it saw and leaves the call to you. A tool that announced "no semantic
change" would eventually say it about somebody's container command, and it
would be wrong.

The comparison counts duplicates, so `["a","a","b"]` and `["a","b","b"]` are
not the same list and are not reported. Elements are compared by their JSON
encoding, so the number `15` and the string `"15"` stay different things. A
list that is unchanged is not reported either, and neither is one Terraform
cannot know until apply.

### The same value, written differently

A reshuffled list is one of several ways a plan shows an attribute as
changed when the two sides are the same thing written differently.
terraken names four more, on exactly the same terms:

| Class | What it found |
|---|---|
| `same-json-written-differently` | Both sides parse as JSON and hold the same data, with the keys in a different order. This is the big one: IAM and bucket policies, `aws_ecs_task_definition.container_definitions`, anything a provider round-trips as a JSON blob |
| `same-text-different-whitespace` | Both sides are the same text laid out differently: a trailing newline, an indent, a CRLF against an LF. Heredocs, policy documents, `user_data` |
| `same-number-written-differently` | Both sides are the same number written another way: `80` and `"80"`, `1e3` and `1000`. Providers are inconsistent about number against string |
| `null-on-one-side-empty-on-the-other` | One side is null and the other is an empty list, object or string. State is full of this |

    terraken plan.json

    aws_security_group.web
    update in place
    ├ same elements, different order
    │   ingress[0].cidr_blocks
    ├ same number, written differently
    │   ingress[0].from_port
    │   ingress[0].to_port
    └ every attribute this plan shows as changed here is a difference in how the
      value is written

**Every one of these is a fact, not a verdict**, for the same reason a
reordering is. Each class has a case where the difference is real, and the
report says so rather than deciding for you:

- A JSON document with its keys moved is the same policy, but a consumer
  that compares the string byte for byte sees a change.
- Whitespace is significant in a `user_data` script, in a YAML document
  carried as a string, and in anything hashed.
- `80` and `"80"` differ in type, and a type change can matter.
- `null` and `[]` are not the same to Terraform in every position, where
  null can mean "inherit a default" and empty means "explicitly none".

None of them moves a finding's level, and none of them says the change is
harmless.

#### The roll-up

The last line above is the useful part. When **every** attribute the plan
shows as changed on a resource is one of these classes - a reordering
included - terraken says so on that finding:

    every attribute this plan shows as changed here is a difference in how the
    value is written

That is still a statement of fact, and it is the single most useful thing
this tool can tell you about an `update in place` that is really nothing.
"Not in what it is" names the kind of difference each attribute fell into,
and it is not a ruling that the change is harmless: order is significant for
a container command, whitespace is significant in a script, and the roll-up
fires over both. It says what kind of difference each one is and rules on
none of them, and it never moves a finding's level.

It is a strict claim, so it is made strictly. One real change alongside four
rewrites and the roll-up stays quiet - the per-attribute lines are all still
reported, because each of them is still true, but the sentence about the
whole resource would not be. An attribute that cannot be compared at all
silences it too: something the plan shows as changed but that is not known
until apply cannot be accounted for, so the roll-up does not speak for that
resource.

Each class is reported once per attribute and never twice. Where a pair
satisfies more than one, the narrowest claim wins - a JSON document with a
trailing newline is reported as whitespace, not as JSON, because "the only
difference is whitespace" says more than "the data matches". A path the
reordering rule has already claimed is never reported again here.

## What it does not do

It takes a file, or a piped stream. It never runs `terraform`, never reads
your cloud credentials, never makes a network call, and never applies
anything. It writes one file, and only the one you name with `--out`.
Sensitive values are redacted and there is no flag to turn that off.

It does not model consequences, validate against provider schemas, check
policy, or estimate cost. Other tools do those.

## A warning about plan files

Treat plan JSON as a secret. While building a test fixture for this project,
a real `terraform show -json` of a live estate turned out to contain a live
credential - a Cloudflare Tunnel token - sitting in plain text, which
Terraform had not marked `sensitive`.

Terraform's own `sensitive` marking is best-effort: it only redacts an
attribute that a provider schema or your configuration told it to redact, and
provider schemas are not exhaustive. A plan file can contain anything state
can contain, in the clear, whether or not anything marked it.

terraken redacts values it is told are sensitive. It has no way to know
about the ones Terraform did not mark - which is precisely why it never
prints an attribute's value at all, marked or not. Never paste a plan file
into an issue, a chat, or anywhere outside a private, access-controlled
pipeline.

The best plan file is the one that never exists. `Terraken -` reads the plan from
standard input, so you can pipe `terraform show -json` straight in and skip
the file entirely.

## Limitations

The missed-`moved`-block detector is a heuristic, not proof, and it says so
every time it fires.

- It needs at least 3 comparable attributes before it will pair a delete with
  a create. Very small resource types, such as `terraform_data` or a bare
  `local_file`, rarely clear that bar, so a rename of one goes unflagged.
  This is not theoretical: `testdata/real-plan.json` contains exactly this
  case, and the detector correctly stays silent on it.
- Before counting, it drops attributes Terraform marks unknown until apply
  (there is nothing yet to compare) and attributes that are null on both
  sides (state is full of unset optional attributes, and counting agreement
  on absence as a match would pair unrelated resources).
- It pairs a delete and a create across modules as well as within one -
  moving a resource into or out of a module is one of the most common
  reasons to write a `moved` block in the first place. A same-module match is
  preferred only when two candidates are otherwise exactly tied; a
  cross-module match is still reported on its own.
- It always shows its working, in every format: the matched-over-compared
  attribute count, and a suggested `moved` block if it looks like a rename.
  In `--format md` the block goes in a collapsed `<details>` section under
  the table, so a pull request comment carries the same evidence the
  terminal does. That block is marked as needing verification before use,
  not something to paste in blind - pairing the wrong two resources adopts
  a decommissioned resource's state under a new address, which is worse
  than the problem it is meant to fix.

The same-elements-reordered rule has edges of its own, and stays quiet at
all of them rather than guessing.

- It only runs on an update or a replacement. A create has no before and a
  delete has no after, so there are not two orderings to compare.
- Two lists of different lengths are never reported, and it does not look
  inside them either: index 2 on one side is not index 2 on the other, so
  nothing found down there would be comparing the same element.
- When a list is reported as reordered, it stops at that list rather than
  descending into it, for the same reason.
- An element that will not encode as JSON is not compared at all. That
  cannot happen to a plan read off disk, but a comparison that cannot be
  made produces silence, not a guess.

The written-differently classes have theirs too.

- They only run on an update or a replacement, for the same reason: a create
  has no before and a delete has no after.
- Whitespace is compared by collapsing each run of it to a single space, not
  by removing it. `"a b"` and `"ab"` stay different strings - one has a space
  in it and the other does not, and that is not a difference in how a value
  is written.
- The JSON class only looks at strings holding a JSON object or array. A
  string holding a bare scalar - `"80"`, `"true"` - is a scalar written as
  text, and the number class is the one that speaks for it.
- A reordered JSON array is not a rewrite. Object key order carries no
  meaning and array order does, so `["run","--fast"]` against
  `["--fast","run"]` inside a document is reported as nothing at all.
- `false` and `0` are values, not absences, so neither is ever paired with
  null.
- Nothing marked unknown until apply is compared, at any depth. A mark on
  one leaf of a block leaves its siblings comparable and they are still
  reported; it is only the roll-up that the unaccounted-for leaf silences.
- A block that gained a key, a list that changed length, an attribute that
  changed shape: none of these is a rewrite, and none of them is walked into
  where the two sides no longer line up.

Being upfront about what a heuristic cannot do is the point of this tool. It
exists because other things - a wall of plan text, a `sensitive` flag that
does not catch everything - are quietly wrong in ways nobody flags.

## Licence

MIT. See [LICENSE](LICENSE).

Built by [DBHQ](https://dbhq.uk). Issues and pull requests welcome - please
read [SECURITY.md](SECURITY.md) before reporting anything sensitive.
