<p align="center">
  <picture>
    <source media="(prefers-color-scheme: dark)" srcset="assets/logo-animated-dark.svg">
    <img alt="o-questions" src="assets/logo-animated-light.svg" width="360">
  </picture>
</p>

# o-questions

A private journal of the owner's questions and the answers given, across agents,
projects and sessions. One JSON record per question, append-only thread entries,
undo history and daily backups. Domain state belongs in this app; ochazuke owns
keys, forms, list motions, settings and page layout.

## Install

```sh
go install ofam.dev/oden/cmd/o@latest   # oden's o: o questions runs o-questions, no dash needed
go install ofam.dev/o-questions@latest
mtm extension register questions # mounts the question.* verbs (and its pages) in mtm
```

## Quick start

Build with Go 1.27.1: `GOWORK=off GOFLAGS=-trimpath go build -o o-questions .`.
For an isolated demo, set HOME and all XDG directories to temporary directories
and pass `--data /tmp/oq-core/demo` on every invocation:

```sh
./o-questions --data /tmp/oq-core/demo ask 'How does the question journal work?'
./o-questions --data /tmp/oq-core/demo answer 1 'Questions have append-only threads and provenance.'
./o-questions --data /tmp/oq-core/demo list
./o-questions --data /tmp/oq-core/demo show 1
./o-questions --data /tmp/oq-core/demo undo
```

`ask` accepts positional words (joined with spaces) or `--question TEXT` with
metadata flags such as `--title`, `--key`, `--cwd`, `--project`, `--task` and
`--asked_at`. A quoted single argument preserves its whitespace. `question`
has alias `q`; `add` and `new` alias `ask`. Targets resolve by exact title or ID,
then displayed number, then unique prefix. Numbers follow `list.order` across
the complete list, including when a view is filtered. Machine consumers can use
`question list --json '{}'`; table output uses `--format table`.

## Rights and persistence

CLI identity uses `O_QUESTIONS_ACTOR` (or an inherited `MTM_ACTOR`), otherwise
harness environment variables identify an agent, otherwise owner. An explicit
actor is a trusted CLI declaration, not an authentication boundary. Agents may
read and write, including editing wording, changing status, replacing titles or
associations, removing metadata, undo, merge, split, restore and decisions.
`question.delete` declares agent-never; use `question.archive` instead.
The catalog and direct-store policy share the same declaration data. The CLI
binds the store's identity into the shared rights gate, and owner
`agents.rights` rows may override its defaults. Every mutation retains attribution.

Records live under `$XDG_DATA_HOME/o-questions` (default
`~/.local/share/o-questions`); `--data` overrides this. Directories use 0700 and
files 0600. A lock serializes CLI/MCP writers. Atomic replacements and a
write-ahead journal recover interrupted multi-record writes. `.history` holds
full undo snapshots, including creation. Hidden records retain their bytes and
idempotency keys, so retrying a hidden question does not recreate it.

`question archive TARGET` hides the question without losing its text or history.
`question restore --target ID` restores it; `restore FILE` keeps its existing
backup meaning. Both operations are repeatable and undoable. The Decision aliases
share these handlers. Archived questions remain addressable by ID or title.
`collection.hide-archived` defaults to true; false includes archived Questions rows.
Archived rows show `-` instead of a positional number, so live numbers remain
stable. Use a stable ID or title for an archived record.

Entry statuses come from the `status.from-entry` setting. Explicit statuses
stick across new entries. Entries over `entry.max-bytes` are linked private files;
show/yank and search hydrate their full text. Daily backups run before the first
write of each UTC day. `backup --to DIR` writes a manual tar.gz;
`restore FILE` restores records and bodies through the same undo mechanism.
History remains private and is not compacted automatically.

## Settings

Use `settings list` and `settings set NAME VALUE`; ochazuke owns the app's config.
Defaults: oldest first, newest initially selected, all status/project/agent filters,
any date, complete wrapped answers, metadata on, thread modal on, detail on,
refresh every 2s, 64 KiB inline entry limit, daily backups on, 14 retained.
Ochazuke v0.67.0 supplies `detail.sections=true` and `detail.side=true`.
`detail.sections=false` restores the generic record detail; `detail.side=false`
hides metadata without hiding answers. `detail.side-width=96` is the detail
content width at which metadata moves from below the discussion to its right.
`detail.provenance=false` hides provenance in the full thread modal;
`view.detail=false`, `view.thread=false`, `list.start-newest=false`, and
`list.refresh=0` opt out of their UI behaviors. The old `detail.answer-lines`
setting remains registered for compatibility but is deprecated: Sections always
show the complete discussion, regardless of its value.
`question.show` and its `decision.show` alias return the complete thread as
declared Markdown to a catalog host. Enter and `v` use this readable result;
standalone they keep the existing thread modal. The structured record is
`question get`, and the thread, the latest answer and the record file are its
computed props (`question get --prop thread`, `latest_answer`, `record_path`; the store
directory is the collection prop `question path`). `view.thread=false` disables
the viewer and publishes that state in the catalog; refresh a host's catalog
after changing it.

