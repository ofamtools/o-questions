package main

import (
	"fmt"
	"maps"
	"os"
	"slices"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"ofam.dev/ochazuke/config"
	"ofam.dev/ochazuke/core"
	otea "ofam.dev/ochazuke/tea"
	"ofam.dev/ochazuke/ui"
	"ofam.dev/ochazuke/widgets/form"
	"ofam.dev/ochazuke/widgets/input"
	"ofam.dev/ochazuke/widgets/markdown"
	"ofam.dev/ochazuke/widgets/picker"
	"ofam.dev/ochazuke/widgets/table"
)

// Frame and node IDs.
const (
	pageID       = "main"
	nodeList     = "list"
	nodeReader   = "reader"
	nodeFilter   = "filter"
	modalPalette = "palette"
	modalChoose  = "choose"
	modalHelp    = "help"
	modalForm    = "form"
)

// Table cell keys.
const (
	cellStatus  = "status"
	cellTitle   = "title"
	cellProject = "project"
	cellEntries = "entries"
	cellUpdated = "updated"
	cellText    = "text"
	cellTags    = "tags"
	cellGroup   = "group"
)

var sortKeys = []string{"", cellTitle, cellStatus, cellProject, cellUpdated, cellEntries}
var groupKeys = []string{"", cellStatus, cellProject, cellGroup}

var cellNames = map[string]string{
	"":          "default order",
	cellTitle:   "title",
	cellStatus:  "status",
	cellProject: "project",
	cellUpdated: "updated",
	cellEntries: "replies",
	cellGroup:   "topic",
}

// App is the page state, kept by value in the page frame's Data.
type App struct {
	Questions []Question
	base      []table.Row
	List      table.Model

	Query     string
	QueryLine input.Line
	Match     table.MatchMode
	FilterErr string
	Typing    bool

	StatusFilter  string // "", "open", "answered", "follow-up", "moot"
	ProjectFilter string // "" = all
	DecisionsOnly bool   // true = owner decisions view
	ShowArchived  bool

	Reader    markdown.Model
	ReaderID  string
	ReaderKey string // ID + Updated timestamp to detect edits

	Focus string // nodeList or nodeReader
	Ratio float64

	Status, Tone string
	Register     string
}

func (a App) clone() App {
	a.List = a.List.Copy()
	a.QueryLine.Text = slices.Clone(a.QueryLine.Text)
	return a
}

func appOf(s core.State) App {
	a, _ := s.Pages[pageID].Data.(App)
	return a
}

func withApp(s core.State, a App) core.State {
	f := s.Pages[pageID]
	f.Data = a
	f.FocusID = a.Focus
	if a.Typing {
		f.FocusID = nodeFilter
	}
	s.Pages = maps.Clone(s.Pages)
	s.Pages[pageID] = f
	return s
}

func (a *App) note(msg string) { a.Status, a.Tone = msg, "success" }
func (a *App) info(msg string) { a.Status, a.Tone = msg, "info" }
func (a *App) warn(msg string) { a.Status, a.Tone = msg, "warning" }
func (a *App) fail(msg string) { a.Status, a.Tone = msg, "error" }

// Env is the immutable session configuration shared by handlers and the view.
type Env struct {
	St       *Store
	Km       *core.Keymap
	Bindings []core.Binding
	Specs    []core.CommandSpec
	Theme    ui.Theme
	Match    table.MatchMode
	Ratio    float64
	StackW   int
	Notes    []string
}

// Config is what main (or a test) supplies to build an Env.
type Config struct {
	Bindings []core.Binding
	Theme    *ui.Theme
	Settings *config.Settings
	Notes    []string
}

var defaultSettings = map[string]any{
	"matchMode":  "substring",
	"listRatio":  0.48,
	"stackWidth": 100,
}

var modes = []core.Mode{
	{Name: "normal"},
	{Name: "input"},
	{Name: "palette"},
	{Name: "choose"},
	{Name: "form"},
	{Name: "help"},
}

