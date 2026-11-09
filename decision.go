// Agent-to-owner questions and their decision. The semantics (recommendation
// shown unconfirmed until accepted, options the owner picks from, outcomes
// (later, moot, ask, write-your-own) held in a data table, decisions carried
// across regenerations by a stable key, an export the agent reads, items
// locked once applied) are adapted from github.com/unixsuperhero/ai-utils/mgmt/mgmt.py
// (build_session, reply, export, apply/result). One store: a question for the
// owner is an ordinary Question whose Direction is "to-owner" and whose
// Decision holds the decision.
package main

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"
)

// Directions: who must answer. Empty is the original kind, the owner's
// question to an agent.
const (
	toAgent = "to-agent"
	toOwner = "to-owner"
)

var directions = []string{"all", toAgent, toOwner}

// Decision states (Decision.Status): empty is pending; applied is locked.
const (
	decisionApplied = "applied"
	decisionFailed  = "failed"
)

// Decision classes: the state decision.show and the decisions filter by. An
// outcome row names the class its pick has; the others follow from the data.
const (
	classUndecided = "undecided"
	classDecided   = "decided"
	classApplied   = "applied"
	classAsked     = "asked"
	classLater     = "later"
)

// Option is one choice the poser offers. ID is stable and unique in the
// question (the pick is stored by id); Label is the owner's words.
type Option struct {
	ID          string `json:"id"`
	Label       string `json:"label,omitempty"`
	Description string `json:"description,omitempty"`
}

// UnmarshalJSON reads an option as a bare string (id = label = the string) or
// as an object.
func (o *Option) UnmarshalJSON(b []byte) error {
	var text string
	if json.Unmarshal(b, &text) == nil {
		*o = Option{ID: strings.TrimSpace(text), Label: strings.TrimSpace(text)}
		return nil
	}
	type plain Option
	var v plain
	if e := json.Unmarshal(b, &v); e != nil {
		return e
	}
	*o = Option(v)
	return nil
}

func (o Option) label() string {
	if o.Label != "" {
		return o.Label
	}
	return o.ID
}

// Item is one line of a checklist: the owner checks it (do it) or leaves it
// unchecked (do not). Recommended items are what accept checks.
type Item struct {
	ID          string `json:"id"`
	Label       string `json:"label,omitempty"`
	Description string `json:"description,omitempty"`
	Recommended bool   `json:"recommended,omitempty"`
}

// UnmarshalJSON reads an item as a bare string (id = label = the string) or
// as an object.
func (it *Item) UnmarshalJSON(b []byte) error {
	var text string
	if json.Unmarshal(b, &text) == nil {
		*it = Item{ID: strings.TrimSpace(text), Label: strings.TrimSpace(text)}
		return nil
	}
	type plain Item
	var v plain
	if e := json.Unmarshal(b, &v); e != nil {
		return e
	}
	*it = Item(v)
	return nil
}

func (it Item) label() string {
	if it.Label != "" {
		return it.Label
	}
	return it.ID
}

// Pick is the owner's state for one checklist item; an item with no pick is
// unchecked. Status and Result record applying the item alone.
type Pick struct {
	Item      string     `json:"item"`
	Checked   bool       `json:"checked"`
	Note      string     `json:"note,omitempty"`
	Status    string     `json:"status,omitempty"`
	Result    string     `json:"result,omitempty"`
	AppliedAt *time.Time `json:"applied_at,omitempty"`
}

// Outcome is one row of the outcome table (setting decision.outcomes): an
// answer that is not an option. Status is the question status a pick of it
// gives (empty: answered); Class is the decision class (empty: decided);
// NeedsNote asks for text in the note; AwaitsReply turns the note into a
// follow-up the agent answers, and a reply reopens the decision.
type Outcome struct {
	ID          string
	Label       string
	Status      string
	Class       string
	NeedsNote   bool
	AwaitsReply bool
	// Key is the page key that picks the outcome (empty: none); a binding row
	// is made from it, so bindings.json can still rebind or drop it.
	Key string
}

// Outcome row flags: note asks for text in the note; reply also sends it to
// the agent and waits for the reply.
const (
	noteFlag  = "note"
	replyFlag = "reply"
)

var outcomeFlags = []string{noteFlag, replyFlag}

