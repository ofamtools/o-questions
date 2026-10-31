#!/usr/bin/env python3
"""Render six o-questions logo concepts and a provisional production logo.

Copied and adapted from o-cmds/docs/logo/build.py (concepts: 54d056b;
production/check: 7104aff), itself adapted from o-proj. The family wordmark,
palette(hue), layout and HTML samples are retained; s comes from o-slack.
q is p's bowl with its straight descender mirrored to the right. Marks use
the 8-unit grid, a 128 tile with rx 28, and the x=112 wordmark origin.

    python3 docs/logo/build.py
    python3 docs/logo/build.py --check
    python3 docs/logo/build.py --preview /tmp/oq-logo/artifacts

The default writes concept 1 to assets/ pending the owner's choice. --check
compares exact production bytes and inherited family wordmarks. --preview
writes each candidate's SVGs and rationale.txt, a self-contained concepts.html,
and concepts-sheet.svg; PNGs are added if rsvg-convert is on PATH. Preview
never changes assets/. Python standard library only, with no fonts in logos.
"""
import argparse
import colorsys
import html
import pathlib
import shutil
import subprocess
import sys
import textwrap

TOOL = "o-questions"
ASSETS = pathlib.Path(__file__).resolve().parents[2] / "assets"
R = 'stroke-linecap="round" stroke-linejoin="round"'


def palette(hue):
    """Five HSL stops: tile, deep ink, primary, highlight, pale ink.

    Hue is the sole input; saturation/lightness are fixed across candidates.
    This is a concept recipe, not a perceptually uniform colour system.
    """
    stops = (("tile", .10, .42), ("deep", .28, .70), ("pri", .53, .64),
             ("light", .76, .67), ("pale", .94, .60))
    return {name: "#" + "".join(f"{round(c * 255):02x}" for c in
            colorsys.hls_to_rgb(hue / 360, light, sat))
            for name, light, sat in stops}


# Checked o-cmds (chartreuse 72), ochazuke (green/lime), matomeru (purple),
# o-json (saffron/teal/cream) and monogatari (blue/mint/red), plus o-proj
# (cyan/teal) and oden (amber). Copper 24 is unused as a primary hue in that
# set; warm secondary colours do occur. Cyan 196 is a comparison, not unique.
HUES = {"copper": 24, "cyan": 196}
PALETTES = {name: palette(hue) for name, hue in HUES.items()}
DEFAULT_HUE = "copper"


def ink(theme, p):
    return p["pri"] if theme == "dark" else p["deep"]


def journal(theme, p):
    body = ink(theme, p)
    return (f'<rect x="32" y="24" width="72" height="80" rx="8" fill="{body}"/>'
            f'<path d="M24 40H40M24 64H40M24 88H40" stroke="{body}" stroke-width="8" {R}/>'
            f'<path d="M56 48C56 32 88 32 88 48C88 56 72 56 72 64" fill="none" stroke="{p["pale"]}" stroke-width="8" {R}/>'
            f'<circle cx="72" cy="88" r="4" fill="{p["pale"]}"/>')


def voices(theme, p):
    body = ink(theme, p)
    return (f'<path d="M24 24H88V56H48L32 72V56H24Z" fill="{body}" stroke="{body}" stroke-width="8" {R}/>'
            f'<path d="M48 72H104V104H80L64 112V104H48Z" fill="{body}" stroke="{body}" stroke-width="8" {R}/>'
            f'<path d="M40 40H72M64 88H88" stroke="{p["pale"]}" stroke-width="8" {R}/>')


def thread(theme, p):
    body = ink(theme, p)
    return (f'<path d="M32 32V96H56M32 64H48M32 32H48M56 32H96M56 64H88" fill="none" stroke="{body}" stroke-width="8" {R}/>'
            + ''.join(f'<circle cx="32" cy="{y}" r="8" fill="{body}"/>' for y in (32, 64, 96))
            + f'<rect x="56" y="80" width="48" height="32" rx="8" fill="{body}"/>'
            f'<path d="M80 88V104M72 96H88" stroke="{p["pale"]}" stroke-width="8" {R}/>')