func newEnv(st *Store, cfg Config) *Env {
	env := &Env{St: st, Theme: ui.DefaultDarkTheme(), Notes: cfg.Notes}
	if cfg.Theme != nil {
		env.Theme = *cfg.Theme
	}
	set := cfg.Settings
	if set == nil {
		set, _ = config.LoadSettings(nil, defaultSettings)
	}
	switch m := table.MatchMode(set.String("matchMode", "substring")); m {
	case table.MatchSubstring, table.MatchFuzzy, table.MatchRegex:
		env.Match = m
	default:
		env.Match = table.MatchSubstring
		env.Notes = append(env.Notes, "settings: matchMode must be substring, fuzzy or regex")
	}
	env.Ratio = min(max(set.Float("listRatio", 0.48), 0.2), 0.8)
	env.StackW = set.Int("stackWidth", 100)
	env.Specs = commandSpecs()
	for _, d := range config.CheckBindings(modes, cfg.Bindings, env.Specs...) {
		if d.Severity == config.SevError {
			env.Notes = append(env.Notes, fmt.Sprintf("bindings.json #%d: %s", d.Index, d.Message))
		}
	}
	env.Bindings = config.MergeBindings(defaultBindings(), cfg.Bindings)
	km, err := core.NewKeymap(modes, env.Bindings, env.Specs...)
	if err != nil && len(env.Notes) == 0 {
		env.Notes = append(env.Notes, "bindings: "+strings.ReplaceAll(err.Error(), "\n", "; "))
	}
	env.Km = km
	return env
}

// --- commands and bindings ------------------------------------------------------

func commandSpecs() []core.CommandSpec {
	user := func(id, desc string) core.CommandSpec {
		return core.CommandSpec{ID: id, Description: desc, Tags: []string{"user"}}
	}
	internal := func(id, desc string) core.CommandSpec { return core.CommandSpec{ID: id, Description: desc} }
	specs := []core.CommandSpec{
		user("filter.open", "Filter questions (title, text, project, tags)"),
		user("filter.mode.cycle", "Cycle filter match mode: substring, fuzzy, regex"),
		user("filter.clear", "Clear active text/status/project filters"),
		user("sort.cycle", "Sort by next column: default, title, status, project, updated, replies"),
		user("sort.reverse", "Reverse sort direction"),
		user("group.cycle", "Group list: status, project, topic, or ungrouped"),
		user("group.collapse", "Collapse or expand focused group"),
		user("filter.status.cycle", "Cycle status filter: all, open, answered, follow-up, moot"),
		user("filter.project.pick", "Filter by project"),
		user("view.decisions.toggle", "Toggle Decisions view (owner decisions vs all questions)"),
		user("archived.toggle", "Show or hide archived questions"),
		user("list.reload", "Reload questions from disk"),
		user("focus.toggle", "Switch focus between list and thread/decision reader"),
		user("focus.reader", "Focus the thread/decision reader"),
		user("focus.list", "Focus the questions list"),
		user("heading.next", "Jump reader to next heading"),
		user("heading.prev", "Jump reader to previous heading"),
		user("question.ask", "Ask a new question"),
		user("question.answer", "Answer the focused question"),
		user("question.followup", "Add a follow-up to the focused question"),
		user("question.decide", "Decide the focused owner decision"),
		user("question.accept", "Accept the recommendation on the focused owner decision"),
		user("question.resolve", "Resolve (mark answered) the focused question"),
		user("question.moot", "Mark the focused question moot"),
		user("question.reopen", "Reopen the focused question"),
		user("question.star", "Star or unstar the focused question"),
		user("question.archive", "Archive or restore the focused question"),
		user("store.undo", "Undo the last store change"),
		user("store.redo", "Redo the last undone store change"),
		user("palette.open", "Open the command palette"),
		user("help.open", "Show active key bindings"),
		user("app.close", "Quit"),

		internal("input.insert", "Type into the filter"),
		internal("input.backspace", "Delete the last filter character"),
		internal("input.accept", "Keep filter and return to list"),
		internal("input.cancel", "Clear filter and return to list"),
		internal("palette.insert", "Type into palette query"),
		internal("palette.backspace", "Delete last palette character"),
		internal("palette.close", "Close palette"),
		internal("choose.insert", "Type into picker query"),
		internal("choose.backspace", "Delete last picker character"),
		internal("choose.close", "Close picker"),
		internal(picker.CmdConfirm, "Choose highlighted item"),
		internal(picker.CmdModeCycle, "Cycle match mode"),
		internal("help.close", "Close help"),
		internal(form.CmdInsert, "Type into form field"),
		internal(form.CmdBackspace, "Delete last character"),
		internal("form.clear", "Clear form field"),
		internal(form.CmdSubmit, "Submit form"),
		internal(form.CmdCancel, "Cancel form"),
	}
	for _, sp := range markdown.Specs() {
		sp.Tags = append(slices.Clone(sp.Tags), "user")
		specs = append(specs, sp)
	}
	return append(specs, input.ReadlineSpecs()...)
}

