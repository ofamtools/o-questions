# Changelog

## Unreleased

- Fixed (SC-072, SC-073): `question pose` dropped unknown keys on an option or item object without a word. It now names each in a notice with did-you-mean (`option 2: unknown key "lable" was not saved (did you mean "label"?)`) and keeps the rest; a bare string stays an id and label.
- Changed (record first, S15, ER-1420, ER-1453): an empty question is saved as `untitled-STAMP` (edit it later); an empty reply, answer or edit is recorded instead of refused.
- Changed (tuios purge): the question source's session is stored as `mtm_session` (`Source.MtmSession`); a stored `tuios_session` is read as `mtm_session` by `migrateLegacy` (one row in `legacyKeys`) and the next write stores the new name. Identity reads `MTM_<name>` only (`paneenv.go` removed; no `TUIOS_` fallback).
- Added: `question list` and `decision list` print a table at a terminal (`cli.tty-format`, ochazuke v0.135.0); a pipe still gets JSON.
- Not yet: the catalog does not declare `serve` (ochazuke `host serve`): the shell's actor is read once per process, so a served call could run with an earlier call's identity (needs a per-call actor hook in ochazuke). The source fields read from the environment (transcript, turn, agent session) are already read per call.
- Deps: ochazuke v0.135.0; the family word list copy gains `look`, `rail`, `resize`.

## v0.11.0 (2026-11-13)

- Changed: reads never call the host for projects. `question list`, `decision list` and the pages show the stored project id with the name kept on the question (`project.label`) and match a `--project` filter from them; the host (`mtm project list`, about 2 s per call when mounted) is asked only to resolve a typed reference on a write, to default a new question's project from its pane, and for a filter that matches no stored id or name. A renamed project shows its old name until a write touches the question.

## v0.10.0 (2026-11-12)

- Added: a question or decision recorded with no project takes one from its context, through the same path for `ask` and `pose`: the asking pane's project as the host reports it (`pane list`, ended panes included), else the project whose directory holds `--cwd` (or the current directory; the longest matching directory wins, several equal matches store nothing), else none. A given `--project` always wins; a follow-up keeps its thread's. The host read is one call, cached for a few seconds; a failed or slow read stores no project and never fails the ask. `project.source` `none` makes no host call and defaults nothing.
- Added: `resolve-project` also fills questions that have no project and a recorded pane from that pane's project where the host still lists the pane; the report gains `pane_filled`, `pane_unknown`, `pane_no_project` and `no_pane` (and `panes_error` when the host's pane list could not be read). Same dry run, backup and byte-splice rules; set projects and every other byte are untouched.
- Added (missed in v0.9.0): a project reference typed as a name, an id or `mtm:<id>` is stored as the host project's id with its name kept in `project.label` (shown as the host's current name); setting `project.source` (`mtm` when installed, else `none`) chooses where references resolve; `resolve-project [--dry-run]` (owner only) rewrites existing questions the same way, after a backup to `<data>.bak-<date>-project-ids`, leaving unmatched and ambiguous names as they were.

## v0.9.0 (2026-11-12)

- Added: the `pane` prop of a question and a decision is a reference to the host's `pane` (`hostNoun`), so ochazuke's relation graph follows it in mtm; a pane that is gone stays as written. `project_id` stays a plain string (stored as a project name or an id; mtm keys by id) and `agent_session` names a harness session, which is no noun. Requires ochazuke v0.124.1; the family word list copy is refreshed.
- Added: setting `history.keep` (default 200, `0` keeps everything) bounds the undo history (`.history/`): the write that adds an entry removes all but the newest N (names only, never entry contents; oldest first, so an interrupted pass resumes), numbering never resets, and undo past the retained range reports `nothing more to undo: older changes were removed by history.keep`. Daily backups already pruned to `backup.keep` (`TestStoreDailyBackupRetention`, now also checking `backups/pre-migration` is left alone); they share one retention function with history. `Store.Remove` makes file removal injectable.

## v0.8.0

