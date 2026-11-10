package main

import (
	"strings"
	"testing"
	"time"

	"ofam.dev/ochazuke"
)

func poseYesNo(t *testing.T, s *Store, title, group string, recommended string) Question {
	t.Helper()
	p, e := agentStore(s).Pose(Pose{Question: title + "?", Title: title, Group: group, Options: []Option{{ID: "yes"}, {ID: "no"}}, Recommended: recommended, Key: title})
	if e != nil {
		t.Fatal(e)
	}
	return p.Question
}

func TestDecisionStatesAndDiffers(t *testing.T) {
	s := fixture(t)
	fresh := poseYesNo(t, s, "fresh", "G", "yes")
	agree := poseYesNo(t, s, "agree", "G", "yes")
	differ := poseYesNo(t, s, "differ", "G", "yes")
	bare := poseYesNo(t, s, "bare", "G", "")
	writeIn := poseYesNo(t, s, "write", "G", "yes")
	for id, ch := range map[string]DecisionChange{agree.ID: {Option: "yes"}, differ.ID: {Option: "no"}, bare.ID: {Option: "no"}, writeIn.ID: {Action: "later"}} {
		if _, e := s.Decide(id, ch); e != nil {
			t.Fatal(e)
		}
	}
	rows, e := s.ownerQuestions()
	if e != nil {
		t.Fatal(e)
	}
	got := map[string]string{}
	for _, q := range rows {
		got[q.Title] = s.decisionState(q)
	}
	want := map[string]string{"fresh": stateUndecided, "agree": stateAnswered, "differ": stateDiffers, "bare": stateAnswered, "write": stateLater}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("%s: state %q, want %q", k, got[k], v)
		}
	}
	_ = fresh
}

func TestChecklistDiffersByMarks(t *testing.T) {
	s, q := checklistFixture(t)
	if _, e := s.Decide(q.ID, DecisionChange{Item: "a", Checked: ptr(true)}); e != nil {
		t.Fatal(e)
	}
	if _, e := s.Decide(q.ID, DecisionChange{Confirm: true}); e != nil {
		t.Fatal(e)
	}
	q, _ = s.Resolve(q.ID, false)
	if d, ok := q.Decision.differs(); d || !ok {
		t.Fatal("recommended marks only: agrees", d, ok)
	}
	if _, e := s.Decide(q.ID, DecisionChange{Item: "b", Checked: ptr(true)}); e != nil {
		t.Fatal(e)
	}
	q, _ = s.Resolve(q.ID, false)
	if d, _ := q.Decision.differs(); !d || !q.Decision.itemDiffers(q.Decision.Items[1]) || q.Decision.itemDiffers(q.Decision.Items[0]) {
		t.Fatal("an extra mark differs")
	}
}

func ptr[T any](v T) *T { return &v }

func TestDecisionRowsTonesAndColorOff(t *testing.T) {
	s := fixture(t)
	differ := poseYesNo(t, s, "differ", "G", "yes")
	poseYesNo(t, s, "fresh", "G", "yes")
	if _, e := s.Decide(differ.ID, DecisionChange{Option: "no"}); e != nil {
		t.Fatal(e)
	}
	for _, color := range []bool{true, false} {
		a, _ := mountDecisions(t, s, map[string]any{"decision.color": color, "decision.show": "all"})
		rows, e := a.decisionRows(nil)
		if e != nil {
			t.Fatal(e)
		}
		for _, r := range rows {
			if r.Label != "differ" && r.Label != "fresh" {
				continue
			}
			tone := ""
			if len(r.Tones) > 0 {
				tone = r.Tones[0].Tone
			}
			wantTone, wantGlyph := "", ""
			if color && r.Label == "differ" {
				wantTone, wantGlyph = ochazuke.ToneWorking, differsMark
			}
			if color && r.Label == "fresh" {
				wantTone, wantGlyph = ochazuke.ToneNeedsInput, "○"
			}
			if tone != wantTone || (wantGlyph != "" && !strings.HasPrefix(r.Cells[0], wantGlyph)) || (!color && (len(r.Tones) > 0 || strings.ContainsAny(r.Cells[0], "○≠"))) {
				t.Errorf("color=%v %s: tone %q cell %q", color, r.Label, tone, r.Cells[0])
			}
		}
	}
}

func TestArchiveMatchingDryRunApplyUndo(t *testing.T) {
	s := fixture(t)
	now := time.Date(2026, 10, 31, 12, 0, 0, 0, time.UTC)
	s.Now = func() time.Time { return now }
	a1 := poseYesNo(t, s, "one", "G", "yes")
	a2 := poseYesNo(t, s, "two", "G", "yes")
	poseYesNo(t, s, "three", "H", "yes")
	for _, q := range []Question{a1, a2} {
		if _, e := s.Decide(q.ID, DecisionChange{Option: "yes"}); e != nil {
			t.Fatal(e)
		}
		if _, e := s.Applied(q.ID, "", "done", "ok"); e != nil {
			t.Fatal(e)
		}
	}
	now = now.AddDate(0, 0, 40)
	plan, e := s.ArchiveMatching(classApplied, "", 30, false)
	if e != nil || plan.Count != 2 || plan.Done {
		t.Fatal(plan, e)
	}
	if qs, _ := s.ownerQuestions(); len(qs) != 3 {
		t.Fatal("a dry run archived something")
	}
	if p, _ := s.ArchiveMatching(classApplied, "", 60, true); p.Count != 0 {
		t.Fatal("younger than 60 days must not match", p)
	}
	plan, e = s.ArchiveMatching(classApplied, "", 30, true)
	if e != nil || plan.Count != 2 || !plan.Done {
		t.Fatal(plan, e)
	}
	if qs, _ := s.ownerQuestions(); len(qs) != 1 {
		t.Fatal("archived questions must be hidden", len(qs))
	}
	if qs, _ := s.ownerQuestionsWith(true); len(qs) != 3 {
		t.Fatal("archived questions are kept", len(qs))
	}
	if e = s.Undo(); e != nil {
		t.Fatal(e)
	}
	if qs, _ := s.ownerQuestions(); len(qs) != 3 {
		t.Fatal("one undo must bring the whole batch back", len(qs))
	}
	if plan, _ = s.ArchiveMatching("all", "G", 0, true); plan.Count != 2 {
		t.Fatal("archiving a group", plan)
	}
	ag := agentStore(s)
	if _, e = ag.ArchiveMany([]string{a1.ID}); e != nil {
		t.Fatal("agents may archive:", e)
	}
}
