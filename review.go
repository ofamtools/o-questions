// Agent-to-owner questions and their review. The semantics (recommendation
// shown unconfirmed until accepted, numbered actions, an "ask" outcome that
// becomes a thread the agent answers, decisions carried across regenerations
// by a stable key, an export the agent reads, items locked once applied) are
// adapted from ~/hub/ai/utils/mgmt/mgmt.py (build_session, reply, export,
// apply/result). One store: a question for the owner is an ordinary Question
// whose Direction is "to-owner" and whose Review holds the decision.
package main

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"path/filepath"
	"slices"
	"strings"
	"time"
)

// Directions: who must answer. Empty is the original kind, the owner's
// question to an agent.
const (
	toAgent = "to-agent"
	toOwner = "to-owner"
	askOut  = "ask"
)

var directions = []string{"all", toAgent, toOwner}

// defaultActions are a review's outcomes when neither the question nor the
// review.actions setting names them (mgmt's owner-questions actions).
var defaultActions = []string{"yes", "no", "later", "moot"}

// Review states (Review.State): empty is pending; applied is locked.
const (
	reviewApplied = "applied"
	reviewFailed  = "failed"
)

type Review struct {
	Group             string     `json:"group,omitempty"`
	Source            string     `json:"source,omitempty"`
	Details           string     `json:"details,omitempty"`
	Origin            string     `json:"origin,omitempty"`
	Plan              string     `json:"plan,omitempty"`
	Section           string     `json:"section,omitempty"`
	Number            int        `json:"number,omitempty"`
	Recommendation    string     `json:"recommendation,omitempty"`
	RecommendedAction string     `json:"recommended_action,omitempty"`
	Actions           []string   `json:"actions,omitempty"`
	Action            string     `json:"action,omitempty"`
	Note              string     `json:"note,omitempty"`
	DecidedAt         *time.Time `json:"decided_at,omitempty"`
	ContextChanged    bool       `json:"context_changed,omitempty"`
	NewReply          bool       `json:"new_reply,omitempty"`
	State             string     `json:"state,omitempty"`
	Result            string     `json:"result,omitempty"`
	AppliedAt         *time.Time `json:"applied_at,omitempty"`
}

// Pose is an agent's question for the owner. Key is the stable id that
// carries a decision across regenerations; reposing the same key updates the
// question and flags a decided item "context changed" when its text moved.
type Pose struct {
	Question          string   `json:"question"`
	Title             string   `json:"title"`
	Group             string   `json:"group"`
	Source            string   `json:"source"`
	Details           string   `json:"details"`
	Recommendation    string   `json:"recommendation"`
	RecommendedAction string   `json:"recommended_action"`
	Actions           []string `json:"actions"`
	Project           string   `json:"project"`
	Cwd               string   `json:"cwd"`
	Task              string   `json:"task"`
	Tags              []string `json:"tags"`
	Links             []Link   `json:"links"`
	Key               string   `json:"key"`
	Origin            string   `json:"origin"`
	Plan              string   `json:"plan"`
	Section           string   `json:"section"`
	Number            int      `json:"number"`
	Provenance        Source   `json:"-"`
}

// Posed reports what a pose did: created, updated (context changed) or unchanged.
type Posed struct {
	Question Question `json:"question"`
	Existed  bool     `json:"existed"`
	Changed  bool     `json:"changed"`
}

func (q Question) forOwner() bool { return q.Direction == toOwner && q.Review != nil }

// actions are the numbered outcomes of q: its own, else the defaults, then
// "ask" when asking is offered.
func reviewActions(q Question, defaults []string, ask bool) []string {
	out := slices.Clone(defaults)
	if q.Review != nil && len(q.Review.Actions) > 0 {
		out = slices.Clone(q.Review.Actions)
	}
	if len(out) == 0 {
		out = slices.Clone(defaultActions)
	}
	if ask && !slices.Contains(out, askOut) {
		out = append(out, askOut)
	}
	return out
}

func (s *Store) reviewDefaults() []string {
	if len(s.ReviewActions) > 0 {
		return s.ReviewActions
	}
	return defaultActions
}

