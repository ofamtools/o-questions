package main

import (
	"maps"
	"slices"
	"strconv"
	"strings"

	"ofam.dev/ochazuke/core"
	"ofam.dev/ochazuke/widgets/form"
	"ofam.dev/ochazuke/widgets/picker"
	"ofam.dev/ochazuke/widgets/table"
)

func listColumns() []table.Column {
	return []table.Column{
		{Key: cellStatus, Title: "Status", Width: 12},
		{Key: cellTitle, Title: "Title"},
		{Key: cellProject, Title: "Project", Width: 14},
		{Key: cellEntries, Title: "Re", Width: 4, Align: "right"},
		{Key: cellUpdated, Title: "Updated", Width: 16},
	}
}

func (env *Env) reload(a *App, focusID string) {
	keep := focusID
	if keep == "" {
		keep, _ = a.focusedID()
	}
	rows, err := env.St.records(a.ShowArchived)
	if err != nil {
		a.fail("loading store: " + err.Error())
		return
	}
	if a.DecisionsOnly {
		rows = slices.DeleteFunc(rows, func(q Question) bool { return !q.forOwner() })
		rows = env.St.orderQuestions(rows, "undecided-first")
	} else {
		slices.Reverse(rows) // newest first
	}
	a.Questions = rows
	a.base = make([]table.Row, len(rows))
	for i, q := range rows {
		st := q.Status
		if q.forOwner() {
			st, _ = env.St.markedCell(q)
		}
		if q.Deleted {
			st = "▫ " + st
		}
		title := q.Title
		if q.Starred {
			title = "★ " + title
		}
		grp := ""
		if q.forOwner() {
			grp = groupOf(q)
		}
		a.base[i] = table.Row{
			ID:  q.ID,
			Ref: &core.Ref{Kind: questionKind, ID: q.ID},
			Cells: map[string]any{
				cellStatus:  st,
				cellTitle:   title,
				cellProject: q.Project.Shown(),
				cellEntries: blankZero(len(q.Entries)),
				cellUpdated: q.Updated.Local().Format("2006-01-02 15:04"),
				cellText:    q.Text,
				cellTags:    strings.Join(q.Tags, " "),
				cellGroup:   grp,
			},
		}
	}
	a.reproject()
	if keep != "" {
		a.focusRow(keep)
	}
}

func blankZero(n int) any {
	if n == 0 {
		return ""
	}
	return n
}

func (a *App) reproject() {
	keep, onRow := a.focusedID()
	var rows []table.Row
	for i, r := range a.base {
		q := a.Questions[i]
		if a.StatusFilter != "" && q.Status != a.StatusFilter {
			continue
		}
		if a.ProjectFilter != "" && !strings.EqualFold(q.Project.Shown(), a.ProjectFilter) && !strings.EqualFold(q.Project.ID, a.ProjectFilter) {
			continue
		}
		rows = append(rows, r)
	}
	a.List.Rows = rows
	a.List.Query, a.List.MatchMode = a.Query, a.Match
	a.List.Items()
	a.FilterErr = a.List.FilterErr
	if onRow {
		a.focusRow(keep)
	}
	a.List.Focused()
}

func (a App) filteredCount() int {
	m := a.List.Copy()
	return len(m.Filtered())
}

func (a App) focusedID() (string, bool) {
	r, ok := a.List.FocusedRow()
	return r.ID, ok
}

func (a App) focusedQuestion() (Question, bool) {
	id, ok := a.focusedID()
	if !ok {
		return Question{}, false
	}
	i := slices.IndexFunc(a.Questions, func(q Question) bool { return q.ID == id })
	if i < 0 {
		return Question{}, false
	}
	return a.Questions[i], true
}

func (a *App) focusRow(id string) bool {
	if id == "" {
		return false
	}
	for i, it := range a.List.Items() {
		if !it.Header && it.Row.ID == id {
			a.List.SelectedIdx = i
			a.List.Focused()
			return true
		}
	}
	return false
}

// --- status & project filters --------------------------------------------------

var statusCycle = []string{"", "open", "answered", "follow-up", "moot"}

func (env *Env) cycleStatusFilter(s core.State, a *App, c core.Command) (core.State, []core.Effect) {
	i := (slices.Index(statusCycle, a.StatusFilter) + 1) % len(statusCycle)
	a.StatusFilter = statusCycle[i]
	a.reproject()
	if a.StatusFilter == "" {
		a.info("status filter: all")
	} else {
		a.info("status filter: " + a.StatusFilter)
	}
	return s, nil
}

type pickChoice struct {
	Kind string // project
	M    *picker.Model
}

