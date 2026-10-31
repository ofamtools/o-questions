// Plan "Owner questions" importer: a Go port of the parsing rules in
// ~/hub/ai/utils/mgmt/inventories/owner_questions.py (scan, split_item,
// group_name, item_title, decisions). Ids match the Python ("oq-" + 12 hex of
// sha256(rel, section, number)), so a question keeps its id whichever tool
// scanned it. Ported, not called, so agents import through the CLI or MCP
// without Python, and the rules are tested here on fixture plans.
package main

import (
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"
)

var (
	questionHead = regexp.MustCompile(`(?i)owner questions|open questions|questions for the owner|open owner decisions`)
	decisionHead = regexp.MustCompile(`(?i)owner (decisions|answers)|decision \(owner`)
	headingLine  = regexp.MustCompile(`^(#{1,6})\s+(.*?)\s*#*\s*$`)
	numberedLine = regexp.MustCompile(`^ {0,3}(\d+)[.)]\s+(.*)$`)
	bulletLine   = regexp.MustCompile(`^ {0,1}[-*]\s+(.*)$`)
	recMarker    = regexp.MustCompile(`(?i)\[?\s*(?:\*{1,2}|_)?\s*\b(Recommended|Recommend|Proposal|Proposed)\b\s*:?\s*`)
	itemPrefix   = regexp.MustCompile(`^ {0,3}(\d+[.)]|[-*])\s+`)
	boldLead     = regexp.MustCompile(`^\*\*(.+?)\*\*`)
	sentenceEnd  = regexp.MustCompile(`[?.]\s`)
	dashJoin     = regexp.MustCompile(`(^|[^0-9])-|-([^0-9]|$)`)
)

type heading struct {
	line, level int
	text        string
}

// PlanQuestion is one numbered (or bulleted) item of a plan's questions section.
type PlanQuestion struct {
	ID             string `json:"id"`
	Path           string `json:"path"`
	Rel            string `json:"rel"`
	Line           int    `json:"line"`
	Plan           string `json:"plan"`
	Section        string `json:"section"`
	Number         int    `json:"number"`
	Title          string `json:"title"`
	Question       string `json:"question"`
	Recommendation string `json:"recommendation,omitempty"`
	Raw            string `json:"raw"`
	Answered       string `json:"answered,omitempty"`
	Unnumbered     []int  `json:"unnumbered_decisions,omitempty"`
}

func planHeadings(lines []string) []heading {
	var out []heading
	fence := false
	for n, line := range lines {
		if strings.HasPrefix(strings.TrimLeft(line, " \t"), "```") {
			fence = !fence
		}
		if fence {
			continue
		}
		if m := headingLine.FindStringSubmatch(line); m != nil {
			out = append(out, heading{n, len(m[1]), m[2]})
		}
	}
	return out
}

func sectionEnd(lines []string, heads []heading, index int) int {
	for _, h := range heads[index+1:] {
		if h.level <= heads[index].level {
			return h.line
		}
	}
	return len(lines)
}

type listItem struct {
	number, first int
	text          string
}

func listItems(lines []string, start, end int, pattern *regexp.Regexp) []listItem {
	var items []listItem
	for n := start; n < end; {
		m := pattern.FindStringSubmatch(lines[n])
		if m == nil || headingLine.MatchString(lines[n]) {
			n++
			continue
		}
		first, body := n, []string{lines[n]}
		n++
		for n < end {
			line := lines[n]
			if headingLine.MatchString(line) || pattern.MatchString(line) {
				break
			}
			if strings.TrimSpace(line) == "" {
				ahead := ""
				for _, l := range lines[n+1 : end] {
					if strings.TrimSpace(l) != "" {
						ahead = l
						break
					}
				}
				if !strings.HasPrefix(ahead, " ") && !strings.HasPrefix(ahead, "\t") {
					break
				}
			}
			body = append(body, line)
			n++
		}
		number := len(items) + 1
		if pattern == numberedLine {
			number, _ = strconv.Atoi(m[1])
		}
		items = append(items, listItem{number, first, strings.TrimSpace(strings.Join(body, "\n"))})
	}
	return items
}

// stripEmphasis drops lone asterisks outside code spans (Python's LONE_STAR,
// which Go's regexp cannot express with lookarounds).
func stripEmphasis(text string) string {
	parts := strings.Split(text, "`")
	for i, p := range parts {
		if i%2 == 1 {
			continue
		}
		r := []rune(p)
		var b strings.Builder
		for j, c := range r {
			if c == '*' && (j == 0 || r[j-1] != '*') && (j == len(r)-1 || r[j+1] != '*') {
				continue
			}
			b.WriteRune(c)
		}
		parts[i] = b.String()
	}
	return strings.Join(parts, "`")
}

