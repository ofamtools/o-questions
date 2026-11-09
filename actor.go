package main

import (
	"encoding/json"
	"fmt"
	"strings"

	"ofam.dev/ochazuke"
)

// Actor and RefusedError adapt matomeru/internal/work/actor.go.
// Entry points choose identity once; the store enforces policy for every write.
type Actor struct {
	Kind    string `json:"kind"`
	Name    string `json:"name,omitempty"`
	Harness string `json:"harness,omitempty"`
	Pane    string `json:"pane,omitempty"`
	Session string `json:"session,omitempty"`
	Task    string `json:"task,omitempty"`
	Model   string `json:"model,omitempty"`
}

func Owner() Actor { return Actor{Kind: "owner"} }

// Undeclared writes are open; these declarations also feed the catalog.
var agentPolicy = map[string]string{"delete": ochazuke.AgentNever}

type RefusedError struct{ Action, Rule string }

func (e *RefusedError) Error() string {
	return fmt.Sprintf("agent refused %s (%s); ask the owner", e.Action, e.Rule)
}
func cliActor(env func(string) string) (Actor, error) {
	a := Actor{Kind: "owner", Name: paneEnv(env, "AGENT_NAME"), Harness: paneEnv(env, "HARNESS"), Pane: paneEnv(env, "PANE_ID"), Session: paneEnv(env, "SESSION"), Model: env("AGENT_MODEL")}
	if a.Harness == "" {
		a.Harness = env("AGENT_HARNESS")
	}
	if a.Harness == "" && (env("CODEX_THREAD_ID") != "" || env("CODEX_CI") != "") {
		a.Harness = "codex"
	}
	if a.Harness == "" && env("CLAUDE_CODE_ENTRYPOINT") != "" {
		a.Harness = "claude-code"
	}
	if a.Harness != "" {
		a.Kind = "agent"
	}
	if a.Name == "" {
		a.Name = a.Harness
	}
	// Like mtm, explicit actor JSON is accepted at the trusted CLI boundary.
	raw := env("O_QUESTIONS_ACTOR")
	if raw == "" {
		raw = env("MTM_ACTOR")
	}
	if raw != "" {
		if raw == "owner" || raw == "agent" {
			a.Kind = raw
		} else {
			if e := json.Unmarshal([]byte(raw), &a); e != nil {
				return a, e
			}
		}
	}
	if !strings.Contains("|owner|agent|", "|"+a.Kind+"|") {
		return a, fmt.Errorf("invalid actor kind %q", a.Kind)
	}
	return a, nil
}
func (s *Store) permit(action string, _ Question) error {
	if s.Actor.Kind == "owner" {
		return nil
	}
	if s.Actor.Kind != "agent" {
		return &RefusedError{action, "actor.unknown"}
	}
	if s.Permit != nil {
		return s.Permit(action)
	}
	if agentPolicy[action] == ochazuke.AgentNever {
		return &RefusedError{action, "verb declaration"}
	}
	return nil
}
