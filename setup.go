package main

import (
	"bytes"
	_ "embed"

	ocli "ofam.dev/ochazuke/cli"
	"ofam.dev/ochazuke/setup"
)

// setup.json declares o-questions' integrations as data for ochazuke's
// shared setup verbs (o-questions setup status|list|plan|install|uninstall):
// a managed block (ochazuke's block kind) holding the "Questions for the
// owner" note in the agents' instruction files. Both items share one marker
// pair, so two paths that are one file hold one note. Symlinked targets are
// reported external by ochazuke setup, with the note as the next step.
//
//go:embed setup.json
var setupItems []byte

func (a *App) declareSetup() error {
	items, err := setup.Decode(bytes.NewReader(setupItems))
	if err != nil {
		return err
	}
	return ocli.DeclareSetup(a.r, items, setup.Options{Tool: "o-questions", Data: a.store.Dir})
}
