package main

import (
	"encoding/json"

	"ofam.dev/ochazuke"
)

// Declare the scalar fields carried by both question lists. The project
// parameter names project_id through an alias so the project record remains
// available to details. status is a derived row prop: question.status is an
// existing action, and declared props cannot take an action's name.
func (a *App) declareFilterProps() {
	for _, noun := range []string{"question", reviewNoun} {
		a.r.Define(noun).Props(
			ochazuke.PropSpec{Name: "project_id", Aliases: []string{"project"}, ReadOnly: true, Parameter: ochazuke.Parameter{Type: "string", Description: "Project ID; empty when unassociated"}},
			ochazuke.PropSpec{Name: "agent", ReadOnly: true, Parameter: ochazuke.Parameter{Type: "string", Description: "Recording harness"}},
			ochazuke.PropSpec{Name: "direction", ReadOnly: true, Parameter: ochazuke.Parameter{Type: "string", Enum: []string{toAgent, toOwner}, Description: "Who answers the question"}},
			ochazuke.PropSpec{Name: "pane", ReadOnly: true, Parameter: ochazuke.Parameter{Type: "string", Description: "Asking pane"}},
			ochazuke.PropSpec{Name: "agent_session", ReadOnly: true, Parameter: ochazuke.Parameter{Type: "string", Description: "Agent session"}},
		)
	}
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
	data["effective_project"] = q.Project.ID
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
