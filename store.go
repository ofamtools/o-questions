package main

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"golang.org/x/sys/unix"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"
)

// One file per question, serialized across processes as in o-cmds/store.go.
// A write-ahead transaction makes multi-record merge/split/undo recoverable.
type Store struct {
	Dir             string
	Now             func() time.Time
	Actor           Actor
	MaxBytes        int
	StatusFromEntry map[string]string
	BackupDaily     bool
	BackupKeep      int
}

func (s *Store) now() time.Time {
	if s.Now != nil {
		return s.Now().UTC()
	}
	return time.Now().UTC()
}
func (s *Store) locked(fn func() error) error {
	if e := os.MkdirAll(filepath.Join(s.Dir, "q"), 0700); e != nil {
		return e
	}
	f, e := os.OpenFile(filepath.Join(s.Dir, ".lock"), os.O_CREATE|os.O_RDWR, 0600)
	if e != nil {
		return e
	}
	defer f.Close()
	if e = unix.Flock(int(f.Fd()), unix.LOCK_EX); e != nil {
		return e
	}
	defer unix.Flock(int(f.Fd()), unix.LOCK_UN)
	if b, e := os.ReadFile(filepath.Join(s.Dir, ".transaction.json")); e == nil {
		var rows []Question
		if e = json.Unmarshal(b, &rows); e != nil {
			return e
		}
		if e = s.apply(rows); e != nil {
			return e
		}
	} else if !errors.Is(e, os.ErrNotExist) {
		return e
	}
	return fn()
}
func (s *Store) path(q Question) string {
	return filepath.Join(s.Dir, "q", q.Created.Format("20060102T150405.000000000Z")+"-"+q.ID+".json")
}
func (s *Store) all() ([]Question, error) {
	files, e := os.ReadDir(filepath.Join(s.Dir, "q"))
	if e != nil {
		return nil, e
	}
	out := []Question{}
	for _, f := range files {
		if f.IsDir() || !strings.HasSuffix(f.Name(), ".json") || strings.HasPrefix(f.Name(), ".") {
			continue
		}
		b, e := os.ReadFile(filepath.Join(s.Dir, "q", f.Name()))
		if e != nil {
			return nil, e
		}
		var q Question
		if e = json.Unmarshal(b, &q); e != nil {
			return nil, fmt.Errorf("%s: %w", f.Name(), e)
		}
		out = append(out, q)
	}
	slices.SortFunc(out, func(a, b Question) int {
		if n := a.Created.Compare(b.Created); n != 0 {
			return n
		}
		return strings.Compare(a.ID, b.ID)
	})
	return out, nil
}
func (s *Store) hydrate(q *Question) error {
	for i := range q.Entries {
		e := &q.Entries[i]
		if e.File != "" {
			if filepath.Base(e.File) != e.File {
				return errors.New("invalid entry file")
			}
			b, err := os.ReadFile(filepath.Join(s.Dir, "q", e.File))
			if err != nil {
				return err
			}
			e.Text = string(b)
		}
	}
	return nil
}
func (s *Store) List() (rows []Question, err error) {
	err = s.locked(func() error {
		all, e := s.all()
		if e != nil {
			return e
		}
		rows = []Question{}
		for _, q := range all {
			if !q.Deleted {
				if e = s.hydrate(&q); e != nil {
					return e
				}
				rows = append(rows, q)
			}
		}
		return nil
	})
	return
}
func resolve(rows []Question, target string) (Question, error) {
	target = strings.TrimPrefix(target, "question/")
	for _, q := range rows {
		if q.ID == target || q.Title == target {
			return q, nil
		}
	}
	if n, e := strconv.Atoi(target); e == nil {
		if n > 0 && n <= len(rows) {
			return rows[n-1], nil
		}
		return Question{}, fmt.Errorf("question number %s not found", target)
	}
	var matches []Question
	for _, q := range rows {
		if strings.HasPrefix(q.ID, target) || strings.HasPrefix(q.Title, target) {
			matches = append(matches, q)
		}
	}
	if len(matches) == 1 {
		return matches[0], nil
	}
	if len(matches) > 1 {
		return Question{}, fmt.Errorf("ambiguous question %q (%d matches)", target, len(matches))
	}
	return Question{}, fmt.Errorf("question %q not found", target)
}
func (s *Store) Resolve(target string, newest bool) (Question, error) {
	rows, e := s.List()
	if e != nil {
		return Question{}, e
	}
	if newest {
		slices.Reverse(rows)
	}
	return resolve(rows, target)
}
func (s *Store) apply(rows []Question) error {
	for _, q := range rows {
		if e := atomicJSON(s.path(q), q); e != nil {
			return e
		}
	}
	return os.Remove(filepath.Join(s.Dir, ".transaction.json"))
}

