package main

import (
	"encoding/json"
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

func TestDecisionNotesAccumulate(t *testing.T) {
	s := fixture(t)
	posed, e := agentStore(s).Pose(Pose{
		Question: "Turn the Decisions page on?", Group: "Decision",
		Recommendation: "Yes", Recommended: "yes",
		Options: []Option{{ID: "yes", Label: "yes"}, {ID: "no", Label: "no"}}, Key: "r1",
	})
	if e != nil {
		t.Fatal(e)
	}
	for _, n := range []string{"first note: did you mean 2EOS?", "second note: ring size"} {
		n := n
		if _, e := s.Decide(posed.Question.ID, DecisionChange{Note: &n}); e != nil {
			t.Fatal(e)
		}
	}
	q, _ := s.Resolve(posed.Question.ID, false)
	if got := noteTexts(q); len(got) != 2 || got[0] != "first note: did you mean 2EOS?" || got[1] != "second note: ring size" {
		t.Fatal("notes must append, not replace:", got)
	}
	if q.Entries[0].By.Kind != "owner" || q.Entries[0].At.IsZero() {
		t.Fatal("a note carries author and time", q.Entries[0])
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