- Added: the Decisions page's right pane is a focusable Markdown editor (the Docs page's component, detail kind `markdown`) over a working file per decision, rendered from the record: `x` flips a `- [ ]` line, options and outcomes are one single choice, a Note section takes free text; edits are read back through the decision verbs (on every save standalone, on the next refresh when mounted), then the file is rendered again. Files live in `$XDG_STATE_HOME/o-questions/decision-md/` and are removed with their row. Setting `decision.detail` (`document` default, `sections` = the read-only detail).
- Fixed: `n` (decision note) and `w` (write your own answer) failed with `question "decision/questions/question/Q" not found` in both hosts: the form passed the list-qualified row id as target. Question, item and topic ids are read at their last `question/`, `item/` or `topic/` segment (`parseRow`).
- Added: decision rows are marked by state with a glyph and a theme tone (`○` new, `✓` answered, `≠` answered differently from the recommendation, `?` asked, `…` later, `●` applied, `✗` failed, `▫` archived); the detail shows the recommended and chosen options, a `Versus` field and, on a topic row, the legend. Setting `decision.color` (default true) turns it off.
- Added: `decision.show` values `differs` and `archived`; `O` steps the filter back, `o` forward. Archived questions stay hidden (they were never listable on this page); `archived` shows only them, `collection.hide-archived` off shows them among the rest. Setting `decision.sort` (`group` default, `undecided-first`).
- Added: `decision.archive-matching` (`--show`, `--group`, `--days`, `--apply`; lists unless `--apply`; also `question archive-matching` and the MCP tool) and `decision.archive-group`, each one store change, so one undo restores the batch. Keys: `d a` archive (a topic row: the whole topic), `d r` restore, `g A` list the applied ones older than 30 days, `g Z` archive them.

## v0.7.0

