package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func agentStore(s *Store) *Store {
	c := *s
	c.Actor = Actor{Kind: "agent", Name: "lane", Harness: "claude-code"}
	return &c
}

func TestPoseDecideReplyCarryExport(t *testing.T) {
	s := fixture(t)
	ag := agentStore(s)
	p := Pose{Question: "Ship the Decisions page on by default?", Group: "Decision", Source: "plan.md:3", Details: "context v1", Recommendation: "Yes, with an opt-out", Recommended: "on", Options: []Option{{ID: "on", Label: "Turn it on by default"}, {ID: "off", Label: "Leave it off"}}, Key: "k1"}
	posed, e := ag.Pose(p)
	if e != nil || posed.Existed || !posed.Question.forOwner() || posed.Question.Decision.Recommended != "on" || len(posed.Warnings) != 0 || posed.Question.AskedBy.Kind != "agent" {
		t.Fatal(posed, e)
	}
	id := posed.Question.ID
	// Decisions are ordinary attributed writes, including an agent's write.
	if _, e = ag.Decide(id, DecisionChange{Accept: true}); e != nil {
		t.Fatal("agent decision refused", e)
	}
	history, e := s.History()
	if e != nil || history[len(history)-1].By != ag.Actor {
		t.Fatal(history, e)
	}
	if e = ag.Undo(); e != nil {
		t.Fatal(e)
	}
	x, _ := s.DecisionExport("", false)
	if len(x.Undecided) != 1 || len(x.Items) != 0 {
		t.Fatal(x)
	}
	// 1 picks the first numbered action; same as the recommendation, so it is an accept.
	q, e := s.Decide(id, DecisionChange{Choice: 1})
	if e != nil || q.Decision.Action != "on" || q.Status != "answered" {
		t.Fatal(q, e)
	}
	x, _ = s.DecisionExport("", false)
	if len(x.Items) != 1 || !x.Items[0].AcceptedRecommendation || x.Counts["accepted"] != 1 {
		t.Fatal(x)
	}
	// Reposing the same key unchanged changes nothing; changed context keeps the decision and flags it.
	again, e := ag.Pose(p)
	if e != nil || !again.Existed || again.Changed {
		t.Fatal(again, e)
	}
	p.Details = "context v2"
	again, e = ag.Pose(p)
	if e != nil || !again.Changed || again.Question.Decision.Action != "on" || !again.Question.Decision.ContextChanged {
		t.Fatal(again, e)
	}
	x, _ = s.DecisionExport("", false)
	if x.Items[0].AcceptedRecommendation || !x.Items[0].ContextChanged {
		t.Fatal("changed context must be read", x)
	}
	// Ask back: open for the agent, listed as a question with its thread.
	if _, e = s.Decide(id, DecisionChange{Choice: 3}); e == nil {
		t.Fatal("choice out of range accepted")
	}
	note := "Why on by default?"
	q, e = s.Decide(id, DecisionChange{Action: "ask", Note: &note})
	if e != nil || q.Status != "follow-up" || q.Decision.ContextChanged {
		t.Fatal(q, e)
	}
	x, _ = s.DecisionExport("", false)
	if len(x.Questions) != 1 || x.Questions[0].Question != note || len(x.Questions[0].Thread) != 1 || x.Questions[0].Thread[0].Role != "owner" {
		t.Fatal(x)
	}
	// The agent's reply clears the ask and the note the thread already holds.
	rec := "Yes; decision.page turns it off"
	q, e = ag.Reply(id, Reply{Text: "Owners asked for it in plan X.", Recommendation: &rec})
	if e != nil || q.Decision.Action != "" || q.Decision.Note != "" || !q.Decision.NewReply || q.Status != "open" || q.Decision.Recommendation != rec {
		t.Fatal(*q.Decision, q.Status, e)
	}
	if th := decisionThread(q); len(th) != 2 || th[1].Role != "agent" {
		t.Fatal(th)
	}
	// Owner accepts; nothing to read.
	q, e = s.Decide(id, DecisionChange{Accept: true})
	if e != nil || q.Decision.Action != "on" || q.Decision.NewReply || !accepted(q.Decision) {
		t.Fatal(*q.Decision, e)
	}
	// Applying: undecided items refuse; done locks.
	other, _ := ag.Pose(Pose{Question: "Second?", Key: "k2", Options: []Option{{ID: "a"}, {ID: "b"}}})
	if _, e = ag.Applied(other.Question.ID, "", "done", "x"); e == nil {
		t.Fatal("applied an undecided item")
	}
	if q, e = ag.Applied(id, "", "done", "Set decision.page default true; tests pass"); e != nil || q.Decision.Status != decisionApplied {
		t.Fatal(q, e)
	}
	if _, e = s.Decide(id, DecisionChange{Clear: true}); e == nil {
		t.Fatal("decided a locked item")
	}
	if _, e = ag.Reply(id, Reply{Text: "late"}); e == nil {
		t.Fatal("replied to a locked item")
	}
	x, _ = s.DecisionExport("", false)
	if len(x.Items) != 0 || len(x.Undecided) != 1 {
		t.Fatal("applied items leave the default export", x)
	}
	x, _ = s.DecisionExport("", true)
	if len(x.Items) != 1 || x.Items[0].Status != decisionApplied {
		t.Fatal(x)
	}
	// Choice 2 is the question's own second option; outcomes are not numbered.
	if q, e = s.Decide(other.Question.ID, DecisionChange{Choice: 2}); e != nil || q.Decision.Action != "b" {
		t.Fatal(q, e)
	}
	if got := s.optionsOf(other.Question.Decision); len(got) != 2 {
		t.Fatal(got)
	}
}

