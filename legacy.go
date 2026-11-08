package main

import (
	"bytes"
	"encoding/json"
)

// migrateLegacy reads question data written before the lexicon rename: a
// question's review block held its progress under "state" (empty, applied,
// failed), now "status". It is a data migration, not a name alias: the next
// write of the file stores the new name. Text without a legacy key is
// returned as is.
func migrateLegacy(b []byte) ([]byte, error) {
	if !bytes.Contains(b, []byte(`"state"`)) {
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

// migrateNode renames the legacy key in every review block under v and
// reports whether it did.
func migrateNode(v any) (changed bool) {
	switch x := v.(type) {
	case []any:
		for _, item := range x {
			changed = migrateNode(item) || changed
		}
	case map[string]any:
		if review, ok := x["review"].(map[string]any); ok {
			if old, has := review["state"]; has {
				if _, taken := review["status"]; !taken {
					review["status"] = old
				}
				delete(review, "state")
				changed = true
			}
		}
		for _, item := range x {
			changed = migrateNode(item) || changed
		}
	}
	return changed
}