// parseOutcomes reads rows "id|label|status|class|flags|key" (flags: note,
// reply; key optional: the page key that picks the outcome).
func parseOutcomes(rows []string) ([]Outcome, error) {
	var out []Outcome
	for _, row := range rows {
		f := strings.Split(row, "|")
		for len(f) < 6 {
			f = append(f, "")
		}
		o := Outcome{ID: strings.TrimSpace(f[0]), Label: strings.TrimSpace(f[1]), Status: strings.TrimSpace(f[2]), Class: strings.TrimSpace(f[3]), Key: strings.TrimSpace(f[5])}
		if o.ID == "" || len(f) > 6 || slices.ContainsFunc(out, func(x Outcome) bool { return x.ID == o.ID }) {
			return nil, fmt.Errorf("invalid outcome row %q: want id|label|status|class|flags|key with a unique id", row)
		}
		if e := checkOutcomeKey(o, out); e != nil {
			return nil, e
		}
		if o.Status != "" && !slices.Contains(statuses, o.Status) {
			return nil, fmt.Errorf("outcome %q: status must be one of %q", o.ID, statuses)
		}
		if o.Class != "" && !slices.Contains(outcomeClasses(), o.Class) {
			return nil, fmt.Errorf("outcome %q: class must be one of %q", o.ID, outcomeClasses())
		}
		flags := strings.FieldsFunc(f[4], func(r rune) bool { return r == ',' || r == ' ' })
		for _, flag := range flags {
			if !slices.Contains(outcomeFlags, flag) {
				return nil, fmt.Errorf("outcome %q: flag %q is not one of %q", o.ID, flag, outcomeFlags)
			}
		}
		o.AwaitsReply = slices.Contains(flags, replyFlag)
		o.NeedsNote = o.AwaitsReply || slices.Contains(flags, noteFlag)
		if o.Label == "" {
			o.Label = o.ID
		}
		out = append(out, o)
	}
	return out, nil
}

// checkOutcomeKey refuses a key the page cannot give an outcome: a number
// (those pick options), a function key (the family never binds them), or one
// another outcome already has.
func checkOutcomeKey(o Outcome, before []Outcome) error {
	if o.Key == "" {
		return nil
	}
	if len([]rune(o.Key)) != 1 && !strings.ContainsAny(o.Key, "+ ") {
		return fmt.Errorf("outcome %q: key %q must be one key", o.ID, o.Key)
	}
	if n, e := strconv.Atoi(o.Key); e == nil && n >= 0 && n <= 9 {
		return fmt.Errorf("outcome %q: key %q picks an option", o.ID, o.Key)
	}
	if fkey.MatchString(o.Key) {
		return fmt.Errorf("outcome %q: function keys are never bound", o.ID)
	}
	if slices.ContainsFunc(before, func(x Outcome) bool { return x.Key == o.Key }) {
		return fmt.Errorf("outcome %q: key %q is used by another outcome", o.ID, o.Key)
	}
	return nil
}

var fkey = regexp.MustCompile(`(?i)^(?:[a-z]+\+)*f(?:[1-9]|1[0-2])$`)

// outcomeClasses are the classes an outcome row may name: every filter class
// but the ones the data decides.
func outcomeClasses() []string {
	return slices.DeleteFunc(slices.Clone(decisionFilters), func(c string) bool {
		return slices.Contains([]string{"pending", classUndecided, classApplied, "changed", "all"}, c)
	})
}

var defaultOutcomes = func() []Outcome {
	o, e := parseOutcomes(defaultOutcomeRows)
	if e != nil {
		panic(e)
	}
	return o
}()

func (s *Store) outcomes() []Outcome {
	if len(s.Outcomes) > 0 {
		return s.Outcomes
	}
	return defaultOutcomes
}

func (s *Store) outcome(id string) (Outcome, bool) {
	for _, o := range s.outcomes() {
		if o.ID == id {
			return o, id != ""
		}
	}
	return Outcome{}, false
}

// replyOutcome is the first outcome that sends the owner's note to the agent.
func (s *Store) replyOutcome() (Outcome, bool) {
	for _, o := range s.outcomes() {
		if o.AwaitsReply {
			return o, true
		}
	}
	return Outcome{}, false
}

func (s *Store) outcomeIDs() []string {
	var ids []string
	for _, o := range s.outcomes() {
		ids = append(ids, o.ID)
	}
	return ids
}

type Decision struct {
	// Format is decisionFormat once the decision carries explicit options.
	Format int    `json:"format,omitempty"`
	Group  string `json:"group,omitempty"`
	Source string `json:"source,omitempty"`
	// Details is the context; Recommendation is the reason for the recommended
	// option (or item), never a pick itself.
	Details string `json:"details,omitempty"`
	Origin  string `json:"origin,omitempty"`
	Plan    string `json:"plan,omitempty"`
	Section string `json:"section,omitempty"`
	Number  int    `json:"number,omitempty"`

	Recommendation string   `json:"recommendation,omitempty"`
	Options        []Option `json:"options,omitempty"`
	Recommended    string   `json:"recommended,omitempty"`
	Items          []Item   `json:"items,omitempty"`
	Picks          []Pick   `json:"picks,omitempty"`
	// Action is the pick: an option id or an outcome id.
	Action string `json:"action,omitempty"`
	Note   string `json:"note,omitempty"`

	// Actions and RecommendedAction are the stored form before options: read
	// as options (id = label = the word) until the migration rewrites them.
	Actions           []string `json:"actions,omitempty"`
	RecommendedAction string   `json:"recommended_action,omitempty"`

	DecidedAt      *time.Time `json:"decided_at,omitempty"`
	ContextChanged bool       `json:"context_changed,omitempty"`
	NewReply       bool       `json:"new_reply,omitempty"`
	Status         string     `json:"status,omitempty"`
	Result         string     `json:"result,omitempty"`
	AppliedAt      *time.Time `json:"applied_at,omitempty"`
}

