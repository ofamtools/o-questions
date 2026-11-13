package main

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
)

// pose keeps a bare-string option and names an unknown key on an option or
// item object with did-you-mean (never refused).
func TestPoseNamesUnknownOptionKeys(t *testing.T) {
	s := &Store{Dir: filepath.Join(t.TempDir(), "q"), Actor: Owner()}
	out, e := runCLI(t, s, "question", "pose", "Which?", "--options", `["plain",{"id":"b","lable":"B"}]`, "--key", "k1")
	if e != nil {
		t.Fatal(e, out)
	}
	if !strings.Contains(out, `option 2: unknown key "lable" was not saved (did you mean "label"?)`) {
		t.Fatalf("notice: %s", out)
	}
}

// Record first: an empty question is saved as untitled-STAMP, an empty reply
// is recorded.
func TestEmptyQuestionAndReplyAreSaved(t *testing.T) {
	s := &Store{Dir: filepath.Join(t.TempDir(), "q"), Actor: Owner()}
	asked, e := s.Ask(Ask{Question: ""})
	if e != nil || !strings.HasPrefix(asked.Question.Title, "untitled-") {
		t.Fatalf("%v %q", e, asked.Question.Title)
	}
	if _, e = s.mutate("answer", asked.Question.ID, func(q *Question, _ *[]Question) error {
		return s.appendEntry(q, Entry{Kind: "answer"})
	}); e != nil {
		t.Fatal(e)
	}
}

// A stored tuios_session reads as mtm_session (one migrateLegacy row).
func TestLegacyTuiosSessionKey(t *testing.T) {
	b, e := migrateLegacy([]byte(`{"source":{"tuios_session":"hub"}}`))
	if e != nil {
		t.Fatal(e)
	}
	var q Question
	if e = json.Unmarshal(b, &q); e != nil || q.Source.MtmSession != "hub" || strings.Contains(string(b), "tuios_session") {
		t.Fatalf("%v %s", e, b)
	}
}