Hook and HTML settings are reserved for the separate Q4/Q5 lanes.

`z /` opens ochazuke's prop filter form on Questions and Decision, in the
standalone TUI and catalog hosts. Filter by `status`, `project_id` (`project`
is an input alias), `agent`, `direction`, `pane` or `agent_session`. These
conditions narrow the rows supplied by the existing `filter.*` settings and
list parameters; use `filter.direction=all` to include both directions on
Questions. Old owner questions have the filter direction `to-agent`.

## Scope

This lean core implements Q1–Q3 of the family plan. Hooks, HTML export, host noun
relations, send/goto/make-task, mtm detail tabs, editable Markdown file views,
import, installation and owner config registration are separate work.

## MCP

`o questions --data DIR mcp` serves newline-delimited JSON-RPC over stdin/stdout.
The owner-question tools are `q_ask`, `q_answer`, `q_annotate`, `q_list`, `q_show`;
the decision tools (questions for the owner) are `q_pose`, `q_queue`, `q_decision_export`,
`q_reply`, `q_applied`, `q_import`. The curated MCP tools still omit decisions;
the CLI and mounted catalog permit them as attributed writes.
`q_ask` requires `question` (verbatim); passing the same `key` returns the existing
record with `existed: true`. `q_answer` requires `target` and `answer`. `q_annotate`
requires `target`; with `text` it appends a note (kind `note` or `correction`, an
optional `replaces` entry ID), and it adds tags/links and fills associations or
proposes a title. `q_show` requires `target`; `q_list` takes
optional text/status/project/pane/agent_session/since/limit filters.

`tags` is an array of strings. `links` is an array of objects with required
`target` and optional `kind` and `label` strings. Schemas come from the app's
verb specs; discovery and dispatch use the same allowlist. Unknown
fields, wrong types and tools outside that allowlist are refused. The server
always stamps writes as an agent, retaining inherited harness/pane/session/model
provenance and using the initialize client's name as a fallback. Its curated
tools do not expose delete or direct wording/status edits. Registration in the
owner's harness is Q6.

## Page and keys

`view` opens the standalone split page; `view --smoke` prints a frame without a
terminal. The left list starts on the newest question, with status, project,
agent, age and answer count. The detail shows the question and all answers,
follow-ups, notes and corrections as wrapped Markdown. Metadata appears on the
right at wide widths and below the complete discussion at narrow widths.
There is no answer line limit; `J`/`K` scroll the document. `g m` opens metadata,
where `y` copies a complete value and Enter expands it read-only. `v` or Enter
on a question opens the full read-only thread.

Keys come from binding rows: `c` asks a question, `a` adds an answer, `f` adds a
follow-up, `s` chooses status, `e` edits the question, `r` renames its title, `m`
associates it, `t` adds a tag, `g x` chooses a link, `y` copies the thread, `d d`
hides, `u` undoes and `o` cycles the status filter. `/` searches complete question
and answer text and tags; `n`/`N` move among shown matches. Question/title/entry
editors commit after a short typing pause. Escape closes them with changes kept.
Undo reverses a change. No save key is required.

The catalog publishes this page for mtm/nabe. Its rows include `cwd` for existing
host project scope rules. Standalone and catalog hosts on ochazuke v0.67.0 or
later render the same declarative Sections; older hosts retain record detail.
See [host notes and Vim comparison](docs/INTEGRATION.md).
The host mount was checked with an isolated mtm database and wrapper directing
all extension calls to a temp store. No owner config is registered by this lane.

## Questions for the owner (decisions)

