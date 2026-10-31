package main

import (
	"fmt"
	"ofam.dev/ochazuke"
	"slices"
)

// The same commit-on-change editor handles question text, titles and entries.
// Creation occurs on the first valid debounce; later writes update that record.
type textAction struct {
	param, label, entry string
	question            bool
}

var textActions = map[string]textAction{
	"ask": {"question", "Question", "", true}, "edit": {"text", "Question", "", true}, "rename": {"title", "Title", "", false}, "answer": {"answer", "Answer", "answer", false}, "follow-up": {"text", "Follow-up", "follow-up", false},
}

func (a *App) interactive(verb string, c ochazuke.Ctx) (ochazuke.Result, bool) {
	if spec, ok := textActions[verb]; ok && ochazuke.StringArg(c.Args, spec.param, "") == "" {
		return a.textEditor(verb, spec, c), true
	}
	if verb == "associate" && ochazuke.StringArg(c.Args, "project", "") == "" && ochazuke.StringArg(c.Args, "task", "") == "" {
		q, e := a.target(c)
		if e != nil {
			return result(nil, e), true
		}
		_, e = a.r.OpenRecordEditor("question-association", ochazuke.RecordSpec{Title: "Associate question", Fields: []ochazuke.RecordField{{Name: "project", Parameter: param("Project ID")}, {Name: "task", Parameter: param("Task ID")}}, Insert: true, Load: func() (ochazuke.Record, error) { return ochazuke.Record{"project": q.Project.ID, "task": q.Task}, nil }, Save: func(w ochazuke.RecordWrite) ochazuke.Result {
			updated, e := a.store.Annotate(q.ID, Annotation{Project: w.Values["project"], Task: w.Values["task"]})
			a.refresh()
			return result(updated, e)
		}})
		return result(nil, e), true
	}
	if verb == "open" && ochazuke.StringArg(c.Args, "link", "") == "" {
		q, e := a.target(c)
		if e != nil {
			return result(nil, e), true
		}
		links := append([]Link{}, q.Links...)
		if q.Source.Transcript != "" {
			links = append(links, Link{Target: q.Source.Transcript, Label: "Transcript"})
		}
		var rows []ochazuke.ChooserRow
		for _, l := range links {
			rows = append(rows, ochazuke.ChooserRow{ID: l.Target, Label: l.Label + " " + l.Target})
		}
		_, e = a.r.OpenChooser(ochazuke.ChooserSpec{Kind: "question-links", Title: "Open link", Rows: rows, Choose: func(row ochazuke.ChooserRow) ochazuke.Result {
			return a.handle("open", ochazuke.Ctx{Runtime: a.r, Args: ochazuke.Arguments{"target": q.ID, "link": row.ID}})
		}})
		return result(nil, e), true
	}
	return ochazuke.Result{}, false
}
func (a *App) textEditor(verb string, spec textAction, c ochazuke.Ctx) ochazuke.Result {
	var q Question
	var e error
	if verb != "ask" {
		q, e = a.target(c)
		if e != nil {
			return result(nil, e)
		}
		if e = a.store.permit(verb, q); e != nil {
			return result(nil, e)
		}
	}
	initial := ""
	if verb == "edit" {
		initial = q.Text
	}
	if verb == "rename" {
		initial = q.Title
	}
	entryID := ""
	key, e := token()
	if e != nil {
		return result(nil, e)
	}
	_, e = a.r.OpenRecordEditor("question-editor", ochazuke.RecordSpec{Title: spec.label, Fields: []ochazuke.RecordField{{Name: "text", Label: spec.label, Parameter: required(spec.label), Multiline: verb != "rename"}}, Insert: true, Rename: verb == "rename", Load: func() (ochazuke.Record, error) { return ochazuke.Record{"text": initial}, nil }, Save: func(w ochazuke.RecordWrite) ochazuke.Result {
		text := w.Values["text"]
		switch {
		case verb == "ask" && q.ID == "":
			v, err := a.store.Ask(Ask{Question: text, Key: "editor:" + key})
			q, e = v.Question, err
		case spec.question:
			q, e = a.store.Change("edit", q.ID, Change{Value: text})
		case verb == "rename":
			q, e = a.store.Change("rename", q.ID, Change{Annotation: Annotation{Title: text}})
		case entryID == "":
			q, e = a.store.Append(q.ID, Entry{Kind: spec.entry, Text: text})
			if e == nil {
				entryID = q.Entries[len(q.Entries)-1].ID
			}
		default:
			q, e = a.store.mutate("edit", q.ID, func(q *Question, _ *[]Question) error {
				idx := slices.IndexFunc(q.Entries, func(en Entry) bool { return en.ID == entryID })
				if idx < 0 {
					return fmt.Errorf("entry not found")
				}
				q.Entries[idx].Text = text
				return a.store.prepareEntry(q.ID, &q.Entries[idx])
			})
		}
		a.refresh()
		return result(q, e)
	}})
	return result(nil, e)
}
