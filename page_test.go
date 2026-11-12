package main

import (
	"bytes"
	tea "charm.land/bubbletea/v2"
	"encoding/json"
	"fmt"
	"github.com/charmbracelet/x/ansi"
	"ofam.dev/ochazuke"
	ocli "ofam.dev/ochazuke/cli"
	"ofam.dev/ochazuke/keytest"
	"ofam.dev/ochazuke/markdown/cliview"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf8"
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

func TestThreadViewerSharedMotions(t *testing.T) {
	for _, height := range []int{24, 40} {
		t.Run(fmt.Sprint(height), func(t *testing.T) {
			s := fixture(t)
			if _, e := s.Ask(Ask{Question: strings.Repeat("thread line\n", 100), Title: "Long thread"}); e != nil {
				t.Fatal(e)
			}
			var out bytes.Buffer
			a, e := newApp(s, strings.NewReader(""), &out, &out)
			if e != nil {
				t.Fatal(e)
			}
			p, e := mountSpec(a, questionsSpec(a))
			if e != nil {
				t.Fatal(e)
			}
			p.m.Width, p.m.Height = 100, height
			_ = p.m.View()
			sendKey(t, p, "v")
			_ = p.m.View()
			d := a.r.Detail(threadViewerTextID)
			if d == nil {
				t.Fatal("v did not open the thread")
			}
			lines := d.PageLines()
			if lines <= 0 {
				t.Fatal("viewer has no page")
			}
			sendKey(t, p, "ctrl+d")
			if got := d.Offset(); got != max(1, lines/2) {
				t.Fatalf("half page offset = %d, page = %d", got, lines)
			}
			sendKey(t, p, "G")
			if d.Offset() == 0 {
				t.Fatal("G did not reach the end")
			}
			sendKey(t, p, "g")
			sendKey(t, p, "g")
			if got := d.Offset(); got != 0 {
				t.Fatalf("gg offset = %d", got)
			}
			sendKey(t, p, "ctrl+f")
			if got := d.Offset(); got != lines {
				t.Fatalf("full page offset = %d, page = %d", got, lines)
			}
			sendKey(t, p, "ctrl+b")
			if got := d.Offset(); got != 0 {
				t.Fatalf("page back offset = %d", got)
			}
			sendKey(t, p, "q")
			if a.r.TopModal() != "" {
				t.Fatal("q did not close the thread")
			}
		})
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
				Skip      string
				Host      bool
				Width     int
				Height    int
				Settings  map[string]any
				Questions []struct{ Question, Title, Answer, Note, Project, Cwd string }
				Page      string
				Poses     []Pose
				Script    []string
				Expect    []struct {
					After      int
					Contains   []string
					Absent     []string
					Count      *int
					Selected   string
					Modal      *bool
					Side       string
					FullAnswer bool
				}
			}
			b, e := os.ReadFile(file)
			if e != nil {
				t.Fatal(e)
			}
			if e = json.Unmarshal(b, &story); e != nil {
				t.Fatal(e)
			}
			if story.Skip != "" {
				t.Skip(story.Skip)
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
			for _, pose := range story.Poses {
				if _, e := agentStore(s).Pose(pose); e != nil {
					t.Fatal(e)
				}
			}
			if style, ok := story.Settings["decision.detail"].(string); ok {
				useDetail(t, style)
			}
			var out bytes.Buffer
			a, e := newApp(s, strings.NewReader(""), &out, &out)
			if e != nil {
				t.Fatal(e)
			}
			for name, value := range story.Settings {
				if e = a.r.Settings().Set(name, value); e != nil {
					t.Fatal(e)
				}
			}
			spec := questionsSpec
			if story.Page == decisionPageID {
				spec = decisionSpec
			}
			p, e := mountSpec(a, spec(a))
			if e != nil {
				t.Fatal(e)
			}
			p.m.Width, p.m.Height = 120, 30
			if story.Host {
				_ = cliview.Register()
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
				for name, value := range story.Settings {
					if e = a.r.Settings().Set(name, value); e != nil {
						t.Fatal(e)
					}
				}
				if len(doc.Pages) != 2 || doc.Pages[0].ID != pageID || doc.Pages[1].ID != decisionPageID {
					t.Fatalf("expected the Questions and Decisions pages, got %+v", doc.Pages)
				}
				hs := spec(a)
				hosted := doc.Pages[0]
				if hs.page.ID == decisionPageID {
					hosted = doc.Pages[1]
				}
				v, e := ocli.MountPage(m, doc, hosted, ocli.PageConfig{})
				if e != nil {
					t.Fatal(e)
				}
				rows, e := hs.rows()
				if e != nil {
					t.Fatal(e)
				}
				if e = v.SetRows(hs.list, rows); e != nil {
					t.Fatal(e)
				}
				p.m, p.v, p.spec = m, v, hs
				p.a = a
				a.store.Actor = Owner()
				a.changed = func() { p.dirty = true }
				m.Width, m.Height = 120, 30
				_ = a.r.OpenPage(hs.page.ID)
				_ = v.List(hs.list).Focus()
				if hs.first != nil {
					_ = v.List(hs.list).Select(hs.list + "/" + hs.first(rows))
				}
			}
			if story.Width > 0 {
				p.m.Width = story.Width
			}
			if story.Height > 0 {
				p.m.Height = story.Height
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
					for _, text := range want.Absent {
						if strings.Contains(frame, text) {
							t.Fatalf("after %d unexpected %q:\n%s", after, text, frame)
						}
					}
					if want.Count != nil && p.v.List(p.spec.list).Shown() != *want.Count {
						t.Fatalf("after %d count %d want %d", after, p.v.List(p.spec.list).Shown(), *want.Count)
					}
					if want.Selected != "" {
						row, ok := p.v.List(p.spec.list).Selected()
						if !ok || strings.TrimSpace(row.Label) != want.Selected {
							t.Fatalf("selected %+v", row)
						}
					}
					if want.Modal != nil && (a.r.TopModal() != "") != *want.Modal {
						t.Fatalf("modal %q", a.r.TopModal())
					}
					if want.FullAnswer {
						for _, seed := range story.Questions {
							firstLine, lastLine := -1, -1
							for _, word := range strings.Fields(seed.Answer) {
								if !strings.Contains(frame, word) {
									t.Fatalf("answer lost %q:\n%s", word, frame)
								}
								line := strings.Count(frame[:strings.Index(frame, word)], "\n")
								if firstLine < 0 {
									firstLine = line
								}
								lastLine = line
							}
							if lastLine <= firstLine {
								t.Fatalf("answer did not wrap:\n%s", frame)
							}
						}
					}
					if want.Side != "" {
						// column is the cell column s starts at in line (-1 when
						// absent): box borders are multi-byte.
						column := func(line, s string) int {
							i := strings.Index(line, s)
							if i < 0 {
								return -1
							}
							return utf8.RuneCountInString(line[:i])
						}
						metadata, question, metaCol, questionCol := -1, -1, -1, -1
						for i, line := range strings.Split(frame, "\n") {
							if c := column(line, "Metadata"); c >= 0 {
								metadata, metaCol = i, c
							}
							if c := column(line, story.Questions[0].Question); c >= 0 && question < 0 {
								question, questionCol = i, c
							}
						}
						// tailLine is the row of the answer's last word (the
						// question's row when there is no answer).
						tail, tailLine := 0, question
						if words := strings.Fields(story.Questions[0].Answer); len(words) > 0 {
							tail = strings.Index(frame, words[len(words)-1])
							tailLine = strings.Count(frame[:max(tail, 0)], "\n")
						}
						if metadata < 0 || question < 0 || tail < 0 {
							t.Fatalf("missing side layout %s:\n%s", want.Side, frame)
						}
						// Beside: the metadata column starts at or above the
						// question's row (the detail's padding may put a blank
						// row over the question, detail.padding) and to its
						// right.
						if want.Side == "right" && (metadata > question || metaCol <= questionCol) {
							t.Fatalf("metadata is not beside question:\n%s", frame)
						}
						if want.Side == "below" && metadata <= tailLine {
							t.Fatalf("metadata is not below complete answer:\n%s", frame)
						}
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
	if a.r.RecordEditor(ochazuke.ArgsFormID) == nil {
		t.Fatalf("s did not open args form: %s", ansi.Strip(p.m.View().Content))
	}
	sendKey(t, p, "enter")
	sendKey(t, p, "m")
	sendKey(t, p, "o")
	sendKey(t, p, "o")
	sendKey(t, p, "t")
	sendKey(t, p, "enter")
	sendKey(t, p, "enter")
	q, e := s.Resolve("1", false)
	if e != nil || q.Status != "moot" {
		t.Fatal(q, e)
	}
	sendKey(t, p, "c")
	if a.r.RecordEditor(ochazuke.ArgsFormID) == nil {
		t.Fatalf("c did not open args form: %s", ansi.Strip(p.m.View().Content))
	}
	for _, r := range "New question?" {
		sendKey(t, p, string(r))
	}
	sendKey(t, p, "enter")
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
			if len(p.Detail.Sections) != 3 || p.Detail.Sections[1].Field != "entries" || p.Detail.Sections[1].Body != "text" || p.Detail.Sections[2].Region != "side" {
				t.Fatal("missing portable question sections", p.Detail)
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
	if a.r.RecordEditor(ochazuke.ArgsFormID) == nil {
		t.Fatalf("empty-page c failed: %s", ansi.Strip(p.m.View().Content))
	}
	sendKey(t, p, "?")
	sendKey(t, p, "enter")
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

func TestThreadViewCLI(t *testing.T) {
	for _, noun := range []string{"question", "decision"} {
		t.Run(noun, func(t *testing.T) {
			s := fixture(t)
			q := ask(t, s, "Which path keeps the complete answer?")
			if _, err := s.Append(q.ID, Entry{Kind: "answer", Text: "Use the readable thread.\nKeep both lines."}); err != nil {
				t.Fatal(err)
			}
			var out, errOut bytes.Buffer
			a, err := newApp(s, strings.NewReader(""), &out, &errOut)
			if err != nil {
				t.Fatal(err)
			}
			if err := dispatch(a, []string{noun, "show", "--target", q.ID, "--json", "{}"}); err != nil {
				t.Fatalf("%v: %s", err, &errOut)
			}
			var text string
			if err := json.Unmarshal(out.Bytes(), &text); err != nil {
				t.Fatal(err, out.String())
			}
			if !strings.Contains(text, "## answer\n\nUse the readable thread.\nKeep both lines.") {
				t.Fatal(text)
			}
		})
	}
}

func TestCatalogDeclaresSingularNamespace(t *testing.T) {
	s := fixture(t)
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
	if doc.Namespace != "question" || ochazuke.DefaultNamespace != "question" {
		t.Fatalf("catalog namespace %q, default %q; want question", doc.Namespace, ochazuke.DefaultNamespace)
	}
	if got, e := ocli.CatalogNamespace(doc, "o-questions"); e != nil || got != "question" {
		t.Fatal(got, e)
	}
	if op, ok := a.r.Operations.Find("question.ask"); !ok || op.Key.Full() != "question.question.ask" {
		t.Fatal(op.Key, ok)
	}
}

func TestCatalogTypesQuestionReferences(t *testing.T) {
	var out bytes.Buffer
	a, e := newApp(fixture(t), strings.NewReader(""), &out, &out)
	if e != nil {
		t.Fatal(e)
	}
	b, e := ocli.Catalog(a.r, "o-questions", version)
	if e != nil {
		t.Fatal(e)
	}
	var doc struct {
		Nouns []struct {
			Name  string
			Props []struct{ Name, Noun, HostNoun string }
		}
	}
	if e = json.Unmarshal(b, &doc); e != nil {
		t.Fatal(e)
	}
	want := map[string][2]string{"pane": {"", "pane"}, "project_id": {"", ""}, "agent_session": {"", ""}}
	seen := 0
	for _, n := range doc.Nouns {
		if n.Name != "question" && n.Name != "decision" {
			continue
		}
		seen++
		for _, p := range n.Props {
			if w, ok := want[p.Name]; ok && [2]string{p.Noun, p.HostNoun} != w {
				t.Errorf("%s.%s declares noun/hostNoun %v, want %v", n.Name, p.Name, [2]string{p.Noun, p.HostNoun}, w)
			}
		}
	}
	if seen != 2 {
		t.Fatalf("found %d of question and decision", seen)
	}
}
