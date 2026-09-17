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

**This file is the register of what holds this line.** `design.md` and
`CONTRIBUTING.md` point here rather than each carrying a count of their own,
because three counts in three files drift apart and the drift is silent.

**The proof is generative, and it is the thing to keep working.**
`cmd/terraken/leakproof_test.go` plants credentials in every position a value
can occupy in a plan file, marks none of them sensitive, and runs every
generated plan through every output the command can produce. Before comparing
it strips whitespace, folds case, removes ANSI sequences and HTML tags and
drops backslashes, then slides a 12-character window - so a token that was
wrapped, re-indented, re-cased, split by a colour change or half printed is
still one run. `cmd/terraken/leakgen_test.go` holds the positions; adding a
place a value can live means adding one there.

Three things about it are load-bearing:

- **It iterates `render.Formats`.** That is why a renderer cannot be added
  without the proof covering it. `internal/render/formats.go` holds ONE
  registry - a map from name to writer - and `Formats` is derived from it, so
  there is nowhere to add a renderer that the proof does not immediately see.
  A list beside a switch would be two registries wearing one name.
- **Every position declares whether the tool `live`ly reads it.** A position
  nothing reads cannot leak from, so its cases prove nothing; recording that
  keeps the count honest, and
  `TestEveryPlantedPositionIsReadOrSaysItIsNot` fails the day a feature starts
  reading one. It measures the exposure detector reaching the planted value,
  not merely a finding existing, because a finding carries an address whether
  or not anything walked to a value. Eleven positions are waiting, including
  `prior_state`, `resource_drift`, `checks` and `deferred_changes`.
- **The detector has its own tests, both directions.**
  `TestTheLeakDetectorCatchesALeak` is what makes a green run evidence rather
  than an absence of evidence, and `TestTheDetectorIgnoresTheSyntaxAroundASecret`
  stops it failing a build over a PEM header, which is published and is not the
  secret.

Seven hand-written tests pin specific known cases and stay, because a named
regression is worth having beside a generated one:

- `TestSensitiveAnnotationNeverPrintsAValue`
- `TestRewritesNeverPrintAValue`
- `TestTheHeadlineNeverPrintsAValue`
- `TestReorderedNeverPrintsAValue` and `TestSecretsFixtureNeverLeaksAValue`,
  against `testdata/reordered-secrets.json`
- `TestNoDetectedValueReachesTheExposure` and
  `TestNoDetectedCredentialReachesAnyFormat`, against
  `testdata/unmarked-credentials.json`

Both of those fixtures exist only to be leaked from. If you add a renderer or a
finding type, add the equivalent test with it. A feature that needs a value in
the output is not a feature this tool can have.

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

**4. Anything from a plan is untrusted input, in every format.** This is the
SECOND GUARANTEE, and it stands beside the first rather than under it: no plan
can make a report say something other than what the plan does.

A resource address carries a `for_each` key chosen by whoever wrote the
Terraform, and on a fork pull request that is not somebody you trust. The
attack is not defacement, it is reviewer deception - a table that grew a row, a
terminal repainted to say nothing is wrong, a line reversed by a
right-to-left override. A report that can be made to lie is worse than no
report, because it is the thing being trusted.

Two layers hold it, and both are needed:

- **`internal/render/untrusted.go` sanitises the whole report** at the entry to
  every renderer. Escape sequences, carriage returns, newlines, and the Unicode
  format characters that reorder or hide text become visible escapes -
  `\x1b`, `\u202e` - rather than being dropped, because dropping them would
  make two different addresses render identically. Doing it once, over the
  report, is what makes the property hold for a renderer somebody adds later
  without reading this file.
- **Each format escapes for its own context on the way out.** `esc` for HTML,
  `cell` and `prose` for a markdown table, `codeSpan` and `fenceFor` for
  delimiters long enough that their own contents cannot close them. This layer
  cannot be shared: the dangerous character in one destination is ordinary text
  in another.

`internal/render/injection_test.go` is the proof. It renders every fragment and
every ORDERED PAIR of fragments - 1,122 payloads through every format - and
asserts the STRUCTURE of the output rather than the absence of a character:
table rows, THE NUMBER OF CELLS IN EACH ROW, `<details>` elements, severity
banners, tree connectors and the gate's verdict all have to match what a
harmless report produces. Cells matter as much as rows: a payload that adds a
pipe does not add a row, it adds a column, and GitHub discards what is past the
header's width - which is how a Notes cell lost a rename proposal while every
row count still matched. Pairs are enumerated rather than
sampled because nearly every real attack is one: a delimiter that ends the
context, then a payload that acts in the one it lands in. Random draws missed
exactly that and a sabotage run caught the miss.

`checks` in that file is keyed by format name and held against `render.Formats`,
so a renderer cannot be added without somebody deciding what "the structure is
intact" means for it.

**An output path OUTSIDE the format registry gets none of this for free**, and
one exists: `--moved` writes HCL through `assess.RenderMoved`. HCL is not
escaped, it is VALIDATED - a display escape would change the address a `moved`
block targets - so `plausibleAddress` refuses anything a real Terraform address
cannot contain, out loud, in the output. Any future output path that does not
go through `render.Write` needs its own answer and its own test.

The HTML report additionally stays a single self-contained document: inline
CSS, no external stylesheet, font, image or script, and the style block is
asserted to contain nothing from the plan at all (`html_test.go`).

**It does not rewrite the plan.** Terraken never edits somebody's artefacts; it
escapes on the way out, and what it prints says plainly what the file held.

**5. Say when something cannot be known.** "Unverifiable until apply" is a
finding, not a gap in the output. An unknown reported as an unknown is the
feature; an unknown quietly rendered as "no change" would be the bug.

The sharpest case is an operation the tool cannot read at all. `classify` ends
at `KindUnsupported` / `Unranked`, never at a no-op, and `render.verb` names
every kind explicitly so that its fallback is reached only by a kind nobody
wired up - and says so when it is. A new `Kind` needs a case in both, or it
arrives in front of a reviewer as nothing at all.

`Unranked` is the absence of a severity, not a fifth one. It stays out of
`Counts`, out of `Report.Max` and therefore out of `--fail-on`; it sorts above
critical, no `--min-level` can hide it, and `--format gate` carries it in its
own array. See "Four levels and no medium" in [`docs/design.md`](docs/design.md)
for why that split is the shape it is.

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