- Added: the `blocker` noun, what waits on the owner, declared in the catalog so a host (mtm) can merge the rows of several tools. `blocker list` (the framework's list: filter, where, sort) holds every undecided decision for the owner (`kind=decision`, resolved by `question.decision.decide`) and every added item not done (`kind=item`, resolved by `question.blocker.done`); a row has `id`, `title`, `kind`, `source` (the tool), `age`, `created`, `group`, `details`, `resolve` and `resolve_args`. `blocker add TEXT [--details --key --group]` adds a plain "waiting on you" item, stored as a one-item checklist decision tagged `owner-waits` (no second store; the key makes adding it again a no-op). `blocker done TARGET` checks, confirms and applies an added item through the decision verbs; `blocker show TARGET` reads one. Decided, applied, later and asked decisions are not listed.

## v0.6.0

- Changed: the pane variables are read as `MTM_X` first, then `TUIOS_X` (mtm sets both for one release): `ENV`, `SESSION`, `PANE_ID`, `HARNESS`, `AGENT_NAME`, through one `paneEnv` helper.

## v0.5.0

- Changed: bump ofam.dev/ochazuke to v0.109.0 (modals close with q; esc only leaves insert; row guards; label-first palette). Question stories and parameter tests close modals with q.
- Added (Decisions page for options and checklists):
  - The detail's Options section lists the numbered options with their descriptions and the recommendation's state, a checklist with its progress, then the outcomes with their keys. New setting `decision.labels` (default true); false draws the compact one-line list (`1 id · 2 id · Later ...`).
  - The outcome table has a sixth field, `key` (`id|label|status|class|flags|key`): the page key that picks the outcome, made into a binding row from the table (decision.decide, or decision.write-in for a row that takes text). Defaults: `l` later, `m` moot, `?` ask the agent, `w` write your own answer. A key may not be a number, a function key or shared by two rows; rows without a key bind nothing. A topic header keeps the library's `l` (expand).
  - A checklist question has one child row per item (`[ ]`/`[x]`, the outline's drawing) and a `2/5` progress cell; item labels are in the question row's search text. `x` on an item row checks or unchecks it (over a `V` selection, every selected item), `d d` clears its mark, and the question's other keys (`a`, `D`, `n`, the outcome keys) act on its question. `D` decides the whole checklist.
  - New verbs: `decision.write-in` (decide with an outcome that takes text; the key rows of `other` and `ask` run it) and `decision.toggle` (check or uncheck an item; homed on decision so a visual selection fans out). `decide` no longer requires `action`: `item`, `confirm`, `option`, `choice` or `note` alone is enough. `ask-back` is unchanged.
  - The `claude-md` and `codex-agents` setup notes tell agents to pose 2 to 4 `options` in the owner's words (never yes/no unless literal), `recommended` with the reason in `recommendation`, `items` for lists of independent picks, a stable `key`, how to read the answer, and `applied` (per item for a checklist). The hub `AGENTS.md` text is the same.
- Changed, breaking (decisions with options, one shot; old `actions` still read as options until the migration below rewrites them): a question for the owner carries its own options.
  - `pose` takes `options` (strings or `{id, label, description}`), `recommended` (an option id; never implied) and `items` (a checklist: strings or `{id, label, description, recommended}`). `actions` and `recommended_action` are refused with an error naming the new parameter. A question posed with no options and no items is still stored; the result carries `warnings` telling the poser to supply options. Option ids must be unique and may not equal an outcome id.
  - No default options: `yes, no, later, moot` is gone as an implicit set. Plan import states its own yes/no options.
  - New setting `decision.outcomes` (rows `id|label|status|class|flags`; default later, moot, ask, other) is the table of answers that are not options. It drives the question status and the `decision.show` class (a new `later` class); flags `note` (needs text) and `reply` (the note goes to the agent, a reply reopens). `decision.actions` and `decision.ask` are no longer read.
  - Checklists are two-state: `decide item=ID [checked=BOOL]` marks an item, `decide confirm` decides the list as it stands, `accept` checks the recommended items, `undecide item=ID` clears one mark, `applied item=ID` applies one checked item (the decision locks when all checked items are applied). `decide option=ID` is new beside `action` and `choice`.
  - Repose by `key` keeps the owner's pick while its option or item id exists; otherwise the pick is cleared, the question flagged context changed, and the lost choice noted in the thread. `reply` takes `recommended` (an option id).
  - `decision export` adds `answer` (kind, id, label, text), `options`, `picks`, `progress` and `recommended`; `action` and `recommended_action` stay.

- Changed (migration for decisions with options, one shot):
  - Stored questions: the first load after the upgrade rewrites each question that still has the old shape, after one backup of the store in `backups/pre-migration/`. A question with no stored `actions` (the old global default) gets the explicit options `yes` and `no`; custom `actions` become options (the words `later`, `moot`, `ask`, `other` are outcomes and drop out); `recommended_action` becomes `recommended` when it names an option. The pick (`action`) is untouched, so a stored `yes` is still option `yes`. The old `actions` and `recommended_action` stay in the record. Migrated decisions carry `format: 2`; a second load changes nothing.
  - Settings: `decision.actions` and `decision.ask` are retired and removed from the settings file at start (comments kept; the file is copied to `settings.json.bak-DATE` first). A custom `decision.actions` list is dropped with a notice (there are no default options); `decision.ask=false` becomes a `decision.outcomes` list without the `ask` row.

## v0.3.0 (2026-11-08)

- Changed, breaking (lexicon, one shot: no old-name aliases; every rename is in `docs/lexicon-renames-v0.3.tsv`): the Decisions page's noun is `decision`, not `review`. The page was already titled Decisions.
  - Noun `review` is `decision` (`question.decision`: create, edit, get, list, open-page, set, watch and the question verbs it shares: decide, accept, undecide, ask-back, decision-note, reply, applied, show, yank, tag, ...). `o-questions review [--web]` is `o-questions decision [--web]`; the page id, picker, key rows (`decision/normal`, `decision.decide`, ...) and the mgmt session directory (`decision-web`) follow.
  - `question decisions` (the export of decisions, asks and undecided items) is folded into the noun as `decision export` (`--group`, `--all`, `--render plan`, `--date`); there is no `question.decisions` alias. The MCP tool `q_decisions` is `q_decision_export`. The export format name `o-questions-decisions/1` is unchanged.
  - Settings `review.page|show|actions|ask|plans-root|mgmt` are `decision.*`, same defaults.
  - Stored data: a question's `review` block is stored as `decision`. Old question files and undo snapshots are read with the old key (and the older `state` key) and written back with the new ones on the next write of the file. The page's row data `decision` (the status cell) is now `decision_view`.
  - Go: `Review` is `Decision`; the owner's change input that was `Decision` is `DecisionChange`; `Store.Decisions` is `Store.DecisionExport`.
  - The setup notes (`claude-md`, `codex-agents`) say the owner answers on the Decisions page and read decisions with `question.decision export`.

## v0.2.0 (2026-11-08)

- Changed, breaking (LX8c, lexicon, one shot: no old-name aliases; every rename is in `docs/lexicon-renames-v0.105.tsv`): requires ochazuke v0.105.0. One word has one part of speech.
  - The question status is the writable `status` prop (it replaces the prop `state` and the verb `status`): `question set ID status answered`, MCP `question.question.set {prop: status, value}`; its Enum is open, answered, follow-up and moot. The Questions page `s` row is `question.choose-status` (the status as a required choice).
  - `note` is `annotate`: `question annotate ID TEXT` appends a note or correction (`--kind`, `--replaces`), and still takes tags, links, a project, a task or a title (both kinds of argument in one call run both steps). The MCP tool `q_note` is `q_annotate`.
  - `view` is `show` (the markdown thread, or the viewer in the TUI; `review.view` is `review.show`); the structured record is `question get`, with the computed props `thread`, `latest_answer` and `record_path`, and the collection prop `path` (the store directory; `question path` is gone). `copy` is `yank` (it always copies to the clipboard and answers the text; `review.copy` is `review.yank`).
  - Stored data: the review block's `state` key (empty, applied, failed) is `status`; old question files and undo snapshots are read with the old key and written back as `status` on the next write (tests `TestLegacyReviewStateLoadsAsStatus`, `TestLegacyUndoSnapshotLoads`). The decisions export item's `state` is `status` too (format name `o-questions-decisions/1` unchanged).
  - ochazuke v0.104 and v0.105 renames reached the code: `list.hide-archived` is `collection.hide-archived`, `setup status` is `setup check`, `setup plan` is `setup install --dry-run`, the store kind is `KindCollection`. `testdata/lexiconcheck.txt` is emptied of the framework's rows; `testdata/lexicon-words.tsv` is the family list.
- Changed (LX7b, lexicon): requires ochazuke v0.103.0. The tool declares its main noun, `question` (`Noun.Main()`), so the reader resolves `o-questions ask WORDS`, `answer ID TEXT`, `pose`, `follow-up`, `add` and `q ask` to `question.question.<verb>`. Removed the hand-written `question` command and its per-verb loop in `main.go`, and the `nounDefault` rewrite: `review` declares `DefaultAction("page")`, so bare `review` and `review --web` still open the page. `TestEveryCatalogVerbRuns` no longer skips question verbs. New `TestLexicon` (`lexiconcheck`) over `testdata/lexiconcheck.txt`, seeded with today's violations (framework nouns).
- Changed (LX7b): the "Questions (o-questions)" setup note names the new operations: MCP `question.question ask|answer|follow-up|pose|decisions|reply|applied` (not `questions.question ...`) and the mtm CLI short forms `mtm question ask WORDS`, `mtm question answer ID TEXT` (not `mtm questions question ...`).
- Changed: requires ochazuke v0.102.0 (the Operations registry and three-part keys; `cmd/opname` moved the code) and oden v0.3.0. o-questions declares its namespace, `question` (`ochazuke.DeclareNamespace`), so its keys are `question.question.<verb>` and the catalog carries `namespace: question`. CLI spellings are unchanged; a host that honours the declared namespace names the MCP verbs `question.question ask` instead of `questions.question ask`.
- Changed: requires ochazuke v0.101.0 (`Operation.OpensPage`, verb-first CLI words, pair reader) and oden v0.3.0; no code changes were needed.
- Fixed: the Review page never appeared as a tab in mtm. mtm refuses a page whose title another page has, and its own turn reviews tab is titled "Review"; the page is now titled "Decisions" (`reviewPageTitle`; the verbs and the noun stay `review`). `review.page` declares `OpensPage("review")` in the catalog, so a host that mounts the page opens it (mtm palette: `questions.review.page`) instead of running the verb as a background job with no terminal; run on its own `review` still opens the terminal page or `--web`. Needs ochazuke after v0.100.0 (`Operation.OpensPage`).
- Fixed: `question open` on a question with no links or transcript says "this question has no links" (CLI and TUI) instead of `choose --link from []` or an empty picker.
- Fixed: every verb the catalog declares for `review` runs from the CLI. A hand-written `review` command shadowed Mount's noun group, so `review list --json {}` (the Review tab in mtm) answered "subcommand not found". The command is gone; bare `review`, `review --web` and the page flags are the new verb `review page`, and `review` with no verb word, or only flags, runs it (`nounDefaults`). `review view` and the rest are unchanged.
- Changed: each verb spec row names its own `run` handler (verbs.go); the verb switches in `handle`, `handleReview` and `Store.Change` are gone (store changes are `ChangeOp` values). `add` and `new` are now `NounBuilder.Alias` rows of `ask`; their CLI summary is ask's.
- Changed: question is a thing noun with a store (list, get, set of title and archived, delete); review is a read-only thing noun over the questions for the owner. Every named verb stays (list, edit, rename, delete, archive, restore); `set` with `@label` renames and `@archive` archives or restores.
- The "Questions (o-questions)" setup note is one text for both flows, identical to the owner's global instructions: recording the owner's questions (`ask`, `answer`, `follow-up`) and posing decisions on the Review page (`pose`, `decisions`, `reply`, `applied`).
- The `claude-md` and `codex-agents` setup note (now "Questions (o-questions)") says to record and read questions only through mtm (MCP `run` with `questions.question ...`, or `mtm questions question ...`), never the `o-questions` binary or another MCP server. It adds the owner's own questions (`ask` with their exact words, `answer`, `follow-up`) beside the decisions text. Markers are unchanged, so `setup install` replaces the old block.

- Open agent writes, replacing the append-only allowlist and owner-title/association restrictions while keeping history attribution. Declare `question.delete` agent-never, add `question.archive` through the retained soft-delete handler and restore archived questions with `question restore --target ID`; existing `restore FILE` still restores backups. Review aliases share archive/restore. `list.hide-archived` defaults to true and opts out of hiding in Questions rows. Undo snapshots record the undoing actor.

- Questions and Review expose scalar project, agent, direction, pane and session
  props to the shared filter form in standalone and catalog hosts. Legacy owner
  questions filter as `to-agent`; project records and existing actions, list
  parameters and filter defaults are preserved.

- Upgrade ochazuke to v0.83.0; the thread viewer uses shared modal closing and
  window-sized page/edge motions (`gg`, `G`, Ctrl-D/U/F/B), and empty detail text
  follows the framework's placeholder format.

- Questions for the owner (mgmt built in): `question pose|reply|applied|decisions|import|queue`
  for agents, `question decide|accept|undecide|ask-back|decision-note` for the owner,
  a Review page (noun `review`, keys 1-9 a ? n x o; `review.page` opts out) and
  `review --web` over mgmt's browser page. Settings `review.*` and `filter.direction`
  (the Questions page keeps the owner's questions by default). The plan importer
  ports owner_questions.py. MCP adds six review tools.
- Setup items `claude-md` and `codex-agents` add the "Questions for the owner
  (o-questions)" note to the agents' instruction files.
- ochazuke v0.78.0: instruction-note setup items follow symlinks, preserve
  links and share writes to one target. The `setup.skip` default omits earlier
  identical blocks in one resolved directory, avoiding duplicate instruction
  reads through co-located import files; an explicit list overrides it.

- Question status declares its required model enum. Create, answer, follow-up,
  edit and rename keys omit empty presets so ochazuke's shared argument form
  prompts before dispatch, including in catalog hosts. Prose parameters consume
  the remaining words verbatim. Shared-shell limits are recorded in the host notes.

- Animated README logo (reduced-motion users see the still logo)

- Questions detail uses ochazuke v0.67.0 catalog Sections: complete wrapped
  question/answer Markdown, responsive side metadata and full-value copy/expand.
  `detail.sections` and `detail.side` opt out; `detail.answer-lines` is deprecated.
- Q1: private question/entry store, actors and append-only agent rights, sticky
  status table, idempotent asks, rich question verbs, settings, backups and undo.
- Q2: six allowlisted MCP tools share CLI verb specs, validate schemas and stamp
  agent provenance; retries return the same question ID.
- Q3: Questions split page and catalog, saved filter settings, full-text list
  search, preview/provenance opt-outs, read-only thread modal and binding rows.
- Standalone text editors commit after typing pauses. Status uses a chooser;
  c creates even from an empty page. n/N navigate matches without taking c's role.
- Integration checks preserve chronological creation order and stable slugged
  filenames; large answer revisions retain old bodies for undo. Daily retention
  excludes manual backups.
