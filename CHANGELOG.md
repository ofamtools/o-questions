# Changelog

## Unreleased

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
