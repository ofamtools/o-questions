package main

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"

	"ofam.dev/ochazuke"
)

// verbRun is the handler of one verb spec row: the row names its own
// behaviour, so no switch on the verb name dispatches it.
type verbRun func(a *App, c ochazuke.Ctx) ochazuke.Result

// invoke runs the spec's handler: settings are applied, and in the TUI a
// verb missing its input first opens its editor or picker.
func (a *App) invoke(v verbSpec, c ochazuke.Ctx) ochazuke.Result {
	a.configure()
	if a.tui {
		if res, ok := a.interactive(v.name, c); ok {
			return res
		}
	}
	return v.run(a, c)
}

// handle runs a verb by name (or alias) as the catalog would.
func (a *App) handle(verb string, c ochazuke.Ctx) ochazuke.Result {
	for _, v := range a.specs() {
		if v.name == verb || slicesContains(v.aliases, verb) {
			return a.invoke(v, c)
		}
	}
	return result(nil, fmt.Errorf("unknown verb %q", verb))
}

func slicesContains(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}

func argStr(c ochazuke.Ctx, k string) string { return ochazuke.StringArg(c.Args, k, "") }

func runAsk(a *App, c ochazuke.Ctx) ochazuke.Result {
	var p Ask
	if e := decodeArgs(c.Args, &p); e != nil {
		return result(nil, e)
	}
	p.Source = fillSource(p.Source, sourceFromEnv(a.store.Actor))
	v, e := a.store.Ask(p)
	a.refresh()
	return result(v, e)
}

func runList(a *App, c ochazuke.Ctx) ochazuke.Result {
	v, e := a.rows(c.Args)
	return result(v, e)
}

func runUndo(a *App, _ ochazuke.Ctx) ochazuke.Result {
	e := a.store.Undo()
	a.refresh()
	return result(nil, e)
}

func runHistory(a *App, _ ochazuke.Ctx) ochazuke.Result {
	v, e := a.store.History()
	return result(v, e)
}

func runBackup(a *App, c ochazuke.Ctx) ochazuke.Result {
	v, e := a.store.Backup(argStr(c, "to"))
	return result(v, e)
}

func runRestore(a *App, c ochazuke.Ctx) ochazuke.Result {
	if file := argStr(c, "file"); file != "" {
		e := a.store.Restore(file)
		a.refresh()
		return result(nil, e)
	}
	q, e := a.target(c)
	if e != nil {
		return result(nil, e)
	}
	q, e = a.store.Change(opRestore, q.ID, Change{})
	a.refresh()
	return result(q, e)
}

// onTarget wraps a handler that works on one resolved question.
func onTarget(f func(a *App, q Question, c ochazuke.Ctx) ochazuke.Result) verbRun {
	return func(a *App, c ochazuke.Ctx) ochazuke.Result {
		q, e := a.target(c)
		if e != nil {
			return result(nil, e)
		}
		return f(a, q, c)
	}
}

// runShow reads the complete thread: the viewer in the TUI, else the markdown.
var runShow = onTarget(func(a *App, q Question, _ ochazuke.Ctx) ochazuke.Result {
	if a.tui && a.showThread != nil {
		return result(nil, a.showThread(q))
	}
	return ochazuke.Done(thread(q, a.r.Settings().Bool("detail.provenance")))
})

var runCheck = onTarget(func(a *App, q Question, _ ochazuke.Ctx) ochazuke.Result { return a.check(q) })

// copyParts are the "part" values that yank something other than the thread.
var copyParts = []struct {
	name string
	text func(Question) string
}{{"question", func(q Question) string { return q.Text }}, {"answer", latestAnswer}}

var runYank = onTarget(func(a *App, q Question, c ochazuke.Ctx) ochazuke.Result {
	text := thread(q, true)
	for _, p := range copyParts {
		if p.name == argStr(c, "part") {
			text = p.text(q)
		}
	}
	cmd := exec.Command("pbcopy")
	cmd.Stdin = strings.NewReader(text)
	if e := a.run(cmd); e != nil {
		return result(nil, e)
	}
	return result(text, nil)
})

