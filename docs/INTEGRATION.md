# Catalog and host integration

The catalog publishes one question noun and one split Questions page. The
standalone and published catalog declare the same read-only Sections, rendered
by ochazuke v0.67.0 without a host factory. The question is Markdown; the thread
reads existing `entries` with `body: text`, `author: by.kind`, and `at: at`.
These entries contain the complete hydrated discussion and do not repeat the
original question. Metadata uses nested row paths in a side region; no display
projections or app renderer changes are needed. There is no `answered_at` row
field: each entry shows its timestamp, and metadata labels `updated` as Updated.

Sections supersede the standalone preview kind. The generic record `fields`
remain declared for `detail.sections=false` and older hosts. `detail.side=false`
hides metadata; by default it is on the right at detail content width 96 and
below the entire discussion at narrower widths. Answers always wrap fully.
The existing thread modal and commit-on-change editors keep their verbs/views.

Rows expose an absolute `cwd`, plus project/task/source provenance. mtm's existing
extension scope derives project membership from cwd under attached directories;
no Questions-specific host branch is needed. A missing cwd/project remains an
unassociated record. The separate mtm integration driver registers the catalog
against a temp database, checks the actual mounted page and proxy noun, calls
its list and checks bindings. The generic host story mounts and renders the
catalog page. A live pinned-project TUI frame was not exercised: smoke does not
start a daemon or run the TUI event loop. The coordinator should include that
walk in Q6. Never connect to the owner's running server for this check.

The host's generic form behavior remains its own. Missing required arguments now
open the shared argument form in standalone and catalog hosts. The key rows must
omit the argument: an explicit empty string counts as supplied in ochazuke.
`palette.missing-opens-form` defaults to true and opts out of these prompts.
`status` declares the model's required enum; prose declares `required` and `rest`.
Target is optional and the request keeps its original row. It does not declare
`focus`: these catalog rows carry a string identity, with no key prop for the
framework to extract, so focus autofill would submit a shell instance path.

The status choice is opened from its field with Enter; choosing a value then
Enter runs the form. Text fields run with Enter. Escape leaves insert mode first;
Escape in normal mode cancels without invoking the verb. Existing immediate
editors remain available to application handlers; the bound missing-argument
requests now use the common form. Large body revisions retain old files for undo.

### Parameter audit and shared-shell gaps

| Key | Verb | Input the row cannot supply | Declaration / result |
| --- | --- | --- | --- |
| c | question.ask | question | Required Rest; empty preset removed, including the empty-page row. |
| a | question.answer | answer | Required Rest; empty preset removed. |
| f | question.follow-up | text | Required Rest; empty preset removed. |
| s | question.status | status | Required model enum: open, answered, follow-up, moot. |
| e | question.edit | text | Required Rest; empty preset removed. |
| r | question.rename | title | Required Rest; empty preset removed. |
| t | question.tag | tag | Already required; generic form prompts. |
| m | question.associate | project or task | Both are optional alternatives; shared schema cannot require either one. Standalone association editor remains; hosted call still needs library support. |
| g x | question.open | link when zero or several available | Conditional requirement; standalone chooser remains. Hosted call still needs a parameter source scoped to the selected question and conditional prompting. |
| v / Enter | question.show | none | Selected row supplies the question. |
| y | question.copy | none | Row supplies question and yank=true; part defaults to thread. |
| d d | question.delete | none | Selected row supplies the question. |
| u | question.undo | none | Store-wide action. |
| n / N | ochazuke.list.move | none | Rows supply delta/clamp/count. |
| o | ochazuke.setting.cycle | none | Row supplies filter.status. |

Unbound CLI twins `note` and `correct` also declare required Rest prose. Tag,
untag, link, unlink, merge, split and restore already declare their required
values; annotate intentionally has only optional associations/tags/links.
No value is guessed to satisfy an association requirement.

Outstanding work belongs in ochazuke (and the asynchronous mtm proxy integration),
not in a question/key special case. These locations are in ochazuke v0.67.0,
the version pinned here; v0.72.1 pinned by the inspected mtm checkout has the
same relevant form behavior:

- `cli/shell.go:276` calls the runtime directly; `runtime.go:1037` returns
  `status is required` for a missing CLI enum. The CLI does not prompt or list
  that enum on omission. Invalid supplied states already list all valid states.
  `TestCLIParameterMissingPrompts` is skipped until the shell supports it.
- `args_form.go:42` detects absent required fields only. There is no required
  group/conditional requirement or data-driven request to collect optional
  parameters, needed by association and link selection. In mtm,
  `cmd/mtm/extensions.go:924` only adds its pane noun chooser through
  `cmd/mtm/extension_context.go:86`; it cannot supply those generic forms.
- `record_editor.go:389` copies Parameter to RecordField without mapping Rest
  to Multiline. The generic input remains one line despite the prose metadata.
- `args_form.go:148` builds a Submit-only form. `record_editor.go:1077` removes
  it on leave with no recovery storage. Typed cancellation therefore loses the
  unsubmitted input; the skipped `follow-up-recover` story tracks this. This
  also conflicts with the family commit-on-change rule: the general form needs
  a shared record-on-change/recovery contract before it can meet that rule.
- `record_editor.go:1034` closes a form before submitting. Synchronous failures
  reopen with values via `runFailed`, but mtm's proxy returns Pending at
  `cmd/mtm/extensions.go:959`; the later job failure sets status at line 974
  without reopening the form. Host failure recovery needs that shared lifecycle.
- `args_form.go:215` reads an Edits parameter's current value only from a noun
  Store.Get and declared prop. The mounted question noun has neither that
  adapter nor a current-value contract. The enum is complete but its current
  value is not marked; edit/rename likewise start empty. Do not add a status
  chooser branch to compensate.

The regression tests mount a fresh namespaced catalog runtime and proxy the
domain handler, so no standalone editor can hide missing host metadata. This
checks the common host form path; it is not a live mtm server acceptance test.

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
| `wrap`, `linebreak` | `'wrap'`, `'linebreak'` | Similar: visual wrapping preserves source text; Sections always wrap Markdown. |
| `gj`, `gk` | `gj`, `gk` | Similar: J/K scroll rendered detail lines while focus stays in the question list. Unlike Vim's J/K, these do not join lines or look up keywords. |
| `y` | `y` | Similar: y in the metadata list copies the complete field value through shared registers. |
| `:copen` | `quickfix` | n/a: questions are thread records rather than source locations. |
| `input()`, `inputlist()` | `input()`, `inputlist()` | Similar: collect missing text or a declared choice before running a command. Our form is supplied by the shared shell; status has no direct Vim counterpart. |
| `<Esc>` in insert | `i_<Esc>` | Same mode transition first; a second Escape leaves the form. Shared recovery of unsubmitted words is still missing. |

Keys are binding rows. s is status; c is create. Star is available through the
palette/CLI without a default key. j/k, g g/G and / remain framework motions.

The integration drivers remove every inherited TUIOS_* and MTM_* variable from
subprocess environments and pass --socket /tmp/oq-core/dead.sock on every mtm
call. The shared runner asserts the socket path is absent and these variables
are absent before execution. HOME/XDG/config/data are all under /tmp/oq-core.