// splitItem returns (title, question, recommendation) from a list item's text.
func splitItem(raw string) (string, string, string) {
	text := raw
	if loc := itemPrefix.FindStringIndex(raw); loc != nil {
		text = raw[loc[1]:]
	}
	var words []string
	for _, l := range strings.Split(text, "\n") {
		words = append(words, strings.TrimSpace(l))
	}
	flat := strings.Join(words, " ")
	question, rec := flat, ""
	if loc := recMarker.FindStringIndex(flat); loc != nil {
		question, rec = flat[:loc[0]], flat[loc[1]:]
	}
	question = strings.TrimSpace(strings.TrimRight(strings.TrimSpace(stripEmphasis(question)), "["))
	rec = strings.TrimSpace(strings.TrimRight(strings.TrimSpace(stripEmphasis(rec)), "]"))
	title := question
	if m := boldLead.FindStringSubmatch(question); m != nil {
		title = strings.TrimSpace(m[1])
	} else if loc := sentenceEnd.FindStringIndex(question); loc != nil {
		title = question[:loc[0]+1]
	}
	title = strings.ReplaceAll(title, "**", "")
	if r := []rune(title); len(r) > 160 {
		title = strings.TrimRight(string(r[:157]), " \t") + "..."
	}
	if title == "" {
		r := []rune(question)
		title = string(r[:min(len(r), 160)])
	}
	return title, question, rec
}

// planGroup is a plan's short name: plans/ochazuke-settings-layout.md is
// "Settings layout".
func planGroup(rel string) string {
	stem := strings.TrimSuffix(filepath.Base(rel), filepath.Ext(rel))
	if s := strings.TrimPrefix(stem, "ochazuke-"); s != "" {
		stem = s
	}
	name := strings.TrimSpace(dashJoin.ReplaceAllStringFunc(stem, func(m string) string { return strings.ReplaceAll(m, "-", " ") }))
	r := []rune(name)
	if len(r) == 0 {
		return name
	}
	return strings.ToUpper(string(r[:1])) + string(r[1:])
}

func planItemTitle(number int, question string) string {
	text := fmt.Sprintf("#%d %s", number, strings.ReplaceAll(question, "**", ""))
	if r := []rune(text); len(r) > 240 {
		return strings.TrimRight(string(r[:237]), " \t") + "..."
	}
	return text
}

// ScanPlans finds the questions in root/*.md and root/plans/*.md.
func ScanPlans(root string, includeAnswered bool) ([]PlanQuestion, error) {
	var files []string
	for _, pattern := range []string{"*.md", "plans/*.md"} {
		m, e := filepath.Glob(filepath.Join(root, pattern))
		if e != nil {
			return nil, e
		}
		files = append(files, m...)
	}
	sort.Strings(files)
	files = slices.Compact(files)
	var out []PlanQuestion
	for _, path := range files {
		b, e := os.ReadFile(path)
		if e != nil {
			return nil, e
		}
		lines := strings.Split(strings.TrimSuffix(strings.ReplaceAll(string(b), "\r\n", "\n"), "\n"), "\n")
		heads := planHeadings(lines)
		rel, _ := filepath.Rel(root, path)
		rel = filepath.ToSlash(rel)
		plan := rel
		for _, h := range heads {
			if h.level == 1 {
				plan = h.text
				break
			}
		}
		covered := map[int]bool{}
		var unnumbered []int
		var sections []int
		for i, h := range heads {
			if questionHead.MatchString(h.text) {
				sections = append(sections, i)
			} else if decisionHead.MatchString(h.text) {
				nums := listItems(lines, h.line+1, sectionEnd(lines, heads, i), numberedLine)
				for _, it := range nums {
					covered[it.number] = true
				}
				if len(nums) == 0 {
					unnumbered = append(unnumbered, h.line+1)
				}
			}
		}
		for _, i := range sections {
			h := heads[i]
			end := sectionEnd(lines, heads, i)
			var answers *heading
			for _, in := range heads {
				if in.line > h.line && in.line < end && decisionHead.MatchString(in.text) {
					answers = &in
					break
				}
			}
			stop := end
			if answers != nil {
				stop = answers.line
			}
			found := listItems(lines, h.line+1, stop, numberedLine)
			if len(found) == 0 {
				found = listItems(lines, h.line+1, stop, bulletLine)
			}
			for _, it := range found {
				title, question, rec := splitItem(it.text)
				answered := ""
				switch {
				case strings.Contains(strings.ToLower(h.text), "answered"):
					answered = "the section heading says answered"
				case answers != nil:
					answered = fmt.Sprintf("owner answers at line %d", answers.line+1)
				case covered[it.number]:
					answered = fmt.Sprintf("owner decision %d in this plan", it.number)
				}
				if answered != "" && !includeAnswered {
					continue
				}
				sum := sha256.Sum256([]byte(fmt.Sprintf("%s\x00%s\x00%d", rel, h.text, it.number)))
				out = append(out, PlanQuestion{ID: fmt.Sprintf("oq-%x", sum)[:15], Path: path, Rel: rel, Line: it.first + 1, Plan: plan, Section: h.text, Number: it.number, Title: title, Question: question, Recommendation: rec, Raw: it.text, Answered: answered, Unnumbered: unnumbered})
			}
		}
	}
	return out, nil
}

