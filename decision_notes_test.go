package main

import (
	"encoding/json"
	"strings"
	"testing"
)

func noteTexts(q Question) []string {
	var out []string
	for _, e := range q.Entries {
		if e.Kind == "note" {
			out = append(out, e.Text)
		}
	}
	return out
}

func TestDecisionNotesAccumulateAndShow(t *testing.T) {
	s, p, qs := decisionFixturePage(t, nil)
	for _, n := range []string{"first note: did you mean 2EOS?", "second note: ring size"} {
		n := n
		if _, e := s.Decide(qs[0].ID, DecisionChange{Note: &n}); e != nil {
			t.Fatal(e)
		}
	}
	q, _ := s.Resolve(qs[0].ID, false)
	if got := noteTexts(q); len(got) != 2 || got[0] != "first note: did you mean 2EOS?" || got[1] != "second note: ring size" {
		t.Fatal("notes must append, not replace:", got)
	}
	if q.Entries[0].By.Kind != "owner" || q.Entries[0].At.IsZero() {
		t.Fatal("a note carries author and time", q.Entries[0])
	}
	if e := p.reload(true); e != nil {
		t.Fatal(e)
	}
	l := p.v.List(decisionListID)
	if e := l.Select(decisionListID + "/question/" + qs[0].ID); e != nil {
		t.Fatal(e)
	}
	p.m.Height = 44 // the Options section takes room above the thread
	frame := frameText(p)
	for _, want := range []string{"first note", "second note", "Notes, asks and replies"} {
		if !strings.Contains(frame, want) {
			t.Fatalf("detail missing %q:\n%s", want, frame)
		}
	}
	rows, e := p.a.decisionRows(nil)
	if e != nil {
		t.Fatal(e)
	}
	found := false
	for _, r := range rows {
		if r.ID == "question/"+qs[0].ID {
			found = r.Cells[4] == "2"
		}
	}
	if !found {
		t.Fatal("the row must show the thread count 2")
	}
}

func TestDecisionNoteMigratesIntoThread(t *testing.T) {
	s := fixture(t)
	posed, e := agentStore(s).Pose(Pose{Question: "Hook?", Key: "k"})
	if e != nil {
		t.Fatal(e)
	}
	n := "By upstream do you mean 2EOS?"
	if _, e = s.Decide(posed.Question.ID, DecisionChange{Note: &n}); e != nil {
		t.Fatal(e)
	}
	q, _ := s.Resolve(posed.Question.ID, false)
	b, _ := json.Marshal(q)
	var doc map[string]any
	_ = json.Unmarshal(b, &doc)
	doc["entries"] = []any{} // data written before notes joined the thread
	b, _ = json.Marshal(doc)
	out, e := migrateLegacy(b)
	if e != nil {
		t.Fatal(e)
	}
	var got Question
	if e = json.Unmarshal(out, &got); e != nil {
		t.Fatal(e)
	}
	if ts := noteTexts(got); len(ts) != 1 || ts[0] != n || got.Entries[0].By.Kind != "owner" {
		t.Fatal(got.Entries)
	}
	again, _ := migrateLegacy(out)
	if string(again) != string(out) {
		t.Fatal("migration must be idempotent")
	}
}
