#!/usr/bin/env python3
"""Generate the terraken mark.

The mark is a form in Terraform's own family of stacked parallelograms,
cut across and slid out of true. Two shapes, two colours, no depiction:
something whole with part of it moved, which is what a destructive plan
does to an estate and the only idea the mark needs to carry.

It is deliberately not a chart, not a gavel, and not a picture of a plan -
all of which were tried and were either generic or said nothing the name
did not already say.

Edit this generator and re-run it. Do not hand-edit the SVGs it emits.

    python3 assets/_gen/logo.py
"""

from pathlib import Path

OUT = Path(__file__).resolve().parent.parent

# DBHQ brand tokens (brand/color/tokens.json in the dbhq repo), plus the
# red the terminal already prints for a critical finding. The displaced
# half is the one at risk, so it is the one that carries the red.
INK = "#0B0E14"
RED = "#E5484D"
PAPER = "#E6EDF3"


def para(x, y, w, h, skew, fill):
    pts = f"{x + skew},{y} {x + w + skew},{y} {x + w},{y + h} {x},{y + h}"
    return f'  <polygon points="{pts}" fill="{fill}"/>'


def mark(size=64, ink=INK):
    w, h, sk = 32, 18, 10
    body = "\n".join([
        para(11, 15, w, h, sk, ink),
        para(21, 35, w, h, sk, RED),
    ])
    return (
        f'<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 {size} {size}" '
        f'width="{size}" height="{size}" role="img" aria-label="terraken">\n'
        f"{body}\n</svg>\n"
    )


def favicon(ink=INK):
    """At 16px the skew and the gap both close up, so the offset is
    widened and the bands thickened - the displacement has to survive
    being four pixels tall."""
    w, h, sk = 30, 20, 8
    body = "\n".join([
        para(10, 12, w, h, sk, ink),
        para(22, 34, w, h, sk, RED),
    ])
    return (
        '<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 64 64" '
        'width="64" height="64" role="img" aria-label="terraken">\n'
        f"{body}\n</svg>\n"
    )


def banner(ink, width=720, height=132):
    """Mark plus wordmark for the top of the README.

    The wordmark is JetBrains Mono where it is installed, falling back
    through the platform monospace stack - a monospace wordmark because
    this is a thing you type at a prompt. No font is embedded, so the
    file stays small and dependency-free. GitHub renders a README SVG
    inside an img tag where it cannot inherit a colour, hence the separate
    light and dark variants.
    """
    m, s = 26, 0.82
    w, h, sk = 32 * s, 18 * s, 10 * s
    top = 30
    bars = "\n".join([
        para(m, top, w, h, sk, ink),
        para(m + 10 * s, top + 20 * s, w, h, sk, RED),
    ])
    tx = m + w + sk + 30
    return (
        f'<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 {width} {height}" '
        f'width="{width}" height="{height}" role="img" '
        f'aria-label="terraken - read a Terraform plan and find out what it actually does">\n'
        f"{bars}\n"
        f'  <text x="{tx:.0f}" y="{top + 26:.0f}" '
        f'font-family="JetBrains Mono, ui-monospace, SFMono-Regular, Menlo, Consolas, monospace" '
        f'font-size="38" font-weight="600" fill="{ink}" '
        f'letter-spacing="-0.5">terraken</text>\n'
        f'  <text x="{tx:.0f}" y="{top + 54:.0f}" '
        f'font-family="ui-sans-serif, system-ui, -apple-system, Segoe UI, Helvetica, Arial, sans-serif" '
        f'font-size="16" fill="{ink}" opacity="0.7">'
        f"Read a Terraform plan and find out what it actually does</text>\n"
        f"</svg>\n"
    )


def social(width=1280, height=640):
    """The card GitHub shows when a link is shared.

    GitHub generates one automatically, in a template every repository
    shares. This is the one thing a reader sees before they have decided
    whether to click, so it is worth owning rather than inheriting.

    Dark, because the tool is a terminal report and a dark card reads as
    one. Set from the same two parallelograms as everything else.
    """
    m, s = 96, 2.1
    w, h, sk = 32 * s, 18 * s, 10 * s
    top = 200
    bars = "\n".join([
        para(m, top, w, h, sk, PAPER),
        para(m + 10 * s, top + 20 * s, w, h, sk, RED),
    ])
    tx = m + w + sk + 62
    return (
        f'<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 {width} {height}" '
        f'width="{width}" height="{height}" role="img" '
        f'aria-label="terraken - read a Terraform plan and find out what it actually does">\n'
        f'  <rect width="{width}" height="{height}" fill="{INK}"/>\n'
        f"{bars}\n"
        f'  <text x="{tx:.0f}" y="{top + 56:.0f}" '
        f'font-family="JetBrains Mono, ui-monospace, SFMono-Regular, Menlo, Consolas, monospace" '
        f'font-size="78" font-weight="600" fill="{PAPER}" '
        f'letter-spacing="-1.5">terraken</text>\n'
        f'  <text x="{tx:.0f}" y="{top + 108:.0f}" '
        f'font-family="ui-sans-serif, system-ui, -apple-system, Segoe UI, Helvetica, Arial, sans-serif" '
        f'font-size="30" fill="{PAPER}" opacity="0.72">'
        f"Read a Terraform plan and find out what it actually does</text>\n"
        f'  <text x="{m}" y="{height - 74:.0f}" '
        f'font-family="ui-monospace, SFMono-Regular, Menlo, Consolas, monospace" '
        f'font-size="26" fill="{RED}">CRITICAL</text>\n'
        f'  <text x="{m + 170}" y="{height - 74:.0f}" '
        f'font-family="ui-monospace, SFMono-Regular, Menlo, Consolas, monospace" '
        f'font-size="26" fill="{PAPER}" opacity="0.85">'
        f"azurerm_postgresql_flexible_server.main</text>\n"
        f'  <text x="{m + 170}" y="{height - 38:.0f}" '
        f'font-family="ui-monospace, SFMono-Regular, Menlo, Consolas, monospace" '
        f'font-size="26" fill="{PAPER}" opacity="0.45">'
        f"destroy and create \u00b7 holds data</text>\n"
        f"</svg>\n"
    )


def main():
    OUT.mkdir(parents=True, exist_ok=True)
    (OUT / "logo.svg").write_text(mark())
    (OUT / "logo-dark.svg").write_text(mark(ink=PAPER))
    (OUT / "favicon.svg").write_text(favicon())
    (OUT / "social.svg").write_text(social())
    (OUT / "banner-light.svg").write_text(banner(INK))
    (OUT / "banner-dark.svg").write_text(banner(PAPER))
    for n in ("logo.svg", "logo-dark.svg", "favicon.svg", "social.svg",
              "banner-light.svg", "banner-dark.svg"):
        print(f"wrote assets/{n}")


if __name__ == "__main__":
    main()
