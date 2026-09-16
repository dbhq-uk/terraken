# terraken.dbhq.uk

The site behind [terraken.dbhq.uk](https://terraken.dbhq.uk) - seven pages
about [Terraken](https://github.com/dbhq-uk/terraken), the command-line tool
that reads a Terraform or OpenTofu plan and ranks the change by how much damage
it can do.

The tool itself lives in its own public repository. This directory is the site
that describes it, and nothing else.

This is the **third** Astro project in this repo. [`../website/`](../website/)
is dbhq.uk and [`../skills-site/`](../skills-site/) is skills.dbhq.uk. Three
sites, three build outputs, three Cloudflare Pages projects, one repo.

## Why it is shaped the way it is

**It is a search site, not a brochure.** The information architecture comes
from measured demand rather than from how the tool is described internally.
Worldwide Google Ads volume, English, measured 16 September 2026 and recorded in
full in [`../docs/research/terraken-seo-worldwide.md`](../docs/research/terraken-seo-worldwide.md):

| Term | Searches a month |
|---|---|
| terraform moved block | 1,300 |
| terraform moved | 1,300 |
| terraform taint | 1,300 |
| terraform state rm | 1,000 |
| terraform replace | 880 |
| terraform state mv | 590 |
| terraform untaint | 210 |
| terraform rename resource | 210 |
| terraform forces replacement | 90 |
| terraform force replacement | 90 |

And, measured the same way and returning nothing at all: `terraform plan
review`, `terraform plan risk`, `review terraform plan`, `terraform plan noise`,
`terraform plan shows changes but nothing changed`, `terraform state mv vs
moved` - **all zero**.

So the anchor of this site is the moved-block cluster, and not "plan review",
which is how the tool is positioned internally and which nobody types.
`tests/content.test.mjs` asserts that decision rather than trusting it to
survive a rewrite: the anchor page has to keep the phrase in its title and its
H1, it has to keep the top sitemap priority, and nothing else is allowed to hold
that priority beside it.

| Page | Search intent |
|---|---|
| [/](https://terraken.dbhq.uk/) | The brand term. What the tool is, what it prints, what it will not do, how to install it |
| [/terraform-moved-block/](https://terraken.dbhq.uk/terraform-moved-block/) | `terraform moved block`, `terraform moved`, `moved block terraform` |
| [/terraform-rename-resource/](https://terraken.dbhq.uk/terraform-rename-resource/) | `terraform rename resource`, `terraform rename resource without destroying`, `terraform refactoring` |
| [/terraform-state-mv/](https://terraken.dbhq.uk/terraform-state-mv/) | `terraform state mv`, `terraform state rm`, `terraform removed block` |
| [/terraform-taint/](https://terraken.dbhq.uk/terraform-taint/) | `terraform taint`, `terraform untaint`, `terraform apply -replace` |
| [/terraform-forces-replacement/](https://terraken.dbhq.uk/terraform-forces-replacement/) | `terraform forces replacement`, `terraform force replacement`, `terraform replace`, `terraform create_before_destroy` |
| [/docs/](https://terraken.dbhq.uk/docs/) | Reference, for somebody who already has the tool |

**Terms deliberately not targeted**, and they are the bigger ones: `terraform
import` (5,400), `terraform destroy` (2,400), `terraform lifecycle` (1,900) and
`terraform for_each` (1,600). The tool does not import, the destroy intent is
mostly "how do I" rather than "why is it", and the last two are language
features rather than problems Terraken solves. Ranking for a query the tool
cannot help with is worse than not ranking.

**The five guide pages are guides, not landing pages.** They are written to be
the best answer on the internet to their query, complete and correct whether or
not the reader ever installs anything. Terraken appears in one section near the
end of each, because it detects the case, and it is not the subject. The suite
caps the tool's name at under one per cent of the words on those pages - it runs
at about 0.3 on every one of them - and fails a page under 1,200 words, because
a guide that turns into an advert stops ranking and deserves to.

**Every guide has a stopping point.** Each one says what a correct plan looks
like afterwards, what a still-wrong one looks like, and what to do about a wrong
one, under a heading that names the check. Somebody mid-incident does not need
to know whether their syntax is valid - Terraform tells them that - they need to
know whether the destroy has gone, and a block with a typo in an address is
valid configuration that does nothing at all. `tests/content.test.mjs` asserts
all three parts on all five guides.

## Every technical claim is true of the shipped tool

Not of its README, which is a document that can fall behind, and not of memory.
Flags came out of `cmd/terraken/main.go`, levels out of
`internal/assess/level.go`, annotation codes out of `internal/assess/finding.go`,
the detector's thresholds out of `internal/assess/moved.go`, and the Action's
inputs out of `action.yml`. Exit codes were checked by running the binary.

**The terminal output on these pages is real.** Every Terraken sample was
produced by running the built binary against a fixture in the tool's own
`testdata/`, and the exact commands are recorded beside the samples in
[`src/lib/site.ts`](src/lib/site.ts). Every abridged `terraform plan` excerpt
was produced the same way: run against Terraform 1.16.1, read, then abridged
onto the page's running example. The header lines, the ordering symbols and
their legends, the command output, the `removed` block's warning and the
`prevent_destroy` error are all quoted from a run rather than from memory.

Do not hand write one and do not tidy one up: a sample that does not match what
the tool prints is the first thing a reader will check.

**The twelve open issues on `dbhq-uk/terraken` are future work and appear
nowhere on this site.** A documented capability that does not exist is worse
than an undocumented one that does, so the suite carries a list of phrases
naming them and fails the build on any of them. Remove an entry from that list
in the same commit that ships the capability, not before.

## The three contracts

The tool's trust proposition, and the reason it is safe to point at a plan
nobody has vetted:

1. **It takes a file, and runs nothing.** Never runs `terraform`, never reads a
   cloud credential, never makes a network call, never applies anything.
2. **It never prints an attribute's value, in any format.** Not masked, not
   redacted, not truncated.
3. **It is deterministic, with no model in the loop.**

Each is held by a test in the tool's own repository - `AGENTS.md` there calls
them the constraints that must not be broken - and each is pinned here, phrase
by phrase, against the built pages. The change that erodes one of these does not
look like vandalism, it looks like a copy-editing pass, and nothing else in the
suite can tell the two apart. Read the note above `PUBLISHED_CONTRACTS` in
[`tests/content.test.mjs`](tests/content.test.mjs) before changing a word of
them.

## Working on it

```bash
cd terraken-site
npm install
npm run dev        # http://100.115.72.85:4332 (Tailscale, not localhost)
npm test           # builds first, then asserts the contract
```

`npm test` runs the build via the `pretest` script, so the assertions always run
against current output. What the 39 tests cover:

- seven indexed pages, a real `404.html` so Cloudflare Pages returns a 404
  status, and the edge files; and the nav list agrees with them, because the
  footer renders that list and it is what keeps every page one click from every
  other
- no em dashes or en dashes, no heading with a trailing full stop, British
  spelling
- DBHQ never written as "we" - DBHQ is one person
- the standing denylist from
  [`../docs/website/content-plan.md`](../docs/website/content-plan.md), and no
  DBHQ price
- every title 50-60 characters and unique; every description written per page,
  60-155 characters and unique
- one JSON-LD graph per page that parses, with **one node per page** and the
  publisher keeping dbhq.uk's own canonical `@id`
- every published FAQ question and answer is on the page in the words the
  structured data claims
- the documented flags, levels, annotation codes and Action inputs match pinned
  lists taken from the tool's source
- no unshipped capability, and no claim that the tool writes to a configuration
- every guide keeps the phrase it was built around in its title and its H1, and
  says how to tell whether the fix worked
- a guide's contents list points only at top-level sections, so it does not fill
  the first screen with navigation
- every internal link trailing-slashed, resolving, and every fragment landing on
  a real id; no page a dead end
- every external link same-tab, glyphed and annotated, and no inline element
  glued to the word beside it. Astro eats the whitespace between a text node and
  an element that starts on the next source line, and the fix is a literal
  `&#32;`. The test watched `<a>` alone until 16 Sep 2026 and found four live
  instances the day it was written; widened to `<code>`, `<strong>` and `<em>`
  it found seventy-five more. `<code>` was the worst of them, because the chip's
  own padding leaves a gap that looks like a space and is not one
- the Content-Security-Policy is still `script-src 'self'` with no inline script
  anywhere in the output

The guards were proved by mutation: an em dash pasted into a built page, a
softened contract, an unshipped capability and a dropped trailing slash each
fail the suite. A guard that cannot fail is not a guard.

### Where the copy comes from

[`src/lib/site.ts`](src/lib/site.ts) holds the structured copy - the contracts,
the flags, the levels, the annotation codes, the FAQs and the captured output.
The long-form prose on the five guide pages lives in the pages themselves,
because it is editorial rather than data.

Nothing on this site is shared with `../website/` or `../skills-site/`, and it
should stay that way. `skills-site/src/lib/skills.ts` is shared with dbhq.uk and
[`../skills-site/tsconfig.json`](../skills-site/tsconfig.json) has to write its
compiler options out by hand as a result. This project imports nothing from
outside its own directory, which is why its `tsconfig.json` can simply extend
the Astro preset.

## Deploying

Not yet wired. The Cloudflare Pages project, the custom domain, the DNS record
and the deploy workflow are Dan's to set up, alongside `../infra/`. When they
exist, the rule is the same as the other two sites: **merging to `main` is the
deploy**, the workflow purges the edge cache afterwards, and the purge is not
optional. See [`../docs/reference/hosting.md`](../docs/reference/hosting.md).

## Licence

Terraken is MIT licensed in its own public repository. This directory is the
site copy and lives in a private repo; the MIT badge on these pages refers to
the tool, not to this site.

Terraform and OpenTofu are the trade marks of their respective owners. This site
is not affiliated with either project.

## Also from DBHQ

- [dbhq.uk](https://dbhq.uk) - the practice that publishes this
- [skills.dbhq.uk](https://skills.dbhq.uk) - free, open-source agent skills for Claude Code and Codex
- [heliograph.dbhq.uk](https://heliograph.dbhq.uk) - remote, captured, auditable execution on a machine you cannot log into
- [bbs.dbhq.uk](https://bbs.dbhq.uk) - browse the live web as an ANSI bulletin board
- [modem.dbhq.uk](https://modem.dbhq.uk) - hear a real Bell 103 dial-up handshake, phase by phase
