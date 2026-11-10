package main

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"ofam.dev/ochazuke/markdown"
)

// The Decisions page's document detail (setting decision.detail=document):
// every decision has a working Markdown file, rendered from the record, that
// ochazuke's markdown editor shows in the right pane (the Docs page's own
// component, so it takes focus and `x` flips a `- [ ]` line). The record is
// the source of truth: the file is a view. What the owner changes in the
// file (a box, the Note text) is read back through the decision verbs on the
// next sync (right after an edit when this process hosts the editor, else on
// the page's next refresh), then the file is rendered again from the record,
// so anything else typed is discarded.
//
// Format (one renderer, docRender; docParse reads it back):
//
//	# Title
//	<!-- decision:Q-ID -->
//	Question text.
//	## Your choice
//	- [ ] yes (recommended) <!-- option:yes -->
//	- [x] no <!-- option:no -->
//	## Other answers
//	- [ ] Later <!-- outcome:later -->
//	## Checklist
//	- [x] feat/alpha (recommended) <!-- item:a -->
//	## Note
//	free text
//
// Options and outcomes are one single choice. A decision that is applied is
// rendered without boxes: nothing in it reads back.

const (
	docChoiceHead = "## Your choice"
	docOtherHead  = "## Other answers"
	docItemsHead  = "## Checklist"
	docNoteHead   = "## Note"
)

var docMarker = regexp.MustCompile(`\s*<!--\s*(option|outcome|item):(.*?)\s*-->\s*$`)
var docItemMarker = regexp.MustCompile(`<!--\s*item:`)

// docDir is the tool's own cache of working files; nothing else lives there.
func docDir() string { return filepath.Join(stateDir(), "o-questions", "decision-md") }

func docFile(id string) string { return filepath.Join(docDir(), id+".md") }
func docBase(id string) string { return filepath.Join(docDir(), ".base", id+".md") }

func docBox(on bool) string {
	if on {
		return "[x]"
	}
	return "[ ]"
}

// docRender is the working file of one decision.
func (s *Store) docRender(q Question) string {
	r := q.Decision
	var b strings.Builder
	fmt.Fprintf(&b, "# %s\n\n", q.Title)
	if !r.locked() {
		b.WriteString("_Only the checkboxes and the Note section are read; anything else typed here is discarded on the next render._\n\n")
	}
	fmt.Fprintf(&b, "%s\n\n", strings.TrimSpace(q.Text))
	if strings.TrimSpace(r.Details) != "" {
		fmt.Fprintf(&b, "%s\n\n", strings.TrimSpace(r.Details))
	}
	if rec := s.recommendationView(q); rec != "" {
		fmt.Fprintf(&b, "## Recommendation\n\n%s\n\n", rec)
	}
	locked := r.locked()
	box := func(on bool) string {
		if locked {
			return map[bool]string{true: "✓", false: "·"}[on]
		}
		return docBox(on)
	}
	line := func(kind, id, label string, on bool, extra string) string {
		l := "- " + box(on) + " " + strings.ReplaceAll(label, "\n", " ") + extra
		if !locked {
			l += " <!-- " + kind + ":" + id + " -->"
		}
		return l + "\n"
	}
	rec := r.recommendedID()
	if opts := s.optionsOf(r); len(opts) > 0 {
		fmt.Fprintf(&b, "%s\n\n", docChoiceHead)
		for _, o := range opts {
			extra := ""
			if o.ID == rec && rec != "" {
				extra = " (recommended)"
			}
			b.WriteString(line("option", o.ID, o.label(), r.Action == o.ID, extra))
			if o.Description != "" {
				fmt.Fprintf(&b, "  %s\n", strings.ReplaceAll(strings.TrimSpace(o.Description), "\n", "\n  "))
			}
		}
		b.WriteString("\n")
	}
	if len(r.Items) > 0 {
		fmt.Fprintf(&b, "%s %s (checked means do it)\n\n", docItemsHead, r.progress())
		for _, it := range r.Items {
			extra := ""
			if it.Recommended {
				extra = " (recommended)"
			}
			b.WriteString(line("item", it.ID, it.label(), r.checked(it.ID), extra))
			if it.Description != "" {
				fmt.Fprintf(&b, "  %s\n", strings.ReplaceAll(strings.TrimSpace(it.Description), "\n", "\n  "))
			}
		}
		b.WriteString("\n")
	}
	fmt.Fprintf(&b, "%s\n\n", docOtherHead)
	for _, o := range s.outcomes() {
		extra := ""
		if o.NeedsNote {
			extra = " (type it under Note)"
		}
		b.WriteString(line("outcome", o.ID, o.Label, r.Action == o.ID, extra))
	}
	b.WriteString("\n")
	if locked {
		fmt.Fprintf(&b, "_Applied: %s_\n\n", strings.TrimSpace(r.Result))
	}
	fmt.Fprintf(&b, "%s\n\n", docNoteHead)
	if !locked {
		fmt.Fprintf(&b, "<!-- your answer for \"Write your own answer\" or a note for the agent; boxes and this section are read back, anything else is discarded -->\n\n")
	}
	if n := strings.TrimSpace(r.Note); n != "" {
		fmt.Fprintf(&b, "%s\n", n)
	}
	return b.String()
}

