package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

var testProjects = []hostProject{{"1", "Alpha", ""}, {"2", "Beta", ""}, {"3", "Dup", ""}, {"4", "dup", ""}}

func withProjects(s *Store) {
	s.ProjectRef = func(ref string) projectMatch { return resolveProjectRef(testProjects, ref) }
}

func askIn(t *testing.T, s *Store, text, project string) Question {
	t.Helper()
	v, e := s.Ask(Ask{Question: text, Key: text, Project: project})
	if e != nil {
		t.Fatal(e)
	}
	return v.Question
}

func questionBytes(t *testing.T, s *Store) map[string]string {
	t.Helper()
	files, _ := filepath.Glob(filepath.Join(s.Dir, "q", "*.json"))
	out := map[string]string{}
	for _, f := range files {
		b, e := os.ReadFile(f)
		if e != nil {
			t.Fatal(e)
		}
		out[filepath.Base(f)] = string(b)
	}
	return out
}

// The migration rewrites what names exactly one project, keeps the name,
// leaves every other question byte for byte, and a second run does nothing.
func TestProjectResolveMigration(t *testing.T) {
	s := fixture(t) // no host: references are stored as typed
	ids := map[string]string{}
	for _, row := range [][2]string{{"a", "alpha"}, {"b", "mtm:2"}, {"c", "1"}, {"d", "ghost"}, {"e", "DUP"}, {"f", ""}} {
		ids[row[0]] = askIn(t, s, row[0], row[1]).ID
	}
	before := questionBytes(t, s)
	rep, e := s.ResolveProjects(testProjects, nil, true)
	if e != nil || rep.Rows != 6 || rep.Empty != 1 || rep.AlreadyID != 1 || rep.KindToID != 1 || rep.NameToID != 1 || rep.Rewritten != 2 ||
		len(rep.Unmatched) != 1 || rep.Unmatched[0].Name != "ghost" || len(rep.Ambiguous) != 1 || rep.Ambiguous[0].Name != "DUP" {
		t.Fatalf("dry run: %+v %v", rep, e)
	}
	for name, b := range questionBytes(t, s) {
		if before[name] != b {
			t.Fatalf("dry run wrote %s", name)
		}
	}
	rep, e = s.ResolveProjects(testProjects, nil, false)
	if e != nil || rep.Rewritten != 2 || rep.Backup == "" {
		t.Fatalf("real run: %+v %v", rep, e)
	}
	if _, e = os.Stat(filepath.Join(rep.Backup, "q")); e != nil {
		t.Fatal("no backup:", e)
	}
	changed := 0
	for name, b := range questionBytes(t, s) {
		if before[name] != b {
			changed++
		}
	}
	if changed != 2 {
		t.Fatalf("%d files changed, want 2 (a and b)", changed)
	}
	got, e := s.List()
	if e != nil {
		t.Fatal(e)
	}
	want := map[string][2]string{"a": {"1", "Alpha"}, "b": {"2", "Beta"}, "c": {"1", ""}, "d": {"ghost", ""}, "e": {"DUP", ""}}
	for _, q := range got {
		if w, ok := want[q.Text]; ok && (q.Project.ID != w[0] || q.Project.Label != w[1]) {
			t.Fatalf("question %s: %+v, want %v", q.Text, q.Project, w)
		}
	}
	if rep, e = s.ResolveProjects(testProjects, nil, false); e != nil || rep.Rewritten != 0 || rep.Backup != "" {
		t.Fatalf("second run: %+v %v", rep, e)
	}
}

