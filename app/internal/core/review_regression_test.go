package core

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"litkit/internal/model"
)

func TestRound2_SearchConflictingDOIs(t *testing.T) {
	ps := []model.Paper{{Title: "Same title", DOI: "10.1/one", Abstract: "Evidence A"}, {Title: "Same title", DOI: "10.1/two", Abstract: "Evidence B"}}
	got := dedupPapers(ps)
	if len(got) != 2 {
		t.Fatalf("distinct DOIs collapsed: %+v", got)
	}
}

func TestRound2_ScoreCacheAfterAbstractUpdate(t *testing.T) {
	st := newTestStore(t)
	p := model.Paper{Title: "Study", DOI: "10.1/cache", Abstract: "Support"}
	key, _, err := st.UpsertPaper(p)
	if err != nil {
		t.Fatal(err)
	}
	scorer := &mockScorer{modelID: "mock", promptVersion: "v1", score: 0.9}
	e := &ScorerEngine{store: st, scorers: []Scorer{scorer}, config: DefaultScoringConfig(), promptVersion: "v1"}
	if _, err := e.Score(context.Background(), key, "sentence", "Claim", p.Abstract); err != nil {
		t.Fatal(err)
	}
	p.Abstract = "Contradict"
	if _, _, err := st.UpsertPaper(p); err != nil {
		t.Fatal(err)
	}
	scorer.score = 0.1
	got, err := e.Score(context.Background(), key, "sentence", "Claim", p.Abstract)
	if err != nil {
		t.Fatal(err)
	}
	if got.Cached || got.MeanScore != 0.1 {
		t.Fatalf("stale score after abstract changed: %+v", got)
	}
	again, err := e.Score(context.Background(), key, "sentence", "Claim", p.Abstract)
	if err != nil || !again.Cached || again.MeanScore != 0.1 {
		t.Fatalf("unchanged input did not reuse new cache: %+v, %v", again, err)
	}
}

func TestRound2_GroupedCitations(t *testing.T) {
	t.Run("scoring", func(t *testing.T) {
		got := ExtractCiteSentences("结论[@AAA,BBB]。")
		if len(got) != 2 {
			t.Fatalf("expected 2 refs, got %+v", got)
		}
		if got[0].SentenceHash != got[1].SentenceHash || got[0].Sentence != "结论[@AAA,BBB]。" {
			t.Fatalf("group must share original sentence and hash: %+v", got)
		}
	})
	t.Run("manuscript", func(t *testing.T) {
		s := newFakeStore()
		s.papers["AAA"] = mustPaper("AAA", "One", 2024, nil, "10.1/one")
		s.papers["BBB"] = mustPaper("BBB", "Two", 2024, nil, "10.1/two")
		got, err := ProcessManuscript(context.Background(), s, nil, "结论[@AAA,BBB]。", StyleGB7714)
		if err != nil {
			t.Fatal(err)
		}
		if len(got.Papers) != 2 || len(got.Unresolved) != 0 || got.Text != "结论[1-2]。" {
			t.Fatalf("group failed resolution: %+v", got)
		}
		prefixed := "引用[@title:One, Two]。"
		if expandCiteGroups(prefixed, StyleGB7714) != prefixed {
			t.Fatal("title identifier comma was split as citation group")
		}
	})
}

func TestRound2_MissingScoreMustFail(t *testing.T) {
	for _, content := range []string{`{}`, `{"rationale":"no score"}`, `{"score":null}`, `null`} {
		t.Run(content, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				_ = json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"message": map[string]any{"content": content}}}})
			}))
			defer srv.Close()
			s := NewLLMScorer(ModelConfig{ID: "mock", APIKey: "test-placeholder"}, srv.URL, "v1", 0, nil)
			v, rationale, err := s.Score(context.Background(), "Claim", "Abstract")
			if err == nil {
				t.Fatalf("missing score accepted as actual score=%v, rationale=%q", v, rationale)
			}
		})
	}
}

func TestRound2_BibTextUnbalancedBrace(t *testing.T) {
	got := FormatBibTeX(model.Paper{CiteKey: "AAA", Title: "Study {BRCA1", Abstract: "Study } result", DocType: model.DocTypeArticle})
	if !strings.Contains(got, `title = {Study \textbraceleft{}BRCA1}`) || !strings.Contains(got, `abstract = {Study \textbraceright{} result}`) {
		t.Fatalf("unescaped braces in BibTeX:\n%s", got)
	}
	if got := escapeBibText("Study {BRCA1} {unclosed"); got != `Study {BRCA1} \textbraceleft{}unclosed` {
		t.Fatalf("balanced case protection changed: %s", got)
	}
	depth := 0
	for _, c := range got {
		if c == '{' {
			depth++
		} else if c == '}' {
			depth--
		}
		if depth < 0 {
			t.Fatal("BibTeX contains unmatched closing brace")
		}
	}
	if depth != 0 {
		t.Fatal("BibTeX contains unmatched opening brace")
	}
}

func TestAPAAuthorsBoundary(t *testing.T) {
	for _, count := range []int{20, 21} {
		authors := make([]model.Author, count)
		for i := range authors {
			authors[i] = model.Author{Family: fmt.Sprintf("Author%02d", i+1), Given: "G"}
		}
		got := authorsAPA(authors)
		if !strings.Contains(got, fmt.Sprintf("Author%02d, G.", count)) {
			t.Fatalf("last author missing: %s", got)
		}
		if count == 20 && (strings.Contains(got, "…") || !strings.Contains(got, ", & Author20, G.")) {
			t.Fatalf("20 authors must all appear: %s", got)
		}
		if count == 21 && (strings.Contains(got, "Author20") || strings.Contains(got, "&") || !strings.Contains(got, ", … Author21, G.")) {
			t.Fatalf("21 authors must show first 19 and last: %s", got)
		}
	}
}

func TestRound2_APAReferenceAuthorsAndDate(t *testing.T) {
	p := model.Paper{Title: "Study", Authors: []model.Author{{Family: "Alpha", Given: "A"}, {Family: "Beta", Given: "B"}, {Family: "Gamma", Given: "C"}, {Family: "Delta", Given: "D"}}}
	got, err := FormatReference(p, StyleAPA, 0)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "Delta, D.") || strings.Contains(got, "et al.") || !strings.Contains(got, "(n.d.).") {
		t.Fatalf("APA7 four-author undated reference invalid: %s", got)
	}
}
