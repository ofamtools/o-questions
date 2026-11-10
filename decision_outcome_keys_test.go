package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func outcomeFixture(t *testing.T) (*Store, []Question) {
	t.Helper()
	s := fixture(t)
	ag := agentStore(s)
	var qs []Question
	for i, text := range []string{"Push the release?", "Which port?", "Cut the tag?", "Merge it?"} {
		posed, e := ag.Pose(Pose{Question: text, Group: "G", Options: []Option{{ID: "yes", Label: "Push now", Description: "tags and pushes"}, {ID: "no", Label: "Keep local"}}, Recommended: "yes", Recommendation: "because", Key: string(rune('a' + i))})
		if e != nil {
			t.Fatal(e)
		}
		qs = append(qs, posed.Question)
	}
	return s, qs
}

// submitForm presses enter until the arguments form has closed (the form walks
// its missing fields, then the optional target, before it runs).
func submitForm(t *testing.T, p *page) {
	t.Helper()
	for i := 0; i < 5 && p.a.r.TopModal() != ""; i++ {
		sendKey(t, p, "enter")
	}
	if p.a.r.TopModal() != "" {
		t.Fatal("the form did not close:\n" + frameText(p))
	}
}

func TestDecisionOutcomeKeys(t *testing.T) {
	s, qs := outcomeFixture(t)
	_, p := mountDecisions(t, s, nil)
	answer := func(i int) Decision { q, _ := s.Resolve(qs[i].ID, false); return *q.Decision }
	frame := frameText(p)
	for _, want := range []string{"1. Push now", "tags and pushes", "2. Keep local", "(recommended · not confirmed)", "l Later · m Moot · ? Ask the agent · w Write your own answer"} {
		if !strings.Contains(frame, want) {
			t.Fatalf("missing %q:\n%s", want, frame)
		}
	}
	// A topic header is no question: l on it is the library's key, not a decision.
	selectRow(t, p, "topic/G")
	sendKey(t, p, "l")
	for i := range qs {
		if answer(i).Action != "" {
			t.Fatal("l on a topic row decided a question")
		}
	}
	selectRow(t, p, "question/"+qs[0].ID)
	sendKey(t, p, "l")
	if d := answer(0); d.Action != "later" {
		t.Fatal("l is later", d)
	}
	selectRow(t, p, "question/"+qs[1].ID)
	sendKey(t, p, "m")
	if d := answer(1); d.Action != "moot" {
		t.Fatal("m is moot", d)
	}
	if q, _ := s.Resolve(qs[1].ID, false); q.Status != "moot" {
		t.Fatal("the outcome row names the status", q.Status)
	}
	// w asks for the text through the form, then writes it as the answer.
	selectRow(t, p, "question/"+qs[2].ID)
	sendKey(t, p, "w")
	if d := answer(2); d.Action != "" {
		t.Fatal("w decides only once the text is given")
	}
	sendKey(t, p, "enter") // to the missing text
	typeParameter(t, p, "port 9000")
	submitForm(t, p)
	if d := answer(2); d.Action != "other" || d.Note != "port 9000" {
		t.Fatal("w writes your own answer", d)
	}
	// ? asks the agent: the text is the follow-up.
	selectRow(t, p, "question/"+qs[3].ID)
	sendKey(t, p, "?")
	sendKey(t, p, "enter")
	typeParameter(t, p, "why now")
	submitForm(t, p)
	if q, _ := s.Resolve(qs[3].ID, false); q.Decision.Action != "ask" || q.Status != "follow-up" || len(q.Entries) == 0 || q.Entries[len(q.Entries)-1].Text != "why now" {
		t.Fatal("? asks the agent", q.Decision, q.Entries)
	}
	// The number keys pick the Nth option, as before.
	selectRow(t, p, "question/"+qs[0].ID)
	sendKey(t, p, "2")
	if d := answer(0); d.Action != "no" {
		t.Fatal("2 picks the second option", d)
	}
	if frame = frameText(p); !strings.Contains(frame, "✓ chosen") {
		t.Fatal("the pick is marked in the Options section:\n" + frame)
	}
}

// The outcome keys come from the outcome table in the settings file, not from
// code: a table that names other keys binds those.
func TestDecisionOutcomeKeysFollowTheTable(t *testing.T) {
	s, qs := outcomeFixture(t)
	dir := filepath.Join(os.Getenv("XDG_CONFIG_HOME"), "o-questions")
	if e := os.MkdirAll(dir, 0700); e != nil {
		t.Fatal(e)
	}
	table := `{"decision.detail": "sections", "decision.outcomes": ["later|Later|answered|later||L", "other|Write it|answered|decided|note|e"]}`
	if e := os.WriteFile(filepath.Join(dir, "settings.json"), []byte(table), 0600); e != nil {
		t.Fatal(e)
	}
	a, p := mountDecisions(t, s, nil)
	for _, k := range a.decisionKeys() {
		if k.key == "l" || k.key == "m" || k.key == "w" {
			t.Fatalf("%s is no outcome key in this table", k.key)
		}
	}
	selectRow(t, p, "question/"+qs[0].ID)
	sendKey(t, p, "L")
	if q, _ := s.Resolve(qs[0].ID, false); q.Decision.Action != "later" {
		t.Fatal("L is bound by the table", q.Decision)
	}
	if frame := frameText(p); !strings.Contains(frame, "L Later · e Write it") {
		t.Fatal(frame)
	}
}

// decision.labels off draws the compact one-line list, as before.
func TestDecisionLabelsOff(t *testing.T) {
	s, qs := outcomeFixture(t)
	_, p := mountDecisions(t, s, map[string]any{"decision.labels": false})
	selectRow(t, p, "question/"+qs[0].ID)
	frame := frameText(p)
	if !strings.Contains(frame, "1 Push now · 2 Keep local · Later · Moot · Ask the agent") {
		t.Fatal(frame)
	}
	for _, gone := range []string{"tags and pushes", "l Later"} {
		if strings.Contains(frame, gone) {
			t.Fatalf("labels off still shows %q", gone)
		}
	}
}
