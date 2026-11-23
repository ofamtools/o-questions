package main

import (
	"strings"
	"time"

	"ofam.dev/ochazuke/core"
)

// The catalog protocol (matomeru's ext package): a host runs
//
//	o-questions catalog        a Manifest: the questions.question schema and commands
//	o-questions query [TEXT]   {"items": [record...], "total": n}, read-only
//
// and never writes to o-questions; what the owner changes on a record in the
// host is kept there as a shadow record.

const questionKind = "questions.question"

// manifestDoc is `o-questions catalog`: the wire shape of ext.Manifest.
type manifestDoc struct {
	Name     string             `json:"name"`
	Version  string             `json:"version,omitempty"`
	Schemas  []schemaDoc        `json:"schemas,omitempty"`
	Commands []core.CommandSpec `json:"commands,omitempty"`
	Bindings []core.Binding     `json:"bindings,omitempty"`
}

// schemaDoc is the wire shape of data.Schema.
type schemaDoc struct {
	Kind           string           `json:"kind"`
	Title          string           `json:"title"`
	Plural         string           `json:"plural"`
	Fields         []core.ParamSpec `json:"fields"`
	DefaultColumns []string         `json:"defaultColumns,omitempty"`
	SummaryField   string           `json:"summaryField,omitempty"`
	UniqueFields   []string         `json:"uniqueFields,omitempty"`
}

// recordDoc is the wire shape of data.Record.
type recordDoc struct {
	Kind            string          `json:"kind"`
	ID              string          `json:"id"`
	Fields          map[string]any  `json:"fields"`
	ReadOnly        map[string]bool `json:"readOnly,omitempty"`
	ArchiveStatus   string          `json:"archiveStatus"`
	StarStatus      string          `json:"starStatus"`
	ChangeSetNumber int64           `json:"changeSetNumber"`
	CreatedAt       time.Time       `json:"createdAt"`
	UpdatedAt       time.Time       `json:"updatedAt"`
}

// queryResult is `o-questions query`.
type queryResult struct {
	Items []recordDoc `json:"items"`
	Total int         `json:"total"`
}

func questionFields() []core.ParamSpec {
	f := func(name string, t core.ParamType, desc string) core.ParamSpec {
		return core.ParamSpec{Name: name, Type: t, Description: desc}
	}
	return []core.ParamSpec{
		f("title", core.ParamString, "Question title"),
		f("text", core.ParamString, "Full question text"),
		f("status", core.ParamString, "open, answered, follow-up, or moot"),
		f("project", core.ParamString, "Associated project"),
		f("direction", core.ParamString, "to-owner for owner decisions, else empty"),
		f("decision_state", core.ParamString, "Decision state when directed to owner"),
		f("entries", core.ParamInt, "Number of thread entries"),
		f("tags", core.ParamString, "Comma-separated tags"),
		f("asked_at", core.ParamString, "When asked (RFC 3339)"),
		f("updated", core.ParamString, "When last updated (RFC 3339)"),
		f("starred", core.ParamBool, "Whether the question is starred"),
		f("archived", core.ParamBool, "Whether the question is archived"),
	}
}

