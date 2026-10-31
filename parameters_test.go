package main

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"ofam.dev/ochazuke"
	ocli "ofam.dev/ochazuke/cli"
)

// Mount the published catalog in a fresh host runtime. The proxy executes
// only the domain handler, as mtm does through the extension CLI: standalone
// interactive handlers cannot make these checks pass for the host.
func parameterPage(t *testing.T, host bool) (*page, *Store, *int) {
	t.Helper()
	s := fixture(t)
	ask(t, s, "Question needing input?")
	var out bytes.Buffer
	a, err := newApp(s, strings.NewReader(""), &out, &out)
	if err != nil {
		t.Fatal(err)
	}
	calls := new(int)
	if !host {
		p, err := mountPage(a)
		if err != nil {
			t.Fatal(err)
		}
		p.m.Width, p.m.Height = 120, 30
		return p, s, calls
	}
	data, err := ocli.Catalog(a.r, "o-questions", version)
	if err != nil {
		t.Fatal(err)
	}
	var doc ocli.CatalogDoc
	if err = json.Unmarshal(data, &doc); err != nil {
		t.Fatal(err)
	}
	r := ochazuke.NewRuntime()
	local := func(noun string) string { return "ext-" + noun }
	mounted := ocli.MountNouns(r, doc.AppNouns(), ocli.HostMount{
		Local: local,
		Proxy: func(noun string, verb ocli.CatalogVerb) any {
			return func(c ochazuke.Ctx) ochazuke.Result {
				if verb.Name != "list" {
					*calls++
				}
				return a.handle(verb.Name, c)
			}
		},
	})
	if len(mounted.Warnings) != 0 {
		t.Fatal(mounted.Warnings)
	}
	// Hosts import the catalog's noun binding rows separately from pages.
	for _, row := range doc.Bindings {
		if row.Noun != "question" || row.Instance != "" || row.Request.Noun != "question" || row.When != "" {
			continue
		}
		row.Noun, row.Request.Noun = local(row.Noun), local(row.Request.Noun)
		if err := r.Bind(row); err != nil {
			t.Fatal(err)
		}
	}
	m := ochazuke.NewModel(r)
	v, err := ocli.MountPage(m, doc, doc.Pages[0], ocli.PageConfig{Noun: local})
	if err != nil {
		t.Fatal(err)
	}
	p := &page{a: a, m: m, v: v, spec: questionsSpec(a)}
	a.changed = func() { p.dirty = true }
	if err = r.OpenPage(pageID); err != nil {
		t.Fatal(err)
	}
	if err = p.reload(true); err != nil {
		t.Fatal(err)
	}
	if err = v.List(listID).Focus(); err != nil {
		t.Fatal(err)
	}
	m.Width, m.Height = 120, 30
	return p, s, calls
}

func typeParameter(t *testing.T, p *page, text string) {
	t.Helper()
	for _, ch := range text {
		sendKey(t, p, string(ch))
	}
}

func TestQuestionParameterStatus(t *testing.T) {
	for _, host := range []bool{false, true} {
		t.Run(map[bool]string{false: "standalone", true: "catalog-host"}[host], func(t *testing.T) {
			p, s, calls := parameterPage(t, host)
			r := p.m.Runtime
			sendKey(t, p, "s")
			if r.RecordEditor(ochazuke.ArgsFormID) == nil || (host && *calls != 0) {
				t.Fatalf("status must prompt before dispatch:\n%s", ansi.Strip(p.View().Content))
			}
			sendKey(t, p, "esc")
			q, err := s.Resolve("1", false)
			if err != nil || q.Status != "open" || r.TopModal() != "" {
				t.Fatalf("cancel changed status: %+v, %v, modal %q", q, err, r.TopModal())
			}
			sendKey(t, p, "s")
			sendKey(t, p, "enter")
			typeParameter(t, p, "answered")
			sendKey(t, p, "enter")
			sendKey(t, p, "enter")
			q, err = s.Resolve("1", false)
			if err != nil || q.Status != "answered" || r.TopModal() != "" {
				t.Fatalf("status not applied: %+v, %v:\n%s", q, err, ansi.Strip(p.View().Content))
			}
			if !strings.Contains(ansi.Strip(p.View().Content), "answered") {
				t.Fatal("row did not refresh")
			}
		})
	}
}

