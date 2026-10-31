# o-questions

A private journal of the owner's questions and the answers given, across agents,
projects and sessions. One JSON record per question, append-only thread entries,
undo history and daily backups. Domain state belongs in this app; ochazuke owns
keys, forms, list motions, settings and page layout.

## Quick start

Build with Go 1.27.1: `GOWORK=off GOFLAGS=-trimpath go build -o o-questions .`.
For an isolated demo, set HOME and all XDG directories to temporary directories
and pass `--data /tmp/oq-core/demo` on every invocation:

```sh
./o-questions --data /tmp/oq-core/demo ask 'How does the question journal work?'
./o-questions --data /tmp/oq-core/demo question answer 1 'Questions have append-only threads and provenance.'
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
ask, append answers/follow-ups/notes/corrections, add tags/links and fill missing
project/task associations. They can suggest a title until the owner renames it.
Only the owner may edit the question, change status explicitly, remove metadata,
star, hide, undo, merge, split or restore. Refusals name the rule and owner.

Records live under `$XDG_DATA_HOME/o-questions` (default
`~/.local/share/o-questions`); `--data` overrides this. Directories use 0700 and
files 0600. A lock serializes CLI/MCP writers. Atomic replacements and a
write-ahead journal recover interrupted multi-record writes. `.history` holds
full undo snapshots, including creation. Hidden records retain their bytes and
idempotency keys, so retrying a hidden question does not recreate it.

Entry statuses come from the `status.from-entry` setting. Explicit owner statuses
stick across new entries. Entries over `entry.max-bytes` are linked private files;
show/copy and search hydrate their full text. Daily backups run before the first
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
Hook and HTML settings are reserved for the separate Q4/Q5 lanes.

## Scope

This lean core implements Q1–Q3 of the family plan. Hooks, HTML export, host noun
relations, send/goto/make-task, mtm detail tabs, editable Markdown file views,
import, installation and owner config registration are separate work.

## MCP

`o-questions --data DIR mcp` serves newline-delimited JSON-RPC over stdin/stdout.
The six tools are `q_ask`, `q_answer`, `q_note`, `q_annotate`, `q_list`, `q_show`.
`q_ask` requires `question` (verbatim); passing the same `key` returns the existing
record with `existed: true`. `q_answer` requires `target` and `answer`. `q_note`
requires `target` and `text`, accepts kind `note` or `correction`, and an optional
`replaces` entry ID. `q_annotate` requires `target`, adding tags/links and filling
associations or proposing a title. `q_show` requires `target`; `q_list` takes
optional text/status/project/pane/agent_session/since/limit filters.

`tags` is an array of strings. `links` is an array of objects with required
`target` and optional `kind` and `label` strings. Schemas come from the app's
verb specs; discovery and dispatch use the same six-entry allowlist. Unknown
fields, wrong types and owner-only tools are refused. The server always stamps
writes as an agent, retaining inherited harness/pane/session/model provenance
and using the initialize client's name as a fallback. It never edits owner words,
deletes or marks a question moot. Registration in the owner's harness is Q6.

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
