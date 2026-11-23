// `o-questions decision --web` reuses mgmt's browser decisions page unchanged:
// it writes an mgmt session (the version 1 shape mgmt.py build_session
// produces) from the open questions for the owner, runs `mgmt.py serve` on
// it, and copies each saved decision back into o-questions while the page is
// served and once more when it stops. o-questions stays the record; the
// session is a disposable view, rewritten on every run.
package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"time"
)

type mgmtItem struct {
	ID                string          `json:"id"`
	Title             string          `json:"title"`
	Source            string          `json:"source"`
	Details           string          `json:"details"`
	Group             string          `json:"group,omitempty"`
	Recommendation    string          `json:"recommendation,omitempty"`
	RecommendedAction string          `json:"recommended_action,omitempty"`
	Thread            []ThreadMessage `json:"thread,omitempty"`
	Action            string          `json:"action"`
	Note              string          `json:"note"`
	Status            string          `json:"status"`
	Result            string          `json:"result,omitempty"`
	ContextChanged    bool            `json:"context_changed,omitempty"`
	NewReply          bool            `json:"new_reply,omitempty"`
}
type mgmtSession struct {
	Version       int        `json:"version"`
	Title         string     `json:"title"`
	Scope         string     `json:"scope"`
	DefaultAction string     `json:"default_action"`
	Actions       []string   `json:"actions"`
	Kind          string     `json:"kind"`
	Ask           bool       `json:"ask"`
	Groups        []string   `json:"groups"`
	GeneratedAt   string     `json:"generated_at"`
	Items         []mgmtItem `json:"items"`
	CreatedAt     string     `json:"created_at"`
	UpdatedAt     string     `json:"updated_at"`
}

// webSession is the mgmt session for every question for the owner; applied
// ones are "done" (locked in the page).
func (s *Store) webSession() (mgmtSession, error) {
	qs, e := s.ownerQuestions()
	if e != nil {
		return mgmtSession{}, e
	}
	qs = sortByGroup(qs)
	_, asks := s.replyOutcome()
	stamp := s.now().Format(time.RFC3339)
	out := mgmtSession{Version: 1, Title: "Questions for the owner", Scope: "Questions agents posed in o-questions (" + s.Dir + "). Decisions are written back to o-questions while this page is served.", DefaultAction: "", Actions: []string{}, Kind: "o-questions", Ask: asks, Groups: groupOrder(qs), GeneratedAt: stamp, CreatedAt: stamp, UpdatedAt: stamp, Items: []mgmtItem{}}
	for _, q := range qs {
		r := q.Decision
		for _, o := range s.optionsOf(r) {
			if !slices.Contains(out.Actions, o.ID) {
				out.Actions = append(out.Actions, o.ID)
			}
		}
		details := strings.TrimSpace(q.Text)
		if strings.TrimSpace(r.Details) != "" {
			details += "\n\n" + strings.TrimSpace(r.Details)
		}
		it := mgmtItem{ID: q.ID, Title: q.Title, Source: r.Source, Details: details, Group: groupOf(q), Recommendation: strings.TrimSpace(r.Recommendation), Action: r.Action, Note: r.Note, Status: "pending", ContextChanged: r.ContextChanged, NewReply: r.NewReply}
		if it.Source == "" {
			it.Source = "o-questions:" + q.ID
		}
		it.RecommendedAction = r.recommendedID()
		if t := decisionThread(q); len(t) > 0 {
			it.Thread = t
		}
		switch r.Status {
		case decisionApplied:
			it.Status, it.Result = "done", r.Result
		case decisionFailed:
			it.Status, it.Result = "failed", r.Result
		}
		out.Items = append(out.Items, it)
	}
	for _, id := range s.outcomeIDs() {
		if !slices.Contains(out.Actions, id) {
			out.Actions = append(out.Actions, id)
		}
	}
	out.DefaultAction = out.Actions[0]
	return out, nil
}

// syncWeb copies the page's saved decisions into o-questions: each item
// whose action or note differs from the store becomes the owner's decision.
func (s *Store) syncWeb(path string) (int, error) {
	b, e := os.ReadFile(path)
	if e != nil {
		return 0, e
	}
	var sess mgmtSession
	if e = json.Unmarshal(b, &sess); e != nil {
		return 0, e
	}
	qs, e := s.ownerQuestions()
	if e != nil {
		return 0, e
	}
	byID := map[string]Question{}
	for _, q := range qs {
		byID[q.ID] = q
	}
	changed := 0
	var errs []error
	for _, it := range sess.Items {
		q, ok := byID[it.ID]
		if !ok || it.Status == "done" || q.Decision.Status == decisionApplied {
			continue
		}
		if it.Action == q.Decision.Action && it.Note == q.Decision.Note {
			continue
		}
		note := it.Note
		d := DecisionChange{Action: it.Action, Note: &note, Clear: it.Action == ""}
		if _, e := s.Decide(q.ID, d); e != nil {
			errs = append(errs, fmt.Errorf("%s: %w", q.Title, e))
			continue
		}
		changed++
	}
	return changed, errors.Join(errs...)
}

func stateDir() string {
	if d := os.Getenv("XDG_STATE_HOME"); d != "" {
		return d
	}
	h, _ := os.UserHomeDir()
	return filepath.Join(h, ".local", "state")
}

func resolveMgmt(setting string) (string, error) {
	if setting == "" {
		if path, err := exec.LookPath("mgmt.py"); err == nil {
			return path, nil
		}
		path := expandHome("~/hub/ai/utils/mgmt/mgmt.py")
		if info, err := os.Stat(path); err == nil && info.Mode().IsRegular() {
			return path, nil
		}
		return "", errors.New("mgmt.py not found: install mgmt/ from https://github.com/unixsuperhero/ai-utils and add mgmt.py to PATH, or set decision.mgmt to the script's path")
	}
	path := expandHome(setting)
	info, err := os.Stat(path)
	if err != nil {
		return "", fmt.Errorf("decision.mgmt: %w", err)
	}
	if !info.Mode().IsRegular() {
		return "", fmt.Errorf("decision.mgmt: %q is not a regular file", path)
	}
	return path, nil
}
