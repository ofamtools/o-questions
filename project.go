package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"ofam.dev/ochazuke"
	ocli "ofam.dev/ochazuke/cli"
)

// A question's project is stored as the id of the host's project (mtm's
// key), so the relation graph can follow it; the name people type and read is
// the host's name for that id, kept in project.label as the fallback for a
// host that is absent or a project that is gone. A reference typed as a name,
// an id or kind:id (mtm:<id>) is resolved to the id when it matches exactly
// one project; one that matches nothing, or several, is stored as typed.

// hostProject is a project of the host: the id mtm keys it by and its name.
type hostProject struct{ ID, Name, Dir string }

// Resolution outcomes.
const (
	projectEmpty     = "empty"
	projectID        = "id"
	projectKindID    = "kind-id"
	projectName      = "name"
	projectAmbiguous = "ambiguous"
	projectUnmatched = "unmatched"
)

type projectMatch struct {
	Status   string
	ID, Name string
	IDs      []string
	Resolved bool // names exactly one project
	Rewrite  bool // resolving changes the stored text
}

// resolveProjectRef resolves ref against the host's projects: an exact id
// first (with or without the mtm: kind), then a name, case-insensitively, as
// mtm resolves names.
func resolveProjectRef(projects []hostProject, ref string) projectMatch {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return projectMatch{Status: projectEmpty}
	}
	id, kind := ref, false
	if rest, ok := strings.CutPrefix(ref, "mtm:"); ok && rest != "" {
		id, kind = rest, true
	}
	for _, p := range projects {
		if p.ID == id {
			status := projectID
			if kind {
				status = projectKindID
			}
			return projectMatch{Status: status, ID: p.ID, Name: p.Name, Resolved: true, Rewrite: kind}
		}
	}
	var found []hostProject
	for _, p := range projects {
		if strings.EqualFold(p.Name, ref) {
			found = append(found, p)
		}
	}
	switch len(found) {
	case 0:
		return projectMatch{Status: projectUnmatched}
	case 1:
		return projectMatch{Status: projectName, ID: found[0].ID, Name: found[0].Name, Resolved: true, Rewrite: true}
	}
	m := projectMatch{Status: projectAmbiguous}
	for _, p := range found {
		m.IDs = append(m.IDs, p.ID)
	}
	return m
}

// hostProjectsFrom reads the host's project list: mounted, through the
// host's project.list; standalone, through mtm on PATH. Source none has none.
func hostProjectsFrom(ctx context.Context, source string) ([]hostProject, error) {
	if source == "" || source == "none" {
		return nil, nil
	}
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	var b []byte
	host, hosted, e := ocli.HostFromEnv()
	if e != nil {
		return nil, e
	}
	if hosted {
		b, e = host.Run(ctx, "project.list", nil)
	} else {
		b, e = exec.CommandContext(ctx, "mtm", "project", "list").Output()
	}
	if e != nil {
		return nil, fmt.Errorf("project.source %s: %w", source, e)
	}
	// mtm answers an array, or {"items": [...]} for a paged list.
	type row struct{ ID, Name, Label, Dir string }
	var rows []row
	if e = json.Unmarshal(b, &rows); e != nil {
		var page struct{ Items []row }
		if e2 := json.Unmarshal(b, &page); e2 != nil {
			return nil, e
		}
		rows = page.Items
	}
	out := []hostProject{}
	for _, r := range rows {
		name := r.Name
		if name == "" {
			name = r.Label
		}
		if name == "" {
			name = r.ID
		}
		out = append(out, hostProject{ID: r.ID, Name: name, Dir: r.Dir})
	}
	return out, nil
}

// projectIndex is the host's projects, read at most every few seconds. A
// failed read resolves nothing and is not cached: names stay as stored.
func (a *App) projectIndex() []hostProject {
	if a.projectsAt.IsZero() || time.Since(a.projectsAt) > 3*time.Second {
		list, e := a.hostProjects(context.Background())
		if e != nil {
			return nil
		}
		a.projects, a.projectsAt = list, time.Now()
	}
	return a.projects
}

func (a *App) hostProjects(ctx context.Context) ([]hostProject, error) {
	if a.projectList != nil {
		return a.projectList(ctx)
	}
	return hostProjectsFrom(ctx, a.r.Settings().String("project.source"))
}

