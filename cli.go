package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

var errUsage = errors.New("usage")

type parsedArgs struct {
	pos   []string
	flags map[string]string
	bools map[string]bool
}

func (a parsedArgs) has(k string) bool { return a.bools[k] }

func parseArgs(argv []string, boolFlags ...string) (parsedArgs, error) {
	res := parsedArgs{flags: map[string]string{}, bools: map[string]bool{}}
	isBool := func(k string) bool { return slices.Contains(boolFlags, k) }
	for i := 0; i < len(argv); i++ {
		arg := argv[i]
		if arg == "--" {
			res.pos = append(res.pos, argv[i+1:]...)
			break
		}
		if strings.HasPrefix(arg, "--") && len(arg) > 2 {
			body := arg[2:]
			if k, v, ok := strings.Cut(body, "="); ok {
				if isBool(k) {
					res.bools[k] = v != "false" && v != "0"
				} else {
					res.flags[k] = v
				}
				continue
			}
			if isBool(body) {
				res.bools[body] = true
				continue
			}
			if i+1 >= len(argv) {
				return res, fmt.Errorf("--%s needs a value", body)
			}
			i++
			res.flags[body] = argv[i]
			continue
		}
		if strings.HasPrefix(arg, "-") && len(arg) == 2 {
			k := arg[1:]
			if isBool(k) {
				res.bools[k] = true
				continue
			}
		}
		res.pos = append(res.pos, arg)
	}
	return res, nil
}

// splitDataArgs removes --data DIR (before or after the subcommand).
func splitDataArgs(argv []string) ([]string, string, error) {
	out := make([]string, 0, len(argv))
	data := ""
	for i := 0; i < len(argv); i++ {
		if argv[i] == "--" {
			out = append(out, argv[i:]...)
			break
		}
		if argv[i] == "--data" {
			i++
			if i >= len(argv) {
				return nil, "", errors.New("--data needs a directory")
			}
			data = argv[i]
		} else if value, ok := strings.CutPrefix(argv[i], "--data="); ok {
			data = value
		} else {
			out = append(out, argv[i])
		}
	}
	return out, data, nil
}

func resolveDataDir(flag string) (string, error) {
	if flag != "" {
		return filepath.Abs(flag)
	}
	if env := os.Getenv("O_QUESTIONS_DATA"); env != "" {
		return filepath.Abs(env)
	}
	base := os.Getenv("XDG_DATA_HOME")
	if base == "" {
		h, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		base = filepath.Join(h, ".local", "share")
	}
	return filepath.Abs(filepath.Join(base, "o-questions"))
}

func writeJSON(w io.Writer, v any) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	enc.SetEscapeHTML(false)
	return enc.Encode(v)
}

type cli struct {
	st     *Store
	stdin  io.Reader
	stdout io.Writer
}

var subcommands = map[string]struct {
	run   func(c *cli, a []string) error
	usage string
}{
	"ask":       {(*cli).ask, "o-questions ask [WORDS...] [--question TEXT] [--title T] [--project P] [--tags a,b] [--key K] [--json]"},
	"answer":    {(*cli).answer, "o-questions answer TARGET [TEXT...] [--answer TEXT] [--summary S] [--json]"},
	"list":      {(*cli).list, "o-questions list [--status S] [--project P] [--filter TEXT] [--archived] [--decisions] [--format table|json] [--json]"},
	"show":      {(*cli).show, "o-questions show TARGET [--provenance] [--json]"},
	"resolve":   {(*cli).resolve, "o-questions resolve TARGET [--moot] [--json]"},
	"close":     {(*cli).resolve, "o-questions close TARGET [--moot] [--json]   (= resolve)"},
	"reopen":    {(*cli).reopen, "o-questions reopen TARGET [--json]"},
	"undo":      {(*cli).undo, "o-questions undo [--json]"},
	"redo":      {(*cli).redo, "o-questions redo [--json]"},
	"decisions": {(*cli).decisions, "o-questions decisions [list|pose|decide|accept|reply|applied|export|import] ... [--json]"},
	"mcp":       {(*cli).mcp, "o-questions mcp   serve agent tools over stdio (JSON-RPC 2.0)"},
	"catalog":   {(*cli).catalog, "o-questions catalog   print the extension manifest JSON"},
	"query":     {(*cli).query, "o-questions query [TEXT] [--status S] [--project P] [--archived]   print {\"items\": [...], \"total\": n}"},
}

var subcommandOrder = []string{
	"ask", "answer", "list", "show", "resolve", "close", "reopen", "undo", "redo", "decisions", "mcp", "catalog", "query",
}

