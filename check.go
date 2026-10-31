package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"github.com/unixsuperhero/ochazuke"
	"os"
)

// Only transcript user-message text can establish a verbatim match. Substring
// matches against assistant/tool records would falsely certify paraphrases.
func (a *App) check(q Question) ochazuke.Result {
	if e := a.store.permit("check", q); e != nil {
		return result(nil, e)
	}
	if q.Source.Transcript == "" {
		return result(q, nil)
	}
	f, e := os.Open(q.Source.Transcript)
	if e != nil {
		return result(nil, e)
	}
	defer f.Close()
	scan := bufio.NewScanner(f)
	scan.Buffer(make([]byte, 65536), 16<<20)
	matched := false
	users := 0
	for scan.Scan() {
		var row struct {
			Type    string `json:"type"`
			Message struct {
				Role    string          `json:"role"`
				Content json.RawMessage `json:"content"`
			} `json:"message"`
		}
		if json.Unmarshal(scan.Bytes(), &row) != nil {
			continue
		}
		if row.Type != "user" || row.Message.Role != "user" {
			continue
		}
		var text string
		if json.Unmarshal(row.Message.Content, &text) != nil {
			var blocks []struct{ Type, Text string }
			if json.Unmarshal(row.Message.Content, &blocks) != nil {
				continue
			}
			for _, b := range blocks {
				if b.Type == "text" {
					text += b.Text
				}
			}
		}
		users++
		matched = matched || text == q.Text
	}
	if e = scan.Err(); e != nil {
		return result(nil, e)
	}
	if users == 0 {
		return result(nil, fmt.Errorf("no supported user records in transcript"))
	}
	v, e := a.store.mutate("check", q.ID, func(q *Question, _ *[]Question) error {
		q.Verbatim = "differs"
		if matched {
			q.Verbatim = "checked"
		}
		return nil
	})
	a.refresh()
	return result(v, e)
}
