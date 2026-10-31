package main

import (
	"bytes"
	"context"
	_ "embed"
	"path/filepath"

	"ofam.dev/ochazuke"
	ocli "ofam.dev/ochazuke/cli"
	"ofam.dev/ochazuke/setup"
)

// setup.json declares o-questions' integrations as data for ochazuke's
// shared setup verbs (o-questions setup status|list|plan|install|uninstall):
// a managed block (ochazuke's block kind) holding the "Questions for the
// owner" note in the agents' instruction files. Both items share one marker
// pair, so two paths that resolve to one file hold one note. Follow manages
// symlink targets while keeping the links intact.
//
//go:embed setup.json
var setupItems []byte

func (a *App) declareSetup() error {
	items, err := setup.Decode(bytes.NewReader(setupItems))
	if err != nil {
		return err
	}
	opts := setup.Options{Tool: "o-questions", Data: a.store.Dir}
	skip, err := setupSkipDefaults(items, opts)
	if err != nil {
		return err
	}
	if err := a.r.DeclareSetting(ochazuke.Setting{Name: ocli.SetupSkipSetting, Type: ochazuke.SettingList, Default: skip, Section: "setup", Description: "Setup item ids to skip; defaults omit earlier identical blocks in the same resolved directory"}); err != nil {
		return err
	}
	return ocli.DeclareSetup(a.r, items, opts)
}

// Co-located instruction files commonly import each other. Prefer the last
// declaration of an identical block there, without parsing harness-specific
// import syntax. This is only a default: an explicit setup.skip overrides it.
func setupSkipDefaults(items []setup.Item, opts setup.Options) ([]string, error) {
	var blocks []setup.Item
	for _, item := range items {
		if item.Kind == setup.Block && item.Follow {
			blocks = append(blocks, item)
		}
	}
	engine, err := setup.New(blocks, opts)
	if err != nil {
		return nil, err
	}
	rows, err := engine.List(context.Background())
	if err != nil {
		return nil, err
	}
	type block struct{ dir, begin, end, content string }
	seen := map[block]bool{}
	skip := []string{}
	for n := len(rows) - 1; n >= 0; n-- {
		row := rows[n]
		path, err := filepath.EvalSymlinks(row.Path)
		if err != nil {
			continue
		}
		key := block{filepath.Dir(path), row.Begin, row.End, row.Content}
		if seen[key] {
			skip = append(skip, row.ID)
		}
		seen[key] = true
	}
	return skip, nil
}
