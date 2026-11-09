# Code map

| File | Owns |
| --- | --- |
| main.go | Global data override, oden command dispatch, readable question/decision CLI views, ochazuke shell mount |
| model.go | Question, entry, source, links and thread projection |
| actor.go | Entry-point identities, shared catalog/store right declarations, open writes except delete, and framework-override policy |
| atomic.go | Reused slug/token and atomic-file helpers |
| store.go | Process lock, record files, write-ahead recovery, idempotency, attributed history/undo and archived-record resolution; live positional numbers skip archived records |
| changes.go | Store mutations, annotations, merge/split and the shared soft-delete/archive/restore changes |
| backup.go | Daily/manual archives, retention, validated undoable restore |
| check.go | Explicit wording check against transcript user records |
| settings.go | Settings, defaults and validation |
| app.go | Shared verb specs/handlers, transport-bound rights, readable thread result, archive/restore, rows with collection.hide-archived opt-out, search projection |
| pickers.go | The question and decision noun pickers (PickerDef), listed by the catalog |
| mcp.go | Tool allowlist, schemas and JSON-RPC stdio lifecycle |
| page.go | Shared catalog detail Sections, legacy preview kind, binding rows |
| tui.go | Standalone mount, refresh and smoke frame |
| editor.go | Commit-on-change question/title/entry and association editors; pickers |
| thread_view.go | Read-only thread modal and scroll rows |
| decision.go | Questions for the owner: pose, decide, reply, applied, export (mgmt semantics) |
| decision_app.go | Decision verbs, rows (topic, question, checklist item), the Options section, the decision noun's aliases, page and key rows (outcome keys come from decision.outcomes) |
| decision_web.go | decision --web: mgmt session out, decisions back |
| plans.go | Plan "Owner questions" importer (owner_questions.py port) and decisions render |
| setup.go, setup.json | Instruction-note setup items and co-located block skip defaults |
| testdata/plans | Fixture plans for the importer |
| *_test.go | Synthetic store, CLI, MCP, catalog and page checks |
| rights_archive_test.go | Agent archive/restore round trips, retained bytes, attribution, numbering/visibility opt-out and mounted catalog rights/aliases |
| cmd/o-story/stories/questions | Page stories run by TestQuestionStories |

All changes belong to o-questions: it owns these domain records, policy and
projections. Ochazuke v0.78.0 supplies the shell, forms, motions, chrome, modals,
settings, catalog and responsive wrapped detail Sections. Oden v0.1.1 supplies
command discovery and prefix dispatch.
No sibling or shared library was changed.
