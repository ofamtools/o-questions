// Command o-questions is a questions & owner-decisions journal TUI, CLI and
// MCP server built on Ochazuke v2.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	tea "charm.land/bubbletea/v2"

	ocli "ofam.dev/ochazuke/cli"
	"ofam.dev/ochazuke/config"
	"ofam.dev/ochazuke/core"
	"ofam.dev/ochazuke/ui"
	"ofam.dev/ochazuke/widgets/table"
	"ofam.dev/oden"
)

func main() { os.Exit(run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr)) }

func progName() string {
	if parent := os.Getenv("ODEN_PARENT"); parent != "" {
		return parent + " questions"
	}
	return "o-questions"
}

func run(argv []string, stdin io.Reader, stdout, stderr io.Writer) int {
	argv, dataFlag, err := splitDataArgs(argv)
	if err != nil {
		fmt.Fprintln(stderr, "o-questions:", err)
		return 2
	}
	app := newOdenApp(argv, dataFlag, stdin, stdout, stderr)
	return oden.Report(stderr, progName(), app.Run(context.Background()))
}

func newOdenApp(argv []string, dataFlag string, stdin io.Reader, stdout, stderr io.Writer) *oden.App {
	openCLI := func() (*cli, error) {
		dir, err := resolveDataDir(dataFlag)
		if err != nil {
			return nil, err
		}
		act, err := cliActor(os.Getenv)
		if err != nil {
			return nil, err
		}
		st := &Store{Dir: dir, Actor: act}
		return &cli{st: st, stdin: stdin, stdout: stdout}, nil
	}

	exportConfig := func(jsonl bool) ([]byte, error) {
		dir, _ := resolveDataDir(dataFlag)
		cfg := map[string]any{
			"name":     "o-questions",
			"version":  Version,
			"config":   configDir(),
			"data":     dir,
			"settings": defaultSettings,
		}
		if jsonl {
			var b strings.Builder
			for _, k := range []string{"name", "version", "config", "data", "settings"} {
				line, err := json.Marshal(map[string]any{"key": k, "value": cfg[k]})
				if err != nil {
					return nil, err
				}
				b.Write(line)
				b.WriteByte('\n')
			}
			return []byte(b.String()), nil
		}
		return json.MarshalIndent(cfg, "", "  ")
	}

	info := ocli.AppInfo{
		Name:         "o-questions",
		Version:      Version,
		Summary:      "Questions & owner-decisions journal TUI, CLI, and MCP server",
		Modes:        func() []core.Mode { return modes },
		Bindings:     func() []core.Binding { return defaultBindings() },
		Specs:        func() []core.CommandSpec { return commandSpecs() },
		ExportConfig: exportConfig,
		RunTUI: func(_ context.Context, tuiArgs []string) error {
			if code := interactive(tuiArgs, dataFlag, stdout, stderr); code != 0 {
				return &oden.ExitError{Code: code}
			}
			return nil
		},
		RunSmoke: func(out io.Writer, _ bool) error {
			if code := runSmoke(out, stderr); code != 0 {
				return &oden.ExitError{Code: code}
			}
			return nil
		},
		Manifest: extManifest(),
		Capabilities: &oden.Capabilities{
			Name:         "o-questions",
			Version:      Version,
			Summary:      "Questions & owner-decisions journal TUI, CLI, and MCP server",
			Capabilities: []string{"query", "catalog", "tui", "mcp"},
		},
		Query: func(_ context.Context, kind string, args []string) (oden.ExtQueryResult, error) {
			c, err := openCLI()
			if err != nil {
				return oden.ExtQueryResult{}, err
			}
			return extQueryQuestions(c.st, kind, args)
		},
	}

	app := ocli.NewApp(info, argv, oden.WithStdio(stdin, stdout, stderr))

	subCmd := func(name string, aliases []string, summary string) oden.Command {
		sub := subcommands[name]
		return oden.Command{
			Name:    name,
			Aliases: aliases,
			Summary: summary,
			Usage:   strings.TrimPrefix(sub.usage, "o-questions "+name+" "),
			Run: func(_ context.Context, args []string) error {
				c, err := openCLI()
				if err != nil {
					return err
				}
				if err := sub.run(c, args); err != nil {
					if errors.Is(err, errUsage) {
						fmt.Fprintln(stderr, "usage:", sub.usage)
						return &oden.ExitError{Code: 2}
					}
					return err
				}
				return nil
			},
		}
	}

	app.Add(
		subCmd("ask", nil, "record a new question"),
		subCmd("answer", nil, "append an answer to a question"),
		subCmd("list", []string{"ls"}, "list questions in the journal"),
		subCmd("show", nil, "show a question and its full thread"),
		subCmd("resolve", []string{"close"}, "mark a question resolved (answered or moot)"),
		subCmd("reopen", nil, "reopen a question"),
		subCmd("undo", nil, "undo the last store change"),
		subCmd("redo", nil, "redo the last undone store change"),
		subCmd("decisions", nil, "list, pose, decide, reply, export, or import owner decisions"),
		subCmd("mcp", nil, "serve agent MCP tools over stdio (JSON-RPC 2.0)"),
	)

	app.HelpText = usageText()
	return app
}

