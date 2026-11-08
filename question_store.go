package main

import (
	"fmt"
	"slices"

	"ofam.dev/ochazuke"
)

// The question and decision nouns are thing nouns (DESIGN-001).
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
// decision is the owner's view of the same records (the questions for the
// owner), read only: a facts store; its verbs are question verbs under
// decision names (decisionAliases).

func (a *App) questionStoreProps() []ochazuke.PropSpec {
	text := func(name, desc string) ochazuke.PropSpec {
		return ochazuke.PropSpec{Name: name, ReadOnly: true, Parameter: ochazuke.Parameter{Type: "string", Description: desc}}
	}
	return []ochazuke.PropSpec{
		{Name: "title", Role: ochazuke.RoleLabel, Parameter: ochazuke.Parameter{Type: "string", Description: "The question's short title"}},
		{Name: "archived", Role: ochazuke.RoleArchive, Parameter: ochazuke.Parameter{Type: ochazuke.SettingBoolean, Description: "Hidden from the default views; the record and its history are kept"}},
		text("text", "The owner's question (question edit changes it)"),
		{Name: "status", Parameter: ochazuke.Parameter{Type: ochazuke.SettingString, Enum: statuses, Description: "open, answered, follow-up or moot; writing it sets an explicit, sticky status"}},
		{Name: "starred", ReadOnly: true, Parameter: ochazuke.Parameter{Type: ochazuke.SettingBoolean, Description: "Starred (question star toggles it)"}},
		{Name: "tags", ReadOnly: true, Parameter: ochazuke.Parameter{Type: ochazuke.SettingList, Description: "Tags (question tag and untag change them)"}},
		text("asked_at", "When it was asked (RFC 3339)"),
		a.thingRead("thread", "The complete thread as markdown", func(q Question) any { return thread(q, a.r.Settings().Bool("detail.provenance")) }),
		a.thingRead("latest_answer", "The latest answer's text", func(q Question) any { return latestAnswer(q) }),
		a.thingRead("record_path", "The question's record file", func(q Question) any { return a.store.path(q) }),
		{Name: "path", Subject: ochazuke.SubjectCollection, Parameter: ochazuke.Parameter{Type: ochazuke.SettingString, Description: "The store directory"},
			Read: func(ochazuke.Ctx, ochazuke.Thing) (any, error) { return a.store.Dir, nil }},
	}
}

// thingRead is a read-only computed prop of one question: Read finds the
// question from the thing's id when the prop is asked for.
func (a *App) thingRead(name, desc string, from func(Question) any) ochazuke.PropSpec {
	return ochazuke.PropSpec{Name: name, ReadOnly: true, Parameter: ochazuke.Parameter{Type: ochazuke.SettingString, Description: desc},
		Read: func(_ ochazuke.Ctx, t ochazuke.Thing) (any, error) {
			q, e := a.store.Resolve(fmt.Sprint(t["id"]), false)
			if e != nil {
				return nil, e
			}
			return from(q), nil
		}}
}

// decisionStoreProps are the question props the decision noun shares: the
// things' values, not the label, the archive role or a collection's.
func (a *App) decisionStoreProps() []ochazuke.PropSpec {
	var out []ochazuke.PropSpec
	for _, p := range a.questionStoreProps() {
		if p.Role == "" && p.Subject == "" {
			out = append(out, p)
		}
	}
	return out
}

func questionThing(q Question) ochazuke.Thing {
	t := ochazuke.Thing(questionRowData(q))
	t["status"] = q.Status
	t["archived"] = q.Deleted
	if q.Tags == nil {
		t["tags"] = []string{}
	}
	return t
}

func (a *App) questionStore() ochazuke.Store {
	return ochazuke.Store{Kind: ochazuke.KindCollection,
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
				if q, e = a.store.Change(opRename, q.ID, Change{Annotation: Annotation{Title: v}}); e != nil {
					return nil, e
				}
			}
			if v, ok := t["status"].(string); ok && v != q.Status {
				if q, e = a.store.Change(opStatus, q.ID, Change{Value: v}); e != nil {
					return nil, e
				}
			}
			if v, ok := t["archived"].(bool); ok && v != q.Deleted {
				op := opRestore
				if v {
					op = opArchive
				}
				if q, e = a.store.Change(op, q.ID, Change{}); e != nil {
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
			_, e = a.store.Change(opDelete, q.ID, Change{})
			a.refresh()
			return e
		},
	}
}

func (a *App) decisionStore() ochazuke.Store {
	return ochazuke.Store{Kind: ochazuke.KindCollection, Facts: true,
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