// openableLinks are what question.open can open: the question's links, then
// its source transcript.
func openableLinks(q Question) []Link {
	links := append([]Link{}, q.Links...)
	if q.Source.Transcript != "" {
		links = append(links, Link{Target: q.Source.Transcript, Label: "Transcript"})
	}
	return links
}

var errNoLinks = errors.New("this question has no links")

var runOpen = onTarget(func(a *App, q Question, c ochazuke.Ctx) ochazuke.Result {
	links := openableLinks(q)
	target := argStr(c, "link")
	if target == "" {
		if len(links) == 0 {
			return result(nil, errNoLinks)
		}
		if len(links) != 1 {
			return result(nil, fmt.Errorf("choose --link from %v", links))
		}
		target = links[0].Target
	}
	return result(nil, a.run(exec.Command("open", target)))
})

// entryVerb appends an entry of the given kind whose text is the textParam
// argument; a "kind" argument, where the spec has one, overrides the kind.
func entryVerb(kind, textParam string) verbRun {
	return onTarget(func(a *App, q Question, c ochazuke.Ctx) ochazuke.Result {
		kind := kind
		if k := argStr(c, "kind"); k != "" {
			kind = k
		}
		var ann Annotation
		if e := decodeArgs(c.Args, &ann); e != nil {
			return result(nil, e)
		}
		v, e := a.store.mutate(mapAction(kind), q.ID, func(q *Question, _ *[]Question) error {
			annotate(q, Annotation{Tags: ann.Tags})
			en := Entry{Kind: kind, Text: argStr(c, textParam), Summary: argStr(c, "summary"), Replaces: argStr(c, "replaces"), Links: ann.Links}
			act := a.store.Actor
			if argStr(c, "model") != "" {
				a.store.Actor.Model = argStr(c, "model")
				defer func() { a.store.Actor = act }()
			}
			return a.store.appendEntry(q, en)
		})
		a.refresh()
		return result(v, e)
	})
}

// changeVerb applies the store change op named by the row: valueParam
// names the argument carried as Change.Value, and extra may add to the
// decoded annotation.
func changeVerb(op ChangeOp, valueParam string, extra func(*Annotation, func(string) string)) verbRun {
	return onTarget(func(a *App, q Question, c ochazuke.Ctx) ochazuke.Result {
		var ann Annotation
		if e := decodeArgs(c.Args, &ann); e != nil {
			return result(nil, e)
		}
		if extra != nil {
			extra(&ann, func(k string) string { return argStr(c, k) })
		}
		v, e := a.store.Change(op, q.ID, Change{Value: argStr(c, valueParam), Entry: argStr(c, "entry"), Other: argStr(c, "other"), Annotation: ann})
		a.refresh()
		return result(v, e)
	})
}

// runAnnotate is annotate: a step runs when its arguments are given, so
// one, two or no step is the same path. The entry step (a text appends a
// note or correction, carrying the tags and links) comes first; the change
// step (a project, a task or a title, and tags or links no text carried)
// second. With neither, annotate says what to give.
var runAnnotate = func(a *App, c ochazuke.Ctx) ochazuke.Result {
	has := func(names ...string) bool {
		for _, name := range names {
			if argStr(c, name) != "" || c.Args[name] != nil {
				return true
			}
		}
		return false
	}
	text := has("text")
	steps := []struct {
		when bool
		run  verbRun
	}{
		{text, entryVerb("note", "text")},
		{has("project", "task", "title") || (!text && has("tags", "links")), changeVerb(opAnnotate, "text", nil)},
	}
	var value any
	ran := false
	for _, step := range steps {
		if !step.when {
			continue
		}
		ran = true
		v, e := step.run(a, c).Await(context.Background())
		if e != nil {
			return result(nil, e)
		}
		value = v
	}
	if !ran {
		return result(nil, errors.New("annotate needs text, tags, links, a project, a task or a title"))
	}
	return result(value, nil)
}