func interactive(argv []string, dataFlag string, stdout, stderr io.Writer) int {
	a, err := parseArgs(argv, "smoke", "version", "help", "h", "decisions")
	if err != nil {
		fmt.Fprintln(stderr, "o-questions:", err)
		return 2
	}
	if a.has("help") || a.has("h") {
		fmt.Fprintln(stdout, usageText())
		return 0
	}
	if a.has("version") {
		fmt.Fprintf(stdout, "o-questions %s\n", Version)
		return 0
	}
	if a.has("smoke") {
		return runSmoke(stdout, stderr)
	}
	if len(a.pos) > 0 {
		fmt.Fprintf(stderr, "o-questions: unknown command %q (try --help)\n", a.pos[0])
		return 2
	}
	dir, err := resolveDataDir(dataFlag)
	if err != nil {
		fmt.Fprintln(stderr, "o-questions:", err)
		return 1
	}
	act, err := cliActor(os.Getenv)
	if err != nil {
		fmt.Fprintln(stderr, "o-questions:", err)
		return 1
	}
	cfg, _ := loadConfig(configDir())
	st := &Store{Dir: dir, Actor: act}
	env := newEnv(st, cfg)
	sh := newShell(env)
	if _, err := tea.NewProgram(sh).Run(); err != nil {
		fmt.Fprintln(stderr, "o-questions:", err)
		return 1
	}
	return 0
}

func configDir() string {
	if x := os.Getenv("XDG_CONFIG_HOME"); x != "" {
		return filepath.Join(x, "o-questions")
	}
	if home, err := os.UserHomeDir(); err == nil {
		return filepath.Join(home, ".config", "o-questions")
	}
	return ""
}

func loadConfig(dir string) (Config, *config.Settings) {
	var cfg Config
	read := func(name string) []byte {
		if dir == "" {
			return nil
		}
		data, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil && !errors.Is(err, fs.ErrNotExist) {
			cfg.Notes = append(cfg.Notes, name+": "+err.Error())
		}
		return data
	}
	if data := read("bindings.json"); data != nil {
		if bs, err := config.LoadBindings(data); err != nil {
			cfg.Notes = append(cfg.Notes, err.Error())
		} else {
			cfg.Bindings = bs
		}
	}
	if data := read("themes.json"); data != nil {
		if th, err := config.LoadTheme(data, ui.DefaultDarkTheme()); err != nil {
			cfg.Notes = append(cfg.Notes, err.Error())
		} else {
			cfg.Theme = &th
		}
	}
	set, err := config.LoadSettings(read("settings.json"), defaultSettings)
	if err != nil {
		cfg.Notes = append(cfg.Notes, err.Error())
		set, _ = config.LoadSettings(nil, defaultSettings)
	}
	cfg.Settings = set
	return cfg, set
}

// --- smoke ---------------------------------------------------------------------

var ansiRe = regexp.MustCompile(`\x1b\[[0-9;:?]*[A-Za-z]|\x1b\][^\a\x1b]*(\a|\x1b\\)`)

func plain(s string) string { return ansiRe.ReplaceAllString(s, "") }

func maxLineW(s string) int {
	w := 0
	for _, l := range strings.Split(s, "\n") {
		w = max(w, textW(l))
	}
	return w
}

