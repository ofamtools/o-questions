// JSON-RPC framing and lifecycle copied and adapted from o-cmds/mcp.go.
package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"slices"
	"strings"
	"time"
)

type rpcRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params"`
}
type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}
type rpcResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Result  any             `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}
type mcpTool struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	InputSchema map[string]any `json:"inputSchema"`
}

type mcpServer struct {
	store *Store
	in    io.Reader
	out   io.Writer
}

func newMCPServer(s *Store, in io.Reader, out io.Writer) *mcpServer {
	return &mcpServer{store: s, in: in, out: out}
}

func (m *mcpServer) tools() []mcpTool {
	str := func(desc string) map[string]any { return map[string]any{"type": "string", "description": desc} }
	boolp := func(desc string) map[string]any { return map[string]any{"type": "boolean", "description": desc} }
	intp := func(desc string) map[string]any { return map[string]any{"type": "integer", "description": desc} }
	arrStr := func(desc string) map[string]any {
		return map[string]any{"type": "array", "description": desc, "items": map[string]any{"type": "string"}}
	}
	linksSchema := map[string]any{
		"type":        "array",
		"description": "Links attached to the question or entry",
		"items": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"kind":   map[string]string{"type": "string"},
				"target": map[string]string{"type": "string"},
				"label":  map[string]string{"type": "string"},
			},
			"required":             []string{"target"},
			"additionalProperties": false,
		},
	}
	obj := func(props map[string]any, req ...string) map[string]any {
		s := map[string]any{"type": "object", "properties": props, "additionalProperties": false}
		if len(req) > 0 {
			s["required"] = req
		}
		return s
	}
	return []mcpTool{
		{Name: "q_ask", Description: "Record a new question", InputSchema: obj(map[string]any{
			"question": str("Question text"), "title": str("Short title"), "project": str("Project"),
			"cwd": str("Working directory"), "task": str("Task id"), "tags": arrStr("Tags"),
			"links": linksSchema, "asked_at": str("RFC 3339 timestamp"), "follow_up_of": str("Parent question id"), "key": str("Idempotency key"),
		}, "question")},
		{Name: "q_answer", Description: "Append an answer to a question", InputSchema: obj(map[string]any{
			"target": str("Question id, prefix, title or index"), "answer": str("Answer text"), "summary": str("Short summary"),
			"model": str("Model name"), "links": linksSchema, "tags": arrStr("Tags"),
		}, "target", "answer")},
		{Name: "q_annotate", Description: "Add a note, correction, tags or links to a question", InputSchema: obj(map[string]any{
			"target": str("Question id, prefix, title or index"), "text": str("Entry text"),
			"kind":     map[string]any{"type": "string", "description": "Entry kind", "enum": []string{"note", "correction", "follow-up"}},
			"replaces": str("Entry id replaced by a correction"), "tags": arrStr("Tags"), "links": linksSchema,
			"project": str("Project"), "task": str("Task id"), "title": str("New title"),
		}, "target")},
		{Name: "q_list", Description: "List questions", InputSchema: obj(map[string]any{
			"filter": str("Search text"), "status": map[string]any{"type": "string", "description": "Filter status", "enum": statuses},
			"project": str("Project filter"), "pane": str("Pane filter"), "agent_session": str("Agent session filter"),
			"since": str("RFC 3339 lower bound"), "limit": intp("Maximum rows"),
		})},
		{Name: "q_show", Description: "Show one question and its thread", InputSchema: obj(map[string]any{
			"target": str("Question id, prefix, title or index"),
		}, "target")},
		{Name: "q_decision_pose", Description: "Pose a decision question for the owner", InputSchema: obj(map[string]any{
			"question": str("Question text"), "title": str("Short title"), "group": str("Topic group"),
			"source": str("Origin path or ref"), "details": str("Extra context"), "recommendation": str("Recommended rationale"),
			"recommended": str("Recommended option id"), "options": arrStr("Option ids or id:label"), "items": arrStr("Checklist items"),
			"project": str("Project"), "cwd": str("Working directory"), "task": str("Task id"), "tags": arrStr("Tags"),
			"links": linksSchema, "key": str("Idempotency key"),
		}, "question")},
		{Name: "q_decision_queue", Description: "List owner decision questions", InputSchema: obj(map[string]any{
			"show": str("Filter class"), "group": str("Topic group"),
		})},
		{Name: "q_decision_export", Description: "Export decided owner questions", InputSchema: obj(map[string]any{
			"group": str("Topic group"), "all": boolp("Include applied"), "render": str("Render format"), "date": str("Date stamp"),
		})},
		{Name: "q_decision_reply", Description: "Reply to an owner ask-back on a decision", InputSchema: obj(map[string]any{
			"target": str("Decision question id"), "text": str("Reply text"), "recommendation": str("Updated recommendation"),
			"recommended": str("Updated recommended option"), "question": str("Updated question text"),
		}, "target", "text")},
		{Name: "q_decision_applied", Description: "Record a decision as applied or failed", InputSchema: obj(map[string]any{
			"target": str("Decision question id"), "item": str("Checklist item id"), "outcome": str("done or failed"), "result": str("Result summary"),
		}, "target")},
		{Name: "q_decision_import", Description: "Import open owner questions from a plans folder", InputSchema: obj(map[string]any{
			"root": str("Plans folder"), "include_answered": boolp("Include already answered"),
		})},
		{Name: "q_decision_archive-matching", Description: "Archive or preview matching owner decisions", InputSchema: obj(map[string]any{
			"show": str("State filter"), "group": str("Topic group"), "days": intp("Older than N days"), "apply": boolp("Actually archive"),
		})},
	}
}

func toolContent(v any, err error) any {
	text := ""
	if err != nil {
		text = err.Error()
	} else {
		b, e := json.Marshal(v)
		if e != nil {
			err = e
			text = e.Error()
		} else {
			text = string(b)
		}
	}
	return map[string]any{"content": []any{map[string]string{"type": "text", "text": text}}, "isError": err != nil}
}

// Validate exactly the published schema before calling the store directly.
// The MCP actor cannot be overridden by tool input or an owner CLI environment.
func (m *mcpServer) callTool(name string, raw json.RawMessage, client string) any {
	var tool *mcpTool
	for _, t := range m.tools() {
		if t.Name == name {
			tool = &t
			break
		}
	}
	if tool == nil {
		return toolContent(nil, fmt.Errorf("unknown tool %q", name))
	}
	if len(raw) == 0 {
		raw = []byte("{}")
	}
	var args map[string]any
	if e := json.Unmarshal(raw, &args); e != nil {
		return toolContent(nil, e)
	}
	if args == nil {
		return toolContent(nil, fmt.Errorf("arguments must be an object"))
	}
	props := tool.InputSchema["properties"].(map[string]any)
	required, _ := tool.InputSchema["required"].([]string)
	for _, k := range required {
		if s, ok := args[k].(string); !ok || strings.TrimSpace(s) == "" {
			return toolContent(nil, fmt.Errorf("%s is required", k))
		}
	}
	for k, v := range args {
		p, ok := props[k].(map[string]any)
		if !ok {
			return toolContent(nil, fmt.Errorf("unknown argument %s", k))
		}
		typ := p["type"].(string)
		valid := false
		switch typ {
		case "string":
			_, valid = v.(string)
		case "boolean":
			_, valid = v.(bool)
		case "integer":
			n, ok := v.(float64)
			valid = ok && n == math.Trunc(n) && n >= 0
		case "array":
			xs, ok := v.([]any)
			valid = ok
			for _, x := range xs {
				if k == "links" {
					lm, ok := x.(map[string]any)
					if !ok {
						valid = false
						break
					}
					target, ok := lm["target"].(string)
					valid = valid && ok && target != ""
					for key, value := range lm {
						_, ok := value.(string)
						valid = valid && ok && slices.Contains([]string{"kind", "target", "label"}, key)
					}
				} else {
					_, ok := x.(string)
					valid = valid && ok
				}
			}
		}
		if !valid {
			return toolContent(nil, fmt.Errorf("%s must match %s schema", k, typ))
		}
		if en, ok := p["enum"].([]string); ok && !slices.Contains(en, v.(string)) {
			return toolContent(nil, fmt.Errorf("invalid %s", k))
		}
	}
	previous := m.store.Actor
	act := previous
	act.Kind = "agent"
	if act.Name == "" || act.Name == "owner" {
		act.Name = client
	}
	if act.Harness == "" {
		act.Harness = client
	}
	m.store.Actor = act
	defer func() { m.store.Actor = previous }()

	val, err := m.dispatchTool(name, args)
	return toolContent(val, err)
}

func mcpStr(args map[string]any, k string) string { s, _ := args[k].(string); return s }
func mcpBool(args map[string]any, k string) bool  { b, _ := args[k].(bool); return b }
func mcpInt(args map[string]any, k string) int    { f, _ := args[k].(float64); return int(f) }
func mcpStrings(args map[string]any, k string) []string {
	xs, _ := args[k].([]any)
	out := make([]string, 0, len(xs))
	for _, x := range xs {
		if s, ok := x.(string); ok {
			out = append(out, s)
		}
	}
	return out
}
func mcpLinks(args map[string]any, k string) []Link {
	xs, _ := args[k].([]any)
	out := make([]Link, 0, len(xs))
	for _, x := range xs {
		if lm, ok := x.(map[string]any); ok {
			l := Link{}
			l.Kind, _ = lm["kind"].(string)
			l.Target, _ = lm["target"].(string)
			l.Label, _ = lm["label"].(string)
			out = append(out, l)
		}
	}
	return out
}

func (m *mcpServer) dispatchTool(name string, args map[string]any) (any, error) {
	s := m.store
	switch name {
	case "q_ask":
		return s.Ask(Ask{
			Question:   mcpStr(args, "question"),
			Title:      mcpStr(args, "title"),
			Project:    mcpStr(args, "project"),
			Cwd:        mcpStr(args, "cwd"),
			Task:       mcpStr(args, "task"),
			Tags:       mcpStrings(args, "tags"),
			Links:      mcpLinks(args, "links"),
			AskedAt:    mcpStr(args, "asked_at"),
			FollowUpOf: mcpStr(args, "follow_up_of"),
			Key:        mcpStr(args, "key"),
			Source:     sourceFromEnv(s.Actor),
		})
	case "q_answer":
		if mod := mcpStr(args, "model"); mod != "" {
			prevModel := s.Actor.Model
			s.Actor.Model = mod
			defer func() { s.Actor.Model = prevModel }()
		}
		q, err := s.Append(mcpStr(args, "target"), Entry{
			Kind:    "answer",
			Text:    mcpStr(args, "answer"),
			Summary: mcpStr(args, "summary"),
			Links:   mcpLinks(args, "links"),
		})
		if err != nil {
			return nil, err
		}
		if tags := mcpStrings(args, "tags"); len(tags) > 0 {
			q, err = s.Annotate(q.ID, Annotation{Tags: tags})
		}
		return q, err
	case "q_annotate":
		target := mcpStr(args, "target")
		var q Question
		var err error
		if text := mcpStr(args, "text"); text != "" {
			kind := mcpStr(args, "kind")
			if kind == "" {
				kind = "note"
			}
			q, err = s.Append(target, Entry{
				Kind:     kind,
				Text:     text,
				Replaces: mcpStr(args, "replaces"),
				Links:    mcpLinks(args, "links"),
			})
			if err != nil {
				return nil, err
			}
			target = q.ID
		}
		ann := Annotation{
			Tags:    mcpStrings(args, "tags"),
			Links:   mcpLinks(args, "links"),
			Project: mcpStr(args, "project"),
			Task:    mcpStr(args, "task"),
			Title:   mcpStr(args, "title"),
		}
		if len(ann.Tags) > 0 || len(ann.Links) > 0 || ann.Project != "" || ann.Task != "" || ann.Title != "" || q.ID == "" {
			q, err = s.Annotate(target, ann)
		}
		return q, err
	case "q_list":
		rows, err := s.List()
		if err != nil {
			return nil, err
		}
		var since time.Time
		if raw := mcpStr(args, "since"); raw != "" {
			since, err = time.Parse(time.RFC3339, raw)
			if err != nil {
				return nil, err
			}
		}
		filter := strings.ToLower(mcpStr(args, "filter"))
		status := mcpStr(args, "status")
		proj := mcpStr(args, "project")
		pane := mcpStr(args, "pane")
		sess := mcpStr(args, "agent_session")
		limit := mcpInt(args, "limit")
		out := []Question{}
		for _, q := range rows {
			if status != "" && q.Status != status {
				continue
			}
			if proj != "" && !strings.EqualFold(q.Project.ID, proj) && !strings.EqualFold(q.Project.Shown(), proj) {
				continue
			}
			if pane != "" && q.Source.Pane != pane {
				continue
			}
			if sess != "" && q.Source.AgentSession != sess {
				continue
			}
			if !since.IsZero() && q.Updated.Before(since) {
				continue
			}
			if filter != "" && !strings.Contains(strings.ToLower(q.Title+"\x00"+q.Text), filter) {
				continue
			}
			out = append(out, q)
			if limit > 0 && len(out) >= limit {
				break
			}
		}
		return out, nil
	case "q_show":
		return s.Resolve(mcpStr(args, "target"), false)
	case "q_decision_pose":
		var opts []Option
		for _, raw := range mcpStrings(args, "options") {
			id, lbl, _ := strings.Cut(raw, ":")
			opts = append(opts, Option{ID: strings.TrimSpace(id), Label: strings.TrimSpace(lbl)})
		}
		var items []Item
		for _, raw := range mcpStrings(args, "items") {
			id, lbl, _ := strings.Cut(raw, ":")
			items = append(items, Item{ID: strings.TrimSpace(id), Label: strings.TrimSpace(lbl)})
		}
		return s.Pose(Pose{
			Question:       mcpStr(args, "question"),
			Title:          mcpStr(args, "title"),
			Group:          mcpStr(args, "group"),
			Source:         mcpStr(args, "source"),
			Details:        mcpStr(args, "details"),
			Recommendation: mcpStr(args, "recommendation"),
			Recommended:    mcpStr(args, "recommended"),
			Options:        opts,
			Items:          items,
			Project:        mcpStr(args, "project"),
			Cwd:            mcpStr(args, "cwd"),
			Task:           mcpStr(args, "task"),
			Tags:           mcpStrings(args, "tags"),
			Links:          mcpLinks(args, "links"),
			Key:            mcpStr(args, "key"),
			Provenance:     sourceFromEnv(s.Actor),
		})
	case "q_decision_queue":
		qs, err := s.ownerQuestions()
		if err != nil {
			return nil, err
		}
		show := mcpStr(args, "show")
		grp := mcpStr(args, "group")
		out := []Question{}
		for _, q := range s.orderQuestions(qs, "undecided-first") {
			if grp != "" && groupOf(q) != grp {
				continue
			}
			if show != "" && !s.decisionShown(q, show) {
				continue
			}
			out = append(out, q)
		}
		return out, nil
	case "q_decision_export":
		if mcpStr(args, "render") == "plan" {
			return s.PlanDecisions(mcpStr(args, "date"))
		}
		return s.DecisionExport(mcpStr(args, "group"), mcpBool(args, "all"))
	case "q_decision_reply":
		rp := Reply{Text: mcpStr(args, "text")}
		if v, ok := args["recommendation"].(string); ok {
			rp.Recommendation = &v
		}
		if v, ok := args["recommended"].(string); ok {
			rp.Recommended = &v
		}
		if v, ok := args["question"].(string); ok {
			rp.Question = v
		}
		return s.Reply(mcpStr(args, "target"), rp)
	case "q_decision_applied":
		return s.Applied(mcpStr(args, "target"), mcpStr(args, "item"), mcpStr(args, "outcome"), mcpStr(args, "result"))
	case "q_decision_import":
		return s.ImportPlans(mcpStr(args, "root"), mcpBool(args, "include_answered"))
	case "q_decision_archive-matching":
		return s.ArchiveMatching(mcpStr(args, "show"), mcpStr(args, "group"), mcpInt(args, "days"), mcpBool(args, "apply"))
	default:
		return nil, fmt.Errorf("unknown tool %q", name)
	}
}

// serveMCP serves newline-delimited JSON-RPC 2.0 over stdio.
func (m *mcpServer) serveMCP() error {
	scan := bufio.NewScanner(m.in)
	scan.Buffer(make([]byte, 64*1024), 16*1024*1024)
	enc := json.NewEncoder(m.out)
	client := "mcp"
	for scan.Scan() {
		line := scan.Bytes()
		if len(bytes.TrimSpace(line)) == 0 {
			continue
		}
		var q rpcRequest
		resp := rpcResponse{JSONRPC: "2.0", ID: json.RawMessage("null")}
		if e := json.Unmarshal(line, &q); e != nil {
			resp.Error = &rpcError{-32700, "parse error"}
		} else {
			if len(q.ID) > 0 {
				resp.ID = q.ID
			}
			if q.JSONRPC != "2.0" || q.Method == "" {
				resp.Error = &rpcError{-32600, "invalid request"}
			} else if len(q.ID) == 0 {
				continue
			} else {
				switch q.Method {
				case "initialize":
					var p struct {
						Protocol string `json:"protocolVersion"`
						Client   struct {
							Name string `json:"name"`
						} `json:"clientInfo"`
					}
					if e := json.Unmarshal(q.Params, &p); e != nil {
						resp.Error = &rpcError{-32602, "invalid initialize parameters"}
						break
					}
					protocol := "2025-11-25"
					for _, v := range []string{"2024-11-05", "2025-03-26", "2025-06-18", "2025-11-25"} {
						if p.Protocol == v {
							protocol = v
						}
					}
					if p.Client.Name != "" {
						client = p.Client.Name
					}
					resp.Result = map[string]any{"protocolVersion": protocol, "capabilities": map[string]any{"tools": map[string]any{}}, "serverInfo": map[string]string{"name": "o-questions", "version": Version}}
				case "ping":
					resp.Result = map[string]any{}
				case "tools/list":
					resp.Result = map[string]any{"tools": m.tools()}
				case "tools/call":
					var p struct {
						Name      string          `json:"name"`
						Arguments json.RawMessage `json:"arguments"`
					}
					if e := json.Unmarshal(q.Params, &p); e != nil || p.Name == "" {
						resp.Error = &rpcError{-32602, "invalid tool parameters"}
					} else {
						resp.Result = m.callTool(p.Name, p.Arguments, client)
					}
				default:
					resp.Error = &rpcError{-32601, "method not found"}
				}
			}
		}
		if e := enc.Encode(resp); e != nil {
			return e
		}
	}
	return scan.Err()
}
