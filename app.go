package main

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/unixsuperhero/ochazuke"
	ocli "github.com/unixsuperhero/ochazuke/cli"
	"io"
	"os"
	"os/exec"
	"slices"
	"strconv"
	"strings"
	"time"
)

const version = "0.1.0"
const pageID = "questions"
const listID = "questions/questions"

type App struct {
	store       *Store
	r           *ochazuke.Runtime
	in          io.Reader
	out, errOut io.Writer
	tui         bool
	changed     func()
	run         func(*exec.Cmd) error
	showThread  func(Question) error
}
type verbSpec struct {
	name, description string
	params            map[string]ochazuke.Parameter
	positional        []string
	rows              bool
}

func param(desc string) ochazuke.Parameter {
	return ochazuke.Parameter{Type: "string", Description: desc}
}
func required(desc string) ochazuke.Parameter { p := param(desc); p.Required = true; return p }
func prose(desc string) ochazuke.Parameter    { p := required(desc); p.Rest = true; return p }
func newApp(s *Store, in io.Reader, out, errOut io.Writer) (*App, error) {
	a := &App{store: s, in: in, out: out, errOut: errOut, run: func(cmd *exec.Cmd) error { return cmd.Run() }}
	a.r = ochazuke.NewRuntime()
	a.r.Config().App = "o-questions"
	a.showThread = func(q Question) error {
		if !a.r.Settings().Bool("view.thread") {
			return nil
		}
		return openThreadViewer(a.r, q.Title, thread(q, a.r.Settings().Bool("detail.provenance")))
	}
	if e := a.r.DeclareSetting(appSettings()...); e != nil {
		return nil, e
	}
	for _, v := range a.specs() {
		v := v
		b := a.r.Define("question").Verb(v.name, func(c ochazuke.Ctx) ochazuke.Result { return a.handle(v.name, c) }, v.description)
		if len(v.positional) > 0 {
			b.Positional(v.positional...)
		}
		if v.rows {
			b.Rows("question").Fetch().Quiet()
		}
		op := a.r.Operations["question."+v.name]
		op.Parameters = v.params
		a.r.Operations[op.Key()] = op
	}
	a.r.EnsureConfigSettings()
	a.r.Define("question").Columns(columns...)
	registerQuestionUI(a)
	if e := ocli.DeclarePage(a.r, questionPage(a.r.Settings().Bool("view.detail"))); e != nil {
		return nil, e
	}
	for _, v := range []struct {
		key, verb string
		args      ochazuke.Arguments
	}{{"c", "ask", nil}, {"u", "undo", nil}} {
		row := ochazuke.Do("question."+v.verb, v.args)
		row.Instance = pageID
		row.Mode = "normal"
		row.Keys = v.key
		if e := a.r.Bind(row); e != nil {
			return nil, e
		}
	}
	return a, nil
}
func (a *App) view(args []string) error { return runTUI(a, args) }
func (a *App) mcp() error               { return a.serveMCP() }
func (a *App) shell() *ocli.Shell {
	sh := ocli.New(a.r, "o-questions", a.in, a.out, a.errOut)
	sh.Version = version
	sh.Prepare = func(context.Context) error { a.configure(); return nil }
	sh.Reveal = func(target string) error {
		q, e := a.store.Resolve(target, a.newestFirst())
		if e != nil {
			return e
		}
		l := a.r.List("question-targets")
		if l == nil {
			l, e = a.r.AddList("question-targets", nil)
			if e != nil {
				return e
			}
		}
		return l.SetItems([]ochazuke.ListItem{{ID: target, Noun: "question", Label: q.Title, Data: "question/" + q.ID}})
	}
	return sh
}
func (a *App) configure() {
	s := a.r.Settings()
	a.store.MaxBytes = s.Int("entry.max-bytes")
	a.store.BackupDaily = s.Bool("backup.daily")
	a.store.BackupKeep = s.Int("backup.keep")
	table := map[string]string{}
	for _, v := range s.Strings("status.from-entry") {
		k, v, ok := strings.Cut(v, "=")
		if ok {
			table[k] = v
		}
	}
	a.store.StatusFromEntry = table
}
func (a *App) newestFirst() bool { return a.r.Settings().String("list.order") == "newest-first" }
func (a *App) specs() []verbSpec {
	target := ochazuke.Parameter{Type: "string", Description: "Question title, number or ID prefix; omitted uses the selected row", Noun: "question"}
	arr := ochazuke.Parameter{Type: "array", Description: "Values to append"}
	ask := map[string]ochazuke.Parameter{"question": {Type: "string", Required: true, Rest: true, Description: "Owner's exact words, verbatim"}, "title": param("Short title"), "project": param("Project ID; absence is unassociated"), "cwd": param("Project directory"), "task": param("Task ID"), "tags": arr, "links": arr, "asked_at": param("RFC3339 asking time"), "follow_up_of": param("Append follow-up to this question"), "key": param("Idempotency key"), "source": {Type: "object", Description: "Source provenance"}}
	out := []verbSpec{
		{"ask", "Record the owner's verbatim question", ask, []string{"question"}, false},
		{"list", "List questions, newest last", map[string]ochazuke.Parameter{"filter": param("Search title, text, entries and tags"), "status": {Type: "string", Enum: append([]string{"all"}, statuses...)}, "project": param("Project ID"), "agent": param("Recording harness"), "pane": param("Asking pane"), "agent_session": param("Agent session"), "since": param("RFC3339, today, 7d or 30d"), "limit": {Type: "integer"}}, nil, true},
		{"show", "Show the complete thread", map[string]ochazuke.Parameter{"target": target}, []string{"target"}, false},
		{"answer", "Append the answer given", map[string]ochazuke.Parameter{"target": target, "answer": prose("Full answer"), "summary": param("Short answer summary"), "model": param("Answering model"), "links": arr, "tags": arr}, []string{"target", "answer"}, false},
		{"note", "Append a note or correction", map[string]ochazuke.Parameter{"target": target, "text": prose("Entry text"), "kind": {Type: "string", Enum: []string{"note", "correction"}}, "replaces": param("Entry corrected")}, []string{"target", "text"}, false},
		{"follow-up", "Append the owner's follow-up", map[string]ochazuke.Parameter{"target": target, "text": prose("Verbatim follow-up")}, []string{"target", "text"}, false},
		{"correct", "Append a correction, preserving old words", map[string]ochazuke.Parameter{"target": target, "text": prose("Correction"), "replaces": param("Entry corrected")}, []string{"target", "text"}, false},
		{"annotate", "Add associations, tags, links or a title", map[string]ochazuke.Parameter{"target": target, "tags": arr, "links": arr, "project": param("Project ID"), "task": param("Task ID"), "title": param("Title")}, []string{"target"}, false},
		{"status", "Set an explicit, sticky status (owner)", map[string]ochazuke.Parameter{"target": target, "status": {Type: "string", Required: true, Description: "New status", Enum: statuses}}, []string{"target", "status"}, false},
		{"rename", "Change the title", map[string]ochazuke.Parameter{"target": target, "title": prose("New title")}, []string{"target", "title"}, false},
		{"edit", "Change the owner's question, retaining undo (owner)", map[string]ochazuke.Parameter{"target": target, "text": prose("Question text")}, []string{"target", "text"}, false},
		{"copy", "Return question, latest answer or thread", map[string]ochazuke.Parameter{"target": target, "part": {Type: "string", Enum: []string{"question", "answer", "thread"}}, "yank": {Type: "boolean"}}, []string{"target"}, false},
		{"tag", "Append a tag", map[string]ochazuke.Parameter{"target": target, "tag": required("Tag")}, []string{"target", "tag"}, false},
		{"untag", "Remove a tag (owner)", map[string]ochazuke.Parameter{"target": target, "tag": required("Tag")}, []string{"target", "tag"}, false},
		{"link", "Append a link", map[string]ochazuke.Parameter{"target": target, "link": required("URL or file")}, []string{"target", "link"}, false},
		{"unlink", "Remove a link (owner)", map[string]ochazuke.Parameter{"target": target, "link": required("URL or file")}, []string{"target", "link"}, false},
		{"associate", "Associate with a project or task", map[string]ochazuke.Parameter{"target": target, "project": param("Project ID"), "task": param("Task ID")}, []string{"target", "project"}, false},
		{"open", "Open a link or source transcript", map[string]ochazuke.Parameter{"target": target, "link": param("Link target; required when several")}, []string{"target", "link"}, false},
		{"split", "Split a thread entry into a child question (owner)", map[string]ochazuke.Parameter{"target": target, "entry": required("Entry ID")}, []string{"target", "entry"}, false},
		{"merge", "Merge another question here (owner)", map[string]ochazuke.Parameter{"target": target, "other": required("Other question")}, []string{"target", "other"}, false},
		{"backup", "Write a private tar.gz backup", map[string]ochazuke.Parameter{"to": param("Destination directory")}, nil, false},
		{"restore", "Restore a backup, preserving undo (owner)", map[string]ochazuke.Parameter{"file": required("Backup tar.gz")}, []string{"file"}, false},
	}
	for _, v := range []struct{ name, desc string }{{"reopen", "Explicitly reopen (owner)"}, {"moot", "Mark moot (owner)"}, {"delete", "Hide, preserving history (owner)"}, {"star", "Toggle star"}, {"unstar", "Remove star"}, {"path", "Record path or store directory"}, {"check", "Check wording against source transcript (owner)"}} {
		out = append(out, verbSpec{v.name, v.desc, map[string]ochazuke.Parameter{"target": target}, []string{"target"}, false})
	}
	out = append(out, verbSpec{"undo", "Undo the last store change (owner)", nil, nil, false}, verbSpec{"history", "Show store change history", nil, nil, false})
	for _, name := range []string{"add", "new"} {
		out = append(out, verbSpec{name, "Alias of ask", ask, []string{"question"}, false})
	}
	return out
}
func result(v any, e error) ochazuke.Result {
	if e != nil {
		return ochazuke.Failed(e)
	}
	return ochazuke.Done(v)
}
func decodeArgs(args ochazuke.Arguments, v any) error {
	b, e := json.Marshal(args)
	if e != nil {
		return e
	}
	return json.Unmarshal(b, v)
}
func (a *App) target(c ochazuke.Ctx) (Question, error) {
	t := ochazuke.StringArg(c.Args, "target", "")
	if t == "" && c.Instance != nil {
		t, _ = c.Instance.Data.(string)
		if t == "" {
			t = c.Instance.ID
		}
	}
	if t == "" {
		return Question{}, fmt.Errorf("name a question or displayed number")
	}
	return a.store.Resolve(t, a.newestFirst())
}
func (a *App) handle(verb string, c ochazuke.Ctx) ochazuke.Result {
	a.configure()
	if a.tui {
		if res, ok := a.interactive(verb, c); ok {
			return res
		}
	}
	if verb == "add" || verb == "new" {
		verb = "ask"
	}
	str := func(k string) string { return ochazuke.StringArg(c.Args, k, "") }
	switch verb {
	case "ask":
		var p Ask
		if e := decodeArgs(c.Args, &p); e != nil {
			return result(nil, e)
		}
		p.Source = fillSource(p.Source, sourceFromEnv(a.store.Actor))
		v, e := a.store.Ask(p)
		a.refresh()
		return result(v, e)
	case "list":
		v, e := a.rows(c.Args)
		return result(v, e)
	case "undo":
		e := a.store.Undo()
		a.refresh()
		return result(nil, e)
	case "history":
		v, e := a.store.History()
		return result(v, e)
	case "backup":
		v, e := a.store.Backup(str("to"))
		return result(v, e)
	case "restore":
		e := a.store.Restore(str("file"))
		a.refresh()
		return result(nil, e)
	case "path":
		if str("target") == "" && c.Instance == nil {
			return result(a.store.Dir, nil)
		}
	}
	q, e := a.target(c)
	if e != nil {
		return result(nil, e)
	}
	switch verb {
	case "show":
		if a.tui && a.showThread != nil {
			return result(nil, a.showThread(q))
		}
		return result(q, nil)
	case "path":
		return result(a.store.path(q), nil)
	case "answer", "note", "follow-up", "correct":
		kind := verb
		text := str("text")
		if verb == "answer" {
			text = str("answer")
		}
		if verb == "correct" {
			kind = "correction"
		}
		if verb == "note" && str("kind") != "" {
			kind = str("kind")
		}
		var ann Annotation
		if e = decodeArgs(c.Args, &ann); e != nil {
			return result(nil, e)
		}
		// Tags and entry append are one transaction so invalid annotations lose no answer.
		v, e := a.store.mutate(mapAction(kind), q.ID, func(q *Question, _ *[]Question) error {
			if e := a.store.annotate(q, Annotation{Tags: ann.Tags}); e != nil {
				return e
			}
			en := Entry{Kind: kind, Text: text, Summary: str("summary"), Replaces: str("replaces"), Links: ann.Links}
			act := a.store.Actor
			if str("model") != "" {
				a.store.Actor.Model = str("model")
				defer func() { a.store.Actor = act }()
			}
			return a.store.appendEntry(q, en)
		})
		a.refresh()
		return result(v, e)
	case "copy":
		text := thread(q, true)
		switch str("part") {
		case "question":
			text = q.Text
		case "answer":
			text = latestAnswer(q)
		}
		if yank, _ := c.Args["yank"].(bool); yank {
			cmd := exec.Command("pbcopy")
			cmd.Stdin = strings.NewReader(text)
			if e = a.run(cmd); e != nil {
				return result(nil, e)
			}
		}
		return result(text, nil)
	case "open":
		links := append([]Link{}, q.Links...)
		if q.Source.Transcript != "" {
			links = append(links, Link{Target: q.Source.Transcript})
		}
		target := str("link")
		if target == "" {
			if len(links) != 1 {
				return result(nil, fmt.Errorf("choose --link from %v", links))
			}
			target = links[0].Target
		}
		return result(nil, a.run(exec.Command("open", target)))
	case "check":
		return a.check(q)
	}
	var ann Annotation
	if e = decodeArgs(c.Args, &ann); e != nil {
		return result(nil, e)
	}
	if verb == "tag" {
		ann.Tags = []string{str("tag")}
	}
	if verb == "link" {
		ann.Links = []Link{{Target: str("link")}}
	}
	value := str("text")
	if verb == "status" {
		value = str("status")
	}
	if verb == "untag" {
		value = str("tag")
	}
	if verb == "unlink" {
		value = str("link")
	}
	v, e := a.store.Change(verb, q.ID, Change{Value: value, Entry: str("entry"), Other: str("other"), Annotation: ann})
	a.refresh()
	return result(v, e)
}
func mapAction(kind string) string {
	if kind == "correction" {
		return "correct"
	}
	return kind
}
func sourceFromEnv(a Actor) Source {
	return Source{Harness: a.Harness, Pane: a.Pane, TuiosSession: a.Session, AgentSession: firstEnv("CODEX_THREAD_ID", "CLAUDE_SESSION_ID"), Transcript: os.Getenv("TRANSCRIPT_PATH"), Turn: os.Getenv("AGENT_TURN_ID")}
}
func firstEnv(keys ...string) string {
	for _, k := range keys {
		if v := os.Getenv(k); v != "" {
			return v
		}
	}
	return ""
}
func (a *App) refresh() {
	if a.changed != nil {
		a.changed()
	}
}
func latestAnswer(q Question) string {
	for i := len(q.Entries) - 1; i >= 0; i-- {
		if q.Entries[i].Kind == "answer" {
			return q.Entries[i].Text
		}
	}
	return ""
}