func b(mode, keys, cmd, hint string) core.Binding {
	return core.Binding{Mode: mode, Keys: strings.Fields(keys), Command: cmd, Hint: hint}
}

func sb(scope, mode, keys, cmd, hint string) core.Binding {
	bd := b(mode, keys, cmd, hint)
	bd.Scope = scope
	return bd
}

func class(mode, cls, cmd string, args map[string]any, hint string) core.Binding {
	return core.Binding{Mode: mode, Class: cls, Command: cmd, Args: args, Hint: hint}
}

var unusedMotions = []string{
	"motion.word.next", "motion.word.prev", "motion.word.end",
	"motion.group.next", "motion.group.prev",
	"motion.left", "motion.right", "motion.start", "motion.end",
}

func standard(mode string) []core.Binding {
	var out []core.Binding
	for _, bd := range core.StandardBindings(mode) {
		if !slices.Contains(unusedMotions, bd.Command) {
			out = append(out, bd)
		}
	}
	return out
}

func defaultBindings() []core.Binding {
	text := map[string]any{"text": "$text"}
	out := []core.Binding{
		b("normal", "/", "filter.open", "filter"),
		b("normal", "alt+m", "filter.mode.cycle", "match mode"),
		sb("list", "normal", "esc", "filter.clear", "clear filter"),
		b("normal", "s", "sort.cycle", "sort"),
		b("normal", "S", "sort.reverse", "reverse sort"),
		b("normal", "g b", "group.cycle", "group"),
		b("normal", "z a", "group.collapse", "fold group"),
		b("normal", "f", "filter.status.cycle", "status filter"),
		b("normal", "g p", "filter.project.pick", "project filter"),
		b("normal", "D", "view.decisions.toggle", "decisions view"),
		b("normal", ".", "archived.toggle", "archived"),
		b("normal", "R", "list.reload", "reload"),
		b("normal", "tab", "focus.toggle", "list / reader"),
		sb("list", "normal", "enter", "focus.reader", "read thread"),
		sb("list", "normal", "l", "focus.reader", "read thread"),
		sb("markdown", "normal", "esc", "focus.list", "back to list"),
		sb("markdown", "normal", "h", "focus.list", "back to list"),
		sb("markdown", "normal", "n", markdown.CmdLinkNext, "next link"),
		sb("markdown", "normal", "N", markdown.CmdLinkPrev, "prev link"),
		sb("markdown", "normal", "ctrl+n", markdown.CmdTOCNext, "next TOC"),
		sb("markdown", "normal", "ctrl+p", markdown.CmdTOCPrev, "prev TOC"),
		sb("markdown", "normal", "g o", markdown.CmdHeadingJump, "jump TOC"),
		b("normal", "t", markdown.CmdTOCToggle, "contents"),
		b("normal", "]", "heading.next", "next heading"),
		b("normal", "[", "heading.prev", "prev heading"),
		b("normal", "a", "question.ask", "ask"),
		b("normal", "A", "question.answer", "answer"),
		b("normal", "F", "question.followup", "follow-up"),
		b("normal", "d", "question.decide", "decide"),
		b("normal", "y", "question.accept", "accept rec"),
		b("normal", "r", "question.resolve", "resolve"),
		b("normal", "m", "question.moot", "moot"),
		b("normal", "o", "question.reopen", "reopen"),
		b("normal", "*", "question.star", "star"),
		b("normal", "x", "question.archive", "archive/restore"),
		b("normal", "u", "store.undo", "undo"),
		b("normal", "ctrl+r", "store.redo", "redo"),
		b("normal", ":", "palette.open", "commands"),
		b("normal", "?", "help.open", "help"),
		b("normal", "q", "app.close", "quit"),
		b("normal", "ctrl+c", "app.close", "quit"),
	}
	out = append(out, standard("normal")...)
	nav := func(mode, next string) []core.Binding {
		return []core.Binding{
			b(mode, "down", "motion.down", next), b(mode, "ctrl+n", "motion.down", next),
			b(mode, "up", "motion.up", "previous"), b(mode, "ctrl+p", "motion.up", "previous"),
			b(mode, "pgdown", "motion.page.down", "page down"), b(mode, "pgup", "motion.page.up", "page up"),
		}
	}
	out = append(out,
		class("input", core.ClassPrintable, "input.insert", text, "type"),
		class("input", core.ClassPaste, "input.insert", text, "paste"),
		b("input", "backspace", "input.backspace", "delete"),
		b("input", "enter", "input.accept", "keep filter"),
		b("input", "esc", "input.cancel", "clear filter"),
		b("input", "ctrl+c", "input.cancel", "clear filter"),
		b("input", "alt+m", "filter.mode.cycle", "match mode"),
		b("input", "tab", "filter.mode.cycle", "match mode"),
	)
	out = append(out, nav("input", "next question")...)
	for _, m := range []string{"palette", "choose"} {
		out = append(out,
			class(m, core.ClassPrintable, m+".insert", text, "type"),
			class(m, core.ClassPaste, m+".insert", text, "paste"),
			b(m, "backspace", m+".backspace", "delete"),
			b(m, "enter", picker.CmdConfirm, "choose"),
			b(m, "esc", m+".close", "close"),
			b(m, "ctrl+c", m+".close", "close"),
			b(m, "alt+m", picker.CmdModeCycle, "match mode"),
			b(m, "tab", picker.CmdModeCycle, "match mode"),
		)
		out = append(out, nav(m, "next")...)
	}
	out = append(out,
		class("form", core.ClassPrintable, form.CmdInsert, text, "type"),
		class("form", core.ClassPaste, form.CmdInsert, text, "paste"),
		b("form", "backspace", form.CmdBackspace, "delete"),
		b("form", "ctrl+u", "form.clear", "clear field"),
		b("form", "tab", "motion.down", "next field"),
		b("form", "down", "motion.down", "next field"),
		b("form", "shift+tab", "motion.up", "previous field"),
		b("form", "up", "motion.up", "previous field"),
		b("form", "enter", form.CmdSubmit, "apply"),
		b("form", "esc", form.CmdCancel, "cancel"),
		b("form", "ctrl+c", form.CmdCancel, "cancel"),

		b("help", "esc", "help.close", "close"),
		b("help", "q", "help.close", "close"),
		b("help", "?", "help.close", "close"),
		b("help", "ctrl+c", "help.close", "close"),
	)
	for _, mode := range []string{"input", "palette", "choose", "form"} {
		taken := map[string]bool{}
		for _, bd := range out {
			if bd.Mode == mode && len(bd.Keys) > 0 {
				taken[strings.Join(bd.Keys, " ")] = true
			}
		}
		for _, rb := range input.ReadlineBindings(mode, "") {
			k := strings.Join(rb.Keys, " ")
			if taken[k] {
				continue
			}
			taken[k] = true
			out = append(out, rb)
		}
	}
	return append(out, standard("help")...)
}