func (s *Store) Pose(p Pose) (out Posed, err error) {
	if strings.TrimSpace(p.Question) == "" {
		return out, errors.New("question must not be empty")
	}
	if p.Key == "" {
		p.Key = "pose:" + fmt.Sprintf("%x", sha256.Sum256([]byte(p.Group+"\x00"+p.Source+"\x00"+p.Question)))[:16]
	}
	for _, a := range p.Actions {
		if strings.TrimSpace(a) == "" {
			return out, errors.New("actions must be nonempty")
		}
	}
	if p.Recommendation != "" && p.RecommendedAction == "" {
		p.RecommendedAction = s.reviewDefaults()[0]
		if len(p.Actions) > 0 {
			p.RecommendedAction = p.Actions[0]
		}
	}
	err = s.locked(func() error {
		if e := s.permit("pose", Question{}); e != nil {
			return e
		}
		rows, e := s.all()
		if e != nil {
			return e
		}
		before := clone(rows)
		for i, q := range rows {
			if q.Key != p.Key || q.Deleted {
				continue
			}
			if !q.forOwner() {
				return fmt.Errorf("key %q belongs to a question for an agent", p.Key)
			}
			out.Existed = true
			r := q.Review
			changed := q.Text != p.Question || r.Details != p.Details || r.Recommendation != p.Recommendation || r.RecommendedAction != p.RecommendedAction
			moved := changed || q.Title != poseTitle(p) || r.Group != p.Group || r.Source != p.Source || !slices.Equal(r.Actions, p.Actions)
			if !moved {
				out.Question = q
				return nil
			}
			if changed && r.State != reviewApplied && (r.Action != "" || r.Note != "") {
				r.ContextChanged = true
			}
			out.Changed = changed
			q.Text, q.Title = p.Question, poseTitle(p)
			fillReview(r, p)
			q.Updated = s.now()
			rows[i] = q
			out.Question = q
			return s.commit(before, rows, "pose")
		}
		id, e := token()
		if e != nil {
			return e
		}
		if p.Cwd != "" {
			if p.Cwd, e = filepath.Abs(p.Cwd); e != nil {
				return e
			}
		}
		now := s.now()
		if len(rows) > 0 && !now.After(rows[len(rows)-1].Created) {
			now = rows[len(rows)-1].Created.Add(time.Nanosecond)
		}
		r := &Review{}
		fillReview(r, p)
		q := Question{ID: "q-" + id, Title: poseTitle(p), Text: p.Question, Verbatim: "agent", AskedBy: s.Actor, RecordedBy: s.Actor, AskedAt: now, Created: now, Updated: now, Status: "open", Direction: toOwner, Review: r, Project: Project{ID: p.Project}, Cwd: p.Cwd, Task: p.Task, Source: p.Provenance, Tags: p.Tags, Links: p.Links, Key: p.Key, Entries: []Entry{}}
		q.FileName = q.Created.Format("20060102T150405.000000000Z") + "-" + slug(q.Title) + "-" + q.ID + ".json"
		out.Question = q
		return s.commit(before, append(rows, q), "pose")
	})
	return
}

func poseTitle(p Pose) string {
	if p.Title != "" {
		return p.Title
	}
	r := []rune(strings.Split(strings.TrimSpace(p.Question), "\n")[0])
	return string(r[:min(len(r), 80)])
}

func fillReview(r *Review, p Pose) {
	r.Group, r.Source, r.Details, r.Recommendation = p.Group, p.Source, p.Details, p.Recommendation
	r.RecommendedAction = p.RecommendedAction
	r.Actions = slices.Clone(p.Actions)
	r.Origin, r.Plan, r.Section, r.Number = p.Origin, p.Plan, p.Section, p.Number
}

// Decision is the owner's change to one review item. Choice (1-based) picks
// from the item's numbered actions; Accept takes the recommendation; Clear
// empties the action. A nil Note leaves the note alone.
type Decision struct {
	Action string
	Choice int
	Accept bool
	Clear  bool
	Note   *string
}

