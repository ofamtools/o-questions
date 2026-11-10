package main

import (
	"slices"
	"strings"

	"ofam.dev/ochazuke"
)

// A decision row's state, for colour, markers, the filter and the sort. One
// table drives all four: the state names, the marker glyph (so a mono theme
// and NO_COLOR still tell them apart), the tone ochazuke's themes already
// define, and the sort rank (undecided first).
const (
	stateUndecided = "undecided"
	stateAnswered  = "answered"
	stateDiffers   = "differs"
	stateAsked     = "asked"
	stateLater     = "later"
	stateApplied   = "applied"
	stateFailed    = "failed"
	stateArchived  = "archived"
)

// differsMark is the glyph for "not what the agent recommended".
const differsMark = "≠"

type stateMark struct {
	name, glyph, tone, meaning string
	rank                       int
}

// stateMarks: undecided is amber (it needs you), answered green, answered
// differently from the recommendation blue, asked back the accent, later and
// applied quiet, failed red, archived dim. The order is the legend's and the
// sort rank.
var stateMarks = []stateMark{
	{stateUndecided, "○", ochazuke.ToneNeedsInput, "new, not decided", 0},
	{stateAnswered, "✓", ochazuke.ToneDone, "answered", 5},
	{stateDiffers, differsMark, ochazuke.ToneWorking, "answered differently from the recommendation", 4},
	{stateAsked, "?", ochazuke.ToneQueued, "asked the agent", 1},
	{stateLater, "…", ochazuke.ToneIdle, "later", 2},
	{stateApplied, "●", ochazuke.ToneExited, "applied", 6},
	{stateFailed, "✗", ochazuke.ToneErrored, "applying failed", 3},
	{stateArchived, "▫", ochazuke.ToneExited, "archived", 7},
}

// markOf is the marker row of a state.
func markOf(state string) stateMark {
	i := slices.IndexFunc(stateMarks, func(m stateMark) bool { return m.name == state })
	if i < 0 {
		return stateMark{}
	}
	return stateMarks[i]
}

// legend is one line naming every marker.
func legend() string {
	var parts []string
	for _, m := range stateMarks {
		parts = append(parts, m.glyph+" "+m.meaning)
	}
	return strings.Join(parts, " · ")
}

// differs says whether a decided question was answered differently from the
// agent's recommendation: the chosen option is not the recommended one (a
// write-in, later or moot differ too), or a checklist mark is not the
// recommended one. ok is false when there is nothing to compare: undecided,
// or the agent recommended nothing.
func (r *Decision) differs() (differs, ok bool) {
	if !r.decided() {
		return false, false
	}
	rec := r.recommendedID()
	if rec == "" && !r.recommendsItems() {
		return false, false
	}
	if rec != "" && r.Action != rec {
		return true, true
	}
	for _, it := range r.Items {
		if r.recommendsItems() && r.checked(it.ID) != it.Recommended {
			return true, true
		}
	}
	return false, true
}

// itemDiffers: the item's mark is not the agent's recommendation, once the
// checklist is decided.
func (r *Decision) itemDiffers(it Item) bool {
	return r.decided() && r.recommendsItems() && r.checked(it.ID) != it.Recommended
}

// decisionState is the question's state: archived, applied, failed, asked
// back, later, undecided, answered differently from the recommendation, or
// answered.
func (s *Store) decisionState(q Question) string {
	r := q.Decision
	switch {
	case q.Deleted:
		return stateArchived
	case r.locked():
		return stateApplied
	case r.Status == decisionFailed:
		return stateFailed
	}
	switch s.decisionClass(q) {
	case classAsked:
		return stateAsked
	case classLater:
		return stateLater
	case classUndecided:
		return stateUndecided
	}
	if d, _ := r.differs(); d {
		return stateDiffers
	}
	return stateAnswered
}

// versus is the detail's "compared with the recommendation" line.
func (s *Store) versus(q Question) string {
	r := q.Decision
	switch d, ok := r.differs(); {
	case !r.decided():
		return "not decided yet"
	case !ok:
		return "the agent recommended nothing"
	case d:
		return differsMark + " differs from the recommendation"
	}
	return "agrees with the recommendation"
}

// markedCell is the Decision cell with the state's glyph in front. The
// applied tick the plain cell carries is the glyph's job here.
func (s *Store) markedCell(q Question) (string, string) {
	st := s.decisionState(q)
	cell := s.decisionCell(q)
	cell = strings.TrimPrefix(cell, "✓ ")
	if strings.HasPrefix(cell, "! ") {
		cell = "! " + strings.TrimPrefix(cell[2:], "✓ ")
	}
	return markOf(st).glyph + " " + cell, st
}

