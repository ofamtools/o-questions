# o-questions logo concepts

The owner has not selected a logo. `SELECTED = "01-journal"` and
`DEFAULT_HUE = "copper"` in `build.py` provisionally generate the three
production SVGs in `assets/`. Selection is data: choose a concept slug and hue
there, then regenerate. The preview always retains all six concepts and the
cyan comparison on concept 1.

| Concept | Pitch |
| --- | --- |
| 01 Question journal | A private notebook for questions and the words that followed. |
| 02 Paired voices | Owner and agent retain distinct voices in one conversation. |
| 03 Append thread | Entries stay in place as new answers and follow-ups join the thread. |
| 04 Verbatim quotes | Exact wording is the central promise. |
| 05 Source seal | A recorded answer keeps its agent and session provenance. |
| 06 Session bridge | One question spans several agents and sessions. |

`--preview DIR` produces one directory per concept (and the hue variant), each
with `icon.svg`, `logo-light.svg`, `logo-dark.svg`, and a one-paragraph
`rationale.txt`. `concepts.html` embeds every SVG and its CSS; it works offline
without adjacent files. Both it and `concepts-sheet.svg` compare 16, 32, and
128 px icons with light/dark wordmarks side by side. When `rsvg-convert` is
available, the same preview also writes individual PNGs and
`concepts-sheet.png`. SVG/HTML generation needs only the Python standard
library. `--preview` does not write production assets; `--check` compares their
exact bytes and checks the inherited ochazuke/matomeru wordmarks.

The initial review artifacts live at `/tmp/oq-logo/artifacts/`. Preview output
contains no timestamps, absolute paths, random identifiers or network assets.
PNG reproducibility assumes the same renderer and system fonts; SVG logos
contain only geometry and have no font dependency.

## Reused family construction

`build.py` copies and adapts `~/proj/o-cmds/docs/logo/build.py`: the concept
version at `54d056b` and production/check flow at `7104aff`. That builder came
from `o-proj`, with additional glyphs from oden and o-slack. The palette's five
HSL stops, hand-drawn monoline wordmark, 11-unit stroke, 15-unit letter gap,
x=112 origin, 128-unit tile with radius 28, and 8-unit mark grid are retained.
Question journal adapts o-cmds' notebook silhouette. The per-concept folders
and `rationale.txt` convention come from the matomeru, monogatari and o-json
generators in `~/hub/logos/`. The vector contact sheet extends o-cmds' HTML
comparison so `rsvg-convert` can also make a PNG sheet without another library.

Wordmark glyphs `o`, `u`, `e`, and `t` come through the o-cmds/o-proj table
from ochazuke/matomeru. The hyphen, `i`, and `n` use the existing o-proj family
pieces. `s` is the two-half-ellipse glyph from o-slack, already in o-cmds.
No `q` was present in the inspected o-slack, o-cmds, o-proj, oden or o-gh
builders. The new `q` mirrors `p`'s straight descender onto the right of the
same 40-unit bowl, down to y=106. The wordmark therefore uses the existing
112-unit descender canvas. No shared glyph library or sibling repo changes.

## Hue inventory

Copper at 24 degrees is the provisional primary (`#d4783a`), with cyan at
196 degrees (`#3aabd4`) for comparison. Both use the unmodified family
`palette(hue)` tonal recipe. Inspected palettes: o-cmds chartreuse 72;
ochazuke green/lime; matomeru purple with pink accents; o-json saffron,
teal and cream; monogatari blue/mint/red. Copper is unused as a primary in
that set. Oden's amber and warm ingredient accents and o-proj's cyan/teal
were also checked; this is a distinct primary choice, not a claim that no
family illustration contains orange.
