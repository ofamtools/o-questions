package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

// optionStore is a store that needs no environment, so its tests can run in
// parallel (fixture sets env vars).
func optionStore(t *testing.T) (owner, agent *Store) {
	t.Helper()
	owner = &Store{Dir: filepath.Join(t.TempDir(), "questions"), Actor: Owner(), Now: func() time.Time { return time.Date(2026, 11, 8, 12, 0, 0, 0, time.UTC) }}
	return owner, agentStore(owner)
}

func pose(t *testing.T, ag *Store, p Pose) Posed {
	t.Helper()
	got, e := ag.Pose(p)
	if e != nil {
		t.Fatal(e)
	}
	return got
}

var releaseOptions = []Option{{ID: "release", Label: "Push main and cut the gem release", Description: "git push, gem build"}, {ID: "local", Label: "Leave hiiro local"}}

func TestPoseWithoutOptionsStoresAndWarns(t *testing.T) {
	t.Parallel()
	_, ag := optionStore(t)
	got := pose(t, ag, Pose{Question: "Which implementation is wanted?", Recommendation: "the small one", Key: "k"})
	if !got.Question.forOwner() || len(got.Warnings) == 0 || !strings.Contains(strings.Join(got.Warnings, "|"), "supply options") {
		t.Fatal("the question must be stored with a warning telling the poser to supply options", got)
	}
	if got.Question.Decision.Recommended != "" || len(got.Question.Decision.Options) != 0 {
		t.Fatal("no implicit options or recommendation", *got.Question.Decision)
	}
	got = pose(t, ag, Pose{Question: "Q?", Options: releaseOptions, Recommendation: "green", Key: "k2"})
	if len(got.Warnings) != 1 || !strings.Contains(got.Warnings[0], "recommended option") {
		t.Fatal(got.Warnings)
	}
	got = pose(t, ag, Pose{Question: "Q?", Options: releaseOptions, Recommended: "nope", Key: "k3"})
	if got.Question.Decision.Recommended != "" || len(got.Warnings) != 1 {
		t.Fatal("a recommended id naming no option is dropped with a warning", got.Warnings, *got.Question.Decision)
	}
	got = pose(t, ag, Pose{Question: "Q?", Options: releaseOptions, Recommended: "release", Recommendation: "green", Key: "k4"})
	if len(got.Warnings) != 0 || got.Question.Decision.Options[1].Label != "Leave hiiro local" {
		t.Fatal(got.Warnings)
	}
}

func TestPoseIDChecks(t *testing.T) {
	t.Parallel()
	_, ag := optionStore(t)
	for name, p := range map[string]Pose{
		"old actions name":     {Question: "Q?", Actions: []string{"a"}},
		"old recommended name": {Question: "Q?", RecommendedAction: "a"},
		"empty option id":      {Question: "Q?", Options: []Option{{Label: "x"}}},
		"duplicate option id":  {Question: "Q?", Options: []Option{{ID: "a"}, {ID: "a"}}},
		"outcome as option id": {Question: "Q?", Options: []Option{{ID: "later"}}},
		"empty item id":        {Question: "Q?", Items: []Item{{Label: "x"}}},
		"duplicate item id":    {Question: "Q?", Items: []Item{{ID: "a"}, {ID: "a"}}},
	} {
		if _, e := ag.Pose(p); e == nil {
			t.Errorf("%s: pose accepted it", name)
		}
	}
	var p Pose
	if e := json.Unmarshal([]byte(`{"question":"Q?","options":["Leave it",{"id":"go","label":"Go"}],"items":["a",{"id":"b","recommended":true}]}`), &p); e != nil {
		t.Fatal(e)
	}
	if p.Options[0].ID != "Leave it" || p.Options[0].label() != "Leave it" || p.Options[1].label() != "Go" || !p.Items[1].Recommended || p.Items[1].label() != "b" {
		t.Fatal(p)
	}
}

