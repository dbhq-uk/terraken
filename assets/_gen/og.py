#!/usr/bin/env python3
"""Render the social card: a picture of the tool working, not a headline.

The card that shipped before this was the generic DBHQ lockup on ink, so every
share of terraken.dbhq.uk showed the practice rather than the product. What a
command-line tool has to show is its output, and this composes a real capture
of it into a 1200x630 card.

REAL OUTPUT, NOT A MOCK-UP. The terminal block is produced by running the built
binary against testdata/critical.json, the same way assets/_gen/demo.py makes
the README capture. A social card showing output the binary does not actually
produce would be the one lie on a site whose entire argument is that its
presentation can be trusted.

    go build -o /tmp/terraken ./cmd/terraken && python3 assets/_gen/og.py
"""

import re
import subprocess
import sys
from pathlib import Path

from rich.console import Console
from rich.text import Text

ROOT = Path(__file__).resolve().parent.parent.parent
OUT = ROOT / "assets"
SITE = ROOT / "site" / "public"
BIN = "/tmp/terraken"

# 74 columns is what makes testdata/critical.json come out at 13 lines, which is
# the tallest block that stays legible at 1200x630. A wider capture shrinks the
# type; a taller fixture crops.
WIDTH = 74
FIXTURE = ROOT / "testdata" / "critical.json"

INK = "#0B0E14"
PAPER = "#E9EDF4"
MUTED = "#5F6B7A"
RED = "#E5484D"

CARD_W, CARD_H = 1200, 630


def capture():
    env = {"FORCE_COLOR": "1", "COLUMNS": str(WIDTH), "PATH": "/usr/bin:/bin"}
    r = subprocess.run([BIN, str(FIXTURE)], capture_output=True, text=True, env=env)
    # 1 is the documented exit code for "findings were reported", not a failure.
    if r.returncode not in (0, 1):
        sys.exit(f"{BIN} failed ({r.returncode}): {r.stderr}")
    return r.stdout


def terminal_svg(ansi):
    """Rich's own SVG export: a terminal window with chrome, in real colour."""
    # The binary has already wrapped to COLUMNS and drawn its tree column, so
    # rich must not re-flow it. Same reasoning as demo.py.
    console = Console(record=True, width=WIDTH + 2, file=open("/dev/null", "w"))
    for line in ansi.rstrip("\n").split("\n"):
        console.print(Text.from_ansi(line), overflow="ignore", crop=False, no_wrap=True)
    return console.export_svg(title="terraken plan.json")


def use_brand_mono(svg):
    """Swap rich's hardcoded Fira Code for a face that is actually installed.

    This is not a styling preference, it is a correctness fix. Fira Code is not
    on this machine, so every glyph falls back - and the box-drawing characters
    the report draws its tree with fall back to a DIFFERENT face from the text,
    with a different advance width. The result is a card where the tree glyphs
    float left of the lines they belong to and "terraken  1 finding" closes up
    to "terraken1 finding".

    JetBrains Mono is installed, carries the box-drawing range, and is already
    the wordmark's face. assets/_gen/demo.py has the same bug in the README
    capture and needs the same fix.
    """
    return svg.replace('"Fira Code"', '"JetBrains Mono"')


def inner_and_box(svg):
    """Strip the outer <svg> wrapper, returning (inner markup, width, height).

    Rich emits a complete document. To nest it inside the card it has to become
    a fragment with its own viewBox, so the card can scale and place it as one
    object rather than reflowing anything inside it.
    """
    m = re.search(r'viewBox="0 0 ([\d.]+) ([\d.]+)"', svg)
    if not m:
        sys.exit("could not read the viewBox out of rich's SVG - its export format changed")
    w, h = float(m.group(1)), float(m.group(2))
    inner = re.sub(r"^[\s\S]*?<svg[^>]*>", "", svg, count=1)
    inner = re.sub(r"</svg>\s*$", "", inner)
    return inner, w, h


def mark(x, y, size):
    """The two sheared parallelograms, as in assets/_gen/logo.py."""
    u = size / 4
    return (
        f'<g transform="translate({x},{y})">'
        f'<path d="M {u*0.9:.2f} 0 L {size:.2f} 0 L {size - u*0.9:.2f} {u*1.4:.2f} '
        f'L 0 {u*1.4:.2f} Z" fill="{PAPER}"/>'
        f'<path d="M {u*0.35:.2f} {u*1.9:.2f} L {size - u*0.55:.2f} {u*1.9:.2f} '
        f'L {size - u*1.45:.2f} {u*3.3:.2f} L {-u*0.55:.2f} {u*3.3:.2f} Z" fill="{RED}"/>'
        f"</g>"
    )


def card(term_inner, term_w, term_h):
    # The terminal is the subject, so it gets the room. The lockup is a caption
    # in the corner rather than a headline: anyone seeing this card already has
    # the link beside it, and the output is the thing worth the pixels.
    pad = 56
    head = 104
    avail_w = CARD_W - pad * 2
    avail_h = CARD_H - head - pad
    scale = min(avail_w / term_w, avail_h / term_h)
    tw, th = term_w * scale, term_h * scale
    tx = (CARD_W - tw) / 2
    ty = head + (avail_h - th) / 2

    ms = 30
    return (
        f'<svg xmlns="http://www.w3.org/2000/svg" width="{CARD_W}" height="{CARD_H}" '
        f'viewBox="0 0 {CARD_W} {CARD_H}" role="img" '
        f'aria-label="terraken ranking a Terraform plan: one critical finding, a '
        f'database being destroyed and recreated">\n'
        f'  <rect width="{CARD_W}" height="{CARD_H}" fill="{INK}"/>\n'
        f"  {mark(pad, 40, ms)}\n"
        f'  <text x="{pad + ms + 18}" y="{40 + ms * 0.78:.0f}" '
        f'font-family="JetBrains Mono, ui-monospace, SFMono-Regular, Menlo, monospace" '
        f'font-size="30" font-weight="700" fill="{PAPER}" letter-spacing="-0.5">terraken</text>\n'
        f'  <text x="{CARD_W - pad}" y="{40 + ms * 0.72:.0f}" text-anchor="end" '
        f'font-family="Geist, Inter, system-ui, sans-serif" font-size="19" fill="{MUTED}">'
        f"Everything you can know about a change, without asking permission</text>\n"
        f'  <svg x="{tx:.1f}" y="{ty:.1f}" width="{tw:.1f}" height="{th:.1f}" '
        f'viewBox="0 0 {term_w} {term_h}" preserveAspectRatio="xMidYMid meet">'
        f"{term_inner}</svg>\n"
        f"</svg>\n"
    )


def main():
    if not Path(BIN).exists():
        sys.exit(f"{BIN} not found - run: go build -o {BIN} ./cmd/terraken")
    inner, w, h = inner_and_box(use_brand_mono(terminal_svg(capture())))
    svg = card(inner, w, h)
    (OUT / "og.svg").write_text(svg)
    print("wrote assets/og.svg")
    print(f"now: rsvg-convert -w {CARD_W} -h {CARD_H} assets/og.svg -o {SITE}/og-image.png")


if __name__ == "__main__":
    main()
