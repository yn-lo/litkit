package sources

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"litkit/internal/util/ratelimit"
)

const semanticScholarSample = `{
  "total": 2,
  "data": [
    {
      "paperId": "abc123def456",
      "title": "Attention Is All You Need",
      "abstract": "We propose a new architecture called Transformer.",
      "year": 2017,
      "venue": "NeurIPS",
      "externalIds": {
        "DOI": "10.1000/xyz",
        "PubMed": "12345678",
        "ArXiv": "1706.03762",
        "CorpusId": 1372
      },
      "authors": [
        {"name": "Ashish Vaswani"},
        {"name": "Noam Shazeer"}
      ],
      "citationCount": 90000
    },
    {
      "paperId": "xyz789",
      "title": "Untitled",
      "abstract": null,
      "year": 2024,
      "venue": "",
      "externalIds": {},
      "authors": [],
      "citationCount": 0
    }
  ]
}`

func TestParseSemanticScholarJSON(t *testing.T) {
	papers, err := parseSemanticScholarJSON([]byte(semanticScholarSample))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(papers) != 2 {
		t.Fatalf("应返回 2 篇，got %d", len(papers))
	}

	p1 := papers[0]
	if p1.Title != "Attention Is All You Need" {
		t.Errorf("Title：got %q", p1.Title)
	}
	if !strings.Contains(p1.Abstract, "Transformer") {
		t.Errorf("Abstract：got %q", p1.Abstract)
	}
	if p1.DOI != "10.1000/xyz" {
		t.Errorf("DOI：got %q", p1.DOI)
	}
	if p1.PMID != "12345678" {
		t.Errorf("PMID：got %q", p1.PMID)
	}
	if p1.ArXivID != "1706.03762" {
		t.Errorf("ArXivID：got %q", p1.ArXivID)
	}
	if p1.Year != 2017 {
		t.Errorf("Year：got %d", p1.Year)
	}
	if p1.Venue != "NeurIPS" {
		t.Errorf("Venue：got %q", p1.Venue)
	}
	if p1.Source != "semantic_scholar" {
		t.Errorf("Source：got %q", p1.Source)
	}
	if p1.Citations != 90000 {
		t.Errorf("Citations：got %d", p1.Citations)
	}
	if len(p1.Authors) != 2 {
		t.Fatalf("应有 2 位作者，got %d", len(p1.Authors))
	}
	if p1.Authors[0].Given != "Ashish" || p1.Authors[0].Family != "Vaswani" {
		t.Errorf("第 1 位作者应拆分，got %+v", p1.Authors[0])
	}
	if !strings.HasPrefix(p1.ID, "sha256:") {
		t.Errorf("ID 应 sha256: 前缀，got %q", p1.ID)
	}

	// 第二篇无摘要（abstract=null）
	p2 := papers[1]
	if p2.Abstract != "" {
		t.Errorf("null abstract 应空串，got %q", p2.Abstract)
	}
	if len(p2.Authors) != 0 {
		t.Errorf("空作者列表，got %d", len(p2.Authors))
	}
}

func TestSemanticScholarSource_Search_endToEnd(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("query") != "transformer" {
			t.Errorf("query 参数应传递，got %q", r.URL.Query().Get("query"))
		}
		if r.URL.Query().Get("limit") != "5" {
			t.Errorf("limit=5，got %q", r.URL.Query().Get("limit"))
		}
		if !strings.Contains(r.URL.Query().Get("fields"), "title") {
			t.Errorf("fields 应含 title，got %q", r.URL.Query().Get("fields"))
		}
		_, _ = w.Write([]byte(semanticScholarSample))
	}))
	defer srv.Close()

	src := NewSemanticScholarSource("", newHTTPClient(2000, 1), ratelimit.New(100, 5))
	src.BaseURL = srv.URL + "/search"

	papers, err := src.Search(context.Background(), "transformer", SearchOptions{MaxResults: 5})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(papers) != 2 {
		t.Fatalf("应返回 2 篇，got %d", len(papers))
	}
}

