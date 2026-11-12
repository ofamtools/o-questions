package main

import (
	"fmt"
	"ofam.dev/ochazuke"
	"strings"
	"time"
)

func positive(v any) error {
	n, ok := v.(int)
	if !ok || n <= 0 {
		return fmt.Errorf("must be positive")
	}
	return nil
}
func nonnegative(v any) error {
	n, ok := v.(int)
	if !ok || n < 0 {
		return fmt.Errorf("must be nonnegative")
	}
	return nil
}
func statusTable(v any) error {
	xs, ok := v.([]string)
	if !ok {
		return fmt.Errorf("expected kind=status rows")
	}
	seen := map[string]bool{}
	for _, x := range xs {
		k, s, ok := strings.Cut(x, "=")
		if !ok || k == "" || seen[k] {
			return fmt.Errorf("invalid kind=status row %q", x)
		}
		seen[k] = true
		if s != "" && s != "open" && s != "answered" && s != "follow-up" {
			return fmt.Errorf("entry status must be open, answered, follow-up or empty")
		}
	}
	return nil
}

// defaultOutcomeRows is the default of decision.outcomes: id|label|status|
// class|flags. Status is the question status a pick gives, class the decision
// class (what decision.show filters), flags note (needs text in the note) and
// reply (the note goes to the agent as a follow-up and a reply reopens it);
// key is the page key that picks it (a binding row; empty: none).
var defaultOutcomeRows = []string{"later|Later|answered|later||l", "moot|Moot|moot|decided||m", "ask|Ask the agent|follow-up|asked|reply|?", "other|Write your own answer|answered|decided|note|w"}

func outcomeTable(v any) error {
	xs, ok := v.([]string)
	if !ok {
		return fmt.Errorf("expected id|label|status|class|flags rows")
	}
	_, e := parseOutcomes(xs)
	return e
}
func appSettings() []ochazuke.Setting {
	return []ochazuke.Setting{
		{Name: "list.order", Type: ochazuke.SettingString, Default: "oldest-first", Enum: []string{"oldest-first", "newest-first"}, Description: "Chronological list and numbered target order"},
		{Name: "list.start-newest", Type: ochazuke.SettingBoolean, Default: true, Description: "Initially select the newest question"},
		{Name: "filter.status", Type: ochazuke.SettingString, Default: "all", Enum: []string{"all", "open", "answered", "follow-up", "moot"}, Description: "Statuses shown"},
		{Name: "filter.project", Type: ochazuke.SettingString, Default: "all", Description: "Project ID or all"},
		{Name: "filter.agent", Type: ochazuke.SettingString, Default: "all", Description: "Recording harness or all"},
		{Name: "filter.since", Type: ochazuke.SettingString, Default: "any", Enum: []string{"any", "today", "7d", "30d"}, Description: "Asking time range"},
		{Name: "detail.answer-lines", Type: ochazuke.SettingInteger, Default: 8, Validate: nonnegative, Description: "Deprecated preview limit; section threads always show complete answers"},
		{Name: "detail.provenance", Type: ochazuke.SettingBoolean, Default: true, Description: "Show provenance in the full thread modal"},
		{Name: "view.detail", Type: ochazuke.SettingBoolean, Default: true, Description: "Show the question detail panel"},
		{Name: "view.thread", Type: ochazuke.SettingBoolean, Default: true, Description: "Open full threads in a modal"},
		{Name: "list.refresh", Type: ochazuke.SettingDuration, Default: 2 * time.Second, Description: "Refresh for agent additions; 0 disables"},
		{Name: "status.from-entry", Type: ochazuke.SettingList, Default: []string{"question=open", "answer=answered", "follow-up=follow-up", "note=", "correction=", "reply=open"}, Validate: statusTable, Description: "kind=status table; empty status keeps current"},
		{Name: "entry.max-bytes", Type: ochazuke.SettingInteger, Default: 65536, Validate: positive, Description: "Larger entries use linked private files"},
		{Name: "backup.daily", Type: ochazuke.SettingBoolean, Default: true, Description: "Back up before the first write of each UTC day"},
		{Name: "backup.keep", Type: ochazuke.SettingInteger, Default: 14, Validate: positive, Description: "Daily archives retained"},
		{Name: "history.keep", Type: ochazuke.SettingInteger, Default: 200, Validate: func(v any) error {
			if n, ok := v.(int); !ok || n < 0 {
				return fmt.Errorf("history.keep: use 0 (keep everything) or a count")
			}
			return nil
		}, Description: "Undo history entries retained, newest first; 0 keeps everything"},
		{Name: "capture.backfill", Type: ochazuke.SettingBoolean, Default: true, Description: "Reserved for optional Q4 hooks"},
		{Name: "capture.verbatim-check", Type: ochazuke.SettingBoolean, Default: true, Description: "Reserved for optional Q4 hooks"},
		{Name: "capture.prompts", Type: ochazuke.SettingString, Default: "off", Enum: []string{"off", "all"}, Description: "Reserved for optional Q4 capture"},
		{Name: "filter.direction", Type: ochazuke.SettingString, Default: toAgent, Enum: directions, Description: "Questions page: to-agent (the owner's questions), to-owner (questions for the owner) or all"},
		{Name: "decision.page", Type: ochazuke.SettingBoolean, Default: true, Description: "Show the Decisions page of questions for the owner (read at start)"},
		{Name: "decision.show", Type: ochazuke.SettingString, Default: "pending", Enum: decisionFilters, Description: "Decision items shown: pending hides applied ones; differs shows those answered differently from the recommendation; archived shows only the archived ones (they are hidden otherwise)"},
		{Name: "decision.detail", Type: ochazuke.SettingString, Default: "document", Enum: []string{"document", "sections"}, Description: "Decisions page right pane (read at start): document is a working Markdown file the editor shows, focusable, boxes flip with x and are read back into the record; sections is the read-only detail"},
		{Name: "decision.color", Type: ochazuke.SettingBoolean, Default: true, Description: "Mark decision rows by state with a glyph and a theme tone: new, answered, answered differently from the recommendation, asked back, later, applied, archived; off draws the plain rows"},
		{Name: "decision.sort", Type: ochazuke.SettingString, Default: "group", Enum: []string{"group", "undecided-first"}, Description: "Order inside a topic: as recorded, or what needs you first (a row you decide then moves down)"},
		{Name: "decision.outcomes", Type: ochazuke.SettingList, Default: defaultOutcomeRows, Validate: outcomeTable, Description: "Answers that are not options, one id|label|status|class|flags|key row each (flags: note, reply; key: the page key, empty for none); status and class drive the question status and the decision.show filter"},
		{Name: "decision.labels", Type: ochazuke.SettingBoolean, Default: true, Description: "Decision detail lists the options and outcomes with their labels, descriptions and keys; off draws the compact one-line list"},
		{Name: "decision.plans-root", Type: ochazuke.SettingString, Default: "~/hub/o-plans", Description: "Plans folder question import reads"},
		{Name: "decision.mgmt", Type: ochazuke.SettingString, Default: "", Description: "mgmt.py path for decision --web; empty discovers it on PATH, then in the legacy home location"},
		{Name: "export.open", Type: ochazuke.SettingBoolean, Default: true, Description: "Reserved for Q5 export"},
	}
}