def quotes(theme, p):
    body = ink(theme, p)
    return (''.join(f'<rect x="{x}" y="32" width="32" height="32" rx="8" fill="{body}"/>' for x in (24, 72))
            + f'<path d="M48 56V72Q48 88 32 88M96 56V72Q96 88 80 88" fill="none" stroke="{body}" stroke-width="8" {R}/>')


def seal(theme, p):
    body = ink(theme, p)
    return (f'<rect x="32" y="24" width="64" height="80" rx="8" fill="{body}"/>'
            f'<path d="M48 40H80M48 56H72" stroke="{p["pale"]}" stroke-width="8" {R}/>'
            f'<path d="M80 88V112L88 104L96 112V88Z" fill="{body}"/>'
            f'<circle cx="88" cy="88" r="16" fill="{p["pale"]}"/>'
            f'<circle cx="88" cy="88" r="8" fill="{body}"/>')


def bridge(theme, p):
    body = ink(theme, p)
    return (f'<path d="M32 80V64A32 32 0 0 1 96 64V80M24 80H104M48 40V80M80 40V80" fill="none" stroke="{body}" stroke-width="8" {R}/>'
            + ''.join(f'<rect x="{x}" y="80" width="16" height="24" rx="8" fill="{body}"/>' for x in (24, 88)))


# Data drives the entire gallery: six independent functions, one hue override.
CONCEPTS = [
    ("01-journal", "Question journal",
     "A bound notebook carries a question mark: a private place to keep the owner's questions and the answers given. "
     "The binding suggests a durable journal across sessions, while the large question mark keeps the purpose clear at 16 px. "
     "This is the provisional first concept, pending the owner's choice.", journal),
    ("02-voices", "Paired voices",
     "Two speech cards descend in reading order: the owner's words first, an agent's reply beneath. "
     "Their separate tails retain the distinction between speakers while the pair forms one conversation. "
     "At 16 px the two stepped silhouettes remain visible without requiring tiny lettering.", voices),
    ("03-thread", "Append thread",
     "A continuous spine holds three entries; the last carries a plus for the next addition. "
     "Earlier entries stay in place as the thread grows, expressing append-only answers and follow-ups rather than a replaceable summary. "
     "The nodes and bottom capsule give the mark a clear small-size structure.", thread),
    ("04-quotes", "Verbatim quotes",
     "A broad pair of quotation marks puts the owner's exact words at the centre of the identity. "
     "The same boundary also protects the wording of each recorded answer: the journal keeps what was said rather than substituting a paraphrase. "
     "Two solid blocks and their curved tails survive the 16 px reduction.", quotes),
    ("05-seal", "Source seal",
     "A recorded page carries a round source seal, suggesting the agent and session attached to every answer. "
     "The seal denotes traceable origin, not approval or correctness; the page remains the primary shape. "
     "At 16 px the offset ring and ribbon distinguish this record from the plain journal.", seal),
    ("06-bridge", "Session bridge",
     "A single arch joins two piers, a thread carried across different agents and sessions. "
     "The deck stays continuous while the supports remain distinct, just as one question can collect attributed answers from several places. "
     "The bridge silhouette offers a less literal alternative to notebooks and speech cards at 16 px.", bridge),
]
SELECTED = "01-journal"  # Provisional; change only after the owner chooses.


# --- the family wordmark -------------------------------------------------
# Each glyph is (width, path builder at x). Shapes are copied from the
# ochazuke and matomeru logos (o, c, h, a, z, u, k, e, m, t, r); i, n, v,
# p, j, y and the hyphen are drawn with the same pieces.
STROKE, GAP, START, BASE_H, DESC_H = 11, 15, 112, 96, 112


def f(v):
    s = f"{v:.1f}"
    return s[:-2] if s.endswith(".0") else s


