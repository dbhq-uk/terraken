# Contributing

Thanks for your interest - contributions are welcome.

## Ways to help

- Report a bug or request a feature via [issues](https://github.com/dbhq-uk/terraken/issues)
- **Send a plan that gets ranked wrong.** This is the most useful thing anybody
  can contribute, and it is worth more than a patch. A `terraform show -json`
  plan where a real risk came out low, or a harmless change came out critical,
  is a test case this tool does not have. Sanitise it first - see below

## Sending a plan

**A plan file is a secret.** It can contain credentials in the clear whether or
not Terraform marked them `sensitive`, which is the whole reason this tool never
prints values. So do not attach a real plan to a public issue.

Reduce it to the smallest thing that still shows the wrong verdict, replace
every value with a placeholder, and check it before you send. `terraken` itself is a
reasonable sanity check, because it names every resource it found:

```bash
terraken --format json reduced.json | jq '.findings[].address'
```

If you cannot reduce it safely, describe the shape instead - resource type,
action, which attribute forced the replacement - and open an issue without the
file. Email <dan@dbhq.uk> if the case itself is sensitive.

## Local development

```bash
git clone https://github.com/dbhq-uk/terraken.git
cd terraken
go build ./cmd/terraken
./terraken testdata/demo.json
```

## Before opening a PR

```bash
go vet ./...
go test ./... -race -cover
gofmt -l .                     # must print nothing
```

CI also runs `goreleaser check` and the composite action against three fixtures,
including an assertion that `--fail-on critical` really does fail. Both are
checked on every pull request because a release config or an action exercised
only on a tag is found to be broken in public, once the tag already exists.

## The bar for a new finding

**It must be readable off the plan, and it must be certain.** A finding is a
statement of fact about what the change does. If answering it needs a cloud API,
a credential, a model or a guess, it does not belong here - and if it is genuine
but unverifiable until apply, report it as exactly that rather than ranking it.

**A new finding type needs a golden test.** `internal/assess/golden_test.go`
pins a report's exact shape, so a change that alters meaning shows up as a diff
rather than as nothing.

**A new severity claim needs a reason a reviewer can check.** The report cites
Terraform's own stated replacement reason and names the attribute that forced
it. "This looks risky" is not a finding.

## Two working practices

Both are in [`AGENTS.md`](AGENTS.md) with the incidents that produced them, and
both are easy to skip because neither is enforced by a test.

**Before building a capability, read the READMEs of the nearest tools in full,
that week.** An inventory is not a substitute, and trusting one has already cost
this project a rename and a near-duplicate feature.

**Sabotage the assertion, not only the implementation.** Reverting a fix and
watching the suite go red proves the tests notice the feature is gone. It does
not prove they notice the feature being wrong. Pin every independently variable
part of a sentence separately - the verb, the count, the evidence and the
anchor can each be wrong on their own, and a claim that agrees with its own
evidence on three of them looks corroborated while it lies on the fourth.

## What we will not accept

**Anything that prints an attribute's value.** Not masked, not redacted, not
truncated, not "just the length", and not in one format because the other four
are safe. This is the tool's one guarantee that comparable tools do not make,
and a set of tests exists to hold it - [`AGENTS.md`](AGENTS.md) names them, and
is the one register of them. A feature that needs a value cannot be built here.

**A model, a network call or a credential.** The same plan must always give the
same verdict, offline. There is a wide field of AI-powered Terraform risk tools
and this is deliberately not one of them.

**Writing to somebody's configuration.** Terraken reports; it never edits
Terraform. If you want the `moved` blocks written for you,
[tfautomv](https://github.com/busser/tfautomv) does that and does it well - the
README says so, and that boundary is a design decision rather than a missing
feature.

**A gate that is on by default.** `--fail-on` stays off unless asked for. A tool
that blocks a pipeline the day it is installed gets removed rather than adopted.

## Licence

By contributing you agree your work is licensed under the [MIT licence](LICENSE).
