package sources

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"litkit/internal/util/ratelimit"
)

// OpenAlex 真实响应片段（关键字段；abstract 为倒排索引）。
const openAlexSample = `{
  "meta": {"count": 2},
  "results": [
    {
      "id": "https://openalex.org/W123456789",
      "doi": "https://doi.org/10.1000/xyz",
      "title": "Graph Neural Networks Survey",
      "abstract_inverted_index": {
        "Graph": [0],
        "neural": [1],
        "networks": [2],
        "survey": [3],
        "methods": [4]
      },
      "publication_year": 2023,
      "type": "article",
      "cited_by_count": 42,
      "authorships": [
        {"author": {"display_name": "Alice Smith"}},
        {"author": {"display_name": "Bob Jones"}}
      ],
      "primary_location": {
        "source": {"display_name": "Nature Machine Intelligence"}
      }
    },
    {
      "id": "https://openalex.org/W987654321",
      "doi": null,
      "title": "Untitled Preprint",
      "abstract_inverted_index": null,
      "publication_year": 2024,
      "type": "preprint",
      "cited_by_count": 0,
      "authorships": [],
      "primary_location": {"source": null}
    }
  ]
}`

func TestReconstructOpenAlexAbstract(t *testing.T) {
	idx := map[string][]int{
		"Graph":    {0},
		"neural":   {1},
		"networks": {2},
	}
	got := reconstructOpenAlexAbstract(idx)
	want := "Graph neural networks"
	if got != want {
		t.Errorf("reconstruct: got %q want %q", got, want)
	}
}

func TestReconstructOpenAlexAbstract_handlesMultiPositions(t *testing.T) {
	// 一个词出现多次
	idx := map[string][]int{
		"the": {0, 3},
		"cat": {1},
		"sat": {2},
		"mat": {4},
	}
	got := reconstructOpenAlexAbstract(idx)
	want := "the cat sat the mat"
	if got != want {
		t.Errorf("got %q want %q", got, want)
	}
}

func TestReconstructOpenAlexAbstract_empty(t *testing.T) {
	if reconstructOpenAlexAbstract(nil) != "" {
		t.Error("nil 应返回空串")
	}
	if reconstructOpenAlexAbstract(map[string][]int{}) != "" {
		t.Error("空 map 应返回空串")
	}
}

func TestParseOpenAlexJSON(t *testing.T) {
	papers, err := parseOpenAlexJSON([]byte(openAlexSample))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(papers) != 2 {
		t.Fatalf("应返回 2 篇，got %d", len(papers))
	}

	p1 := papers[0]
	if p1.DOI != "10.1000/xyz" {
		t.Errorf("DOI 应去 https://doi.org/ 前缀，got %q", p1.DOI)
	}
	if p1.Title != "Graph Neural Networks Survey" {
		t.Errorf("Title：got %q", p1.Title)
	}
	if p1.Abstract != "Graph neural networks survey methods" {
		t.Errorf("Abstract 应由倒排索引重建，got %q", p1.Abstract)
	}
	if p1.Year != 2023 {
		t.Errorf("Year：got %d", p1.Year)
	}
	if p1.Venue != "Nature Machine Intelligence" {
		t.Errorf("Venue：got %q", p1.Venue)
	}
	if p1.Source != "openalex" {
		t.Errorf("Source：got %q", p1.Source)
	}
	if p1.DocType != "article" {
		t.Errorf("DocType：got %q", p1.DocType)
	}
	if p1.Citations != 42 {
		t.Errorf("Citations：got %d", p1.Citations)
	}
	if len(p1.Authors) != 2 {
		t.Fatalf("应有 2 位作者，got %d", len(p1.Authors))
	}
	if p1.Authors[0].Given != "Alice" || p1.Authors[0].Family != "Smith" {
		t.Errorf("第 1 位作者应拆分 given/family，got %+v", p1.Authors[0])
	}
	if p1.URL == "" {
		t.Errorf("URL 不应为空")
	}

	// 第二篇无摘要（abstract_inverted_index=null）
	p2 := papers[1]
	if p2.Abstract != "" {
		t.Errorf("null 倒排索引应空串 abstract，got %q", p2.Abstract)
	}
	if p2.DOI != "" {
		t.Errorf("null DOI 应空串，got %q", p2.DOI)
	}
	if len(p2.Authors) != 0 {
		t.Errorf("空作者列表，got %d", len(p2.Authors))
	}
}

func TestParseOpenAlexJSON_invalidJSON(t *testing.T) {
	_, err := parseOpenAlexJSON([]byte("not json"))
	if err == nil {
		t.Fatal("非法 JSON 应报错")
	}
}

func TestOpenAlexSource_Search_endToEnd(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("search") != "graph neural" {
			t.Errorf("search 参数应传递，got %q", r.URL.Query().Get("search"))
		}
		if r.URL.Query().Get("per-page") != "5" {
			t.Errorf("per-page=5，got %q", r.URL.Query().Get("per-page"))
		}
		// year 过滤
		if r.URL.Query().Get("filter") != "publication_year:2023" {
			t.Errorf("year 应转 filter，got %q", r.URL.Query().Get("filter"))
		}
		_, _ = w.Write([]byte(openAlexSample))
	}))
	defer srv.Close()

	src := NewOpenAlexSource(newHTTPClient(2000, 1), ratelimit.New(100, 5))
	src.BaseURL = srv.URL + "/works"

	papers, err := src.Search(context.Background(), "graph neural", SearchOptions{MaxResults: 5, Year: 2023})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(papers) != 2 {
		t.Fatalf("应返回 2 篇，got %d", len(papers))
	}
}

func TestOpenAlexSource_Search_HTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer srv.Close()

	src := NewOpenAlexSource(newHTTPClient(1000, 0), ratelimit.New(100, 5))
	src.BaseURL = srv.URL + "/works"

	_, err := src.Search(context.Background(), "x", SearchOptions{MaxResults: 1})
	if err == nil || !strings.Contains(err.Error(), "HTTP") {
		t.Fatalf("HTTP 502 应返回含 HTTP 错误，got %v", err)
	}
}