type Snapshot struct {
	Rows   []Question `json:"rows"`
	At     time.Time  `json:"at"`
	Action string     `json:"action"`
	By     Actor      `json:"by"`
	Undone bool       `json:"undone"`
}

func (s *Store) history() ([]string, error) {
	files, e := filepath.Glob(filepath.Join(s.Dir, ".history", "*.json"))
	return files, e
}
func (s *Store) commit(before, after []Question, action string) error {
	if s.BackupDaily {
		if _, e := s.backup(filepath.Join(s.Dir, "backups"), true); e != nil {
			return e
		}
	}
	if e := os.MkdirAll(filepath.Join(s.Dir, ".history"), 0700); e != nil {
		return e
	}
	files, e := s.history()
	if e != nil {
		return e
	}
	next := 1
	if len(files) > 0 {
		n, _ := strconv.Atoi(strings.TrimSuffix(filepath.Base(files[len(files)-1]), ".json"))
		next = n + 1
	}
	if e = atomicJSON(filepath.Join(s.Dir, ".history", fmt.Sprintf("%020d.json", next)), Snapshot{Rows: before, At: s.now(), Action: action, By: s.Actor}); e != nil {
		return e
	}
	return s.write(after)
}
func (s *Store) write(rows []Question) error {
	if e := atomicJSON(filepath.Join(s.Dir, ".transaction.json"), rows); e != nil {
		return e
	}
	return s.apply(rows)
}
func clone(rows []Question) []Question {
	b, _ := json.Marshal(rows)
	var out []Question
	_ = json.Unmarshal(b, &out)
	return out
}
func (s *Store) mutate(action, target string, fn func(*Question, *[]Question) error) (q Question, err error) {
	err = s.locked(func() error {
		rows, e := s.all()
		if e != nil {
			return e
		}
		before := clone(rows)
		visible := []Question{}
		for _, q := range rows {
			if !q.Deleted {
				visible = append(visible, q)
			}
		}
		q, e = resolve(visible, target)
		if e != nil {
			return e
		}
		if e = s.permit(action, q); e != nil {
			return e
		}
		if e = fn(&q, &rows); e != nil {
			return e
		}
		q.Updated = s.now()
		for i := range rows {
			if rows[i].ID == q.ID {
				rows[i] = q
			}
		}
		return s.commit(before, rows, action)
	})
	if err == nil {
		err = s.hydrate(&q)
	}
	return
}

type Ask struct {
	Question   string   `json:"question"`
	Title      string   `json:"title"`
	Project    string   `json:"project"`
	Cwd        string   `json:"cwd"`
	Task       string   `json:"task"`
	Tags       []string `json:"tags"`
	Links      []Link   `json:"links"`
	AskedAt    string   `json:"asked_at"`
	FollowUpOf string   `json:"follow_up_of"`
	Key        string   `json:"key"`
	Source     Source   `json:"source"`
}
type Asked struct {
	Question Question `json:"question"`
	Existed  bool     `json:"existed"`
}

