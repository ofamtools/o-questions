package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

// oldShape rewrites the stored decision of q into the shape written before
// options: no options, no format, the given actions, recommended action and pick.
func oldShape(t *testing.T, s *Store, q Question, actions []string, recommended, action string) string {
	t.Helper()
	path := s.path(q)
	b, e := os.ReadFile(path)
	if e != nil {
		t.Fatal(e)
	}
	var doc map[string]any
	if e = json.Unmarshal(b, &doc); e != nil {
		t.Fatal(e)
	}
	d := doc["decision"].(map[string]any)
	for _, k := range []string{"options", "recommended", "format", "action"} {
		delete(d, k)
	}
	if actions != nil {
		d["actions"] = actions
	}
	if recommended != "" {
		d["recommended_action"] = recommended
	}
	if action != "" {
		d["action"] = action
	}
	if b, e = json.Marshal(doc); e != nil {
		t.Fatal(e)
	}
	if e = os.WriteFile(path, b, 0600); e != nil {
		t.Fatal(e)
	}
	migratedDirs.Delete(s.Dir) // a fresh process would check the records again
	return path
}

func optionIDs(s *Store, q Question) []string {
	var ids []string
	for _, o := range s.optionsOf(q.Decision) {
		ids = append(ids, o.ID)
	}
	return ids
}

func TestMigrateDefaultQuestionKeepsYesNo(t *testing.T) {
	t.Parallel()
	owner, ag := optionStore(t)
	q := pose(t, ag, Pose{Question: "Ship it?", Options: releaseOptions, Key: "old-default"}).Question
	path := oldShape(t, owner, q, nil, "", "yes")
	got, e := owner.Resolve(q.ID, false)
	if e != nil {
		t.Fatal(e)
	}
	if ids := optionIDs(owner, got); !slices.Equal(ids, []string{"yes", "no"}) {
		t.Fatal("a question with no stored actions offers yes and no", ids)
	}
	if got.Decision.Action != "yes" || owner.answerLabel(got.Decision) != "yes" || got.Decision.Recommended != "" {
		t.Fatal("the pick stays, the old implicit recommendation is not invented", *got.Decision)
	}
	b, _ := os.ReadFile(path)
	if !strings.Contains(string(b), `"format": 2`) || !strings.Contains(string(b), `"id": "yes"`) {
		t.Fatalf("the first load writes the migrated record back: %s", b)
	}
}

func TestMigrateCustomActionsBecomeOptions(t *testing.T) {
	t.Parallel()
	owner, ag := optionStore(t)
	q := pose(t, ag, Pose{Question: "Which?", Options: releaseOptions, Key: "custom"}).Question
	path := oldShape(t, owner, q, []string{"merge", "later", "hold", "moot"}, "hold", "merge")
	got, e := owner.Resolve(q.ID, false)
	if e != nil {
		t.Fatal(e)
	}
	if ids := optionIDs(owner, got); !slices.Equal(ids, []string{"merge", "hold"}) {
		t.Fatal("custom actions are options; outcome words drop out", ids)
	}
	if got.Decision.Recommended != "hold" || got.Decision.Action != "merge" || len(got.Decision.Actions) != 4 {
		t.Fatal("recommended action and pick kept, the stored actions stay readable", *got.Decision)
	}
	b, _ := os.ReadFile(path)
	if !strings.Contains(string(b), `"actions"`) || !strings.Contains(string(b), `"recommended": "hold"`) {
		t.Fatalf("%s", b)
	}
}

func TestMigrateRecommendedOutcomeIsNotKept(t *testing.T) {
	t.Parallel()
	owner, ag := optionStore(t)
	q := pose(t, ag, Pose{Question: "Which?", Options: releaseOptions, Key: "rec"}).Question
	oldShape(t, owner, q, nil, "later", "")
	got, e := owner.Resolve(q.ID, false)
	if e != nil || got.Decision.Recommended != "" || got.Decision.RecommendedAction != "later" {
		t.Fatal("a recommended action that is no option stays as stored text", e, got.Decision)
	}
}

func TestMigrateIsIdempotentAndBacksUpOnce(t *testing.T) {
	t.Parallel()
	owner, ag := optionStore(t)
	q := pose(t, ag, Pose{Question: "Ship it?", Options: releaseOptions, Key: "idem"}).Question
	path := oldShape(t, owner, q, nil, "", "")
	if _, e := owner.List(); e != nil {
		t.Fatal(e)
	}
	first, _ := os.ReadFile(path)
	backups, _ := filepath.Glob(filepath.Join(owner.Dir, "backups", "pre-migration", "*.tar.gz"))
	if len(backups) != 1 {
		t.Fatal("one backup before the first rewrite", backups)
	}
	migratedDirs.Delete(owner.Dir)
	if _, e := owner.List(); e != nil {
		t.Fatal(e)
	}
	second, _ := os.ReadFile(path)
	again, _ := filepath.Glob(filepath.Join(owner.Dir, "backups", "pre-migration", "*.tar.gz"))
	if string(first) != string(second) || len(again) != 1 {
		t.Fatal("a second load changes nothing and takes no backup", len(again))
	}
}

