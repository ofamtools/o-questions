package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"ofam.dev/ochazuke"
	ocli "ofam.dev/ochazuke/cli"
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
	run               verbRun
	aliases           []string // other names of the verb on the same noun
}

func param(desc string) ochazuke.Parameter {
	return ochazuke.Parameter{Type: "string", Description: desc}
}
func required(desc string) ochazuke.Parameter { p := param(desc); p.Required = true; return p }
func prose(desc string) ochazuke.Parameter    { p := required(desc); p.Rest = true; return p }

// Namespace is the singular namespace o-questions declares (lexicon plan):
// its operations are question.<noun>.<verb> and a host mounts its catalog
// under it.
const Namespace = "question"

func init() {
	if err := ochazuke.DeclareNamespace(Namespace); err != nil {
		panic(err)
	}
}

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
	a.r.Define("question").Main()                        // the tool's main noun: `o-questions ask` is `question ask`
	a.r.Define(reviewNoun).DefaultAction(reviewPageVerb) // review.VERB aliases name question verbs (reviewAliases); bare `review` opens the page
	for _, v := range a.specs() {
		v := v
		// A verb homed on another noun (verbHomes) keeps question.NAME as an alias.
		noun, verb := "question", v.name
		if home, ok := verbHomes[v.name]; ok {
			noun, verb = home[0], home[1]
		}
		b := a.r.Define(noun).Verb(verb, func(c ochazuke.Ctx) ochazuke.Result { return a.invoke(v, c) }, v.description).Alias(v.aliases...)
		right := agentPolicy[v.name]
		if right == "" {
			right = ochazuke.AgentAllow
		}
		b.Agent(right)
		if len(v.positional) > 0 {
			b.Positional(v.positional...)
		}
		if v.rows {
			b.Rows(noun).Fetch().Quiet()
		}
		if noun != "question" {
			b.AliasOn("question", v.name)
		}
		if alias, ok := reviewAliases[v.name]; ok {
			b.AliasOn(reviewNoun, alias)
		}
		op := a.r.Operations.At(ochazuke.Key(noun + "." + verb))
		op.Parameters = v.params
		a.r.Operations.Set(op)
	}
	// show reads the thread: markdown, or the thread viewer in the TUI (setting view.thread).
	show := a.r.Operations.At("question.show")
	show.Result = ochazuke.ResultMarkdown
	show.EnabledSetting = "view.thread"
	a.r.Operations.Set(show)
	declareReviewPage(a)
	a.r.EnsureConfigSettings()
	a.declareFilterProps()
	a.r.Define("question").Columns(columns...)
	registerQuestionUI(a)
	if e := a.declareSetup(); e != nil {
		return nil, e
	}
	if e := ocli.DeclarePage(a.r, questionPage(a.r.Settings().Bool("view.detail"))); e != nil {
		return nil, e
	}
	if a.r.Settings().Bool("review.page") {
		if e := ocli.DeclarePage(a.r, reviewPage()); e != nil {
			return nil, e
		}
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
	if e := declarePickers(a.r); e != nil {
		return nil, e
	}
	return a, nil
}
func frameworkActor(a Actor) ochazuke.Actor {
	return ochazuke.Actor{Kind: a.Kind, Name: a.Name, Harness: a.Harness, Pane: a.Pane, Session: a.Session, Model: a.Model}
}

