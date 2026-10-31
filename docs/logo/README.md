# o-questions logo concepts

The owner chose **02 Paired voices** as the direction for round 2 and asked
for Japanese-themed variations in the earlier family's idiom. The final mark
and colour are still open. `SELECTED = "01-journal"` and
`DEFAULT_HUE = "copper"` in `build.py` keep the three production SVGs in
`assets/` provisional and byte-for-byte unchanged. `REFERENCE` names the
round-2 base idea. Round-1 marks remain in `ROUND1` for later selection;
production selection can use a slug from either round.

## Round 2

The preview has seven cards, with copper and ai-indigo versions on each:

| Concept | Japanese | Pitch |
| --- | --- | --- |
| 02 Paired voices / reference | — | Owner and agent retain distinct voices in one conversation. |
| 07 Fukidashi taiwa | 吹き出し 対話 | A soft manga question balloon and an angular answering balloon, each with its own tail. |
| 08 Mondō | 問答 | Two balloons echo the question's gate and the answer's roof and mouth. |
| 09 Kataribe | 語り部 | A voice line carries the owner's words onward into a separate reply. |
| 10 Hanko fukidashi | 判子 吹き出し | A red seal closes the reply with its agent/session provenance. |
| 11 Makimono taiwa | 巻物 対話 | A handscroll carries paired balloons as an append-only conversation. |
| 12 Toi | 問 | The question's gate frames a mouth drawn as a speech balloon. |

Mondō's internal strokes suggest 問 and 答 at large sizes; they are not
miniature typeset glyphs. At 16 px the two balloons take priority. Toi is the
most reduced variation: one voice inside a gate, rather than two balloons.
The hanko indicates source, never correctness or approval.

`--preview DIR` writes each concept's copper `icon.svg`, `logo-light.svg`,
`logo-dark.svg`, and one-paragraph `rationale.txt` directly in its slug folder.
The corresponding indigo files live in that folder's `ai/` subdirectory.
`concepts.html` embeds all SVGs and CSS and works offline without adjacent
files. Both it and `concepts-sheet.svg` show 16, 32 and 128 px icons and
light/dark wordmarks in both palettes. With `rsvg-convert` on PATH, the same
preview also writes individual PNGs and `concepts-sheet.png`. SVG/HTML output
needs only the Python standard library.

`--preview` does not write production assets. `--check` compares their exact
bytes and checks the inherited ochazuke/matomeru wordmarks. The requested
round-2 output is `/tmp/oq-logo/artifacts/round2/`; initial review artifacts
remain at `/tmp/oq-logo/artifacts/`. Output contains no generated timestamps,
absolute paths, random identifiers or network assets. PNG reproducibility
assumes the same renderer and system fonts; the logo SVGs contain geometry
only and have no font dependency.

## Reused family construction

`build.py` copies and adapts `~/proj/o-cmds/docs/logo/build.py`: the concept
version at `54d056b` and production/check flow at `7104aff`. That builder came
from `o-proj`, with additional glyphs from oden and o-slack. The hand-drawn
monoline wordmark, 11-unit stroke, 15-unit letter gap, x=112 origin,
128-unit tile with radius 28 and 8-unit mark grid are retained. Fine kanji
cues and seal details use half-grid subdivisions. No shared library or sibling
repo changes are needed: these are o-questions identity/review artifacts.

Round 2 adapts the ideas and tone of `~/hub/logos/monogatari/gen.py` and its
Emaki, Koma, Shiori, Kataribe and Kakuin marks/rationales. Kataribe replaces
monogatari's prompt/cursor with a continuing voice line and an answering
balloon. Makimono replaces Emaki's frame panels with dialogue balloons.
Hanko borrows the seal vocabulary of matomeru's Hanko and monogatari's Kakuin;
its new square inset is a source device, not a copied glyph. Matomeru's
Furoshiki and o-json's Kakko/Bento also informed the approach: a Japanese
object or written form expresses what the app does, with a strong small-size
silhouette. The per-concept folders and `rationale.txt` convention come from
these family generators. The vector contact sheet extends o-cmds' HTML
comparison for rasterisation without another Python library.

Wordmark glyphs `o`, `u`, `e`, and `t` come through the o-cmds/o-proj table
from ochazuke/matomeru. The hyphen, `i`, and `n` use existing o-proj pieces.
`s` is the two-half-ellipse glyph from o-slack, already in o-cmds. Round 1's
`q` mirrors `p`'s straight descender onto the right of the same 40-unit bowl,
down to y=106; the descender canvas remains 112 high. Round 2 changes no
wordmark geometry.

## Colour comparison

Copper 24° retains the family's five HSL stops (`#d4783a` primary), as in
round 1. Ai-inspired indigo (藍) uses five explicit stops, anchored on deep
`#165e83`, with `#4c8ba8` primary; it is a colour interpretation, not a claim
of one canonical dye value. Every card compares identical geometry in these
two palettes. Hanko additionally uses shu-vermilion (朱): `#c73e2b` on light
wordmarks and `#e85b42` on dark wordmarks/icons. The red seal stays red in
both main palettes.

The initial inventory checked o-cmds chartreuse 72; ochazuke green/lime;
matomeru purple with pink accents; o-json saffron/teal/cream; monogatari
blue/mint/red; oden amber/warm accents; and o-proj cyan/teal. Copper was unused
as a primary in that inspected set. Round 1's cyan comparison is replaced in
the round-2 sheet by ai; no claim of colour exclusivity is made.
