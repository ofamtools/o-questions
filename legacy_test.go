package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// A question written before the lexicon renames keeps its decision block
// under "review", with the progress under "state". Reading migrates it to status; the next write
// of the file stores the new name.
func TestLegacyReviewBlockLoadsAsDecision(t *testing.T) {
	s := fixture(t)
	q := ask(t, s, "Turn the page on?")
	path := s.path(q)
	b, e := os.ReadFile(path)
	if e != nil {
		t.Fatal(e)
	}
	var doc map[string]any
	if e = json.Unmarshal(b, &doc); e != nil {
		t.Fatal(e)
	}
	doc["review"] = map[string]any{"group": "Review", "action": "yes", "state": "applied", "result": "done"}
	if b, e = json.Marshal(doc); e != nil {
		t.Fatal(e)
	}
	if e = os.WriteFile(path, b, 0600); e != nil {
		t.Fatal(e)
	}
	got, e := s.Resolve(q.ID, false)
	if e != nil || got.Decision == nil || got.Decision.Status != decisionApplied || got.Decision.Result != "done" {
		t.Fatalf("legacy decision not migrated: %+v %v", got.Decision, e)
	}
	if _, e = s.Change(opStar, q.ID, Change{}); e != nil {
		t.Fatal(e)
	}
	b, _ = os.ReadFile(path)
	if text := string(b); strings.Contains(text, `"state"`) || strings.Contains(text, `"review"`) || !strings.Contains(text, `"decision"`) || !strings.Contains(text, `"status": "applied"`) && !strings.Contains(text, `"status":"applied"`) {
		t.Fatalf("the write stores status: %s", filepath.Base(path))
	}
}

func TestLegacyUndoSnapshotLoads(t *testing.T) {
	b, e := migrateLegacy([]byte(`{"rows":[{"id":"x","review":{"state":"failed"}}],"action":"decide"}`))
	if e != nil {
		t.Fatal(e)
	}
	var snap Snapshot
	if e = json.Unmarshal(b, &snap); e != nil || snap.Rows[0].Decision.Status != decisionFailed {
		t.Fatalf("%v %+v", e, snap)
	}
}

// A record that already has a decision keeps it; a stray review key is left.
func TestLegacyReviewKeyKeepsExistingDecision(t *testing.T) {
	b, e := migrateLegacy([]byte(`{"review":{"action":"old"},"decision":{"action":"new"}}`))
	if e != nil {
		t.Fatal(e)
	}
	var doc struct {
		Decision struct{ Action string }
	}
	if e = json.Unmarshal(b, &doc); e != nil || doc.Decision.Action != "new" {
		t.Fatalf("%v %s", e, b)
	}
}