func (env *Env) projectPick(s core.State, a *App, c core.Command) (core.State, []core.Effect) {
	counts := map[string]int{}
	var projs []string
	for _, q := range a.Questions {
		p := q.Project.Shown()
		if p == "" {
			continue
		}
		if counts[p] == 0 {
			projs = append(projs, p)
		}
		counts[p]++
	}
	slices.Sort(projs)
	items := []picker.Item{{ID: "", Title: "All projects", Subtitle: strconv.Itoa(len(a.Questions)) + " questions"}}
	for _, p := range projs {
		items = append(items, picker.Item{ID: p, Title: p, Subtitle: strconv.Itoa(counts[p]) + " questions"})
	}
	m := picker.New("Filter by project", items)
	m.Scope, m.MatchMode = "choose", picker.MatchMode(a.Match)
	return openModal(s, modalChoose, "choose", pickChoice{Kind: "project", M: m}), nil
}

// --- forms: ask, answer, followup, decide --------------------------------------

type qFormData struct {
	M      *form.Model
	Action string // ask, answer, followup, decide
	Target string
}

func (f qFormData) clone() qFormData {
	m := *f.M
	m.Values = maps.Clone(m.Values)
	m.Errors = maps.Clone(m.Errors)
	f.M = &m
	return f
}

func (env *Env) openForm(action string) fn {
	return func(s core.State, a *App, c core.Command) (core.State, []core.Effect) {
		q, hasQ := a.focusedQuestion()
		str := func(name, desc string, req bool) core.ParamSpec {
			return core.ParamSpec{Name: name, Type: core.ParamString, Required: req, Description: desc}
		}
		var m *form.Model
		target := ""
		switch action {
		case "ask":
			m = form.New("Ask a question", []core.ParamSpec{
				str("question", "question text", true),
				str("title", "short title (optional)", false),
				str("project", "project (optional)", false),
				str("tags", "comma-separated tags (optional)", false),
			}, map[string]any{"question": "", "title": "", "project": a.ProjectFilter, "tags": ""})
		case "answer":
			if !hasQ {
				a.warn("no question selected")
				return s, nil
			}
			target = q.ID
			m = form.New("Answer: "+clip(q.Title, 40), []core.ParamSpec{
				str("answer", "answer text", true),
				str("summary", "short summary (optional)", false),
			}, map[string]any{"answer": "", "summary": ""})
		case "followup":
			if !hasQ {
				a.warn("no question selected")
				return s, nil
			}
			target = q.ID
			m = form.New("Follow-up: "+clip(q.Title, 40), []core.ParamSpec{
				str("text", "follow-up text", true),
			}, map[string]any{"text": ""})
		case "decide":
			if !hasQ || !q.forOwner() {
				a.warn("select an owner decision (press D for Decisions view)")
				return s, nil
			}
			if q.Decision.locked() {
				a.warn("decision is already applied and locked")
				return s, nil
			}
			target = q.ID
			var optIDs []string
			for _, o := range env.St.optionsOf(q.Decision) {
				optIDs = append(optIDs, o.ID)
			}
			for _, o := range env.St.outcomes() {
				if !slices.Contains(optIDs, o.ID) {
					optIDs = append(optIDs, o.ID)
				}
			}
			defAction := q.Decision.Action
			if defAction == "" {
				defAction = q.Decision.recommendedID()
			}
			m = form.New("Decide: "+clip(q.Title, 40), []core.ParamSpec{
				str("action", "choice ("+strings.Join(optIDs, ", ")+")", true),
				str("note", "note or write-in answer (optional)", false),
			}, map[string]any{"action": defAction, "note": q.Decision.Note})
		}
		m.SubmitLabel = "Apply (enter) · esc cancels"
		return openModal(s, modalForm, "form", qFormData{M: m, Action: action, Target: target}), nil
	}
}

