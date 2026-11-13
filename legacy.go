package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
)

// migrateLegacy reads question data written before the lexicon renames. A
// question's decision block was stored under "review" (v0.2.0 and earlier),
// and before that held its progress under "state" (empty, applied, failed),
// now "status". It is a data migration, not a name alias: the next write of
// the file stores the new names. Text without a legacy key is returned as is.
func migrateLegacy(b []byte) ([]byte, error) {
	if !bytes.Contains(b, []byte(`"state"`)) && !bytes.Contains(b, []byte(`"review"`)) && !bytes.Contains(b, []byte(`"note":`)) && !bytes.Contains(b, []byte(`"decision"`)) && !hasLegacyKey(b) {
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

// legacyKeys are keys renamed one shot, old to new, in any object of the
// store: the next write stores the new name.
var legacyKeys = map[string]string{"tuios_session": "mtm_session"}

func hasLegacyKey(b []byte) bool {
	for old := range legacyKeys {
		if bytes.Contains(b, []byte(`"`+old+`"`)) {
			return true
		}
	}
	return false
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
		for old, name := range legacyKeys {
			if v, has := x[old]; has {
				if _, taken := x[name]; !taken {
					x[name] = v
				}
				delete(x, old)
				changed = true
			}
		}
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
		if block, ok := x["decision"].(map[string]any); ok {
			changed = migrateOptions(block) || changed
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

// decisionFormat marks a decision that carries explicit options. A decision
// without it was posed before options: its choices were its stored actions,
// else the global default of yes, no, later, moot.
const decisionFormat = 2

// legacyDefaultOptions are the choices a question had when it stored no
// actions of its own (the old global default, less the words that are
// outcomes now). Questions posed that way keep offering them.
var legacyDefaultOptions = []string{"yes", "no"}

// outcomeIDs are the ids of the default outcome rows: a stored action with
// one of these words is the outcome, not an option.
func outcomeIDs() map[string]bool {
	ids := map[string]bool{}
	for _, row := range defaultOutcomeRows {
		id, _, _ := strings.Cut(row, "|")
		ids[id] = true
	}
	return ids
}

// migrateOptions gives an old decision explicit options and the format mark:
// its stored actions (id = label = the word, outcome words dropped), else
// yes and no; its recommended action becomes "recommended" when it names a
// kept option. The pick (action) is untouched, so a stored "yes" is still
// option yes. The old actions and recommended_action stay in the record.
// It reports whether it changed the block; a marked block is left alone.
func migrateOptions(block map[string]any) bool {
	if f, ok := block["format"].(json.Number); ok {
		if n, _ := f.Int64(); n >= decisionFormat {
			return false
		}
	}
	if f, ok := block["format"].(float64); ok && f >= decisionFormat {
		return false
	}
	if _, has := block["options"]; !has {
		words := legacyDefaultOptions
		if stored, ok := block["actions"].([]any); ok && len(stored) > 0 {
			words = nil
			for _, a := range stored {
				if w, ok := a.(string); ok && w != "" && !outcomeIDs()[w] {
					words = append(words, w)
				}
			}
		}
		opts := []any{}
		for _, w := range words {
			opts = append(opts, map[string]any{"id": w, "label": w})
		}
		if len(opts) > 0 {
			block["options"] = opts
			if _, has := block["recommended"]; !has {
				if rec, _ := block["recommended_action"].(string); rec != "" && slices.Contains(words, rec) {
					block["recommended"] = rec
				}
			}
		}
	}
	block["format"] = json.Number(strconv.Itoa(decisionFormat))
	return true
}

// migrateRecords writes back the question files that hold an old shape, once
// per file: the first load after an upgrade. It reads every record file,
// migrates it as a read would (migrateLegacy), and when anything changed it
// first takes a backup of the store (never overwritten, never pruned), then
// replaces each changed file atomically with the migrated text. Nothing is
// deleted: the old actions stay in the record. A second run finds nothing to
// change. Called with the store locked.
func (s *Store) migrateRecords() error {
	files, e := os.ReadDir(filepath.Join(s.Dir, "q"))
	if e != nil {
		return e
	}
	type rewrite struct {
		path string
		text []byte
	}
	var todo []rewrite
	for _, f := range files {
		if f.IsDir() || !strings.HasSuffix(f.Name(), ".json") || strings.HasPrefix(f.Name(), ".") {
			continue
		}
		path := filepath.Join(s.Dir, "q", f.Name())
		b, e := os.ReadFile(path)
		if e != nil {
			return e
		}
		m, e := migrateLegacy(b)
		if e != nil {
			return fmt.Errorf("%s: %w", f.Name(), e)
		}
		if bytes.Equal(m, b) {
			continue
		}
		var out bytes.Buffer
		if e = json.Indent(&out, m, "", "  "); e != nil {
			return fmt.Errorf("%s: %w", f.Name(), e)
		}
		out.WriteByte('\n')
		todo = append(todo, rewrite{path, out.Bytes()})
	}
	if len(todo) == 0 {
		return nil
	}
	if _, e = s.backup(filepath.Join(s.Dir, "backups", "pre-migration"), false); e != nil {
		return fmt.Errorf("backup before migration: %w", e)
	}
	for _, r := range todo {
		if e = atomicBytes(r.path, r.text); e != nil {
			return e
		}
	}
	return nil
}
