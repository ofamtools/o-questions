package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	ocli "ofam.dev/ochazuke/cli"
	"ofam.dev/oden"
	"os"
	"path/filepath"
	"strings"
)

func main() {
	os.Exit(oden.Report(os.Stderr, "o-questions", Main(os.Args[1:], os.Stdin, os.Stdout, os.Stderr)))
}
func dataDir() (string, error) {
	base := os.Getenv("XDG_DATA_HOME")
	if base == "" {
		h, e := os.UserHomeDir()
		if e != nil {
			return "", e
		}
		base = filepath.Join(h, ".local", "share")
	}
	return filepath.Abs(filepath.Join(base, "o-questions"))
}
func globalData(args []string) ([]string, string, error) {
	var out []string
	dir := ""
	for i := 0; i < len(args); i++ {
		if args[i] == "--" {
			out = append(out, args[i:]...)
			break
		}
		if args[i] == "--data" {
			i++
			if i >= len(args) {
				return nil, "", errors.New("--data needs a directory")
			}
			dir = args[i]
			continue
		}
		if strings.HasPrefix(args[i], "--data=") {
			dir = strings.TrimPrefix(args[i], "--data=")
			continue
		}
		out = append(out, args[i])
	}
	var e error
	if dir == "" {
		dir, e = dataDir()
	} else {
		dir, e = filepath.Abs(dir)
	}
	return out, dir, e
}

func Main(args []string, in io.Reader, out, errOut io.Writer) error {
	args, dir, e := globalData(args)
	if e != nil {
		return e
	}
	act, e := cliActor(os.Getenv)
	if e != nil {
		return e
	}
	a, e := newApp(&Store{Dir: dir, Actor: act}, in, out, errOut)
	if e != nil {
		return e
	}
	return dispatch(a, args)
}

// nounDefaults names the verb a mounted noun runs when it is given no verb
// word: `review` and `review --web` are `review page` and `review page --web`.
// Mount's group has no default of its own (library gap), so the words are
// rewritten before it resolves them.
var nounDefaults = []struct{ noun, verb string }{{reviewNoun, reviewPageVerb}}

func nounDefault(args []string) []string {
	verb, ok := "", false
	for _, d := range nounDefaults {
		if len(args) > 0 && args[0] == d.noun {
			verb, ok = d.verb, true
		}
	}
	if !ok || len(args) > 1 && (!strings.HasPrefix(args[1], "-") || args[1] == "-h" || args[1] == "--help") {
		return args
	}
	return append([]string{args[0], verb}, args[1:]...)
}

func dispatch(a *App, args []string) error {
	args = nounDefault(args)
	app := oden.New("o-questions", args, oden.WithBuiltins(), oden.WithStdio(a.in, a.out, a.errOut), oden.WithDefault(func(_ context.Context, args []string) error {
		if len(args) > 0 {
			return oden.ErrNotFound
		}
		return a.view(nil)
	}))
	sh := a.shell()
	var children []oden.Command
	for _, v := range a.specs() {
		v := v
		for _, name := range append([]string{v.name}, v.aliases...) {
			name := name
			run := func(ctx context.Context, words []string) error {
				return sh.Run(ctx, append([]string{"question", name}, words...))
			}
			children = append(children, oden.Command{Name: name, Summary: v.description, Run: run})
			app.Add(oden.Command{Name: name, Summary: v.description, Run: run})
		}
	}
	children = append(children, oden.Command{Name: "view", Summary: "Read the complete thread", Run: func(ctx context.Context, words []string) error {
		return sh.Run(ctx, append([]string{"question", "view"}, words...))
	}})
	app.Add(oden.Command{Name: "question", Aliases: []string{"q"}, Summary: "Record and review questions", Children: children})
	app.Add(oden.Command{Name: "view", Summary: "Questions page", Run: func(_ context.Context, args []string) error { return a.view(args) }})
	app.Add(oden.Command{Name: "mcp", Summary: "Agent tools over stdio", Run: func(_ context.Context, args []string) error {
		if len(args) > 0 {
			return fmt.Errorf("usage: mcp")
		}
		return a.mcp()
	}})
	ocli.Mount(app, sh)
	app.HelpText += "\nGlobal: --data DIR. ask WORDS records words verbatim; use --question TEXT with metadata flags.\n"
	return app.Run(context.Background())
}
