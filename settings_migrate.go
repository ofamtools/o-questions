package main

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"regexp"
	"slices"
	"strings"
	"time"

	"ofam.dev/ochazuke"
)

// retiredSettings are the settings the decision options change retired. Both
// were global: questions carry their own options now, and the standard
// answers are the rows of decision.outcomes.
var retiredSettings = []string{"decision.actions", "decision.ask"}

// foldedRows say which outcome row each retired boolean setting removes when
// it is false (data, so the rewrite branches on no name).
var foldedRows = map[string]string{"decision.ask": "ask"}

var (
	retiredMember = regexp.MustCompile(`[ \t]*"decision\.(?:actions|ask)"\s*:\s*(?:\[[^\]]*\]|"[^"]*"|[^,\s}\]]+)[ \t]*,?[ \t]*\n?`)
	danglingComma = regexp.MustCompile(`,(\s*[}\]])`)
)

// migrateSettingsFile rewrites the owner's settings file once: decision.actions
// is dropped (there are no default options; a custom list is reported, not
// kept), and decision.ask=false removes the ask row from decision.outcomes
// (written out in full when the file did not set it). The file is edited as
// text so its comments and layout stay. A file without a retired key is not
// touched and not backed up; otherwise the original is copied to
// <file>.bak-<date> first (an existing backup of the day gets -2, -3, ...).
// It returns notices for the owner and does nothing when the file is absent.
func migrateSettingsFile(path string, now time.Time) (notices []string, err error) {
	if path == "" {
		return nil, nil
	}
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var cur map[string]any
	if err = ochazuke.DecodeJSONC(data, &cur); err != nil {
		return nil, nil // not valid: the framework reports it; leave it alone
	}
	if !slices.ContainsFunc(retiredSettings, func(k string) bool { _, has := cur[k]; return has }) {
		return nil, nil
	}
	if v, has := cur["decision.actions"]; has {
		if list, ok := v.([]any); ok && !sameWords(list, []string{"yes", "no", "later", "moot"}) {
			notices = append(notices, fmt.Sprintf("decision.actions %v was removed: questions carry their own options and none are added by default", list))
		}
	}
	text := retiredMember.ReplaceAllString(string(data), "")
	for setting, row := range foldedRows {
		if off, has := cur[setting]; has && off == false {
			notices = append(notices, setting+"=false became a decision.outcomes list without the "+row+" row")
			text = withoutOutcomeRow(text, cur, row)
		}
	}
	text = danglingComma.ReplaceAllString(text, "$1")
	var check map[string]any
	if err = ochazuke.DecodeJSONC([]byte(text), &check); err != nil {
		return nil, fmt.Errorf("settings rewrite would not parse, file left as is: %w", err)
	}
	for _, k := range retiredSettings {
		if _, still := check[k]; still {
			return nil, fmt.Errorf("settings rewrite left %s, file left as is", k)
		}
	}
	if err = backupConfig(path, data, now); err != nil {
		return nil, err
	}
	if err = atomicBytes(path, []byte(text)); err != nil {
		return nil, err
	}
	return append([]string{"settings retired " + strings.Join(retiredSettings, ", ") + " (backup kept next to " + path + ")"}, notices...), nil
}

func sameWords(list []any, want []string) bool {
	if len(list) != len(want) {
		return false
	}
	for i, v := range list {
		if s, _ := v.(string); s != want[i] {
			return false
		}
	}
	return true
}

// withoutOutcomeRow takes one row out of decision.outcomes: edited in place
// when the file lists it, else the default rows less that one are added.
func withoutOutcomeRow(text string, cur map[string]any, drop string) string {
	if _, has := cur["decision.outcomes"]; has {
		return regexp.MustCompile(`"`+regexp.QuoteMeta(drop)+`\|[^"]*"\s*,?\s*`).ReplaceAllString(text, "")
	}
	var rows []string
	for _, row := range defaultOutcomeRows {
		if id, _, _ := strings.Cut(row, "|"); id != drop {
			rows = append(rows, fmt.Sprintf("%q", row))
		}
	}
	member := `  "decision.outcomes": [` + strings.Join(rows, ", ") + "],"
	if i := strings.Index(text, "{"); i >= 0 {
		return text[:i+1] + "\n" + member + text[i+1:]
	}
	return text
}

// backupConfig copies an owner config file to <file>.bak-<date> before it is
// rewritten, never overwriting an earlier backup.
func backupConfig(path string, data []byte, now time.Time) error {
	base := path + ".bak-" + now.Format("2006-01-02")
	for n := 1; ; n++ {
		name := base
		if n > 1 {
			name = fmt.Sprintf("%s-%d", base, n)
		}
		f, err := os.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
		if errors.Is(err, fs.ErrExist) {
			continue
		}
		if err != nil {
			return err
		}
		_, err = f.Write(data)
		if cerr := f.Close(); err == nil {
			err = cerr
		}
		return err
	}
}