func TestSemanticScholarSource_Search_yearFilter(t *testing.T) {
	var capturedYear string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedYear = r.URL.Query().Get("year")
		_, _ = w.Write([]byte(semanticScholarSample))
	}))
	defer srv.Close()

	src := NewSemanticScholarSource("", newHTTPClient(2000, 1), ratelimit.New(100, 5))
	src.BaseURL = srv.URL + "/search"

	_, err := src.Search(context.Background(), "x", SearchOptions{MaxResults: 1, Year: 2023})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if capturedYear != "2023-2023" {
		t.Errorf("year=2023 应转 2023-2023，got %q", capturedYear)
	}
}

func TestSemanticScholarSource_Search_sinceFilter(t *testing.T) {
	var capturedYear string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedYear = r.URL.Query().Get("year")
		_, _ = w.Write([]byte(semanticScholarSample))
	}))
	defer srv.Close()

	src := NewSemanticScholarSource("", newHTTPClient(2000, 1), ratelimit.New(100, 5))
	src.BaseURL = srv.URL + "/search"

	_, err := src.Search(context.Background(), "x", SearchOptions{MaxResults: 1, Since: 2023})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if capturedYear != "2023-" {
		t.Errorf("since=2023 应转开区间 2023-，got %q", capturedYear)
	}
}

func TestSemanticScholarSource_Search_403WithKeyRetriesAnonymous(t *testing.T) {
	// 403 应自动降级匿名（重试不带 key）
	var calls int32
	var sawAuthHeader []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		sawAuthHeader = append(sawAuthHeader, r.Header.Get("x-api-key"))
		if atomic.LoadInt32(&calls) == 1 {
			// 首次带 key 返回 403
			w.WriteHeader(http.StatusForbidden)
			return
		}
		// 第二次匿名请求返回结果
		_, _ = w.Write([]byte(semanticScholarSample))
	}))
	defer srv.Close()

	src := NewSemanticScholarSource("secret-key", newHTTPClient(2000, 0), ratelimit.New(100, 5))
	src.BaseURL = srv.URL + "/search"

	papers, err := src.Search(context.Background(), "x", SearchOptions{MaxResults: 5})
	if err != nil {
		t.Fatalf("403 应降级重试而非报错： %v", err)
	}
	if len(papers) != 2 {
		t.Fatalf("降级后应返回 2 篇，got %d", len(papers))
	}
	if atomic.LoadInt32(&calls) != 2 {
		t.Fatalf("应有 2 次调用（带 key + 匿名），got %d", calls)
	}
	if sawAuthHeader[0] != "secret-key" {
		t.Errorf("首次应带 key，got %q", sawAuthHeader[0])
	}
	if sawAuthHeader[1] != "" {
		t.Errorf("降级后应不带 key，got %q", sawAuthHeader[1])
	}
}

func TestSemanticScholarSource_Search_403WithoutKeyReturnsEmpty(t *testing.T) {
	// 无 key 时 403 应优雅返回空（NFR-REL-02），不报错中断
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer srv.Close()

	src := NewSemanticScholarSource("", newHTTPClient(1000, 0), ratelimit.New(100, 5))
	src.BaseURL = srv.URL + "/search"

	papers, err := src.Search(context.Background(), "x", SearchOptions{MaxResults: 5})
	if err != nil {
		t.Fatalf("无 key 403 应优雅返回空，不应报错： %v", err)
	}
	if len(papers) != 0 {
		t.Fatalf("应返回 0 篇，got %d", len(papers))
	}
}

func TestSemanticScholarSource_Search_429ReturnsError(t *testing.T) {
	// 非 403 错误状态码（429 限速）应返回 error 而非静默 nil
	// （回归：曾返回 nil 切片导致 JSON 输出 null，调用方无法感知失败）
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer srv.Close()

	src := NewSemanticScholarSource("", newHTTPClient(1000, 0), ratelimit.New(100, 5))
	src.BaseURL = srv.URL + "/search"

	_, err := src.Search(context.Background(), "x", SearchOptions{MaxResults: 5})
	if err == nil {
		t.Fatal("429 应返回 error，got nil")
	}
	if !strings.Contains(err.Error(), "HTTP 429") {
		t.Errorf("错误应含状态码 429，got %q", err.Error())
	}
}
