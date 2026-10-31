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
| mcp.go | Tool allowlist, schemas and JSON-RPC stdio lifecycle |
| page.go | Shared catalog detail Sections, legacy preview kind, binding rows |
| tui.go | Standalone mount, refresh and smoke frame |
| editor.go | Commit-on-change question/title/entry and association editors; choosers |
| thread_view.go | Read-only thread modal and scroll rows |
| review.go | Questions for the owner: pose, decide, reply, applied, export (mgmt semantics) |
| review_app.go | Review verbs, rows, the review noun's aliases, page and keys |
| review_web.go | review --web: mgmt session out, decisions back |
| plans.go | Plan "Owner questions" importer (owner_questions.py port) and decisions render |
| setup.go, setup.json | Setup items for the agents' instruction note |
| testdata/plans | Fixture plans for the importer |
| *_test.go | Synthetic store, CLI, MCP, catalog and page checks |
| cmd/o-story/stories/questions | Page stories run by TestQuestionStories |

All changes belong to o-questions: it owns these domain records, policy and
projections. Ochazuke v0.77.0 supplies the shell, forms, motions, chrome, modals,
settings, catalog and responsive wrapped detail Sections. Oden v0.1.1 supplies
command discovery and prefix dispatch.
No sibling or shared library was changed.