The record above is the owner asking an agent. The other direction uses the same
store: an agent poses a question for the owner, with a recommendation, and the
owner decides it on the Decisions page. This is the [mgmt review flow](https://github.com/unixsuperhero/ai-utils/tree/main/mgmt)
built in: recommendations show as unconfirmed until
accepted, numbered options (or a checklist of items), a note per item, an ask that becomes a thread the
agent answers, decisions that survive re-posing, an export the agent reads, and
items locked once applied.

```sh
# agent
o questions question pose "Turn the Decisions page on by default?" \
  --group Decisions --source plans/x.md:42 --details "context" \
  --options '["Turn it on","Leave it off"]' --recommended "Turn it on" \
  --recommendation "Opt-out setting exists" --key x-42
o questions question import            # pose the open "Owner questions" of decision.plans-root
o questions decision export            # JSON: asked questions, decisions, undecided
o questions decision export --render plan | jq -r .   # "## Owner decisions (date)" per plan
o questions question pose "Which branches to delete?" --key br-1 \
  --items '["feat/a",{"id":"b","label":"feat/b","recommended":true}]'   # a checklist: checked = do it
o questions question reply Q "answer" [--recommendation T] [--recommended OPTION_ID]
o questions question applied Q done "what changed, how checked"   # locks it
# owner
o questions decision                     # the Decisions page (also a tab in mtm)
o questions decision --web [--no-open] [--port N]   # mgmt's browser page
o questions question decide Q yes [--note T] | --choice 2
o questions question accept|undecide Q
o questions question ask-back Q "why?"
```

A question for the owner is a `Question` with `direction: to-owner` and a
`decision` record (topic, source, details, recommendation, recommended option,
options, items, pick, note, flags, applied status). Posing the same `key`
again updates the text and keeps the decision; a decided item whose question,
details or recommendation changed is flagged `context_changed` until the owner
touches it. Asking appends the owner's question to the thread (`follow-up`
entry, status follow-up); the agent's `reply` (a `reply` entry) clears the ask,
flags `new_reply` and may replace the recommendation. `applied ... done` locks
the item; `failed` leaves it open. Agents may pose, reply, record results and
decide through the CLI/catalog, with actor-attributed history.

`question import` ports `owner_questions.py`'s rules to Go (same sections,
recommendation markers, answered detection, groups, titles and `oq-` ids), so
agents import through the CLI or MCP without Python. It never edits a plan:
`decision export --render plan` prints the sections for the agent to insert. Plan
questions that disappear are reported `stale` and left alone.