// rawOptions are the question's options, else its stored actions read as options.
func (r *Decision) rawOptions() []Option {
	if len(r.Options) > 0 {
		return r.Options
	}
	var out []Option
	for _, a := range r.Actions {
		out = append(out, Option{ID: a, Label: a})
	}
	return out
}

// optionsOf are the options the owner can pick: a stored action that is an
// outcome id is the outcome, not an option.
func (s *Store) optionsOf(r *Decision) []Option {
	return slices.DeleteFunc(slices.Clone(r.rawOptions()), func(o Option) bool { _, is := s.outcome(o.ID); return is })
}

// recommendedID is the recommended option's id: the stored one, else the old
// recommended action when it names an option.
func (r *Decision) recommendedID() string {
	if r.Recommended != "" {
		return r.Recommended
	}
	if i := slices.IndexFunc(r.rawOptions(), func(o Option) bool { return o.ID == r.RecommendedAction }); i >= 0 {
		return r.RecommendedAction
	}
	return ""
}

func (r *Decision) recommendsItems() bool {
	return slices.ContainsFunc(r.Items, func(it Item) bool { return it.Recommended })
}

// locked: the decision (or its checklist) was applied and no longer changes.
func (r *Decision) locked() bool { return isApplied(r.Status) }

func isApplied(status string) bool { return status == decisionApplied }

// decided: the owner picked an answer or confirmed the checklist.
func (r *Decision) decided() bool { return r.Action != "" || r.DecidedAt != nil }

func (r *Decision) pickAt(item string) int {
	return slices.IndexFunc(r.Picks, func(p Pick) bool { return p.Item == item })
}

func (r *Decision) checked(item string) bool {
	i := r.pickAt(item)
	return i >= 0 && r.Picks[i].Checked
}

// progress is "checked/total" of a checklist, empty for a question without items.
func (r *Decision) progress() string {
	if len(r.Items) == 0 {
		return ""
	}
	n := 0
	for _, it := range r.Items {
		if r.checked(it.ID) {
			n++
		}
	}
	return fmt.Sprintf("%d/%d", n, len(r.Items))
}

// answerLabel is the pick as the owner sees it: the option's or outcome's
// label, else the stored word.
func (s *Store) answerLabel(r *Decision) string {
	if i := slices.IndexFunc(r.rawOptions(), func(o Option) bool { return o.ID == r.Action }); i >= 0 {
		return r.rawOptions()[i].label()
	}
	if o, ok := s.outcome(r.Action); ok {
		return o.Label
	}
	return r.Action
}