// hostRows runs a host list verb: mounted, through the host; standalone,
// through mtm on PATH. The answer is an array or a page {"items": [...]}.
func hostRows(ctx context.Context, noun string, args ochazuke.Arguments, cli []string) ([]map[string]any, error) {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	var b []byte
	host, hosted, e := ocli.HostFromEnv()
	if e != nil {
		return nil, e
	}
	if hosted {
		b, e = host.Run(ctx, noun+".list", args)
	} else {
		b, e = exec.CommandContext(ctx, "mtm", append([]string{noun, "list"}, cli...)...).Output()
	}
	if e != nil {
		return nil, e
	}
	var rows []map[string]any
	if e = json.Unmarshal(b, &rows); e != nil {
		var page struct{ Items []map[string]any }
		if e2 := json.Unmarshal(b, &page); e2 != nil {
			return nil, e
		}
		rows = page.Items
	}
	return rows, nil
}

// hostPanesFrom reads the host's panes as pane id to project id (empty when
// the pane has none): one pane when id is given, every pane, ended ones
// included, when it is empty. A pane the host does not know is absent.
// Source none has none.
func hostPanesFrom(ctx context.Context, source, id string) (map[string]string, error) {
	if source == "" || source == "none" {
		return nil, nil
	}
	args, cli := ochazuke.Arguments{"props": "project", "limit": 0}, []string{"--props", "project", "--limit", "0"}
	if id != "" {
		args["where"] = "id=" + id
		cli = append(cli, "--where", "id="+id)
	}
	rows, e := hostRows(ctx, "pane", args, cli)
	if e != nil {
		return nil, fmt.Errorf("project.source %s: %w", source, e)
	}
	out := map[string]string{}
	for _, r := range rows {
		pid, _ := r["id"].(string)
		proj, _ := r["project"].(string)
		if pid != "" {
			out[pid] = proj
		}
	}
	return out, nil
}

// projectForDir is the id of the project whose directory contains dir: the
// longest matching directory wins, and several different projects at that
// length match nothing (ambiguous).
func projectForDir(projects []hostProject, dir string) string {
	dir = filepath.Clean(dir)
	best, id, tied := -1, "", false
	for _, p := range projects {
		if p.Dir == "" {
			continue
		}
		root := filepath.Clean(p.Dir)
		if dir != root && !strings.HasPrefix(dir, strings.TrimSuffix(root, "/")+"/") {
			continue
		}
		switch {
		case len(root) > best:
			best, id, tied = len(root), p.ID, false
		case len(root) == best && p.ID != id:
			tied = true
		}
	}
	if tied {
		return ""
	}
	return id
}

type paneProject struct {
	id string
	at time.Time
}

// defaultProject is the project a new question gets when none was given: the
// asking pane's project as the host reports it, else the project whose
// directory contains cwd (the given one, else the current one). A failed
// read, a pane the host does not know, or an ambiguous directory gives
// none: the question is recorded without a project.
func (a *App) defaultProject(pane, cwd string) string {
	if src := a.r.Settings().String("project.source"); a.projectList == nil && (src == "" || src == "none") {
		return ""
	}
	if pane != "" && !strings.ContainsAny(pane, " ,=") {
		if c, ok := a.paneProjects[pane]; ok && time.Since(c.at) < 3*time.Second {
			if c.id != "" {
				return c.id
			}
		} else if m, e := a.hostPanes(context.Background(), pane); e == nil {
			if a.paneProjects == nil {
				a.paneProjects = map[string]paneProject{}
			}
			a.paneProjects[pane] = paneProject{m[pane], time.Now()}
			if m[pane] != "" {
				return m[pane]
			}
		}
	}
	if cwd == "" {
		cwd, _ = os.Getwd()
	}
	if cwd == "" {
		return ""
	}
	return projectForDir(a.projectIndex(), cwd)
}

func (a *App) hostPanes(ctx context.Context, id string) (map[string]string, error) {
	if a.paneList != nil {
		return a.paneList(ctx, id)
	}
	return hostPanesFrom(ctx, a.r.Settings().String("project.source"), id)
}

