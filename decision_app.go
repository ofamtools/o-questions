package main

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"ofam.dev/ochazuke"
	ocli "ofam.dev/ochazuke/cli"
)

const decisionPageID = "decision"

// decisionPageTitle is the Decisions page's tab. A host that mounts the page
// refuses a title another of its pages already has (mtm's own Review tab is
// its turn reviews), so the page is named for what it holds.
const decisionPageTitle = "Decisions"
const decisionListID = "decision/questions"

func (a *App) decisionSpecs() []verbSpec {
	// The target picker lists the decisions (not topic headers), each by its
	// title; it opens on the selected row, which an omitted target uses.
	target := ochazuke.Parameter{Type: "string", Description: "Question title, number or ID prefix; omitted uses the selected row", Noun: decisionNoun, Focus: true, Source: "decision.list", SourceFixed: map[string]any{"flat": true}}
	arr := func(d string) ochazuke.Parameter { return ochazuke.Parameter{Type: "array", Description: d} }
	pose := map[string]ochazuke.Parameter{
		"question": {Type: "string", Required: true, Rest: true, Description: "The question for the owner"},
		"title":    param("Short title"), "group": param("Topic; default the source file"),
		"source": param("Where it comes from, for example plan.md:42"), "details": param("Context the owner needs to decide"),
		"recommendation": param("Your recommendation; shown unconfirmed until accepted"), "recommended_action": param("The action you recommend (default the first action)"),
		"actions": arr("Allowed actions; default the decision.actions setting"),
		"project": param("Project ID"), "cwd": param("Project directory"), "task": param("Task ID"),
		"tags": arr("Values to append"), "links": arr("Values to append"),
		"key": param("Stable id; posing it again updates the question and keeps the decision"),
	}
	return []verbSpec{
		{"pose", "Record a question for the owner, with your recommendation", pose, []string{"question"}, false, runPose, nil},
		{"queue", "List questions for the owner, grouped by topic", map[string]ochazuke.Parameter{"show": {Type: "string", Enum: decisionFilters, Description: "Decision state shown; default decision.show"}, "group": param("Topic"), "flat": {Type: "boolean", Description: "Only the questions, no topic header rows (what a picker lists)"}}, nil, true, runQueue, nil},
		{"decide", "Decide a question for the owner (owner)", map[string]ochazuke.Parameter{"target": target, "action": param("One of the item's actions, ask, or any action"), "choice": {Type: "integer", Description: "Numbered action, 1 first"}, "note": param("Note for the agent")}, []string{"target", "action"}, false, decisionVerb(decide), nil},
		{"accept", "Accept the recommendation (owner)", map[string]ochazuke.Parameter{"target": target}, []string{"target"}, false, decisionVerb(accept), nil},
		{"undecide", "Clear the decision, keeping its note (owner)", map[string]ochazuke.Parameter{"target": target}, []string{"target"}, false, decisionVerb(undecide), nil},
		{"ask-back", "Ask the agent before deciding (owner)", map[string]ochazuke.Parameter{"target": target, "text": prose("Your question")}, []string{"target", "text"}, false, decisionVerb(askBack), nil},
		{"decision-note", "Set the note on a decision (owner)", map[string]ochazuke.Parameter{"target": target, "note": prose("Note for the agent")}, []string{"target", "note"}, false, decisionVerb(decisionNote), nil},
		{"reply", "Answer the owner's ask on a question for the owner", map[string]ochazuke.Parameter{"target": target, "text": prose("Your answer"), "recommendation": param("Replace the recommendation"), "recommended_action": param("Replace the recommended action"), "question": param("The owner's question, when the thread lacks it")}, []string{"target", "text"}, false, decisionVerb(reply), nil},
		{"applied", "Record the result of applying a decision; done locks it", map[string]ochazuke.Parameter{"target": target, "outcome": {Type: "string", Required: true, Enum: []string{"done", "failed"}, Description: "done or failed"}, "result": prose("What changed and how it was checked, or the failure")}, []string{"target", "outcome", "result"}, false, decisionVerb(applied), nil},
		{"decision-export", "The owner's decisions, asks and undecided items, for the agent", map[string]ochazuke.Parameter{"group": param("Topic"), "all": {Type: "boolean", Description: "Include applied decisions"}, "render": {Type: "string", Enum: []string{"json", "plan"}, Description: "json (default) or plan: Owner decisions sections for plan-imported items"}, "date": param("Date for plan sections (YYYY-MM-DD)")}, nil, false, runDecisionExport, nil},
		{"import", "Pose the open owner questions of a plans folder", map[string]ochazuke.Parameter{"root": param("Plans folder; default decision.plans-root"), "include_answered": {Type: "boolean", Description: "Also pose questions the plan already answers"}}, []string{"root"}, false, runImport, nil},
	}
}

