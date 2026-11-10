package main

import (
	"os"
	"strings"
	"testing"
)

// editDoc rewrites the working file of a decision the way the editor would.
func editDoc(t *testing.T, id string, f func(string) string) {
	t.Helper()
	b, e := os.ReadFile(docFile(id))
	if e != nil {
		t.Fatal(e)
	}
	if e = os.WriteFile(docFile(id), []byte(f(string(b))), 0o600); e != nil {
		t.Fatal(e)
	}
}

func TestDocRoundTripChoiceChecklistAndNote(t *testing.T) {
	s := fixture(t)
	q := poseYesNo(t, s, "nightly", "G", "no")
	if e := s.docWrite(q); e != nil {
		t.Fatal(e)
	}
	text, _ := os.ReadFile(docFile(q.ID))
	for _, want := range []string{"# nightly", "## Your choice", "- [ ] yes <!-- option:yes -->", "- [ ] no (recommended) <!-- option:no -->", "<!-- outcome:later -->", "## Note"} {
		if !strings.Contains(string(text), want) {
			t.Fatalf("missing %q in\n%s", want, text)
		}
	}
	editDoc(t, q.ID, func(s string) string { return strings.Replace(s, "- [ ] yes", "- [x] yes", 1) })
	if e := s.docSyncAll(); e != nil {
		t.Fatal(e)
	}
	got, _ := s.Resolve(q.ID, false)
	if got.Decision.Action != "yes" {
		t.Fatal("a box checked in the file must decide the record", got.Decision.Action)
	}
	// Two boxes checked: the one changed last wins, and the file shows one.
	editDoc(t, q.ID, func(s string) string { return strings.Replace(s, "- [ ] no", "- [x] no", 1) })
	if e := s.docSyncAll(); e != nil {
		t.Fatal(e)
	}
	got, _ = s.Resolve(q.ID, false)
	b, _ := os.ReadFile(docFile(q.ID))
	if got.Decision.Action != "no" || strings.Contains(string(b), "- [x] yes") {
		t.Fatal(got.Decision.Action, string(b))
	}
	// A write-in outcome takes the Note text.
	editDoc(t, q.ID, func(s string) string {
		s = strings.Replace(s, "- [x] no (recommended)", "- [ ] no (recommended)", 1)
		s = strings.Replace(s, "- [ ] Write your own answer", "- [x] Write your own answer", 1)
		return s + "line one\nline two\n"
	})
	if e := s.docSyncAll(); e != nil {
		t.Fatal(e)
	}
	got, _ = s.Resolve(q.ID, false)
	if got.Decision.Action != "other" || got.Decision.Note != "line one\nline two" {
		t.Fatalf("%q %q", got.Decision.Action, got.Decision.Note)
	}
	// Unchecking the chosen box undecides; typing over the id marker or
	// deleting a line changes nothing.
	editDoc(t, q.ID, func(s string) string {
		return strings.Replace(s, "- [x] Write your own answer", "- [ ] Write your own answer", 1)
	})
	if e := s.docSyncAll(); e != nil {
		t.Fatal(e)
	}
	got, _ = s.Resolve(q.ID, false)
	if got.Decision.Action != "" {
		t.Fatal("an unchecked choice clears the decision", got.Decision.Action)
	}
	editDoc(t, q.ID, func(s string) string { return strings.ReplaceAll(s, "- [ ] yes <!-- option:yes -->\n", "") })
	if e := s.docSyncAll(); e != nil {
		t.Fatal(e)
	}
	if got, _ = s.Resolve(q.ID, false); got.Decision.Action != "" {
		t.Fatal("a deleted line is no change")
	}
	if b, _ = os.ReadFile(docFile(q.ID)); !strings.Contains(string(b), "option:yes") {
		t.Fatal("the file is rendered again from the record")
	}
}

