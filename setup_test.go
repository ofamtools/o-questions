package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"ofam.dev/ochazuke/setup"
)

func runCLI(t *testing.T, s *Store, args ...string) (string, error) {
	t.Helper()
	var out, errOut bytes.Buffer
	a, e := newApp(s, strings.NewReader(""), &out, &errOut)
	if e != nil {
		t.Fatal(e)
	}
	e = dispatch(a, args)
	return out.String() + errOut.String(), e
}

func setupHome(t *testing.T) (string, string, string) {
	t.Helper()
	t.Setenv("CLAUDE_CONFIG_DIR", "")
	home := t.TempDir()
	if resolved, e := filepath.EvalSymlinks(home); e == nil {
		home = resolved
	}
	claude, codex := filepath.Join(home, ".claude", "CLAUDE.md"), filepath.Join(home, ".codex", "AGENTS.md")
	for _, d := range []string{filepath.Dir(claude), filepath.Dir(codex)} {
		if e := os.MkdirAll(d, 0700); e != nil {
			t.Fatal(e)
		}
	}
	return home, claude, codex
}

const existing = "# Global rules\n\n<!-- h-tuios:begin -->\nkeep me\n<!-- h-tuios:end -->\n"

func TestSetupInstructionNote(t *testing.T) {
	s := fixture(t)
	home, claude, codex := setupHome(t)
	if e := os.WriteFile(claude, []byte(existing), 0600); e != nil {
		t.Fatal(e)
	}
	out, e := runCLI(t, s, "setup", "status", "--home", home)
	if e != nil || !strings.Contains(out, "claude-md: missing") || !strings.Contains(out, "codex-agents: missing") {
		t.Fatal(out, e)
	}
	if _, e = runCLI(t, s, "setup", "install", "--home", home); e == nil {
		t.Fatal("install ran without confirmation")
	}
	if out, e = runCLI(t, s, "setup", "install", "--yes", "--home", home); e != nil {
		t.Fatal(out, e)
	}
	for _, path := range []string{claude, codex} {
		b, _ := os.ReadFile(path)
		text := string(b)
		if strings.Count(text, "<!-- o-questions:begin -->") != 1 || !strings.Contains(text, "## Questions for the owner (o-questions)") || !strings.Contains(text, "question pose") {
			t.Fatalf("%s:\n%s", path, text)
		}
	}
	if b, _ := os.ReadFile(claude); !strings.HasPrefix(string(b), existing) {
		t.Fatalf("existing text changed:\n%s", b)
	}
	first, _ := os.ReadFile(claude)
	if out, e = runCLI(t, s, "setup", "install", "--yes", "--home", home); e != nil {
		t.Fatal(out, e)
	}
	if again, _ := os.ReadFile(claude); !bytes.Equal(first, again) {
		t.Fatal("install is not idempotent")
	}
	if out, _ = runCLI(t, s, "setup", "status", "--home", home); !strings.Contains(out, "claude-md: installed") || !strings.Contains(out, "codex-agents: installed") {
		t.Fatal(out)
	}
	if out, e = runCLI(t, s, "setup", "uninstall", "--yes", "--home", home); e != nil {
		t.Fatal(out, e)
	}
	if b, _ := os.ReadFile(claude); string(b) != existing {
		t.Fatalf("uninstall left:\n%s", b)
	}
	if _, e = os.Stat(codex); !os.IsNotExist(e) {
		t.Fatal("uninstall kept the file it created", e)
	}
}

// Two paths that are one file (a hard link here) hold one note: the items
// share a marker pair, so the second finds the first's block.
func TestSetupOneFileTwoPaths(t *testing.T) {
	s := fixture(t)
	home, claude, codex := setupHome(t)
	if e := os.WriteFile(codex, []byte(existing), 0600); e != nil {
		t.Fatal(e)
	}
	if e := os.Link(codex, claude); e != nil {
		t.Fatal(e)
	}
	for range 2 {
		if out, e := runCLI(t, s, "setup", "install", "--yes", "--home", home); e != nil {
			t.Fatal(out, e)
		}
	}
	b, _ := os.ReadFile(codex)
	if strings.Count(string(b), "<!-- o-questions:begin -->") != 1 {
		t.Fatalf("one file must hold one note:\n%s", b)
	}
	if out, _ := runCLI(t, s, "setup", "status", "--home", home); !strings.Contains(out, "claude-md: installed") || !strings.Contains(out, "codex-agents: installed") {
		t.Fatal(out)
	}
}