// linkProjects gives the store its views of the host's projects.
func (a *App) linkProjects() {
	a.store.ProjectRef = func(ref string) projectMatch {
		if strings.TrimSpace(ref) == "" {
			return projectMatch{Status: projectEmpty}
		}
		return resolveProjectRef(a.projectIndex(), ref)
	}
	a.store.ProjectDefault = a.defaultProject
	a.store.ProjectNameOf = func(id string) string {
		if id == "" {
			return ""
		}
		for _, p := range a.projectIndex() {
			if p.ID == id {
				return p.Name
			}
		}
		return ""
	}
}

// projectDisplay is the name a stored project is shown as: the host's name
// for the id, else the name kept on the question, else the stored value.
func (s *Store) projectDisplay(p Project) string {
	if p.ID == "" {
		return ""
	}
	if s.ProjectNameOf != nil {
		if n := s.ProjectNameOf(p.ID); n != "" {
			return n
		}
	}
	return p.Shown()
}

// Shown is the name kept on the question, else the stored value.
func (p Project) Shown() string {
	if p.Label != "" {
		return p.Label
	}
	return p.ID
}

// canonProject is the id a reference resolves to, or the reference itself.
func (s *Store) canonProject(ref string) string {
	if s.ProjectRef != nil {
		if m := s.ProjectRef(ref); m.Resolved {
			return m.ID
		}
	}
	return strings.TrimPrefix(strings.TrimSpace(ref), "mtm:")
}

// projectMatcher reports whether a question's project is the queried
// reference: by id, by name or by kind:id.
func (s *Store) projectMatcher(query string) func(Project) bool {
	q := s.canonProject(query)
	return func(p Project) bool {
		return p.ID != "" && (s.canonProject(p.ID) == q || p.Label != "" && strings.EqualFold(p.Label, strings.TrimSpace(query)))
	}
}

// normalizeProjects resolves the project of every question a write added or
// changed. A reference that resolves is stored as the id with the project's
// name beside it; one that does not stays as typed (and loses a name kept
// for the old project); an ambiguous one is stored as typed and reported.
func (s *Store) normalizeProjects(before, after []Question) {
	if s.ProjectRef == nil {
		return
	}
	old := map[string]string{}
	for _, q := range before {
		old[q.ID] = q.Project.ID
	}
	for i := range after {
		q := &after[i]
		if prev, ok := old[q.ID]; ok && prev == q.Project.ID || q.Project.ID == "" {
			continue
		}
		s.resolveProject(q)
	}
}

// resolveProject stores q's project as the id it resolves to, with the name.
func (s *Store) resolveProject(q *Question) {
	if s.ProjectRef == nil || q.Project.ID == "" {
		return
	}
	m := s.ProjectRef(q.Project.ID)
	if m.Resolved {
		q.Project.ID, q.Project.Label = m.ID, m.Name
		return
	}
	q.Project.Label = ""
	if len(m.IDs) > 0 && s.Warn != nil {
		s.Warn(fmt.Sprintf("project %q matches %d projects; question %s keeps it as typed", q.Project.ID, len(m.IDs), q.ID))
	}
}

// ProjectReport is what the migration found and did.
type ProjectReport struct {
	DryRun    bool          `json:"dry_run"`
	Rows      int           `json:"rows"`
	Empty     int           `json:"empty"`
	AlreadyID int           `json:"already_id"`
	KindToID  int           `json:"kind_id_to_id"`
	NameToID  int           `json:"name_to_id"`
	Rewritten int           `json:"rewritten"`
	Unmatched []ProjectKept `json:"unmatched"`
	Ambiguous []ProjectKept `json:"ambiguous"`
	Backup    string        `json:"backup,omitempty"`

	// Backfill: questions with no project and a recorded pane take that
	// pane's project where the host still knows the pane.
	PaneFilled  int    `json:"pane_filled"`
	PaneUnknown int    `json:"pane_unknown"`    // the host does not know the pane
	PaneNoProj  int    `json:"pane_no_project"` // the host knows the pane; it has no project
	NoPane      int    `json:"no_pane"`         // no pane recorded
	PanesError  string `json:"panes_error,omitempty"`
}

// ProjectKept is a reference left as stored, with its row count.
type ProjectKept struct {
	Name string   `json:"name"`
	Rows int      `json:"rows"`
	IDs  []string `json:"ids,omitempty"`
}

type splice struct {
	from, to int
	text     string
}

type rowFrame struct {
	obj   bool
	key   bool // the next string is a key
	cur   string
	keyAt int
}