// --- engine -----------------------------------------------------------------------

type fn func(s core.State, a *App, c core.Command) (core.State, []core.Effect)

func (env *Env) wrap(f fn) core.Handler {
	return func(s core.State, c core.Command) core.StepResult {
		a := appOf(s).clone()
		a.Status, a.Tone = "", ""
		env.size(&a, s)
		s, effects := f(s, &a, c)
		env.syncReader(&a)
		env.size(&a, s)
		return core.Next(withApp(s, a), effects...)
	}
}

func (env *Env) size(a *App, s core.State) {
	g := env.geometry(a, s.Width, s.Height)
	a.List.PageSize = g.listRows
	a.Reader.PageSize = g.readerRows
}

func (env *Env) syncReader(a *App) {
	q, ok := a.focusedQuestion()
	if !ok {
		if a.ReaderID != "" || a.Reader.Title == "" {
			a.ReaderID, a.ReaderKey = "", ""
			a.Reader = *markdown.New("Thread", "*No question selected. Press `a` to ask a question.*")
		}
		return
	}
	key := q.ID + "|" + q.Updated.Format(time.RFC3339Nano) + "|" + fmt.Sprintf("%d", len(q.Entries))
	if q.forOwner() && q.Decision != nil {
		key += "|" + q.Decision.Action + "|" + q.Decision.Note + "|" + q.Decision.Status
	}
	if key == a.ReaderKey {
		return
	}
	var src string
	if q.forOwner() {
		src = env.St.docRender(q)
	} else {
		src = thread(q, false)
	}
	ps := a.Reader.PageSize
	toc := a.Reader.ShowTOC
	a.Reader = *markdown.New(q.Title, src)
	a.Reader.PageSize, a.Reader.ShowTOC = ps, toc
	a.ReaderID, a.ReaderKey = q.ID, key
}

