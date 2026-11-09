package main

import (
	"encoding/json"
	"strings"
	"testing"
)

// pose --options/--items take a JSON array of strings or objects (the
// framework's array flag), through the verb layer as the CLI runs it.
func TestPoseOptionsAndItemsFlags(t *testing.T) {
	s := agentStore(fixture(t))
	a, out := testApp(t, s)
	e := dispatch(a, []string{"question", "pose", "Which?", "--options", `["Leave it",{"id":"go","label":"Go ahead","description":"do it now"}]`, "--items", `["feat/a",{"id":"b","label":"feat/b","recommended":true}]`, "--recommended", "go", "--key", "k1"})
	if e != nil {
		t.Fatalf("%v\n%s", e, out)
	}
	var got struct{ Question Question }
	if e = json.Unmarshal(out.Bytes(), &got); e != nil {
		t.Fatalf("%v\n%s", e, out)
	}
	r := got.Question.Decision
	if r == nil || len(r.Options) != 2 || r.Options[0].ID != "Leave it" || r.Options[0].label() != "Leave it" || r.Options[1].ID != "go" || r.Options[1].Description != "do it now" || r.Recommended != "go" {
		t.Fatal("options as strings and objects", r)
	}
	if len(r.Items) != 2 || r.Items[0].ID != "feat/a" || r.Items[1].label() != "feat/b" || !r.Items[1].Recommended || r.Items[0].Recommended {
		t.Fatal("items as strings and objects", r.Items)
	}
	// The same through --json.
	out.Reset()
	if e = dispatch(a, []string{"question", "pose", "--json", `{"question":"Q2?","options":["x",{"id":"y"}],"items":["i1"],"key":"k2"}`}); e != nil {
		t.Fatalf("%v\n%s", e, out)
	}
	if !strings.Contains(out.String(), `"id": "y"`) || !strings.Contains(out.String(), `"id": "i1"`) {
		t.Fatal(out.String())
	}
}

// decide needs no action for an item or a confirm (a decide call names one
// thing: an option, an item, accept, confirm, clear or a note).
func TestDecideItemAndConfirmNeedNoAction(t *testing.T) {
	s, q := checklistFixture(t)
	owner := &Store{Dir: s.Dir, Actor: Owner(), Now: s.Now}
	a, out := testApp(t, owner)
	if e := dispatch(a, []string{"question", "decide", "question/" + q.ID, "--item", "b", "--checked", "true"}); e != nil {
		t.Fatalf("decide --item: %v\n%s", e, out)
	}
	got, _ := s.Resolve(q.ID, false)
	if !got.Decision.checked("b") || got.Decision.DecidedAt != nil {
		t.Fatal(got.Decision)
	}
	if e := dispatch(a, []string{"question", "decide", "question/" + q.ID, "--confirm"}); e != nil {
		t.Fatalf("decide --confirm: %v\n%s", e, out)
	}
	if got, _ = s.Resolve(q.ID, false); got.Decision.DecidedAt == nil {
		t.Fatal("confirm decides the list", got.Decision)
	}
	out.Reset()
	if e := dispatch(a, []string{"decision", "toggle", "question/" + q.ID, "--item", "a"}); e != nil {
		t.Fatalf("toggle: %v\n%s", e, out)
	}
	if got, _ = s.Resolve(q.ID, false); !got.Decision.checked("a") || !got.Decision.checked("b") {
		t.Fatal("toggle flips an item", got.Decision.Picks)
	}
	// A host passes the focused row's id as the target: an item row names both.
	out.Reset()
	if e := dispatch(a, []string{"decision", "toggle", "--target", "item/" + q.ID + "/c"}); e != nil {
		t.Fatalf("toggle on an item row id: %v\n%s", e, out)
	}
	if got, _ = s.Resolve(q.ID, false); !got.Decision.checked("c") {
		t.Fatal("an item row id as the target", got.Decision.Picks)
	}
}

func TestOutcomeKeyRows(t *testing.T) {
	for name, row := range map[string]string{
		"a number":     "x|X|||| 3",
		"a second key": "x|X||||ab",
		"a function":   "x|X||||f5",
		"a duplicate":  "later|Again||||l",
	} {
		rows := append([]string{"later|Later|answered|later||l"}, row)
		if _, e := parseOutcomes(rows); e == nil {
			t.Errorf("%s: row %q accepted", name, row)
		}
	}
	got, e := parseOutcomes(defaultOutcomeRows)
	if e != nil {
		t.Fatal(e)
	}
	keys := map[string]string{}
	for _, o := range got {
		keys[o.ID] = o.Key
	}
	if keys["later"] != "l" || keys["moot"] != "m" || keys["ask"] != "?" || keys["other"] != "w" {
		t.Fatal(keys)
	}
	if _, e := parseOutcomes([]string{"later|Later|answered|later|"}); e != nil {
		t.Fatal("rows without a key stay valid", e)
	}
}