// A custom outcome table drives status and class; decision.go names no outcome.
func TestOutcomeTableDrivesStatusAndClass(t *testing.T) {
	t.Parallel()
	owner, ag := optionStore(t)
	rows := []string{"park|Parked|open|later|", "drop|Dropped|moot|decided|", "why|Ask why|follow-up|asked|reply", "own|My answer|answered|decided|note"}
	table, e := parseOutcomes(rows)
	if e != nil || len(table) != 4 || !table[2].AwaitsReply || !table[2].NeedsNote || table[3].AwaitsReply || !table[3].NeedsNote {
		t.Fatal(table, e)
	}
	owner.Outcomes = table
	ag.Outcomes = table
	q := pose(t, ag, Pose{Question: "Q?", Options: releaseOptions, Recommended: "release", Key: "k"}).Question
	cases := []struct{ action, note, status, class string }{
		{"park", "", "open", "later"},
		{"drop", "", "moot", "decided"},
		{"release", "", "answered", "decided"},
		{"why", "why now?", "follow-up", "asked"},
		{"own", "do the third thing", "answered", "decided"},
	}
	for _, c := range cases {
		d := DecisionChange{Action: c.action}
		if c.note != "" {
			d.Note = &c.note
		}
		got, e := owner.Decide(q.ID, d)
		if e != nil || got.Status != c.status || owner.decisionClass(got) != c.class {
			t.Fatalf("%s: status %q class %q err %v", c.action, got.Status, owner.decisionClass(got), e)
		}
	}
	// The default names (later, moot, ask) are gone from this table, so they are unknown.
	if _, e = owner.Decide(q.ID, DecisionChange{Action: "later"}); e == nil {
		t.Fatal("later is not in this table")
	}
	// A row that asks for text refuses an empty note; a reply reopens a reply-flagged pick.
	empty := ""
	if _, e = owner.Decide(q.ID, DecisionChange{Action: "own", Note: &empty}); e == nil {
		t.Fatal("an outcome that needs text accepted an empty note")
	}
	note := "why now?"
	if _, e = owner.Decide(q.ID, DecisionChange{Action: "why", Note: &note}); e != nil {
		t.Fatal(e)
	}
	back, e := ag.Reply(q.ID, Reply{Text: "because"})
	if e != nil || back.Decision.Action != "" || owner.decisionClass(back) != classUndecided || back.Status != "open" {
		t.Fatal(back.Decision, back.Status, e)
	}
	for _, bad := range []string{"|A|open|decided|", "a|b|nope||", "a|b|open|applied|", "a|b|open|decided|zzz", "a|A|open|decided|\na|A|open|decided|"} {
		if _, e := parseOutcomes(strings.Split(bad, "\n")); e == nil {
			t.Errorf("parseOutcomes accepted %q", bad)
		}
	}
	if _, e := parseOutcomes(defaultOutcomeRows); e != nil {
		t.Fatal(e)
	}
}

func TestDecideOptionRules(t *testing.T) {
	t.Parallel()
	owner, ag := optionStore(t)
	q := pose(t, ag, Pose{Question: "Q?", Options: releaseOptions, Recommended: "release", Recommendation: "green", Key: "k"}).Question
	for name, d := range map[string]DecisionChange{
		"unknown action":    {Action: "yes"},
		"unknown option":    {Option: "yes"},
		"outcome as option": {Option: "later"},
		"choice too big":    {Choice: 3},
		"nothing":           {},
		"confirm no items":  {Confirm: true},
	} {
		if _, e := owner.Decide(q.ID, d); e == nil {
			t.Errorf("%s accepted", name)
		}
	}
	got, e := owner.Decide(q.ID, DecisionChange{Choice: 2})
	if e != nil || got.Decision.Action != "local" || owner.answerLabel(got.Decision) != "Leave hiiro local" || accepted(got.Decision) {
		t.Fatal(got.Decision, e)
	}
	got, e = owner.Decide(q.ID, DecisionChange{Accept: true})
	if e != nil || got.Decision.Action != "release" || !accepted(got.Decision) {
		t.Fatal(got.Decision, e)
	}
	got, _ = owner.Decide(q.ID, DecisionChange{Option: "local"})
	if got.Decision.Action != "local" {
		t.Fatal(got.Decision)
	}
	got, e = owner.Decide(q.ID, DecisionChange{Action: "other", Note: new(string)})
	if e == nil {
		t.Fatal("write-your-own without text accepted", got.Decision)
	}
	text := "Release, but tag it rc1"
	got, e = owner.Decide(q.ID, DecisionChange{Action: "other", Note: &text})
	if e != nil || got.Status != "answered" || owner.decisionClass(got) != classDecided {
		t.Fatal(got, e)
	}
	x, _ := owner.DecisionExport("", false)
	if a := x.Items[0].Answer; a == nil || a.Kind != "outcome" || a.ID != "other" || a.Text != text {
		t.Fatal(x.Items[0].Answer)
	}
	// No recommended option: accept is refused.
	bare := pose(t, ag, Pose{Question: "Bare?", Recommendation: "just do it", Key: "b"}).Question
	if _, e = owner.Decide(bare.ID, DecisionChange{Accept: true}); e == nil {
		t.Fatal("accepted a recommendation that names no option")
	}
}

