package main

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"strings"

	"ofam.dev/ochazuke"
)

// A blocker is something that waits on the owner. It is not a second store:
// an undecided decision is a blocker, and a plain "waiting on you" item an
// agent adds is a one-item checklist decision carrying the blocker tag. A host
// (mtm) merges the blocker.list rows of every tool that declares the noun; each
// row says what kind it is and which operation resolves it, so the host acts
// without knowing the tool.
const blockerNoun = "blocker"

// blockerTag marks a decision an agent added as a plain item.
const blockerTag = "owner-waits"

// blockerGroup is the topic of added items that name none.
const blockerGroup = "Blockers"

// blockerItem is the id of the single checklist item of an added blocker.
const blockerItem = "done"

// Blocker kinds: what the row is, which decides its resolving verb.
const (
	kindDecision = "decision"
	kindItem     = "item"
)

// BlockerRow is the shape every tool's blocker.list rows share (the row's
// Data): id, title, kind, source tool, age, resolve verb. A host merges rows
// of several tools by these fields.
type BlockerRow struct {
	ID      string `json:"id"`
	Title   string `json:"title"`
	Kind    string `json:"kind"`
	Source  string `json:"source"`
	Age     string `json:"age"`
	Created string `json:"created"`
	Group   string `json:"group"`
	Details string `json:"details,omitempty"`
	// Resolve is the operation that resolves the row, qualified with the
	// namespace a host mounts the tool under; ResolveArgs are its arguments.
	Resolve     string         `json:"resolve"`
	ResolveArgs map[string]any `json:"resolve_args"`
}

func (q Question) isBlockerItem() bool {
	for _, t := range q.Tags {
		if t == blockerTag {
			return true
		}
	}
	return false
}

// blockerKind is what the row is and the operation (noun, verb, local to
// this tool) that resolves it: an added item is marked done, a decision is
// decided.
func (q Question) blockerKind() (kind, noun, verb string) {
	if q.isBlockerItem() {
		return kindItem, blockerNoun, "done"
	}
	return kindDecision, decisionNoun, "decide"
}

// Blockers are the owner's open blockers: every undecided decision for the
// owner, and every added item not yet done. Decided, applied, archived and
// asked-back decisions are not waiting on the owner.
func (s *Store) Blockers() ([]Question, error) {
	qs, e := s.ownerQuestions()
	if e != nil {
		return nil, e
	}
	var out []Question
	for _, q := range qs {
		if s.decisionClass(q) == classUndecided {
			out = append(out, q)
		}
	}
	return out, nil
}

func (s *Store) blockerRow(q Question) BlockerRow {
	kind, noun, verb := q.blockerKind()
	return BlockerRow{ID: q.ID, Title: q.Title, Kind: kind, Source: "o-questions", Age: age(q.AskedAt, s.now()), Created: q.AskedAt.Format("2006-01-02T15:04:05Z07:00"),
		Group: groupOf(q), Details: q.Decision.Details, Resolve: Namespace + "." + noun + "." + verb, ResolveArgs: map[string]any{"target": q.ID}}
}

// BlockerAdd records a plain "waiting on you" item as a one-item checklist
// decision. The key is stable: adding it again updates the one item rather
// than duplicating it (pose's key rule); without a key the text is the key.
func (s *Store) BlockerAdd(text, details, key, group string, prov Source) (Posed, error) {
	if strings.TrimSpace(text) == "" {
		return Posed{}, errors.New("text must not be empty")
	}
	if key == "" {
		key = fmt.Sprintf("%x", sha256.Sum256([]byte(text)))[:16]
	}
	if group == "" {
		group = blockerGroup
	}
	return s.Pose(Pose{Question: text, Title: text, Group: group, Details: details, Key: "blocker:" + key, Items: []Item{{ID: blockerItem, Label: text}}, Tags: []string{blockerTag}, Provenance: prov})
}

// BlockerDone marks an added item done through the decision machinery: check
// the item, confirm the list, record it applied. A real decision is not done
// here; it is answered with its own verb.
func (s *Store) BlockerDone(target, result string) (Question, error) {
	q, e := s.Resolve(itemRowQuestion(target), false)
	if e != nil {
		return q, e
	}
	if !q.forOwner() || !q.isBlockerItem() {
		_, noun, verb := Question{}.blockerKind()
		return q, fmt.Errorf("not an added blocker; answer a decision with %s.%s", noun, verb)
	}
	if q.Decision.locked() {
		return q, nil
	}
	if !q.Decision.decided() {
		checked := true
		if _, e = s.Decide(q.ID, DecisionChange{Item: blockerItem, Checked: &checked}); e != nil {
			return q, e
		}
		if _, e = s.Decide(q.ID, DecisionChange{Confirm: true}); e != nil {
			return q, e
		}
	}
	if strings.TrimSpace(result) == "" {
		result = "done"
	}
	return s.Applied(q.ID, "", "done", result)
}

func blockerTarget() ochazuke.Parameter {
	return ochazuke.Parameter{Type: "string", Description: "Blocker id, title or ID prefix; omitted uses the selected row", Noun: blockerNoun, Focus: true, Free: true, Source: blockerNoun + ".list"}
}