// Pose is an agent's question for the owner. Key is the stable id that
// carries a decision across regenerations; reposing the same key updates the
// question and flags a decided item "context changed" when its text moved.
// Options are what the owner picks from, Recommended names one by id and
// Recommendation is the reason. Items make a checklist. Actions and
// RecommendedAction are the renamed parameters, kept only to refuse them
// with the new name.
type Pose struct {
	Question          string   `json:"question"`
	Title             string   `json:"title"`
	Group             string   `json:"group"`
	Source            string   `json:"source"`
	Details           string   `json:"details"`
	Recommendation    string   `json:"recommendation"`
	Recommended       string   `json:"recommended"`
	Options           []Option `json:"options"`
	Items             []Item   `json:"items"`
	Actions           []string `json:"actions"`
	RecommendedAction string   `json:"recommended_action"`
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

// Posed reports what a pose did: created, updated (context changed) or
// unchanged. Warnings are about a stored question the poser should improve.
type Posed struct {
	Question Question `json:"question"`
	Existed  bool     `json:"existed"`
	Changed  bool     `json:"changed"`
	Warnings []string `json:"warnings,omitempty"`
}

func (q Question) forOwner() bool { return q.Direction == toOwner && q.Decision != nil }

// checkPose refuses what cannot be stored unambiguously: the old parameter
// names and empty, duplicate or outcome-colliding ids. Missing options are a
// warning, not a refusal (see poseWarnings).
func (s *Store) checkPose(p *Pose) error {
	if len(p.Actions) > 0 {
		return errors.New("actions was renamed options: give each option an id and a label")
	}
	if p.RecommendedAction != "" {
		return errors.New("recommended_action was renamed recommended: give an option id")
	}
	seen := map[string]bool{}
	for i, o := range p.Options {
		o.ID = strings.TrimSpace(o.ID)
		if o.ID == "" {
			return fmt.Errorf("option %d needs an id", i+1)
		}
		if _, is := s.outcome(o.ID); is {
			return fmt.Errorf("option id %q is an outcome (%s); rename the option", o.ID, strings.Join(s.outcomeIDs(), ", "))
		}
		if seen[o.ID] {
			return fmt.Errorf("duplicate option id %q", o.ID)
		}
		seen[o.ID] = true
		p.Options[i] = o
	}
	seen = map[string]bool{}
	for i, it := range p.Items {
		it.ID = strings.TrimSpace(it.ID)
		if it.ID == "" {
			return fmt.Errorf("item %d needs an id", i+1)
		}
		if seen[it.ID] {
			return fmt.Errorf("duplicate item id %q", it.ID)
		}
		seen[it.ID] = true
		p.Items[i] = it
	}
	return nil
}

// poseWarnings are the improvements a stored question still needs. opts and
// hasItems describe what the question will hold once stored. A recommended id
// that names no option is dropped.
func (s *Store) poseWarnings(p *Pose, opts []Option, hasItems bool) []string {
	var w []string
	if p.Recommended != "" && !slices.ContainsFunc(opts, func(o Option) bool { return o.ID == p.Recommended }) {
		w = append(w, fmt.Sprintf("recommended %q names no option; dropped", p.Recommended))
		p.Recommended = ""
	}
	if len(opts) == 0 && !hasItems {
		w = append(w, "no options: supply options (a short label in the owner's own words and an id each) and name your pick in recommended; until then the owner can only answer with the outcomes ("+strings.Join(s.outcomeIDs(), ", ")+")")
	}
	if len(opts) > 0 && p.Recommended == "" && p.Recommendation != "" {
		w = append(w, "recommendation has no recommended option; set recommended to an option id")
	}
	return w
}

func (s *Store) Pose(p Pose) (out Posed, err error) {
	if strings.TrimSpace(p.Question) == "" {
		return out, errors.New("question must not be empty")
	}
	if p.Key == "" {
		p.Key = "pose:" + fmt.Sprintf("%x", sha256.Sum256([]byte(p.Group+"\x00"+p.Source+"\x00"+p.Question)))[:16]
	}
	if err = s.checkPose(&p); err != nil {
		return out, err
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
			return s.repose(&out, p, rows, i, before)
		}
		out.Warnings = s.poseWarnings(&p, p.Options, len(p.Items) > 0)
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
		r := &Decision{}
		fillDecision(r, p)
		q := Question{ID: "q-" + id, Title: poseTitle(p), Text: p.Question, Verbatim: "agent", AskedBy: s.Actor, RecordedBy: s.Actor, AskedAt: now, Created: now, Updated: now, Status: "open", Direction: toOwner, Decision: r, Project: Project{ID: p.Project}, Cwd: p.Cwd, Task: p.Task, Source: p.Provenance, Tags: p.Tags, Links: p.Links, Key: p.Key, Entries: []Entry{}}
		q.FileName = q.Created.Format("20060102T150405.000000000Z") + "-" + slug(q.Title) + "-" + q.ID + ".json"
		out.Question = q
		return s.commit(before, append(rows, q), "pose")
	})
	return
}

// repose updates the question at rows[i] from p. The key-stability rule: a
// repose that names options or items replaces them; the owner's pick and
// item states stay while the option or item id exists in the new set (or is
// an outcome), otherwise the pick is cleared, the question flagged context
// changed, and the lost choice recorded in the thread. A repose that names
// none leaves them as they are.
func (s *Store) repose(out *Posed, p Pose, rows []Question, i int, before []Question) error {
	q := rows[i]
	r := q.Decision
	opts, hasItems := p.Options, len(p.Items) > 0
	if len(opts) == 0 {
		opts = s.optionsOf(r)
	}
	hasItems = hasItems || len(r.Items) > 0
	out.Warnings = s.poseWarnings(&p, opts, hasItems)
	next := *r
	fillDecision(&next, p)
	changed := q.Text != p.Question || r.Details != p.Details || r.Recommendation != p.Recommendation || r.Recommended != next.Recommended
	moved := changed || q.Title != poseTitle(p) || r.Group != p.Group || r.Source != p.Source || !slices.Equal(r.Options, next.Options) || !slices.Equal(r.Items, next.Items)
	if !moved {
		out.Question = q
		return nil
	}
	lost := s.lostChoices(&next, len(p.Options) > 0, len(p.Items) > 0)
	if changed && !r.locked() && (r.Action != "" || r.Note != "" || len(r.Picks) > 0) {
		next.ContextChanged = true
	}
	*r = next
	q.Text, q.Title = p.Question, poseTitle(p)
	for _, text := range lost {
		if e := s.appendEntry(&q, Entry{Kind: "note", Text: text}); e != nil {
			return e
		}
	}
	if len(lost) > 0 && !q.StatusExplicit {
		q.Status = s.decisionStatus(q)
	}
	out.Changed = changed || len(lost) > 0
	q.Updated = s.now()
	rows[i] = q
	out.Question = q
	return s.commit(before, rows, "pose")
}