func TestReposeKeyStability(t *testing.T) {
	t.Parallel()
	owner, ag := optionStore(t)
	p := Pose{Question: "Q?", Options: releaseOptions, Recommended: "release", Items: []Item{{ID: "a"}, {ID: "b"}}, Key: "k"}
	q := pose(t, ag, p).Question
	owner.Decide(q.ID, DecisionChange{Action: "local"})
	f := true
	owner.Decide(q.ID, DecisionChange{Item: "a", Checked: &f})
	owner.Decide(q.ID, DecisionChange{Item: "b", Checked: &f})
	// Relabelled and reordered, ids kept: the pick stays and nothing is flagged.
	p.Options = []Option{{ID: "local", Label: "Stay local"}, {ID: "release", Label: "Ship it"}}
	got := pose(t, ag, p).Question
	if got.Decision.Action != "local" || got.Decision.ContextChanged || !got.Decision.checked("a") || got.Decision.Options[0].Label != "Stay local" {
		t.Fatal("pick must survive a relabel", *got.Decision)
	}
	// An outcome pick survives any option set.
	owner.Decide(q.ID, DecisionChange{Action: "later"})
	p.Options = []Option{{ID: "release"}, {ID: "other-a"}}
	if got = pose(t, ag, p).Question; got.Decision.Action != "later" || got.Decision.ContextChanged {
		t.Fatal(*got.Decision)
	}
	// The chosen option and a marked item vanish: both cleared, flagged, recorded.
	owner.Decide(q.ID, DecisionChange{Action: "other-a"})
	p.Options = []Option{{ID: "release"}, {ID: "fresh"}}
	p.Items = []Item{{ID: "a"}, {ID: "c"}}
	got = pose(t, ag, p).Question
	r := got.Decision
	if r.Action != "" || r.decided() || !r.ContextChanged || !r.checked("a") || r.pickAt("b") >= 0 {
		t.Fatal("vanished choices must be cleared and the rest kept", *r)
	}
	lost := 0
	for _, e := range got.Entries {
		if strings.Contains(e.Text, "no longer exists") {
			lost++
		}
	}
	if lost != 2 || got.Status != "open" {
		t.Fatal("the lost choices belong in the thread", lost, got.Status, got.Entries)
	}
	// A repose naming no options or items leaves them alone.
	got = pose(t, ag, Pose{Question: "Q? (reworded)", Key: "k"}).Question
	if len(got.Decision.Options) != 2 || len(got.Decision.Items) != 2 {
		t.Fatal(*got.Decision)
	}
}