// declareBlockerVerbs defines the blocker noun and its verbs. Rows are the
// shared shape (BlockerRow) a host merges across tools.
func declareBlockerVerbs(a *App) {
	n := a.r.Define(blockerNoun)
	n.Props(blockerProps()...)
	n.Store(a.blockerStore())
	n.Columns(ochazuke.Column{Title: "Kind", Width: 9}, ochazuke.Column{Title: "Blocker", Min: 18}, ochazuke.Column{Title: "Source", Max: 14, Priority: 1}, ochazuke.Column{Title: "Age", Width: 7})
	specs := []verbSpec{
		{name: "add", description: "Add a plain 'waiting on you' item (idempotent by key)", params: map[string]ochazuke.Parameter{"text": prose("What the owner needs to do"), "details": param("Context the owner needs"), "key": param("Stable id; adding it again does not duplicate"), "group": param("Topic; default " + blockerGroup)}, positional: []string{"text"}, run: runBlockerAdd},
		{name: "done", description: "Mark an added item done: check it, confirm it, record it applied", params: map[string]ochazuke.Parameter{"target": blockerTarget(), "result": param("What was done; default done")}, positional: []string{"target"}, run: runBlockerDone},
		{name: "show", description: "Read one blocker: its row and its thread", params: map[string]ochazuke.Parameter{"target": blockerTarget()}, positional: []string{"target"}, run: runBlockerShow},
	}
	for _, v := range specs {
		v := v
		b := n.Verb(v.name, func(c ochazuke.Ctx) ochazuke.Result { return a.invoke(v, c) }, v.description).Agent(ochazuke.AgentAllow)
		if len(v.positional) > 0 {
			b.Positional(v.positional...)
		}
		op := a.r.Operations.At(ochazuke.Key(blockerNoun + "." + v.name))
		op.Parameters = v.params
		a.r.Operations.Set(op)
	}
}

func runBlockerAdd(a *App, c ochazuke.Ctx) ochazuke.Result {
	v, e := a.store.BlockerAdd(argStr(c, "text"), argStr(c, "details"), argStr(c, "key"), argStr(c, "group"), sourceFromEnv(a.store.Actor))
	a.refresh()
	if e != nil {
		return result(nil, e)
	}
	return result(a.store.blockerRow(v.Question), nil)
}

var runBlockerDone = onTarget(func(a *App, q Question, c ochazuke.Ctx) ochazuke.Result {
	v, e := a.store.BlockerDone(q.ID, argStr(c, "result"))
	a.refresh()
	return result(v, e)
})

var runBlockerShow = onTarget(func(a *App, q Question, _ ochazuke.Ctx) ochazuke.Result {
	if !q.forOwner() {
		return result(nil, errors.New("not a blocker"))
	}
	return result(map[string]any{"blocker": a.store.blockerRow(q), "class": a.store.decisionClass(q), "thread": thread(q, false)}, nil)
})

// blockerProps are the row fields a host merges across tools.
func blockerProps() []ochazuke.PropSpec {
	text := func(name, desc string) ochazuke.PropSpec {
		return ochazuke.PropSpec{Name: name, ReadOnly: true, Parameter: ochazuke.Parameter{Type: "string", Description: desc}}
	}
	return []ochazuke.PropSpec{
		{Name: "id", Key: true, ReadOnly: true, Parameter: ochazuke.Parameter{Type: "string", Description: "Blocker id"}},
		{Name: "title", Role: ochazuke.RoleLabel, ReadOnly: true, Parameter: ochazuke.Parameter{Type: "string", Description: "What waits on the owner"}},
		{Name: "kind", ReadOnly: true, Parameter: ochazuke.Parameter{Type: "string", Enum: []string{kindDecision, kindItem}, Description: "decision (answer it) or item (an added chore: mark it done)"}},
		text("source", "The tool that holds the blocker"),
		text("age", "How long it has waited"),
		text("created", "When it was added (RFC 3339)"),
		text("group", "Topic"),
		text("details", "Context the owner needs"),
		text("resolve", "The operation that resolves it, qualified with the tool's namespace"),
		{Name: "resolve_args", ReadOnly: true, Parameter: ochazuke.Parameter{Type: "object", Description: "Arguments of the resolving operation"}},
	}
}

func (s *Store) blockerThing(q Question) ochazuke.Thing {
	r := s.blockerRow(q)
	return ochazuke.Thing{"id": r.ID, "title": r.Title, "kind": r.Kind, "source": r.Source, "age": r.Age, "created": r.Created, "group": r.Group, "details": r.Details, "resolve": r.Resolve, "resolve_args": r.ResolveArgs}
}

// blockerStore is a read-only store over Blockers: the framework's get and
// list (filter, where, sort, group-by) are the blocker rows.
func (a *App) blockerStore() ochazuke.Store {
	return ochazuke.Store{Kind: ochazuke.KindCollection, Facts: true,
		Get: func(_ ochazuke.Ctx, id string) (ochazuke.Thing, error) {
			q, e := a.store.Resolve(itemRowQuestion(id), false)
			if e != nil {
				return nil, e
			}
			return a.store.blockerThing(q), nil
		},
		List: func(ochazuke.Ctx, ochazuke.ListQuery) ([]ochazuke.Thing, error) {
			qs, e := a.store.Blockers()
			if e != nil {
				return nil, e
			}
			out := make([]ochazuke.Thing, len(qs))
			for i, q := range qs {
				out[i] = a.store.blockerThing(q)
			}
			return out, nil
		},
	}
}