func setupPlan(t *testing.T, s *Store, home string) setup.Plan {
	t.Helper()
	out, err := runCLI(t, s, "setup", "plan", "--home", home, "--json", "{}")
	if err != nil {
		t.Fatal(out, err)
	}
	var plan setup.Plan
	if err := json.Unmarshal([]byte(out), &plan); err != nil {
		t.Fatal(out, err)
	}
	return plan
}

func putSetupFile(t *testing.T, path, text string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(text), 0600); err != nil {
		t.Fatal(err)
	}
}

func setupFileText(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func assertSetupLinks(t *testing.T, links map[string]string) {
	t.Helper()
	for link, target := range links {
		got, err := os.Readlink(link)
		if err != nil || got != target {
			t.Fatalf("link %s: got %q, want %q: %v", link, got, target, err)
		}
	}
}

// Exercise the app declaration through the shared CLI, including distinct
// target files, two links to one target, and a link in a parent directory.
func TestSetupFollowSymlinkedPair(t *testing.T) {
	for _, mode := range []string{"distinct", "shared", "parent"} {
		t.Run(mode, func(t *testing.T) {
			s := fixture(t)
			home, claude, codex := setupHome(t)
			t.Setenv("HOME", home)
			one := filepath.Join(home, "repo-a", "CLAUDE.md")
			two := filepath.Join(home, "repo-b", "AGENTS.md")
			if mode == "shared" {
				one = two
			}
			originals := map[string]string{one: existing, two: existing + "\n# Other rules\n"}
			for path, text := range originals {
				putSetupFile(t, path, text)
			}
			links := map[string]string{claude: one, codex: two}
			if mode == "parent" {
				// Use another path for the directory link, leaving setupHome's .claude alone.
				dirLink := filepath.Join(home, "claude-link")
				targetDir := filepath.Dir(one)
				if err := os.Symlink(targetDir, dirLink); err != nil {
					t.Fatal(err)
				}
				t.Setenv("CLAUDE_CONFIG_DIR", dirLink)
				links = map[string]string{dirLink: targetDir, codex: two}
			}
			for link, target := range links {
				if _, err := os.Lstat(link); os.IsNotExist(err) {
					if err := os.Symlink(target, link); err != nil {
						t.Fatal(err)
					}
				}
			}
			// Explicitly enable both even when their resolved directory is shared.
			// This also verifies the owner's persisted override of the derived default.
			config := filepath.Join(os.Getenv("XDG_CONFIG_HOME"), "o-questions", "settings.json")
			putSetupFile(t, config, `{"setup.skip": []}`)
			// --home intentionally ignores inherited harness directories; the parent
			// case uses HOME alone to exercise CLAUDE_CONFIG_DIR.
			argsHome := []string{"--home", home}
			if mode == "parent" {
				argsHome = nil
			}
			if mode != "parent" {
				plan := setupPlan(t, s, home)
				if len(plan.Writes) != len(originals) {
					t.Fatalf("plan: %+v", plan)
				}
				for _, write := range plan.Writes {
					if _, ok := originals[write.Path]; !ok {
						t.Fatalf("write to link instead of target: %+v", write)
					}
				}
			}
			for range 2 {
				args := append([]string{"setup", "install", "--yes"}, argsHome...)
				if out, err := runCLI(t, s, args...); err != nil {
					t.Fatal(out, err)
				}
				assertSetupLinks(t, links)
				for path, original := range originals {
					got := setupFileText(t, path)
					if !strings.HasPrefix(got, original) || strings.Count(got, "<!-- o-questions:begin -->") != 1 {
						t.Fatalf("target %s: %q", path, got)
					}
					if backup := setupFileText(t, path+".o-questions.bak"); backup != original {
						t.Fatalf("backup changed: %q", backup)
					}
				}
			}
			statusArgs := append([]string{"setup", "status"}, argsHome...)
			if out, err := runCLI(t, s, statusArgs...); err != nil || !strings.Contains(out, "claude-md: installed") || !strings.Contains(out, "codex-agents: installed") {
				t.Fatal(out, err)
			}
			if mode != "parent" && len(setupPlan(t, s, home).Writes) != 0 {
				t.Fatal("reinstall would write")
			}
			args := append([]string{"setup", "uninstall", "--yes"}, argsHome...)
			if out, err := runCLI(t, s, args...); err != nil {
				t.Fatal(out, err)
			}
			assertSetupLinks(t, links)
			for path, original := range originals {
				if got := setupFileText(t, path); got != original {
					t.Fatalf("uninstall %s: %q", path, got)
				}
			}
		})
	}
}

func TestSetupImportOnlyInstructions(t *testing.T) {
	s := fixture(t)
	home, claude, codex := setupHome(t)
	t.Setenv("HOME", home)
	dir := filepath.Join(home, "repo", "instructions")
	wrapper, source := filepath.Join(dir, "CLAUDE.md"), filepath.Join(dir, "AGENTS.md")
	putSetupFile(t, wrapper, "@AGENTS.md\n")
	putSetupFile(t, source, existing)
	links := map[string]string{claude: wrapper, codex: source}
	for link, target := range links {
		if err := os.Symlink(target, link); err != nil {
			t.Fatal(err)
		}
	}
	plan := setupPlan(t, s, home)
	if len(plan.Writes) != 1 || plan.Writes[0].Path != source || plan.Items[0].State != setup.Skipped {
		t.Fatalf("double-read plan: %+v", plan)
	}
	for range 2 {
		if out, err := runCLI(t, s, "setup", "install", "--yes", "--home", home); err != nil {
			t.Fatal(out, err)
		}
		if got := setupFileText(t, wrapper); got != "@AGENTS.md\n" {
			t.Fatalf("wrapper changed: %q", got)
		}
		if got := setupFileText(t, source); strings.Count(got, "<!-- o-questions:begin -->") != 1 {
			t.Fatalf("note duplicated: %q", got)
		}
		if got := setupFileText(t, source+".o-questions.bak"); got != existing {
			t.Fatalf("backup: %q", got)
		}
		assertSetupLinks(t, links)
	}
	if _, err := os.Stat(wrapper + ".o-questions.bak"); !os.IsNotExist(err) {
		t.Fatal("skipped wrapper got a backup", err)
	}
	if out, err := runCLI(t, s, "setup", "uninstall", "--yes", "--home", home); err != nil {
		t.Fatal(out, err)
	}
	if got := setupFileText(t, source); got != existing {
		t.Fatalf("uninstall: %q", got)
	}
	assertSetupLinks(t, links)
	// A persisted empty skip list re-enables the wrapper at the owner's choice.
	putSetupFile(t, filepath.Join(os.Getenv("XDG_CONFIG_HOME"), "o-questions", "settings.json"), `{"setup.skip": []}`)
	plan = setupPlan(t, s, home)
	if len(plan.Writes) != 2 {
		t.Fatalf("owner override: %+v", plan)
	}
}

func TestSetupSkipDefaultsUseData(t *testing.T) {
	home := t.TempDir()
	one, two := filepath.Join(home, "a", "one.md"), filepath.Join(home, "a", "two.md")
	putSetupFile(t, one, "first rules\n")
	putSetupFile(t, two, "second rules\n")
	items := []setup.Item{
		{ID: "first", Name: "First", Kind: setup.Block, Path: one, Content: "shared", Begin: "<!-- note:begin -->", End: "<!-- note:end -->", Follow: true},
		{ID: "second", Name: "Second", Kind: setup.Block, Path: two, Content: "shared", Begin: "<!-- note:begin -->", End: "<!-- note:end -->", Follow: true},
	}
	opts := setup.Options{Tool: "fixture", Home: home}
	skip, err := setupSkipDefaults(items, opts)
	if err != nil || !reflect.DeepEqual(skip, []string{"first"}) {
		t.Fatal(skip, err)
	}
	items[0].Content = "different"
	skip, err = setupSkipDefaults(items, opts)
	if err != nil || len(skip) != 0 {
		t.Fatal(skip, err)
	}
	items[0].Content = "shared"
	items[0].Path = filepath.Join(home, "b", "one.md")
	putSetupFile(t, items[0].Path, "separate rules\n")
	skip, err = setupSkipDefaults(items, opts)
	if err != nil || len(skip) != 0 {
		t.Fatal(skip, err)
	}
}