func setMode(s core.State, mode string) core.State {
	f := s.Pages[pageID]
	f.Mode = mode
	s.Pages = maps.Clone(s.Pages)
	s.Pages[pageID] = f
	return s
}

func openModal(s core.State, id, mode string, data any) core.State {
	s = closeModal(s, id)
	s.Modals = append(slices.Clone(s.Modals), core.Frame{ID: id, Kind: core.FrameModal, Mode: mode, FocusID: id, Data: data})
	return s
}

func closeModal(s core.State, id string) core.State {
	if i := slices.IndexFunc(s.Modals, func(f core.Frame) bool { return f.ID == id }); i >= 0 {
		s.Modals = slices.Delete(slices.Clone(s.Modals), i, i+1)
	}
	return s
}

func modalData(s core.State, id string) any {
	if i := slices.IndexFunc(s.Modals, func(f core.Frame) bool { return f.ID == id }); i >= 0 {
		return s.Modals[i].Data
	}
	return nil
}

func setModalData(s core.State, id string, data any) core.State {
	if i := slices.IndexFunc(s.Modals, func(f core.Frame) bool { return f.ID == id }); i >= 0 {
		s.Modals = slices.Clone(s.Modals)
		s.Modals[i].Data = data
	}
	return s
}

func dispatch(c core.Command) core.Effect { return core.CustomEffect{Kind: "dispatch", Data: c} }

var motions = []string{
	"motion.down", "motion.up", "motion.top", "motion.bottom",
	"motion.page.down", "motion.page.up", "motion.half.down", "motion.half.up",
	"motion.block.next", "motion.block.prev",
}

