package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
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

// Symlinked instruction files (the owner's are links into a dotfiles repo)
// are external to ochazuke setup: reported with the note as the next step,
// never written through.
func TestSetupSymlinkedPairIsExternal(t *testing.T) {
	s := fixture(t)
	home, claude, codex := setupHome(t)
	source := filepath.Join(home, "hub", "AGENTS.md")
	if e := os.MkdirAll(filepath.Dir(source), 0700); e != nil {
		t.Fatal(e)
	}
	if e := os.WriteFile(source, []byte(existing), 0600); e != nil {
		t.Fatal(e)
	}
	for _, link := range []string{claude, codex} {
		if e := os.Symlink(source, link); e != nil {
			t.Fatal(e)
		}
	}
	out, e := runCLI(t, s, "setup", "status", "--home", home)
	if e != nil || !strings.Contains(out, "claude-md: external") || !strings.Contains(out, "codex-agents: external") {
		t.Fatal(out, e)
	}
	// Install refuses the external items rather than writing through the link.
	if out, e = runCLI(t, s, "setup", "install", "--yes", "--home", home); e == nil || !strings.Contains(out+e.Error(), "is a symlink") {
		t.Fatal(out, e)
	}
	if b, _ := os.ReadFile(source); string(b) != existing {
		t.Fatalf("wrote through a symlink:\n%s", b)
	}
}