var columns = []ochazuke.Column{{Title: "#", Width: 4}, {Title: "Status", Width: 10}, {Title: "Question", Min: 18}, {Title: "Project", Max: 20, Priority: 2}, {Title: "Agent", Max: 15, Priority: 1}, {Title: "Age", Width: 7, Priority: 2}, {Title: "Answers", Width: 7, Priority: 2}}

func age(at, now time.Time) string {
	d := now.Sub(at)
	if d < time.Minute {
		return "now"
	}
	if d < time.Hour {
		return fmt.Sprintf("%dm", int(d.Minutes()))
	}
	if d < 24*time.Hour {
		return fmt.Sprintf("%dh", int(d.Hours()))
	}
	return fmt.Sprintf("%dd", int(d.Hours()/24))
}
func (a *App) rows(args ochazuke.Arguments) ([]ocli.Row, error) {
	qs, e := a.store.List()
	if e != nil {
		return nil, e
	}
	if a.newestFirst() {
		slices.Reverse(qs)
	}
	out := []ocli.Row{}
	get := func(k, setting string) string {
		if v := ochazuke.StringArg(args, k, ""); v != "" {
			return v
		}
		if setting != "" {
			return a.r.Settings().String(setting)
		}
		return ""
	}
	status := get("status", "filter.status")
	project := get("project", "filter.project")
	agent := get("agent", "filter.agent")
	since := get("since", "filter.since")
	var cutoff time.Time
	switch since {
	case "", "any":
	case "today":
		now := a.store.now()
		cutoff = time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	case "7d":
		cutoff = a.store.now().Add(-7 * 24 * time.Hour)
	case "30d":
		cutoff = a.store.now().Add(-30 * 24 * time.Hour)
	default:
		cutoff, e = time.Parse(time.RFC3339, since)
		if e != nil {
			return nil, e
		}
	}
	filter := strings.ToLower(get("filter", ""))
	pane := get("pane", "")
	session := get("agent_session", "")
	for i, q := range qs {
		if status != "" && status != "all" && q.Status != status {
			continue
		}
		if project != "" && project != "all" && q.Project.ID != project {
			continue
		}
		if agent != "" && agent != "all" && q.RecordedBy.Harness != agent {
			continue
		}
		if pane != "" && q.Source.Pane != pane {
			continue
		}
		if session != "" && q.Source.AgentSession != session {
			continue
		}
		if q.AskedAt.Before(cutoff) {
			continue
		}
		search := thread(q, true) + strings.Join(q.Tags, " ")
		if !strings.Contains(strings.ToLower(search), filter) {
			continue
		}
		b, _ := json.Marshal(q)
		data := map[string]any{}
		_ = json.Unmarshal(b, &data)
		data["search"] = search
		data["question"] = q.Text
		data["answer"] = latestAnswer(q)
		data["thread"] = thread(q, true)
		count := 0
		for _, en := range q.Entries {
			if en.Kind == "answer" {
				count++
			}
		}
		label := q.Title
		if q.Starred {
			label = "★ " + label
		}
		out = append(out, ocli.Row{Noun: "question", ID: "question/" + q.ID, Label: q.Title, Data: data, Cells: []string{strconv.Itoa(i + 1), q.Status, label, q.Project.ID, q.RecordedBy.Harness, age(q.AskedAt, a.store.now()), strconv.Itoa(count), search}})
	}
	limit := 0
	switch v := args["limit"].(type) {
	case int:
		limit = v
	case float64:
		limit = int(v)
	}
	if limit > 0 && len(out) > limit {
		out = out[len(out)-limit:]
	}
	return out, nil
}

func fillSource(given, fallback Source) Source {
	for _, v := range []struct {
		to       *string
		fallback string
	}{{&given.Harness, fallback.Harness}, {&given.AgentSession, fallback.AgentSession}, {&given.Pane, fallback.Pane}, {&given.TuiosSession, fallback.TuiosSession}, {&given.Transcript, fallback.Transcript}, {&given.Turn, fallback.Turn}} {
		if *v.to == "" {
			*v.to = v.fallback
		}
	}
	return given
}