func (env *Env) formCmd(s core.State, a *App, c core.Command) (core.State, []core.Effect) {
	old, ok := modalData(s, modalForm).(qFormData)
	if !ok {
		return s, nil
	}
	f := old.clone()
	switch c.ID {
	case "form.clear":
		if i := f.M.ActiveIdx; i >= 0 && i < len(f.M.Specs) {
			f.M.SetValue(f.M.Specs[i].Name, "")
		}
	case form.CmdSubmit:
		res, _ := f.M.HandleCommand(c)
		if !res.Submitted {
			var msgs []string
			for _, name := range res.ErrorFields {
				msgs = append(msgs, name+": "+f.M.Errors[name])
			}
			a.fail("not applied — " + strings.Join(msgs, "; "))
			return setModalData(s, modalForm, f), nil
		}
		val := func(k string) string { v, _ := f.M.Values[k].(string); return strings.TrimSpace(v) }
		switch f.Action {
		case "ask":
			got, err := env.St.Ask(Ask{
				Question: val("question"),
				Title:    val("title"),
				Project:  val("project"),
				Cwd:      cwdOrEmpty(),
				Tags:     splitCSV(val("tags")),
				Source:   sourceFromEnv(env.St.Actor),
			})
			if err != nil {
				a.fail("ask: " + err.Error())
				return setModalData(s, modalForm, f), nil
			}
			env.reload(a, got.Question.ID)
			a.note("asked " + got.Question.ID)
		case "answer":
			got, err := env.St.Append(f.Target, Entry{
				Kind:    "answer",
				Text:    val("answer"),
				Summary: val("summary"),
			})
			if err != nil {
				a.fail("answer: " + err.Error())
				return setModalData(s, modalForm, f), nil
			}
			env.reload(a, got.ID)
			a.note("answered " + got.ID)
		case "followup":
			got, err := env.St.Append(f.Target, Entry{
				Kind: "follow-up",
				Text: val("text"),
			})
			if err != nil {
				a.fail("follow-up: " + err.Error())
				return setModalData(s, modalForm, f), nil
			}
			env.reload(a, got.ID)
			a.note("added follow-up to " + got.ID)
		case "decide":
			ch := DecisionChange{Action: val("action")}
			if note := val("note"); note != "" {
				ch.Note = &note
			}
			got, err := env.St.Decide(f.Target, ch)
			if err != nil {
				a.fail("decide: " + err.Error())
				return setModalData(s, modalForm, f), nil
			}
			env.reload(a, got.ID)
			cell, _ := env.St.markedCell(got)
			a.note("decided " + got.ID + ": " + cell)
		}
		return closeModal(s, modalForm), nil
	default:
		res, _ := f.M.HandleCommand(c)
		if res.Cancelled {
			a.info("cancelled")
			return closeModal(s, modalForm), nil
		}
	}
	return setModalData(s, modalForm, f), nil
}

// --- direct question & store actions -------------------------------------------

func (env *Env) acceptDecision(s core.State, a *App, c core.Command) (core.State, []core.Effect) {
	q, ok := a.focusedQuestion()
	if !ok || !q.forOwner() {
		a.warn("select an owner decision to accept its recommendation")
		return s, nil
	}
	got, err := env.St.Decide(q.ID, DecisionChange{Accept: true})
	if err != nil {
		a.fail(err.Error())
		return s, nil
	}
	env.reload(a, got.ID)
	cell, _ := env.St.markedCell(got)
	a.note("accepted recommendation: " + cell)
	return s, nil
}

func (env *Env) resolveQuestion(moot bool) fn {
	return func(s core.State, a *App, c core.Command) (core.State, []core.Effect) {
		q, ok := a.focusedQuestion()
		if !ok {
			a.warn("no question selected")
			return s, nil
		}
		var got Question
		var err error
		if moot {
			got, err = env.St.Change(opMoot, q.ID, Change{})
		} else {
			got, err = env.St.Change(opStatus, q.ID, Change{Value: "answered"})
		}
		if err != nil {
			a.fail(err.Error())
			return s, nil
		}
		env.reload(a, got.ID)
		a.note(got.ID + " → " + got.Status)
		return s, nil
	}
}

func (env *Env) reopenQuestion(s core.State, a *App, c core.Command) (core.State, []core.Effect) {
	q, ok := a.focusedQuestion()
	if !ok {
		a.warn("no question selected")
		return s, nil
	}
	got, err := env.St.Change(opReopen, q.ID, Change{})
	if err != nil {
		a.fail(err.Error())
		return s, nil
	}
	env.reload(a, got.ID)
	a.note("reopened " + got.ID)
	return s, nil
}

func (env *Env) toggleStar(s core.State, a *App, c core.Command) (core.State, []core.Effect) {
	q, ok := a.focusedQuestion()
	if !ok {
		a.warn("no question selected")
		return s, nil
	}
	got, err := env.St.Change(opStar, q.ID, Change{})
	if err != nil {
		a.fail(err.Error())
		return s, nil
	}
	env.reload(a, got.ID)
	if got.Starred {
		a.note("starred " + got.ID)
	} else {
		a.info("unstarred " + got.ID)
	}
	return s, nil
}

