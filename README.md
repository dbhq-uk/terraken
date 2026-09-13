<p align="center">
  <picture>
    <source media="(prefers-color-scheme: dark)" srcset="assets/banner-dark.svg">
    <img src="assets/banner-light.svg" alt="terraverdict" width="620">
  </picture>
</p>

<p align="center">
  <a href="https://github.com/dbhq-uk/terraverdict/releases"><img src="https://img.shields.io/github/v/release/dbhq-uk/terraverdict?color=2B6BF3&label=release" alt="Release"></a>
  <a href="https://github.com/dbhq-uk/terraverdict/actions/workflows/ci.yml"><img src="https://github.com/dbhq-uk/terraverdict/actions/workflows/ci.yml/badge.svg" alt="CI"></a>
  <a href="https://goreportcard.com/report/github.com/dbhq-uk/terraverdict"><img src="https://goreportcard.com/badge/github.com/dbhq-uk/terraverdict" alt="Go Report Card"></a>
  <a href="LICENSE"><img src="https://img.shields.io/badge/licence-MIT-2AD4C5" alt="MIT licence"></a>
</p>

`terraform plan` already knows the blast radius of your change. It just prints
it as several hundred lines of undifferentiated text, and the one line that
destroys your database looks exactly like the one that adds a tag.

terraverdict ranks it.

<p align="center">
  <img src="assets/demo.svg" alt="terraverdict ranking a plan by risk" width="800">
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
- **Lists whose before and after hold the same elements in a different
  order**, named by attribute path, so you can tell a reshuffle from a change
  at a glance - and decide for yourself which it is
- **What cannot be known until apply**, so you can see which claims about this
  change are unverifiable in review

## Install

    go install github.com/dbhq-uk/terraverdict/cmd/tv@latest

Or download a binary from the releases page. The release ships both `tv` and
`terraverdict`; they are the same program, so use whichever name is free on
your machine.

## Use

    terraform plan -out tfplan
    terraform show -json tfplan > plan.json
    tv plan.json

Or pipe it, and never write the plan to disk at all:

    terraform show -json tfplan | tv -

For example, a plan that replaces a database because of an attribute that
cannot be updated in place:

    terraverdict  1 finding  terraform 1.9.8
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

    tv --format md plan.json >> "$GITHUB_STEP_SUMMARY"
    tv --fail-on critical plan.json

`--fail-on` is off by default. Adopt it read-only first.

There is a GitHub Action in this repository that does both in one step:

    - uses: dbhq-uk/terraverdict@v0.1.0
      with:
        plan: plan.json
        fail-on: critical

It writes the markdown report to the job summary and uses the same run's
exit code as the gate, so the summary and the verdict cannot disagree.

## Flags

| Flag | What it does |
|---|---|
| `--format terminal\|md\|json\|html` | Output format. Default `terminal`. |
| `--out <path>` | Write the report to a file instead of standard output. Works for every format. |
| `--fail-on critical\|high\|low\|info` | Exit 1 if any finding reaches this level. Off by default. |
| `--min-level critical\|high\|low\|info` | Only show findings at this level or above. Shows everything by default. |
| `--plain` | No colour, and ASCII only - no box drawing anywhere in the output. |
| `--no-colour`, `--no-color` | Never colour terminal output. |
| `--version` | Print the version and exit. |

Colour is only used when output is going to a terminal. Setting
[`NO_COLOR`](https://no-color.org) to anything non-empty switches it off
too, and `FORCE_COLOR` turns it back on when the destination is a pipe -
a CI log that renders ANSI, or a pager held open with `less -R`. `NO_COLOR`
wins if both are set, because turning colour off should never be the
setting that loses. `--plain` goes further and also drops the box-drawing characters, for
a pipeline, a log viewer, or a console that renders them badly:

    tv --plain plan.json

    terraverdict  1 finding  terraform 1.9.8
    ========================================================================

    CRITICAL ------------------------------------------------------------  1

      azurerm_postgresql_flexible_server.main
      destroy and create
      |- holds data, so destroying it loses that data
      |- an attribute changed that cannot be updated in place
      `- forces replacement   zone

Flags go before the file: `tv --format md plan.json`.

### Writing the report to a file

`--out` sends the report to a path and prints one line naming it, so
nothing else lands on standard output:

    tv --format html --out report.html plan.json
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

    tv --min-level high plan.json

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

For an update or a replacement, terraverdict says when a changed list holds
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
reordering any of those changes what the infrastructure does. terraverdict
has no way to know which attribute you are looking at, so it reports what
it saw and leaves the call to you. A tool that announced "no semantic
change" would eventually say it about somebody's container command, and it
would be wrong.

The comparison counts duplicates, so `["a","a","b"]` and `["a","b","b"]` are
not the same list and are not reported. Elements are compared by their JSON
encoding, so the number `15` and the string `"15"` stay different things. A
list that is unchanged is not reported either, and neither is one Terraform
cannot know until apply.

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

terraverdict redacts values it is told are sensitive. It has no way to know
about the ones Terraform did not mark - which is precisely why it never
prints an attribute's value at all, marked or not. Never paste a plan file
into an issue, a chat, or anywhere outside a private, access-controlled
pipeline.

The best plan file is the one that never exists. `tv -` reads the plan from
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

Being upfront about what a heuristic cannot do is the point of this tool. It
exists because other things - a wall of plan text, a `sensitive` flag that
does not catch everything - are quietly wrong in ways nobody flags.

## Licence

MIT. See [LICENSE](LICENSE).

Built by [DBHQ](https://dbhq.uk). Issues and pull requests welcome - please
read [SECURITY.md](SECURITY.md) before reporting anything sensitive.