func runPose(a *App, c ochazuke.Ctx) ochazuke.Result {
	var p Pose
	if e := decodeArgs(c.Args, &p); e != nil {
		return result(nil, e)
	}
	p.Provenance = sourceFromEnv(a.store.Actor)
	v, e := a.store.Pose(p)
	a.refresh()
	return result(v, e)
}

// declareDecisionPage defines decision.open-page, the verb bare `decision` runs.
func declareDecisionPage(a *App) {
	a.r.Define(decisionNoun).Verb(decisionPageVerb, a.decisionPageVerb, "Open the Decisions page (--web: mgmt's browser page)").OpensPage(decisionPageID)
	op := a.r.Operations.At(ochazuke.Key(decisionNoun + "." + decisionPageVerb))
	op.Parameters = map[string]ochazuke.Parameter{
		"web": {Type: "boolean", Description: "Serve mgmt's browser page instead of the terminal page"}, "no-open": {Type: "boolean", Description: "With --web: do not open the browser"}, "port": {Type: "integer", Description: "With --web: port to serve on"},
		"smoke": {Type: "boolean", Description: "Render one frame and exit"}, "width": {Type: "integer", Description: "Frame width for --smoke"}, "height": {Type: "integer", Description: "Frame height for --smoke"}}
	a.r.Operations.Set(op)
}

func (a *App) decisionPageVerb(c ochazuke.Ctx) ochazuke.Result { return runDecisionPage(a, c) }

// runDecisionPage is decision.open-page: the flags decisionCommand reads, as verb
// parameters. It is also what bare `decision` runs ((the decision noun's default action)).
func runDecisionPage(a *App, c ochazuke.Ctx) ochazuke.Result {
	var args []string
	for _, k := range []string{"web", "no-open", "smoke"} {
		if truthy(c.Args[k]) {
			args = append(args, "--"+k)
		}
	}
	for _, k := range []string{"port", "width", "height"} {
		if v, ok := c.Args[k]; ok && fmt.Sprint(v) != "" {
			args = append(args, "--"+k, fmt.Sprint(v))
		}
	}
	return result(nil, a.decisionCommand(args))
}

func truthy(v any) bool {
	switch x := v.(type) {
	case bool:
		return x
	case string:
		return x == "true" || x == "1"
	}
	return false
}

func runQueue(a *App, c ochazuke.Ctx) ochazuke.Result {
	v, e := a.decisionRows(c.Args)
	return result(v, e)
}

func runDecisionExport(a *App, c ochazuke.Ctx) ochazuke.Result {
	for _, r := range decisionRenders {
		if r.name == argStr(c, "render") {
			v, e := r.render(a, c)
			return result(v, e)
		}
	}
	all, _ := c.Args["all"].(bool)
	v, e := a.store.DecisionExport(argStr(c, "group"), all)
	return result(v, e)
}

// decisionRenders are the "render" values other than the default json.
var decisionRenders = []struct {
	name   string
	render func(a *App, c ochazuke.Ctx) (any, error)
}{{"plan", func(a *App, c ochazuke.Ctx) (any, error) { return a.store.PlanDecisions(argStr(c, "date")) }}}

func runImport(a *App, c ochazuke.Ctx) ochazuke.Result {
	root := argStr(c, "root")
	if root == "" {
		root = a.r.Settings().String("decision.plans-root")
	}
	answered, _ := c.Args["include_answered"].(bool)
	v, e := a.store.ImportPlans(root, answered)
	a.refresh()
	return result(v, e)
}

// decisionVerb runs a store call on one resolved question and refreshes.
func decisionVerb(f func(a *App, id string, c ochazuke.Ctx) (Question, error)) verbRun {
	return onTarget(func(a *App, q Question, c ochazuke.Ctx) ochazuke.Result {
		v, e := f(a, q.ID, c)
		a.refresh()
		return result(v, e)
	})
}

