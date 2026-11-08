package main

import (
	"bytes"
	"encoding/json"
	"sort"
	"strings"
	"testing"
)

func testApp(t *testing.T, s *Store) (*App, *bytes.Buffer) {
	t.Helper()
	var out bytes.Buffer
	a, e := newApp(s, strings.NewReader(""), &out, &out)
	if e != nil {
		t.Fatal(e)
	}
	return a, &out
}

// Every verb the catalog declares must be reachable from the CLI: a
// hand-written command that owns a noun's name hides the noun's group
// (review list was advertised and answered "subcommand not found").
func TestEveryCatalogVerbRuns(t *testing.T) {
	s := fixture(t)
	a, _ := testApp(t, s)
	var keys []string
	for _, op := range a.r.Catalog() {
		keys = append(keys, op.Noun()+" "+op.Name())
	}
	sort.Strings(keys)
	if len(keys) == 0 {
		t.Fatal("empty catalog")
	}
	for _, key := range keys {
		words := append(strings.Fields(key), "--help")
		t.Run(key, func(t *testing.T) {
			a, out := testApp(t, fixture(t))
			if e := dispatch(a, words); e != nil && strings.Contains(e.Error(), "not found") {
				t.Fatalf("%v: %v\n%s", words, e, out)
			}
			if strings.Contains(out.String(), "not found") {
				t.Fatalf("%v: %s", words, out)
			}
		})
	}
}

func TestReviewListJSON(t *testing.T) {
	s := fixture(t)
	s.ReviewAsk = true
	if _, e := agentStore(s).Pose(Pose{Question: "Ship the Review page?", Group: "Review", Recommendation: "Yes", Key: "k1"}); e != nil {
		t.Fatal(e)
	}
	a, out := testApp(t, s)
	if e := dispatch(a, []string{"review", "list", "--json", "{}"}); e != nil {
		t.Fatalf("%v: %s", e, out)
	}
	var rows []map[string]any
	if e := json.Unmarshal(out.Bytes(), &rows); e != nil {
		t.Fatalf("%v: %s", e, out)
	}
	questions := 0 // the tree has a topic row above each question
	for _, r := range rows {
		if id, _ := r["id"].(string); strings.HasPrefix(id, "question/") && r["label"] == "Ship the Review page?" {
			questions++
		}
	}
	if questions != 1 {
		t.Fatalf("question rows = %d: %s", questions, out)
	}
}

// Bare `review` and its flags still reach the review page: the words are
// read as the noun's default verb ((the review noun's default action)), and verbs and help are not.
func TestReviewBareReachesPage(t *testing.T) {
	a, out := testApp(t, fixture(t))
	if e := dispatch(a, []string{"review", "--smoke", "--width", "100", "--height", "20"}); e != nil {
		t.Fatalf("%v: %s", e, out)
	}
	if !strings.Contains(out.String(), "For you") {
		t.Fatal(out)
	}
}

// A host that mounts the Review page refuses a title another of its pages
// has (mtm has its own Review tab), and review.page says it opens that page
// so the host opens it instead of running the verb as a background job.
func TestReviewPageIsNamedAndDeclaredToHosts(t *testing.T) {
	a, out := testApp(t, fixture(t))
	if e := dispatch(a, []string{"catalog"}); e != nil {
		t.Fatalf("%v: %s", e, out)
	}
	data := out.Bytes()
	var doc struct {
		Pages []struct{ ID, Title string }
		Nouns []struct {
			Name  string
			Verbs []struct {
				Name      string
				OpensPage string
			}
		}
	}
	if e := json.Unmarshal(data, &doc); e != nil {
		t.Fatalf("%v: %s", e, data)
	}
	titles := map[string]string{}
	for _, p := range doc.Pages {
		titles[p.ID] = p.Title
	}
	if titles["review"] == "" || titles["review"] == "Review" {
		t.Fatalf("review page title %q must name what it holds, not collide with a host's Review tab: %v", titles["review"], titles)
	}
	opens := ""
	for _, n := range doc.Nouns {
		for _, v := range n.Verbs {
			if n.Name == "review" && v.Name == "page" {
				opens = v.OpensPage
			}
		}
	}
	if opens != "review" {
		t.Fatalf("review.page opensPage = %q", opens)
	}
}

func TestOpenWithoutLinksSaysSo(t *testing.T) {
	s := fixture(t)
	q := ask(t, s, "No links here?")
	a, out := testApp(t, s)
	e := dispatch(a, []string{"question", "open", q.ID})
	if e == nil || !strings.Contains(e.Error()+out.String(), "this question has no links") || strings.Contains(e.Error()+out.String(), "choose --link") {
		t.Fatalf("open with no links: %v %s", e, out)
	}
}
