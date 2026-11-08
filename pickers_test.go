package main

import (
	"bytes"
	"ofam.dev/ochazuke"
	"strings"
	"testing"
)

// The tool's noun pickers are declared, so its catalog lists them.
func TestDeclaredPickers(t *testing.T) {
	a, e := newApp(fixture(t), strings.NewReader(""), &bytes.Buffer{}, &bytes.Buffer{})
	if e != nil {
		t.Fatal(e)
	}
	got := map[string]bool{}
	for _, d := range a.r.CatalogPickerDefs() {
		got[d.Name] = d.Noun != "" && d.About != ""
	}
	for _, name := range []string{"question", "decision"} {
		if !got[name] {
			t.Errorf("catalog does not list the %s picker: %v", name, got)
		}
	}
}

// The decision verbs' target picker lists the decisions by title (not bare
// ids, not topic header rows), and the target is a Focus parameter so the
// selected row is the default.
func TestDecisionTargetPickerRowsCarryTitles(t *testing.T) {
	s := fixture(t)
	if _, e := agentStore(s).Pose(Pose{Question: "Ship the Decisions page?", Title: "Ship it", Group: "Decision", Recommendation: "Yes", Key: "k1"}); e != nil {
		t.Fatal(e)
	}
	a, _ := testApp(t, s)
	for _, verb := range []string{"decision.decision-note", "question.decision-note"} {
		op, ok := a.r.Operations.Find(ochazuke.Key(verb))
		if !ok {
			continue
		}
		p := op.Parameters["target"]
		if !p.Focus || p.Noun != "decision" {
			t.Errorf("%s target %+v: want a Focus decision parameter", verb, p)
		}
		choices, err := a.r.Choices(a.r.Context(), p)
		if err != nil || len(choices) != 1 {
			t.Fatalf("%s choices %v %v (topic rows must not be offered)", verb, choices, err)
		}
		row := choices[0].Row()
		if choices[0].Label != "Ship it" || !strings.HasPrefix(row.Label, "Ship it  q-") {
			t.Errorf("%s row %+v: want the title first, the short id after", verb, row)
		}
	}
}