// pose is the plan question as an agent's question for the owner.
func (q PlanQuestion) pose() Pose {
	details := []string{fmt.Sprintf("Plan: %s (%s)", q.Plan, q.Rel), fmt.Sprintf("Section: %s, question %d", q.Section, q.Number), "", q.Raw}
	if len(q.Unnumbered) > 0 {
		var ns []string
		for _, n := range q.Unnumbered {
			ns = append(ns, strconv.Itoa(n))
		}
		details = append(details, "", "Note: this plan has an unnumbered owner decision section (line "+strings.Join(ns, ", ")+"); check whether it already settles this question.")
	}
	if q.Answered != "" {
		details = append(details, "", "Already answered: "+q.Answered)
	}
	p := Pose{Question: q.Question, Title: planItemTitle(q.Number, q.Question), Group: planGroup(q.Rel), Source: fmt.Sprintf("%s:%d", q.Path, q.Line), Details: strings.Join(details, "\n"), Recommendation: q.Recommendation, Key: "plan:" + q.ID, Origin: "plan", Plan: q.Rel, Section: q.Section, Number: q.Number}
	if q.Recommendation != "" {
		p.RecommendedAction = "yes"
	}
	return p
}

// Imported counts what an import did. Stale lists plan questions posed
// earlier that the scan no longer finds (answered or moved); they are left
// as they are.
type Imported struct {
	Root      string   `json:"root"`
	Found     int      `json:"found"`
	Created   int      `json:"created"`
	Changed   int      `json:"changed"`
	Unchanged int      `json:"unchanged"`
	Stale     []string `json:"stale"`
}

func (s *Store) ImportPlans(root string, includeAnswered bool) (Imported, error) {
	root, e := filepath.Abs(expandHome(root))
	if e != nil {
		return Imported{}, e
	}
	out := Imported{Root: root, Stale: []string{}}
	qs, e := ScanPlans(root, includeAnswered)
	if e != nil {
		return out, e
	}
	out.Found = len(qs)
	seen := map[string]bool{}
	for _, q := range qs {
		p := q.pose()
		seen[p.Key] = true
		r, e := s.Pose(p)
		if e != nil {
			return out, fmt.Errorf("%s:%d: %w", q.Rel, q.Line, e)
		}
		switch {
		case !r.Existed:
			out.Created++
		case r.Changed:
			out.Changed++
		default:
			out.Unchanged++
		}
	}
	owned, e := s.ownerQuestions()
	if e != nil {
		return out, e
	}
	for _, q := range owned {
		if q.Review.Origin == "plan" && !seen[q.Key] && strings.HasPrefix(q.Review.Source, root+string(filepath.Separator)) && q.Review.State != reviewApplied {
			out.Stale = append(out.Stale, q.ID)
		}
	}
	return out, nil
}

func expandHome(p string) string {
	if p == "~" || strings.HasPrefix(p, "~/") {
		if h, e := os.UserHomeDir(); e == nil {
			return filepath.Join(h, strings.TrimPrefix(p, "~"))
		}
	}
	return p
}

// PlanDecisions renders, per plan, the "## Owner decisions (date)" section
// for decided plan questions (owner_questions.py decisions). It never edits a
// plan: the agent inserts each block above the named section.
func (s *Store) PlanDecisions(date string) (string, error) {
	if date == "" {
		date = s.now().Format(time.DateOnly)
	}
	qs, e := s.ownerQuestions()
	if e != nil {
		return "", e
	}
	byPlan := map[string][]Question{}
	for _, q := range qs {
		r := q.Review
		if r.Origin != "plan" || r.Action == "" || (r.Action == askOut && s.ReviewAsk) || r.State == reviewApplied {
			continue
		}
		byPlan[r.Plan] = append(byPlan[r.Plan], q)
	}
	plans := make([]string, 0, len(byPlan))
	for p := range byPlan {
		plans = append(plans, p)
	}
	sort.Strings(plans)
	var b strings.Builder
	for _, plan := range plans {
		rows := byPlan[plan]
		slices.SortStableFunc(rows, func(a, b Question) int { return a.Review.Number - b.Review.Number })
		fmt.Fprintf(&b, "<!-- %s: insert above %q -->\n## Owner decisions (%s)\n\n", plan, rows[0].Review.Section, date)
		for _, q := range rows {
			r := q.Review
			note := strings.TrimSpace(r.Note)
			with := func(s string) string {
				if note != "" {
					return s + " " + note
				}
				return s
			}
			var text string
			switch r.Action {
			case "yes":
				text = r.Recommendation
				if text == "" {
					text = "Yes."
				}
				text = with(text)
			case "later":
				text = with("Later; still open.")
			case "moot":
				text = with("Moot; no longer needed.")
			case "no":
				text = "No, not the recommendation."
				if note != "" {
					text += " " + note
				} else {
					text += " (no reason given)"
				}
			default:
				text = with(r.Action + ".")
			}
			if rr := []rune(text); len(rr) > 0 {
				text = strings.ToUpper(string(rr[:1])) + string(rr[1:])
			}
			short, _, _ := splitItem(q.Text)
			fmt.Fprintf(&b, "%d. **%s.** %s\n", r.Number, strings.TrimRight(short, "?."), text)
		}
		b.WriteString("\n")
	}
	return b.String(), nil
}
