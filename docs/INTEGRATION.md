# Catalog and host integration

The catalog publishes one question noun and one split Questions page. The
standalone mount registers a preview kind locally. The published catalog uses
ordinary record detail, so hosts need no custom renderer and show no unknown-kind
warning. A full read-only Markdown projection is Q8, not part of this lane.

Rows expose an absolute `cwd`, plus project/task/source provenance. mtm's existing
extension scope derives project membership from cwd under attached directories;
no Questions-specific host branch is needed. A missing cwd/project remains an
unassociated record. The separate mtm integration driver registers the catalog
against a temp database, checks the actual mounted page and proxy noun, calls
its list and checks bindings. The generic host story mounts and renders the
catalog page. A live pinned-project TUI frame was not exercised: smoke does not
start a daemon or run the TUI event loop. The coordinator should include that
walk in Q6. Never connect to the owner's running server for this check.

The host's generic form behavior remains its own. Standalone question/title/entry
editors write changes after the framework's typing debounce; there is no save key.
Large body revisions get new files, retaining the earlier files for undo.

## Compared with Vim

The comparison was checked against the installed Vim 9.1 help files under
/usr/share/vim/vim91/doc before adding the explicit n/N binding rows.

| Vim form | Help tag | Ours |
| --- | --- | --- |
| `/pattern` | `/` | Similar: filters rows by title, question, complete answers and tags, rather than moving within a buffer. |
| `n`, `N` | `n`, `N` | Similar: move down/up among shown matches; clamp at the edges. No create action uses n. |
| `gx` | `netrw-externapp` | Similar: g x opens a record link; a chooser handles several links. |
| `gf` | `gf` | n/a: source files are links opened by question.open; there is no buffer filename under a cursor. |
| `yy` | `yy` | Similar: y copies the thread; copy --part selects question or answer. |
| `u` | `u` | Similar: undo one store change, including across records; no buffer-local redo tree. |
| `:copen` | `quickfix` | n/a: questions are thread records rather than source locations. |

Keys are binding rows. s is status; c is create. Star is available through the
palette/CLI without a default key. j/k, g g/G and / remain framework motions.

The integration drivers remove every inherited TUIOS_* and MTM_* variable from
subprocess environments and pass --socket /tmp/oq-core/dead.sock on every mtm
call. The shared runner asserts the socket path is absent and these variables
are absent before execution. HOME/XDG/config/data are all under /tmp/oq-core.