func decide(a *App, id string, c ochazuke.Ctx) (Question, error) {
	d := DecisionChange{Action: argStr(c, "action")}
	d.Choice = intArg(c.Args["choice"])
	if _, ok := c.Args["note"]; ok {
		n := argStr(c, "note")
		d.Note = &n
	}
	return a.store.Decide(id, d)
}

func accept(a *App, id string, _ ochazuke.Ctx) (Question, error) {
	return a.store.Decide(id, DecisionChange{Accept: true})
}

func undecide(a *App, id string, _ ochazuke.Ctx) (Question, error) {
	return a.store.Decide(id, DecisionChange{Clear: true})
}

func askBack(a *App, id string, c ochazuke.Ctx) (Question, error) {
	text := argStr(c, "text")
	return a.store.Decide(id, DecisionChange{Action: askOut, Note: &text})
}

func decisionNote(a *App, id string, c ochazuke.Ctx) (Question, error) {
	note := argStr(c, "note")
	return a.store.Decide(id, DecisionChange{Note: &note})
}

func reply(a *App, id string, c ochazuke.Ctx) (Question, error) {
	rp := Reply{Text: argStr(c, "text"), Question: argStr(c, "question")}
	if _, ok := c.Args["recommendation"]; ok {
		s := argStr(c, "recommendation")
		rp.Recommendation = &s
	}
	if _, ok := c.Args["recommended_action"]; ok {
		s := argStr(c, "recommended_action")
		rp.RecommendedAction = &s
	}
	return a.store.Reply(id, rp)
}

func applied(a *App, id string, c ochazuke.Ctx) (Question, error) {
	return a.store.Applied(id, argStr(c, "outcome"), argStr(c, "result"))
}

func intArg(v any) int {
	switch n := v.(type) {
	case int:
		return n
	case float64:
		return int(n)
	case string:
		i, _ := strconv.Atoi(n)
		return i
	case json.Number:
		i, _ := n.Int64()
		return int(i)
	}
	return 0
}

// decisionCell is the Status column of a decision row: the decision, the
// unconfirmed recommendation ("yes?"), asked, or applied, with "!" when the
// context changed or the agent replied since the owner last touched it.
func decisionCell(q Question, ask bool) string {
	r := q.Decision
	cell := ""
	switch decisionClass(q, ask) {
	case "applied":
		cell = "✓ " + r.Action
	case "asked":
		cell = "asked"
	case "undecided":
		cell = "·"
		if r.RecommendedAction != "" || r.Recommendation != "" {
			cell = recommended(r, nil) + "?"
		}
	default:
		cell = r.Action
	}
	if r.Status == decisionFailed {
		cell += " ✗"
	}
	if r.ContextChanged || r.NewReply {
		cell = "! " + cell
	}
	return cell
}

func recommended(r *Decision, defaults []string) string {
	if r.RecommendedAction != "" {
		return r.RecommendedAction
	}
	if len(defaults) > 0 {
		return defaults[0]
	}
	return defaultActions[0]
}

// recommendationView is the recommendation as the detail shows it: marked
// unconfirmed until the owner accepts it or decides otherwise.
func recommendationView(q Question, defaults []string, ask bool) string {
	r := q.Decision
	if r.Recommendation == "" && r.RecommendedAction == "" {
		return ""
	}
	rec := recommended(r, defaults)
	state := "recommended · not confirmed"
	switch {
	case r.Action == rec:
		state = "recommended · accepted"
	case r.Action != "" && !(r.Action == askOut && ask):
		state = "recommended · you chose " + r.Action
	}
	return fmt.Sprintf("**%s** (%s)\n\n%s", rec, state, r.Recommendation)
}

func choicesView(q Question, defaults []string, ask bool) string {
	var out []string
	for i, act := range decisionActions(q, defaults, ask) {
		if i >= 9 {
			break
		}
		out = append(out, fmt.Sprintf("%d %s", i+1, act))
	}
	return strings.Join(out, " · ")
}