// lostChoices clears the picks of d whose option or item no longer exists
// (only for the sets the repose replaced) and returns one thread line each.
func (s *Store) lostChoices(d *Decision, optionsNew, itemsNew bool) []string {
	var lost []string
	if d.locked() {
		return nil
	}
	if optionsNew && d.Action != "" {
		_, outcome := s.outcome(d.Action)
		if !outcome && !slices.ContainsFunc(d.Options, func(o Option) bool { return o.ID == d.Action }) {
			lost = append(lost, fmt.Sprintf("Your earlier choice %q no longer exists; it was cleared.", d.Action))
			d.Action, d.DecidedAt = "", nil
		}
	}
	if itemsNew {
		var keep []Pick
		for _, pk := range d.Picks {
			if slices.ContainsFunc(d.Items, func(it Item) bool { return it.ID == pk.Item }) || isApplied(pk.Status) {
				keep = append(keep, pk)
				continue
			}
			lost = append(lost, fmt.Sprintf("Your earlier mark on item %q no longer exists; it was cleared.", pk.Item))
		}
		d.Picks = keep
	}
	if len(lost) > 0 {
		d.ContextChanged = true
	}
	return lost
}

func poseTitle(p Pose) string {
	if p.Title != "" {
		return p.Title
	}
	r := []rune(strings.Split(strings.TrimSpace(p.Question), "\n")[0])
	return string(r[:min(len(r), 80)])
}

// fillDecision copies the pose into r. Options and items are replaced only
// when the pose names them; a pose naming options also replaces the old
// stored actions and recommended action.
func fillDecision(r *Decision, p Pose) {
	r.Group, r.Source, r.Details, r.Recommendation = p.Group, p.Source, p.Details, p.Recommendation
	r.Format = decisionFormat
	if len(p.Options) > 0 {
		r.Options, r.Recommended = slices.Clone(p.Options), p.Recommended
		r.Actions, r.RecommendedAction = nil, ""
		for i := range r.Options {
			r.Options[i].Label = r.Options[i].label()
		}
	} else if p.Recommended != "" {
		r.Recommended = p.Recommended
	}
	if len(p.Items) > 0 {
		r.Items = slices.Clone(p.Items)
		for i := range r.Items {
			r.Items[i].Label = r.Items[i].label()
		}
	}
	r.Origin, r.Plan, r.Section, r.Number = p.Origin, p.Plan, p.Section, p.Number
}

// DecisionChange is the owner's change to one decision item. Option names an
// option id; Action an option or outcome id; Choice (1-based) the Nth option;
// Accept takes the recommendation (the recommended option and the
// recommended items); Clear empties the decision, or one item's mark when
// Item is set; Confirm decides a checklist as it stands (unchecked means do
// not). With Item, Checked sets that item's mark (nil flips it) and PickNote
// its note. A nil Note leaves the note alone.
type DecisionChange struct {
	Action   string
	Option   string
	Choice   int
	Accept   bool
	Clear    bool
	Confirm  bool
	Item     string
	Checked  *bool
	PickNote *string
	Note     *string
}

// choice is what a DecisionChange resolves to on a decision.
type choice struct {
	action  string
	setAct  bool
	items   bool // check the recommended items
	confirm bool
	clear   bool
}

func (s *Store) validAction(r *Decision, id string, outcomes bool) error {
	if slices.ContainsFunc(s.optionsOf(r), func(o Option) bool { return o.ID == id }) {
		return nil
	}
	if _, is := s.outcome(id); is && outcomes {
		return nil
	}
	var have []string
	for _, o := range s.optionsOf(r) {
		have = append(have, o.ID)
	}
	if outcomes {
		have = append(have, s.outcomeIDs()...)
	}
	return fmt.Errorf("unknown choice %q (%s)", id, strings.Join(have, ", "))
}

// choose resolves d to a choice, checking ids against the decision.
func (s *Store) choose(r *Decision, d DecisionChange) (choice, error) {
	var c choice
	switch {
	case d.Clear:
		c.clear = true
	case d.Accept:
		if id := r.recommendedID(); id != "" {
			c.action, c.setAct = id, true
		}
		if r.recommendsItems() {
			c.items = true
		}
		if !c.setAct && !c.items {
			return c, errors.New("no recommended option to accept")
		}
	case d.Choice > 0:
		opts := s.optionsOf(r)
		if d.Choice > len(opts) {
			if len(opts) == 0 {
				return c, errors.New("no options to choose from; use an outcome")
			}
			return c, fmt.Errorf("choice %d out of range (1-%d)", d.Choice, len(opts))
		}
		c.action, c.setAct = opts[d.Choice-1].ID, true
	case strings.TrimSpace(d.Option) != "":
		c.action, c.setAct = strings.TrimSpace(d.Option), true
		if e := s.validAction(r, c.action, false); e != nil {
			return c, e
		}
	case strings.TrimSpace(d.Action) != "":
		c.action, c.setAct = strings.TrimSpace(d.Action), true
		if e := s.validAction(r, c.action, true); e != nil {
			return c, e
		}
	case d.Confirm:
		if len(r.Items) == 0 {
			return c, errors.New("nothing to confirm: the question has no items")
		}
	}
	c.confirm = d.Confirm && len(r.Items) > 0
	return c, nil
}

