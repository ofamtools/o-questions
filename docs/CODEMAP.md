# Code map

| File | Owns |
| --- | --- |
| main.go | Global data override, oden command dispatch, ochazuke shell mount |
| model.go | Question, entry, source, links and thread projection |
| actor.go | Entry-point identities and the single store rights policy |
| atomic.go | Reused slug/token and atomic-file helpers |
| store.go | Process lock, record files, write-ahead recovery, idempotency, history, undo |
| changes.go | Store mutations, annotations, merge and split |
| backup.go | Daily/manual archives, retention, validated undoable restore |
| check.go | Explicit wording check against transcript user records |
| settings.go | Settings, defaults and validation |
| app.go | Shared verb specs/handlers, rows, search projection |
| mcp.go | Six-tool allowlist, schemas and JSON-RPC stdio lifecycle |
| page.go | Catalog page, preview detail, binding rows |
| tui.go | Standalone mount, refresh and smoke frame |
| editor.go | Commit-on-change question/title/entry and association editors; choosers |
| thread_view.go | Read-only thread modal and scroll rows |
| *_test.go | Synthetic store, CLI, MCP, catalog and page checks |
| cmd/o-story/stories/questions | Page stories run by TestQuestionStories |

All changes belong to o-questions: it owns these domain records, policy and
projections. Ochazuke v0.65.0 supplies the shell, forms, motions, chrome, modals,
settings and catalog. Oden v0.1.1 supplies command discovery and prefix dispatch.
No sibling or shared library was changed.
