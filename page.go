package main

import (
	"encoding/json"
	"fmt"
	"ofam.dev/ochazuke"
	ocli "ofam.dev/ochazuke/cli"
	"strings"
)

const questionDetailKind = "question-preview"

func questionPage(detail bool) ocli.CatalogPage {
	p := ocli.CatalogPage{ID: pageID, Title: "Questions", Layout: ocli.LayoutSplit, Lists: []ocli.CatalogList{{ID: listID, Noun: "question", Source: "question.list", Title: "Questions"}}}
	if detail {
		p.Detail = &ocli.CatalogDetail{
			Fields: []string{"question", "answer", "status", "recorded_by", "source", "cwd", "project", "asked_at", "verbatim", "links"},
			Sections: []ocli.CatalogDetailSection{
				{Kind: "markdown", Field: "question", Heading: true},
				{Kind: "thread", Title: "Answers", Field: "entries", Author: "by.kind", At: "at", Body: "text"},
				{Kind: "fields", Title: "Metadata", Region: "side", Fields: []ocli.CatalogDetailField{
					{Label: "Status", Field: "status", Format: "badge"},
					{Label: "Project", Field: "project.id", Format: "text"},
					{Label: "Asker", Field: "asked_by.kind", Format: "text"},
					{Label: "Agent", Field: "recorded_by.name", Format: "text"},
					{Label: "Harness", Field: "recorded_by.harness", Format: "text"},
					{Label: "Model", Field: "recorded_by.model", Format: "text"},
					{Label: "Pane", Field: "source.pane", Format: "short-id"},
					{Label: "Session", Field: "source.agent_session", Format: "short-id"},
					{Label: "Asked", Field: "asked_at", Format: "relative-time"},
					{Label: "Updated", Field: "updated", Format: "relative-time"},
					{Label: "Cwd", Field: "cwd", Format: "path"},
					{Label: "Task", Field: "task", Format: "text"},
					{Label: "Transcript", Field: "source.transcript", Format: "path"},
					{Label: "Verbatim check", Field: "verbatim", Format: "badge"},
					{Label: "Tags", Field: "tags", Format: "list"},
					{Label: "Links", Field: "links", Format: "list"},
				}},
			},
		}
	}
	return p
}
func standalonePage(detail bool) ocli.CatalogPage {
	p := questionPage(detail)
	if p.Detail != nil {
		p.Detail.Kind = questionDetailKind
	}
	return p
}
func registerQuestionUI(a *App) {
	a.r.DefineGuard("question-thread-enabled", func(r *ochazuke.Runtime, _ ochazuke.KeyEvent) bool { return r.Settings().Bool("view.thread") })
	b := a.r.Define("question").Mode("normal")
	for _, v := range []struct {
		key, verb string
		args      ochazuke.Arguments
	}{
		{"v", "show", nil}, {"enter", "show", nil}, {"c", "ask", nil}, {"a", "answer", nil}, {"f", "follow-up", nil}, {"s", "status", nil}, {"y", "copy", ochazuke.Arguments{"yank": true}}, {"e", "edit", nil}, {"r", "rename", nil}, {"m", "associate", nil}, {"t", "tag", nil}, {"g x", "open", nil}, {"d d", "delete", nil}, {"u", "undo", nil},
	} {
		row := ochazuke.Do("question."+v.verb, v.args)
		if v.verb == "show" {
			row = row.Guard("question-thread-enabled")
		}
		b.Bind(v.key, row)
	}
	b.Bind("n", ochazuke.Do("ochazuke.list.move", ochazuke.Arguments{"delta": 1, "clamp": true}).FromPayload("steps", "count"))
	b.Bind("N", ochazuke.Do("ochazuke.list.move", ochazuke.Arguments{"delta": -1, "clamp": true}).FromPayload("steps", "count"))
	b.Bind("o", ochazuke.Do("ochazuke.setting.cycle", ochazuke.Arguments{"name": "filter.status"}))

	registerThreadViewer(a.r)
}

type questionDetail struct {
	host   ocli.DetailHost
	detail *ochazuke.Detail
	q      Question
	valid  bool
}

func newQuestionDetail(host ocli.DetailHost) (ocli.DetailView, error) {
	p := host.Model.Runtime.Instances[host.Parent]
	if p == nil {
		return nil, fmt.Errorf("missing detail parent")
	}
	d := host.Model.Runtime.Detail(p.Parent)
	if d == nil {
		return nil, fmt.Errorf("missing detail window")
	}
	return &questionDetail{host: host, detail: d}, nil
}
func (v *questionDetail) Show(row ocli.Row, ok bool) error {
	v.valid = ok
	if !ok {
		v.q = Question{}
		return nil
	}
	b, e := json.Marshal(row.Data)
	if e != nil {
		return e
	}
	return json.Unmarshal(b, &v.q)
}
func (v *questionDetail) Focused() bool { return false }
func (v *questionDetail) Block(w, h int) ochazuke.Block {
	if w <= 0 || h <= 0 {
		return ochazuke.Text("")
	}
	if !v.valid {
		return ochazuke.Text("No question selected")
	}
	q := v.q
	settings := v.host.Model.Runtime.Settings()
	text := q.Text
	if n := settings.Int("detail.answer-lines"); n > 0 {
		answer := strings.Split(latestAnswer(q), "\n")
		if len(answer) > n {
			answer = append(answer[:n], "…")
		}
		text += "\n\nLatest answer\n" + strings.Join(answer, "\n")
	}
	if settings.Bool("detail.provenance") {
		text += fmt.Sprintf("\n\nStatus: %s\nAgent: %s · Model: %s\nProject: %s\nCwd: %s\nAsked: %s\nVerbatim: %s\nPane: %s · Session: %s\nTranscript: %s", q.Status, q.RecordedBy.Harness, q.RecordedBy.Model, q.Project.ID, q.Cwd, q.AskedAt.Format("2006-01-02 15:04Z"), q.Verbatim, q.Source.Pane, q.Source.AgentSession, q.Source.Transcript)
	}
	for _, l := range q.Links {
		text += "\n" + l.Label + ": " + l.Target
	}
	body, marker := v.detail.Window(threadAtWidth(text, w), 0, max(1, h-1))
	if marker != "" {
		body = marker + "\n" + body
	}
	return ochazuke.Text(body)
}
