// JSON-RPC framing and lifecycle copied and adapted from o-cmds/mcp.go.
package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"math"
	"slices"
	"strings"
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

var mcpAllowlist = map[string][]string{
	"ask":      {"question", "title", "project", "cwd", "task", "tags", "links", "asked_at", "follow_up_of", "key"},
	"answer":   {"target", "answer", "summary", "model", "links", "tags"},
	"note":     {"target", "text", "kind", "replaces"},
	"annotate": {"target", "tags", "links", "project", "task", "title"},
	"list":     {"filter", "status", "project", "pane", "agent_session", "since", "limit"},
	"show":     {"target"},
}

func (a *App) tools() []mcpTool {
	var out []mcpTool
	for _, v := range a.specs() {
		names, ok := mcpAllowlist[v.name]
		if !ok {
			continue
		}
		props := map[string]any{}
		req := []string{}
		for _, name := range names {
			p := v.params[name]
			field := map[string]any{"type": p.Type, "description": p.Description}
			if len(p.Enum) > 0 {
				field["enum"] = p.Enum
			}
			if p.Type == "array" {
				field["items"] = map[string]any{"type": "string"}
				if name == "links" {
					field["items"] = map[string]any{"type": "object", "properties": map[string]any{"kind": map[string]string{"type": "string"}, "target": map[string]string{"type": "string"}, "label": map[string]string{"type": "string"}}, "required": []string{"target"}, "additionalProperties": false}
				}
			}
			if p.Required || name == "target" {
				req = append(req, name)
			}
			props[name] = field
		}
		schema := map[string]any{"type": "object", "properties": props, "additionalProperties": false}
		if len(req) > 0 {
			schema["required"] = req
		}
		out = append(out, mcpTool{Name: "q_" + v.name, Description: v.description, InputSchema: schema})
	}
	return out
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

// Validate exactly the published schema before calling the shared CLI verbs.
// The MCP actor cannot be overridden by tool input or an owner CLI environment.
func (a *App) callTool(name string, raw json.RawMessage, client string) any {
	var tool *mcpTool
	for _, t := range a.tools() {
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
		case "integer":
			n, ok := v.(float64)
			valid = ok && n == math.Trunc(n) && n >= 0
		case "array":
			xs, ok := v.([]any)
			valid = ok
			for _, x := range xs {
				if k == "links" {
					m, ok := x.(map[string]any)
					if !ok {
						valid = false
						break
					}
					target, ok := m["target"].(string)
					valid = valid && ok && target != ""
					for key, value := range m {
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
	previous := a.store.Actor
	act := previous
	act.Kind = "agent"
	if act.Name == "" || act.Name == "owner" {
		act.Name = client
	}
	if act.Harness == "" {
		act.Harness = client
	}
	a.store.Actor = act
	defer func() { a.store.Actor = previous }()
	var buf bytes.Buffer
	sh := a.shell()
	sh.Stdout = &buf
	e := sh.Run(context.Background(), []string{"question", strings.TrimPrefix(name, "q_"), "--json", string(raw)})
	if e != nil {
		return toolContent(nil, e)
	}
	var value any
	if e = json.Unmarshal(buf.Bytes(), &value); e != nil {
		return toolContent(nil, e)
	}
	return toolContent(value, nil)
}

// Stdio framing and the initialize/tools lifecycle follow the family's tuios
// MCP server: newline-delimited JSON-RPC, notifications receive no response.
func (a *App) serveMCP() error {
	scan := bufio.NewScanner(a.in)
	scan.Buffer(make([]byte, 64*1024), 16*1024*1024)
	enc := json.NewEncoder(a.out)
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
					resp.Result = map[string]any{"protocolVersion": protocol, "capabilities": map[string]any{"tools": map[string]any{}}, "serverInfo": map[string]string{"name": "o-questions", "version": version}}
				case "ping":
					resp.Result = map[string]any{}
				case "tools/list":
					resp.Result = map[string]any{"tools": a.tools()}
				case "tools/call":
					var p struct {
						Name      string          `json:"name"`
						Arguments json.RawMessage `json:"arguments"`
					}
					if e := json.Unmarshal(q.Params, &p); e != nil || p.Name == "" {
						resp.Error = &rpcError{-32602, "invalid tool parameters"}
					} else {
						resp.Result = a.callTool(p.Name, p.Arguments, client)
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
