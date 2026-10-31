package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

func agentStore(s *Store) *Store {
	c := *s
	c.Actor = Actor{Kind: "agent", Name: "lane", Harness: "claude-code"}
	return &c
}

func TestPoseDecideReplyCarryExport(t *testing.T) {
	s := fixture(t)
	s.ReviewAsk = true
	ag := agentStore(s)
	p := Pose{Question: "Ship the Review page on by default?", Group: "Review", Source: "plan.md:3", Details: "context v1", Recommendation: "Yes, with an opt-out", Key: "k1"}
	posed, e := ag.Pose(p)
	if e != nil || posed.Existed || !posed.Question.forOwner() || posed.Question.Review.RecommendedAction != "yes" || posed.Question.AskedBy.Kind != "agent" {
		t.Fatal(posed, e)
	}
	id := posed.Question.ID
	// Agents never decide for the owner.
	var refused *RefusedError
	if _, e = ag.Decide(id, Decision{Accept: true}); !errors.As(e, &refused) {
		t.Fatal("agent decided", e)
	}
	x, _ := s.Decisions("", false)
	if len(x.Undecided) != 1 || len(x.Items) != 0 {
		t.Fatal(x)
	}
	// 1 picks the first numbered action; same as the recommendation, so it is an accept.
	q, e := s.Decide(id, Decision{Choice: 1})
	if e != nil || q.Review.Action != "yes" || q.Status != "answered" {
		t.Fatal(q, e)
	}
	x, _ = s.Decisions("", false)
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
	if e != nil || !again.Changed || again.Question.Review.Action != "yes" || !again.Question.Review.ContextChanged {
		t.Fatal(again, e)
	}
	x, _ = s.Decisions("", false)
	if x.Items[0].AcceptedRecommendation || !x.Items[0].ContextChanged {
		t.Fatal("changed context must be read", x)
	}
	// Ask back: open for the agent, listed as a question with its thread.
	if _, e = s.Decide(id, Decision{Choice: 6}); e == nil {
		t.Fatal("choice out of range accepted")
	}
	note := "Why on by default?"
	q, e = s.Decide(id, Decision{Action: askOut, Note: &note})
	if e != nil || q.Status != "follow-up" || q.Review.ContextChanged {
		t.Fatal(q, e)
	}
	x, _ = s.Decisions("", false)
	if len(x.Questions) != 1 || x.Questions[0].Question != note || len(x.Questions[0].Thread) != 1 || x.Questions[0].Thread[0].Role != "owner" {
		t.Fatal(x)
	}
	// The agent's reply clears the ask and the note the thread already holds.
	rec := "Yes; review.page turns it off"
	q, e = ag.Reply(id, Reply{Text: "Owners asked for it in plan X.", Recommendation: &rec})
	if e != nil || q.Review.Action != "" || q.Review.Note != "" || !q.Review.NewReply || q.Status != "open" || q.Review.Recommendation != rec {
		t.Fatal(*q.Review, q.Status, e)
	}
	if th := reviewThread(q); len(th) != 2 || th[1].Role != "agent" {
		t.Fatal(th)
	}
	// Owner accepts; nothing to read.
	q, e = s.Decide(id, Decision{Accept: true})
	if e != nil || q.Review.Action != "yes" || q.Review.NewReply || !accepted(q.Review) {
		t.Fatal(*q.Review, e)
	}
	// Applying: undecided items refuse; done locks.
	other, _ := ag.Pose(Pose{Question: "Second?", Key: "k2", Actions: []string{"a", "b"}})
	if _, e = ag.Applied(other.Question.ID, "done", "x"); e == nil {
		t.Fatal("applied an undecided item")
	}
	if q, e = ag.Applied(id, "done", "Set review.page default true; tests pass"); e != nil || q.Review.State != reviewApplied {
		t.Fatal(q, e)
	}
	if _, e = s.Decide(id, Decision{Clear: true}); e == nil {
		t.Fatal("decided a locked item")
	}
	if _, e = ag.Reply(id, Reply{Text: "late"}); e == nil {
		t.Fatal("replied to a locked item")
	}
	x, _ = s.Decisions("", false)
	if len(x.Items) != 0 || len(x.Undecided) != 1 {
		t.Fatal("applied items leave the default export", x)
	}
	x, _ = s.Decisions("", true)
	if len(x.Items) != 1 || x.Items[0].State != reviewApplied {
		t.Fatal(x)
	}
	// Item actions: choice 2 is the item's own second action; 3 is ask.
	if q, e = s.Decide(other.Question.ID, Decision{Choice: 2}); e != nil || q.Review.Action != "b" {
		t.Fatal(q, e)
	}
	if got := reviewActions(other.Question, s.reviewDefaults(), true); strings.Join(got, ",") != "a,b,ask" {
		t.Fatal(got)
	}
}

