package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
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

// decisionTarget is the target parameter of a verb on a question for the
// owner: the picker lists the decisions (not topic headers), each by its
// title; it opens on the selected row, which an omitted target uses. Free,
// because a host passes the focused row's id, which may be a checklist item's.
func decisionTarget() ochazuke.Parameter {
	return ochazuke.Parameter{Type: "string", Description: "Question title, number or ID prefix; omitted uses the selected row", Noun: decisionNoun, Focus: true, Free: true, Source: "decision.list", SourceFixed: map[string]any{"flat": true}}
}

func (a *App) decisionSpecs() []verbSpec {
	// The target picker lists the decisions (not topic headers), each by its
	// title; it opens on the selected row, which an omitted target uses.
	target := decisionTarget()
	arr := func(d string) ochazuke.Parameter { return ochazuke.Parameter{Type: "array", Description: d} }
	pose := map[string]ochazuke.Parameter{
		"question": {Type: "string", Required: true, Rest: true, Description: "The question for the owner"},
		"title":    param("Short title"), "group": param("Topic; default the source file"),
		"source": param("Where it comes from, for example plan.md:42"), "details": param("Context the owner needs to decide"),
		"recommendation": param("Why you recommend it; shown unconfirmed until accepted"), "recommended": param("Id of the option you recommend (never implied)"),
		"options": arr("What the owner picks from: each a label (or {id, label, description}) in the owner's own words; give them, there are no defaults"),
		"items":   arr("A checklist: each a label (or {id, label, description, recommended}); the owner checks what to do"),
		"project": param("Project ID"), "cwd": param("Project directory"), "task": param("Task ID"),
		"tags": arr("Values to append"), "links": arr("Values to append"),
		"key": param("Stable id; posing it again updates the question and keeps the decision"),
	}
	return []verbSpec{
		{"pose", "Record a question for the owner, with your recommendation", pose, []string{"question"}, false, runPose, nil},
		{"queue", "List questions for the owner, grouped by topic", map[string]ochazuke.Parameter{"show": {Type: "string", Enum: decisionFilters, Description: "Decision state shown; default decision.show"}, "group": param("Topic"), "flat": {Type: "boolean", Description: "Only the questions, no topic header rows (what a picker lists)"}}, nil, true, runQueue, nil},
		{"decide", "Decide a question for the owner (owner)", map[string]ochazuke.Parameter{"target": target, "action": param("An option id or an outcome id (not needed with item, confirm or a note)"), "option": param("An option id"), "choice": {Type: "integer", Description: "Numbered option, 1 first"}, "item": param("A checklist item id: set its mark instead of the decision"), "checked": {Type: "boolean", Description: "With item: checked or not (default: flip it)"}, "confirm": {Type: "boolean", Description: "Decide the checklist as it stands: unchecked means do not"}, "pick_note": param("With item: a note on that item"), "note": param("Note for the agent")}, []string{"target"}, false, decisionVerb(decide), nil},
		{"accept", "Accept the recommendation: the recommended option, and check the recommended items (owner)", map[string]ochazuke.Parameter{"target": target}, []string{"target"}, false, decisionVerb(accept), nil},
		{"undecide", "Clear the decision, keeping its note; with item, clear that item's mark (owner)", map[string]ochazuke.Parameter{"target": target, "item": param("A checklist item id")}, []string{"target"}, false, decisionVerb(undecide), nil},
		{"ask-back", "Ask the agent before deciding (owner)", map[string]ochazuke.Parameter{"target": target, "text": prose("Your question")}, []string{"target", "text"}, false, decisionVerb(askBack), nil},
		{"decision-note", "Set the note on a decision (owner)", map[string]ochazuke.Parameter{"target": target, "note": prose("Note for the agent")}, []string{"target", "note"}, false, decisionVerb(decisionNote), nil},
		{"reply", "Answer the owner's ask on a question for the owner", map[string]ochazuke.Parameter{"target": target, "text": prose("Your answer"), "recommendation": param("Replace the recommendation"), "recommended": param("Replace the recommended option (an option id)"), "question": param("The owner's question, when the thread lacks it")}, []string{"target", "text"}, false, decisionVerb(reply), nil},
		{"applied", "Record the result of applying a decision; done locks it", map[string]ochazuke.Parameter{"target": target, "item": param("A checked checklist item id: apply that item alone"), "outcome": {Type: "string", Required: true, Enum: []string{"done", "failed"}, Description: "done or failed"}, "result": prose("What changed and how it was checked, or the failure")}, []string{"target", "outcome", "result"}, false, decisionVerb(applied), nil},
		{"decision-export", "The owner's decisions, asks and undecided items, for the agent", map[string]ochazuke.Parameter{"group": param("Topic"), "all": {Type: "boolean", Description: "Include applied decisions"}, "render": {Type: "string", Enum: []string{"json", "plan"}, Description: "json (default) or plan: Owner decisions sections for plan-imported items"}, "date": param("Date for plan sections (YYYY-MM-DD)")}, nil, false, runDecisionExport, nil},
		{"archive-matching", "Archive the questions in a decision state, a group and/or older than N days: lists them unless apply is set; one undo brings them all back", map[string]ochazuke.Parameter{"show": {Type: "string", Enum: decisionFilters, Description: "Decision state (default applied)"}, "group": param("Topic; default any"), "days": {Type: "integer", Description: "Only those last changed more than this many days ago (default 0: any)"}, "apply": {Type: "boolean", Description: "Archive them; without it nothing changes and the list is returned"}}, nil, false, runArchiveMatching, nil},
		{"archive-group", "Archive every question of a topic (the focused topic row's, else --group); one undo brings them back", map[string]ochazuke.Parameter{"target": target, "group": param("Topic; default the focused topic row's"), "apply": {Type: "boolean", Description: "Archive; without it only lists (default from the keys: true)"}}, nil, false, runArchiveGroup, nil},
		{"import", "Pose the open owner questions of a plans folder", map[string]ochazuke.Parameter{"root": param("Plans folder; default decision.plans-root"), "include_answered": {Type: "boolean", Description: "Also pose questions the plan already answers"}}, []string{"root"}, false, runImport, nil},
	}
}

