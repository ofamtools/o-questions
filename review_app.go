package main

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"ofam.dev/ochazuke"
	ocli "ofam.dev/ochazuke/cli"
)

const reviewPageID = "review"

// reviewPageTitle is the Review page's tab. A host that mounts the page
// refuses a title another of its pages already has (mtm's own Review tab is
// its turn reviews), so the page is named for what it holds.
const reviewPageTitle = "Decisions"
const reviewListID = "review/questions"

func (a *App) reviewSpecs() []verbSpec {
	target := ochazuke.Parameter{Type: "string", Description: "Question title, number or ID prefix; omitted uses the selected row", Noun: "question"}
	arr := func(d string) ochazuke.Parameter { return ochazuke.Parameter{Type: "array", Description: d} }
	pose := map[string]ochazuke.Parameter{
		"question": {Type: "string", Required: true, Rest: true, Description: "The question for the owner"},
		"title":    param("Short title"), "group": param("Topic; default the source file"),
		"source": param("Where it comes from, for example plan.md:42"), "details": param("Context the owner needs to decide"),
		"recommendation": param("Your recommendation; shown unconfirmed until accepted"), "recommended_action": param("The action you recommend (default the first action)"),
		"actions": arr("Allowed actions; default the review.actions setting"),
		"project": param("Project ID"), "cwd": param("Project directory"), "task": param("Task ID"),
		"tags": arr("Values to append"), "links": arr("Values to append"),
		"key": param("Stable id; posing it again updates the question and keeps the decision"),
	}
	return []verbSpec{
		{"pose", "Record a question for the owner, with your recommendation", pose, []string{"question"}, false, runPose, nil},
		{"queue", "List questions for the owner, grouped by topic", map[string]ochazuke.Parameter{"show": {Type: "string", Enum: reviewFilters, Description: "Review state shown; default review.show"}, "group": param("Topic")}, nil, true, runQueue, nil},
		{"decide", "Decide a question for the owner (owner)", map[string]ochazuke.Parameter{"target": target, "action": param("One of the item's actions, ask, or any action"), "choice": {Type: "integer", Description: "Numbered action, 1 first"}, "note": param("Note for the agent")}, []string{"target", "action"}, false, decisionVerb(decide), nil},
		{"accept", "Accept the recommendation (owner)", map[string]ochazuke.Parameter{"target": target}, []string{"target"}, false, decisionVerb(accept), nil},
		{"undecide", "Clear the decision, keeping its note (owner)", map[string]ochazuke.Parameter{"target": target}, []string{"target"}, false, decisionVerb(undecide), nil},
		{"ask-back", "Ask the agent before deciding (owner)", map[string]ochazuke.Parameter{"target": target, "text": prose("Your question")}, []string{"target", "text"}, false, decisionVerb(askBack), nil},
		{"decision-note", "Set the note on a decision (owner)", map[string]ochazuke.Parameter{"target": target, "note": prose("Note for the agent")}, []string{"target", "note"}, false, decisionVerb(decisionNote), nil},
		{"reply", "Answer the owner's ask on a question for the owner", map[string]ochazuke.Parameter{"target": target, "text": prose("Your answer"), "recommendation": param("Replace the recommendation"), "recommended_action": param("Replace the recommended action"), "question": param("The owner's question, when the thread lacks it")}, []string{"target", "text"}, false, decisionVerb(reply), nil},
		{"applied", "Record the result of applying a decision; done locks it", map[string]ochazuke.Parameter{"target": target, "outcome": {Type: "string", Required: true, Enum: []string{"done", "failed"}, Description: "done or failed"}, "result": prose("What changed and how it was checked, or the failure")}, []string{"target", "outcome", "result"}, false, decisionVerb(applied), nil},
		{"decisions", "The owner's decisions, asks and undecided items, for the agent", map[string]ochazuke.Parameter{"group": param("Topic"), "all": {Type: "boolean", Description: "Include applied decisions"}, "render": {Type: "string", Enum: []string{"json", "plan"}, Description: "json (default) or plan: Owner decisions sections for plan-imported items"}, "date": param("Date for plan sections (YYYY-MM-DD)")}, nil, false, runDecisions, nil},
		{"import", "Pose the open owner questions of a plans folder", map[string]ochazuke.Parameter{"root": param("Plans folder; default review.plans-root"), "include_answered": {Type: "boolean", Description: "Also pose questions the plan already answers"}}, []string{"root"}, false, runImport, nil},
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

// declareReviewPage defines review.page, the verb bare `review` runs.
func declareReviewPage(a *App) {
	a.r.Define(reviewNoun).Verb(reviewPageVerb, a.reviewPageVerb, "Open the Decisions page (--web: mgmt's browser page)").OpensPage(reviewPageID)
	op := a.r.Operations.At(ochazuke.Key(reviewNoun + "." + reviewPageVerb))
	op.Parameters = map[string]ochazuke.Parameter{
		"web": {Type: "boolean", Description: "Serve mgmt's browser page instead of the terminal page"}, "no-open": {Type: "boolean", Description: "With --web: do not open the browser"}, "port": {Type: "integer", Description: "With --web: port to serve on"},
		"smoke": {Type: "boolean", Description: "Render one frame and exit"}, "width": {Type: "integer", Description: "Frame width for --smoke"}, "height": {Type: "integer", Description: "Frame height for --smoke"}}
	a.r.Operations.Set(op)
}

func (a *App) reviewPageVerb(c ochazuke.Ctx) ochazuke.Result { return runReviewPage(a, c) }

// runReviewPage is review.page: the flags reviewCommand reads, as verb
// parameters. It is also what bare `review` runs ((the review noun's default action)).
func runReviewPage(a *App, c ochazuke.Ctx) ochazuke.Result {
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
	return result(nil, a.reviewCommand(args))
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
	v, e := a.reviewRows(c.Args)
	return result(v, e)
}

func runDecisions(a *App, c ochazuke.Ctx) ochazuke.Result {
	for _, r := range decisionRenders {
		if r.name == argStr(c, "render") {
			v, e := r.render(a, c)
			return result(v, e)
		}
	}
	all, _ := c.Args["all"].(bool)
	v, e := a.store.Decisions(argStr(c, "group"), all)
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
		root = a.r.Settings().String("review.plans-root")
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
	d := Decision{Action: argStr(c, "action")}
	d.Choice = intArg(c.Args["choice"])
	if _, ok := c.Args["note"]; ok {
		n := argStr(c, "note")
		d.Note = &n
	}
	return a.store.Decide(id, d)
}

func accept(a *App, id string, _ ochazuke.Ctx) (Question, error) {
	return a.store.Decide(id, Decision{Accept: true})
}

func undecide(a *App, id string, _ ochazuke.Ctx) (Question, error) {
	return a.store.Decide(id, Decision{Clear: true})
}

func askBack(a *App, id string, c ochazuke.Ctx) (Question, error) {
	text := argStr(c, "text")
	return a.store.Decide(id, Decision{Action: askOut, Note: &text})
}

func decisionNote(a *App, id string, c ochazuke.Ctx) (Question, error) {
	note := argStr(c, "note")
	return a.store.Decide(id, Decision{Note: &note})
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

// decisionCell is the Status column of a review row: the decision, the
// unconfirmed recommendation ("yes?"), asked, or applied, with "!" when the
// context changed or the agent replied since the owner last touched it.
func decisionCell(q Question, ask bool) string {
	r := q.Review
	cell := ""
	switch reviewClass(q, ask) {
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
	if r.State == reviewFailed {
		cell += " ✗"
	}
	if r.ContextChanged || r.NewReply {
		cell = "! " + cell
	}
	return cell
}

func recommended(r *Review, defaults []string) string {
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
	r := q.Review
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
	for i, act := range reviewActions(q, defaults, ask) {
		if i >= 9 {
			break
		}
		out = append(out, fmt.Sprintf("%d %s", i+1, act))
	}
	return strings.Join(out, " · ")
}

// reviewRows answer the Review page and `question queue`: a tree with one
// parent row per topic (its counts in the label) and the topic's questions.
func (a *App) reviewRows(args ochazuke.Arguments) ([]ocli.Row, error) {
	qs, e := a.store.ownerQuestions()
	if e != nil {
		return nil, e
	}
	ask, defaults := a.store.ReviewAsk, a.store.reviewDefaults()
	show := ochazuke.StringArg(args, "show", "")
	if show == "" {
		show = a.r.Settings().String("review.show")
	}
	group := ochazuke.StringArg(args, "group", "")
	qs = sortByGroup(qs)
	type counts struct{ n, decided, asked int }
	tally := map[string]*counts{}
	for _, q := range qs {
		g := groupOf(q)
		if tally[g] == nil {
			tally[g] = &counts{}
		}
		tally[g].n++
		switch reviewClass(q, ask) {
		case "decided", "applied":
			tally[g].decided++
		case "asked":
			tally[g].asked++
		}
	}
	lines := map[string][]string{}
	for _, q := range qs {
		if g := groupOf(q); (group == "" || g == group) && reviewShown(q, ask, show) {
			lines[g] = append(lines[g], fmt.Sprintf("- %s · %s", decisionCell(q, ask), q.Title))
		}
	}
	out := []ocli.Row{}
	last := ""
	for _, q := range qs {
		g := groupOf(q)
		if (group != "" && g != group) || !reviewShown(q, ask, show) {
			continue
		}
		if g != last {
			last = g
			t := tally[g]
			label := fmt.Sprintf("%s  %d/%d decided", g, t.decided, t.n)
			if t.asked > 0 {
				label += fmt.Sprintf(" · %d asked", t.asked)
			}
			out = append(out, ocli.Row{Noun: reviewNoun, ID: "topic/" + g, Label: label, Data: map[string]any{"topic": g, "question": "## " + label + "\n\n" + strings.Join(lines[g], "\n")}, Cells: []string{"", label, "", "", "", g}})
		}
		data := questionRowData(q)
		r := q.Review
		data["question"] = q.Text
		data["topic"] = g
		data["decision"] = decisionCell(q, ask)
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
		out = append(out, ocli.Row{Parent: "topic/" + g, Noun: reviewNoun, ID: "question/" + q.ID, Label: q.Title, Data: data, Cells: []string{decisionCell(q, ask), q.Title, q.RecordedBy.Name, age(q.AskedAt, a.store.now()), strconv.Itoa(replies), search}})
	}
	return out, nil
}

func reviewPage() ocli.CatalogPage {
	return ocli.CatalogPage{ID: reviewPageID, Title: reviewPageTitle, Layout: ocli.LayoutSplit,
		Lists: []ocli.CatalogList{{ID: reviewListID, Noun: reviewNoun, Source: "review.list", Title: "For you", Tree: true}},
		Detail: &ocli.CatalogDetail{
			Fields: []string{"question", "recommendation_view", "choices", "details_view", "topic", "decision"},
			Sections: []ocli.CatalogDetailSection{
				{Kind: "markdown", Field: "question", Heading: true},
				{Kind: "markdown", Title: "Recommendation", Field: "recommendation_view"},
				{Kind: "markdown", Title: "Actions", Field: "choices"},
				{Kind: "thread", Title: "Asks and replies", Field: "entries", Author: "by.kind", At: "at", Body: "text"},
				{Kind: "markdown", Title: "Context", Field: "details_view"},
				{Kind: "fields", Title: "Decision", Region: "side", Fields: []ocli.CatalogDetailField{
					{Label: "Topic", Field: "topic", Format: "text"},
					{Label: "Decision", Field: "decision", Format: "badge"},
					{Label: "Note", Field: "review.note", Format: "text"},
					{Label: "Recommended", Field: "review.recommended_action", Format: "text"},
					{Label: "Changed", Field: "review.context_changed", Format: "text"},
					{Label: "New reply", Field: "review.new_reply", Format: "text"},
					{Label: "Applied", Field: "review.state", Format: "badge"},
					{Label: "Result", Field: "review.result", Format: "text"},
					{Label: "Source", Field: "review.source", Format: "path"},
					{Label: "Agent", Field: "recorded_by.name", Format: "text"},
					{Label: "Asked", Field: "asked_at", Format: "relative-time"},
					{Label: "Project", Field: "project.id", Format: "text"},
				}},
			},
		}}
}

// The Review page lists review rows: questions for the owner seen as
// decisions. The review noun reaches the question verbs that make sense on
// one (decide, accept, show, copy, ...) as aliases (AliasOn): one handler,
// two names. Its keys are mgmt's: 1-9 pick the item's numbered action, a
// accepts the recommendation, ? asks back, n edits the note, x clears the
// decision, o cycles review.show.
const reviewNoun = "review"
const reviewPageVerb = "page"

// verbHomes are question verbs declared on another noun: review.list
// answers the Review page's rows and is also question.queue.
var verbHomes = map[string][2]string{"queue": {reviewNoun, "list"}}

// reviewAliases are the question verbs also named review.VERB.
var reviewAliases = map[string]string{"decide": "decide", "accept": "accept", "undecide": "undecide", "ask-back": "ask-back", "decision-note": "decision-note", "reply": "reply", "applied": "applied", "show": "show", "copy": "copy", "tag": "tag", "untag": "untag", "link": "link", "open": "open", "star": "star", "unstar": "unstar", "delete": "delete", "archive": "archive", "restore": "restore", "path": "path", "associate": "associate", "rename": "rename", "undo": "undo"}

var reviewColumns = []ochazuke.Column{{Title: "Decision", Width: 12}, {Title: "Question", Min: 18}, {Title: "Agent", Max: 15, Priority: 1}, {Title: "Age", Width: 7, Priority: 2}, {Title: "Replies", Width: 7, Priority: 2}}

type keyRow struct {
	key, verb string
	args      ochazuke.Arguments
}

func reviewKeys() []keyRow {
	var rows []keyRow
	for i := 1; i <= 9; i++ {
		rows = append(rows, keyRow{strconv.Itoa(i), "review.decide", ochazuke.Arguments{"choice": i}})
	}
	return append(rows,
		keyRow{"a", "review.accept", nil},
		keyRow{"?", "review.ask-back", nil},
		keyRow{"n", "review.decision-note", nil},
		keyRow{"x", "review.undecide", nil},
		keyRow{"o", "ochazuke.setting.cycle", ochazuke.Arguments{"name": "review.show"}},
		keyRow{"v", "review.view", nil},
		keyRow{"enter", "review.view", nil},
		keyRow{"y", "review.copy", ochazuke.Arguments{"yank": true}},
		keyRow{"t", "review.tag", nil},
		keyRow{"g x", "review.open", nil},
		keyRow{"d d", "review.delete", nil},
		keyRow{"u", "review.undo", nil},
	)
}

func registerReviewUI(a *App) {
	b := a.r.Define(reviewNoun).Columns(reviewColumns...).Mode("normal")
	for _, k := range reviewKeys() {
		row := ochazuke.Do(k.verb, k.args)
		if k.verb == "review.decide" {
			if k.key == "1" {
				row = row.Hinted("decide")
			} else {
				row = row.Hide()
			}
		}
		if k.verb == "review.view" {
			row = row.Guard("question-thread-enabled")
		}
		b.Bind(k.key, row)
	}
}

// firstQuestion is where the Review page starts: its first question, not
// the topic row above it.
func firstQuestion(rows []ocli.Row) string {
	for _, r := range rows {
		if r.Parent != "" {
			return r.ID
		}
	}
	return ""
}
