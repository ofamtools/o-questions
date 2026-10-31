package main

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func fixture(t *testing.T) *Store {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(dir, "config"))
	t.Setenv("XDG_STATE_HOME", filepath.Join(dir, "state"))
	t.Setenv("XDG_DATA_HOME", filepath.Join(dir, "data"))
	return &Store{Dir: filepath.Join(dir, "questions"), Actor: Owner(), Now: func() time.Time { return time.Date(2026, 10, 31, 12, 0, 0, 0, time.UTC) }}
}
func ask(t *testing.T, s *Store, text string) Question {
	t.Helper()
	v, e := s.Ask(Ask{Question: text, Key: text})
	if e != nil {
		t.Fatal(e)
	}
	return v.Question
}
func TestStoreThreadUndo(t *testing.T) {
	s := fixture(t)
	q := ask(t, s, " how?\n exactly ")
	if q.Text != " how?\n exactly " || q.Project.ID != "" {
		t.Fatal(q)
	}
	q, e := s.Append(q.ID, Entry{Kind: "answer", Text: "An answer"})
	if e != nil || q.Status != "answered" {
		t.Fatal(q, e)
	}
	q, e = s.Append(q.ID, Entry{Kind: "follow-up", Text: "Why?"})
	if e != nil || q.Status != "follow-up" {
		t.Fatal(q, e)
	}
	_, e = s.Change("moot", q.ID, Change{})
	if e != nil {
		t.Fatal(e)
	}
	q, e = s.Append(q.ID, Entry{Kind: "answer", Text: "Another answer"})
	if e != nil || q.Status != "moot" {
		t.Fatal(q, e)
	}
	if e = s.Undo(); e != nil {
		t.Fatal(e)
	}
	q, e = s.Resolve(q.ID, false)
	if e != nil || len(q.Entries) != 2 {
		t.Fatal(q, e)
	}
	info, e := os.Stat(s.path(q))
	if e != nil || info.Mode().Perm() != 0600 {
		t.Fatal(info, e)
	}
}
func TestStoreRights(t *testing.T) {
	s := fixture(t)
	q := ask(t, s, "Question?")
	_, e := s.Change("rename", q.ID, Change{Annotation: Annotation{Title: "Owner title"}})
	if e != nil {
		t.Fatal(e)
	}
	s.Actor = Actor{Kind: "agent", Harness: "test"}
	for _, action := range []string{"moot", "delete", "undo", "status", "edit", "merge", "split", "star", "untag"} {
		var e error
		if action == "undo" {
			e = s.Undo()
		} else {
			_, e = s.Change(action, q.ID, Change{})
		}
		var refused *RefusedError
		if !errors.As(e, &refused) {
			t.Fatalf("%s: %v", action, e)
		}
	}
	if _, e = s.Annotate(q.ID, Annotation{Title: "overwrite"}); e == nil {
		t.Fatal("agent renamed owner's title")
	}
	if _, e = s.Annotate(q.ID, Annotation{Project: "p", Tags: []string{"tag"}}); e != nil {
		t.Fatal(e)
	}
	if _, e = s.Annotate(q.ID, Annotation{Project: "other"}); e == nil {
		t.Fatal("agent reassociated")
	}
	if _, e = s.Append(q.ID, Entry{Kind: "answer", Text: "Agent answer"}); e != nil {
		t.Fatal(e)
	}
	q, e = s.Resolve(q.ID, false)
	if e != nil || q.Text != "Question?" || q.Entries[0].By.Harness != "test" {
		t.Fatal(q, e)
	}
}
func TestStoreIdempotencyConcurrency(t *testing.T) {
	s := fixture(t)
	var wg sync.WaitGroup
	errs := make(chan error, 12)
	for i := 0; i < 12; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); _, e := s.Ask(Ask{Question: "Same?", Key: "retry"}); errs <- e }()
	}
	wg.Wait()
	close(errs)
	for e := range errs {
		if e != nil {
			t.Fatal(e)
		}
	}
	rows, e := s.List()
	if e != nil || len(rows) != 1 {
		t.Fatal(rows, e)
	}
	a, e := s.Ask(Ask{Question: "Next?", FollowUpOf: rows[0].ID, Key: "follow"})
	if e != nil {
		t.Fatal(e)
	}
	b, e := s.Ask(Ask{Question: "Next?", FollowUpOf: rows[0].ID, Key: "follow"})
	if e != nil || !b.Existed || a.Question.ID != b.Question.ID || len(b.Question.Entries) != 1 {
		t.Fatal(b, e)
	}
}
func TestStoreLargeEntryBackupRecovery(t *testing.T) {
	s := fixture(t)
	s.MaxBytes = 10
	s.BackupDaily = true
	s.BackupKeep = 2
	q := ask(t, s, "Question?")
	q, e := s.Append(q.ID, Entry{Kind: "answer", Text: strings.Repeat("long answer ", 20)})
	if e != nil || q.Entries[0].Text == "" || q.Entries[0].File == "" {
		t.Fatal(q, e)
	}
	backup, e := s.Backup("")
	if e != nil {
		t.Fatal(e)
	}
	_, e = s.Change("delete", q.ID, Change{})
	if e != nil {
		t.Fatal(e)
	}
	if e = s.Restore(backup); e != nil {
		t.Fatal(e)
	}
	q, e = s.Resolve(q.ID, false)
	if e != nil || len(q.Entries[0].Text) != 240 {
		t.Fatal(q, e)
	}
	rows, e := s.all()
	if e != nil {
		t.Fatal(e)
	}
	rows[0].Title = "Recovered"
	if e = atomicJSON(filepath.Join(s.Dir, ".transaction.json"), rows); e != nil {
		t.Fatal(e)
	}
	q, e = s.Resolve(q.ID, false)
	if e != nil || q.Title != "Recovered" {
		t.Fatal(q, e)
	}
	if _, e = os.Stat(filepath.Join(s.Dir, ".transaction.json")); !errors.Is(e, os.ErrNotExist) {
		t.Fatal(e)
	}
}
func TestStoreResolveMergeSplit(t *testing.T) {
	s := fixture(t)
	q := ask(t, s, "First?")
	r := ask(t, s, "Second?")
	q, e := s.Append(q.ID, Entry{Kind: "follow-up", Text: "Different topic?"})
	if e != nil {
		t.Fatal(e)
	}
	_, e = s.Change("split", q.ID, Change{Entry: q.Entries[0].ID})
	if e != nil {
		t.Fatal(e)
	}
	rows, e := s.List()
	if e != nil || len(rows) != 3 {
		t.Fatal(rows, e)
	}
	_, e = s.Change("merge", q.ID, Change{Other: r.ID})
	if e != nil {
		t.Fatal(e)
	}
	rows, e = s.List()
	if e != nil || len(rows) != 2 {
		t.Fatal(rows, e)
	}
	if e = s.Undo(); e != nil {
		t.Fatal(e)
	}
	rows, _ = s.List()
	if len(rows) != 3 {
		t.Fatal(rows)
	}
	q, e = s.Resolve(q.ID[:8], false)
	if e != nil || q.Text != "First?" {
		t.Fatal(q, e)
	}
	if _, e = s.Resolve("q-", false); e == nil {
		t.Fatal("ambiguous prefix succeeded")
	}
}
func TestStoreCorruptRecordVisible(t *testing.T) {
	s := fixture(t)
	q := ask(t, s, "Question?")
	if e := os.WriteFile(s.path(q), []byte("bad json"), 0600); e != nil {
		t.Fatal(e)
	}
	if _, e := s.List(); e == nil {
		t.Fatal("silently ignored corruption")
	}
}
func TestActor(t *testing.T) {
	env := map[string]string{"CODEX_THREAD_ID": "test", "TUIOS_PANE_ID": "pane"}
	a, e := cliActor(func(k string) string { return env[k] })
	if e != nil || a.Kind != "agent" || a.Harness != "codex" {
		t.Fatal(a, e)
	}
	env["O_QUESTIONS_ACTOR"] = "invalid"
	if _, e = cliActor(func(k string) string { return env[k] }); e == nil {
		t.Fatal("invalid actor allowed")
	}
}
func TestCLIQuestion(t *testing.T) {
	s := fixture(t)
	t.Setenv("O_QUESTIONS_ACTOR", "owner")
	var out strings.Builder
	run := func(args ...string) string {
		t.Helper()
		out.Reset()
		args = append([]string{"--data", s.Dir}, args...)
		if e := Main(args, strings.NewReader(""), &out, &out); e != nil {
			t.Fatalf("%v: %v\n%s", args, e, out.String())
		}
		return out.String()
	}
	text := "  Verbatim?  "
	got := run("ask", text, "--key", "cli")
	var rec Asked
	if e := json.Unmarshal([]byte(got), &rec); e != nil || rec.Question.Text != text {
		t.Fatal(got, e)
	}
	run("question", "answer", rec.Question.ID, "Answer")
	got = run("show", rec.Question.ID)
	if !strings.Contains(got, "Answer") {
		t.Fatal(got)
	}
	run("undo")
	run("list")
	t.Setenv("O_QUESTIONS_ACTOR", "agent")
	if e := Main([]string{"--data", s.Dir, "moot", rec.Question.ID}, strings.NewReader(""), &out, &out); e == nil {
		t.Fatal("agent CLI moot succeeded")
	}
}
