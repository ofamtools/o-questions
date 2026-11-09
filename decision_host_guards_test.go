package main

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"ofam.dev/ochazuke"
	ocli "ofam.dev/ochazuke/cli"
)

// hostDecisions mounts the published Decisions page in a fresh host
// runtime the way mtm does: the catalog's nouns through the proxy, its
// binding rows of the decision noun with only their portable guards
// (ochazuke.PortableGuard; an application guard is left off, as the host
// cannot judge it), and the page.
func hostDecisions(t *testing.T, s *Store) *page {
	t.Helper()
	var out bytes.Buffer
	a, err := newApp(s, strings.NewReader(""), &out, &out)
	if err != nil {
		t.Fatal(err)
	}
	data, err := ocli.Catalog(a.r, "o-questions", version)
	if err != nil {
		t.Fatal(err)
	}
	var doc ocli.CatalogDoc
	if err = json.Unmarshal(data, &doc); err != nil {
		t.Fatal(err)
	}
	r := ochazuke.NewRuntime()
	r.Config().Disable()
	local := func(noun string) string { return "ext-" + noun }
	mounted := ocli.MountNouns(r, doc.AppNouns(), ocli.HostMount{
		Local: local,
		Proxy: func(noun string, verb ocli.CatalogVerb) any {
			return func(c ochazuke.Ctx) ochazuke.Result { return a.handle(verb.Name, c) }
		},
	})
	if len(mounted.Warnings) != 0 {
		t.Fatal(mounted.Warnings)
	}
	for _, row := range doc.Bindings {
		// The decision rows run question verbs through their decision aliases.
		if row.Noun != decisionNoun || row.Instance != "" || row.Request.Noun != decisionNoun && row.Request.Noun != "question" {
			continue
		}
		if !ochazuke.PortableGuard(row.When) {
			row.When = ""
		}
		row.Noun, row.Request.Noun = local(row.Noun), local(row.Request.Noun)
		if err := r.Bind(row); err != nil {
			t.Fatal(err)
		}
	}
	m := ochazuke.NewModel(r)
	m.Width, m.Height = 140, 40
	v, err := ocli.MountPage(m, doc, doc.Pages[1], ocli.PageConfig{Noun: local})
	if err != nil {
		t.Fatal(err)
	}
	p := &page{a: a, m: m, v: v, spec: decisionSpec(a)}
	a.changed = func() { p.dirty = true }
	if err = r.OpenPage(decisionPageID); err != nil {
		t.Fatal(err)
	}
	if err = p.reload(true); err != nil {
		t.Fatal(err)
	}
	if err = v.List(decisionListID).Focus(); err != nil {
		t.Fatal(err)
	}
	return p
}

// Mounted in a host, the Decisions page's keys keep to the rows they are
// for: l on a topic header is the tree's key, not a decision, and d d on a
// checklist item does not delete its question; on the question row both
// act.
func TestDecisionHostKeysKeepToTheirRows(t *testing.T) {
	s, q := checklistFixture(t)
	p := hostDecisions(t, s)
	decided := func() string { got, _ := s.Resolve(q.ID, false); return got.Decision.Action }
	deleted := func() bool { got, _ := s.Resolve(q.ID, false); return got.Deleted }

	ran := func(verb string) bool {
		for _, h := range p.m.Runtime.History(10) {
			if h.Request.Verb == verb {
				return true
			}
		}
		return false
	}

	selectRow(t, p, itemRowPrefix+q.ID+"/a")
	sendKey(t, p, "d")
	sendKey(t, p, "d")
	if modal := p.m.Runtime.TopModal(); modal != "" || ran("delete") || deleted() {
		t.Fatalf("d d on an item row acted on the question (modal %q):\n%s", modal, frameText(p))
	}

	selectRow(t, p, "topic/Git")
	sendKey(t, p, "l")
	if ran("decide") || decided() != "" {
		t.Fatalf("l on a topic row ran decide (status %q)", p.m.Status)
	}

	selectRow(t, p, "question/"+q.ID)
	sendKey(t, p, "l")
	if decided() != "later" {
		t.Fatalf("l on the question row is later, got %q", decided())
	}
	selectRow(t, p, "question/"+q.ID)
	sendKey(t, p, "d")
	sendKey(t, p, "d")
	if p.m.Runtime.TopModal() != "" {
		sendKey(t, p, "y")
	}
	if !deleted() {
		t.Fatalf("d d on the question row deletes it:\n%s", frameText(p))
	}
}