// projectScan walks one question file (a JSON object) and finds its own
// project's id string and the edit that rewrites it, without touching a byte
// elsewhere.
type projectScan struct {
	b                []byte
	projects         []hostProject
	rep              *ProjectReport
	kept, amb        map[string]*ProjectKept
	stack            []rowFrame
	out              []splice
	obj, field, name string // the keys: the project object, its id, its label
	seen             bool
	hasLabel         bool
}

func (p *projectScan) stringStart(end int) int {
	for i := end - 2; i >= 0; i-- {
		if p.b[i] != '"' {
			continue
		}
		n := 0
		for j := i - 1; j >= 0 && p.b[j] == '\\'; j-- {
			n++
		}
		if n%2 == 0 {
			return i
		}
	}
	return 0
}

func (p *projectScan) valueDone() {
	if n := len(p.stack); n > 0 && p.stack[n-1].obj {
		p.stack[n-1].key = true
	}
}

// inProject: at a key of the question's project object.
func (p *projectScan) inProject() bool {
	return len(p.stack) == 2 && p.stack[1].obj && p.stack[0].cur == p.obj
}

func (p *projectScan) delim(t json.Delim) {
	if t == '{' || t == '[' {
		p.stack = append(p.stack, rowFrame{obj: t == '{', key: t == '{'})
		return
	}
	if len(p.stack) == 1 {
		p.rep.Rows++
		if !p.seen {
			p.rep.Empty++
		}
	}
	p.stack = p.stack[:len(p.stack)-1]
	p.valueDone()
}

func (p *projectScan) value(t string, end int) {
	p.seen = true
	m := resolveProjectRef(p.projects, t)
	switch {
	case m.Status == projectEmpty:
		p.rep.Empty++
	case m.Resolved && !m.Rewrite:
		p.rep.AlreadyID++
	case m.Resolved:
		p.rewrite(m, end)
	case len(m.IDs) > 0:
		tally(p.amb, t, m.IDs)
	default:
		tally(p.kept, t, nil)
	}
}

func tally(into map[string]*ProjectKept, name string, ids []string) {
	if into[name] == nil {
		into[name] = &ProjectKept{Name: name, IDs: ids}
	}
	into[name].Rows++
}

// rewrite splices the id over the value and, unless the project already has
// a label, the name in beside it, indented as the id key is.
func (p *projectScan) rewrite(m projectMatch, end int) {
	if m.Status == projectKindID {
		p.rep.KindToID++
	} else {
		p.rep.NameToID++
	}
	p.rep.Rewritten++
	text := `"` + m.ID + `"`
	if !p.labelAhead() {
		name, _ := json.Marshal(m.Name)
		ks := p.stack[1].keyAt
		nl := bytes.LastIndexByte(p.b[:ks], '\n')
		if nl >= 0 && len(bytes.TrimSpace(p.b[nl+1:ks])) == 0 {
			text += ",\n" + string(p.b[nl+1:ks]) + `"` + p.name + `": ` + string(name)
		} else {
			text += `,"` + p.name + `":` + string(name)
		}
	}
	p.out = append(p.out, splice{from: p.stringStart(end), to: end, text: text})
}

// labelAhead reports whether the project object already holds a label: the
// name stays as it is.
func (p *projectScan) labelAhead() bool {
	var probe struct {
		Project map[string]json.RawMessage `json:"project"`
	}
	_ = json.Unmarshal(p.b, &probe)
	_, ok := probe.Project[p.name]
	return ok
}

func (p *projectScan) token(tok json.Token, end int) {
	switch t := tok.(type) {
	case json.Delim:
		p.delim(t)
	case string:
		n := len(p.stack)
		if n > 0 && p.stack[n-1].obj && p.stack[n-1].key {
			p.stack[n-1].cur, p.stack[n-1].key, p.stack[n-1].keyAt = t, false, p.stringStart(end)
			return
		}
		if p.inProject() && p.stack[1].cur == p.field {
			p.value(t, end)
		}
		p.valueDone()
	default:
		p.valueDone()
	}
}

func projectSplices(b []byte, projects []hostProject, rep *ProjectReport, kept, amb map[string]*ProjectKept) ([]splice, error) {
	p := &projectScan{obj: "project", field: "id", name: "label", b: b, projects: projects, rep: rep, kept: kept, amb: amb}
	dec := json.NewDecoder(bytes.NewReader(b))
	for {
		tok, err := dec.Token()
		if errors.Is(err, io.EOF) {
			return p.out, nil
		}
		if err != nil {
			return nil, err
		}
		p.token(tok, int(dec.InputOffset()))
	}
}