func TestMigrateRoundTripKeepsNewQuestionsAlone(t *testing.T) {
	t.Parallel()
	owner, ag := optionStore(t)
	q := pose(t, ag, Pose{Question: "Open question, no options", Key: "none"}).Question
	if q.Decision.Format != decisionFormat {
		t.Fatal("a new decision carries the format mark")
	}
	path := owner.path(q)
	before, _ := os.ReadFile(path)
	migratedDirs.Delete(owner.Dir)
	got, e := owner.Resolve(q.ID, false)
	after, _ := os.ReadFile(path)
	if e != nil || len(got.Decision.Options) != 0 || string(before) != string(after) {
		t.Fatal("a question posed without options stays without options and is not rewritten", e)
	}
	if _, e = os.Stat(filepath.Join(owner.Dir, "backups", "pre-migration")); e == nil {
		t.Fatal("nothing migrated, so no backup")
	}
	note := "mine"
	if _, e = owner.Decide(q.ID, DecisionChange{Action: "other", Note: &note}); e != nil {
		t.Fatal(e)
	}
}

func TestMigrateNodeLeavesMarkedBlocks(t *testing.T) {
	t.Parallel()
	b, e := migrateLegacy([]byte(`{"decision":{"format":2,"actions":["a","b"]}}`))
	if e != nil || strings.Contains(string(b), `"options"`) {
		t.Fatal(e, string(b))
	}
	b, e = migrateLegacy([]byte(`{"rows":[{"id":"x","decision":{"actions":["a"]}}]}`))
	var snap Snapshot
	if e != nil || json.Unmarshal(b, &snap) != nil || snap.Rows[0].Decision.Options[0].ID != "a" {
		t.Fatal("undo snapshots migrate too", e, string(b))
	}
}

var migrateNow = time.Date(2026, 11, 8, 12, 0, 0, 0, time.UTC)

func writeSettings(t *testing.T, text string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "settings.json")
	if e := os.WriteFile(path, []byte(text), 0600); e != nil {
		t.Fatal(e)
	}
	return path
}

func TestSettingsRewriteDropsActionsAndAsk(t *testing.T) {
	t.Parallel()
	path := writeSettings(t, "{\n  // my picks\n  \"list.order\": \"newest-first\",\n  \"decision.actions\": [\n    \"merge\",\n    \"hold\"\n  ],\n  \"decision.ask\": false,\n  \"backup.keep\": 3\n}\n")
	notes, e := migrateSettingsFile(path, migrateNow)
	if e != nil {
		t.Fatal(e)
	}
	got, _ := os.ReadFile(path)
	text := string(got)
	if strings.Contains(text, "decision.actions") || strings.Contains(text, "decision.ask") || !strings.Contains(text, "// my picks") || !strings.Contains(text, `"backup.keep": 3`) {
		t.Fatalf("retired keys go, the rest and comments stay: %s", text)
	}
	if !strings.Contains(text, `"decision.outcomes"`) || strings.Contains(text, "ask|") || !strings.Contains(text, "moot|Moot") {
		t.Fatalf("ask=false writes the outcomes without the ask row: %s", text)
	}
	if len(notes) < 3 || !strings.Contains(strings.Join(notes, "\n"), "merge") {
		t.Fatal("the custom actions are reported", notes)
	}
	bak, _ := os.ReadFile(path + ".bak-2026-11-08")
	if !strings.Contains(string(bak), "decision.actions") {
		t.Fatal("the original is backed up first")
	}
	// Idempotent: nothing left to do, no second backup.
	notes, e = migrateSettingsFile(path, migrateNow)
	again, _ := os.ReadFile(path)
	if e != nil || notes != nil || string(again) != text {
		t.Fatal("second run changes nothing", notes, e)
	}
	if m, _ := filepath.Glob(path + ".bak-*"); len(m) != 1 {
		t.Fatal(m)
	}
}

func TestSettingsRewriteEditsExistingOutcomes(t *testing.T) {
	t.Parallel()
	path := writeSettings(t, `{"decision.outcomes": ["later|Later|answered|later|", "ask|Ask|follow-up|asked|reply"], "decision.ask": false}`)
	if _, e := migrateSettingsFile(path, migrateNow); e != nil {
		t.Fatal(e)
	}
	got, _ := os.ReadFile(path)
	var doc map[string]any
	if e := json.Unmarshal(got, &doc); e != nil {
		t.Fatalf("%v: %s", e, got)
	}
	rows, _ := doc["decision.outcomes"].([]any)
	if len(rows) != 1 || rows[0] != "later|Later|answered|later|" || len(doc) != 1 {
		t.Fatalf("%s", got)
	}
}

func TestSettingsRewriteDefaultsAreQuiet(t *testing.T) {
	t.Parallel()
	path := writeSettings(t, `{"decision.actions": ["yes", "no", "later", "moot"], "decision.ask": true, "list.order": "oldest-first"}`)
	notes, e := migrateSettingsFile(path, migrateNow)
	got, _ := os.ReadFile(path)
	var doc map[string]any
	if e != nil || json.Unmarshal(got, &doc) != nil || len(doc) != 1 || len(notes) != 1 {
		t.Fatal("the old default list needs no notice beyond the retirement", e, notes, string(got))
	}
}

func TestSettingsRewriteLeavesOtherFiles(t *testing.T) {
	t.Parallel()
	path := writeSettings(t, `{"list.order": "newest-first"}`)
	if notes, e := migrateSettingsFile(path, migrateNow); e != nil || notes != nil {
		t.Fatal(e, notes)
	}
	if m, _ := filepath.Glob(path + ".bak-*"); len(m) != 0 {
		t.Fatal("no retired key, no backup", m)
	}
	if notes, e := migrateSettingsFile(filepath.Join(t.TempDir(), "missing.json"), migrateNow); e != nil || notes != nil {
		t.Fatal("an absent file is fine", e)
	}
	bad := writeSettings(t, `{"decision.ask": `)
	if notes, e := migrateSettingsFile(bad, migrateNow); e != nil || notes != nil {
		t.Fatal("a file that does not parse is left to the framework", e)
	}
}