GLYPHS = {
    "o": (40, lambda x: f"M{f(x+20)} 44a20 20 0 1 0 0.01 0"),
    "c": (34.1, lambda x: f"M{f(x+34.1)} 49.9A20 20 0 1 0 {f(x+34.1)} 78.1"),
    "h": (36, lambda x: f"M{f(x)} 18V84M{f(x)} 62a18 18 0 0 1 36 0V84"),
    "a": (40, lambda x: f"M{f(x+20)} 44a20 20 0 1 0 0.01 0M{f(x+40)} 44V84"),
    "z": (34, lambda x: f"M{f(x)} 44h34L{f(x)} 84h34"),
    "u": (36, lambda x: f"M{f(x)} 44V66a18 18 0 0 0 36 0M{f(x+36)} 44V84"),
    "k": (32, lambda x: f"M{f(x)} 18V84M{f(x+31)} 44L{f(x)} 70M{f(x+11)} 61L{f(x+32)} 84"),
    "e": (40, lambda x: f"M{f(x)} 64H{f(x+40)}A20 20 0 1 0 {f(x+34.1)} 78.1"),
    "m": (52, lambda x: f"M{f(x)} 44V84M{f(x)} 57a13 13 0 0 1 26 0V84M{f(x+26)} 57a13 13 0 0 1 26 0V84"),
    "t": (23, lambda x: f"M{f(x+7)} 26V73a11 11 0 0 0 11 11h4M{f(x)} 44H{f(x+23)}"),
    "r": (24, lambda x: f"M{f(x)} 44V84M{f(x)} 64a20 20 0 0 1 20 -20h4"),
    "i": (0, lambda x: f"M{f(x)} 44V84M{f(x)} 24h0.01"),
    "n": (36, lambda x: f"M{f(x)} 44V84M{f(x)} 62a18 18 0 0 1 36 0V84"),
    "v": (34, lambda x: f"M{f(x)} 44L{f(x+17)} 84L{f(x+34)} 44"),
    "-": (16, lambda x: f"M{f(x)} 64h16"),  # as in o-ai's logo
    # Descenders reach y 106; the hook is t's foot turned down and left.
    "p": (40, lambda x: f"M{f(x)} 44V106M{f(x+20)} 44a20 20 0 1 0 0.01 0"),
    # j is spaced like i, by its stem: its hook tucks under the letter before.
    "j": (0, lambda x: f"M{f(x)} 44V95a11 11 0 0 1 -11 11h-4M{f(x)} 24h0.01"),
    "y": (36, lambda x: f"M{f(x)} 44V66a18 18 0 0 0 36 0M{f(x+36)} 44V95a11 11 0 0 1 -11 11h-4"),
    "d": (40, lambda x: f"M{f(x+20)} 44a20 20 0 1 0 0.01 0M{f(x+40)} 18V84"),
    "s": (30, lambda x: f"M{f(x+30)} 50A15 10 0 0 0 {f(x+15)} 44A15 10 0 0 0 {f(x+15)} 64A15 10 0 0 1 {f(x+15)} 84A15 10 0 0 1 {f(x)} 78"),
    # q mirrors p's stem across its bowl; no new curve vocabulary is needed.
    "q": (40, lambda x: f"M{f(x+20)} 44a20 20 0 1 0 0.01 0M{f(x+40)} 44V106"),
}
DESCENDERS = set("pjyq")


def wordmark(name):
    x, parts = START, []
    for ch in name:
        w, d = GLYPHS[ch]
        parts.append(d(x))
        x += w + GAP
    right = x - GAP
    width = int(right + STROKE / 2 + 3.5)  # ochazuke 509.1 -> 518, matomeru 524 -> 533
    height = DESC_H if DESCENDERS & set(name) else BASE_H
    return "".join(parts), width, height


def head(w, h):
    return (f'<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 {w} {h}" width="{w}" height="{h}" '
            f'role="img" aria-label="{TOOL}">\n<title>{TOOL}</title>\n')


def icon(mark, palette):
    TILE = palette["tile"]
    return (head(128, 128) + f'<rect width="128" height="128" rx="28" fill="{TILE}"/>\n'
            f'<g>\n{mark("dark", palette)}\n</g>\n</svg>\n')


def logo(theme, mark, palette):
    TEXT = {"light": palette["tile"], "dark": palette["pale"]}
    d, w, h = wordmark(TOOL)
    # The mark's centre (64, 64) lands at (44, 48), where ochazuke's and
    # matomeru's marks sit in their logos.
    return (head(w, h) + f'<g transform="translate(-20 -16)">\n{mark(theme, palette)}\n</g>\n'
            f'<path d="{d}" fill="none" stroke="{TEXT[theme]}" stroke-width="{STROKE}" {R}/>\n</svg>\n')


