# AGENTS.md

Guidance for AI agents (and people) working in this repository.

## What this is

**Terraken** reads a `terraform show -json` plan and ranks what the change
does by how much damage it can do. A CLI (`terraken`, also installed as
`terraken`), plus a composite GitHub Action in `action.yml`. Go, no
dependencies on anything that talks to a cloud.

## Layout

```
cmd/terraken/                  # the command: flags, exit codes, colour decisions
internal/plan/           # load and validate a plan file or stream
internal/assess/         # the judgements - findings, levels, data loss, moved
                         #   blocks, reorders, rewrites, replacement reasons
internal/render/         # terminal, markdown, json and html output
testdata/                # plan fixtures, including deliberately nasty ones
docs/                    # why the tool is shaped this way, what it reads, what
                         #   it builds next - read these before a design change
action.yml               # the composite action the README tells people to use
assets/                  # banner, logo and the demo SVG; _gen holds their source
.goreleaser.yaml         # the release build
```

Before changing anything structural, read [`docs/design.md`](docs/design.md) -
it records the reasoning behind the constraints below, so a change that looks
like an improvement can be checked against the argument it would overturn.
[`docs/plan-file.md`](docs/plan-file.md) measures how much of the plan file the
tool actually reads, and [`docs/roadmap.md`](docs/roadmap.md) is what comes
next and in what order.

## The constraints that must not be broken

Everything else here is a preference. These are not.

**1. No attribute value ever reaches the output. In any format.** Not masked,
not redacted, not truncated - values are not printed at all. This is the one
guarantee no comparable tool makes, and it is the reason the tool is safe to
point at a plan nobody has vetted. Masking is not good enough, because masking
relies on Terraform having marked the value `sensitive`, and a live credential
has been found in a real plan that Terraform had not marked. Paths, counts,
levels and the tool's own sentences are all it may show.

Six tests exist purely to hold this line - `TestSensitiveAnnotationNeverPrintsAValue`,
`TestReorderedNeverPrintsAValue`, `TestRewritesNeverPrintAValue`,
`TestSecretsFixtureNeverLeaksAValue`, `TestNoDetectedValueReachesTheExposure`
and `TestNoDetectedCredentialReachesAnyFormat`. The last two are against
`testdata/unmarked-credentials.json` and the two before them against
`testdata/reordered-secrets.json` - both fixtures exist only to be leaked from.
If you add a renderer or a finding type, add the equivalent test with it. A
feature that needs a value in the output is not a feature this tool can have.

**The credential detector is the sharpest edge of this rule.** It is the one
part of the tool that knows which values are worth stealing, so a leak there
would be worse than not looking at all - it would name the ones to take.
`internal/assess/credentials.go` may read any value it likes and may return
only a path and a class chosen from a fixed set. A class that interpolated any
part of a value - a prefix, a length, a character count - is a leak wearing a
hat, and `TestClassifyValueNeverReturnsAnythingDerivedFromTheValue` is there to
catch one being added later.

**New fixtures use `terraform_data` and the `local` provider only. Never real
infrastructure.** Generating one from a real estate is how a live Cloudflare
token ended up in a plan file in the first place.

**2. Deterministic, offline, read only.** The same plan always produces the same
verdict. There is no model in the loop, no network call, no credential, and
`terraform` is never invoked. The only file written is the one `--out` names,
created mode 0600. Do not add an API, a cache, a lookup or a "smart" ranking
that cannot be read off the plan.

**3. The roll-up states a fact, it does not rule.** The report says what the
change destroys and what cannot be verified until apply. It does not tell
somebody whether to approve. `--fail-on` is the only thing that turns a finding
into a decision, and it is off by default, because a tool that blocks by default
gets switched off on day one rather than adopted.

**4. Anything from a plan is untrusted input in HTML.** A resource address can
carry a `for_each` key chosen by whoever wrote the Terraform, and on a fork pull
request that is not somebody you trust. Everything taken from the plan is
HTML-escaped before it is written, and the style block is asserted to contain
nothing from the plan at all (`html_test.go`). The HTML report stays a single
self-contained document: inline CSS, no external stylesheet, font, image or
script.

**5. Say when something cannot be known.** "Unverifiable until apply" is a
finding, not a gap in the output. An unknown reported as an unknown is the
feature; an unknown quietly rendered as "no change" would be the bug.

## Conventions

- House style: British English, plain hyphens, **no em dashes**, no trailing
  full stops on headings. `--no-colour` is the spelling, with `--no-color`
  accepted as an alias
- `NO_COLOR` wins over `FORCE_COLOR` if both are set, because turning colour
  off should never be the setting that loses
- Flags go before the file argument
- A new finding type needs a golden test. `internal/assess/golden_test.go` is
  how a report's exact shape is pinned, so a rendering change that alters
  meaning shows up as a diff rather than as nothing

## Validating a change

```bash
go vet ./...
go test ./... -race -cover
gofmt -l .                     # must print nothing
```

CI runs those three, and two more that are easy to forget locally:

```bash
goreleaser check                # the release config, validated on every PR
```

and the composite action itself, run against `testdata/minimal.json`,
`testdata/critical.json` and `testdata/demo.json` - including an assertion that
`--fail-on critical` actually fails. Both exist because a release config and an
action that are only exercised on a tag are discovered to be broken in public,
after the tag exists.
