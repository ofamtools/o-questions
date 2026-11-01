package main

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestReviewMgmtDefault(t *testing.T) {
	for _, setting := range appSettings() {
		if setting.Name == "review.mgmt" {
			if setting.Default != "" {
				t.Fatalf("review.mgmt default = %q, want empty for runtime discovery", setting.Default)
			}
			return
		}
	}
	t.Fatal("review.mgmt setting missing")
}

func TestReviewMgmtResolution(t *testing.T) {
	for _, tt := range []struct {
		name       string
		setting    string
		pathMode   os.FileMode
		legacy     bool
		want       string
		wantErrors []string
	}{
		{name: "setting wins", setting: "custom.py", pathMode: 0700, legacy: true, want: "custom.py"},
		{name: "setting expands home", setting: "~/custom.py", pathMode: 0700, legacy: true, want: "custom.py"},
		{name: "PATH wins over legacy", pathMode: 0700, legacy: true, want: "bin/mgmt.py"},
		{name: "PATH without legacy", pathMode: 0700, want: "bin/mgmt.py"},
		{name: "legacy script need not be executable", legacy: true, want: "hub/ai/utils/mgmt/mgmt.py"},
		{name: "nonexecutable PATH falls back", pathMode: 0600, legacy: true, want: "hub/ai/utils/mgmt/mgmt.py"},
		{name: "missing", wantErrors: []string{"https://github.com/unixsuperhero/ai-utils", "mgmt/", "review.mgmt"}},
		{name: "invalid setting does not fall back", setting: "missing.py", pathMode: 0700, legacy: true, wantErrors: []string{"review.mgmt", "missing.py"}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			s := fixture(t)
			home := os.Getenv("HOME")
			a, err := newApp(s, strings.NewReader(""), &bytes.Buffer{}, &bytes.Buffer{})
			if err != nil {
				t.Fatal(err)
			}
			if _, err = agentStore(s).Pose(Pose{Question: "Use this?", Key: "resolution"}); err != nil {
				t.Fatal(err)
			}
			// Inject discovery inputs after construction: resolution happens at run time.
			bin := filepath.Join(home, "bin")
			t.Setenv("PATH", bin)
			write := func(rel string, mode os.FileMode) {
				t.Helper()
				path := filepath.Join(home, rel)
				if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(path, []byte("# synthetic mgmt; never executed\n"), mode); err != nil {
					t.Fatal(err)
				}
			}
			write("custom.py", 0600)
			if tt.pathMode != 0 {
				write("bin/mgmt.py", tt.pathMode)
			}
			if tt.legacy {
				write("hub/ai/utils/mgmt/mgmt.py", 0600)
			}
			if tt.setting != "" {
				setting := tt.setting
				if !strings.HasPrefix(setting, "~/") {
					setting = filepath.Join(home, setting)
				}
				if err = a.r.Settings().Set("review.mgmt", setting); err != nil {
					t.Fatal(err)
				}
			}
			var got string
			a.run = func(cmd *exec.Cmd) error {
				got = cmd.Args[1]
				return nil
			}
			err = a.reviewCommand([]string{"--web", "--no-open"})
			if len(tt.wantErrors) != 0 {
				if err == nil || got != "" {
					t.Fatalf("err = %v, launched = %q; want error without launch", err, got)
				}
				for _, part := range tt.wantErrors {
					if !strings.Contains(err.Error(), part) {
						t.Errorf("error %q missing %q", err, part)
					}
				}
				return
			}
			if want := filepath.Join(home, tt.want); err != nil || got != want {
				t.Fatalf("script = %q, err = %v; want %q", got, err, want)
			}
		})
	}
}
