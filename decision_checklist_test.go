package main

import "testing"

func checklistFixture(t *testing.T) (*Store, Question) {
	t.Helper()
	s := fixture(t)
	posed, e := agentStore(s).Pose(Pose{
		Question: "Which branches go?",
		Group:    "Git",
		Items:    []Item{{ID: "a", Label: "feat/alpha", Recommended: true}, {ID: "b", Label: "feat/beta"}, {ID: "c", Label: "feat/gamma"}},
		Key:      "c1",
	})
	if e != nil {
		t.Fatal(e)
	}
	return s, posed.Question
}
