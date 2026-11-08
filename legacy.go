package main

import (
	"bytes"
	"encoding/json"
	"strings"
)

// migrateLegacy reads question data written before the lexicon renames. A
// question's decision block was stored under "review" (v0.2.0 and earlier),
// and before that held its progress under "state" (empty, applied, failed),
// now "status". It is a data migration, not a name alias: the next write of
// the file stores the new names. Text without a legacy key is returned as is.
func migrateLegacy(b []byte) ([]byte, error) {
	if !bytes.Contains(b, []byte(`"state"`)) && !bytes.Contains(b, []byte(`"review"`)) && !bytes.Contains(b, []byte(`"note":`)) {
		return b, nil
	}
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.UseNumber()
	var doc any
	if err := dec.Decode(&doc); err != nil {
		return nil, err
	}
	if !migrateNode(doc) {
		return b, nil
	}
	return json.Marshal(doc)
}

// migrateNode renames the legacy keys in every decision block under v and
// reports whether it did: "review" becomes "decision" (when the record has
// no decision yet), and the block's "state" becomes "status".
func migrateNode(v any) (changed bool) {
	switch x := v.(type) {
	case []any:
		for _, item := range x {
			changed = migrateNode(item) || changed
		}
	case map[string]any:
		if old, ok := x["review"].(map[string]any); ok {
			if _, taken := x["decision"]; !taken {
				x["decision"] = old
				delete(x, "review")
				changed = true
			}
		}
		if block, ok := x["decision"].(map[string]any); ok {
			if old, has := block["state"]; has {
				if _, taken := block["status"]; !taken {
					block["status"] = old
				}
				delete(block, "state")
				changed = true
			}
		}
		changed = migrateNote(x) || changed
		for _, item := range x {
			changed = migrateNode(item) || changed
		}
	}
	return changed
}

// migrateNote gives a question's single decision note a place in its thread:
// a decision whose note has no matching owner entry gets a "note" entry
// (dated when the item was decided), so older notes show beside the replies.
func migrateNote(q map[string]any) bool {
	block, ok := q["decision"].(map[string]any)
	if !ok {
		return false
	}
	note, _ := block["note"].(string)
	note = strings.TrimSpace(note)
	if note == "" {
		return false
	}
	entries, _ := q["entries"].([]any)
	for _, e := range entries {
		m, _ := e.(map[string]any)
		if t, _ := m["text"].(string); strings.TrimSpace(t) == note {
			return false
		}
	}
	id, err := token()
	if err != nil {
		return false
	}
	at, _ := block["decided_at"].(string)
	if at == "" {
		at, _ = q["asked_at"].(string)
	}
	by, _ := json.Marshal(Owner())
	var byDoc any
	_ = json.Unmarshal(by, &byDoc)
	q["entries"] = append(entries, map[string]any{"id": "e-" + id, "kind": "note", "text": note, "by": byDoc, "at": at, "captured": false})
	return true
}
