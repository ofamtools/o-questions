package main

import (
	"slices"

	"ofam.dev/ochazuke"
)

// The question and review nouns are thing nouns (DESIGN-001).
//
// question is a store over the question records: list and get read them
// (list includes archived ones, which the archive-role prop marks), set
// writes title (the label: the old rename) and archived (the archive role:
// archive and restore), delete hides a record, as question.delete always
// did (agents archive instead). The named verbs stay, replacing the
// generated ones of the same name where they exist: list (filters), edit
// (the question's text), delete, archive, restore (a backup file too) and
// rename (bound to r on the page).
//
// review is the owner's view of the same records (the questions for the
// owner), read only: a facts store; its verbs are question verbs under
// review names (reviewAliases).

func questionStoreProps() []ochazuke.PropSpec {
	text := func(name, desc string) ochazuke.PropSpec {
		return ochazuke.PropSpec{Name: name, ReadOnly: true, Parameter: ochazuke.Parameter{Type: "string", Description: desc}}
	}
	return []ochazuke.PropSpec{
		{Name: "title", Role: ochazuke.RoleLabel, Parameter: ochazuke.Parameter{Type: "string", Description: "The question's short title"}},
		{Name: "archived", Role: ochazuke.RoleArchive, Parameter: ochazuke.Parameter{Type: ochazuke.SettingBoolean, Description: "Hidden from the default views; the record and its history are kept"}},
		text("text", "The owner's question (question edit changes it)"),
		text("state", "open, answered, follow-up or moot (question status sets it)"),
		{Name: "starred", ReadOnly: true, Parameter: ochazuke.Parameter{Type: ochazuke.SettingBoolean, Description: "Starred (question star toggles it)"}},
		{Name: "tags", ReadOnly: true, Parameter: ochazuke.Parameter{Type: ochazuke.SettingList, Description: "Tags (question tag and untag change them)"}},
		text("asked_at", "When it was asked (RFC 3339)"),
	}
}

func questionThing(q Question) ochazuke.Thing {
	t := ochazuke.Thing(questionRowData(q))
	t["state"] = q.Status
	delete(t, "status")
	t["archived"] = q.Deleted
	if q.Tags == nil {
		t["tags"] = []string{}
	}
	return t
}

func (a *App) questionStore() ochazuke.Store {
	return ochazuke.Store{Kind: ochazuke.Collection,
		Get: func(_ ochazuke.Ctx, id string) (ochazuke.Thing, error) {
			q, e := a.store.Resolve(id, false)
			if e != nil {
				return nil, e
			}
			return questionThing(q), nil
		},
		List: func(ochazuke.Ctx, ochazuke.ListQuery) ([]ochazuke.Thing, error) {
			qs, e := a.store.records(true)
			if e != nil {
				return nil, e
			}
			if a.newestFirst() {
				slices.Reverse(qs)
			}
			out := make([]ochazuke.Thing, len(qs))
			for i, q := range qs {
				out[i] = questionThing(q)
			}
			return out, nil
		},
		Patch: func(_ ochazuke.Ctx, id string, t ochazuke.Thing) (ochazuke.Thing, error) {
			q, e := a.store.Resolve(id, false)
			if e != nil {
				return nil, e
			}
			defer a.refresh()
			if v, ok := t["title"].(string); ok && v != q.Title {
				if q, e = a.store.Change("rename", q.ID, Change{Annotation: Annotation{Title: v}}); e != nil {
					return nil, e
				}
			}
			if v, ok := t["archived"].(bool); ok && v != q.Deleted {
				action := "restore"
				if v {
					action = "archive"
				}
				if q, e = a.store.Change(action, q.ID, Change{}); e != nil {
					return nil, e
				}
			}
			return questionThing(q), nil
		},
		Delete: func(_ ochazuke.Ctx, id string) error {
			q, e := a.store.Resolve(id, false)
			if e != nil {
				return e
			}
			_, e = a.store.Change("delete", q.ID, Change{})
			a.refresh()
			return e
		},
	}
}

func (a *App) reviewStore() ochazuke.Store {
	return ochazuke.Store{Kind: ochazuke.Collection, Facts: true,
		Get: func(_ ochazuke.Ctx, id string) (ochazuke.Thing, error) {
			q, e := a.store.Resolve(id, false)
			if e != nil {
				return nil, e
			}
			return questionThing(q), nil
		},
		List: func(ochazuke.Ctx, ochazuke.ListQuery) ([]ochazuke.Thing, error) {
			qs, e := a.store.ownerQuestions()
			if e != nil {
				return nil, e
			}
			out := make([]ochazuke.Thing, len(qs))
			for i, q := range qs {
				out[i] = questionThing(q)
			}
			return out, nil
		},
	}
}
