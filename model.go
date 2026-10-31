package main

import (
	"fmt"
	"strings"
	"time"
)

type Link struct {
	Kind   string `json:"kind,omitempty"`
	Target string `json:"target"`
	Label  string `json:"label,omitempty"`
}
type Project struct {
	ID    string `json:"id,omitempty"`
	Label string `json:"label,omitempty"`
}
type Source struct {
	Harness      string `json:"harness,omitempty"`
	AgentSession string `json:"agent_session,omitempty"`
	Pane         string `json:"pane,omitempty"`
	TuiosSession string `json:"tuios_session,omitempty"`
	Transcript   string `json:"transcript,omitempty"`
	Turn         string `json:"turn,omitempty"`
}
type Entry struct {
	ID       string    `json:"id"`
	Kind     string    `json:"kind"`
	Text     string    `json:"text"`
	Summary  string    `json:"summary,omitempty"`
	By       Actor     `json:"by"`
	At       time.Time `json:"at"`
	Captured bool      `json:"captured"`
	Links    []Link    `json:"links,omitempty"`
	Replaces string    `json:"replaces,omitempty"`
	File     string    `json:"file,omitempty"`
	Key      string    `json:"key,omitempty"`
}
type Question struct {
	FileName       string    `json:"file_name,omitempty"`
	ID             string    `json:"id"`
	Title          string    `json:"title"`
	Text           string    `json:"text"`
	Verbatim       string    `json:"verbatim"`
	AskedBy        Actor     `json:"asked_by"`
	RecordedBy     Actor     `json:"recorded_by"`
	AskedAt        time.Time `json:"asked_at"`
	Created        time.Time `json:"created"`
	Updated        time.Time `json:"updated"`
	Status         string    `json:"status"`
	StatusExplicit bool      `json:"status_explicit,omitempty"`
	Project        Project   `json:"project"`
	Cwd            string    `json:"cwd,omitempty"`
	Task           string    `json:"task,omitempty"`
	Source         Source    `json:"source"`
	Tags           []string  `json:"tags,omitempty"`
	Links          []Link    `json:"links,omitempty"`
	Parent         string    `json:"parent,omitempty"`
	Starred        bool      `json:"starred"`
	Deleted        bool      `json:"deleted,omitempty"`
	Key            string    `json:"key"`
	Entries        []Entry   `json:"entries"`
	OwnerRenamed   bool      `json:"owner_renamed,omitempty"`
}

var statuses = []string{"open", "answered", "follow-up", "moot"}
var entryStatuses = map[string]string{"question": "open", "answer": "answered", "follow-up": "follow-up", "note": "", "correction": ""}

func thread(q Question, provenance bool) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# %s\n\n%s\n", q.Title, q.Text)
	if provenance {
		fmt.Fprintf(&b, "\nStatus: %s · %s · %s\nProject: %s · Cwd: %s\nAsked: %s · Verbatim: %s\n", q.Status, q.RecordedBy.Harness, q.RecordedBy.Model, q.Project.ID, q.Cwd, q.AskedAt.Format(time.RFC3339), q.Verbatim)
	}
	for _, e := range q.Entries {
		fmt.Fprintf(&b, "\n## %s\n\n%s\n", e.Kind, e.Text)
		if provenance {
			fmt.Fprintf(&b, "\nBy %s %s · %s · %s\n", e.By.Kind, e.By.Harness, e.By.Model, e.At.Format(time.RFC3339))
		}
		if e.Replaces != "" {
			fmt.Fprintf(&b, "Corrects: %s\n", e.Replaces)
		}
		for _, l := range e.Links {
			fmt.Fprintf(&b, "%s: %s\n", l.Label, l.Target)
		}
	}
	for _, l := range q.Links {
		fmt.Fprintf(&b, "\n%s: %s\n", l.Label, l.Target)
	}
	return b.String()
}
