// Read-only modal and scroll binding rows adapted from o-cmds/script_view.go.
package main

import (
	"github.com/charmbracelet/x/ansi"
	"ofam.dev/ochazuke"
	"strings"
)

const threadViewerID = "question-thread"
const threadViewerTextID = threadViewerID + "/text"

type threadViewer struct {
	title, text string
	detail      *ochazuke.Detail
}

func (v *threadViewer) ModalTitle() string { return v.title }
func (v *threadViewer) ModalBlock(styles ochazuke.Styles, width, height int) ochazuke.Block {
	body, marker := v.detail.Window(threadAtWidth(v.text, width), 0, max(0, height-1))
	header := marker + strings.Repeat(" ", max(0, width-ansi.StringWidth(marker)))
	return ochazuke.Text(styles.Muted.Render(header) + "\n" + body)
}

// Hard wrapping retains spaces at wrap boundaries; word wrapping would alter
// shell indentation and spacing. The framework renders tabs as four spaces.
func threadAtWidth(text string, width int) string {
	if width <= 0 {
		return ""
	}
	return ansi.Hardwrap(strings.ReplaceAll(text, "\t", "    "), width, true)
}

func registerThreadViewer(r *ochazuke.Runtime) {
	b := r.Define(threadViewerID).Pin("normal").Mode("normal")
	for _, row := range []struct {
		key   string
		lines int
	}{{"j", 1}, {"k", -1}, {"ctrl+d", 8}, {"ctrl+u", -8}, {"g", -1_000_000_000}, {"G", 1_000_000_000}} {
		b.Bind(row.key, ochazuke.Do("ochazuke.detail.scroll", ochazuke.Arguments{"id": threadViewerTextID, "lines": row.lines}).FromPayload("count", "count").Captured())
	}
	b.Bind("q", ochazuke.Do("ochazuke.modal.pop")).Bind("esc", ochazuke.Do("ochazuke.modal.pop"))
}

func openThreadViewer(r *ochazuke.Runtime, title, source string) error {
	inst, e := r.OpenModal(threadViewerID, ochazuke.Payload{}, ochazuke.ReplaceOpen())
	if e != nil {
		return e
	}
	d, e := r.AddDetail(threadViewerTextID, nil, ochazuke.Parent(inst.ID))
	if e != nil {
		_ = r.Pop()
		return e
	}
	inst.Component = &threadViewer{title: title, text: source, detail: d}
	return nil
}