func applySplices(b []byte, sp []splice) []byte {
	sort.Slice(sp, func(i, j int) bool { return sp[i].from > sp[j].from })
	out := append([]byte(nil), b...)
	for _, s := range sp {
		out = append(out[:s.from], append([]byte(s.text), out[s.to:]...)...)
	}
	return out
}

// paneFillSplice finds a question whose project is empty and whose source
// pane the host knows with a project, and gives the edit that writes that
// project into the empty object (id, and the name when the host lists it),
// indented as the file is. Nothing else in the file is touched.
func paneFillSplice(b []byte, projects []hostProject, panes map[string]string, rep *ProjectReport) ([]splice, error) {
	var probe struct {
		Project struct{ ID string }
		Source  struct{ Pane string }
	}
	if e := json.Unmarshal(b, &probe); e != nil {
		return nil, e
	}
	if probe.Project.ID != "" {
		return nil, nil
	}
	pane := probe.Source.Pane
	if pane == "" {
		rep.NoPane++
		return nil, nil
	}
	proj, known := panes[pane]
	switch {
	case !known:
		rep.PaneUnknown++
		return nil, nil
	case proj == "":
		rep.PaneNoProj++
		return nil, nil
	}
	// Find the top-level "project" object's braces.
	dec := json.NewDecoder(bytes.NewReader(b))
	var stack []rowFrame
	var open int
	ps := &projectScan{obj: "project", b: b}
	objKey := ps.obj
	done := func() {
		if n := len(stack); n > 0 && stack[n-1].obj {
			stack[n-1].key = true
		}
	}
	for {
		tok, e := dec.Token()
		if e != nil {
			return nil, nil
		}
		off := int(dec.InputOffset())
		switch t := tok.(type) {
		case json.Delim:
			if t == '{' || t == '[' {
				if len(stack) == 1 && stack[0].cur == objKey && t == '{' {
					open = off - 1
				}
				stack = append(stack, rowFrame{obj: t == '{', key: t == '{'})
				continue
			}
			if len(stack) == 2 && stack[0].cur == objKey {
				if len(bytes.TrimSpace(b[open+1:off-1])) > 0 {
					return nil, nil // fields but no id: left alone
				}
				rep.PaneFilled++
				return []splice{{from: open, to: off, text: projectObject(b, stack[0].keyAt, proj, projects)}}, nil
			}
			stack = stack[:len(stack)-1]
			done()
		case string:
			if n := len(stack); n > 0 && stack[n-1].obj && stack[n-1].key {
				stack[n-1].cur, stack[n-1].key, stack[n-1].keyAt = t, false, ps.stringStart(off)
				continue
			}
			done()
		default:
			done()
		}
	}
}

// projectObject writes {"id", "label"} in the file's own layout: one field
// per line, indented one step past the project key, when the key sits on a
// line of its own.
func projectObject(b []byte, keyAt int, id string, projects []hostProject) string {
	name := ""
	for _, p := range projects {
		if p.ID == id {
			name = p.Name
		}
	}
	idj, _ := json.Marshal(id)
	namej, _ := json.Marshal(name)
	nl := bytes.LastIndexByte(b[:keyAt], '\n')
	if nl < 0 || len(bytes.TrimSpace(b[nl+1:keyAt])) != 0 {
		if name == "" {
			return `{"id":` + string(idj) + `}`
		}
		return `{"id":` + string(idj) + `,"label":` + string(namej) + `}`
	}
	ind := string(b[nl+1 : keyAt])
	out := "{\n" + ind + "  \"id\": " + string(idj)
	if name != "" {
		out += ",\n" + ind + "  \"label\": " + string(namej)
	}
	return out + "\n" + ind + "}"
}