The Decisions page lists decision rows (noun `decision`, a tree with one row per topic
and its counts; a checklist question has one child row per item, drawn `[ ]` or `[x]`
like the outline's tasks, and its Decision cell shows `2/5`). The decision noun reaches
the question verbs that fit one as aliases (`decision.decide` is `question.decide`).
The detail's Options section lists the numbered options (with descriptions and the
recommendation's state), the checklist, then the outcomes with their keys; the setting
`decision.labels` (default on) turns that off for the compact one-line list.

Keys (all binding rows; `bindings.json` overrides them): `1`-`9` pick the Nth option;
the outcome keys come from the rows of `decision.outcomes` (`id|label|status|class|flags|key`,
defaults `l` later, `m` moot, `?` ask the agent, `w` write your own answer; the last two
ask for text in a form); `a` accepts the recommendation, `D` decides a whole checklist as
it stands (unchecked means do not), `n` edits the note, `x` clears the decision (on an item
row, its mark), `c` checks or unchecks a checklist item, `o`
cycles `decision.show` (pending, undecided, asked, later, changed, decided, answered, differs, applied, archived, all),
`v` shows the thread, `y` copies, `u` undoes. The other keys on an item row act on its
question. Over a visual selection (`V`) `c` checks every selected item. A topic header is no
question: `l` on it stays the library's expand when the page runs standalone (a host such as mtm drops key guards). `l` and `w` shadow ochazuke's table/tree
motions on question rows (move right, next word); rebind or empty the key column of
`decision.outcomes` to get them back.

#### The document pane

With `decision.detail=document` (default; `sections` restores the read-only detail; read at
start) the right pane is ochazuke's Markdown editor, as on the Docs page, over a working
file per decision: it takes focus (`tab`, `}`, or `j` past the last row; `tab`/`esc` back),
moves by element, and `x` flips a `- [ ]` line. The record stays the source of truth: the
file is rendered from it, and what you change in it (a box, the Note text) is read back
through the decision verbs. Standalone that happens on every save of the editor; mounted in
mtm, on the page's next refresh (`list.refresh`, 2 s by default), when the file is shown
again from the record. Anything else you type is discarded on the next render.

```
# Import nightly?
Question text, details.
## Recommendation
## Your choice
- [ ] yes <!-- option:yes -->
- [x] no (recommended) <!-- option:no -->
## Checklist 1/3 (checked means do it)
- [x] feat/alpha (recommended) <!-- item:a -->
## Other answers
- [ ] Later <!-- outcome:later -->
- [ ] Write your own answer (type it under Note) <!-- outcome:other -->
## Note
free text, any number of lines
```

The `<!-- kind:id -->` markers carry the ids (the editor draws them hidden), so rewording or
reordering lines cannot mis-assign them. Options and outcomes are one single choice: when
two boxes end up checked the last one in the file wins, and unchecking the chosen box
undecides. Checking an outcome that needs text (write your own answer, ask the agent) uses
the Note text. A deleted line changes nothing. An applied decision renders without boxes
and ignores edits. Working files live in `$XDG_STATE_HOME/o-questions/decision-md/`
(one `QUESTION-ID.md`, a `.base/` copy of the last render for diffing, `topic-*.md`
summaries); a file is removed once its decision is no longer listed (applied and hidden,
archived, gone). Only the owner's page writes them; an agent listing the queue does not.

#### Colour, markers and keeping the list manageable

Each decision row carries a glyph and a theme tone by its state (setting
`decision.color`, default on; off draws the plain rows): `○` new, not decided (tone
`state.needs-input`), `✓` answered as recommended or with nothing recommended
(`state.done`), `≠` answered differently from the recommendation, a write-in, later or
moot included, and a checklist whose marks differ (`state.working`), `?` asked back
(`marker.queued`), `…` later (`state.idle`), `●` applied and `▫` archived (`state.exited`),
`✗` applying failed (`state.errored`). The glyph keeps mono themes and `NO_COLOR`
readable; recolour a state in `themes.json` through its tone. The detail marks the
recommended option and the chosen one (`✓ chosen`, `≠ differs`), a `Versus` field says
whether the answer agrees, and a topic row's detail carries the legend.

Archive, never delete: `A` archives the question under the cursor (on a topic row, every
question of the topic), `R` restores one, `u` undoes (a batch is one change, so one `u`
brings it all back), `g A` lists what `decision.archive-matching` would archive (applied ones
changed more than 30 days ago) and `g Z` archives them. The verb is
`decision.archive-matching --show STATE --group TOPIC --days N [--apply]`
(`question archive-matching` on the CLI; the MCP tool of the same name): without `--apply`
it only lists. Archived questions are hidden; `o` / `O` step `decision.show` forward and
back through pending, undecided, asked, later, changed, decided, answered, differs, applied,
archived and all (`archived` shows only them; `collection.hide-archived` off shows them
among the rest). `decision.sort` (default `group`, as recorded; `undecided-first` puts what
needs you first inside each topic, and a row you decide then moves down). Topics fold and
unfold with the tree's own keys.

`decision --web` writes an mgmt session (`$XDG_STATE_HOME/o-questions/decision-web`)
from the store, runs `python3 mgmt.py serve` on it (`decision.mgmt`), and copies
each saved decision back into o-questions while the page runs and once more
after Ctrl-C. The session is a view; o-questions stays the record.

For the browser page, install `mgmt/` from [ai-utils](https://github.com/unixsuperhero/ai-utils).
Set `decision.mgmt` to the path of `mgmt.py`, or leave it empty to discover an
executable `mgmt.py` on `PATH`. Existing installations in the legacy hub home
location are used as a final fallback. Python 3 is required for this page.

Settings: `decision.page` (default true; false removes the page), `decision.show`
(pending), `decision.detail` (document), `decision.color` (true), `decision.sort` (group), `decision.outcomes` (the answers that are not options: later, moot, ask, other),
`decision.plans-root` (the folder to import planning documents from), `decision.mgmt`
(empty; discovers `mgmt.py` at run time), and `filter.direction` (to-agent) for the
Questions page, which keeps showing only the owner's own questions by default.

### Setup: the agents' note

`o questions setup` adds a managed block to the agents' instruction files
(`${CLAUDE_CONFIG_DIR:-~/.claude}/CLAUDE.md`, `~/.codex/AGENTS.md`) between
`<!-- o-questions:begin -->` and `<!-- o-questions:end -->`. The note tells agents to record the owner's questions (`question.question ask|answer|follow-up`) and pose decisions only through mtm, by MCP `run` or `mtm question ...` (`mtm question ask WORDS`, `mtm question answer ID TEXT`), never the `o-questions` binary directly:

```sh
o questions setup                  # check
o questions setup install --dry-run   # dry run
o questions setup install --yes    # or: setup install claude-md --yes
o questions setup uninstall --yes
```

Both items use ochazuke v0.78.0's `follow: true`: setup resolves symlinks on
these paths, manages the target files and leaves the links intact. Paths that
resolve to one file share one write and one note. Each existing target gets a
`.o-questions.bak` backup; reinstall is idempotent and uninstall removes only
the managed note (restoring the original bytes when no other edits intervene).

`setup.skip` lists items to leave out. Its default is computed at startup from
the instruction paths under the process's HOME (and `CLAUDE_CONFIG_DIR`, when
set): identical managed blocks whose files resolve into the same directory
keep only the last declared item enabled. With the built-in item order this
skips `claude-md` and keeps `codex-agents`. This conservative directory rule
avoids reading the note twice when a co-located `CLAUDE.md` imports `AGENTS.md`,
without parsing a particular harness's import syntax or checking tool names.
Files in separate resolved directories stay enabled. An explicit `setup.skip`
value overrides the default; an empty list enables both, even for co-located
files. `--home` redirects setup's targets; it does not change the startup
setting default, so use the fixture HOME as well when testing defaults.
