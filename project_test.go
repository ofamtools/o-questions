package main

import (
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