func (s *Store) Decide(target string, d Decision) (Question, error) {
	return s.mutate("decide", target, func(q *Question, _ *[]Question) error {
		if !q.forOwner() {
			return errors.New("not a question for the owner; use answer")
		}
		r := q.Review
		if r.State == reviewApplied {
			return errors.New("decision already applied; it is locked")
		}
		action, set := r.Action, false
		switch {
		case d.Clear:
			action, set = "", true
		case d.Accept:
			if r.RecommendedAction == "" && r.Recommendation == "" {
				return errors.New("no recommendation to accept")
			}
			action, set = r.RecommendedAction, true
			if action == "" {
				action = s.reviewDefaults()[0]
			}
		case d.Choice > 0:
			choices := reviewActions(*q, s.reviewDefaults(), s.ReviewAsk)
			if d.Choice > len(choices) {
				return fmt.Errorf("choice %d out of range (1-%d: %s)", d.Choice, len(choices), strings.Join(choices, ", "))
			}
			action, set = choices[d.Choice-1], true
		case strings.TrimSpace(d.Action) != "":
			action, set = strings.TrimSpace(d.Action), true
		}
		if !set && d.Note == nil {
			return errors.New("name an action, a choice, accept, clear or a note")
		}
		if d.Note != nil {
			r.Note = *d.Note
		}
		if action == askOut && s.ReviewAsk {
			text := strings.TrimSpace(r.Note)
			if text == "" {
				return errors.New("ask needs the question in the note")
			}
			if last := lastOwnerQuestion(*q); last != text {
				if e := s.appendEntry(q, Entry{Kind: "follow-up", Text: text}); e != nil {
					return e
				}
			}
		}
		r.Action = action
		r.ContextChanged, r.NewReply = false, false
		if r.State == reviewFailed && set {
			r.State, r.Result = "", ""
		}
		if set {
			now := s.now()
			r.DecidedAt = &now
			if action == "" {
				r.DecidedAt = nil
			}
		}
		if !q.StatusExplicit {
			q.Status = reviewStatus(*q, s.ReviewAsk)
		}
		return nil
	})
}

// reviewStatus maps a review to the shared statuses: open waits for the
// owner, follow-up for the agent's reply, answered once decided.
func reviewStatus(q Question, ask bool) string {
	r := q.Review
	switch {
	case r.Action == "":
		return "open"
	case r.Action == askOut && ask:
		return "follow-up"
	case r.Action == "moot":
		return "moot"
	}
	return "answered"
}

func lastOwnerQuestion(q Question) string {
	for i := len(q.Entries) - 1; i >= 0; i-- {
		if q.Entries[i].Kind == "follow-up" {
			return strings.TrimSpace(q.Entries[i].Text)
		}
	}
	return ""
}

// Reply is the agent's answer to the owner's ask. An asked item comes back
// undecided with a "new reply" flag; its note is cleared when the thread
// already records it as the owner's question (mgmt build_session).
type Reply struct {
	Text              string  `json:"text"`
	Question          string  `json:"question"`
	Recommendation    *string `json:"recommendation"`
	RecommendedAction *string `json:"recommended_action"`
}

func (s *Store) Reply(target string, rp Reply) (Question, error) {
	return s.mutate("reply", target, func(q *Question, _ *[]Question) error {
		if !q.forOwner() {
			return errors.New("not a question for the owner; use answer")
		}
		r := q.Review
		if r.State == reviewApplied {
			return errors.New("decision already applied; it is locked")
		}
		if strings.TrimSpace(rp.Text) == "" {
			return errors.New("reply text must not be empty")
		}
		if text := strings.TrimSpace(rp.Question); text != "" && text != lastOwnerQuestion(*q) {
			owner := s.Actor
			s.Actor = Owner()
			e := s.appendEntry(q, Entry{Kind: "follow-up", Text: text})
			s.Actor = owner
			if e != nil {
				return e
			}
		}
		if e := s.appendEntry(q, Entry{Kind: "reply", Text: rp.Text}); e != nil {
			return e
		}
		if rp.Recommendation != nil {
			r.Recommendation = strings.TrimSpace(*rp.Recommendation)
		}
		if rp.RecommendedAction != nil {
			r.RecommendedAction = strings.TrimSpace(*rp.RecommendedAction)
		}
		if r.Action == askOut {
			r.Action, r.DecidedAt = "", nil
			if strings.TrimSpace(r.Note) == lastOwnerQuestion(*q) {
				r.Note = ""
			}
		}
		r.NewReply, r.ContextChanged = true, true
		if !q.StatusExplicit {
			q.Status = reviewStatus(*q, s.ReviewAsk)
		}
		return nil
	})
}