func newEngine(env *Env) *core.Engine {
	e := core.NewEngine(env.Km)
	h := func(id string, f fn) { e.Handle(id, env.wrap(f)) }
	scoped := func(scope, id string, f fn) { e.HandleScope(scope, id, env.wrap(f)) }

	for _, id := range motions {
		scoped("list", id, env.listMotion)
		scoped("markdown", id, env.readerCmd)
		scoped("palette", id, env.pickCmd(modalPalette))
		scoped("choose", id, env.pickCmd(modalChoose))
		scoped("help", id, env.helpScroll)
		scoped("form", id, env.formCmd)
	}
	for _, sp := range markdown.Specs() {
		h(sp.ID, env.readerCmd)
	}
	h("heading.next", func(s core.State, a *App, c core.Command) (core.State, []core.Effect) {
		c.ID = "motion.block.next"
		return env.readerCmd(s, a, c)
	})
	h("heading.prev", func(s core.State, a *App, c core.Command) (core.State, []core.Effect) {
		c.ID = "motion.block.prev"
		return env.readerCmd(s, a, c)
	})

	h("filter.open", func(s core.State, a *App, c core.Command) (core.State, []core.Effect) {
		a.Typing, a.Focus = true, nodeList
		return setMode(s, "input"), nil
	})
	h("filter.mode.cycle", func(s core.State, a *App, c core.Command) (core.State, []core.Effect) {
		m := table.Model{MatchMode: a.Match}
		m.CycleMatchMode()
		a.Match = m.MatchMode
		a.reproject()
		a.info("match mode: " + string(a.Match))
		return s, nil
	})
	h("filter.clear", func(s core.State, a *App, c core.Command) (core.State, []core.Effect) {
		if a.Query == "" && a.StatusFilter == "" && a.ProjectFilter == "" {
			return s, nil
		}
		a.Query, a.StatusFilter, a.ProjectFilter = "", "", ""
		a.reproject()
		a.info("filters cleared")
		return s, nil
	})
	lineEdit := func(s core.State, a *App, c core.Command) (core.State, []core.Effect) {
		a.QueryLine.Sync(a.Query)
		var changed bool
		switch c.ID {
		case "input.insert":
			changed = a.QueryLine.Insert(c.Arg("text"))
		default:
			changed, _ = a.QueryLine.HandleCommand(c)
		}
		if changed {
			a.Query = a.QueryLine.Value()
			a.reproject()
		}
		return s, nil
	}
	h("input.insert", lineEdit)
	for _, id := range input.ReadlineCommandIDs() {
		h(id, lineEdit)
	}
	h("input.accept", func(s core.State, a *App, c core.Command) (core.State, []core.Effect) {
		a.Typing = false
		return setMode(s, "normal"), nil
	})
	h("input.cancel", func(s core.State, a *App, c core.Command) (core.State, []core.Effect) {
		a.Typing, a.Query = false, ""
		a.reproject()
		return setMode(s, "normal"), nil
	})

	h("sort.cycle", func(s core.State, a *App, c core.Command) (core.State, []core.Effect) {
		i := (slices.Index(sortKeys, a.List.SortField) + 1) % len(sortKeys)
		a.List.SortField = sortKeys[i]
		a.List.SortDir = table.SortAsc
		if a.List.SortField == "" {
			a.List.SortDir = table.SortNone
		}
		a.reproject()
		a.info(sortLabel(a))
		return s, nil
	})
	h("sort.reverse", func(s core.State, a *App, c core.Command) (core.State, []core.Effect) {
		if a.List.SortField == "" {
			a.List.SortField, a.List.SortDir = cellUpdated, table.SortAsc
		} else if a.List.SortDir == table.SortAsc {
			a.List.SortDir = table.SortDesc
		} else {
			a.List.SortDir = table.SortAsc
		}
		a.reproject()
		a.info(sortLabel(a))
		return s, nil
	})
	h("group.cycle", func(s core.State, a *App, c core.Command) (core.State, []core.Effect) {
		keep, onRow := a.focusedID()
		a.List.GroupField = groupKeys[(slices.Index(groupKeys, a.List.GroupField)+1)%len(groupKeys)]
		a.List.Collapsed = nil
		if a.List.GroupField == "" {
			a.info("not grouped")
		} else {
			a.info("grouped by " + cellNames[a.List.GroupField])
		}
		if !onRow || !a.focusRow(keep) {
			a.List.SelectedIdx = 0
		}
		a.List.Focused()
		return s, nil
	})
	h("group.collapse", func(s core.State, a *App, c core.Command) (core.State, []core.Effect) {
		if a.List.GroupField == "" {
			a.warn("not grouped — g b groups the list")
			return s, nil
		}
		a.List.ToggleGroup()
		return s, nil
	})
	h("filter.status.cycle", env.cycleStatusFilter)
	h("filter.project.pick", env.projectPick)
	h("view.decisions.toggle", func(s core.State, a *App, c core.Command) (core.State, []core.Effect) {
		a.DecisionsOnly = !a.DecisionsOnly
		env.reload(a, "")
		if a.DecisionsOnly {
			a.info("Decisions view (owner decisions only)")
		} else {
			a.info("All questions view")
		}
		return s, nil
	})
	h("archived.toggle", func(s core.State, a *App, c core.Command) (core.State, []core.Effect) {
		a.ShowArchived = !a.ShowArchived
		env.reload(a, "")
		if a.ShowArchived {
			a.info("showing archived questions")
		} else {
			a.info("archived questions hidden")
		}
		return s, nil
	})
	h("list.reload", func(s core.State, a *App, c core.Command) (core.State, []core.Effect) {
		env.reload(a, "")
		a.note(fmt.Sprintf("%d questions", len(a.Questions)))
		return s, nil
	})
	h("focus.toggle", func(s core.State, a *App, c core.Command) (core.State, []core.Effect) {
		if a.Focus == nodeReader {
			a.Focus = nodeList
		} else {
			a.Focus = nodeReader
		}
		return s, nil
	})
	h("focus.reader", func(s core.State, a *App, c core.Command) (core.State, []core.Effect) {
		if _, ok := a.focusedQuestion(); !ok {
			a.warn("no question selected")
			return s, nil
		}
		a.Focus = nodeReader
		return s, nil
	})
	h("focus.list", func(s core.State, a *App, c core.Command) (core.State, []core.Effect) {
		a.Focus = nodeList
		return s, nil
	})

	h("question.ask", env.openForm("ask"))
	h("question.answer", env.openForm("answer"))
	h("question.followup", env.openForm("followup"))
	h("question.decide", env.openForm("decide"))
	h("question.accept", env.acceptDecision)
	h("question.resolve", env.resolveQuestion(false))
	h("question.moot", env.resolveQuestion(true))
	h("question.reopen", env.reopenQuestion)
	h("question.star", env.toggleStar)
	h("question.archive", env.toggleArchive)
	h("store.undo", env.undoStore)
	h("store.redo", env.redoStore)

	for _, id := range append([]string{form.CmdInsert, form.CmdBackspace, "form.clear", form.CmdSubmit, form.CmdCancel}, input.ReadlineCommandIDs()...) {
		scoped("form", id, env.formCmd)
	}
	h("palette.open", env.paletteOpen)
	for _, id := range append([]string{"palette.insert", "palette.backspace", "palette.close", picker.CmdConfirm, picker.CmdModeCycle}, input.ReadlineCommandIDs()...) {
		scoped("palette", id, env.pickCmd(modalPalette))
	}
	for _, id := range append([]string{"choose.insert", "choose.backspace", "choose.close", picker.CmdConfirm, picker.CmdModeCycle}, input.ReadlineCommandIDs()...) {
		scoped("choose", id, env.pickCmd(modalChoose))
	}
	h("help.open", func(s core.State, a *App, c core.Command) (core.State, []core.Effect) {
		return openModal(s, modalHelp, "help", 0), nil
	})
	h("help.close", func(s core.State, a *App, c core.Command) (core.State, []core.Effect) {
		return closeModal(s, modalHelp), nil
	})
	h("app.close", func(s core.State, a *App, c core.Command) (core.State, []core.Effect) {
		return s, []core.Effect{core.QuitEffect{}}
	})
	return e
}