func (env *Env) toggleArchive(s core.State, a *App, c core.Command) (core.State, []core.Effect) {
	q, ok := a.focusedQuestion()
	if !ok {
		a.warn("no question selected")
		return s, nil
	}
	op := opArchive
	if q.Deleted {
		op = opRestore
	}
	got, err := env.St.Change(op, q.ID, Change{})
	if err != nil {
		a.fail(err.Error())
		return s, nil
	}
	env.reload(a, got.ID)
	if got.Deleted {
		a.note("archived " + got.ID + " — . shows archived, u undoes")
	} else {
		a.note("restored " + got.ID)
	}
	return s, nil
}

func (env *Env) undoStore(s core.State, a *App, c core.Command) (core.State, []core.Effect) {
	if err := env.St.Undo(); err != nil {
		a.warn(err.Error())
		return s, nil
	}
	env.reload(a, "")
	a.note("undone")
	return s, nil
}

func (env *Env) redoStore(s core.State, a *App, c core.Command) (core.State, []core.Effect) {
	if err := env.St.Redo(); err != nil {
		a.warn(err.Error())
		return s, nil
	}
	env.reload(a, "")
	a.note("redone")
	return s, nil
}

// --- palette & picker ----------------------------------------------------------

func (env *Env) paletteOpen(s core.State, a *App, c core.Command) (core.State, []core.Effect) {
	keys := keysByCommand(env)
	var items []picker.Item
	for _, spec := range env.Specs {
		if spec.HasTag("user") {
			items = append(items, paletteItem(spec, keys))
		}
	}
	for _, spec := range core.MotionSpecs() {
		if !slices.Contains(unusedMotions, spec.ID) {
			items = append(items, paletteItem(spec, keys))
		}
	}
	m := picker.New("Commands", items)
	m.Scope, m.Placeholder, m.MatchMode = "palette", "type to filter commands · tab cycles match mode", picker.MatchMode(a.Match)
	return openModal(s, modalPalette, "palette", m), nil
}

func paletteItem(spec core.CommandSpec, keys map[string][]string) picker.Item {
	sub := spec.Description
	if k := keys[spec.ID]; len(k) > 0 {
		sub = "[" + strings.Join(k, ", ") + "]  " + sub
	}
	return picker.Item{ID: spec.ID, Title: spec.ID, Subtitle: sub, Keywords: append([]string{spec.Description}, keys[spec.ID]...)}
}

func (env *Env) pickCmd(id string) fn {
	return func(s core.State, a *App, c core.Command) (core.State, []core.Effect) {
		var m picker.Model
		var ch pickChoice
		switch d := modalData(s, id).(type) {
		case *picker.Model:
			m = *d
		case pickChoice:
			ch, m = d, *d.M
		default:
			return s, nil
		}
		store := func() core.State {
			if id == modalPalette {
				return setModalData(s, id, &m)
			}
			ch.M = &m
			return setModalData(s, id, ch)
		}
		switch strings.TrimPrefix(strings.TrimPrefix(c.ID, "palette."), "choose.") {
		case "insert":
			m.AppendQuery(c.Arg("text"))
			return store(), nil
		case "backspace":
			m.BackspaceQuery()
			return store(), nil
		case "close":
			return closeModal(s, id), nil
		}
		m.PageSize = max(s.Height*70/100-4, 1)
		picked, _ := m.HandleCommand(c)
		if m.FilterErr != "" {
			a.warn("regex: " + m.FilterErr)
		}
		if picked == nil {
			return store(), nil
		}
		s = closeModal(s, id)
		if id == modalPalette {
			return s, []core.Effect{dispatch(core.Command{ID: picked.ID, Source: "palette", Actor: core.Actor{Kind: "user"}})}
		}
		if ch.Kind == "project" {
			a.ProjectFilter = picked.ID
			a.reproject()
			if a.ProjectFilter == "" {
				a.info("all projects")
			} else {
				a.info("project: " + a.ProjectFilter)
			}
		}
		return s, nil
	}
}

func (env *Env) helpScroll(s core.State, a *App, c core.Command) (core.State, []core.Effect) {
	off, _ := modalData(s, modalHelp).(int)
	lines := env.helpLines()
	page := max(s.Height*85/100-4, 1)
	step := max(c.Count, 1)
	switch c.ID {
	case "motion.down":
		off += step
	case "motion.up":
		off -= step
	case "motion.page.down":
		off += page
	case "motion.page.up":
		off -= page
	case "motion.half.down":
		off += max(page/2, 1)
	case "motion.half.up":
		off -= max(page/2, 1)
	case "motion.top":
		off = 0
	case "motion.bottom":
		off = max(len(lines)-page, 0)
	}
	off = min(max(off, 0), max(len(lines)-page, 0))
	return setModalData(s, modalHelp, off), nil
}