def check():
    """Check production bytes and inherited family wordmarks."""
    import re
    ok = True
    for name, svg in production().items():
        path = ASSETS / name
        same = path.exists() and path.read_bytes() == svg.encode("utf-8")
        ok &= same
        print(f"{name}: {'same' if same else 'MISSING or DIFFERENT'}")
    for fam in ("ochazuke", "matomeru"):
        f = pathlib.Path.home() / f"proj/{fam}/assets/logo-light.svg"
        if not f.exists():
            print(f"skip {fam}: {f} not found")
            continue
        t = f.read_text()
        want = re.findall(r'<path d="(M[^"]*)" fill="none" stroke="#[0-9a-f]+" stroke-width="11"', t)[-1]
        want_w = int(re.search(r'viewBox="0 0 (\d+) 96"', t).group(1))
        d, w, _ = wordmark(fam)
        split = lambda s: re.sub(r"([A-Za-z])", r" \1 ", s).split()
        a, b = split(d), split(want)
        # Numbers may differ by 0.1 from rounding (ochazuke's e ends at 503.3, x + 34.2).
        near = lambda u, v: u == v or (u[0] not in "MmAaHhVvLlZz" and abs(float(u) - float(v)) < 0.15)
        same = len(a) == len(b) and all(map(near, a, b)) and w == want_w
        ok &= same
        print(f"{fam}: {'same' if same else 'DIFFERENT'} wordmark ({w} wide)")
    return ok


def production():
    p = PALETTES[DEFAULT_HUE]
    _, _, _, mark = next(c for c in CONCEPTS if c[0] == SELECTED)
    return {"icon.svg": icon(mark, p),
            **{f"logo-{theme}.svg": logo(theme, mark, p)
               for theme in ("light", "dark")}}


def variants():
    for slug, title, idea, mark in CONCEPTS:
        yield slug, title, idea, mark, DEFAULT_HUE
    slug, title, idea, mark = CONCEPTS[0]
    yield (slug + "-cyan", title + " / cyan",
           idea + " This comparison changes only the hue to cyan (196°); geometry and tonal stops are identical.",
           mark, "cyan")


def placed(svg, x, y, width):
    """Nest an SVG at a display width, preserving its aspect ratio."""
    box = svg.split('viewBox="', 1)[1].split('"', 1)[0]
    _, _, w, h = map(float, box.split())
    body = svg.split('>', 1)[1].rsplit('</svg>', 1)[0]
    return (f'<svg x="{x}" y="{y}" width="{width}" height="{width * h / w:g}" '
            f'viewBox="{box}" role="img" aria-label="{TOOL}">{body}</svg>')