func sortLabel(a *App) string {
	if a.List.SortField == "" || a.List.SortDir == table.SortNone {
		return "default order"
	}
	dir := "ascending"
	if a.List.SortDir == table.SortDesc {
		dir = "descending"
	}
	return "sorted by " + cellNames[a.List.SortField] + " " + dir
}

func (env *Env) listMotion(s core.State, a *App, c core.Command) (core.State, []core.Effect) {
	if (c.ID == "motion.bottom" || c.ID == "motion.top") && c.Count > 0 {
		a.List.SelectedIdx = c.Count - 1
		a.List.Focused()
		return s, nil
	}
	a.List.HandleCommand(c)
	return s, nil
}

func (env *Env) readerCmd(s core.State, a *App, c core.Command) (core.State, []core.Effect) {
	if a.ReaderID == "" {
		a.warn("no question in the reader")
		return s, nil
	}
	act, _ := a.Reader.HandleCommand(c)
	switch act.Kind {
	case markdown.ActionYank:
		a.Register = act.Text
		a.info("yanked code block to register")
	case markdown.ActionOpen:
		a.Register = act.Target
		a.info("link: " + act.Target)
	}
	return s, nil
}

// --- shell ---------------------------------------------------------------------------

func newShell(env *Env) *otea.Shell {
	a := App{Match: env.Match, Ratio: env.Ratio, Focus: nodeList}
	a.List = *table.New(listColumns(), nil)
	a.List.MatchMode = env.Match
	env.reload(&a, "")
	env.syncReader(&a)
	if len(env.Notes) > 0 {
		a.warn(strings.Join(env.Notes, " · "))
	}
	initial := withApp(core.NewState(core.Frame{ID: pageID, Mode: "normal"}), a)
	sh := otea.New(newEngine(env), initial, env.view, env.Theme)
	sh.OnPassThrough = func(e core.PassToWidgetEffect) tea.Cmd {
		if e.Mouse == nil {
			return nil
		}
		id := map[string]string{"wheeldown": "motion.down", "wheelup": "motion.up"}[e.Mouse.Button]
		if id == "" {
			return nil
		}
		return func() tea.Msg { return core.CommandEvent{Command: core.Command{ID: id, Count: 3, Source: "mouse"}} }
	}
	sh.OnCustom = func(e core.CustomEffect) tea.Cmd {
		c, ok := e.Data.(core.Command)
		if e.Kind != "dispatch" || !ok {
			return nil
		}
		return func() tea.Msg { return core.CommandEvent{Command: c} }
	}
	return sh
}

