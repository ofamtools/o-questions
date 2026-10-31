package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"
)

func jsonDecode(b []byte, v any) error { return json.Unmarshal(b, v) }
func safeQuestion(q Question) bool {
	if q.FileName != "" && (strings.ContainsAny(q.FileName, "/\\") || !strings.HasSuffix(q.FileName, ".json")) {
		return false
	}
	if !strings.HasPrefix(q.ID, "q-") || strings.ContainsAny(q.ID, "/\\") || q.Created.IsZero() {
		return false
	}
	for _, en := range q.Entries {
		if strings.ContainsAny(en.File, "/\\") || en.File == ".." {
			return false
		}
	}
	return true
}

type Annotation struct {
	Tags    []string `json:"tags"`
	Links   []Link   `json:"links"`
	Project string   `json:"project"`
	Task    string   `json:"task"`
	Title   string   `json:"title"`
}

func addUnique[T comparable](old, more []T) []T {
	for _, v := range more {
		if !slices.Contains(old, v) {
			old = append(old, v)
		}
	}
	return old
}
func (s *Store) annotate(q *Question, a Annotation) error {
	if s.Actor.Kind != "owner" {
		if a.Project != "" && q.Project.ID != "" && q.Project.ID != a.Project {
			return &RefusedError{"associate", "project.already-associated"}
		}
		if a.Task != "" && q.Task != "" && q.Task != a.Task {
			return &RefusedError{"associate", "task.already-associated"}
		}
		if a.Title != "" && a.Title != q.Title && q.OwnerRenamed {
			return &RefusedError{"rename", "title.owner-renamed"}
		}
	}
	q.Tags = addUnique(q.Tags, a.Tags)
	q.Links = addUnique(q.Links, a.Links)
	if a.Project != "" {
		q.Project.ID = a.Project
	}
	if a.Task != "" {
		q.Task = a.Task
	}
	if a.Title != "" {
		q.Title = a.Title
		if s.Actor.Kind == "owner" {
			q.OwnerRenamed = true
		}
	}
	return nil
}
func (s *Store) Annotate(target string, a Annotation) (Question, error) {
	return s.mutate("annotate", target, func(q *Question, _ *[]Question) error { return s.annotate(q, a) })
}

type Change struct {
	Value      string
	Entry      string
	Other      string
	Annotation Annotation
}

func (s *Store) Change(action, target string, c Change) (Question, error) {
	return s.mutate(action, target, func(q *Question, rows *[]Question) error {
		switch action {
		case "annotate", "associate", "tag", "link", "rename":
			return s.annotate(q, c.Annotation)
		case "untag":
			q.Tags = slices.DeleteFunc(q.Tags, func(v string) bool { return v == c.Value })
		case "unlink":
			q.Links = slices.DeleteFunc(q.Links, func(v Link) bool { return v.Target == c.Value })
		case "star":
			q.Starred = !q.Starred
		case "unstar":
			q.Starred = false
		case "delete":
			q.Deleted = true
		case "edit":
			if strings.TrimSpace(c.Value) == "" {
				return errors.New("question must not be empty")
			}
			q.Text = c.Value
			q.Verbatim = "unchecked"
		case "status", "reopen", "moot":
			status := c.Value
			if action == "reopen" {
				status = "open"
			}
			if action == "moot" {
				status = "moot"
			}
			if !slices.Contains(statuses, status) {
				return fmt.Errorf("unknown status %q", status)
			}
			q.Status = status
			q.StatusExplicit = true
		case "merge":
			other, e := resolve(*rows, c.Other)
			if e != nil {
				return e
			}
			if other.ID == q.ID {
				return errors.New("cannot merge a question into itself")
			}
			if e = s.hydrate(&other); e != nil {
				return e
			}
			if e = s.appendEntry(q, Entry{Kind: "note", Text: other.Text, Summary: other.Title, Links: []Link{{Kind: "question", Target: other.ID}}}); e != nil {
				return e
			}
			q.Entries = append(q.Entries, other.Entries...)
			q.Tags = addUnique(q.Tags, other.Tags)
			q.Links = addUnique(q.Links, other.Links)
			for i := range *rows {
				if (*rows)[i].ID == other.ID {
					(*rows)[i].Deleted = true
				}
			}
		case "split":
			if e := s.hydrate(q); e != nil {
				return e
			}
			idx := slices.IndexFunc(q.Entries, func(en Entry) bool { return en.ID == c.Entry })
			if idx < 0 {
				return errors.New("entry not found")
			}
			en := q.Entries[idx]
			id, e := token()
			if e != nil {
				return e
			}
			newQ := Question{ID: "q-" + id, Title: en.Summary, Text: en.Text, Verbatim: "unchecked", AskedBy: Owner(), RecordedBy: s.Actor, Created: s.now(), Updated: s.now(), AskedAt: en.At, Status: "open", Parent: q.ID, Project: q.Project, Cwd: q.Cwd, Source: q.Source, Key: "split:" + q.ID + ":" + en.ID, Entries: []Entry{}}
			if newQ.Title == "" {
				newQ.Title = string([]rune(en.Text)[:min(80, len([]rune(en.Text)))])
			}
			for _, v := range *rows {
				if v.Key == newQ.Key {
					return errors.New("entry already split")
				}
			}
			newQ.Created = s.now()
			for _, old := range *rows {
				if !newQ.Created.After(old.Created) {
					newQ.Created = old.Created.Add(time.Nanosecond)
				}
			}
			newQ.FileName = newQ.Created.Format("20060102T150405.000000000Z") + "-" + slug(newQ.Title) + "-" + newQ.ID + ".json"
			*rows = append(*rows, newQ)
			q.Links = append(q.Links, Link{Kind: "question", Target: newQ.ID, Label: "split entry " + en.ID})
		default:
			return fmt.Errorf("unsupported change %q", action)
		}
		return nil
	})
}
