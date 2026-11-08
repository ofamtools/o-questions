package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// A question written before the lexicon rename keeps the review block's
// progress under "state". Reading migrates it to status; the next write
// of the file stores the new name.
func TestLegacyReviewStateLoadsAsStatus(t *testing.T) {
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
	if e != nil || got.Review == nil || got.Review.Status != reviewApplied || got.Review.Result != "done" {
		t.Fatalf("legacy review not migrated: %+v %v", got.Review, e)
	}
	if _, e = s.Change(opStar, q.ID, Change{}); e != nil {
		t.Fatal(e)
	}
	b, _ = os.ReadFile(path)
	if text := string(b); strings.Contains(text, `"state"`) || !strings.Contains(text, `"status": "applied"`) && !strings.Contains(text, `"status":"applied"`) {
		t.Fatalf("the write stores status: %s", filepath.Base(path))
	}
}

func TestLegacyUndoSnapshotLoads(t *testing.T) {
	b, e := migrateLegacy([]byte(`{"rows":[{"id":"x","review":{"state":"failed"}}],"action":"decide"}`))
	if e != nil {
		t.Fatal(e)
	}
	var snap Snapshot
	if e = json.Unmarshal(b, &snap); e != nil || snap.Rows[0].Review.Status != reviewFailed {
		t.Fatalf("%v %+v", e, snap)
	}
}