// driver feeds a Shell synchronously, without a terminal.
type driver struct{ sh *otea.Shell }

func newDriver(sh *otea.Shell, w, h int) *driver {
	sh.Engine.PrefixTimeout = 0
	sh.MaxFPS = 0
	d := &driver{sh: sh}
	d.send(tea.WindowSizeMsg{Width: w, Height: h})
	return d
}

func (d *driver) send(msg tea.Msg) {
	_, cmd := d.sh.Update(msg)
	d.pump(cmd)
}

func (d *driver) pump(cmd tea.Cmd) {
	if cmd == nil {
		return
	}
	done := make(chan tea.Msg, 1)
	go func() { done <- cmd() }()
	var m tea.Msg
	select {
	case m = <-done:
	case <-time.After(5 * time.Second):
		return
	}
	switch m := m.(type) {
	case tea.BatchMsg:
		for _, c := range m {
			d.pump(c)
		}
	case tea.QuitMsg, nil:
	default:
		d.send(m)
	}
}

func (d *driver) keys(names ...string) {
	for _, n := range names {
		d.send(keyMsg(n))
	}
}

func (d *driver) typeText(s string) {
	for _, r := range s {
		if r == ' ' {
			d.send(keyMsg("space"))
		} else {
			d.send(tea.KeyPressMsg{Code: r, Text: string(r)})
		}
	}
}

var namedKeys = map[string]rune{
	"enter": tea.KeyEnter, "esc": tea.KeyEscape, "tab": tea.KeyTab, "backspace": tea.KeyBackspace,
	"up": tea.KeyUp, "down": tea.KeyDown, "left": tea.KeyLeft, "right": tea.KeyRight,
	"pgup": tea.KeyPgUp, "pgdown": tea.KeyPgDown,
}

func keyMsg(n string) tea.KeyPressMsg {
	var mod tea.KeyMod
	for {
		if rest, ok := strings.CutPrefix(n, "ctrl+"); ok && len(rest) > 0 {
			mod, n = mod|tea.ModCtrl, rest
		} else if rest, ok := strings.CutPrefix(n, "alt+"); ok && len(rest) > 0 {
			mod, n = mod|tea.ModAlt, rest
		} else if rest, ok := strings.CutPrefix(n, "shift+"); ok && len(rest) > 0 {
			mod, n = mod|tea.ModShift, rest
		} else {
			break
		}
	}
	if r, ok := namedKeys[n]; ok {
		return tea.KeyPressMsg{Code: r, Mod: mod}
	}
	if n == "space" {
		return tea.KeyPressMsg{Code: tea.KeySpace, Text: " ", Mod: mod}
	}
	r := []rune(n)[0]
	if mod != 0 {
		return tea.KeyPressMsg{Code: r, Mod: mod}
	}
	return tea.KeyPressMsg{Code: r, Text: n}
}

func cwdOrEmpty() string {
	d, _ := os.Getwd()
	return d
}