func TestQuestionParameterFollowUp(t *testing.T) {
	for _, host := range []bool{false, true} {
		t.Run(map[bool]string{false: "standalone", true: "catalog-host"}[host], func(t *testing.T) {
			p, s, calls := parameterPage(t, host)
			r := p.m.Runtime
			sendKey(t, p, "f")
			if r.RecordEditor(ochazuke.ArgsFormID) == nil || (host && *calls != 0) {
				t.Fatalf("follow-up must prompt before dispatch:\n%s", ansi.Strip(p.View().Content))
			}
			sendKey(t, p, "esc")
			sendKey(t, p, "esc")
			q, err := s.Resolve("1", false)
			if err != nil || len(q.Entries) != 0 || r.TopModal() != "" {
				t.Fatalf("blank cancel wrote an entry: %+v, %v", q, err)
			}
			sendKey(t, p, "f")
			text := "  What about the second case?  "
			typeParameter(t, p, text)
			sendKey(t, p, "enter")
			q, err = s.Resolve("1", false)
			if err != nil || len(q.Entries) != 1 || q.Entries[0].Kind != "follow-up" || q.Entries[0].Text != text {
				t.Fatalf("follow-up lost words: %+v, %v:\n%s", q, err, ansi.Strip(p.View().Content))
			}
			if !strings.Contains(ansi.Strip(p.View().Content), strings.TrimSpace(text)) {
				t.Fatal("follow-up missing from thread")
			}
		})
	}
}

func TestQuestionParameterBoundInputs(t *testing.T) {
	for key, name := range map[string]string{"c": "question", "a": "answer", "f": "text", "e": "text", "r": "title", "t": "tag"} {
		for _, host := range []bool{false, true} {
			t.Run(key+map[bool]string{false: "/standalone", true: "/catalog-host"}[host], func(t *testing.T) {
				p, s, calls := parameterPage(t, host)
				sendKey(t, p, key)
				r := p.m.Runtime
				if r.RecordEditor(ochazuke.ArgsFormID) == nil || r.Focus != ochazuke.ArgsFormID+"/"+name || (host && *calls != 0) {
					t.Fatalf("%s must prompt for %s:\n%s", key, name, ansi.Strip(p.View().Content))
				}
				sendKey(t, p, "esc")
				sendKey(t, p, "esc")
				q, err := s.Resolve("1", false)
				if err != nil || len(q.Entries) != 0 || len(q.Tags) != 0 || q.Text != "Question needing input?" {
					t.Fatalf("cancel mutated record: %+v, %v", q, err)
				}
			})
		}
	}
}

func TestCLIParameterProseAndInvalidStatus(t *testing.T) {
	s := fixture(t)
	t.Setenv("O_QUESTIONS_ACTOR", "owner")
	q := ask(t, s, "Question?")
	var out bytes.Buffer
	for _, verb := range []string{"answer", "follow-up", "note", "correct"} {
		out.Reset()
		if err := Main([]string{"--data", s.Dir, "question", verb, q.ID, "  first", "second  "}, strings.NewReader(""), &out, &out); err != nil {
			t.Fatalf("%s: %v", verb, err)
		}
		got, err := s.Resolve(q.ID, false)
		if err != nil || got.Entries[len(got.Entries)-1].Text != "  first second  " {
			t.Fatalf("%s lost words: %+v, %v", verb, got, err)
		}
	}
	out.Reset()
	err := Main([]string{"--data", s.Dir, "question", "status", q.ID, "bogus"}, strings.NewReader(""), &out, &out)
	if err == nil || !strings.Contains(err.Error(), "status must be one of "+strings.Join(statuses, ", ")) || strings.Contains(err.Error(), "unknown status") {
		t.Fatalf("invalid status must list choices: %v", err)
	}
}

func TestCLIParameterMissingPrompts(t *testing.T) {
	t.Skip("ochazuke v0.67.0 cli.Shell.Run calls HandleCall without a missing-argument prompt; needs a library fix, see docs/INTEGRATION.md")
	s := fixture(t)
	t.Setenv("O_QUESTIONS_ACTOR", "owner")
	q := ask(t, s, "Question?")
	for _, verb := range []string{"status", "follow-up", "answer", "edit", "rename", "tag"} {
		t.Run(verb, func(t *testing.T) {
			var out bytes.Buffer
			err := Main([]string{"--data", s.Dir, "question", verb, q.ID}, strings.NewReader(""), &out, &out)
			if err != nil {
				t.Fatalf("missing input should prompt or describe choices: %v", err)
			}
			if verb == "status" {
				for _, status := range statuses {
					if !strings.Contains(out.String(), status) {
						t.Fatalf("missing choice %s in %q", status, out.String())
					}
				}
			}
		})
	}
}
