package main

import (
	"bytes"
	"encoding/json"
)

// migrateLegacy reads question data written before the lexicon renames. A
// question's decision block was stored under "review" (v0.2.0 and earlier),
// and before that held its progress under "state" (empty, applied, failed),
// now "status". It is a data migration, not a name alias: the next write of
// the file stores the new names. Text without a legacy key is returned as is.
func migrateLegacy(b []byte) ([]byte, error) {
	if !bytes.Contains(b, []byte(`"state"`)) && !bytes.Contains(b, []byte(`"review"`)) {
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
		for _, item := range x {
			changed = migrateNode(item) || changed
		}
	}
	return changed
}
