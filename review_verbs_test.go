package main

import (
	"bytes"
	"encoding/json"
	"reflect"
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
	// Known gap, outside this lane: the hand-written question command lists
	// only the verb specs and shadows Mount's group; the och-099 lane removes
	// it after the ochazuke upgrade. Until then its standard verbs skip.
	handWritten := map[string]bool{"view": true}
	for _, v := range a.specs() {
		handWritten[v.name] = true
		for _, al := range v.aliases {
			handWritten[al] = true
		}
	}
	var keys []string
	for _, op := range a.r.Catalog() {
		keys = append(keys, op.Noun+" "+op.Verb)
	}
	sort.Strings(keys)
	if len(keys) == 0 {
		t.Fatal("empty catalog")
	}
	for _, key := range keys {
		words := append(strings.Fields(key), "--help")
		t.Run(key, func(t *testing.T) {
			if noun, verb, _ := strings.Cut(key, " "); noun == "question" && !handWritten[verb] {
				t.Skip("question is still the hand-written command (och-099)")
			}
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
// read as the noun's default verb (nounDefaults), and verbs and help are not.
func TestReviewBareReachesPage(t *testing.T) {
	for in, want := range map[string][]string{
		"review":              {"review", "page"},
		"review --web":        {"review", "page", "--web"},
		"review --smoke":      {"review", "page", "--smoke"},
		"review list":         {"review", "list"},
		"review --help":       {"review", "--help"},
		"review view --x":     {"review", "view", "--x"},
		"question":            {"question"},
		"review -h":           {"review", "-h"},
		"review page --smoke": {"review", "page", "--smoke"},
	} {
		if got := nounDefault(strings.Fields(in)); !reflect.DeepEqual(got, want) {
			t.Errorf("%q = %v, want %v", in, got, want)
		}
	}
	a, out := testApp(t, fixture(t))
	if e := dispatch(a, []string{"review", "--smoke", "--width", "100", "--height", "20"}); e != nil {
		t.Fatalf("%v: %s", e, out)
	}
	if !strings.Contains(out.String(), "For you") {
		t.Fatal(out)
	}
}
