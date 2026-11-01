// `o-questions review --web` reuses mgmt's browser review page unchanged:
// it writes an mgmt session (the version 1 shape mgmt.py build_session
// produces) from the open questions for the owner, runs `mgmt.py serve` on
// it, and copies each saved decision back into o-questions while the page is
// served and once more when it stops. o-questions stays the record; the
// session is a disposable view, rewritten on every run.
package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"
)

type mgmtItem struct {
	ID                string          `json:"id"`
	Title             string          `json:"title"`
	Source            string          `json:"source"`
	Details           string          `json:"details"`
	Group             string          `json:"group,omitempty"`
	Recommendation    string          `json:"recommendation,omitempty"`
	RecommendedAction string          `json:"recommended_action,omitempty"`
	Thread            []ThreadMessage `json:"thread,omitempty"`
	Action            string          `json:"action"`
	Note              string          `json:"note"`
	Status            string          `json:"status"`
	Result            string          `json:"result,omitempty"`
	ContextChanged    bool            `json:"context_changed,omitempty"`
	NewReply          bool            `json:"new_reply,omitempty"`
}
type mgmtSession struct {
	Version       int        `json:"version"`
	Title         string     `json:"title"`
	Scope         string     `json:"scope"`
	DefaultAction string     `json:"default_action"`
	Actions       []string   `json:"actions"`
	Kind          string     `json:"kind"`
	Ask           bool       `json:"ask"`
	Groups        []string   `json:"groups"`
	GeneratedAt   string     `json:"generated_at"`
	Items         []mgmtItem `json:"items"`
	CreatedAt     string     `json:"created_at"`
	UpdatedAt     string     `json:"updated_at"`
}

// webSession is the mgmt session for every question for the owner; applied
// ones are "done" (locked in the page).
func (s *Store) webSession() (mgmtSession, error) {
	qs, e := s.ownerQuestions()
	if e != nil {
		return mgmtSession{}, e
	}
	qs = sortByGroup(qs)
	defaults := s.reviewDefaults()
	stamp := s.now().Format(time.RFC3339)
	out := mgmtSession{Version: 1, Title: "Questions for the owner", Scope: "Questions agents posed in o-questions (" + s.Dir + "). Decisions are written back to o-questions while this page is served.", DefaultAction: defaults[0], Actions: slices.Clone(defaults), Kind: "o-questions", Ask: s.ReviewAsk, Groups: groupOrder(qs), GeneratedAt: stamp, CreatedAt: stamp, UpdatedAt: stamp, Items: []mgmtItem{}}
	for _, q := range qs {
		r := q.Review
		for _, act := range r.Actions {
			if !slices.Contains(out.Actions, act) {
				out.Actions = append(out.Actions, act)
			}
		}
		details := strings.TrimSpace(q.Text)
		if strings.TrimSpace(r.Details) != "" {
			details += "\n\n" + strings.TrimSpace(r.Details)
		}
		it := mgmtItem{ID: q.ID, Title: q.Title, Source: r.Source, Details: details, Group: groupOf(q), Recommendation: strings.TrimSpace(r.Recommendation), Action: r.Action, Note: r.Note, Status: "pending", ContextChanged: r.ContextChanged, NewReply: r.NewReply}
		if it.Source == "" {
			it.Source = "o-questions:" + q.ID
		}
		if it.Recommendation != "" || r.RecommendedAction != "" {
			it.RecommendedAction = recommended(r, defaults)
		}
		if t := reviewThread(q); len(t) > 0 {
			it.Thread = t
		}
		switch r.State {
		case reviewApplied:
			it.Status, it.Result = "done", r.Result
		case reviewFailed:
			it.Status, it.Result = "failed", r.Result
		}
		out.Items = append(out.Items, it)
	}
	return out, nil
}

