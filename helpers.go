package main

import (
	"fmt"
	"os"
	"strings"
	"time"
)

const (
	Version      = "0.2.0"
	version      = Version
	Namespace    = "question"
	decisionNoun = "decision"
)

var defaultOutcomeRows = []string{
	"later|Later|answered|later||l",
	"moot|Moot|moot|decided||m",
	"ask|Ask the agent|follow-up|asked|reply|?",
	"other|Write your own answer|answered|decided|note|w",
}

func sourceFromEnv(a Actor) Source {
	return Source{
		Harness:      a.Harness,
		AgentSession: os.Getenv("AGENT_SESSION"),
		Pane:         a.Pane,
		MtmSession:   a.Session,
		Transcript:   os.Getenv("AGENT_TRANSCRIPT"),
		Turn:         os.Getenv("AGENT_TURN"),
	}
}

func age(at, now time.Time) string {
	d := max(now.Sub(at), 0)
	switch {
	case d < time.Minute:
		return fmt.Sprintf("%ds", int(d.Seconds()))
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh", int(d.Hours()))
	default:
		return fmt.Sprintf("%dd", int(d.Hours()/24))
	}
}

func itemRowQuestion(id string) string {
	if rest, ok := strings.CutPrefix(id, "item/"); ok {
		if qid, _, ok := strings.Cut(rest, "/"); ok {
			return qid
		}
	}
	return id
}

func (s *Store) decisionCell(q Question) string {
	r := q.Decision
	class := s.decisionClass(q)
	cell := s.answerLabel(r)
	if r.Action == "" {
		cell = r.progress()
	}
	if r.locked() {
		cell = "✓ " + cell
	}
	if class == classUndecided {
		cell = "·"
		if rec := s.recommendedLabel(r); rec != "" {
			cell = rec + "?"
		}
		if p := r.progress(); p != "" {
			cell = p
		}
	}
	if r.Status == decisionFailed {
		cell += " ✗"
	}
	if r.ContextChanged || r.NewReply {
		cell = "! " + cell
	}
	return cell
}

func (s *Store) recommendedLabel(r *Decision) string {
	id := r.recommendedID()
	for _, o := range r.rawOptions() {
		if o.ID == id && id != "" {
			return o.label()
		}
	}
	return ""
}

func (s *Store) recommendationView(q Question) string {
	r := q.Decision
	rec := s.recommendedLabel(r)
	if rec == "" && r.Recommendation == "" {
		return ""
	}
	state := "recommended · not confirmed"
	switch {
	case accepted(r):
		state = "recommended · accepted"
	case r.Action != "" && r.recommendedID() != "":
		state = "recommended · you chose " + s.answerLabel(r)
	}
	if rec == "" {
		return r.Recommendation
	}
	return fmt.Sprintf("**%s** (%s)\n\n%s", rec, state, r.Recommendation)
}