func (a *App) view(args []string) error { return runTUI(a, args) }
func (a *App) mcp() error               { return a.serveMCP() }
func (a *App) shell() *ocli.Shell {
	sh := ocli.New(a.r, "o-questions", a.in, a.out, a.errOut)
	sh.Version = version
	actor := frameworkActor(a.store.Actor)
	sh.Actor = &actor
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
	a.store.ReviewActions = s.Strings("review.actions")
	a.store.ReviewAsk = s.Bool("review.ask")
	a.store.Permit = func(action string) error {
		op, ok := a.r.Operation("question." + action)
		if !ok {
			return nil
		}
		right := a.r.AgentRight(op, false)
		if right.Right != ochazuke.AgentAllow {
			return &RefusedError{action, right.Rule}
		}
		return nil
	}
	// Kinds missing from a customised table keep their default status.
	table := map[string]string{}
	for k, v := range entryStatuses {
		table[k] = v
	}
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
		{"ask", "Record the owner's verbatim question", ask, []string{"question"}, false, runAsk, []string{"add", "new"}},
		{"list", "List questions, newest last", map[string]ochazuke.Parameter{"filter": param("Search title, text, entries and tags"), "status": {Type: "string", Enum: append([]string{"all"}, statuses...)}, "project": param("Project ID"), "agent": param("Recording harness"), "direction": {Type: "string", Enum: directions, Description: "Who answers: to-agent (the owner asked), to-owner (an agent asked); default filter.direction"}, "pane": param("Asking pane"), "agent_session": param("Agent session"), "since": param("RFC3339, today, 7d or 30d"), "limit": {Type: "integer"}}, nil, true, runList, nil},
		{"show", "Read the complete thread: the markdown thread, or the thread viewer in the TUI", map[string]ochazuke.Parameter{"target": target}, []string{"target"}, false, runShow, nil},
		{"answer", "Append the answer given", map[string]ochazuke.Parameter{"target": target, "answer": prose("Full answer"), "summary": param("Short answer summary"), "model": param("Answering model"), "links": arr, "tags": arr}, []string{"target", "answer"}, false, entryVerb("answer", "answer"), nil},
		{"follow-up", "Append the owner's follow-up", map[string]ochazuke.Parameter{"target": target, "text": prose("Verbatim follow-up")}, []string{"target", "text"}, false, entryVerb("follow-up", "text"), nil},
		{"correct", "Append a correction, preserving old words", map[string]ochazuke.Parameter{"target": target, "text": prose("Correction"), "replaces": param("Entry corrected")}, []string{"target", "text"}, false, entryVerb("correction", "text"), nil},
		{"annotate", "Annotate a question: append a note or correction (text), or add tags, links, a project, a task or a title", map[string]ochazuke.Parameter{"target": target, "text": {Type: "string", Rest: true, Description: "Entry text; appends a note (or the correction kind)"}, "kind": {Type: "string", Enum: []string{"note", "correction"}}, "replaces": param("Entry corrected"), "tags": arr, "links": arr, "project": param("Project ID"), "task": param("Task ID"), "title": param("Title")}, []string{"target", "text"}, false, runAnnotate, nil},
		{"choose-status", "Choose the question's status (open, answered, follow-up or moot): the status prop's write, with the status asked for as a required argument; explicit and sticky", map[string]ochazuke.Parameter{"target": target, "status": {Type: "string", Required: true, Description: "New status", Enum: statuses}}, []string{"target", "status"}, false, changeVerb(opStatus, "status", nil), nil},
		{"rename", "Change the title", map[string]ochazuke.Parameter{"target": target, "title": prose("New title")}, []string{"target", "title"}, false, changeVerb(opRename, "text", nil), nil},
		{"edit", "Change the owner's question, retaining undo", map[string]ochazuke.Parameter{"target": target, "text": prose("Question text")}, []string{"target", "text"}, false, changeVerb(opEdit, "text", nil), nil},
		{"yank", "Copy the thread, the question or the latest answer to the clipboard; answers the text", map[string]ochazuke.Parameter{"target": target, "part": {Type: "string", Enum: []string{"question", "answer", "thread"}}}, []string{"target"}, false, runYank, nil},
		{"tag", "Append a tag", map[string]ochazuke.Parameter{"target": target, "tag": required("Tag")}, []string{"target", "tag"}, false, changeVerb(opTag, "text", func(an *Annotation, str func(string) string) { an.Tags = []string{str("tag")} }), nil},
		{"untag", "Remove a tag", map[string]ochazuke.Parameter{"target": target, "tag": required("Tag")}, []string{"target", "tag"}, false, changeVerb(opUntag, "tag", nil), nil},
		{"link", "Append a link", map[string]ochazuke.Parameter{"target": target, "link": required("URL or file")}, []string{"target", "link"}, false, changeVerb(opLink, "text", func(an *Annotation, str func(string) string) { an.Links = []Link{{Target: str("link")}} }), nil},
		{"unlink", "Remove a link", map[string]ochazuke.Parameter{"target": target, "link": required("URL or file")}, []string{"target", "link"}, false, changeVerb(opUnlink, "link", nil), nil},
		{"associate", "Associate with a project or task", map[string]ochazuke.Parameter{"target": target, "project": param("Project ID"), "task": param("Task ID")}, []string{"target", "project"}, false, changeVerb(opAssociate, "text", nil), nil},
		{"open", "Open a link or source transcript", map[string]ochazuke.Parameter{"target": target, "link": param("Link target; required when several")}, []string{"target", "link"}, false, runOpen, nil},
		{"split", "Split a thread entry into a child question", map[string]ochazuke.Parameter{"target": target, "entry": required("Entry ID")}, []string{"target", "entry"}, false, changeVerb(opSplit, "text", nil), nil},
		{"merge", "Merge another question here", map[string]ochazuke.Parameter{"target": target, "other": required("Other question")}, []string{"target", "other"}, false, changeVerb(opMerge, "text", nil), nil},
		{"backup", "Write a private tar.gz backup", map[string]ochazuke.Parameter{"to": param("Destination directory")}, nil, false, runBackup, nil},
		{"restore", "Restore an archived question (--target) or a backup (FILE), preserving undo", map[string]ochazuke.Parameter{"target": target, "file": param("Backup tar.gz; omit when restoring a question with --target")}, []string{"file"}, false, runRestore, nil},
	}
	for _, v := range []struct {
		name, desc string
		run        verbRun
	}{{"reopen", "Explicitly reopen", changeVerb(opReopen, "text", nil)}, {"moot", "Mark moot", changeVerb(opMoot, "text", nil)}, {"delete", "Hide, preserving history; agents use question.archive", changeVerb(opDelete, "text", nil)}, {"archive", "Archive a question, retaining its text and history; restore --target brings it back", changeVerb(opArchive, "text", nil)}, {"star", "Toggle star", changeVerb(opStar, "text", nil)}, {"unstar", "Remove star", changeVerb(opUnstar, "text", nil)}, {"check", "Check wording against source transcript", runCheck}} {
		out = append(out, verbSpec{name: v.name, description: v.desc, params: map[string]ochazuke.Parameter{"target": target}, positional: []string{"target"}, run: v.run})
	}
	out = append(out, a.reviewSpecs()...)
	out = append(out, verbSpec{name: "undo", description: "Undo the last store change", run: runUndo}, verbSpec{name: "history", description: "Show store change history", run: runHistory})
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
	qs, e := a.store.records(!a.r.Settings().Bool("collection.hide-archived"))
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
	direction := get("direction", "filter.direction")
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
	number := 0
	for _, q := range qs {
		cellNumber := "-"
		if !q.Deleted {
			number++
			cellNumber = strconv.Itoa(number)
		}
		if dir := q.Direction; direction == toAgent && dir == toOwner || direction == toOwner && dir != toOwner {
			continue
		}
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
		data := questionRowData(q)
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
		out = append(out, ocli.Row{Noun: "question", ID: "question/" + q.ID, Label: q.Title, Data: data, Cells: []string{cellNumber, q.Status, label, q.Project.ID, q.RecordedBy.Harness, age(q.AskedAt, a.store.now()), strconv.Itoa(count), search}})
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