// A name typed on write is stored as the project's id with the name beside
// it; the name is shown and matches a filter; an ambiguous name stays typed.
func TestProjectNameResolvesOnWriteAndShowsAsName(t *testing.T) {
	s := fixture(t)
	withProjects(s)
	q := askIn(t, s, "x", "beta")
	if q.Project.ID != "2" || q.Project.Label != "Beta" {
		t.Fatalf("stored %+v", q.Project)
	}
	if d := askIn(t, s, "y", "dup"); d.Project.ID != "dup" || d.Project.Label != "" {
		t.Fatalf("ambiguous stored as %+v", d.Project)
	}
	if !s.projectMatcher("BETA", nil)(q.Project) || !s.projectMatcher("mtm:2", nil)(q.Project) || s.projectMatcher("alpha", nil)(q.Project) {
		t.Fatal("filter by name or id")
	}
	if s.projectDisplay(q.Project) != "Beta" {
		t.Fatal(s.projectDisplay(q.Project))
	}
	s.ProjectRef = func(string) projectMatch { t.Fatal("a read asked the host"); return projectMatch{} }
	if s.projectDisplay(q.Project) != "Beta" || !s.projectMatcher("beta", []Question{q})(q.Project) {
		t.Fatal("a read needed the host")
	}
	if r := questionRowData(q); r["project_id"] != "2" || r["project_name"] != "Beta" {
		t.Fatalf("row data %v", r)
	}
}

// With no host projects nothing is rewritten and nothing errors.
func TestProjectStandaloneKeepsNames(t *testing.T) {
	s := fixture(t)
	q := askIn(t, s, "x", "Beta")
	if q.Project.ID != "Beta" || q.Project.Label != "" || s.projectDisplay(q.Project) != "Beta" {
		t.Fatalf("standalone stored %+v", q.Project)
	}
}

var defaultProjects = []hostProject{{"1", "Alpha", "/w/alpha"}, {"2", "Beta", "/w/alpha/beta"}, {"3", "Twin", "/w/twin"}, {"4", "Twin2", "/w/twin"}}

// defaultApp is an app whose host knows defaultProjects and the panes p-1
// (project 1) and p-2 (no project); everything else is unknown.
func defaultApp(t *testing.T, source string) (*App, *int) {
	t.Helper()
	a, _ := testApp(t, fixture(t))
	calls := 0
	a.projectList = func(context.Context) ([]hostProject, error) { return defaultProjects, nil }
	a.paneList = func(_ context.Context, id string) (map[string]string, error) {
		calls++
		all := map[string]string{"p-1": "1", "p-2": ""}
		if id == "" {
			return all, nil
		}
		if v, ok := all[id]; ok {
			return map[string]string{id: v}, nil
		}
		return map[string]string{}, nil
	}
	if e := a.r.Settings().Set("project.source", source); e != nil {
		t.Fatal(e)
	}
	a.configure()
	return a, &calls
}

// A question with no project takes its pane's project, else the project
// whose directory holds the cwd (longest wins, ties store nothing); a given
// project wins; a follow-up keeps its thread's; pose follows the same path.
func TestProjectDefaultsFromPaneThenCwd(t *testing.T) {
	a, _ := defaultApp(t, "mtm")
	ask := func(text, project, pane, cwd string) Question {
		v, e := a.store.Ask(Ask{Question: text, Key: text, Project: project, Cwd: cwd, Source: Source{Pane: pane}})
		if e != nil {
			t.Fatal(e)
		}
		return v.Question
	}
	for _, c := range []struct{ name, project, pane, cwd, id, label string }{
		{"pane", "", "p-1", "/w/twin", "1", "Alpha"},
		{"cwd after a pane without project", "", "p-2", "/w/alpha/beta/sub", "2", "Beta"},
		{"cwd after an unknown pane", "", "gone", "/w/alpha/x", "1", "Alpha"},
		{"ambiguous cwd", "", "", "/w/twin", "", ""},
		{"nothing known", "", "", "/elsewhere", "", ""},
		{"given wins", "Beta", "p-1", "/w/twin", "2", "Beta"},
	} {
		if q := ask(c.name, c.project, c.pane, c.cwd); q.Project.ID != c.id || q.Project.Label != c.label {
			t.Errorf("%s: stored %+v, want %s/%s", c.name, q.Project, c.id, c.label)
		}
	}
	p, e := a.store.Pose(Pose{Question: "q?", Key: "pose", Provenance: Source{Pane: "p-1"}, Options: []Option{{ID: "a", Label: "A"}, {ID: "b", Label: "B"}}})
	if e != nil || p.Question.Project.ID != "1" {
		t.Fatalf("pose: %+v %v", p.Question.Project, e)
	}
	th := ask("thread", "Beta", "", "")
	f, e := a.store.Ask(Ask{Question: "more", Key: "more", FollowUpOf: th.ID, Source: Source{Pane: "p-1"}})
	if e != nil || f.Question.Project.ID != "2" {
		t.Fatalf("follow-up: %+v %v", f.Question.Project, e)
	}
}

