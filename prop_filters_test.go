package main

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"ofam.dev/ochazuke"
	ocli "ofam.dev/ochazuke/cli"
)

func TestQuestionPropFilters(t *testing.T) {
	for _, host := range []bool{false, true} {
		t.Run(map[bool]string{false: "standalone", true: "catalog-host"}[host], func(t *testing.T) {
			p, s, _ := parameterPage(t, host)
			s.Actor = Actor{Kind: "owner", Harness: "codex"}
			q, err := s.Ask(Ask{Question: "Other project?", Project: "beta", Source: Source{Pane: "pane-1", AgentSession: "session-1"}})
			if err != nil {
				t.Fatal(err)
			}
			if _, err = s.Append(q.Question.ID, Entry{Kind: "answer", Text: "Answered"}); err != nil {
				t.Fatal(err)
			}
			if _, err = agentStore(s).Pose(Pose{Question: "Owner input?", Project: "beta", Key: "owner-input"}); err != nil {
				t.Fatal(err)
			}
			if err = p.a.r.Settings().Set("filter.direction", "all"); err != nil {
				t.Fatal(err)
			}
			if err = p.reload(false); err != nil {
				t.Fatal(err)
			}
			l := p.v.List(listID)
			if l.Shown() != 3 {
				t.Fatalf("initial rows: %d", l.Shown())
			}
			for _, test := range []struct {
				prop, op string
				value    any
				want     int
			}{{"status", "=", "answered", 1}, {"project", "=", "beta", 2}, {"project_id", "=", "beta", 2}, {"project", "!=", "beta", 1}, {"agent", "=", "codex", 1}, {"direction", "=", toAgent, 2}, {"direction", "=", toOwner, 1}, {"pane", "=", "", 2}, {"pane", "=", "pane-1", 1}, {"agent_session", "=", "session-1", 1}} {
				if err = l.SetPropFilters([]ochazuke.PropFilter{{Prop: test.prop, Op: test.op, Value: test.value}}); err != nil {
					t.Fatal(err)
				}
				if l.Shown() != test.want {
					t.Errorf("%s %s %v: shown %d, want %d", test.prop, test.op, test.value, l.Shown(), test.want)
				}
			}
			if err = l.SetPropFilters(nil); err != nil {
				t.Fatal(err)
			}
			sendKey(t, p, "z")
			sendKey(t, p, "/")
			if p.m.Runtime.RecordEditor("list-filter-form") == nil {
				t.Fatal("z / did not open filter form")
			}
			// The enum chips write live, through the same form used in mtm.
			if err = p.m.Runtime.SetFocus("list-filter-form/direction"); err != nil {
				t.Fatal(err)
			}
			sendKey(t, p, "l")
			sendKey(t, p, "space")
			if l.Shown() != 1 || len(l.PropFilters()) != 1 || l.PropFilters()[0].Value != toOwner {
				t.Fatalf("direction form did not narrow: %v, shown %d", l.PropFilters(), l.Shown())
			}
			sendKey(t, p, "esc")
			if err = p.reload(false); err != nil || l.Shown() != 1 {
				t.Fatalf("refresh lost filter: %v, shown %d", err, l.Shown())
			}
			var got Question
			rows, err := p.a.rows(ochazuke.Arguments{"project": "beta", "status": "answered"})
			if err != nil || len(rows) != 1 {
				t.Fatalf("list parameters changed: %v %v", rows, err)
			}
			b, _ := json.Marshal(rows[0].Data)
			if err = json.Unmarshal(b, &got); err != nil || got.Project.ID != "beta" {
				t.Fatalf("project detail record changed: %+v %v", got, err)
			}
		})
	}
}

func TestDecisionPropFilters(t *testing.T) {
	for _, host := range []bool{false, true} {
		t.Run(map[bool]string{false: "standalone", true: "catalog-host"}[host], func(t *testing.T) {
			p, s, _ := parameterPage(t, host)
			for _, project := range []string{"alpha", "beta"} {
				if _, err := agentStore(s).Pose(Pose{Question: "Ship " + project + "?", Project: project, Group: "Release", Key: project}); err != nil {
					t.Fatal(err)
				}
			}
			b, err := ocli.Catalog(p.a.r, "o-questions", version)
			if err != nil {
				t.Fatal(err)
			}
			var doc ocli.CatalogDoc
			if err = json.Unmarshal(b, &doc); err != nil {
				t.Fatal(err)
			}
			cfg := ocli.PageConfig{}
			if host {
				cfg.Noun = func(noun string) string { return "ext-" + noun }
			}
			p.v, err = ocli.MountPage(p.m, doc, decisionPage(false), cfg)
			if err != nil {
				t.Fatal(err)
			}
			p.spec = decisionSpec(p.a)
			if err = p.reload(false); err != nil {
				t.Fatal(err)
			}
			if err = p.m.Runtime.OpenPage(decisionPageID); err != nil {
				t.Fatal(err)
			}
			l := p.v.List(decisionListID)
			if err = l.Focus(); err != nil {
				t.Fatal(err)
			}
			if l.Shown() != 3 {
				t.Fatalf("initial tree rows: %d", l.Shown())
			}
			if err = l.SetPropFilters([]ochazuke.PropFilter{{Prop: "project", Value: "beta"}}); err != nil {
				t.Fatal(err)
			}
			if l.Shown() != 2 {
				t.Fatalf("project filter must keep matching question and topic: %d", l.Shown())
			}
			frame := ansi.Strip(p.m.View().Content)
			if !strings.Contains(frame, "Ship beta?") {
				t.Fatal(frame)
			}
			for _, row := range l.Snapshot()["items"].([]ochazuke.ListItem) {
				if row.Label == "Ship alpha?" {
					t.Fatal("nonmatching Decision question is visible")
				}
			}
			if err = l.SetPropFilters(nil); err != nil {
				t.Fatal(err)
			}
			sendKey(t, p, "z")
			sendKey(t, p, "/")
			if err = p.m.Runtime.SetFocus("list-filter-form/project_id"); err != nil {
				t.Fatal(err)
			}
			for _, key := range []string{"i", "b", "e", "t", "a", "esc", "esc"} {
				sendKey(t, p, key)
			}
			if l.Shown() != 2 {
				t.Fatalf("project form did not narrow Decision: %d %v", l.Shown(), l.PropFilters())
			}
		})
	}
}