func TestDocChecklistAndExternalChange(t *testing.T) {
	s, q := checklistFixture(t)
	if e := s.docWrite(q); e != nil {
		t.Fatal(e)
	}
	editDoc(t, q.ID, func(s string) string { return strings.Replace(s, "- [ ] feat/beta", "- [x] feat/beta", 1) })
	// An agent re-poses while the owner's edit is unsynced: both survive.
	if _, e := agentStore(s).Pose(Pose{Question: "Which branches go?", Group: "Git", Items: []Item{{ID: "a", Label: "feat/alpha", Recommended: true}, {ID: "b", Label: "feat/beta"}, {ID: "c", Label: "feat/gamma renamed"}}, Key: "c1"}); e != nil {
		t.Fatal(e)
	}
	if e := s.docSyncAll(); e != nil {
		t.Fatal(e)
	}
	got, _ := s.Resolve(q.ID, false)
	b, _ := os.ReadFile(docFile(q.ID))
	if !got.Decision.checked("b") || !strings.Contains(string(b), "feat/gamma renamed") || !strings.Contains(string(b), "- [x] feat/beta") {
		t.Fatal(got.Decision.Picks, string(b))
	}
	// A CLI decide re-renders the file.
	if _, e := s.Decide(q.ID, DecisionChange{Item: "a", Checked: ptr(true)}); e != nil {
		t.Fatal(e)
	}
	if e := s.docWrite(mustResolve(t, s, q.ID)); e != nil {
		t.Fatal(e)
	}
	if b, _ = os.ReadFile(docFile(q.ID)); !strings.Contains(string(b), "- [x] feat/alpha") {
		t.Fatal(string(b))
	}
}

func mustResolve(t *testing.T, s *Store, id string) Question {
	t.Helper()
	q, e := s.Resolve(id, false)
	if e != nil {
		t.Fatal(e)
	}
	return q
}

func TestDocAppliedIsReadOnlyAndPrune(t *testing.T) {
	s := fixture(t)
	q := poseYesNo(t, s, "done deal", "G", "yes")
	if _, e := s.Decide(q.ID, DecisionChange{Option: "yes"}); e != nil {
		t.Fatal(e)
	}
	if _, e := s.Applied(q.ID, "", "done", "shipped"); e != nil {
		t.Fatal(e)
	}
	q = mustResolve(t, s, q.ID)
	if e := s.docWrite(q); e != nil {
		t.Fatal(e)
	}
	b, _ := os.ReadFile(docFile(q.ID))
	if strings.Contains(string(b), "- [") || strings.Contains(string(b), "<!--  option") || !strings.Contains(string(b), "shipped") {
		t.Fatal("an applied decision renders without boxes:\n" + string(b))
	}
	editDoc(t, q.ID, func(s string) string { return s + "sneaky\n" })
	if e := s.docSyncAll(); e != nil {
		t.Fatal(e)
	}
	if got := mustResolve(t, s, q.ID); got.Decision.Note != "" {
		t.Fatal("edits to an applied decision are ignored")
	}
	docPrune(map[string]bool{})
	if _, e := os.Stat(docFile(q.ID)); !os.IsNotExist(e) {
		t.Fatal("pruned files go", e)
	}
}

func TestDocRowsCarryPathOnlyForTheOwner(t *testing.T) {
	s := fixture(t)
	q := poseYesNo(t, s, "path", "G", "yes")
	useDetail(t, "document")
	a, _ := mountDecisions(t, s, nil)
	rows, e := a.decisionRows(nil)
	if e != nil {
		t.Fatal(e)
	}
	var got string
	for _, r := range rows {
		if r.ID == "question/"+q.ID {
			got, _ = r.Data.(map[string]any)["path"].(string)
		}
	}
	if got != docFile(q.ID) {
		t.Fatal("the row names its working file", got)
	}
	a.store.Actor = Actor{Kind: "agent", Harness: "codex"}
	docPrune(map[string]bool{})
	rows, _ = a.decisionRows(nil)
	for _, r := range rows {
		if _, ok := r.Data.(map[string]any)["path"]; ok {
			t.Fatal("an agent listing the queue leaves no working files")
		}
	}
}