func (s *Store) Decide(target string, d DecisionChange) (Question, error) {
	return s.mutate("decide", target, func(q *Question, _ *[]Question) error {
		if !q.forOwner() {
			return errors.New("not a question for the owner; use answer")
		}
		r := q.Decision
		if r.locked() {
			return errors.New("decision already applied; it is locked")
		}
		if d.Item != "" {
			if e := decidePick(r, d); e != nil {
				return e
			}
			r.ContextChanged, r.NewReply = false, false
			return nil
		}
		c, e := s.choose(r, d)
		if e != nil {
			return e
		}
		if !c.setAct && !c.clear && !c.items && !c.confirm && d.Note == nil {
			return errors.New("name an option, an item, a choice, accept, confirm, clear or a note")
		}
		out, isOutcome := s.outcome(c.action)
		if !c.setAct {
			out, isOutcome = Outcome{}, false
		}
		if d.Note != nil {
			if e := s.recordNote(q, *d.Note, isOutcome && out.AwaitsReply); e != nil {
				return e
			}
		}
		if isOutcome && out.NeedsNote && strings.TrimSpace(r.Note) == "" {
			return fmt.Errorf("%s needs text in the note", out.Label)
		}
		if isOutcome && out.AwaitsReply {
			if last := lastOwnerQuestion(*q); last != strings.TrimSpace(r.Note) {
				if e := s.appendEntry(q, Entry{Kind: "follow-up", Text: strings.TrimSpace(r.Note)}); e != nil {
					return e
				}
			}
		}
		s.applyChoice(r, c)
		if !q.StatusExplicit {
			q.Status = s.decisionStatus(*q)
		}
		return nil
	})
}

// applyChoice writes a resolved choice into r.
func (s *Store) applyChoice(r *Decision, c choice) {
	changed := c.setAct || c.clear || c.items || c.confirm
	if c.setAct {
		r.Action = c.action
	}
	if c.items {
		for _, it := range r.Items {
			if i := r.pickAt(it.ID); i >= 0 && isApplied(r.Picks[i].Status) {
				continue
			}
			setPick(r, it.ID, func(p *Pick) { p.Checked = it.Recommended })
		}
	}
	r.ContextChanged, r.NewReply = false, false
	if r.Status == decisionFailed && changed {
		r.Status, r.Result = "", ""
	}
	switch {
	case c.clear:
		r.Action, r.DecidedAt = "", nil
		r.Picks = slices.DeleteFunc(r.Picks, func(p Pick) bool { return !isApplied(p.Status) })
	case changed:
		now := s.now()
		r.DecidedAt = &now
	}
}

func setPick(r *Decision, item string, f func(*Pick)) {
	i := r.pickAt(item)
	if i < 0 {
		r.Picks = append(r.Picks, Pick{Item: item})
		i = len(r.Picks) - 1
	}
	f(&r.Picks[i])
}

// decidePick changes one checklist item's mark or note; it does not decide
// the checklist (that is Confirm or Accept).
func decidePick(r *Decision, d DecisionChange) error {
	if !slices.ContainsFunc(r.Items, func(it Item) bool { return it.ID == d.Item }) {
		var ids []string
		for _, it := range r.Items {
			ids = append(ids, it.ID)
		}
		return fmt.Errorf("unknown item %q (items: %s)", d.Item, strings.Join(ids, ", "))
	}
	if i := r.pickAt(d.Item); i >= 0 && isApplied(r.Picks[i].Status) {
		return fmt.Errorf("item %q already applied; it is locked", d.Item)
	}
	if d.Clear {
		r.Picks = slices.DeleteFunc(r.Picks, func(p Pick) bool { return p.Item == d.Item })
		return nil
	}
	if d.Checked == nil && d.PickNote == nil {
		d.Checked = new(bool)
		*d.Checked = !r.checked(d.Item)
	}
	setPick(r, d.Item, func(p *Pick) {
		if d.Checked != nil {
			p.Checked = *d.Checked
		}
		if d.PickNote != nil {
			p.Note = *d.PickNote
		}
		p.Status, p.Result, p.AppliedAt = "", "", nil
	})
	return nil
}

// recordNote sets the decision's current note. Every distinct note also joins
// the thread, so a later note never replaces an earlier one; an ask records
// its text as the follow-up instead, not twice.
func (s *Store) recordNote(q *Question, note string, asking bool) error {
	r := q.Decision
	text := strings.TrimSpace(note)
	if text != "" && text != strings.TrimSpace(r.Note) && !asking {
		if e := s.appendEntry(q, Entry{Kind: "note", Text: text}); e != nil {
			return e
		}
	}
	r.Note = note
	return nil
}

