package main

import (
	"strings"
	"testing"
)

func blockerIDs(t *testing.T, s *Store) map[string]BlockerRow {
	t.Helper()
	qs, e := s.Blockers()
	if e != nil {
		t.Fatal(e)
	}
	out := map[string]BlockerRow{}
	for _, q := range qs {
		out[q.Title] = s.blockerRow(q)
	}
	return out
}

func TestBlockerListAddDone(t *testing.T) {
	s := fixture(t)
	ag := agentStore(s)
	open, e := ag.Pose(Pose{Question: "Ship it?", Options: []Option{{ID: "yes"}, {ID: "no"}}, Key: "open"})
	if e != nil {
		t.Fatal(e)
	}
	decided, _ := ag.Pose(Pose{Question: "Rename it?", Options: []Option{{ID: "yes"}}, Key: "decided"})
	applied, _ := ag.Pose(Pose{Question: "Drop it?", Options: []Option{{ID: "yes"}}, Key: "applied"})
	if _, e := s.Decide(decided.Question.ID, DecisionChange{Action: "yes"}); e != nil {
		t.Fatal(e)
	}
	s.Decide(applied.Question.ID, DecisionChange{Action: "yes"})
	if _, e := s.Applied(applied.Question.ID, "", "done", "ok"); e != nil {
		t.Fatal(e)
	}
	posed, e := ag.BlockerAdd("Restart the daemon", "after the merge", "restart", "", Source{})
	if e != nil || posed.Existed {
		t.Fatal(posed, e)
	}
	rows := blockerIDs(t, s)
	if len(rows) != 2 || rows["Ship it?"].Kind != kindDecision || rows["Restart the daemon"].Kind != kindItem {
		t.Fatalf("%+v", rows)
	}
	d, it := rows["Ship it?"], rows["Restart the daemon"]
	if d.ID != open.Question.ID || d.Resolve != "question.decision.decide" || d.Source != "o-questions" || d.Age == "" || d.ResolveArgs["target"] != d.ID {
		t.Fatalf("%+v", d)
	}
	if it.Resolve != "question.blocker.done" || it.Group != blockerGroup || it.Details != "after the merge" {
		t.Fatalf("%+v", it)
	}
	// Adding the same key again does not duplicate.
	again, e := ag.BlockerAdd("Restart the daemon", "", "restart", "", Source{})
	if e != nil || !again.Existed || len(blockerIDs(t, s)) != 2 {
		t.Fatal(again, e)
	}
	// A decision is not marked done here.
	if _, e := ag.BlockerDone(d.ID, ""); e == nil || !strings.Contains(e.Error(), "decision.decide") {
		t.Fatalf("done on a decision: %v", e)
	}
	if q, e := ag.BlockerDone(it.ID, "restarted"); e != nil || !q.Decision.locked() {
		t.Fatal(q, e)
	}
	rows = blockerIDs(t, s)
	if len(rows) != 1 || rows["Ship it?"].ID == "" {
		t.Fatalf("%+v", rows)
	}
	// Done twice is quiet; re-adding a finished item does not reopen it.
	if _, e := s.BlockerDone(it.ID, ""); e != nil {
		t.Fatal(e)
	}
	if _, e := ag.BlockerAdd("Restart the daemon", "", "restart", "", Source{}); e != nil || len(blockerIDs(t, s)) != 1 {
		t.Fatal(e)
	}
	// Free text is never refused.
	if _, e := ag.BlockerAdd("line one\nline two: \"quoted\"", "", "", "", Source{}); e != nil {
		t.Fatal(e)
	}
	if _, e := ag.BlockerAdd("  ", "", "", "", Source{}); e == nil {
		t.Fatal("empty text accepted")
	}
}

func TestBlockerOwnerDoneByCheckingItem(t *testing.T) {
	s := fixture(t)
	p, _ := agentStore(s).BlockerAdd("Merge branch X", "", "", "", Source{})
	if _, e := s.Decide(p.Question.ID, DecisionChange{Item: blockerItem}); e != nil {
		t.Fatal(e)
	}
	if _, e := s.Decide(p.Question.ID, DecisionChange{Confirm: true}); e != nil {
		t.Fatal(e)
	}
	if len(blockerIDs(t, s)) != 0 {
		t.Fatal("decided item still listed")
	}
}