func (s *Store) Ask(a Ask) (out Asked, err error) {
	if strings.TrimSpace(a.Question) == "" {
		return out, errors.New("question must not be empty")
	}
	at := s.now()
	if a.AskedAt != "" {
		at, err = time.Parse(time.RFC3339, a.AskedAt)
		if err != nil {
			return
		}
	}
	if a.Key == "" {
		if a.Source.AgentSession != "" && a.Source.Turn != "" {
			a.Key = s.Actor.Harness + ":" + a.Source.AgentSession + ":" + a.Source.Turn
		} else {
			a.Key = fmt.Sprintf("%x", sha256.Sum256([]byte(a.Question+at.UTC().Truncate(time.Minute).String())))
		}
	}
	err = s.locked(func() error {
		rows, e := s.all()
		if e != nil {
			return e
		}
		for _, q := range rows {
			if q.Key == a.Key {
				out = Asked{q, true}
				return nil
			}
			for _, en := range q.Entries {
				if en.Key == a.Key {
					out = Asked{q, true}
					return nil
				}
			}
		}
		if e = s.permit("ask", Question{}); e != nil {
			return e
		}
		before := clone(rows)
		if a.FollowUpOf != "" {
			q, e := resolve(rows, a.FollowUpOf)
			if e != nil {
				return e
			}
			en := Entry{Kind: "follow-up", Text: a.Question, Key: a.Key}
			if e = s.appendEntry(&q, en); e != nil {
				return e
			}
			for i := range rows {
				if rows[i].ID == q.ID {
					rows[i] = q
				}
			}
			out.Question = q
			return s.commit(before, rows, "ask")
		}
		id, e := token()
		if e != nil {
			return e
		}
		if a.Title == "" {
			r := []rune(strings.Split(a.Question, "\n")[0])
			a.Title = string(r[:min(len(r), 80)])
		}
		if a.Cwd != "" {
			a.Cwd, e = filepath.Abs(a.Cwd)
			if e != nil {
				return e
			}
		}
		now := s.now()
		q := Question{ID: "q-" + id, Title: a.Title, Text: a.Question, Verbatim: "unchecked", AskedBy: Owner(), RecordedBy: s.Actor, AskedAt: at, Created: now, Updated: now, Status: "open", Project: Project{ID: a.Project}, Cwd: a.Cwd, Task: a.Task, Source: a.Source, Tags: a.Tags, Links: a.Links, Key: a.Key, Entries: []Entry{}}
		out.Question = q
		rows = append(rows, q)
		return s.commit(before, rows, "ask")
	})
	if err == nil {
		err = s.hydrate(&out.Question)
	}
	return
}
func (s *Store) appendEntry(q *Question, en Entry) error {
	if strings.TrimSpace(en.Text) == "" {
		return errors.New("entry text must not be empty")
	}
	table := s.StatusFromEntry
	if table == nil {
		table = entryStatuses
	}
	if _, ok := table[en.Kind]; !ok {
		return fmt.Errorf("unknown entry kind %q", en.Kind)
	}
	if en.Replaces != "" {
		found := false
		for _, old := range q.Entries {
			found = found || old.ID == en.Replaces
		}
		if !found {
			return errors.New("correction target entry not found")
		}
	}
	id, e := token()
	if e != nil {
		return e
	}
	en.ID = "e-" + id
	en.By = s.Actor
	en.At = s.now()
	if en.Kind == "follow-up" {
		en.By = Owner()
	}
	limit := s.MaxBytes
	if limit == 0 {
		limit = 65536
	}
	if len(en.Text) > limit {
		en.File = q.ID + "-" + en.ID + ".txt"
		if e = atomicBytes(filepath.Join(s.Dir, "q", en.File), []byte(en.Text)); e != nil {
			return e
		}
		en.Links = append(en.Links, Link{Kind: "file", Target: filepath.Join(s.Dir, "q", en.File), Label: "full entry"})
		en.Text = ""
	}
	q.Entries = append(q.Entries, en)
	if !q.StatusExplicit && table[en.Kind] != "" {
		q.Status = table[en.Kind]
	}
	q.Updated = s.now()
	return nil
}
func (s *Store) Append(target string, en Entry) (Question, error) {
	action := en.Kind
	if action == "correction" {
		action = "correct"
	}
	return s.mutate(action, target, func(q *Question, _ *[]Question) error { return s.appendEntry(q, en) })
}
func (s *Store) Undo() (err error) {
	if err = s.permit("undo", Question{}); err != nil {
		return
	}
	return s.locked(func() error {
		files, e := s.history()
		if e != nil {
			return e
		}
		for i := len(files) - 1; i >= 0; i-- {
			b, e := os.ReadFile(files[i])
			if e != nil {
				return e
			}
			var snap Snapshot
			if e = json.Unmarshal(b, &snap); e != nil {
				return e
			}
			if snap.Undone {
				continue
			}
			current, e := s.all()
			if e != nil {
				return e
			}
			ids := map[string]bool{}
			for _, q := range snap.Rows {
				ids[q.ID] = true
			}
			for _, q := range current {
				if !ids[q.ID] {
					q.Deleted = true
					snap.Rows = append(snap.Rows, q)
				}
			}
			if e = s.write(snap.Rows); e != nil {
				return e
			}
			snap.Undone = true
			return atomicJSON(files[i], snap)
		}
		return errors.New("nothing to undo")
	})
}
func (s *Store) History() (out []Snapshot, err error) {
	err = s.locked(func() error {
		files, e := s.history()
		if e != nil {
			return e
		}
		out = []Snapshot{}
		for _, f := range files {
			b, e := os.ReadFile(f)
			if e != nil {
				return e
			}
			var snap Snapshot
			if e = json.Unmarshal(b, &snap); e != nil {
				return e
			}
			out = append(out, snap)
		}
		return nil
	})
	return
}