// decisionStatus maps a decision to the shared statuses: open waits for the
// owner; once decided, the pick's outcome row names the status (follow-up
// waits for the agent's reply), else answered.
func (s *Store) decisionStatus(q Question) string {
	r := q.Decision
	if !r.decided() {
		return "open"
	}
	if o, ok := s.outcome(r.Action); ok && o.Status != "" {
		return o.Status
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
	Text           string  `json:"text"`
	Question       string  `json:"question"`
	Recommendation *string `json:"recommendation"`
	Recommended    *string `json:"recommended"`
}

func (s *Store) Reply(target string, rp Reply) (Question, error) {
	return s.mutate("reply", target, func(q *Question, _ *[]Question) error {
		if !q.forOwner() {
			return errors.New("not a question for the owner; use answer")
		}
		r := q.Decision
		if r.locked() {
			return errors.New("decision already applied; it is locked")
		}
		if strings.TrimSpace(rp.Text) == "" {
			return errors.New("reply text must not be empty")
		}
		rec := ""
		if rp.Recommended != nil {
			rec = strings.TrimSpace(*rp.Recommended)
			if rec != "" && !slices.ContainsFunc(s.optionsOf(r), func(o Option) bool { return o.ID == rec }) {
				return fmt.Errorf("recommended %q names no option", rec)
			}
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
		if rp.Recommended != nil {
			r.Recommended, r.RecommendedAction = rec, ""
		}
		if s.awaitsReply(r) {
			r.Action, r.DecidedAt = "", nil
			if strings.TrimSpace(r.Note) == lastOwnerQuestion(*q) {
				r.Note = ""
			}
		}
		r.NewReply, r.ContextChanged = true, true
		if !q.StatusExplicit {
			q.Status = s.decisionStatus(*q)
		}
		return nil
	})
}

// Applied records the agent's result for a decided item. done locks it;
// failed leaves it open to a new decision (mgmt result). With item, the
// result is for that checked checklist item alone: done locks the item, and
// the decision locks when every checked item is applied; failed reopens only
// that item.
func (s *Store) Applied(target, item, outcome, result string) (Question, error) {
	return s.mutate("apply", target, func(q *Question, _ *[]Question) error {
		if !q.forOwner() {
			return errors.New("not a question for the owner")
		}
		r := q.Decision
		if r.locked() {
			return errors.New("decision already applied; it is locked")
		}
		if !r.decided() || s.awaitsReply(r) {
			return errors.New("only a decided item can be applied")
		}
		if strings.TrimSpace(result) == "" {
			return errors.New("a nonempty result receipt is required")
		}
		if outcome != "done" && outcome != "failed" {
			return errors.New("outcome must be done or failed")
		}
		now := s.now()
		if item == "" {
			r.Status, r.Result, r.AppliedAt = decisionApplied, result, &now
			if outcome == "failed" {
				r.Status = decisionFailed
			}
			return nil
		}
		if !r.checked(item) {
			return fmt.Errorf("item %q is not checked; only a checked item is applied", item)
		}
		i := r.pickAt(item)
		if isApplied(r.Picks[i].Status) {
			return fmt.Errorf("item %q already applied; it is locked", item)
		}
		r.Picks[i].Status, r.Picks[i].Result, r.Picks[i].AppliedAt = decisionApplied, result, &now
		if outcome == "failed" {
			r.Picks[i].Status = decisionFailed
			return nil
		}
		for _, it := range r.Items {
			if r.checked(it.ID) && !isApplied(r.Picks[r.pickAt(it.ID)].Status) {
				return nil
			}
		}
		r.Status, r.Result, r.AppliedAt = decisionApplied, "every checked item applied", &now
		return nil
	})
}

// accepted is mgmt's "just takes the recommendation, nothing to read": the
// pick (and every item's mark) is exactly the recommendation, with no note.
func accepted(r *Decision) bool {
	if !r.decided() || strings.TrimSpace(r.Note) != "" || r.ContextChanged {
		return false
	}
	rec := r.recommendedID()
	if rec == "" && !r.recommendsItems() {
		return false
	}
	if rec != "" && r.Action != rec {
		return false
	}
	for _, it := range r.Items {
		if r.recommendsItems() && r.checked(it.ID) != it.Recommended {
			return false
		}
	}
	return true
}

// awaitsReply: the pick is an outcome that waits for the agent's reply.
func (s *Store) awaitsReply(r *Decision) bool {
	o, ok := s.outcome(r.Action)
	return ok && o.AwaitsReply
}

// decisionClass is the item's decision state, one of decisionFilters: what the
// decision.show setting and decisions filter by. A decided pick of an outcome
// has the class its row names.
func (s *Store) decisionClass(q Question) string {
	r := q.Decision
	switch {
	case r.locked():
		return classApplied
	case !r.decided():
		return classUndecided
	}
	if o, ok := s.outcome(r.Action); ok && o.Class != "" {
		return o.Class
	}
	return classDecided
}

var decisionFilters = []string{"pending", classUndecided, classAsked, classLater, "changed", classDecided, classApplied, "all"}

func (s *Store) decisionShown(q Question, filter string) bool {
	class := s.decisionClass(q)
	switch filter {
	case "", "all":
		return true
	case "pending":
		return !q.Decision.locked()
	case "changed":
		return q.Decision.ContextChanged || q.Decision.NewReply
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
	r := q.Decision
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

// Export is the decisions contract an agent reads (`decision export`):
// the shape of mgmt-export/1, with o-questions ids. The fields answer,
// picks and progress are additive to the original.
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

// ExportAnswer is the owner's pick: Kind is option or outcome, ID the stored
// id, Label what the owner saw, Text the note of an outcome that asks for text.
type ExportAnswer struct {
	Kind  string `json:"kind"`
	ID    string `json:"id"`
	Label string `json:"label"`
	Text  string `json:"text,omitempty"`
}

// ExportPick is one checklist item: its mark and what applying it recorded.
type ExportPick struct {
	Item    string `json:"item"`
	Label   string `json:"label"`
	Checked bool   `json:"checked"`
	Note    string `json:"note,omitempty"`
	Status  string `json:"status,omitempty"`
	Result  string `json:"result,omitempty"`
}

type ExportItem struct {
	ID                     string        `json:"id"`
	Key                    string        `json:"key"`
	Group                  string        `json:"group"`
	Source                 string        `json:"source,omitempty"`
	Title                  string        `json:"title"`
	Options                []Option      `json:"options,omitempty"`
	Action                 string        `json:"action,omitempty"`
	Answer                 *ExportAnswer `json:"answer,omitempty"`
	Picks                  []ExportPick  `json:"picks,omitempty"`
	Progress               string        `json:"progress,omitempty"`
	Recommended            string        `json:"recommended,omitempty"`
	RecommendedAction      string        `json:"recommended_action,omitempty"`
	Recommendation         string        `json:"recommendation,omitempty"`
	AcceptedRecommendation bool          `json:"accepted_recommendation"`
	Note                   string        `json:"note,omitempty"`
	Status                 string        `json:"status,omitempty"`
	Result                 string        `json:"result,omitempty"`
	ContextChanged         bool          `json:"context_changed"`
	NewReply               bool          `json:"new_reply"`
}

// ThreadMessage is mgmt's thread row: the owner's asks and the agent's replies.
type ThreadMessage struct {
	Role string `json:"role"`
	Text string `json:"text"`
	At   string `json:"at,omitempty"`
}

func decisionThread(q Question) []ThreadMessage {
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

// exportAnswer is the pick as the agent reads it: an option or an outcome
// (with its note as text when the outcome asks for one). Nil when undecided.
func (s *Store) exportAnswer(r *Decision) *ExportAnswer {
	if r.Action == "" {
		return nil
	}
	a := &ExportAnswer{Kind: "option", ID: r.Action, Label: s.answerLabel(r)}
	if o, ok := s.outcome(r.Action); ok {
		a.Kind = "outcome"
		if o.NeedsNote {
			a.Text = strings.TrimSpace(r.Note)
		}
	}
	return a
}

func exportPicks(r *Decision) []ExportPick {
	var out []ExportPick
	for _, it := range r.Items {
		p := ExportPick{Item: it.ID, Label: it.label()}
		if i := r.pickAt(it.ID); i >= 0 {
			pk := r.Picks[i]
			p.Checked, p.Note, p.Status, p.Result = pk.Checked, pk.Note, pk.Status, pk.Result
		}
		out = append(out, p)
	}
	return out
}

func (s *Store) DecisionExport(group string, includeApplied bool) (Export, error) {
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
		class := s.decisionClass(q)
		if q.Decision.locked() && !includeApplied {
			continue
		}
		if !slices.Contains(x.Groups, g) {
			x.Groups = append(x.Groups, g)
		}
		r := q.Decision
		item := ExportItem{ID: q.ID, Key: q.Key, Group: g, Source: r.Source, Title: q.Title, Options: s.optionsOf(r), Action: r.Action, Answer: s.exportAnswer(r), Picks: exportPicks(r), Progress: r.progress(), Recommended: r.recommendedID(), Recommendation: r.Recommendation, AcceptedRecommendation: accepted(r), Note: r.Note, Status: r.Status, Result: r.Result, ContextChanged: r.ContextChanged, NewReply: r.NewReply}
		item.RecommendedAction = item.Recommended
		if s.awaitsReply(r) && !r.locked() {
			x.Questions = append(x.Questions, ExportQuestion{ID: q.ID, Key: q.Key, Group: g, Source: r.Source, Title: q.Title, Question: lastOwnerQuestion(q), Thread: decisionThread(q)})
			continue
		}
		if class == classUndecided {
			x.Undecided = append(x.Undecided, item)
			continue
		}
		if item.AcceptedRecommendation {
			accepts++
		}
		x.Items = append(x.Items, item)
	}
	x.Counts = map[string]int{"decided": len(x.Items), "accepted": accepts, "asked": len(x.Questions), "undecided": len(x.Undecided)}
	return x, nil
}
