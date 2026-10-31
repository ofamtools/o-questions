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
any date, 8 preview lines, provenance on, thread modal on, detail on, refresh every
2s, 64 KiB inline entry limit, daily backups on, 14 retained. `detail.answer-lines=0`
hides answers; `detail.provenance=false`, `view.detail=false`, `view.thread=false`,
`list.start-newest=false`, and `list.refresh=0` opt out of their UI behaviors.
Hook and HTML settings are reserved for the separate Q4/Q5 lanes.

## Scope

This lean core implements Q1–Q3 of the family plan. Hooks, HTML export, host noun
relations, send/goto/make-task, mtm detail tabs, markdown detail projections,
import, installation and owner config registration are separate work.