func runArchiveMatching(a *App, c ochazuke.Ctx) ochazuke.Result {
	show := argStr(c, "show")
	if show == "" {
		show = classApplied
	}
	apply, _ := c.Args["apply"].(bool)
	v, e := a.store.ArchiveMatching(show, argStr(c, "group"), intArg(c.Args["days"]), apply)
	a.refresh()
	return result(v, e)
}

// runArchiveGroup archives a topic: --group, else the topic row the verb runs
// on (its target is topic/NAME).
func runArchiveGroup(a *App, c ochazuke.Ctx) ochazuke.Result {
	group := argStr(c, "group")
	if group == "" {
		group = parseRow(argStr(c, "target")).Topic
	}
	if group == "" {
		return result(nil, errors.New("name a topic with --group, or run it on a topic row"))
	}
	apply := true
	if v, ok := c.Args["apply"]; ok {
		apply = truthy(v)
	}
	v, e := a.store.ArchiveMatching("all", group, 0, apply)
	a.refresh()
	return result(v, e)
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

// declareDecisionItemVerbs defines the verbs that belong to the decision
// noun alone (no question verb of the same name): the outcome that takes text
// and the checklist item's mark. They are real operations of the noun, so a
// visual selection fans them out (collection.apply finds no alias).
func declareDecisionItemVerbs(a *App) {
	rows := []verbSpec{
		{"write-in", "Decide with an outcome that takes your text: write your own answer, or ask the agent (owner)", map[string]ochazuke.Parameter{"target": decisionTarget(), "action": required("An outcome id that takes text (see decision.outcomes)"), "text": prose("Your text")}, nil, false, decisionVerb(writeIn), nil},
		{"toggle", "Check or uncheck a checklist item: checked means do it; omit checked to flip it (owner)", map[string]ochazuke.Parameter{"target": decisionTarget(), "item": param("Checklist item id; omitted uses the selected item row (a target of item/QUESTION/ITEM names both)"), "checked": {Type: "boolean", Description: "Checked or not (default: flip it)"}}, nil, false, decisionVerb(checkItem), nil},
	}
	for _, v := range rows {
		v := v
		a.r.Define(decisionNoun).Verb(v.name, func(c ochazuke.Ctx) ochazuke.Result { return a.invoke(v, c) }, v.description).Agent(ochazuke.AgentAllow)
		op := a.r.Operations.At(ochazuke.Key(decisionNoun + "." + v.name))
		op.Parameters = v.params
		a.r.Operations.Set(op)
	}
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
	d := DecisionChange{Action: argStr(c, "action"), Option: argStr(c, "option"), Item: argStr(c, "item"), Confirm: truthy(c.Args["confirm"])}
	d.Choice = intArg(c.Args["choice"])
	if v, ok := c.Args["checked"]; ok {
		b := truthy(v)
		d.Checked = &b
	}
	if _, ok := c.Args["pick_note"]; ok {
		n := argStr(c, "pick_note")
		d.PickNote = &n
	}
	if _, ok := c.Args["note"]; ok {
		n := argStr(c, "note")
		d.Note = &n
	}
	return a.store.Decide(id, d)
}

// writeIn decides with an outcome whose row asks for text, carrying the text
// as the note (the outcome's own flags say whether the agent is asked).
func writeIn(a *App, id string, c ochazuke.Ctx) (Question, error) {
	text := argStr(c, "text")
	return a.store.Decide(id, DecisionChange{Action: argStr(c, "action"), Note: &text})
}

func checkItem(a *App, id string, c ochazuke.Ctx) (Question, error) {
	d := DecisionChange{Item: itemArg(c)}
	if v, ok := c.Args["checked"]; ok {
		b := truthy(v)
		d.Checked = &b
	}
	return a.store.Decide(id, d)
}

func accept(a *App, id string, _ ochazuke.Ctx) (Question, error) {
	return a.store.Decide(id, DecisionChange{Accept: true})
}

func undecide(a *App, id string, c ochazuke.Ctx) (Question, error) {
	return a.store.Decide(id, DecisionChange{Clear: true, Item: itemArg(c)})
}

func askBack(a *App, id string, c ochazuke.Ctx) (Question, error) {
	text := argStr(c, "text")
	out, ok := a.store.replyOutcome()
	if !ok {
		return Question{}, errors.New("no outcome asks the agent; add a row with the reply flag to decision.outcomes")
	}
	return a.store.Decide(id, DecisionChange{Action: out.ID, Note: &text})
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
	if _, ok := c.Args["recommended"]; ok {
		s := argStr(c, "recommended")
		rp.Recommended = &s
	}
	return a.store.Reply(id, rp)
}

func applied(a *App, id string, c ochazuke.Ctx) (Question, error) {
	return a.store.Applied(id, argStr(c, "item"), argStr(c, "outcome"), argStr(c, "result"))
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

// decisionCell is the Status column of a decision row: the pick's label, the
// unconfirmed recommendation ("Push?"), a checklist's progress ("2/5"), the
// outcome's label, or applied, with "!" when the context changed or the agent
// replied since the owner last touched it.
func (s *Store) decisionCell(q Question) string {
	r := q.Decision
	cell := ""
	class := s.decisionClass(q)
	cell = s.answerLabel(r)
	if r.Action == "" {
		cell = r.progress()
	}
	if r.locked() {
		cell = "✓ " + cell
	}
	if class == classUndecided {
		cell = "·"
		if rec := s.recommendedLabel(r); rec != "" {
			cell = rec + "?"
		}
		if p := r.progress(); p != "" {
			cell = p
		}
	}
	if r.Status == decisionFailed {
		cell += " ✗"
	}
	if r.ContextChanged || r.NewReply {
		cell = "! " + cell
	}
	return cell
}

// recommendedLabel is the recommended option's label; empty when none.
func (s *Store) recommendedLabel(r *Decision) string {
	id := r.recommendedID()
	for _, o := range r.rawOptions() {
		if o.ID == id && id != "" {
			return o.label()
		}
	}
	return ""
}

// recommendationView is the recommendation as the detail shows it: marked
// unconfirmed until the owner accepts it or decides otherwise.
func (s *Store) recommendationView(q Question) string {
	r := q.Decision
	rec := s.recommendedLabel(r)
	if rec == "" && r.Recommendation == "" {
		return ""
	}
	state := "recommended · not confirmed"
	switch {
	case accepted(r):
		state = "recommended · accepted"
	case r.Action != "" && r.recommendedID() != "":
		state = "recommended · you chose " + s.answerLabel(r)
	}
	if rec == "" {
		return r.Recommendation
	}
	return fmt.Sprintf("**%s** (%s)\n\n%s", rec, state, r.Recommendation)
}

// choicesView is the Options section: the numbered options with their
// descriptions, the checklist, then the outcomes with their keys. With
// decision.labels off it is the compact line (1 id · 2 id · Later ...).
func (s *Store) choicesView(q Question, labels bool) string {
	r := q.Decision
	if !labels {
		return s.compactChoices(r)
	}
	var out []string
	rec := r.recommendedID()
	for i, o := range s.optionsOf(r) {
		if i >= 9 {
			break
		}
		line := fmt.Sprintf("%d. %s", i+1, o.label())
		if r.Action != "" && r.Action == o.ID {
			line += "  ✓ chosen"
			if rec != "" && o.ID != rec {
				line += "  " + differsMark + " differs"
			}
		}
		if o.ID == rec && rec != "" {
			line += "  (" + s.recommendedState(r) + ")"
		}
		out = append(out, line)
		if o.Description != "" {
			out = append(out, "", "    "+strings.ReplaceAll(strings.TrimSpace(o.Description), "\n", "\n    "), "")
		}
	}
	if len(r.Items) > 0 {
		out = append(out, "", "Checklist "+r.progress()+" (checked means do it)", "")
		for _, it := range r.Items {
			line := fmt.Sprintf("%s %s", itemGlyph(r, it.ID), it.label())
			if it.Recommended {
				line += "  (recommended)"
			}
			if r.itemDiffers(it) {
				line += "  " + differsMark + " differs"
			}
			if it.Description != "" {
				line += "  \n" + strings.ReplaceAll(strings.TrimSpace(it.Description), "\n", "  \n")
			}
			out = append(out, line, "")
		}
	}
	var outcomes []string
	for _, o := range s.outcomes() {
		if o.Key != "" {
			outcomes = append(outcomes, o.Key+" "+o.Label)
		} else {
			outcomes = append(outcomes, o.Label)
		}
	}
	if len(outcomes) > 0 {
		out = append(out, "", strings.Join(outcomes, " · "))
	}
	return strings.Join(out, "\n")
}

func (s *Store) compactChoices(r *Decision) string {
	var out []string
	for i, o := range s.optionsOf(r) {
		if i >= 9 {
			break
		}
		out = append(out, fmt.Sprintf("%d %s", i+1, o.label()))
	}
	for _, o := range s.outcomes() {
		out = append(out, o.Label)
	}
	return strings.Join(out, " · ")
}

// itemGlyph is a checklist item's checkbox, drawn as the outline draws a task.
func itemGlyph(r *Decision, item string) string {
	if r.checked(item) {
		return "[x]"
	}
	return "[ ]"
}

// recommendedState says whether the recommendation is confirmed.
func (s *Store) recommendedState(r *Decision) string {
	switch {
	case accepted(r):
		return "recommended · accepted"
	case r.Action != "":
		return "recommended · not chosen"
	}
	return "recommended · not confirmed"
}

// itemRow is a checklist item as a child row of its question: the same
// detail as the question, with the item's checkbox for a cell.
func (a *App) itemRow(q Question, it Item, data map[string]any, search string, colored bool) ocli.Row {
	r := q.Decision
	d := map[string]any{}
	for k, v := range data {
		d[k] = v
	}
	d["item"] = it.ID
	d["checked"] = r.checked(it.ID)
	d["item_label"] = it.label()
	cell := ""
	if i := r.pickAt(it.ID); i >= 0 && isApplied(r.Picks[i].Status) {
		cell = "✓ applied"
	}
	row := ocli.Row{Parent: "question/" + q.ID, Noun: decisionNoun, ID: itemRowPrefix + q.ID + "/" + it.ID, Label: it.label(), Data: d, Cells: []string{cell, itemGlyph(r, it.ID) + " " + it.label(), "", "", "", search}}
	if colored && r.recommendsItems() {
		// A mark is toned by whether it matches the agent's recommendation:
		// differing blue, agreeing green (once the checklist is decided).
		switch {
		case r.itemDiffers(it):
			row.Cells[0] = strings.TrimSpace(differsMark + " " + cell)
			row.Tones = stateTones(stateDiffers, 0, 1)
		case r.decided():
			row.Tones = stateTones(stateAnswered, 1)
		}
	}
	return row
}

// itemRowPrefix starts a checklist item row's id: item/<question id>/<item id>.
const itemRowPrefix = "item/"

// questionRowPrefix starts a question row's id: question/<question id>.
const questionRowPrefix = "question/"

// topicRowPrefix starts a topic header row's id: topic/<group>.
const topicRowPrefix = "topic/"

// rowRef is what a row id names: the topic, question and checklist item. A
// host that mounts the page hands over the list's own id in front
// (decision/questions/question/Q, ext/NAME/.../item/Q/I), so the id is read
// at its last known segment, at the start or after a "/".
type rowRef struct{ Topic, Question, Item string }

// rowKinds are the row id segments and how each fills a rowRef.
var rowKinds = []struct {
	prefix string
	fill   func(r *rowRef, rest string)
}{
	{itemRowPrefix, func(r *rowRef, rest string) { r.Question, r.Item, _ = strings.Cut(rest, "/") }},
	{questionRowPrefix, func(r *rowRef, rest string) { r.Question = rest }},
	{topicRowPrefix, func(r *rowRef, rest string) { r.Topic = rest }},
}

func parseRow(id string) rowRef {
	best, at := -1, 0
	for n, k := range rowKinds {
		for i := strings.LastIndex(id, k.prefix); i >= 0; i = strings.LastIndex(id[:i], k.prefix) {
			if i == 0 || id[i-1] == '/' {
				if i > best {
					best, at = i, n
				}
				break
			}
		}
	}
	var ref rowRef
	if best >= 0 {
		rowKinds[at].fill(&ref, id[best+len(rowKinds[at].prefix):])
	}
	return ref
}

// itemArg is the checklist item a verb acts on: the item argument, else the
// item row named by the target (a host passes the focused row's id) or the
// item row the verb runs on (each row of a selection in turn).
func itemArg(c ochazuke.Ctx) string {
	if id := argStr(c, "item"); id != "" {
		return id
	}
	rows := []string{argStr(c, "target")}
	if c.Instance != nil {
		if row, ok := c.Instance.Data.(string); ok {
			rows = append(rows, row)
		}
	}
	for _, row := range rows {
		if item := parseRow(row).Item; item != "" {
			return item
		}
	}
	return ""
}

// itemRowQuestion is the question id of a question or item row id (as the
// page or a host names it), else id unchanged.
func itemRowQuestion(id string) string {
	if q := parseRow(id).Question; q != "" {
		return q
	}
	return id
}

// decisionRows answer the Decisions page and `question queue`: a tree with one
// parent row per topic (its counts in the label) and the topic's questions.
type topicCounts struct{ n, decided, asked int }

// topicRow is a topic header: its counts in the label, the questions and the
// legend in its detail, and (document detail) its summary file.
func (a *App) topicRow(g string, t *topicCounts, lines []string, colored, docs bool, keep map[string]bool) ocli.Row {
	label := fmt.Sprintf("%s  %d/%d decided", g, t.decided, t.n)
	if t.asked > 0 {
		label += fmt.Sprintf(" · %d asked", t.asked)
	}
	body := "## " + label + "\n\n" + strings.Join(lines, "\n")
	row := ocli.Row{Noun: decisionNoun, ID: "topic/" + g, Label: label, Cells: []string{"", label, "", "", "", g}}
	if colored {
		body += "\n\n" + legend()
		if t.n > t.decided+t.asked {
			row.Tones = stateTones(stateUndecided, 1)
		}
	}
	data := map[string]any{"topic": g, "question": body}
	row.Data = data
	if docs {
		docTopicPath(data, g, body, keep)
	}
	return row
}

// questionData is a decision row's record: the question row data and the
// views the detail sections read.
func (a *App) questionData(q Question, g string) map[string]any {
	st, r := a.store, q.Decision
	data := questionRowData(q)
	data["project_name"] = st.projectDisplay(q.Project)
	data["question"] = q.Text
	data["topic"] = g
	data["decision_view"] = st.decisionCell(q)
	data["recommendation_view"] = st.recommendationView(q)
	data["choices"] = st.choicesView(q, a.r.Settings().Bool("decision.labels"))
	data["details_view"] = r.Details
	data["thread"] = thread(q, false)
	fields := []string{q.Title, q.Text, r.Details, r.Recommendation, r.Note, g}
	for _, it := range r.Items {
		fields = append(fields, it.label())
	}
	data["search"] = strings.Join(fields, " ")
	return data
}

func (a *App) decisionRows(args ochazuke.Arguments) ([]ocli.Row, error) {
	st := a.store
	show := ochazuke.StringArg(args, "show", "")
	if show == "" {
		show = a.r.Settings().String("decision.show")
	}
	docs := a.docsOn()
	if docs {
		if e := st.docSyncAll(); e != nil {
			a.r.UI().Status("not applied from the document: " + e.Error())
		}
	}
	// Archived questions stay hidden unless the filter asks for them or
	// collection.hide-archived is off (the setting every list shares).
	qs, e := st.ownerQuestionsWith(filterOf(show).archived || !a.r.Settings().Bool("collection.hide-archived"))
	if e != nil {
		return nil, e
	}
	colored := a.colored()
	keep := map[string]bool{}
	group := ochazuke.StringArg(args, "group", "")
	flat, _ := args["flat"].(bool)
	qs = st.orderQuestions(qs, a.r.Settings().String("decision.sort"))
	tally := map[string]*topicCounts{}
	for _, q := range qs {
		g := groupOf(q)
		if tally[g] == nil {
			tally[g] = &topicCounts{}
		}
		tally[g].n++
		switch st.decisionClass(q) {
		case classUndecided:
		case classAsked:
			tally[g].asked++
		default:
			tally[g].decided++
		}
	}
	lines := map[string][]string{}
	for _, q := range qs {
		if g := groupOf(q); (group == "" || g == group) && st.decisionShown(q, show) {
			lines[g] = append(lines[g], fmt.Sprintf("- %s · %s", st.decisionCell(q), q.Title))
		}
	}
	out := []ocli.Row{}
	last := ""
	for _, q := range qs {
		g := groupOf(q)
		if (group != "" && g != group) || !st.decisionShown(q, show) {
			continue
		}
		if g != last && !flat {
			last = g
			out = append(out, a.topicRow(g, tally[g], lines[g], colored, docs, keep))
		}
		data := a.questionData(q, g)
		r := q.Decision
		search, _ := data["search"].(string)
		replies := len(q.Entries)
		if docs {
			docRowPath(st, q, data, keep)
		}
		data["decision_state"] = st.decisionState(q)
		data["decision_vs"] = st.versus(q)
		cell := st.decisionCell(q)
		row := ocli.Row{Parent: parentOf(g, flat), Noun: decisionNoun, ID: "question/" + q.ID, Label: q.Title, Data: data, Cells: []string{cell, q.Title, q.RecordedBy.Name, age(q.AskedAt, a.store.now()), strconv.Itoa(replies), search}}
		if colored {
			var state string
			row.Cells[0], state = st.markedCell(q)
			row.Tones = stateTones(state, 0, 1)
		}
		out = append(out, row)
		if !flat {
			for _, it := range r.Items {
				out = append(out, a.itemRow(q, it, data, search, colored))
			}
		}
	}
	if docs {
		docPrune(keep)
	}
	return out, nil
}

// docTopicPath writes a topic row's summary document and names it in data.
func docTopicPath(data map[string]any, group, body string, keep map[string]bool) {
	p := docTopicFile(group)
	if e := os.MkdirAll(docDir(), 0o700); e == nil && writeFileAtomic(p, []byte(docTopicText(body))) == nil {
		data["path"] = p
	}
	keep[strings.TrimSuffix(filepath.Base(p), ".md")] = true
}

// docRowPath renders a question's working file and names it in data.
func docRowPath(st *Store, q Question, data map[string]any, keep map[string]bool) {
	if e := st.docWrite(q); e == nil {
		data["path"] = docFile(q.ID)
		keep[q.ID] = true
	}
}

// docsOn: the owner's page draws the document detail (setting decision.detail)
// and files are written for the owner only: an agent listing the queue
// leaves no working files behind.
func (a *App) docsOn() bool {
	return a.store.Actor.Kind == "owner" && a.r.Settings().String("decision.detail") == "document"
}

// parentOf is a decision row's parent topic row; a flat answer has none.
func parentOf(group string, flat bool) string {
	if flat {
		return ""
	}
	return "topic/" + group
}

func decisionPage(document bool) ocli.CatalogPage {
	p := ocli.CatalogPage{ID: decisionPageID, Title: decisionPageTitle, Layout: ocli.LayoutSplit,
		Lists: []ocli.CatalogList{{ID: decisionListID, Noun: decisionNoun, Source: "decision.list", Title: "For you", Tree: true}},
		Detail: &ocli.CatalogDetail{
			Fields: []string{"question", "recommendation_view", "choices", "details_view", "topic", "decision_view"},
			Sections: []ocli.CatalogDetailSection{
				{Kind: "markdown", Field: "question", Heading: true},
				{Kind: "markdown", Title: "Recommendation", Field: "recommendation_view"},
				{Kind: "markdown", Title: "Options", Field: "choices"},
				{Kind: "thread", Title: "Notes, asks and replies", Field: "entries", Author: "by.kind", At: "at", Body: "text"},
				{Kind: "markdown", Title: "Context", Field: "details_view"},
				{Kind: "fields", Title: "Decision", Region: "side", Fields: []ocli.CatalogDetailField{
					{Label: "Topic", Field: "topic", Format: "text"},
					{Label: "Decision", Field: "decision_view", Format: "badge"},
					{Label: "Versus", Field: "decision_vs", Format: "text"},
					{Label: "Note", Field: "decision.note", Format: "text"},
					{Label: "Recommended", Field: "decision.recommended", Format: "text"},
					{Label: "Changed", Field: "decision.context_changed", Format: "text"},
					{Label: "New reply", Field: "decision.new_reply", Format: "text"},
					{Label: "Applied", Field: "decision.status", Format: "badge"},
					{Label: "Result", Field: "decision.result", Format: "text"},
					{Label: "Source", Field: "decision.source", Format: "path"},
					{Label: "Agent", Field: "recorded_by.name", Format: "text"},
					{Label: "Asked", Field: "asked_at", Format: "relative-time"},
					{Label: "Project", Field: "project_name", Format: "text"},
				}},
			},
		}}
	if document {
		p.Detail = &ocli.CatalogDetail{Kind: "markdown", PathField: "path"}
	}
	return p
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
var decisionAliases = map[string]string{"decide": "decide", "accept": "accept", "undecide": "undecide", "ask-back": "ask-back", "decision-note": "decision-note", "reply": "reply", "applied": "applied", "show": "show", "yank": "yank", "tag": "tag", "untag": "untag", "link": "link", "open": "open", "star": "star", "unstar": "unstar", "delete": "delete", "archive": "archive", "archive-matching": "archive-matching", "archive-group": "archive-group", "restore": "restore", "associate": "associate", "rename": "rename", "undo": "undo"}

var decisionColumns = []ochazuke.Column{{Title: "Decision", Width: 12}, {Title: "Question", Min: 18}, {Title: "Agent", Max: 15, Priority: 1}, {Title: "Age", Width: 7, Priority: 2}, {Title: "Thread", Width: 7, Priority: 2}}

type keyRow struct {
	key, verb string
	args      ochazuke.Arguments
	hint      string // footer hint text
	hide      bool   // left out of the footer hints
	guard     string // guard the row waits on
	ownTarget bool   // the row names its question (the verb's form then asks only for its text)
}

func plainKey(key, verb string, args ochazuke.Arguments) keyRow {
	return keyRow{key: key, verb: verb, args: args}
}

// decisionKeys are the page's key rows. The numbers pick the Nth option; the
// outcome keys come from the rows of decision.outcomes (a row with a key is a
// binding to decision.decide, or to decision.write-in when the outcome takes
// text); the rest are fixed rows. The ask-back verb stays for the palette and
// for a bindings.json that binds it.
func (a *App) decisionKeys() []keyRow {
	var rows []keyRow
	for i := 1; i <= 9; i++ {
		row := plainKey(strconv.Itoa(i), "decision.decide", ochazuke.Arguments{"choice": i})
		if i == 1 {
			row.hint = "decide"
		} else {
			row.hide = true
		}
		rows = append(rows, row)
	}
	for _, o := range a.outcomeTable() {
		if o.Key == "" {
			continue
		}
		row := plainKey(o.Key, "decision.decide", ochazuke.Arguments{"action": o.ID})
		// A topic header is no question: its row keeps the library's key (l
		// expands it). A row guard is data, so a host mounting the page
		// (mtm) judges it on its rows too.
		row.guard, row.hide = questionRowGuard, true
		if o.NeedsNote {
			row.verb, row.ownTarget = "decision.write-in", true
		}
		rows = append(rows, row)
	}
	check := plainKey("c", "decision.toggle", nil)
	check.hint = "check"
	thread := func(key string) keyRow {
		row := plainKey(key, "decision.show", nil)
		row.guard = "question-thread-enabled"
		return row
	}
	return append(rows,
		plainKey("a", "decision.accept", nil),
		plainKey("D", "decision.decide", ochazuke.Arguments{"confirm": true}),
		plainKey("n", "decision.decision-note", nil),
		plainKey("x", "decision.undecide", nil),
		check,
		plainKey("o", "ochazuke.setting.cycle", ochazuke.Arguments{"name": "decision.show"}),
		thread("v"), thread("enter"),
		plainKey("y", "decision.yank", nil),
		plainKey("t", "decision.tag", nil),
		plainKey("g x", "decision.open", nil),
		deleteKey(),
		plainKey("u", "decision.undo", nil),
		archiveKey("d a", "decision.archive", questionRowGuard),
		archiveKey("d a", "decision.archive-group", topicRowGuard),
		plainKey("d r", "decision.restore", nil),
		plainKey("g A", "decision.archive-matching", ochazuke.Arguments{"show": classApplied, "days": 30}),
		plainKey("g Z", "decision.archive-matching", ochazuke.Arguments{"show": classApplied, "days": 30, "apply": true}),
		plainKey("O", "ochazuke.setting.cycle", ochazuke.Arguments{"name": "decision.show", "step": -1}),
	)
}

// outcomeTable is the outcome table the page binds keys from: the setting
// (a table that does not parse falls back to the default, as the store does).
func (a *App) outcomeTable() []Outcome {
	if out, e := parseOutcomes(a.r.Settings().Strings("decision.outcomes")); e == nil {
		return out
	}
	return defaultOutcomes
}

// questionRowGuard holds on a question or checklist item row, not on a
// topic header: the rows whose record names a question (asked_at; a topic
// row's record is its topic and summary only). It is a row guard, not one
// registered with DefineGuard, so mounted pages keep it.
const questionRowGuard = ochazuke.RowGuardPrefix + "asked_at"

// topicRowGuard holds on a topic header row only (its record has no asked_at).
const topicRowGuard = ochazuke.RowGuardPrefix + "!asked_at"

func archiveKey(key, verb, guard string) keyRow {
	row := plainKey(key, verb, nil)
	row.guard = guard
	return row
}

// deleteKey is d d: delete the question, on its own row only. On a
// checklist item row it would delete the whole question, so it is off there
// (row:!item: an item row's record carries item).
func deleteKey() keyRow {
	row := plainKey("d d", "decision.delete", nil)
	row.guard = ochazuke.RowGuardPrefix + "!item"
	return row
}

func registerDecisionUI(a *App) {
	b := a.r.Define(decisionNoun).Columns(decisionColumns...).Mode("normal")
	for _, k := range a.decisionKeys() {
		row := ochazuke.Do(k.verb, k.args)
		if k.hint != "" {
			row = row.Hinted(k.hint)
		}
		if k.hide {
			row = row.Hide()
		}
		if k.ownTarget {
			// The form asks for the text only: the row names its question.
			row = row.FromRow("target", "id")
		}
		if k.guard != "" {
			row = row.Guard(k.guard)
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