func TestDecisionUndoAndAnswerGuards(t *testing.T) {
	s := fixture(t)
	plain := ask(t, s, "Owner's own question?")
	if _, e := s.Decide(plain.ID, DecisionChange{Choice: 1}); e == nil {
		t.Fatal("decided an owner's question")
	}
	posed, _ := agentStore(s).Pose(Pose{Question: "Q?", Key: "k"})
	if _, e := s.Decide(posed.Question.ID, DecisionChange{Choice: 1}); e == nil {
		t.Fatal("picked a numbered option of a question with none")
	}
	if _, e := s.Decide(posed.Question.ID, DecisionChange{Action: "later"}); e != nil {
		t.Fatal(e)
	}
	if e := s.Undo(); e != nil {
		t.Fatal(e)
	}
	q, _ := s.Resolve(posed.Question.ID, false)
	if q.Decision.Action != "" {
		t.Fatal("undo kept the decision", q.Decision)
	}
}

func TestScanPlansMatchesOwnerQuestionsPy(t *testing.T) {
	root, _ := filepath.Abs("testdata/plans")
	qs, e := ScanPlans(root, false)
	if e != nil {
		t.Fatal(e)
	}
	// Expected rows from: owner_questions.py scan --root testdata/plans (ids, titles, groups, lines, recommendations).
	want := [][]string{
		{"oq-5bc768fe0995", "#4 Rename o-plans?", "README", "README.md:5", "no."},
		{"oq-cebc9a1515e0", "#1 Which base by default?", "Git compare", "plans/git-compare.md:10", "the merge base."},
		{"oq-01b70a424f47", "#2 Show stat column?", "Git compare", "plans/git-compare.md:11", ""},
		{"oq-b25f0c50b17e", "#1 Group by section? The page could group rows by section.", "Settings layout", "plans/ochazuke-settings-layout.md:7", "yes, sections with headings."},
		{"oq-0c01122c4f1e", "#3 Keep the old layout behind a setting? It costs one setting.", "Settings layout", "plans/ochazuke-settings-layout.md:9", "** keep it for one release"},
	}
	if len(qs) != len(want) {
		t.Fatalf("got %d questions: %+v", len(qs), qs)
	}
	for i, q := range qs {
		p := q.pose()
		got := []string{q.ID, p.Title, p.Group, q.Rel + ":" + itoa(q.Line), q.Recommendation}
		if strings.Join(got, "|") != strings.Join(want[i], "|") {
			t.Fatalf("row %d\n got %q\nwant %q", i, got, want[i])
		}
	}
	all, _ := ScanPlans(root, true)
	if len(all) != len(want)+2 {
		t.Fatal("include answered", len(all))
	}
}

func itoa(n int) string { b, _ := json.Marshal(n); return string(b) }

func TestImportPlansCarriesDecisionsAndRendersDecisions(t *testing.T) {
	s := fixture(t)
	root := filepath.Join(t.TempDir(), "plans")
	if e := os.CopyFS(root, os.DirFS("testdata/plans")); e != nil {
		t.Fatal(e)
	}
	ag := agentStore(s)
	got, e := ag.ImportPlans(root, false)
	if e != nil || got.Created != 5 || got.Found != 5 {
		t.Fatal(got, e)
	}
	got, e = ag.ImportPlans(root, false)
	if e != nil || got.Created != 0 || got.Unchanged != 5 {
		t.Fatal("reimport must be idempotent", got, e)
	}
	byKey := map[string]Question{}
	qs, _ := s.ownerQuestions()
	for _, q := range qs {
		byKey[q.Key] = q
	}
	group := byKey["plan:oq-b25f0c50b17e"]
	keep := byKey["plan:oq-0c01122c4f1e"]
	note := "but only for one release"
	if _, e = s.Decide(group.ID, DecisionChange{Accept: true}); e != nil {
		t.Fatal(e)
	}
	if _, e = s.Decide(keep.ID, DecisionChange{Action: "no", Note: &note}); e != nil {
		t.Fatal(e)
	}
	// Editing the plan text flags the decided question; the decision carries by id.
	path := filepath.Join(root, "plans", "ochazuke-settings-layout.md")
	b, _ := os.ReadFile(path)
	if e = os.WriteFile(path, bytes.Replace(b, []byte("sections with headings"), []byte("sections with rules"), 1), 0600); e != nil {
		t.Fatal(e)
	}
	got, _ = ag.ImportPlans(root, false)
	if got.Changed != 1 {
		t.Fatal(got)
	}
	q, _ := s.Resolve(group.ID, false)
	if q.Decision.Action != "yes" || !q.Decision.ContextChanged || q.Decision.Recommendation != "yes, sections with rules." {
		t.Fatal(q.Decision)
	}
	text, e := s.PlanDecisions("2026-10-31")
	if e != nil {
		t.Fatal(e)
	}
	want := "<!-- plans/ochazuke-settings-layout.md: insert above \"Owner questions\" -->\n## Owner decisions (2026-10-31)\n\n1. **Group by section.** Yes, sections with rules.\n3. **Keep the old layout behind a setting.** No, not the recommendation. but only for one release\n\n"
	if text != want {
		t.Fatalf("got\n%s\nwant\n%s", text, want)
	}
	// A question the plans no longer hold is reported stale, not removed.
	if e = os.Remove(filepath.Join(root, "README.md")); e != nil {
		t.Fatal(e)
	}
	got, _ = ag.ImportPlans(root, false)
	if len(got.Stale) != 1 || got.Stale[0] != byKey["plan:oq-5bc768fe0995"].ID {
		t.Fatal(got)
	}
}
