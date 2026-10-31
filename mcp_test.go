package main

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func TestMCPLifecycleAndIdempotency(t *testing.T) {
	s := fixture(t)
	input := strings.Join([]string{
		`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-11-25","clientInfo":{"name":"fixture-agent"}}}`,
		`{"jsonrpc":"2.0","method":"notifications/initialized"}`,
		`{"jsonrpc":"2.0","id":2,"method":"tools/list"}`,
		`{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"q_ask","arguments":{"question":"  Exact wording?\nYes?  ","key":"retry","links":[{"kind":"url","target":"https://example.test"}]}}}`,
		`{"jsonrpc":"2.0","id":4,"method":"tools/call","params":{"name":"q_answer","arguments":{"target":"1","answer":"Full answer","model":"fixture-model","tags":["demo"]}}}`,
		`{"jsonrpc":"2.0","id":5,"method":"tools/call","params":{"name":"q_list","arguments":{}}}`,
		`{"jsonrpc":"2.0","id":6,"method":"tools/call","params":{"name":"q_show","arguments":{"target":"1"}}}`,
		`{"jsonrpc":"2.0","id":7,"method":"tools/call","params":{"name":"q_ask","arguments":{"question":"  Exact wording?\nYes?  ","key":"retry"}}}`,
		`{"jsonrpc":"2.0","id":8,"method":"tools/call","params":{"name":"q_delete","arguments":{"target":"1"}}}`,
		`{"jsonrpc":"2.0","id":9,"method":"ping"}`,
	}, "\n") + "\n"
	var out, errOut bytes.Buffer
	a, e := newApp(s, strings.NewReader(input), &out, &errOut)
	if e != nil {
		t.Fatal(e)
	}
	if e = a.mcp(); e != nil {
		t.Fatal(e)
	}
	lines := strings.Split(strings.TrimSpace(out.String()), "\n")
	if len(lines) != 9 {
		t.Fatal(out.String(), errOut.String())
	}
	for i, line := range lines {
		if i != 7 && strings.Contains(line, `"isError":true`) {
			t.Fatal(line, errOut.String())
		}
	}
	var tools struct {
		Result struct {
			Tools []mcpTool `json:"tools"`
		} `json:"result"`
	}
	if e = json.Unmarshal([]byte(lines[1]), &tools); e != nil {
		t.Fatal(e)
	}
	if len(tools.Result.Tools) != 6 {
		t.Fatal(tools)
	}
	rows, e := s.List()
	if e != nil || len(rows) != 1 {
		t.Fatal(rows, e)
	}
	q := rows[0]
	if q.Text != "  Exact wording?\nYes?  " || q.RecordedBy.Kind != "agent" || q.RecordedBy.Name != "fixture-agent" || len(q.Entries) != 1 || q.Entries[0].By.Model != "fixture-model" || q.Status != "answered" {
		t.Fatal(q)
	}
	if !strings.Contains(lines[6], `\"existed\":true`) {
		t.Fatal(lines[6])
	}
	if !strings.Contains(lines[7], `"isError":true`) {
		t.Fatal(lines[7])
	}
}
func TestMCPSchemaAndRefusals(t *testing.T) {
	s := fixture(t)
	var out bytes.Buffer
	a, e := newApp(s, strings.NewReader("invalid\n{\"jsonrpc\":\"2.0\",\"id\":9,\"method\":\"missing\"}\n"), &out, &out)
	if e != nil {
		t.Fatal(e)
	}
	if e = a.mcp(); e != nil {
		t.Fatal(e)
	}
	if !strings.Contains(out.String(), "-32700") || !strings.Contains(out.String(), "-32601") {
		t.Fatal(out.String())
	}
	for _, c := range []struct{ name, raw string }{{"q_ask", `{}`}, {"q_ask", `{"question":""}`}, {"q_ask", `{"question":"Q?","actor":"owner"}`}, {"q_ask", `{"question":"Q?","links":["url"]}`}, {"q_list", `{"limit":1.5}`}, {"q_note", `{"target":"1","text":"x","kind":"answer"}`}, {"q_annotate", `{"target":"1","status":"moot"}`}, {"q_ask", `null`}} {
		b, _ := json.Marshal(a.callTool(c.name, json.RawMessage(c.raw), "fixture"))
		if !strings.Contains(string(b), `"isError":true`) {
			t.Fatalf("%s %s: %s", c.name, c.raw, b)
		}
	}
	b, _ := json.Marshal(a.callTool("q_ask", json.RawMessage(`{"question":"Q?"}`), "fixture"))
	if strings.Contains(string(b), `"isError":true`) {
		t.Fatal(string(b))
	}
	b, _ = json.Marshal(a.callTool("q_note", json.RawMessage(`{"target":"1","text":"Note"}`), "fixture"))
	if strings.Contains(string(b), `"isError":true`) {
		t.Fatal(string(b))
	}
}
