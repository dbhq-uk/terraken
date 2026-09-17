#!/usr/bin/env python3
"""Capture the tool's real output WITH its colours, as HTML for the site.

The site used to carry these samples as plain text, captured without
FORCE_COLOR, so the page showed a monochrome version of a report that is
colour-coded in the terminal - and the colour is not decoration here. Red means
this change can destroy something. Showing the report without it removes the
one thing the reader is supposed to take from a glance.

WHAT IT EMITS. Class names, never inline styles, so the colours are the site's
tokens rather than a second set of hex values living in a data file. The five
codes below are the whole vocabulary - internal/render/render.go defines them
and nothing else paints anything:

    \\x1b[0m     reset
    \\x1b[1m     bold      the tool's name, a resource address
    \\x1b[31;1m  red       critical
    \\x1b[33;1m  amber     high
    \\x1b[34m    blue      low
    \\x1b[90m    grey      rules, tree glyphs, everything secondary

An unrecognised code is a failure rather than something to pass through: it
means the renderer gained a colour and this needs a class for it.

    go build -o /tmp/terraken ./cmd/terraken && python3 assets/_gen/samples.py
"""

import html
import re
import subprocess
import sys
from pathlib import Path

ROOT = Path(__file__).resolve().parent.parent.parent
BIN = "/tmp/terraken"

CLASS = {
    "0": None,          # reset - closes the open span
    "1": "t-b",         # bold
    "31;1": "t-crit",
    "33;1": "t-high",
    "34": "t-low",
    "90": "t-dim",
}

ANSI = re.compile(r"\x1b\[([0-9;]*)m")

# name -> (argv, columns). Kept here rather than in a comment in site.ts, so
# the command and the capture cannot drift apart. 78 is the width the in-body samples are set to and
# what the guides have always used.
#
# heroCritical is the SAME report at 64, for the hero's narrower column. The
# tool sets its report to the terminal width between 60 and 100 columns, so a
# 64-column capture is a real thing it prints rather than a squeezed copy of
# the 78 - which is why this is a second capture and not CSS scaling the first.
SAMPLES = {
    "critical": (["testdata/critical.json"], 78),
    "heroCritical": (["testdata/critical.json"], 64),
    "missedMove": (["testdata/rename-no-moved.json"], 78),
    "rewritten": (["testdata/written-differently.json"], 78),
    "minLevel": (["--min-level", "high", "testdata/demo.json"], 78),
    # Filtered to critical, so every finding is held back and the credentials
    # block is all that remains. That is the point of the capture as well as a
    # way of keeping it short: the block is not a finding and not a level, so
    # no display filter reaches it.
    #
    # Every value in this fixture is fabricated - see
    # TestNoDetectedCredentialReachesAnyFormat, which asserts none of them can
    # reach any output, this capture included.
    "credentials": (["--min-level", "critical", "testdata/unmarked-credentials.json"], 78),
}


def capture(args, cols):
    env = {"FORCE_COLOR": "1", "COLUMNS": str(cols), "PATH": "/usr/bin:/bin"}
    r = subprocess.run([BIN, *args], capture_output=True, text=True, cwd=ROOT, env=env)
    if r.returncode not in (0, 1):
        sys.exit(f"{BIN} {' '.join(args)} failed ({r.returncode}): {r.stderr}")
    return r.stdout.rstrip("\n")


def to_html(ansi):
    """ANSI to spans. Escapes first, so no capture can inject markup."""
    out = []
    open_span = False
    pos = 0
    for m in ANSI.finditer(ansi):
        out.append(html.escape(ansi[pos:m.start()]))
        pos = m.end()
        code = m.group(1) or "0"
        if code not in CLASS:
            sys.exit(f"unknown ANSI code {code!r} - the renderer gained a colour")
        if open_span:
            out.append("</span>")
            open_span = False
        cls = CLASS[code]
        if cls:
            out.append(f'<span class="{cls}">')
            open_span = True
    out.append(html.escape(ansi[pos:]))
    if open_span:
        out.append("</span>")
    return "".join(out)


HEADER = '''// GENERATED - do not edit. assets/_gen/samples.py in this repository.
//
// Real output from the built binary, run with FORCE_COLOR against the fixtures
// in testdata/, with its ANSI converted to spans. The colour is not decoration:
// red means this change can destroy something, and the site carried a
// monochrome capture until 16 Sep 2026, which threw that away.
//
// A .ts module rather than .html files imported with ?raw. Vite resolves ?raw
// happily; tests/content.test.mjs imports this through plain Node, which
// cannot load a .html file at all and fails at module load with
// ERR_UNKNOWN_FILE_EXTENSION before a single assertion runs.
//
// Regenerate whenever the renderer changes:
//   go build -o /tmp/terraken ./cmd/terraken && python3 assets/_gen/samples.py

'''


def main():
    if not Path(BIN).exists():
        sys.exit(f"{BIN} not found - run: go build -o {BIN} ./cmd/terraken")
    parts = [HEADER]
    for name, (args, cols) in SAMPLES.items():
        h = to_html(capture(args, cols))
        print(f"  {name}: {cols} cols, {h.count(chr(10)) + 1} lines, {h.count("<span")} spans")
        # A template literal, so the newlines stay readable in the diff. The
        # capture is already HTML-escaped, and a backslash or backtick in it
        # would break out of the literal, so both are escaped here.
        lit = h.replace("\\", "\\\\").replace("`", "\\`").replace("${", "\\${")
        parts.append(f"export const {name} = `{lit}`;\n\n")
    out = ROOT / "site" / "src" / "lib" / "samples.ts"
    out.write_text("".join(parts).rstrip() + "\n")
    print(f"  wrote {out.relative_to(ROOT)}")


if __name__ == "__main__":
    main()