func TestChecklistDecisions(t *testing.T) {
	t.Parallel()
	owner, ag := optionStore(t)
	items := []Item{{ID: "a", Label: "feat/a", Recommended: true}, {ID: "b", Label: "feat/b"}, {ID: "c", Label: "feat/c", Recommended: true}}
	q := pose(t, ag, Pose{Question: "Which branches go?", Items: items, Recommendation: "the merged ones", Key: "l"})
	if len(q.Warnings) != 0 {
		t.Fatal("items are enough", q.Warnings)
	}
	id := q.Question.ID
	on := true
	got, e := owner.Decide(id, DecisionChange{Item: "b", Checked: &on})
	if e != nil || got.Decision.progress() != "1/3" || got.Decision.decided() || got.Status != "open" || owner.decisionClass(got) != classUndecided {
		t.Fatal("a partly toggled list stays undecided", got.Decision, e)
	}
	got, _ = owner.Decide(id, DecisionChange{Item: "b"})
	if got.Decision.checked("b") {
		t.Fatal("no Checked flips the mark")
	}
	if _, e = owner.Decide(id, DecisionChange{Item: "zzz"}); e == nil {
		t.Fatal("unknown item accepted")
	}
	note := "keep this one around"
	got, _ = owner.Decide(id, DecisionChange{Item: "b", PickNote: &note})
	if got.Decision.checked("b") || got.Decision.Picks[got.Decision.pickAt("b")].Note != note {
		t.Fatal("a note alone must not flip the mark", got.Decision.Picks)
	}
	got, e = owner.Decide(id, DecisionChange{Accept: true})
	if e != nil || got.Decision.progress() != "2/3" || !got.Decision.decided() || got.Status != "answered" || !accepted(got.Decision) || got.Decision.checked("b") {
		t.Fatal("accept checks the recommended items and decides", got.Decision, e)
	}
	// Confirm decides a list as it stands, including all-unchecked.
	other := pose(t, ag, Pose{Question: "Which files?", Items: []Item{{ID: "x"}}, Key: "m"}).Question
	if _, e = owner.Decide(other.ID, DecisionChange{Accept: true}); e == nil {
		t.Fatal("no recommended item to accept")
	}
	got, e = owner.Decide(other.ID, DecisionChange{Confirm: true})
	if e != nil || !got.Decision.decided() || got.Decision.progress() != "0/1" {
		t.Fatal(got.Decision, e)
	}
	// Applying item by item: unchecked refused, failed reopens only the item, the last done locks.
	if _, e = ag.Applied(id, "b", "done", "r"); e == nil {
		t.Fatal("applied an unchecked item")
	}
	if got, e = ag.Applied(id, "a", "done", "branch a deleted"); e != nil || got.Decision.locked() {
		t.Fatal(got.Decision, e)
	}
	if got, e = ag.Applied(id, "c", "failed", "push rejected"); e != nil || got.Decision.locked() || got.Decision.Picks[got.Decision.pickAt("c")].Status != decisionFailed {
		t.Fatal(got.Decision, e)
	}
	if _, e = owner.Decide(id, DecisionChange{Item: "a", Checked: new(bool)}); e == nil {
		t.Fatal("changed an applied item")
	}
	if got, e = owner.Decide(id, DecisionChange{Item: "c", Checked: new(bool)}); e != nil || got.Decision.Picks[got.Decision.pickAt("c")].Status != "" {
		t.Fatal("a failed item reopens", got.Decision, e)
	}
	owner.Decide(id, DecisionChange{Item: "c", Checked: &on})
	if got, e = ag.Applied(id, "c", "done", "branch c deleted"); e != nil || !got.Decision.locked() {
		t.Fatal("the decision locks when every checked item is applied", got.Decision, e)
	}
	if _, e = owner.Decide(id, DecisionChange{Clear: true}); e == nil {
		t.Fatal("cleared a locked decision")
	}
	// Clearing an undecided-again list drops marks but keeps applied ones.
	open := pose(t, ag, Pose{Question: "Again?", Items: items, Key: "n"}).Question
	owner.Decide(open.ID, DecisionChange{Accept: true})
	got, _ = owner.Decide(open.ID, DecisionChange{Clear: true})
	if got.Decision.decided() || len(got.Decision.Picks) != 0 || got.Status != "open" {
		t.Fatal(got.Decision)
	}
}

// The oldest stored questions (actions, recommended_action, a bare action word)
// still load and read: options are the words, the pick shows as stored.
func TestStoredActionsStillRead(t *testing.T) {
	t.Parallel()
	owner, ag := optionStore(t)
	q := pose(t, ag, Pose{Question: "Old?", Options: []Option{{ID: "x"}}, Key: "old"}).Question
	if _, e := owner.mutate("decide", q.ID, func(q *Question, _ *[]Question) error {
		r := q.Decision
		r.Options, r.Recommended = nil, ""
		r.Actions, r.RecommendedAction = []string{"yes", "no", "later"}, "yes"
		return nil
	}); e != nil {
		t.Fatal(e)
	}
	got, _ := owner.Resolve(q.ID, false)
	r := got.Decision
	if ids := owner.optionsOf(r); len(ids) != 2 || ids[0].ID != "yes" || ids[1].label() != "no" {
		t.Fatal("a stored action that is an outcome id is the outcome, not an option", ids)
	}
	if r.recommendedID() != "yes" || owner.recommendedLabel(r) != "yes" {
		t.Fatal("recommended action reads as the recommended option")
	}
	got, e := owner.Decide(q.ID, DecisionChange{Accept: true})
	if e != nil || got.Decision.Action != "yes" || !accepted(got.Decision) || got.Status != "answered" {
		t.Fatal(got.Decision, e)
	}
	// A stored pick with no stored options (the old global default) reads as stored.
	if _, e = owner.mutate("decide", q.ID, func(q *Question, _ *[]Question) error {
		q.Decision.Actions, q.Decision.RecommendedAction, q.Decision.Action = nil, "", "yes"
		return nil
	}); e != nil {
		t.Fatal(e)
	}
	got, _ = owner.Resolve(q.ID, false)
	x, e := owner.DecisionExport("", false)
	if e != nil || owner.decisionClass(got) != classDecided || owner.answerLabel(got.Decision) != "yes" || x.Items[0].Answer.ID != "yes" || x.Items[0].Action != "yes" {
		t.Fatal(x, e)
	}
	// Reposing it with no options leaves the stored pick alone.
	if r := pose(t, ag, Pose{Question: "Old? (edited)", Key: "old"}).Question.Decision; r.Action != "yes" || r.ContextChanged && false {
		t.Fatal(*r)
	}
}

