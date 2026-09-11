# terraverdict

Read a Terraform or OpenTofu plan and find out what it actually does.

`terraform plan` already knows the blast radius of your change. It just prints
it as several hundred lines of undifferentiated text, and the one line that
destroys your database looks exactly like the one that adds a tag.

terraverdict ranks it.

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

    CRITICAL  azurerm_postgresql_flexible_server.main
              destroy and create
              this resource type holds data, so destroying it loses that data
              because an attribute changed that cannot be updated in place
              forces replacement: zone

    1 finding: 1 critical

## Flags

| Flag | What it does |
|---|---|
| `--format terminal\|md\|json` | Output format. Default `terminal`. |
| `--fail-on critical\|high\|low\|info` | Exit 1 if any finding reaches this level. Off by default. |
| `--min-level critical\|high\|low\|info` | Only show findings at this level or above. Shows everything by default. |
| `--no-colour`, `--no-color` | Never colour terminal output. |
| `--version` | Print the version and exit. |

Colour is only used when output is going to a terminal. Setting
[`NO_COLOR`](https://no-color.org) to anything non-empty switches it off
too.

Flags go before the file: `tv --format md plan.json`.

### Turning the volume down

A 90-resource plan runs to a few hundred lines, most of it `update in
place` stanzas that say nothing else. Ranking sorts that problem; it does
not remove it. `--min-level` does:

    tv --min-level high plan.json

    90 findings: 4 critical, 60 low, 26 info (86 below high not shown)

The summary always counts the whole plan and always says how much is
hidden. `--min-level` changes what you read, never what was found, and
never the exit code - `--fail-on` is measured against every finding, so
turning the volume down cannot turn a gate off.

## What it tells you

- **What this change destroys**, ranked, with the ones that lose data first
- **Why** a resource is being replaced, using Terraform's own stated reason
- **Which attribute** forced the replacement
- **Renames that forgot a `moved` block** - a destroy and a create that look
  like the same resource, which is how an agent refactor quietly destroys a
  database it meant to keep
- **What cannot be known until apply**, so you can see which claims about this
  change are unverifiable in review

## What it does not do

It takes a file, or a piped stream. It never runs `terraform`, never reads
your cloud credentials, never makes a network call, and never applies
anything. Sensitive values are redacted and there is no flag to turn that
off.

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

Being upfront about what a heuristic cannot do is the point of this tool. It
exists because other things - a wall of plan text, a `sensitive` flag that
does not catch everything - are quietly wrong in ways nobody flags.

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