// decisionRows answer the Decisions page and `question queue`: a tree with one
// parent row per topic (its counts in the label) and the topic's questions.
func (a *App) decisionRows(args ochazuke.Arguments) ([]ocli.Row, error) {
	qs, e := a.store.ownerQuestions()
	if e != nil {
		return nil, e
	}
	ask, defaults := a.store.DecisionAsk, a.store.decisionDefaults()
	show := ochazuke.StringArg(args, "show", "")
	if show == "" {
		show = a.r.Settings().String("decision.show")
	}
	group := ochazuke.StringArg(args, "group", "")
	flat, _ := args["flat"].(bool)
	qs = sortByGroup(qs)
	type counts struct{ n, decided, asked int }
	tally := map[string]*counts{}
	for _, q := range qs {
		g := groupOf(q)
		if tally[g] == nil {
			tally[g] = &counts{}
		}
		tally[g].n++
		switch decisionClass(q, ask) {
		case "decided", "applied":
			tally[g].decided++
		case "asked":
			tally[g].asked++
		}
	}
	lines := map[string][]string{}
	for _, q := range qs {
		if g := groupOf(q); (group == "" || g == group) && decisionShown(q, ask, show) {
			lines[g] = append(lines[g], fmt.Sprintf("- %s · %s", decisionCell(q, ask), q.Title))
		}
	}
	out := []ocli.Row{}
	last := ""
	for _, q := range qs {
		g := groupOf(q)
		if (group != "" && g != group) || !decisionShown(q, ask, show) {
			continue
		}
		if g != last && !flat {
			last = g
			t := tally[g]
			label := fmt.Sprintf("%s  %d/%d decided", g, t.decided, t.n)
			if t.asked > 0 {
				label += fmt.Sprintf(" · %d asked", t.asked)
			}
			out = append(out, ocli.Row{Noun: decisionNoun, ID: "topic/" + g, Label: label, Data: map[string]any{"topic": g, "question": "## " + label + "\n\n" + strings.Join(lines[g], "\n")}, Cells: []string{"", label, "", "", "", g}})
		}
		data := questionRowData(q)
		r := q.Decision
		data["question"] = q.Text
		data["topic"] = g
		data["decision_view"] = decisionCell(q, ask)
		data["recommendation_view"] = recommendationView(q, defaults, ask)
		data["choices"] = choicesView(q, defaults, ask)
		data["details_view"] = r.Details
		data["thread"] = thread(q, false)
		search := strings.Join([]string{q.Title, q.Text, r.Details, r.Recommendation, r.Note, g}, " ")
		data["search"] = search
		replies := 0
		for _, en := range q.Entries {
			if en.Kind == "reply" {
				replies++
			}
		}
		out = append(out, ocli.Row{Parent: parentOf(g, flat), Noun: decisionNoun, ID: "question/" + q.ID, Label: q.Title, Data: data, Cells: []string{decisionCell(q, ask), q.Title, q.RecordedBy.Name, age(q.AskedAt, a.store.now()), strconv.Itoa(replies), search}})
	}
	return out, nil
}

// parentOf is a decision row's parent topic row; a flat answer has none.
func parentOf(group string, flat bool) string {
	if flat {
		return ""
	}
	return "topic/" + group
}

func decisionPage() ocli.CatalogPage {
	return ocli.CatalogPage{ID: decisionPageID, Title: decisionPageTitle, Layout: ocli.LayoutSplit,
		Lists: []ocli.CatalogList{{ID: decisionListID, Noun: decisionNoun, Source: "decision.list", Title: "For you", Tree: true}},
		Detail: &ocli.CatalogDetail{
			Fields: []string{"question", "recommendation_view", "choices", "details_view", "topic", "decision_view"},
			Sections: []ocli.CatalogDetailSection{
				{Kind: "markdown", Field: "question", Heading: true},
				{Kind: "markdown", Title: "Recommendation", Field: "recommendation_view"},
				{Kind: "markdown", Title: "Actions", Field: "choices"},
				{Kind: "thread", Title: "Asks and replies", Field: "entries", Author: "by.kind", At: "at", Body: "text"},
				{Kind: "markdown", Title: "Context", Field: "details_view"},
				{Kind: "fields", Title: "Decision", Region: "side", Fields: []ocli.CatalogDetailField{
					{Label: "Topic", Field: "topic", Format: "text"},
					{Label: "Decision", Field: "decision_view", Format: "badge"},
					{Label: "Note", Field: "decision.note", Format: "text"},
					{Label: "Recommended", Field: "decision.recommended_action", Format: "text"},
					{Label: "Changed", Field: "decision.context_changed", Format: "text"},
					{Label: "New reply", Field: "decision.new_reply", Format: "text"},
					{Label: "Applied", Field: "decision.status", Format: "badge"},
					{Label: "Result", Field: "decision.result", Format: "text"},
					{Label: "Source", Field: "decision.source", Format: "path"},
					{Label: "Agent", Field: "recorded_by.name", Format: "text"},
					{Label: "Asked", Field: "asked_at", Format: "relative-time"},
					{Label: "Project", Field: "project.id", Format: "text"},
				}},
			},
		}}
}