// Applied records the agent's result for a decided item. done locks it;
// failed leaves it open to a new decision (mgmt result).
func (s *Store) Applied(target, outcome, result string) (Question, error) {
	return s.mutate("apply", target, func(q *Question, _ *[]Question) error {
		if !q.forOwner() {
			return errors.New("not a question for the owner")
		}
		r := q.Review
		if r.State == reviewApplied {
			return errors.New("decision already applied; it is locked")
		}
		if r.Action == "" || (r.Action == askOut && s.ReviewAsk) {
			return errors.New("only a decided item can be applied")
		}
		if strings.TrimSpace(result) == "" {
			return errors.New("a nonempty result receipt is required")
		}
		switch outcome {
		case "done":
			r.State = reviewApplied
		case "failed":
			r.State = reviewFailed
		default:
			return errors.New("outcome must be done or failed")
		}
		now := s.now()
		r.Result, r.AppliedAt = result, &now
		return nil
	})
}

// accepted is mgmt's "just takes the recommendation, nothing to read".
func accepted(r *Review) bool {
	return r.RecommendedAction != "" && r.Action == r.RecommendedAction && strings.TrimSpace(r.Note) == "" && !r.ContextChanged
}

// reviewClass is the item's review state, one of reviewFilters: what the
// review.show setting and decisions filter by.
func reviewClass(q Question, ask bool) string {
	r := q.Review
	switch {
	case r.State == reviewApplied:
		return "applied"
	case r.Action == askOut && ask:
		return "asked"
	case r.Action == "":
		return "undecided"
	}
	return "decided"
}

var reviewFilters = []string{"pending", "undecided", "asked", "changed", "decided", "applied", "all"}

func reviewShown(q Question, ask bool, filter string) bool {
	class := reviewClass(q, ask)
	switch filter {
	case "", "all":
		return true
	case "pending":
		return class != "applied"
	case "changed":
		return q.Review.ContextChanged || q.Review.NewReply
	}
	return class == filter
}

func (s *Store) ownerQuestions() ([]Question, error) {
	rows, e := s.List()
	if e != nil {
		return nil, e
	}
	return slices.DeleteFunc(rows, func(q Question) bool { return !q.forOwner() }), nil
}

// groupOf is mgmt's group_of: the item's topic, else its source file.
func groupOf(q Question) string {
	r := q.Review
	if r.Group != "" {
		return r.Group
	}
	if head, tail, ok := cutLast(r.Source, ":"); ok && head != "" && strings.Trim(tail, "0123456789-") == "" && tail != "" {
		return head
	}
	if r.Source != "" {
		return r.Source
	}
	return "Ungrouped"
}

func cutLast(s, sep string) (string, string, bool) {
	i := strings.LastIndex(s, sep)
	if i < 0 {
		return s, "", false
	}
	return s[:i], s[i+len(sep):], true
}

// groupOrder keeps topics in order of first appearance (oldest question first).
func groupOrder(qs []Question) []string {
	var out []string
	for _, q := range qs {
		if g := groupOf(q); !slices.Contains(out, g) {
			out = append(out, g)
		}
	}
	return out
}

// sortByGroup orders questions by topic, then creation, so a tree list draws
// each topic once.
func sortByGroup(qs []Question) []Question {
	order := groupOrder(qs)
	out := slices.Clone(qs)
	slices.SortStableFunc(out, func(a, b Question) int {
		return slices.Index(order, groupOf(a)) - slices.Index(order, groupOf(b))
	})
	return out
}