// docState is what docParse reads: the boxes and the Note text.
type docState struct {
	Choice  []string // checked option:/outcome: ids, in file order
	ChoiceN map[string]bool
	Items   map[string]bool
	Note    string
	Has     bool // the file carried at least one marker
}

func docParse(text string) docState {
	st := docState{ChoiceN: map[string]bool{}, Items: map[string]bool{}}
	inNote := false
	var note []string
	for _, ln := range strings.Split(text, "\n") {
		if strings.HasPrefix(ln, "## ") {
			inNote = strings.HasPrefix(ln, docNoteHead)
			continue
		}
		if inNote {
			note = append(note, ln)
			continue
		}
		m := docMarker.FindStringSubmatch(ln)
		if m == nil {
			continue
		}
		trim := strings.TrimSpace(ln)
		on := strings.HasPrefix(trim, "- [x]") || strings.HasPrefix(trim, "- [X]")
		if !on && !strings.HasPrefix(trim, "- [ ]") {
			continue
		}
		st.Has = true
		if docItemMarker.MatchString(ln) {
			st.Items[m[2]] = on
			continue
		}
		st.ChoiceN[m[1]+":"+m[2]] = true
		if on {
			st.Choice = append(st.Choice, m[2])
		}
	}
	var kept []string
	for _, l := range note {
		if strings.HasPrefix(strings.TrimSpace(l), "<!--") && strings.HasSuffix(strings.TrimSpace(l), "-->") {
			continue
		}
		kept = append(kept, l)
	}
	st.Note = strings.TrimSpace(strings.Join(kept, "\n"))
	return st
}

// docSync reads back one decision's working file (what changed since the
// version the renderer last wrote) through the decision verbs, then writes
// the file again from the record. It returns an error naming what the record
// refused; the file is rendered from the record either way.
func (s *Store) docSync(id string) error {
	q, e := s.Resolve(id, false)
	if e != nil || !q.forOwner() {
		return e
	}
	var errs []string
	cur, e1 := os.ReadFile(docFile(id))
	base, e2 := os.ReadFile(docBase(id))
	if e1 == nil && e2 == nil && string(cur) != string(base) && !q.Decision.locked() {
		errs = s.docApply(q, docParse(string(base)), docParse(string(cur)))
	}
	if q, e = s.Resolve(id, false); e != nil {
		return e
	}
	if e = s.docWrite(q); e != nil {
		return e
	}
	if len(errs) > 0 {
		return fmt.Errorf("%s", strings.Join(errs, "; "))
	}
	return nil
}

// docApply turns the difference between the last rendered file and the
// current one into decisions: item marks, the single choice (the last newly
// checked box wins; unchecking the chosen one clears it) and the note.
func (s *Store) docApply(q Question, base, cur docState) (errs []string) {
	do := func(d DecisionChange) {
		if _, e := s.Decide(q.ID, d); e != nil {
			errs = append(errs, e.Error())
		}
	}
	for _, it := range q.Decision.Items {
		if on, ok := cur.Items[it.ID]; ok && on != base.Items[it.ID] {
			do(DecisionChange{Item: it.ID, Checked: &on})
		}
	}
	noteChanged := cur.Note != base.Note
	note := cur.Note
	var fresh string
	for _, id := range cur.Choice {
		if !contains(base.Choice, id) {
			fresh = id
		}
	}
	switch {
	case fresh != "":
		d := DecisionChange{Action: fresh}
		if noteChanged {
			d.Note, noteChanged = &note, false
		}
		do(d)
	case len(base.Choice) > 0 && len(cur.Choice) == 0:
		do(DecisionChange{Clear: true})
	}
	if noteChanged {
		do(DecisionChange{Note: &note})
	}
	return errs
}

