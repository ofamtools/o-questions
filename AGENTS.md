# o-questions

Created by o-proj.

## Plans and todos

Family plans, todos and the roadmap live in `~/hub/o-plans/` (`README.md` is the index). Read it before planning work; add or update items there. Do not add a TODO.md or docs/plans/ to this repo.

## Project notes

- Domain records and policy live in model.go, actor.go, store.go and changes.go.
  Every mutation goes through the store's lock, policy and transaction path.
- app.go's verb specs are the shared CLI/catalog/MCP schema source. MCP has an
  explicit additive allowlist; never expose owner-only actions there.
- All tests use synthetic temporary data. Set GOWORK=off, GOFLAGS=-trimpath,
  HOME/XDG and --data explicitly for manual app runs. Never install in a lane.
- UI keys must remain binding rows. Reuse ochazuke forms and motions; no key
  switches, F-keys, save key or app-side library workarounds.
