#!/usr/bin/env python3
"""Title-case the product name in prose, and only in prose.

Dan, 16 Sep 2026: the product is **Terraken**, one word, capital T. The K is
never capitalised - not terraKen, not TerraKen - and the rest of the name is
never capitalised either.

WHAT MUST STAY LOWERCASE, which is why this is not a sed. Four separate things
share the string and every one of them would break if it were capitalised:

  the command          terraken plan.json
  the module path      github.com/dbhq-uk/terraken/cmd/terraken
  the hostname         terraken.dbhq.uk, terraken.pages.dev
  the logotype         the h1 and the wordmark, which are drawings

and a fifth that would merely become a lie: the captured terminal output on the
site was produced by running the binary, and the binary prints its own command
name. Rewriting it would make a real capture into a fake one.

So this masks everything that is an identifier, a URL, a code span, a fenced
block or a captured sample, title-cases what is left, and puts the masked spans
back untouched. Run it, then read the diff - it is meant to be checked, not
trusted.

    python3 assets/_gen/casing.py <file> [<file> ...]
"""

import re
import sys

# Anything matching one of these is an identifier rather than the product name,
# and is put back exactly as it was found.
PROTECT = [
    r"```[\s\S]*?```",                # fenced block
    # An INDENTED code block - four spaces at the start of a line. This was
    # missing on the first run and it capitalised two real commands in the
    # README's "In CI" section, which is markdown's other code-block syntax and
    # the easier one to forget. Matched before the single-line patterns so a
    # whole block is taken in one piece.
    #
    # Written as (?:^|\n) rather than (?m)^ deliberately: these patterns are
    # joined with | into one expression, and Python rejects an inline global
    # flag that is not at the very start of it. The (?m) form compiles alone
    # and then fails the moment it is joined, which is a trap worth naming.
    r"(?:^|\n)(?: {4,}|\t)[^\n]*",
    r"`[^`]*`",                       # inline code
    r"\$\{[^}]*\}",                   # template interpolation
    r"https?://[^\s\"'<>)\]]+",       # any URL
    r"[\w.-]*terraken[\w./-]*\.(?:uk|dev|com|io)\b",  # hostnames
    r"\bdbhq-uk/terraken\b",          # repo
    # ANY path segment before or after the name - cmd/terraken, infra/terraken,
    # terraken/site, ~/dbhq-uk/terraken. Listing them one at a time missed
    # infra/terraken and turned a heading into "# infra/Terraken", so this is
    # deliberately general: a slash on either side means it is a path.
    r"[\w.~-]+/terraken\b",
    r"\bterraken/[\w./-]+",
    r"\bterraken(?:-[a-z]+)*\.(?:svg|png|ico|json|go|mjs|ts)\b",  # filenames
    # The command being invoked, in any of the forms that actually occur: with
    # a flag, with a fixture, with a plan, or reading stdin. The flag case was
    # also missing on the first run.
    r"\bterraken\s+(?:--[a-z-]+|-[a-z]\b|plan\.json|testdata/|-\s|-$)",
    r"\|\s*terraken\b",               # piped into the command
    r"\bterraken\s+and\s+tken\b",     # the two binary names
    r"\bterraken\s{2,}\d+\s+finding", # a captured report header
]
PROTECT_RE = re.compile("|".join(PROTECT))

# The product named in a sentence. Lower-case t, word boundary both ends, and
# NOT immediately followed by something that makes it an identifier.
PROSE_RE = re.compile(r"\bterraken\b(?![\w./-])")


def convert(text):
    saved = []

    def stash(m):
        saved.append(m.group(0))
        return f"\x00{len(saved) - 1}\x00"

    masked = PROTECT_RE.sub(stash, text)
    out = PROSE_RE.sub("Terraken", masked)
    return re.sub(r"\x00(\d+)\x00", lambda m: saved[int(m.group(1))], out)


def main(paths):
    for p in paths:
        with open(p, encoding="utf-8") as f:
            before = f.read()
        after = convert(before)
        if before == after:
            print(f"  unchanged  {p}")
            continue
        with open(p, "w", encoding="utf-8") as f:
            f.write(after)
        n = sum(1 for _ in re.finditer(r"\bTerraken\b", after)) - sum(
            1 for _ in re.finditer(r"\bTerraken\b", before)
        )
        print(f"  {n:>4} fixed  {p}")


if __name__ == "__main__":
    if len(sys.argv) < 2:
        sys.exit(__doc__)
    main(sys.argv[1:])