// runSmoke drives a scripted headless 120x32 session over a temporary --data
// directory through the real keymap, engine, shell, and renderer.
func runSmoke(stdout, stderr io.Writer) int {
	tmp, err := os.MkdirTemp("", "o-questions-smoke-*")
	if err != nil {
		fmt.Fprintln(stderr, "smoke:", err)
		return 1
	}
	defer os.RemoveAll(tmp)

	st := &Store{Dir: tmp, Actor: Owner()}
	// Seed one question and one owner decision.
	if _, err := st.Ask(Ask{
		Question: "How should we structure the v2 TUI?",
		Title:    "TUI Architecture",
		Project:  "ochazuke",
	}); err != nil {
		fmt.Fprintln(stderr, "smoke:", err)
		return 1
	}
	ag := *st
	ag.Actor = Actor{Kind: "agent", Name: "smoke-agent", Harness: "smoke"}
	posed, err := ag.Pose(Pose{
		Question:       "Ship Ochazuke v2 migration now?",
		Title:          "Ship v2 Migration",
		Group:          "Release",
		Options:        []Option{{ID: "yes", Label: "Yes, ship it"}, {ID: "no", Label: "Hold"}},
		Recommended:    "yes",
		Recommendation: "All smoke and race tests pass.",
		Project:        "ochazuke",
		Key:            "smoke-ship-v2",
	})
	if err != nil {
		fmt.Fprintln(stderr, "smoke:", err)
		return 1
	}

	env := newEnv(st, Config{})
	sh := newShell(env)
	d := newDriver(sh, 120, 32)
	app := func() App { return appOf(sh.State) }
	frame := func() string { return plain(sh.View().Content) }

	var results []string
	fails := 0
	check := func(name string, ok bool) {
		mark := "ok  "
		if !ok {
			mark, fails = "FAIL", fails+1
		}
		results = append(results, mark+" "+name)
	}

	f := frame()
	check("frame is 120x32", len(strings.Split(f, "\n")) == 32 && maxLineW(f) <= 120)
	check("lists seeded questions", len(app().Questions) == 2 && strings.Contains(f, " LIST "))

	// Ask a new question via form modal.
	d.keys("a")
	d.typeText("Does readline work in forms?")
	d.keys("tab")
	d.typeText("Readline Check")
	d.keys("enter")
	check("a asks question via form", len(app().Questions) == 3)

	// Filter with / and clear with esc.
	d.keys("/")
	d.typeText("Readline")
	check("/ filters live while typing", app().Typing && app().filteredCount() == 1)
	d.keys("esc")
	check("esc clears filter", !app().Typing && app().Query == "" && app().filteredCount() == 3)

	// Sort with s.
	d.keys("s")
	check("s sorts by title", app().List.SortField == cellTitle && app().List.SortDir == table.SortAsc)

	// Answer focused question via A form, then resolve/reopen, then undo/redo.
	d.keys("A")
	d.typeText("Yes, readline bindings work everywhere.")
	d.keys("enter")
	q0, _ := app().focusedQuestion()
	check("A appends answer", q0.Status == "answered" && len(q0.Entries) == 1)

	d.keys("o")
	q0, _ = app().focusedQuestion()
	check("o reopens question", q0.Status == "open")

	d.keys("r")
	q0, _ = app().focusedQuestion()
	check("r resolves question", q0.Status == "answered")

	d.keys("u")
	q0, _ = app().focusedQuestion()
	check("u undoes resolve", q0.Status == "open")

	d.keys("ctrl+r")
	q0, _ = app().focusedQuestion()
	check("ctrl+r redoes resolve", q0.Status == "answered")

	// Switch to Decisions view (D), decide via d form, and verify atomic reload from fresh Store.
	d.keys("D")
	check("D switches to Decisions view", app().DecisionsOnly && len(app().Questions) == 1)
	d.keys("d")
	d.keys("ctrl+u")
	d.typeText("yes")
	d.keys("tab")
	d.typeText("Approved in smoke test")
	d.keys("enter")
	fresh := &Store{Dir: tmp, Actor: Owner()}
	reloaded, rerr := fresh.Resolve(posed.Question.ID, false)
	check("decision answer persisted atomically on reload", rerr == nil && reloaded.Decision != nil && reloaded.Decision.Action == "yes" && reloaded.Decision.Note == "Approved in smoke test")

	// Palette & live help.
	d.keys(":")
	check(": opens palette", len(sh.State.Modals) == 1 && sh.State.Modals[0].ID == modalPalette)
	d.keys("esc", "?")
	check("? opens live help", len(sh.State.Modals) == 1 && strings.Contains(frame(), "filter.open"))
	d.keys("esc")

	// Catalog & query wire protocol.
	catBytes, cerr := json.Marshal(manifest())
	check("catalog marshals cleanly", cerr == nil && strings.Contains(string(catBytes), questionKind))
	qr, qerr := queryQuestions(fresh, "Readline", "", "", false)
	check("query returns matching record", qerr == nil && qr.Total == 1 && qr.Items[0].Kind == questionKind)

	f = frame()
	for _, line := range strings.Split(f, "\n") {
		fmt.Fprintln(stdout, strings.TrimRight(line, " "))
	}
	fmt.Fprintln(stdout)
	for _, r := range results {
		fmt.Fprintln(stdout, r)
	}
	fmt.Fprintf(stdout, "smoke: %d/%d checks passed · %d items · %d renders\n", len(results)-fails, len(results), len(app().Questions), sh.Renders)
	if fails > 0 {
		return 1
	}
	return 0
}
