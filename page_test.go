package main

import (
	"bytes"
	tea "charm.land/bubbletea/v2"
	"encoding/json"
	"fmt"
	"github.com/charmbracelet/x/ansi"
	"github.com/unixsuperhero/ochazuke"
	ocli "github.com/unixsuperhero/ochazuke/cli"
	"github.com/unixsuperhero/ochazuke/keytest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func deliver(t *testing.T, p *page, msg tea.Msg) {
	t.Helper()
	if batch, ok := msg.(tea.BatchMsg); ok {
		for _, cmd := range batch {
			if cmd != nil {
				deliver(t, p, cmd())
			}
		}
		return
	}
	_, cmd := p.Update(msg)
	if cmd != nil {
		deliver(t, p, cmd())
	}
}
func sendKey(t *testing.T, p *page, key string) {
	t.Helper()
	_, cmd := p.Update(keytest.Msg(key, false, false))
	if cmd != nil {
		deliver(t, p, cmd())
	}
}
func TestQuestionStories(t *testing.T) {
	files, e := filepath.Glob("cmd/o-story/stories/questions/*.json")
	if e != nil || len(files) == 0 {
		t.Fatal(files, e)
	}
	for _, file := range files {
		t.Run(filepath.Base(file), func(t *testing.T) {
			var story struct {
				Name      string
				Host      bool
				Questions []struct{ Question, Title, Answer, Note, Project, Cwd string }
				Script    []string
				Expect    []struct {
					After    int
					Contains []string
					Count    *int
					Selected string
					Modal    *bool
				}
			}
			b, e := os.ReadFile(file)
			if e != nil {
				t.Fatal(e)
			}
			if e = json.Unmarshal(b, &story); e != nil {
				t.Fatal(e)
			}
			s := fixture(t)
			for _, seed := range story.Questions {
				asked, e := s.Ask(Ask{Question: seed.Question, Title: seed.Title, Project: seed.Project, Cwd: seed.Cwd, Key: seed.Question})
				if e != nil {
					t.Fatal(e)
				}
				if seed.Answer != "" {
					if _, e = s.Append(asked.Question.ID, Entry{Kind: "answer", Text: seed.Answer}); e != nil {
						t.Fatal(e)
					}
				}
				if seed.Note != "" {
					if _, e = s.Append(asked.Question.ID, Entry{Kind: "note", Text: seed.Note}); e != nil {
						t.Fatal(e)
					}
				}
			}
			var out bytes.Buffer
			a, e := newApp(s, strings.NewReader(""), &out, &out)
			if e != nil {
				t.Fatal(e)
			}
			p, e := mountPage(a)
			if e != nil {
				t.Fatal(e)
			}
			p.m.Width, p.m.Height = 120, 30
			if story.Host {
				b, e := ocli.Catalog(a.r, "o-questions", version)
				if e != nil {
					t.Fatal(e)
				}
				var doc ocli.CatalogDoc
				if e = json.Unmarshal(b, &doc); e != nil {
					t.Fatal(e)
				}
				a, e = newApp(s, strings.NewReader(""), &out, &out)
				if e != nil {
					t.Fatal(e)
				}
				m := ochazuke.NewModel(a.r)
				v, e := ocli.MountPage(m, doc, questionPage(true), ocli.PageConfig{})
				if e != nil {
					t.Fatal(e)
				}
				rows, e := a.rows(nil)
				if e != nil {
					t.Fatal(e)
				}
				if e = v.SetRows(listID, rows); e != nil {
					t.Fatal(e)
				}
				p.m, p.v = m, v
				m.Width, m.Height = 120, 30
				_ = a.r.OpenPage(pageID)
				_ = v.List(listID).Focus()
			}
			check := func(after int) {
				t.Helper()
				frame := ansi.Strip(p.m.View().Content)
				for _, want := range story.Expect {
					if want.After != after {
						continue
					}
					for _, text := range want.Contains {
						if !strings.Contains(frame, text) {
							t.Fatalf("after %d missing %q:\n%s", after, text, frame)
						}
					}
					if want.Count != nil && p.v.List(listID).Shown() != *want.Count {
						t.Fatalf("after %d count %d want %d", after, p.v.List(listID).Shown(), *want.Count)
					}
					if want.Selected != "" {
						row, ok := p.v.List(listID).Selected()
						if !ok || row.Label != want.Selected {
							t.Fatalf("selected %+v", row)
						}
					}
					if want.Modal != nil && (a.r.TopModal() != "") != *want.Modal {
						t.Fatalf("modal %q", a.r.TopModal())
					}
				}
				if dir := os.Getenv("O_QUESTIONS_STORY_ARTIFACTS"); dir != "" {
					_ = os.MkdirAll(dir, 0700)
					if e := os.WriteFile(filepath.Join(dir, fmt.Sprintf("%s-%02d.txt", filepath.Base(file), after)), []byte(frame), 0600); e != nil {
						t.Fatal(e)
					}
				}
			}
			check(0)
			for i, key := range story.Script {
				sendKey(t, p, key)
				check(i + 1)
			}
		})
	}
}
func TestPageSmokeAndSearchBody(t *testing.T) {
	s := fixture(t)
	q := ask(t, s, "Title?")
	_, e := s.Append(q.ID, Entry{Kind: "answer", Text: "UniqueBodyNeedle"})
	if e != nil {
		t.Fatal(e)
	}
	var out bytes.Buffer
	a, e := newApp(s, strings.NewReader(""), &out, &out)
	if e != nil {
		t.Fatal(e)
	}
	if e = runTUI(a, []string{"--smoke"}); e != nil {
		t.Fatal(e)
	}
	if !strings.Contains(out.String(), "UniqueBodyNeedle") {
		t.Fatal(out.String())
	}
	a, e = newApp(s, strings.NewReader(""), &out, &out)
	if e != nil {
		t.Fatal(e)
	}
	p, e := mountPage(a)
	if e != nil {
		t.Fatal(e)
	}
	p.v.List(listID).SetFilter("UniqueBodyNeedle")
	if p.v.List(listID).Shown() != 1 {
		t.Fatal("body not searchable")
	}
	p.v.List(listID).SetFilter("missing")
	if p.v.List(listID).Shown() != 0 {
		t.Fatal("filter not applied")
	}
}
func TestPageStatusAndEditor(t *testing.T) {
	s := fixture(t)
	ask(t, s, "Question?")
	var out bytes.Buffer
	a, e := newApp(s, strings.NewReader(""), &out, &out)
	if e != nil {
		t.Fatal(e)
	}
	p, e := mountPage(a)
	if e != nil {
		t.Fatal(e)
	}
	p.m.Width, p.m.Height = 100, 24
	sendKey(t, p, "s")
	if a.r.Chooser("question-status") == nil {
		t.Fatalf("s did not open chooser: %s", ansi.Strip(p.m.View().Content))
	}
	sendKey(t, p, "m")
	sendKey(t, p, "o")
	sendKey(t, p, "o")
	sendKey(t, p, "t")
	sendKey(t, p, "enter")
	q, e := s.Resolve("1", false)
	if e != nil || q.Status != "moot" {
		t.Fatal(q, e)
	}
	sendKey(t, p, "c")
	if a.r.RecordEditor("question-editor") == nil {
		t.Fatalf("c did not open immediate editor: %s", ansi.Strip(p.m.View().Content))
	}
	for _, r := range "New question?" {
		sendKey(t, p, string(r))
	}
	rows, e := s.List()
	if e != nil || len(rows) != 2 || rows[1].Text != "New question?" {
		t.Fatal(rows, e)
	}
	sendKey(t, p, "esc")
	sendKey(t, p, "esc")
	if a.r.TopModal() != "" {
		t.Fatal(a.r.TopModal())
	}
}
func TestCatalogRowsAndSettings(t *testing.T) {
	s := fixture(t)
	ask(t, s, "Question?")
	var out bytes.Buffer
	a, e := newApp(s, strings.NewReader(""), &out, &out)
	if e != nil {
		t.Fatal(e)
	}
	b, e := ocli.Catalog(a.r, "o-questions", version)
	if e != nil {
		t.Fatal(e)
	}
	var doc ocli.CatalogDoc
	if e = json.Unmarshal(b, &doc); e != nil {
		t.Fatal(e)
	}
	found := false
	for _, p := range doc.Pages {
		if p.ID == pageID {
			found = true
			if p.Detail.Kind != "" {
				t.Fatal("custom detail leaked into host", p.Detail)
			}
		}
	}
	if !found {
		t.Fatal("missing catalog page")
	}
	if e = a.r.Settings().Set("detail.answer-lines", 0); e != nil {
		t.Fatal(e)
	}
	if e = a.r.Settings().Set("detail.provenance", false); e != nil {
		t.Fatal(e)
	}
	if e = a.r.Settings().Set("filter.project", "absent"); e != nil {
		t.Fatal(e)
	}
	rows, e := a.rows(nil)
	if e != nil || len(rows) != 0 {
		t.Fatal(rows, e)
	}
}

func TestPageEmptyCreateAndOptOut(t *testing.T) {
	s := fixture(t)
	var out bytes.Buffer
	a, e := newApp(s, strings.NewReader(""), &out, &out)
	if e != nil {
		t.Fatal(e)
	}
	if e = a.r.Settings().Set("view.thread", false); e != nil {
		t.Fatal(e)
	}
	p, e := mountPage(a)
	if e != nil {
		t.Fatal(e)
	}
	p.m.Width, p.m.Height = 100, 24
	sendKey(t, p, "c")
	if a.r.RecordEditor("question-editor") == nil {
		t.Fatalf("empty-page c failed: %s", ansi.Strip(p.m.View().Content))
	}
	sendKey(t, p, "?")
	sendKey(t, p, "esc")
	sendKey(t, p, "esc")
	rows, e := s.List()
	if e != nil || len(rows) != 1 || rows[0].Text != "?" {
		t.Fatal(rows, e)
	}
	sendKey(t, p, "v")
	if a.r.TopModal() != "" {
		t.Fatal("thread opt-out ignored")
	}
}
