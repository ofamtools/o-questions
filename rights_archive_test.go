package main

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"ofam.dev/ochazuke"
	ocli "ofam.dev/ochazuke/cli"
)

func TestAgentArchiveRestore(t *testing.T) {
	s := fixture(t)
	q := ask(t, s, "Retain these exact words?")
	other := ask(t, s, "Visible question?")
	actor := Actor{Kind: "agent", Name: "writer", Harness: "test", Pane: "pane", Session: "session", Model: "model"}
	s.Actor = actor
	var out, errs bytes.Buffer
	a, e := newApp(s, strings.NewReader(""), &out, &errs)
	if e != nil {
		t.Fatal(e)
	}
	call := func(args ...string) {
		t.Helper()
		out.Reset()
		if e := dispatch(a, args); e != nil {
			t.Fatalf("%v: %v; %s", args, e, errs.String())
		}
	}
	if e := dispatch(a, []string{"question", "delete", q.ID}); e == nil || !strings.Contains(e.Error(), "verb declaration") {
		t.Fatalf("agent delete: %v", e)
	}
	for range 2 {
		call("question", "archive", q.ID)
	}
	rows, e := s.List()
	if e != nil || len(rows) != 1 || rows[0].ID != other.ID {
		t.Fatal(rows, e)
	}
	if numbered, e := s.Resolve("1", false); e != nil || numbered.ID != other.ID {
		t.Fatal(numbered, e)
	}
	archived, e := s.Resolve(q.ID, false)
	if e != nil || !archived.Deleted || archived.Text != q.Text {
		t.Fatal(archived, e)
	}
	if _, e := a.r.Settings().SetSession("list.hide-archived", false); e != nil {
		t.Fatal(e)
	}
	call("question", "list")
	var listed []ocli.Row
	if e := json.Unmarshal(out.Bytes(), &listed); e != nil || len(listed) != 2 {
		t.Fatal(out.String(), e)
	}
	if listed[0].ID != "question/"+q.ID || listed[0].Cells[0] != "-" || listed[1].ID != "question/"+other.ID || listed[1].Cells[0] != "1" {
		t.Fatal(listed)
	}
	for range 2 {
		call("question", "restore", "--target", q.ID)
	}
	rows, e = s.List()
	if e != nil || len(rows) != 2 || rows[0].ID != q.ID || rows[0].Text != q.Text || rows[0].Deleted {
		t.Fatal(rows, e)
	}
	history, e := s.History()
	if e != nil {
		t.Fatal(e)
	}
	for i, want := range []string{"archive", "archive", "restore", "restore"} {
		snapshot := history[i+2]
		if snapshot.Action != want || snapshot.By != actor {
			t.Fatalf("lost attribution: %+v", snapshot)
		}
	}
	call("question", "archive", q.ID)
	call("question", "undo")
	rows, e = s.List()
	if e != nil || len(rows) != 2 || rows[0].ID != q.ID {
		t.Fatal(rows, e)
	}
}

func TestQuestionRightsCatalog(t *testing.T) {
	s := fixture(t)
	var out bytes.Buffer
	a, e := newApp(s, strings.NewReader(""), &out, &out)
	if e != nil {
		t.Fatal(e)
	}
	if e = dispatch(a, []string{"catalog"}); e != nil {
		t.Fatal(e)
	}
	var catalog ocli.CatalogDoc
	if e = json.Unmarshal(out.Bytes(), &catalog); e != nil {
		t.Fatal(e)
	}
	found := map[string]bool{}
	wants := map[string]string{"delete": ochazuke.AgentNever, "archive": ochazuke.AgentAllow, "restore": ochazuke.AgentAllow, "edit": ochazuke.AgentAllow, "decide": ochazuke.AgentAllow}
	for _, noun := range catalog.Nouns {
		if noun.Name != "question" && noun.Name != "review" {
			continue
		}
		for _, verb := range noun.Verbs {
			want, ok := wants[verb.Name]
			if !ok {
				continue
			}
			if verb.Agent != want || verb.Destructive {
				t.Fatalf("%s.%s: %+v", noun.Name, verb.Name, verb)
			}
			found[noun.Name+"."+verb.Name] = true
			for _, alias := range verb.Aliases {
				found[alias.Alias] = true
			}
		}
	}
	for _, key := range []string{"question.delete", "question.archive", "question.restore", "question.edit", "question.decide", "review.delete", "review.archive", "review.restore"} {
		if !found[key] {
			t.Fatalf("missing catalog row %s", key)
		}
	}
}

func TestRestoreBackupCLI(t *testing.T) {
	s := fixture(t)
	q := ask(t, s, "Backup wording?")
	s.Actor = Actor{Kind: "agent", Name: "restorer", Harness: "test"}
	var out bytes.Buffer
	a, e := newApp(s, strings.NewReader(""), &out, &out)
	if e != nil {
		t.Fatal(e)
	}
	if e = dispatch(a, []string{"question", "backup", "--to", t.TempDir()}); e != nil {
		t.Fatal(e)
	}
	var file string
	if e = json.Unmarshal(out.Bytes(), &file); e != nil {
		t.Fatal(out.String(), e)
	}
	if _, e = s.Change(opEdit, q.ID, Change{Value: "Changed wording?"}); e != nil {
		t.Fatal(e)
	}
	out.Reset()
	if e = dispatch(a, []string{"question", "restore", file}); e != nil {
		t.Fatal(e)
	}
	restored, e := s.Resolve(q.ID, false)
	if e != nil || restored.Text != "Backup wording?" {
		t.Fatal(restored, e)
	}
	history, e := s.History()
	if e != nil || history[len(history)-1].By != s.Actor {
		t.Fatal(history, e)
	}
}