// ResolveProjects rewrites every question's project id that matches exactly
// one of projects to that project's id, keeping its name in project.label.
// References that match nothing or several, and every question without a
// project, stay byte for byte as they were. A dry run writes nothing. A real
// run first copies q/ to backup (dir + ".bak-<date>-project-ids"), refuses to
// write when that fails, and is idempotent: ids are not rewritten.
func (s *Store) ResolveProjects(projects []hostProject, panes map[string]string, dry bool) (ProjectReport, error) {
	rep := ProjectReport{DryRun: dry, Unmatched: []ProjectKept{}, Ambiguous: []ProjectKept{}}
	err := s.locked(func() error {
		files, e := filepath.Glob(filepath.Join(s.Dir, "q", "*.json"))
		if e != nil {
			return e
		}
		sort.Strings(files)
		kept, amb := map[string]*ProjectKept{}, map[string]*ProjectKept{}
		type edit struct {
			path string
			data []byte
			sp   []splice
		}
		var edits []edit
		for _, f := range files {
			b, e := os.ReadFile(f)
			if e != nil {
				return e
			}
			sp, e := projectSplices(b, projects, &rep, kept, amb)
			if e != nil {
				return fmt.Errorf("%s: %w", f, e)
			}
			if panes != nil {
				fill, e := paneFillSplice(b, projects, panes, &rep)
				if e != nil {
					return fmt.Errorf("%s: %w", f, e)
				}
				sp = append(sp, fill...)
			}
			if len(sp) > 0 {
				edits = append(edits, edit{path: f, data: b, sp: sp})
			}
		}
		for _, m := range kept {
			rep.Unmatched = append(rep.Unmatched, *m)
		}
		for _, m := range amb {
			rep.Ambiguous = append(rep.Ambiguous, *m)
		}
		for _, x := range [][]ProjectKept{rep.Unmatched, rep.Ambiguous} {
			sort.Slice(x, func(i, j int) bool {
				if x[i].Rows != x[j].Rows {
					return x[i].Rows > x[j].Rows
				}
				return x[i].Name < x[j].Name
			})
		}
		if dry || len(edits) == 0 {
			return nil
		}
		next := make([][]byte, len(edits))
		for i, ed := range edits {
			next[i] = applySplices(ed.data, ed.sp)
			if !json.Valid(next[i]) {
				return fmt.Errorf("%s: the rewrite is not valid JSON; nothing written", ed.path)
			}
		}
		rep.Backup = s.Dir + ".bak-" + s.now().Format("20060102") + "-project-ids"
		if e := copyTree(filepath.Join(s.Dir, "q"), filepath.Join(rep.Backup, "q")); e != nil {
			return fmt.Errorf("backup %s failed, nothing written: %w", rep.Backup, e)
		}
		for i, ed := range edits {
			if e := writeFileAtomic(ed.path, next[i]); e != nil {
				return e
			}
		}
		return nil
	})
	return rep, err
}

// copyTree copies the regular files directly in src (not its subfolders)
// into dst, which must not exist.
func copyTree(src, dst string) error {
	if _, e := os.Stat(dst); e == nil {
		return fmt.Errorf("%s already exists", dst)
	}
	if e := os.MkdirAll(dst, 0700); e != nil {
		return e
	}
	entries, e := os.ReadDir(src)
	if e != nil {
		return e
	}
	for _, en := range entries {
		if !en.Type().IsRegular() {
			continue
		}
		b, e := os.ReadFile(filepath.Join(src, en.Name()))
		if e != nil {
			return e
		}
		if e = writeFileAtomic(filepath.Join(dst, en.Name()), b); e != nil {
			return e
		}
	}
	return nil
}

func runResolveProjects(a *App, c ochazuke.Ctx) ochazuke.Result {
	list, e := a.hostProjects(context.Background())
	if e != nil {
		return result(nil, e)
	}
	if len(list) == 0 {
		return result(nil, errors.New("no projects from project.source (mtm); nothing to resolve against, nothing written"))
	}
	dry, _ := c.Args["dry-run"].(bool)
	panes, pe := a.hostPanes(context.Background(), "")
	if pe != nil || panes == nil {
		panes = nil
	}
	rep, e := a.store.ResolveProjects(list, panes, dry)
	if pe != nil {
		rep.PanesError = pe.Error()
	}
	return result(rep, e)
}

// defaultProjectOf gives q the project the host can name from its pane and
// the given directory when it has none; one path for every verb that
// creates a question. A project that was given is never replaced.
func (s *Store) defaultProjectOf(q *Question, cwd string) {
	if q.Project.ID != "" || s.ProjectDefault == nil {
		return
	}
	q.Project = Project{ID: s.ProjectDefault(q.Source.Pane, cwd)}
}
