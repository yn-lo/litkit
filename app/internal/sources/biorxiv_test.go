package sources

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"litkit/internal/util/ratelimit"
)

// bioRxiv API 真实响应片段。
const biorxivSample = `{
  "collection": [
    {
      "doi": "10.1101/2024.01.15.123",
      "title": "CRISPR Gene Editing in Yeast",
      "authors": "Alice Smith;Bob Jones;Carol Lee",
      "date": "2024-01-15",
      "category": "genetics",
      "abstract": "We describe a new CRISPR method for yeast gene editing.",
      "published": "2024-01-15",
      "server": "biorxiv"
    },
    {
      "doi": "10.1101/2024.02.20.456",
      "title": "Protein Folding Networks",
      "authors": "Dave Wang",
      "date": "2024-02-20",
      "category": "biophysics",
      "abstract": "Protein folding simulations using graph neural networks.",
      "published": "2024-02-20",
      "server": "biorxiv"
    }
  ],
  "messages": [{"status": "ok", "count": 2, "total": 2}]
}`

func TestParseBiorxivJSON(t *testing.T) {
	papers, err := parseBiorxivJSON([]byte(biorxivSample), "biorxiv")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(papers) != 2 {
		t.Fatalf("应返回 2 篇，got %d", len(papers))
	}

	p1 := papers[0]
	if p1.DOI != "10.1101/2024.01.15.123" {
		t.Errorf("DOI：got %q", p1.DOI)
	}
	if p1.Title != "CRISPR Gene Editing in Yeast" {
		t.Errorf("Title：got %q", p1.Title)
	}
	if !strings.Contains(p1.Abstract, "CRISPR") {
		t.Errorf("Abstract 不应丢失关键词：got %q", p1.Abstract)
	}
	if p1.Year != 2024 {
		t.Errorf("Year：got %d want 2024", p1.Year)
	}
	if p1.Source != "biorxiv" {
		t.Errorf("Source：got %q want biorxiv", p1.Source)
	}
	if p1.DocType != "preprint" {
		t.Errorf("DocType 应为 preprint，got %q", p1.DocType)
	}
	if !strings.HasPrefix(p1.URL, "https://www.biorxiv.org/content/") {
		t.Errorf("URL 应指向 biorxiv.org，got %q", p1.URL)
	}
	if len(p1.Authors) != 3 {
		t.Fatalf("应有 3 位作者，got %d", len(p1.Authors))
	}
	if p1.Authors[0].Given != "Alice" || p1.Authors[0].Family != "Smith" {
		t.Errorf("第 1 位作者应拆分，got %+v", p1.Authors[0])
	}
}

func TestParseBiorxivJSON_medrxivServer(t *testing.T) {
	// 同样的 JSON，但传入 server="medrxiv"，应影响 Source 字段
	papers, err := parseBiorxivJSON([]byte(biorxivSample), "medrxiv")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if papers[0].Source != "medrxiv" {
		t.Errorf("Source 应为 medrxiv，got %q", papers[0].Source)
	}
	if !strings.Contains(papers[0].URL, "medrxiv.org") {
		t.Errorf("URL 应指向 medrxiv.org，got %q", papers[0].URL)
	}
}

func TestParseBiorxivJSON_invalidJSON(t *testing.T) {
	_, err := parseBiorxivJSON([]byte("not json"), "biorxiv")
	if err == nil {
		t.Fatal("非法 JSON 应报错")
	}
}

func TestBiorxivSource_Search_filtersByKeyword(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// 验证请求路径为 /details/{server}/{数字 interval}
		path := r.URL.Path
		if want := "/details/biorxiv/5"; !strings.HasSuffix(path, want) {
			t.Errorf("路径应以 %s 结尾（interval=MaxResults），got %q", want, path)
		}
		_, _ = w.Write([]byte(biorxivSample))
	}))
	defer srv.Close()

	src := NewBiorxivSource("biorxiv", newHTTPClient(2000, 1), ratelimit.New(100, 5))
	src.BaseURL = srv.URL

	// 关键词 "CRISPR" 应只命中第一篇
	papers, err := src.Search(context.Background(), "CRISPR", SearchOptions{MaxResults: 5})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(papers) != 1 {
		t.Fatalf("关键词 CRISPR 应过滤为 1 篇，got %d", len(papers))
	}
	if papers[0].Title != "CRISPR Gene Editing in Yeast" {
		t.Errorf("保留篇标题：got %q", papers[0].Title)
	}
}

func TestBiorxivSource_Search_yearFilter(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(biorxivSample))
	}))
	defer srv.Close()

	src := NewBiorxivSource("biorxiv", newHTTPClient(2000, 1), ratelimit.New(100, 5))
	src.BaseURL = srv.URL

	// year=2024 + 关键词 "Protein" 应命中第二篇
	papers, err := src.Search(context.Background(), "Protein", SearchOptions{MaxResults: 5, Year: 2024})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(papers) != 1 {
		t.Fatalf("应过滤为 1 篇，got %d", len(papers))
	}
	if papers[0].Year != 2024 {
		t.Errorf("保留篇应为 2024，got %d", papers[0].Year)
	}
}

func TestBiorxivSource_Search_medrxivName(t *testing.T) {
	src := NewBiorxivSource("medrxiv", newHTTPClient(2000, 1), ratelimit.New(100, 5))
	if src.Name() != "medrxiv" {
		t.Errorf("Name 应为 medrxiv，got %q", src.Name())
	}
}

func TestBiorxivSource_Search_zeroMaxResultsFallsBackTo100(t *testing.T) {
	// MaxResults=0 时 interval 回退 100（不是不截断）
	var path string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path = r.URL.Path
		_, _ = w.Write([]byte(biorxivSample))
	}))
	defer srv.Close()

	src := NewBiorxivSource("biorxiv", newHTTPClient(2000, 1), ratelimit.New(100, 5))
	src.BaseURL = srv.URL

	if _, err := src.Search(context.Background(), "", SearchOptions{}); err != nil {
		t.Fatalf("Search: %v", err)
	}
	if want := "/details/biorxiv/100"; !strings.HasSuffix(path, want) {
		t.Errorf("MaxResults=0 应回退 interval=100，got %q", path)
	}
}