// syncWeb copies the page's saved decisions into o-questions: each item
// whose action or note differs from the store becomes the owner's decision.
func (s *Store) syncWeb(path string) (int, error) {
	b, e := os.ReadFile(path)
	if e != nil {
		return 0, e
	}
	var sess mgmtSession
	if e = json.Unmarshal(b, &sess); e != nil {
		return 0, e
	}
	qs, e := s.ownerQuestions()
	if e != nil {
		return 0, e
	}
	byID := map[string]Question{}
	for _, q := range qs {
		byID[q.ID] = q
	}
	changed := 0
	var errs []error
	for _, it := range sess.Items {
		q, ok := byID[it.ID]
		if !ok || it.Status == "done" || q.Review.State == reviewApplied {
			continue
		}
		if it.Action == q.Review.Action && it.Note == q.Review.Note {
			continue
		}
		note := it.Note
		d := Decision{Action: it.Action, Note: &note, Clear: it.Action == ""}
		if _, e := s.Decide(q.ID, d); e != nil {
			errs = append(errs, fmt.Errorf("%s: %w", q.Title, e))
			continue
		}
		changed++
	}
	return changed, errors.Join(errs...)
}

func stateDir() string {
	if d := os.Getenv("XDG_STATE_HOME"); d != "" {
		return d
	}
	h, _ := os.UserHomeDir()
	return filepath.Join(h, ".local", "state")
}

func (a *App) reviewCommand(args []string) error {
	web, rest := false, []string{}
	var serve []string
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--web":
			web = true
		case "--no-open":
			serve = append(serve, args[i])
		case "--port":
			if i+1 >= len(args) {
				return errors.New("--port needs a number")
			}
			serve = append(serve, args[i], args[i+1])
			i++
		default:
			rest = append(rest, args[i])
		}
	}
	if !web {
		if len(serve) > 0 {
			return errors.New("--port and --no-open need --web")
		}
		return runPage(a, "review", rest, reviewSpec)
	}
	if len(rest) > 0 {
		return fmt.Errorf("unexpected review arguments %v", rest)
	}
	return a.reviewWeb(serve)
}

func resolveMgmt(setting string) (string, error) {
	if setting == "" {
		if path, err := exec.LookPath("mgmt.py"); err == nil {
			return path, nil
		}
		path := expandHome("~/hub/ai/utils/mgmt/mgmt.py")
		if info, err := os.Stat(path); err == nil && info.Mode().IsRegular() {
			return path, nil
		}
		return "", errors.New("mgmt.py not found: install mgmt/ from https://github.com/unixsuperhero/ai-utils and add mgmt.py to PATH, or set review.mgmt to the script's path")
	}
	path := expandHome(setting)
	info, err := os.Stat(path)
	if err != nil {
		return "", fmt.Errorf("review.mgmt: %w", err)
	}
	if !info.Mode().IsRegular() {
		return "", fmt.Errorf("review.mgmt: %q is not a regular file", path)
	}
	return path, nil
}

func (a *App) reviewWeb(serve []string) error {
	a.r.EnsureConfigSettings()
	a.configure()
	mgmt, e := resolveMgmt(a.r.Settings().String("review.mgmt"))
	if e != nil {
		return e
	}
	sess, e := a.store.webSession()
	if e != nil {
		return e
	}
	if len(sess.Items) == 0 {
		_, e = fmt.Fprintln(a.out, "No questions for the owner.")
		return e
	}
	dir := filepath.Join(stateDir(), "o-questions", "review-web")
	path := filepath.Join(dir, "session.json")
	if e = os.MkdirAll(dir, 0700); e != nil {
		return e
	}
	if e = atomicJSON(path, sess); e != nil {
		return e
	}
	cmd := exec.Command("python3", append([]string{mgmt, "serve", path}, serve...)...)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = a.in, a.out, a.errOut
	done := make(chan struct{})
	var wg sync.WaitGroup
	var mu sync.Mutex
	pull := func() {
		mu.Lock()
		defer mu.Unlock()
		if n, e := a.store.syncWeb(path); e != nil {
			fmt.Fprintln(a.errOut, "o-questions:", e)
		} else if n > 0 {
			fmt.Fprintf(a.errOut, "o-questions: saved %d decision(s)\n", n)
		}
	}
	wg.Add(1)
	go func() {
		defer wg.Done()
		var last time.Time
		tick := time.NewTicker(500 * time.Millisecond)
		defer tick.Stop()
		for {
			select {
			case <-done:
				return
			case <-tick.C:
				if info, e := os.Stat(path); e == nil && info.ModTime() != last {
					last = info.ModTime()
					pull()
				}
			}
		}
	}()
	// Ctrl-C stops mgmt's server; this process stays to save the last decisions.
	signal.Ignore(os.Interrupt)
	defer signal.Reset(os.Interrupt)
	runErr := a.run(cmd)
	close(done)
	wg.Wait()
	pull()
	return runErr
}