// Export is the decisions contract an agent reads (`question decisions`):
// the shape of mgmt-export/1, with o-questions ids.
type Export struct {
	Format     string           `json:"format"`
	ExportedAt time.Time        `json:"exported_at"`
	Counts     map[string]int   `json:"counts"`
	Groups     []string         `json:"groups"`
	Questions  []ExportQuestion `json:"questions"`
	Items      []ExportItem     `json:"items"`
	Undecided  []ExportItem     `json:"undecided"`
}
type ExportQuestion struct {
	ID       string          `json:"id"`
	Key      string          `json:"key"`
	Group    string          `json:"group"`
	Source   string          `json:"source,omitempty"`
	Title    string          `json:"title"`
	Question string          `json:"question"`
	Thread   []ThreadMessage `json:"thread"`
}
type ExportItem struct {
	ID                     string `json:"id"`
	Key                    string `json:"key"`
	Group                  string `json:"group"`
	Source                 string `json:"source,omitempty"`
	Title                  string `json:"title"`
	Action                 string `json:"action,omitempty"`
	RecommendedAction      string `json:"recommended_action,omitempty"`
	Recommendation         string `json:"recommendation,omitempty"`
	AcceptedRecommendation bool   `json:"accepted_recommendation"`
	Note                   string `json:"note,omitempty"`
	State                  string `json:"state,omitempty"`
	Result                 string `json:"result,omitempty"`
	ContextChanged         bool   `json:"context_changed"`
	NewReply               bool   `json:"new_reply"`
}

// ThreadMessage is mgmt's thread row: the owner's asks and the agent's replies.
type ThreadMessage struct {
	Role string `json:"role"`
	Text string `json:"text"`
	At   string `json:"at,omitempty"`
}

func reviewThread(q Question) []ThreadMessage {
	out := []ThreadMessage{}
	for _, e := range q.Entries {
		role := ""
		switch e.Kind {
		case "follow-up":
			role = "owner"
		case "reply":
			role = "agent"
		}
		if role != "" && strings.TrimSpace(e.Text) != "" {
			out = append(out, ThreadMessage{Role: role, Text: strings.TrimSpace(e.Text), At: e.At.Format(time.RFC3339)})
		}
	}
	return out
}

func (s *Store) Decisions(group string, includeApplied bool) (Export, error) {
	qs, e := s.ownerQuestions()
	if e != nil {
		return Export{}, e
	}
	qs = sortByGroup(qs)
	x := Export{Format: "o-questions-decisions/1", ExportedAt: s.now(), Groups: []string{}, Questions: []ExportQuestion{}, Items: []ExportItem{}, Undecided: []ExportItem{}}
	accepts := 0
	for _, q := range qs {
		g := groupOf(q)
		if group != "" && g != group {
			continue
		}
		class := reviewClass(q, s.ReviewAsk)
		if class == "applied" && !includeApplied {
			continue
		}
		if !slices.Contains(x.Groups, g) {
			x.Groups = append(x.Groups, g)
		}
		r := q.Review
		item := ExportItem{ID: q.ID, Key: q.Key, Group: g, Source: r.Source, Title: q.Title, Action: r.Action, RecommendedAction: r.RecommendedAction, Recommendation: r.Recommendation, AcceptedRecommendation: accepted(r), Note: r.Note, State: r.State, Result: r.Result, ContextChanged: r.ContextChanged, NewReply: r.NewReply}
		switch class {
		case "asked":
			x.Questions = append(x.Questions, ExportQuestion{ID: q.ID, Key: q.Key, Group: g, Source: r.Source, Title: q.Title, Question: lastOwnerQuestion(q), Thread: reviewThread(q)})
		case "undecided":
			x.Undecided = append(x.Undecided, item)
		default:
			if item.AcceptedRecommendation {
				accepts++
			}
			x.Items = append(x.Items, item)
		}
	}
	x.Counts = map[string]int{"decided": len(x.Items), "accepted": accepts, "asked": len(x.Questions), "undecided": len(x.Undecided)}
	return x, nil
}
