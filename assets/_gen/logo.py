#!/usr/bin/env python3
"""Generate the terraverdict mark.

The mark is the tool's own risk scale: four bars ascending info, low, high,
critical. It is not decoration - a reader who has seen one report already
knows what the shape means, and the colours are the same ones the terminal
prints.

Edit this generator and re-run it. Do not hand-edit the SVGs it emits.

    python3 assets/_gen/logo.py
"""

from pathlib import Path

OUT = Path(__file__).resolve().parent.parent

# DBHQ brand tokens (brand/color/tokens.json in the dbhq repo). The
# critical accent is the one colour not in the house palette: a risk scale
# has to end in a warning, and it matches the red the terminal already
# uses for a critical finding.
AQUA = "#86ECE0"
TURQUOISE = "#2AD4C5"
BLUE = "#2B6BF3"
CRITICAL = "#E5484D"
INK = "#0B0E14"

LEVELS = [
    ("info", AQUA, 0.34),
    ("low", TURQUOISE, 0.55),
    ("high", BLUE, 0.78),
    ("critical", CRITICAL, 1.00),
]


def mark(size=64, pad=6, gap=3.2):
    """The square mark: four bottom-aligned ascending bars."""
    inner = size - pad * 2
    bar_w = (inner - gap * (len(LEVELS) - 1)) / len(LEVELS)
    radius = bar_w / 2
    floor = size - pad

    bars = []
    for i, (name, colour, frac) in enumerate(LEVELS):
        h = inner * frac
        x = pad + i * (bar_w + gap)
        y = floor - h
        bars.append(
            f'  <rect x="{x:.2f}" y="{y:.2f}" width="{bar_w:.2f}" '
            f'height="{h:.2f}" rx="{radius:.2f}" fill="{colour}">'
            f"<title>{name}</title></rect>"
        )

    return (
        f'<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 {size} {size}" '
        f'width="{size}" height="{size}" role="img" '
        f'aria-label="terraverdict">\n'
        + "\n".join(bars)
        + "\n</svg>\n"
    )


def favicon():
    """At 16px the four bars blur together, so drop to the two that carry
    the meaning - the tallest and the one before it."""
    size, pad, gap = 32, 3, 2.5
    inner = size - pad * 2
    bar_w = (inner - gap) / 2
    radius = bar_w / 2
    floor = size - pad

    bars = []
    for i, (colour, frac) in enumerate([(BLUE, 0.62), (CRITICAL, 1.0)]):
        h = inner * frac
        x = pad + i * (bar_w + gap)
        bars.append(
            f'  <rect x="{x:.2f}" y="{floor - h:.2f}" width="{bar_w:.2f}" '
            f'height="{h:.2f}" rx="{radius:.2f}" fill="{colour}"/>'
        )

    return (
        f'<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 {size} {size}" '
        f'width="{size}" height="{size}" role="img" aria-label="terraverdict">\n'
        + "\n".join(bars)
        + "\n</svg>\n"
    )


def banner(width=760, height=150):
    """Mark plus wordmark for the top of the README.

    The wordmark is set in a system stack rather than an embedded font, so
    the file stays small and dependency-free. currentColor would not work
    here because GitHub renders a README SVG inside an img tag, where it
    cannot inherit - hence the explicit light and dark variants.
    """

    def build(ink):
        m = 28
        scale = 0.92
        bars = []
        inner, gap = 64 * scale, 3.2 * scale
        bar_w = (inner - gap * 3) / 4
        radius = bar_w / 2
        floor = m + inner
        for i, (_, colour, frac) in enumerate(LEVELS):
            h = inner * frac
            x = m + i * (bar_w + gap)
            bars.append(
                f'  <rect x="{x:.2f}" y="{floor - h:.2f}" width="{bar_w:.2f}" '
                f'height="{h:.2f}" rx="{radius:.2f}" fill="{colour}"/>'
            )
        text_x = m + inner + 26
        return (
            f'<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 {width} {height}" '
            f'width="{width}" height="{height}" role="img" '
            f'aria-label="terraverdict - read a Terraform plan and find out what it actually does">\n'
            + "\n".join(bars)
            + f'\n  <text x="{text_x}" y="{m + 44}" '
            f'font-family="ui-monospace, SFMono-Regular, Menlo, Consolas, monospace" '
            f'font-size="42" font-weight="600" fill="{ink}" '
            f'letter-spacing="-0.5">terraverdict</text>\n'
            f'  <text x="{text_x}" y="{m + 76}" '
            f'font-family="ui-sans-serif, system-ui, -apple-system, Segoe UI, Helvetica, Arial, sans-serif" '
            f'font-size="17" fill="{ink}" opacity="0.72">'
            f"Read a Terraform plan and find out what it actually does</text>\n"
            f"</svg>\n"
        )

    return build(INK), build("#E6EDF3")


def main():
    OUT.mkdir(parents=True, exist_ok=True)
    (OUT / "logo.svg").write_text(mark())
    (OUT / "favicon.svg").write_text(favicon())
    light, dark = banner()
    (OUT / "banner-light.svg").write_text(light)
    (OUT / "banner-dark.svg").write_text(dark)
    for name in ("logo.svg", "favicon.svg", "banner-light.svg", "banner-dark.svg"):
        print(f"wrote assets/{name}")


if __name__ == "__main__":
    main()