// stateTones are the spans that colour a question's cells (the state and
// title cells): tones from the theme, so a themes.json recolours them.
func stateTones(state string, cells ...int) []ochazuke.ToneSpan {
	var out []ochazuke.ToneSpan
	for _, c := range cells {
		out = append(out, ochazuke.CellTone(c, markOf(state).tone))
	}
	return out
}

// colored is the decision.color setting: marker glyphs and tones on the rows.
func (a *App) colored() bool { return a.r.Settings().Bool("decision.color") }

// rank orders states for decision.sort undecided-first.
func rank(state string) int { return markOf(state).rank }

// orderQuestions keeps topics in order of first appearance and, with the
// undecided-first sort, puts what needs the owner first inside a topic.
func (s *Store) orderQuestions(qs []Question, sortBy string) []Question {
	out := sortByGroup(qs)
	if sortBy != "undecided-first" {
		return out
	}
	order := groupOrder(qs)
	slices.SortStableFunc(out, func(a, b Question) int {
		if d := slices.Index(order, groupOf(a)) - slices.Index(order, groupOf(b)); d != 0 {
			return d
		}
		return rank(s.decisionState(a)) - rank(s.decisionState(b))
	})
	return out
}

// ownerQuestionsWith lists the questions for the owner; archived ones only
// when asked for (hidden by default, like every archived question).
func (s *Store) ownerQuestionsWith(archived bool) ([]Question, error) {
	rows, e := s.records(archived)
	if e != nil {
		return nil, e
	}
	return slices.DeleteFunc(rows, func(q Question) bool { return !q.forOwner() }), nil
}

// ArchiveMany archives questions in one store change (one history entry, so
// one undo brings them all back). It returns the questions archived.
func (s *Store) ArchiveMany(ids []string) (n int, err error) {
	if len(ids) == 0 {
		return 0, nil
	}
	err = s.locked(func() error {
		rows, e := s.all()
		if e != nil {
			return e
		}
		before := clone(rows)
		want := map[string]bool{}
		for _, id := range ids {
			want[id] = true
		}
		now := s.now()
		for i := range rows {
			if !want[rows[i].ID] || rows[i].Deleted {
				continue
			}
			if e = s.permit("archive", rows[i]); e != nil {
				return e
			}
			rows[i].Deleted, rows[i].Updated = true, now
			n++
		}
		if n == 0 {
			return nil
		}
		return s.commit(before, rows, "archive")
	})
	return
}

// ArchivePlan is what an archive-matching call found; Done says it was archived.
type ArchivePlan struct {
	Count     int              `json:"count"`
	Done      bool             `json:"done"`
	Show      string           `json:"show,omitempty"`
	Group     string           `json:"group,omitempty"`
	OlderThan int              `json:"older_than_days,omitempty"`
	Questions []ArchivePlanRow `json:"questions"`
	Note      string           `json:"note,omitempty"`
}
type ArchivePlanRow struct {
	ID    string `json:"id"`
	Title string `json:"title"`
	Group string `json:"group"`
	State string `json:"state"`
	Age   int    `json:"age_days"`
}

// ArchiveMatching archives (apply) or lists (dry run) the visible questions
// for the owner that are in the show filter's state, in the group (empty:
// any) and last touched more than days days ago (0: any age).
func (s *Store) ArchiveMatching(show, group string, days int, apply bool) (ArchivePlan, error) {
	plan := ArchivePlan{Show: show, Group: group, OlderThan: days, Questions: []ArchivePlanRow{}}
	qs, e := s.ownerQuestionsWith(false)
	if e != nil {
		return plan, e
	}
	now := s.now()
	var ids []string
	for _, q := range s.orderQuestions(qs, "group") {
		if group != "" && groupOf(q) != group {
			continue
		}
		if show != "" && !s.decisionShown(q, show) {
			continue
		}
		age := int(now.Sub(q.Updated).Hours() / 24)
		if days > 0 && age < days {
			continue
		}
		plan.Questions = append(plan.Questions, ArchivePlanRow{ID: q.ID, Title: q.Title, Group: groupOf(q), State: s.decisionState(q), Age: age})
		ids = append(ids, q.ID)
	}
	plan.Count = len(ids)
	if !apply {
		plan.Note = "dry run: nothing archived; repeat with apply to archive these (undo brings them back)"
		return plan, nil
	}
	n, e := s.ArchiveMany(ids)
	plan.Count, plan.Done = n, e == nil
	return plan, e
}
