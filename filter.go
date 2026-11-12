package main

import (
	"encoding/json"

	"ofam.dev/ochazuke"
)

// The pane prop is a reference to the host's pane (hostNoun only: the
// framework has a pane noun of its own, which a Noun "pane" would name), so ochazuke's
// relation graph follows it; a pane that is gone stays as written. project_id
// and agent_session are not references: project_id is stored as a project name
// or an id (mtm keys by id), and agent_session names a harness session, which
// is no noun.
//
// Declare the scalar fields carried by both question lists. The project
// parameter names project_id through an alias so the project record remains
// available to details.
func (a *App) declareFilterProps() {
	for _, noun := range []string{"question", decisionNoun} {
		a.r.Define(noun).Props(
			ochazuke.PropSpec{Name: "id", Key: true, ReadOnly: true, Parameter: ochazuke.Parameter{Type: "string", Description: "Record ID"}},
			ochazuke.PropSpec{Name: "project_id", Aliases: []string{"project"}, ReadOnly: true, Parameter: ochazuke.Parameter{Type: "string", Description: "Project ID; empty when unassociated"}},
			ochazuke.PropSpec{Name: "agent", ReadOnly: true, Parameter: ochazuke.Parameter{Type: "string", Description: "Recording harness"}},
			ochazuke.PropSpec{Name: "direction", ReadOnly: true, Parameter: ochazuke.Parameter{Type: "string", Enum: []string{toAgent, toOwner}, Description: "Who answers the question"}},
			ochazuke.PropSpec{Name: "pane", ReadOnly: true, Parameter: ochazuke.Parameter{Type: "string", HostNoun: "pane", Free: true, Description: "Asking pane"}},
			ochazuke.PropSpec{Name: "agent_session", ReadOnly: true, Parameter: ochazuke.Parameter{Type: "string", Description: "Agent session"}},
		)
	}
	a.r.Define("question").Props(a.questionStoreProps()...)
	a.r.Define(decisionNoun).Props(a.decisionStoreProps()...)
	a.r.Define("question").Store(a.questionStore())
	a.r.Define(decisionNoun).Store(a.decisionStore())
}

// questionRowData preserves the record and exposes the list parameters as
// scalar filter fields. Keep optional values present so negated filters also
// match unassociated questions, as they do on Lanes. Old owner questions have
// no stored direction; they answer to-agent just as the list parameter does.
func questionRowData(q Question) map[string]any {
	b, _ := json.Marshal(q)
	data := map[string]any{}
	_ = json.Unmarshal(b, &data)
	data["project_id"] = q.Project.ID
	data["agent"] = q.RecordedBy.Harness
	data["pane"] = q.Source.Pane
	data["agent_session"] = q.Source.AgentSession
	direction := toAgent
	if q.Direction == toOwner {
		direction = toOwner
	}
	data["direction"] = direction
	return data
}