func contains(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}

// docWrite renders q's working file and the base copy the next read-back is
// diffed against, atomically and only when the text changed.
func (s *Store) docWrite(q Question) error {
	text := s.docRender(q)
	if e := os.MkdirAll(filepath.Join(docDir(), ".base"), 0o700); e != nil {
		return e
	}
	for _, p := range []string{docBase(q.ID), docFile(q.ID)} {
		if old, e := os.ReadFile(p); e == nil && string(old) == text {
			continue
		}
		if e := writeFileAtomic(p, []byte(text)); e != nil {
			return e
		}
	}
	return nil
}

func writeFileAtomic(path string, data []byte) error {
	tmp, e := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".*")
	if e != nil {
		return e
	}
	name := tmp.Name()
	_, e = tmp.Write(data)
	if ce := tmp.Close(); e == nil {
		e = ce
	}
	if e == nil {
		e = os.Rename(name, path)
	}
	if e != nil {
		_ = os.Remove(name)
	}
	return e
}

// docTopic is a topic row's file: a read-only summary the editor shows.
func docTopicFile(group string) string {
	safe := regexp.MustCompile(`[^A-Za-z0-9._-]+`).ReplaceAllString(group, "_")
	return filepath.Join(docDir(), "topic-"+safe+".md")
}

// docPrune removes working files (and bases) of decisions not in keep: the
// ones no longer shown, archived or gone. Only this directory is touched.
func docPrune(keep map[string]bool) {
	for _, dir := range []string{docDir(), filepath.Join(docDir(), ".base")} {
		entries, e := os.ReadDir(dir)
		if e != nil {
			continue
		}
		for _, f := range entries {
			if f.IsDir() || !strings.HasSuffix(f.Name(), ".md") {
				continue
			}
			if !keep[strings.TrimSuffix(f.Name(), ".md")] {
				_ = os.Remove(filepath.Join(dir, f.Name()))
			}
		}
	}
}

// docPending lists the decisions whose working file differs from the version
// last rendered (the owner edited it), without touching the store.
func docPending() []string {
	entries, _ := os.ReadDir(docDir())
	var out []string
	for _, f := range entries {
		id, ok := strings.CutSuffix(f.Name(), ".md")
		if f.IsDir() || !ok || strings.HasPrefix(id, "topic-") {
			continue
		}
		cur, e1 := os.ReadFile(docFile(id))
		base, e2 := os.ReadFile(docBase(id))
		if e1 == nil && e2 == nil && string(cur) != string(base) {
			out = append(out, id)
		}
	}
	return out
}

// docSyncAll reads back every edited working file. The error names what the
// record refused; the files are rendered from the record regardless.
func (s *Store) docSyncAll() error {
	var errs []string
	for _, id := range docPending() {
		if e := s.docSync(id); e != nil {
			errs = append(errs, e.Error())
		}
	}
	if len(errs) > 0 {
		return fmt.Errorf("%s", strings.Join(errs, "; "))
	}
	return nil
}

// docTopicText is a topic row's read-only summary document.
func docTopicText(body string) string { return body + "\n" }

// docSource is the editor's source for a working file in this process (the
// standalone page): a save is read back at once, so the record changes with
// the edit; a host that draws the file itself (mtm) reads it back on the
// page's next refresh.
func (a *App) docSource(path string) markdown.Source {
	src := markdown.File(path)
	save := src.Save
	src.Save = func(text string) error {
		if e := save(text); e != nil {
			return e
		}
		if id, ok := strings.CutSuffix(filepath.Base(path), ".md"); ok && filepath.Dir(path) == docDir() && !strings.HasPrefix(id, "topic-") {
			if e := a.store.docSync(id); e != nil {
				a.r.UI().Status("not applied from the document: " + e.Error())
			}
			a.refresh()
		}
		return nil
	}
	return src
}