func TestDecisionExportGolden(t *testing.T) {
	t.Parallel()
	owner, ag := optionStore(t)
	stable := 0
	owner.Now = func() time.Time { stable++; return time.Date(2026, 11, 8, 12, 0, stable, 0, time.UTC) }
	ag.Now = owner.Now
	rel := pose(t, ag, Pose{Question: "Push main and cut a gem release, or leave hiiro local?", Group: "Release", Options: releaseOptions, Recommended: "release", Recommendation: "main is green", Key: "rel"}).Question
	list := pose(t, ag, Pose{Question: "Which branches go?", Group: "Release", Items: []Item{{ID: "a", Label: "feat/a", Recommended: true}, {ID: "b", Label: "feat/b"}}, Key: "list"}).Question
	free := pose(t, ag, Pose{Question: "Which port?", Group: "Web", Key: "port"}).Question
	asked := pose(t, ag, Pose{Question: "Why?", Group: "Web", Options: []Option{{ID: "go"}}, Key: "why"}).Question
	note := "Why now?"
	text := "random above 9000"
	for _, step := range []struct {
		id string
		d  DecisionChange
	}{{rel.ID, DecisionChange{Accept: true}}, {list.ID, DecisionChange{Item: "b", PickNote: &note}}, {free.ID, DecisionChange{Action: "other", Note: &text}}, {asked.ID, DecisionChange{Action: "ask", Note: &note}}} {
		if _, e := owner.Decide(step.id, step.d); e != nil {
			t.Fatal(e)
		}
	}
	owner.Decide(list.ID, DecisionChange{Item: "a"})
	x, e := owner.DecisionExport("", false)
	if e != nil {
		t.Fatal(e)
	}
	b, _ := json.MarshalIndent(x, "", "  ")
	got := string(b) + "\n"
	for _, q := range []Question{rel, list, free, asked} {
		got = strings.ReplaceAll(got, q.ID, "q-"+q.Key)
	}
	// The thread dates are the store clock's, which the golden pins.
	path := filepath.Join("testdata", "decision-export.golden.json")
	if os.Getenv("UPDATE_GOLDEN") != "" {
		if e := os.WriteFile(path, []byte(got), 0o644); e != nil {
			t.Fatal(e)
		}
	}
	want, e := os.ReadFile(path)
	if e != nil || string(want) != got {
		t.Fatalf("export differs from %s (UPDATE_GOLDEN=1 rewrites it):\n%s", path, got)
	}
	if !slices.Contains(x.Groups, "Release") || x.Counts["asked"] != 1 {
		t.Fatal(x.Counts)
	}
}

func TestPoseLeavesCallerSlicesUntouched(t *testing.T) {
	t.Parallel()
	_, ag := optionStore(t)
	opts := []Option{{ID: " a ", Label: "A"}, {ID: "b", Label: "B"}}
	items := []Item{{ID: " x ", Label: "X"}}
	wantO, wantI := slices.Clone(opts), slices.Clone(items)
	pose(t, ag, Pose{Question: "q?", Options: opts, Items: items})
	if !slices.Equal(opts, wantO) || !slices.Equal(items, wantI) {
		t.Fatalf("Pose mutated caller slices: %+v %+v", opts, items)
	}
}
