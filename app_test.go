package main

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func TestSmokePasses(t *testing.T) {
	var out, errOut bytes.Buffer
	code := run([]string{"--smoke"}, strings.NewReader(""), &out, &errOut)
	if code != 0 {
		t.Fatalf("smoke exited %d:\nstdout:\n%s\nstderr:\n%s", code, out.String(), errOut.String())
	}
	if !strings.Contains(out.String(), "smoke: ") || strings.Contains(out.String(), "FAIL ") {
		t.Fatalf("unexpected smoke output:\n%s", out.String())
	}
}

func TestCLIEndToEndAndCatalogQuery(t *testing.T) {
	dir := t.TempDir()
	execCLI := func(args ...string) (int, string, string) {
		var out, errOut bytes.Buffer
		argv := append([]string{"--data", dir}, args...)
		code := run(argv, strings.NewReader(""), &out, &errOut)
		return code, out.String(), errOut.String()
	}

	// --version
	if code, out, _ := execCLI("--version"); code != 0 || !strings.Contains(out, Version) {
		t.Fatalf("version: %d %q", code, out)
	}

	// ask
	code, out, errOut := execCLI("ask", "How does v2 work?", "--title", "V2 Design", "--project", "ochazuke", "--json")
	if code != 0 {
		t.Fatalf("ask failed: %d %s", code, errOut)
	}
	var asked Asked
	if err := json.Unmarshal([]byte(out), &asked); err != nil || asked.Question.ID == "" {
		t.Fatalf("ask json: %v %s", err, out)
	}

	// answer
	if code, _, errOut = execCLI("answer", asked.Question.ID, "With core, ui, tea, and widgets.", "--json"); code != 0 {
		t.Fatalf("answer failed: %d %s", code, errOut)
	}

	// show
	if code, out, errOut = execCLI("show", asked.Question.ID); code != 0 || !strings.Contains(out, "With core, ui, tea, and widgets.") {
		t.Fatalf("show failed: %d %q %q", code, out, errOut)
	}

	// reopen -> resolve -> undo -> redo
	if code, _, errOut = execCLI("reopen", asked.Question.ID); code != 0 {
		t.Fatalf("reopen failed: %d %s", code, errOut)
	}
	if code, _, errOut = execCLI("resolve", asked.Question.ID); code != 0 {
		t.Fatalf("resolve failed: %d %s", code, errOut)
	}
	if code, _, errOut = execCLI("undo"); code != 0 {
		t.Fatalf("undo failed: %d %s", code, errOut)
	}
	if code, _, errOut = execCLI("redo"); code != 0 {
		t.Fatalf("redo failed: %d %s", code, errOut)
	}

	// decisions pose + decide + reload persistence check
	code, out, errOut = execCLI("decisions", "pose", "Enable strict mode?", "--title", "Strict Mode", "--group", "Config", "--options", "yes:Yes,no:No", "--recommended", "yes", "--key", "strict-1", "--json")
	if code != 0 {
		t.Fatalf("decisions pose failed: %d %s", code, errOut)
	}
	var posed Posed
	if err := json.Unmarshal([]byte(out), &posed); err != nil || posed.Question.ID == "" {
		t.Fatalf("pose json: %v %s", err, out)
	}
	if code, _, errOut = execCLI("decisions", "decide", posed.Question.ID, "--action", "yes", "--note", "Locked in", "--json"); code != 0 {
		t.Fatalf("decisions decide failed: %d %s", code, errOut)
	}
	fresh := &Store{Dir: dir, Actor: Owner()}
	reloaded, err := fresh.Resolve(posed.Question.ID, false)
	if err != nil || reloaded.Decision == nil || reloaded.Decision.Action != "yes" || reloaded.Decision.Note != "Locked in" {
		t.Fatalf("decision did not persist atomically: %+v %v", reloaded.Decision, err)
	}

	// catalog
	code, out, errOut = execCLI("catalog")
	if code != 0 {
		t.Fatalf("catalog failed: %d %s", code, errOut)
	}
	var cat manifestDoc
	if err := json.Unmarshal([]byte(out), &cat); err != nil {
		t.Fatalf("catalog json: %v", err)
	}
	if cat.Name != "o-questions" || len(cat.Schemas) != 1 || cat.Schemas[0].Kind != questionKind || len(cat.Commands) < 6 {
		t.Fatalf("unexpected catalog: %+v", cat)
	}

	// query
	code, out, errOut = execCLI("query", "V2 Design")
	if code != 0 {
		t.Fatalf("query failed: %d %s", code, errOut)
	}
	var qr queryResult
	if err := json.Unmarshal([]byte(out), &qr); err != nil {
		t.Fatalf("query json: %v", err)
	}
	if qr.Total != 1 || len(qr.Items) != 1 || qr.Items[0].Kind != questionKind || qr.Items[0].ID != asked.Question.ID {
		t.Fatalf("unexpected query result: %+v", qr)
	}
}
