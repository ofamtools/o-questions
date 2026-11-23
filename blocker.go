package main

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"strings"
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