// questionCommands are the questions.question verbs a host may run through the CLI.
func questionCommands() []core.CommandSpec {
	target := core.ParamSpec{Name: "target", Type: core.ParamRef, Required: true, RefKinds: []string{questionKind}, Description: "Question id, prefix, title, or 1-based index"}
	str := func(name string, req bool, desc string) core.ParamSpec {
		return core.ParamSpec{Name: name, Type: core.ParamString, Required: req, Description: desc}
	}
	cmd := func(verb, cli, desc string, params ...core.ParamSpec) core.CommandSpec {
		return core.CommandSpec{
			ID:          questionKind + "." + verb,
			Description: desc,
			Params:      params,
			Tags:        []string{"questions"},
			Meta:        map[string]any{"cli": "o-questions " + cli},
		}
	}
	return []core.CommandSpec{
		cmd("list", "list --json", "List questions in the journal",
			str("status", false, "Filter by status: open, answered, follow-up, moot"),
			str("project", false, "Filter by project id or name"),
			core.ParamSpec{Name: "archived", Type: core.ParamBool, Description: "Include archived questions"}),
		cmd("show", "show --json", "Show a question and its full thread", target),
		cmd("ask", "ask --json", "Record a new question",
			str("question", true, "Question text"),
			str("title", false, "Short title"),
			str("project", false, "Associated project")),
		cmd("answer", "answer --json", "Append an answer to a question",
			target,
			str("answer", true, "Answer text"),
			str("summary", false, "Short summary")),
		cmd("resolve", "resolve --json", "Mark a question resolved (answered or moot)",
			target,
			core.ParamSpec{Name: "moot", Type: core.ParamBool, Description: "Mark moot instead of answered"}),
		cmd("reopen", "reopen --json", "Reopen a question", target),
		cmd("undo", "undo --json", "Undo the last store change"),
		cmd("decisions", "decisions --json", "List or act on owner decisions",
			str("group", false, "Filter by topic group")),
	}
}

// manifest is the catalog o-questions prints.
func manifest() manifestDoc {
	return manifestDoc{
		Name:    "o-questions",
		Version: Version,
		Schemas: []schemaDoc{{
			Kind:           questionKind,
			Title:          "Question",
			Plural:         "Questions",
			Fields:         questionFields(),
			DefaultColumns: []string{"title", "status", "project", "entries", "updated"},
			SummaryField:   "title",
		}},
		Commands: questionCommands(),
	}
}

// questionRecord converts a Question into a read-only catalog record.
func questionRecord(s *Store, q Question) recordDoc {
	decState := ""
	if q.forOwner() {
		decState = s.decisionState(q)
	}
	fields := map[string]any{
		"title":          q.Title,
		"text":           q.Text,
		"status":         q.Status,
		"project":        q.Project.Shown(),
		"direction":      q.Direction,
		"decision_state": decState,
		"entries":        len(q.Entries),
		"tags":           strings.Join(q.Tags, ", "),
		"asked_at":       q.AskedAt.Format(time.RFC3339),
		"updated":        q.Updated.Format(time.RFC3339),
		"starred":        q.Starred,
		"archived":       q.Deleted,
	}
	ro := make(map[string]bool, len(fields))
	for k := range fields {
		ro[k] = true
	}
	arch := "active"
	if q.Deleted {
		arch = "archived"
	}
	star := "unstarred"
	if q.Starred {
		star = "starred"
	}
	return recordDoc{
		Kind:          questionKind,
		ID:            q.ID,
		Fields:        fields,
		ReadOnly:      ro,
		ArchiveStatus: arch,
		StarStatus:    star,
		CreatedAt:     q.Created,
		UpdatedAt:     q.Updated,
	}
}

// queryQuestions lists questions matching text, status and project as catalog records.
func queryQuestions(st *Store, text, status, project string, includeArchived bool) (queryResult, error) {
	rows, err := st.records(includeArchived)
	if err != nil {
		return queryResult{}, err
	}
	qText := strings.ToLower(strings.TrimSpace(text))
	res := queryResult{Items: []recordDoc{}}
	for _, q := range rows {
		if status != "" && q.Status != status {
			continue
		}
		if project != "" && !strings.EqualFold(q.Project.ID, project) && !strings.EqualFold(q.Project.Shown(), project) {
			continue
		}
		if qText != "" {
			hay := strings.ToLower(q.Title + "\x00" + q.Text + "\x00" + q.Project.Shown() + "\x00" + strings.Join(q.Tags, " "))
			if !strings.Contains(hay, qText) {
				continue
			}
		}
		res.Items = append(res.Items, questionRecord(st, q))
	}
	res.Total = len(res.Items)
	return res, nil
}
