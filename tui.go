// Standalone mount and reload lifecycle adapted from o-cmds/tui.go.
package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"ofam.dev/ochazuke"
	ocli "ofam.dev/ochazuke/cli"
)

type page struct {
	a     *App
	m     *ochazuke.Model
	v     *ocli.PageView
	dirty bool
}

type reloadMsg struct{}

func (p *page) tick() tea.Cmd {
	every := p.a.r.Settings().Duration("list.refresh")
	if every <= 0 {
		return nil
	}
	return tea.Tick(every, func(time.Time) tea.Msg { return reloadMsg{} })
}

func mountPage(a *App) (*page, error) {
	data, e := ocli.Catalog(a.r, "o-questions", version)
	if e != nil {
		return nil, e
	}
	var doc ocli.CatalogDoc
	if e = json.Unmarshal(data, &doc); e != nil {
		return nil, e
	}
	m := ochazuke.NewModel(a.r)
	m.Title = "o-questions"
	v, e := ocli.MountPage(m, doc, standalonePage(a.r.Settings().Bool("view.detail")), ocli.PageConfig{DetailKinds: map[string]ocli.DetailFactory{questionDetailKind: newQuestionDetail}})
	if e != nil {
		return nil, e
	}
	p := &page{a: a, m: m, v: v}
	a.tui = true
	a.store.Actor = Owner()
	a.configure()
	a.changed = func() { p.dirty = true }
	for _, name := range []string{"list.order", "filter.status", "filter.project", "filter.agent", "filter.since", "detail.answer-lines", "detail.provenance"} {
		a.r.Settings().Observe(name, func(string, any) { p.dirty = true })
	}
	quit := ochazuke.Do("app.quit")
	quit.Instance = pageID
	quit.Mode = "normal"
	quit.Keys = "q"
	if e = a.r.Bind(quit); e != nil {
		return nil, e
	}
	if e = a.r.OpenPage(pageID); e != nil {
		return nil, e
	}
	if e = p.reload(true); e != nil {
		return nil, e
	}
	if e = v.List(listID).Focus(); e != nil {
		return nil, e
	}
	return p, nil
}

func (p *page) reload(initial bool) error {
	rows, e := p.a.rows(nil)
	if e != nil {
		return e
	}
	if e = p.v.SetRows(listID, rows); e != nil {
		return e
	}
	if initial && len(rows) > 0 && p.a.r.Settings().Bool("list.start-newest") {
		i := len(rows) - 1
		if p.a.newestFirst() {
			i = 0
		}
		return p.v.List(listID).Select(listID + "/" + rows[i].ID)
	}
	return nil
}

func runTUI(a *App, args []string) error {
	a.r.EnsureConfigSettings()
	fs := flag.NewFlagSet("view", flag.ContinueOnError)
	fs.SetOutput(a.errOut)
	smoke := fs.Bool("smoke", false, "Render one frame and exit")
	w := fs.Int("width", 120, "Frame width")
	h := fs.Int("height", 30, "Frame height")
	if e := fs.Parse(args); e != nil {
		if errors.Is(e, flag.ErrHelp) {
			return nil
		}
		return e
	}
	if fs.NArg() > 0 {
		return errors.New("unexpected view arguments")
	}
	p, e := mountPage(a)
	if e != nil {
		return e
	}
	if *smoke {
		p.m.Width, p.m.Height = *w, *h
		_, e = fmt.Fprintln(a.out, strings.TrimRight(ansi.Strip(p.m.View().Content), "\n"))
		return e
	}
	_, e = tea.NewProgram(p, tea.WithInput(a.in), tea.WithOutput(a.out)).Run()
	return e
}

func (p *page) Init() tea.Cmd { return tea.Batch(p.m.Init(), p.a.r.PendingCommands(), p.tick()) }
func (p *page) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if _, ok := msg.(reloadMsg); ok {
		if e := p.reload(false); e != nil {
			p.a.r.UI().Status(e.Error())
		}
		return p, p.tick()
	}
	_, cmd := p.m.Update(msg)
	if p.dirty {
		p.dirty = false
		if e := p.reload(false); e != nil {
			p.a.r.UI().Status(e.Error())
		}
		cmd = tea.Batch(cmd, p.a.r.PendingCommands())
	}
	return p, cmd
}
func (p *page) View() tea.View { return p.m.View() }