def contact_sheet():
    """Vector companion to the HTML sheet; rsvg-convert can rasterise it."""
    rows = (sum(1 for _ in variants()) + 1) // 2
    parts = [head(1440, 152 + rows * 496),
             '<rect width="100%" height="100%" fill="#e9ece3"/>',
             '<g font-family="sans-serif" fill="#20251b">',
             '<text x="32" y="48" font-size="32" font-weight="bold">o-questions / logo concepts</text>',
             '<text x="32" y="80" font-size="16">Six marks + one hue comparison · copper 24° · concept 1 is provisional</text>',
             '<text x="32" y="108" font-size="14">16 / 32 / 128 px icons · light and dark wordmarks · 31 October 2026</text>']
    for i, (slug, title, idea, mark, hue) in enumerate(variants()):
        x, y = 32 + (i % 2) * 700, 136 + (i // 2) * 496
        p = PALETTES[hue]
        tile = icon(mark, p)
        parts.extend([f'<g transform="translate({x} {y})">',
                      '<rect width="676" height="472" rx="18" fill="#f2f4ed" stroke="#cbd1c1"/>',
                      f'<text x="24" y="36" font-size="22" font-weight="bold">{html.escape(title)}</text>'])
        for line, text in enumerate(textwrap.wrap(idea, 89)):
            parts.append(f'<text x="24" y="{60 + line * 16}" font-size="12">{html.escape(text)}</text>')
        for j, (theme, bg, fg) in enumerate((("light", "#f8f9f3", "#414938"),
                                            ("dark", "#10151b", "#cdd3d9"))):
            parts.extend([f'<g transform="translate({24 + j * 320} 152)">',
                          f'<rect width="308" height="272" rx="12" fill="{bg}"/>',
                          f'<g fill="{fg}" font-size="12">',
                          f'<text x="16" y="24">{theme} background</text>'])
            for size, left in ((16, 16), (32, 64), (128, 128)):
                parts.extend([placed(tile, left, 48 + (128 - size) / 2, size),
                              f'<text x="{left + size / 2:g}" y="196" text-anchor="middle">{size} px</text>'])
            parts.extend([placed(logo(theme, mark, p), 16, 216, 276), '</g></g>'])
        parts.extend([f'<text x="24" y="452" font-size="12">{slug} · {hue} {HUES[hue]}° · {p["pri"]}</text>', '</g>'])
    parts.append('</g></svg>\n')
    return '\n'.join(parts)


def preview(out):
    out.mkdir(parents=True, exist_ok=True)
    renderer = shutil.which("rsvg-convert")
    cards = []
    for slug, title, idea, mark, hue in variants():
        folder = out / slug
        folder.mkdir(exist_ok=True)
        p = PALETTES[hue]
        tile = icon(mark, p)
        logos = {theme: logo(theme, mark, p) for theme in ("light", "dark")}
        svgs = {"icon": tile, **{f"logo-{t}": s for t, s in logos.items()}}
        for name, svg in svgs.items():
            (folder / f"{name}.svg").write_text(svg, encoding="utf-8")
        (folder / "rationale.txt").write_text(f"{title}\n\n{idea}\n", encoding="utf-8")
        samples = []
        for theme, bg in (("light", "#f8f9f3"), ("dark", "#10151b")):
            icons = "".join(f'<figure><div class="sample" style="width:{size}px;height:{size}px">{tile}</div>'
                            f'<figcaption>{size} px</figcaption></figure>' for size in (16, 32, 128))
            samples.append(f'<div class="theme {theme}"><div class="theme-label">{theme} background</div>'
                           f'<div class="sizes">{icons}</div><div class="logo">{logos[theme]}</div></div>')
            if renderer:
                for size in (16, 32, 128):
                    subprocess.run([renderer, "-w", str(size), "-h", str(size), "-b", bg,
                                    "-o", str(folder / f"icon-{theme}-{size}.png"),
                                    str(folder / "icon.svg")], check=True)
                subprocess.run([renderer, "-w", "720", "-b", bg,
                                "-o", str(folder / f"logo-{theme}.png"),
                                str(folder / f"logo-{theme}.svg")], check=True)
        badge = ' <span class="badge">provisional default</span>' if slug == SELECTED else ""
        cards.append(f'<article><h2>{html.escape(title)}{badge}</h2><p class="idea">{html.escape(idea)}</p>'
                     f'<div class="pair">{"".join(samples)}</div><p class="meta">{slug} · {hue} {HUES[hue]}° · {p["pri"]}</p></article>')
    swatches = "".join(f'<span class="swatch" style="background:{value};color:{PALETTES[DEFAULT_HUE]["pale"] if key in ("tile", "deep") else PALETTES[DEFAULT_HUE]["tile"]}">{key} {value}</span>'
                       for key, value in PALETTES[DEFAULT_HUE].items())
    document = """<!doctype html>
<html lang="en"><meta charset="utf-8"><meta name="viewport" content="width=device-width, initial-scale=1">
<title>o-questions / logo concepts</title>
<style>
*{box-sizing:border-box}body{margin:0;background:#e9ece3;color:#20251b;font:16px/1.5 system-ui,sans-serif}
main{max-width:1440px;margin:auto;padding:48px 32px}header{max-width:920px;margin-bottom:32px}
h1{font-size:clamp(32px,5vw,56px);line-height:1.1;letter-spacing:-.04em;margin:12px 0 24px}
h2{font-size:23px;letter-spacing:-.02em;margin:0}.eyebrow,.meta,.theme-label,figcaption{font:12px/1.5 ui-monospace,monospace}
.eyebrow{text-transform:uppercase;letter-spacing:.16em}.intro{font-size:18px}.swatch{display:inline-block;padding:8px 12px;border-radius:6px;margin:0 6px 6px 0;font:12px ui-monospace,monospace}
.gallery{display:grid;gap:28px}article{border:1px solid #cbd1c1;border-radius:18px;padding:24px;background:#f2f4ed;break-inside:avoid}
.idea{margin:8px 0 20px}.pair{display:grid;grid-template-columns:1fr 1fr;gap:12px}.theme{padding:24px;border-radius:12px;min-width:0}
.light{background:#f8f9f3;color:#414938}.dark{background:#10151b;color:#cdd3d9}.theme-label{text-transform:uppercase;letter-spacing:.08em}
.sizes{display:flex;align-items:center;gap:32px;min-height:190px}figure{margin:0;text-align:center}.sample{margin:auto}.sample svg{display:block;width:100%;height:100%}
figcaption{margin-top:12px}.logo{margin-top:20px;max-width:480px}.logo svg{display:block;width:100%;height:auto}
.meta{margin-bottom:0;color:#606954}.badge{display:inline-block;font:11px ui-monospace,monospace;vertical-align:middle;text-transform:uppercase;background:#edc8ae;padding:5px 8px;border-radius:20px;margin-left:8px}
footer{padding:28px 0;color:#606954;max-width:850px}@media(max-width:760px){main{padding:28px 16px}.pair{grid-template-columns:1fr}article{padding:16px}.sizes{gap:24px}}
@media print{body{background:white}main{padding:0}article{border-radius:0}.theme{-webkit-print-color-adjust:exact;print-color-adjust:exact}}
</style><main><header><div class="eyebrow">o-family / concepts first / 31 October 2026</div>
<h1>Your questions.<br>The words that followed.</h1>
<p class="intro">Six directions for o-questions: a private journal of the owner's questions and the answers given, kept verbatim in attributed, append-only threads across agents and sessions.</p>
<p>Copper (24°) is unused as a primary hue in the inspected o-cmds (chartreuse 72°), ochazuke (green/lime), matomeru (purple), o-json (saffron/teal/cream) and monogatari (blue/mint/red) palettes. Oden has amber and warm accents; o-proj has cyan/teal. Copper keeps the fixed family tonal recipe. Cyan (196°) is a comparison on concept 1, not a claim of hue exclusivity.</p>
<div>SWATCHES</div><p>Each tile is 128 × 128 with a 28-unit corner. Marks follow an 8-unit grid; samples below are actual 16, 32 and 128 CSS pixels. Logos retain the family's hand-drawn monoline wordmark, with the mark at x 0–88 and lettering from x 112.</p>
</header><div class="gallery">CARDS</div><footer>The owner has not selected a logo. Concept 1 (Question journal / copper) is the provisional assets/ default. Preview generation does not change those assets. All SVGs and styling are inline; this sheet needs no network or adjacent files. The source seal denotes provenance, not verification or approval.</footer></main></html>
""".replace("SWATCHES", swatches).replace("CARDS", "".join(cards))
    (out / "concepts.html").write_text(document, encoding="utf-8")
    sheet = out / "concepts-sheet.svg"
    sheet.write_text(contact_sheet(), encoding="utf-8")
    if renderer:
        subprocess.run([renderer, "-o", str(out / "concepts-sheet.png"), str(sheet)], check=True)
    print(out / "concepts.html")
    print("PNG previews rendered" if renderer else "rsvg-convert unavailable; SVG and HTML only")


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    mode = parser.add_mutually_exclusive_group()
    mode.add_argument("--check", action="store_true", help="compare production assets and inherited family wordmarks")
    mode.add_argument("--preview", type=pathlib.Path, metavar="DIR", help="render the original concept sheet")
    args = parser.parse_args()
    if args.check:
        sys.exit(0 if check() else 1)
    if args.preview is not None:
        preview(args.preview)
        return
    ASSETS.mkdir(exist_ok=True)
    for name, svg in production().items():
        path = ASSETS / name
        path.write_bytes(svg.encode("utf-8"))
        print(path)


if __name__ == "__main__":
    main()