func TestReviewUndoAndAnswerGuards(t *testing.T) {
	s := fixture(t)
	s.ReviewAsk = true
	plain := ask(t, s, "Owner's own question?")
	if _, e := s.Decide(plain.ID, Decision{Choice: 1}); e == nil {
		t.Fatal("decided an owner's question")
	}
	posed, _ := agentStore(s).Pose(Pose{Question: "Q?", Key: "k"})
	if _, e := s.Decide(posed.Question.ID, Decision{Choice: 1}); e != nil {
		t.Fatal(e)
	}
	if e := s.Undo(); e != nil {
		t.Fatal(e)
	}
	q, _ := s.Resolve(posed.Question.ID, false)
	if q.Review.Action != "" {
		t.Fatal("undo kept the decision", q.Review)
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
	s.ReviewAsk = true
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
	if _, e = s.Decide(group.ID, Decision{Accept: true}); e != nil {
		t.Fatal(e)
	}
	if _, e = s.Decide(keep.ID, Decision{Action: "no", Note: &note}); e != nil {
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
	if q.Review.Action != "yes" || !q.Review.ContextChanged || q.Review.Recommendation != "yes, sections with rules." {
		t.Fatal(q.Review)
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

func TestReviewWebRoundTrip(t *testing.T) {
	s := fixture(t)
	ag := agentStore(s)
	a, e := newApp(s, strings.NewReader(""), &bytes.Buffer{}, &bytes.Buffer{})
	if e != nil {
		t.Fatal(e)
	}
	first, _ := ag.Pose(Pose{Question: "First?", Group: "G", Recommendation: "do it", Key: "a"})
	second, _ := ag.Pose(Pose{Question: "Second?", Group: "G", Key: "b"})
	mgmt := filepath.Join(t.TempDir(), "mgmt.py")
	if e = os.WriteFile(mgmt, []byte("# fake"), 0600); e != nil {
		t.Fatal(e)
	}
	if e = a.r.Settings().Set("review.mgmt", mgmt); e != nil {
		t.Fatal(e)
	}
	var argv []string
	// The fake server stands for the owner using mgmt's page: it edits the session file.
	a.run = func(cmd *exec.Cmd) error {
		argv = cmd.Args
		path := cmd.Args[3]
		b, e := os.ReadFile(path)
		if e != nil {
			return e
		}
		var sess map[string]any
		if e = json.Unmarshal(b, &sess); e != nil {
			return e
		}
		if sess["version"].(float64) != 1 || sess["kind"] != "o-questions" {
			t.Errorf("session header %v", sess)
		}
		items := sess["items"].([]any)
		one, two := items[0].(map[string]any), items[1].(map[string]any)
		if one["recommended_action"] != "yes" || one["source"] != "o-questions:"+first.Question.ID {
			t.Errorf("item %v", one)
		}
		one["action"] = "yes"
		two["action"], two["note"] = "ask", "What would it cost?"
		b, _ = json.Marshal(sess)
		return os.WriteFile(path, b, 0600)
	}
	if e = a.reviewCommand([]string{"--web", "--no-open", "--port", "0"}); e != nil {
		t.Fatal(e)
	}
	if len(argv) != 7 || argv[0] != "python3" || argv[1] != mgmt || argv[2] != "serve" || argv[4] != "--no-open" {
		t.Fatal(argv)
	}
	q1, _ := s.Resolve(first.Question.ID, false)
	q2, _ := s.Resolve(second.Question.ID, false)
	if q1.Review.Action != "yes" || q2.Review.Action != "ask" || lastOwnerQuestion(q2) != "What would it cost?" {
		t.Fatal(q1.Review, q2.Review, q2.Entries)
	}
}

func reviewFixturePage(t *testing.T, settings map[string]any) (*Store, *page, []Question) {
	t.Helper()
	s := fixture(t)
	ag := agentStore(s)
	var qs []Question
	for _, p := range []Pose{
		{Question: "Turn the Review page on?", Group: "Review", Recommendation: "Yes, opt-out setting", Key: "r1"},
		{Question: "Which port?", Group: "Web", Actions: []string{"8765", "random"}, Key: "w1"},
	} {
		posed, e := ag.Pose(p)
		if e != nil {
			t.Fatal(e)
		}
		qs = append(qs, posed.Question)
	}
	a, e := newApp(s, strings.NewReader(""), &bytes.Buffer{}, &bytes.Buffer{})
	if e != nil {
		t.Fatal(e)
	}
	for k, v := range settings {
		if e = a.r.Settings().Set(k, v); e != nil {
			t.Fatal(e)
		}
	}
	p, e := mountSpec(a, reviewSpec(a))
	if e != nil {
		t.Fatal(e)
	}
	p.m.Width, p.m.Height = 140, 30
	return s, p, qs
}

func TestReviewPageKeys(t *testing.T) {
	s, p, qs := reviewFixturePage(t, nil)
	l := p.v.List(reviewListID)
	if l.Shown() != 4 {
		t.Fatalf("two topics and two questions, got %d", l.Shown())
	}
	if e := l.Select(reviewListID + "/question/" + qs[0].ID); e != nil {
		t.Fatal(e)
	}
	frame := frameText(p)
	for _, want := range []string{"Review  0/1 decided", "yes?", "recommended · not confirmed", "1 yes · 2 no · 3 later · 4 moot · 5 ask"} {
		if !strings.Contains(frame, want) {
			t.Fatalf("missing %q:\n%s", want, frame)
		}
	}
	sendKey(t, p, "a")
	q, _ := s.Resolve(qs[0].ID, false)
	if q.Review.Action != "yes" || !accepted(q.Review) {
		t.Fatal("a must accept", q.Review)
	}
	if frame = frameText(p); !strings.Contains(frame, "recommended · accepted") || !strings.Contains(frame, "Review  1/1 decided") {
		t.Fatal(frame)
	}
	sendKey(t, p, "x")
	if q, _ = s.Resolve(qs[0].ID, false); q.Review.Action != "" {
		t.Fatal("x must clear", q.Review)
	}
	if e := l.Select(reviewListID + "/question/" + qs[1].ID); e != nil {
		t.Fatal(e)
	}
	sendKey(t, p, "2")
	if q, _ = s.Resolve(qs[1].ID, false); q.Review.Action != "random" {
		t.Fatal("2 must pick the item's second action", q.Review)
	}
	// o cycles review.show: undecided hides the decided question and its topic.
	sendKey(t, p, "o")
	if got := p.a.r.Settings().String("review.show"); got != "undecided" {
		t.Fatal(got)
	}
	if l.Shown() != 2 {
		t.Fatal("undecided view", l.Shown(), frameText(p))
	}
}

func TestReviewPageOptOut(t *testing.T) {
	s := fixture(t)
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	dir := filepath.Join(os.Getenv("XDG_CONFIG_HOME"), "o-questions")
	if e := os.MkdirAll(dir, 0700); e != nil {
		t.Fatal(e)
	}
	if e := os.WriteFile(filepath.Join(dir, "settings.json"), []byte(`{"review.page": false}`), 0600); e != nil {
		t.Fatal(e)
	}
	a, e := newApp(s, strings.NewReader(""), &bytes.Buffer{}, &bytes.Buffer{})
	if e != nil {
		t.Fatal(e)
	}
	if a.r.Instances[reviewPageID] != nil {
		t.Fatal("review.page false still declared the page")
	}
}

func frameText(p *page) string {
	return ansi.Strip(p.m.View().Content)
}
