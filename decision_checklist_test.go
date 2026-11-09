package main

import (
	"bytes"
	"strings"
	"testing"
)

// mountDecisions mounts the Decisions page over s, wide and tall enough for
// the Options section.
func mountDecisions(t *testing.T, s *Store, settings map[string]any) (*App, *page) {
	t.Helper()
	a, e := newApp(s, strings.NewReader(""), &bytes.Buffer{}, &bytes.Buffer{})
	if e != nil {
		t.Fatal(e)
	}
	for k, v := range settings {
		if e = a.r.Settings().Set(k, v); e != nil {
			t.Fatal(e)
		}
	}
	p, e := mountSpec(a, decisionSpec(a))
	if e != nil {
		t.Fatal(e)
	}
	p.m.Width, p.m.Height = 140, 40
	return a, p
}

func checklistFixture(t *testing.T) (*Store, Question) {
	t.Helper()
	s := fixture(t)
	posed, e := agentStore(s).Pose(Pose{Question: "Which branches go?", Group: "Git", Items: []Item{{ID: "a", Label: "feat/alpha", Recommended: true}, {ID: "b", Label: "feat/beta"}, {ID: "c", Label: "feat/gamma"}}, Key: "c1"})
	if e != nil {
		t.Fatal(e)
	}
	return s, posed.Question
}

func selectRow(t *testing.T, p *page, id string) {
	t.Helper()
	if e := p.v.List(decisionListID).Select(decisionListID + "/" + id); e != nil {
		t.Fatal(e)
	}
}

func TestDecisionChecklistKeys(t *testing.T) {
	s, q := checklistFixture(t)
	_, p := mountDecisions(t, s, nil)
	frame := frameText(p)
	for _, want := range []string{"0/3", "[ ] feat/alpha", "[ ] feat/beta", "[ ] feat/gamma"} {
		if !strings.Contains(frame, want) {
			t.Fatalf("missing %q:\n%s", want, frame)
		}
	}
	item := func(id string) string { return "item/" + q.ID + "/" + id }
	checked := func() Decision { got, _ := s.Resolve(q.ID, false); return *got.Decision }
	selectRow(t, p, item("b"))
	sendKey(t, p, "c")
	if d := checked(); !d.checked("b") || d.checked("a") || d.DecidedAt != nil {
		t.Fatal("c on an item row must check that item only, and not decide the list", d.Picks)
	}
	if frame = frameText(p); !strings.Contains(frame, "[x] feat/beta") || !strings.Contains(frame, "1/3") || !strings.Contains(frame, "c check") {
		t.Fatal(frame)
	}
	sendKey(t, p, "c")
	if d := checked(); d.checked("b") {
		t.Fatal("c again must uncheck", d.Picks)
	}
	// A visual selection: c acts on every selected item (set all = V, move, c).
	selectRow(t, p, item("a"))
	sendKey(t, p, "V")
	sendKey(t, p, "j")
	sendKey(t, p, "c")
	if d := checked(); !d.checked("a") || !d.checked("b") || d.checked("c") {
		t.Fatal("c over a selection must check both rows", d.Picks)
	}
	sendKey(t, p, "esc")
	// D decides the whole checklist, from an item row as from the question row.
	selectRow(t, p, item("c"))
	sendKey(t, p, "D")
	if d := checked(); d.DecidedAt == nil {
		t.Fatal("D must confirm the checklist", d)
	}
	// x on an item row clears that item's mark (undecide, with the item the row names).
	selectRow(t, p, item("c"))
	sendKey(t, p, "c")
	if d := checked(); !d.checked("c") {
		t.Fatal("c checks the item", d.Picks)
	}
	sendKey(t, p, "x")
	if d := checked(); d.pickAt("c") >= 0 || d.DecidedAt == nil {
		t.Fatal("x on an item row clears its mark and leaves the list decided", d)
	}
}

func TestDecisionChecklistSearchFindsItems(t *testing.T) {
	s, q := checklistFixture(t)
	a, _ := mountDecisions(t, s, nil)
	rows, e := a.decisionRows(nil)
	if e != nil {
		t.Fatal(e)
	}
	for _, r := range rows {
		if r.ID == "question/"+q.ID {
			if !strings.Contains(r.Cells[len(r.Cells)-1], "feat/gamma") {
				t.Fatal("the question row's search cell must carry the item labels", r.Cells)
			}
			if r.Cells[0] != "0/3" {
				t.Fatal("the question row shows progress", r.Cells[0])
			}
		}
	}
	items := 0
	for _, r := range rows {
		if r.Parent == "question/"+q.ID {
			items++
		}
	}
	if items != 3 {
		t.Fatal("one child row per item", items)
	}
	flat, e := a.decisionRows(map[string]any{"flat": true})
	if e != nil {
		t.Fatal(e)
	}
	for _, r := range flat {
		if strings.HasPrefix(r.ID, itemRowPrefix) {
			t.Fatal("a flat answer (the target picker) lists questions only", r.ID)
		}
	}
}