// The Decisions page lists decision rows: questions for the owner seen as
// decisions. The decision noun reaches the question verbs that make sense on
// one (decide, accept, show, copy, ...) as aliases (AliasOn): one handler,
// two names. Its keys are mgmt's: 1-9 pick the item's numbered action, a
// accepts the recommendation, ? asks back, n edits the note, x clears the
// decision, o cycles decision.show.
const decisionNoun = "decision"
const decisionPageVerb = "open-page"

// verbHomes are question verbs declared on another noun: decision.list
// answers the Decisions page's rows and is also question.queue.
var verbHomes = map[string]verbHome{
	"queue":           {decisionNoun, "list", true},
	"decision-export": {decisionNoun, "export", false},
}

// verbHome is where a verb lives: noun.verb, and whether question.NAME stays
// an alias for it. Only queue keeps one; the export is decision.export.
type verbHome struct {
	noun, verb string
	alias      bool
}

// cliPath is the command words that run spec v: question NAME for a verb on
// question (or one that keeps its alias), else the verb on its own noun.
func cliPath(name string) []string {
	if home, ok := verbHomes[name]; ok && !home.alias {
		return []string{home.noun, home.verb}
	}
	return []string{"question", name}
}

// decisionAliases are the question verbs also named decision.VERB.
var decisionAliases = map[string]string{"decide": "decide", "accept": "accept", "undecide": "undecide", "ask-back": "ask-back", "decision-note": "decision-note", "reply": "reply", "applied": "applied", "show": "show", "yank": "yank", "tag": "tag", "untag": "untag", "link": "link", "open": "open", "star": "star", "unstar": "unstar", "delete": "delete", "archive": "archive", "restore": "restore", "associate": "associate", "rename": "rename", "undo": "undo"}

var decisionColumns = []ochazuke.Column{{Title: "Decision", Width: 12}, {Title: "Question", Min: 18}, {Title: "Agent", Max: 15, Priority: 1}, {Title: "Age", Width: 7, Priority: 2}, {Title: "Replies", Width: 7, Priority: 2}}

type keyRow struct {
	key, verb string
	args      ochazuke.Arguments
}

func decisionKeys() []keyRow {
	var rows []keyRow
	for i := 1; i <= 9; i++ {
		rows = append(rows, keyRow{strconv.Itoa(i), "decision.decide", ochazuke.Arguments{"choice": i}})
	}
	return append(rows,
		keyRow{"a", "decision.accept", nil},
		keyRow{"?", "decision.ask-back", nil},
		keyRow{"n", "decision.decision-note", nil},
		keyRow{"x", "decision.undecide", nil},
		keyRow{"o", "ochazuke.setting.cycle", ochazuke.Arguments{"name": "decision.show"}},
		keyRow{"v", "decision.show", nil},
		keyRow{"enter", "decision.show", nil},
		keyRow{"y", "decision.yank", nil},
		keyRow{"t", "decision.tag", nil},
		keyRow{"g x", "decision.open", nil},
		keyRow{"d d", "decision.delete", nil},
		keyRow{"u", "decision.undo", nil},
	)
}

func registerDecisionUI(a *App) {
	b := a.r.Define(decisionNoun).Columns(decisionColumns...).Mode("normal")
	for _, k := range decisionKeys() {
		row := ochazuke.Do(k.verb, k.args)
		if k.verb == "decision.decide" {
			if k.key == "1" {
				row = row.Hinted("decide")
			} else {
				row = row.Hide()
			}
		}
		if k.verb == "decision.show" {
			row = row.Guard("question-thread-enabled")
		}
		b.Bind(k.key, row)
	}
}

// firstQuestion is where the Decisions page starts: its first question, not
// the topic row above it.
func firstQuestion(rows []ocli.Row) string {
	for _, r := range rows {
		if r.Parent != "" {
			return r.ID
		}
	}
	return ""
}