// A host that fails stores no project and does not fail the ask; source none
// makes no host call at all.
func TestProjectDefaultHostFailsOrOff(t *testing.T) {
	a, calls := defaultApp(t, "mtm")
	a.paneList = func(context.Context, string) (map[string]string, error) { return nil, errors.New("host down") }
	a.projectList = func(context.Context) ([]hostProject, error) { return nil, errors.New("host down") }
	v, e := a.store.Ask(Ask{Question: "x", Key: "x", Source: Source{Pane: "p-1"}})
	if e != nil || v.Question.Project.ID != "" {
		t.Fatalf("failed host: %+v %v", v.Question.Project, e)
	}
	a, calls = defaultApp(t, "none")
	a.projectList = nil
	a.configure()
	if v, e = a.store.Ask(Ask{Question: "y", Key: "y", Source: Source{Pane: "p-1"}, Cwd: "/w/alpha"}); e != nil || v.Question.Project.ID != "" || *calls != 0 {
		t.Fatalf("source none: %+v %v calls %d", v.Question.Project, e, *calls)
	}
}

// The backfill fills a question with no project from its pane's project,
// counts the unknown panes and the rest, never touches a set project or any
// other byte, and a second run does nothing.
func TestProjectBackfillFromPane(t *testing.T) {
	s := fixture(t)
	for _, c := range [][3]string{{"known", "p-1", ""}, {"unknown", "gone", ""}, {"nopane", "", ""}, {"set", "p-1", "2"}, {"nopanes-project", "p-2", ""}} {
		if _, e := s.Ask(Ask{Question: c[0], Key: c[0], Project: c[2], Source: Source{Pane: c[1]}}); e != nil {
			t.Fatal(e)
		}
	}
	before := questionBytes(t, s)
	panes := map[string]string{"p-1": "1", "p-2": ""}
	rep, e := s.ResolveProjects(defaultProjects, panes, true)
	if e != nil || rep.PaneFilled != 1 || rep.PaneUnknown != 1 || rep.NoPane != 1 || rep.PaneNoProj != 1 || rep.Backup != "" {
		t.Fatalf("dry run: %+v %v", rep, e)
	}
	if rep, e = s.ResolveProjects(defaultProjects, panes, false); e != nil || rep.PaneFilled != 1 || rep.Backup == "" {
		t.Fatalf("real run: %+v %v", rep, e)
	}
	changed := 0
	for name, b := range questionBytes(t, s) {
		if b != before[name] {
			changed++
			if !strings.Contains(b, `"id": "1"`) || !strings.Contains(b, `"label": "Alpha"`) {
				t.Fatalf("filled file: %s", b)
			}
		}
	}
	if changed != 1 {
		t.Fatalf("%d files changed, want 1", changed)
	}
	rows, _ := s.List()
	for _, q := range rows {
		if want := map[string]string{"known": "1", "set": "2"}[q.Text]; q.Project.ID != want {
			t.Errorf("%s: project %q, want %q", q.Text, q.Project.ID, want)
		}
	}
	if rep, e = s.ResolveProjects(defaultProjects, panes, false); e != nil || rep.PaneFilled != 0 || rep.Backup != "" {
		t.Fatalf("second run: %+v %v", rep, e)
	}
}