func usageText() string {
	var b strings.Builder
	b.WriteString("Usage:\n  o-questions [--data DIR]   interactive questions and decisions journal (TUI)\n")
	for _, name := range subcommandOrder {
		fmt.Fprintf(&b, "  %s\n", subcommands[name].usage)
	}
	b.WriteString(`  o-questions --version | --smoke | --help

Store lives in --data, else $O_QUESTIONS_DATA, else $XDG_DATA_HOME/o-questions,
else ~/.local/share/o-questions. Settings, bindings and themes load from
$XDG_CONFIG_HOME/o-questions (else ~/.config/o-questions).
In the TUI press ? for live key help, : for the command palette.`)
	return b.String()
}

func splitCSV(s string) []string {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	var out []string
	for _, p := range strings.Split(s, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

func (c *cli) ask(in []string) error {
	a, err := parseArgs(in, "json")
	if err != nil {
		return err
	}
	text := a.flags["question"]
	if text == "" {
		text = strings.Join(a.pos, " ")
	}
	if strings.TrimSpace(text) == "" {
		return fmt.Errorf("%w: question text required", errUsage)
	}
	cwd, _ := os.Getwd()
	if v := a.flags["cwd"]; v != "" {
		cwd = v
	}
	res, err := c.st.Ask(Ask{
		Question:   text,
		Title:      a.flags["title"],
		Project:    a.flags["project"],
		Cwd:        cwd,
		Task:       a.flags["task"],
		Tags:       splitCSV(a.flags["tags"]),
		FollowUpOf: a.flags["follow-up-of"],
		Key:        a.flags["key"],
		Source:     sourceFromEnv(c.st.Actor),
	})
	if err != nil {
		return err
	}
	if a.has("json") {
		return writeJSON(c.stdout, res)
	}
	fmt.Fprintf(c.stdout, "%s  %s\n", res.Question.ID, res.Question.Title)
	return nil
}

func (c *cli) answer(in []string) error {
	a, err := parseArgs(in, "json")
	if err != nil {
		return err
	}
	if len(a.pos) == 0 && a.flags["target"] == "" {
		return fmt.Errorf("%w: target required", errUsage)
	}
	target := a.flags["target"]
	rest := a.pos
	if target == "" {
		target = rest[0]
		rest = rest[1:]
	}
	text := a.flags["answer"]
	if text == "" {
		text = strings.Join(rest, " ")
	}
	if strings.TrimSpace(text) == "" {
		return fmt.Errorf("%w: answer text required", errUsage)
	}
	q, err := c.st.Append(target, Entry{
		Kind:    "answer",
		Text:    text,
		Summary: a.flags["summary"],
	})
	if err != nil {
		return err
	}
	if a.has("json") {
		return writeJSON(c.stdout, q)
	}
	fmt.Fprintf(c.stdout, "%s  [%s] %s\n", q.ID, q.Status, q.Title)
	return nil
}

func (c *cli) list(in []string) error {
	a, err := parseArgs(in, "archived", "all", "decisions", "json")
	if err != nil {
		return err
	}
	rows, err := c.st.records(a.has("archived") || a.has("all"))
	if err != nil {
		return err
	}
	status := a.flags["status"]
	proj := a.flags["project"]
	filter := strings.ToLower(a.flags["filter"])
	onlyDecisions := a.has("decisions")
	var out []Question
	for _, q := range rows {
		if onlyDecisions && !q.forOwner() {
			continue
		}
		if status != "" && q.Status != status {
			continue
		}
		if proj != "" && !strings.EqualFold(q.Project.ID, proj) && !strings.EqualFold(q.Project.Shown(), proj) {
			continue
		}
		if filter != "" && !strings.Contains(strings.ToLower(q.Title+"\x00"+q.Text+"\x00"+q.Project.Shown()), filter) {
			continue
		}
		out = append(out, q)
	}
	format := a.flags["format"]
	if a.has("json") {
		format = "json"
	}
	switch format {
	case "json":
		if out == nil {
			out = []Question{}
		}
		return writeJSON(c.stdout, out)
	case "", "table":
		for i, q := range out {
			st := q.Status
			if q.forOwner() {
				st = c.st.decisionState(q)
			}
			fmt.Fprintf(c.stdout, "%3d  %-18s  %-10s  %-14s  %s\n", i+1, q.ID, st, q.Project.Shown(), q.Title)
		}
		return nil
	default:
		return fmt.Errorf("unknown --format %q (table or json)", format)
	}
}

func (c *cli) show(in []string) error {
	a, err := parseArgs(in, "provenance", "json")
	if err != nil {
		return err
	}
	if len(a.pos) == 0 {
		return fmt.Errorf("%w: target required", errUsage)
	}
	q, err := c.st.Resolve(a.pos[0], false)
	if err != nil {
		return err
	}
	if a.has("json") {
		return writeJSON(c.stdout, q)
	}
	if q.forOwner() {
		_, err = io.WriteString(c.stdout, c.st.docRender(q))
		return err
	}
	_, err = io.WriteString(c.stdout, thread(q, a.has("provenance")))
	return err
}

func (c *cli) resolve(in []string) error {
	a, err := parseArgs(in, "moot", "json")
	if err != nil {
		return err
	}
	if len(a.pos) == 0 {
		return fmt.Errorf("%w: target required", errUsage)
	}
	var q Question
	if a.has("moot") {
		q, err = c.st.Change(opMoot, a.pos[0], Change{})
	} else {
		q, err = c.st.Change(opStatus, a.pos[0], Change{Value: "answered"})
	}
	if err != nil {
		return err
	}
	if a.has("json") {
		return writeJSON(c.stdout, q)
	}
	fmt.Fprintf(c.stdout, "%s  [%s] %s\n", q.ID, q.Status, q.Title)
	return nil
}

func (c *cli) reopen(in []string) error {
	a, err := parseArgs(in, "json")
	if err != nil {
		return err
	}
	if len(a.pos) == 0 {
		return fmt.Errorf("%w: target required", errUsage)
	}
	q, err := c.st.Change(opReopen, a.pos[0], Change{})
	if err != nil {
		return err
	}
	if a.has("json") {
		return writeJSON(c.stdout, q)
	}
	fmt.Fprintf(c.stdout, "%s  [%s] %s\n", q.ID, q.Status, q.Title)
	return nil
}

func (c *cli) undo(in []string) error {
	a, err := parseArgs(in, "json")
	if err != nil {
		return err
	}
	if err := c.st.Undo(); err != nil {
		return err
	}
	if a.has("json") {
		return writeJSON(c.stdout, map[string]any{"undone": true})
	}
	fmt.Fprintln(c.stdout, "undone")
	return nil
}

func (c *cli) redo(in []string) error {
	a, err := parseArgs(in, "json")
	if err != nil {
		return err
	}
	if err := c.st.Redo(); err != nil {
		return err
	}
	if a.has("json") {
		return writeJSON(c.stdout, map[string]any{"redone": true})
	}
	fmt.Fprintln(c.stdout, "redone")
	return nil
}

func (c *cli) decisions(in []string) error {
	sub := "list"
	rest := in
	if len(in) > 0 && !strings.HasPrefix(in[0], "-") {
		sub = in[0]
		rest = in[1:]
	}
	switch sub {
	case "list", "queue":
		a, err := parseArgs(rest, "archived", "all", "json")
		if err != nil {
			return err
		}
		qs, err := c.st.ownerQuestionsWith(a.has("archived") || a.has("all"))
		if err != nil {
			return err
		}
		grp := a.flags["group"]
		show := a.flags["show"]
		var out []Question
		for _, q := range c.st.orderQuestions(qs, "undecided-first") {
			if grp != "" && groupOf(q) != grp {
				continue
			}
			if show != "" && !c.st.decisionShown(q, show) {
				continue
			}
			out = append(out, q)
		}
		if a.has("json") {
			if out == nil {
				out = []Question{}
			}
			return writeJSON(c.stdout, out)
		}
		for i, q := range out {
			cell, _ := c.st.markedCell(q)
			fmt.Fprintf(c.stdout, "%3d  %-18s  %-14s  %-16s  %s\n", i+1, q.ID, groupOf(q), cell, q.Title)
		}
		return nil
	case "pose":
		a, err := parseArgs(rest, "json")
		if err != nil {
			return err
		}
		text := a.flags["question"]
		if text == "" {
			text = strings.Join(a.pos, " ")
		}
		var opts []Option
		for _, raw := range splitCSV(a.flags["options"]) {
			id, lbl, _ := strings.Cut(raw, ":")
			opts = append(opts, Option{ID: strings.TrimSpace(id), Label: strings.TrimSpace(lbl)})
		}
		var items []Item
		for _, raw := range splitCSV(a.flags["items"]) {
			id, lbl, _ := strings.Cut(raw, ":")
			items = append(items, Item{ID: strings.TrimSpace(id), Label: strings.TrimSpace(lbl)})
		}
		res, err := c.st.Pose(Pose{
			Question:       text,
			Title:          a.flags["title"],
			Group:          a.flags["group"],
			Source:         a.flags["source"],
			Details:        a.flags["details"],
			Recommendation: a.flags["recommendation"],
			Recommended:    a.flags["recommended"],
			Options:        opts,
			Items:          items,
			Project:        a.flags["project"],
			Tags:           splitCSV(a.flags["tags"]),
			Key:            a.flags["key"],
			Provenance:     sourceFromEnv(c.st.Actor),
		})
		if err != nil {
			return err
		}
		if a.has("json") {
			return writeJSON(c.stdout, res)
		}
		fmt.Fprintf(c.stdout, "%s  %s\n", res.Question.ID, res.Question.Title)
		return nil
	case "decide":
		a, err := parseArgs(rest, "accept", "clear", "confirm", "json")
		if err != nil {
			return err
		}
		target := a.flags["target"]
		pos := a.pos
		if target == "" && len(pos) > 0 {
			target = pos[0]
			pos = pos[1:]
		}
		if target == "" {
			return fmt.Errorf("%w: target required", errUsage)
		}
		ch := DecisionChange{
			Action:  a.flags["action"],
			Option:  a.flags["option"],
			Item:    a.flags["item"],
			Accept:  a.has("accept"),
			Clear:   a.has("clear"),
			Confirm: a.has("confirm"),
		}
		if ch.Action == "" && ch.Option == "" && len(pos) > 0 {
			ch.Action = pos[0]
		}
		if note, ok := a.flags["note"]; ok {
			ch.Note = &note
		}
		q, err := c.st.Decide(target, ch)
		if err != nil {
			return err
		}
		if a.has("json") {
			return writeJSON(c.stdout, q)
		}
		cell, _ := c.st.markedCell(q)
		fmt.Fprintf(c.stdout, "%s  %s  %s\n", q.ID, cell, q.Title)
		return nil
	case "accept":
		a, err := parseArgs(rest, "json")
		if err != nil {
			return err
		}
		if len(a.pos) == 0 {
			return fmt.Errorf("%w: target required", errUsage)
		}
		q, err := c.st.Decide(a.pos[0], DecisionChange{Accept: true})
		if err != nil {
			return err
		}
		if a.has("json") {
			return writeJSON(c.stdout, q)
		}
		cell, _ := c.st.markedCell(q)
		fmt.Fprintf(c.stdout, "%s  %s  %s\n", q.ID, cell, q.Title)
		return nil
	case "reply":
		a, err := parseArgs(rest, "json")
		if err != nil {
			return err
		}
		if len(a.pos) == 0 {
			return fmt.Errorf("%w: target required", errUsage)
		}
		text := a.flags["text"]
		if text == "" && len(a.pos) > 1 {
			text = strings.Join(a.pos[1:], " ")
		}
		rp := Reply{Text: text}
		if v, ok := a.flags["recommendation"]; ok {
			rp.Recommendation = &v
		}
		if v, ok := a.flags["recommended"]; ok {
			rp.Recommended = &v
		}
		if v, ok := a.flags["question"]; ok {
			rp.Question = v
		}
		q, err := c.st.Reply(a.pos[0], rp)
		if err != nil {
			return err
		}
		if a.has("json") {
			return writeJSON(c.stdout, q)
		}
		fmt.Fprintf(c.stdout, "%s  replied\n", q.ID)
		return nil
	case "applied":
		a, err := parseArgs(rest, "json")
		if err != nil {
			return err
		}
		if len(a.pos) == 0 {
			return fmt.Errorf("%w: target required", errUsage)
		}
		q, err := c.st.Applied(a.pos[0], a.flags["item"], a.flags["outcome"], a.flags["result"])
		if err != nil {
			return err
		}
		if a.has("json") {
			return writeJSON(c.stdout, q)
		}
		fmt.Fprintf(c.stdout, "%s  applied\n", q.ID)
		return nil
	case "export":
		a, err := parseArgs(rest, "all", "json")
		if err != nil {
			return err
		}
		if a.flags["render"] == "plan" {
			out, err := c.st.PlanDecisions(a.flags["date"])
			if err != nil {
				return err
			}
			return writeJSON(c.stdout, out)
		}
		out, err := c.st.DecisionExport(a.flags["group"], a.has("all"))
		if err != nil {
			return err
		}
		return writeJSON(c.stdout, out)
	case "import":
		a, err := parseArgs(rest, "include-answered", "json")
		if err != nil {
			return err
		}
		root := a.flags["root"]
		if root == "" && len(a.pos) > 0 {
			root = a.pos[0]
		}
		out, err := c.st.ImportPlans(root, a.has("include-answered"))
		if err != nil {
			return err
		}
		return writeJSON(c.stdout, out)
	default:
		return fmt.Errorf("%w: unknown decisions subcommand %q", errUsage, sub)
	}
}

func (c *cli) mcp(in []string) error {
	if len(in) > 0 {
		return fmt.Errorf("%w: mcp takes no arguments", errUsage)
	}
	return newMCPServer(c.st, c.stdin, c.stdout).serveMCP()
}

func (c *cli) catalog(in []string) error {
	return writeJSON(c.stdout, manifest())
}

func (c *cli) query(in []string) error {
	a, err := parseArgs(in, "archived")
	if err != nil {
		return err
	}
	res, err := queryQuestions(c.st, strings.Join(a.pos, " "), a.flags["status"], a.flags["project"], a.has("archived"))
	if err != nil {
		return err
	}
	return writeJSON(c.stdout, res)
}
