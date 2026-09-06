package sources

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"litkit/internal/util/ratelimit"
)

// Crossref /works 中文检索响应片段（含中文标题与 JATS 中文摘要）。
const crossrefSample = `{
  "message": {
    "items": [
      {
        "title": ["糖尿病的中医药治疗"],
        "abstract": "<jats:p>目的：评价中医药治疗糖尿病的临床疗效。</jats:p>",
        "author": [{"given":" ", "family":"张伟"}],
        "issued": {"date-parts": [[2023, 5, 1]]},
        "container-title": ["中医药学报"],
        "DOI": "10.69979/3029-2808.25.03.046",
        "type": "journal-article",
        "volume": "25",
        "issue": "3",
        "page": "10-15",
        "URL": "https://doi.org/10.69979/3029-2808.25.03.046"
      },
      {
        "title": ["Diabetes treatment"],
        "abstract": "",
        "author": [],
        "issued": {"date-parts": [[2020, 1, 1]]},
        "container-title": [],
        "DOI": "",
        "type": "",
        "URL": ""
      }
    ]
  }
}`

func TestParseCrossrefSearch(t *testing.T) {
	papers, err := parseCrossrefSearch([]byte(crossrefSample))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(papers) != 2 {
		t.Fatalf("应返回 2 篇，got %d", len(papers))
	}

	p1 := papers[0]
	if p1.Title != "糖尿病的中医药治疗" {
		t.Errorf("中文标题：got %q", p1.Title)
	}
	if p1.Abstract != "目的：评价中医药治疗糖尿病的临床疗效。" {
		t.Errorf("JATS 标签应被去除，got %q", p1.Abstract)
	}
	if p1.DOI != "10.69979/3029-2808.25.03.046" {
		t.Errorf("DOI：got %q", p1.DOI)
	}
	if p1.Year != 2023 {
		t.Errorf("Year：got %d", p1.Year)
	}
	if p1.Venue != "中医药学报" {
		t.Errorf("Venue：got %q", p1.Venue)
	}
	if p1.Source != "crossref" {
		t.Errorf("Source：got %q", p1.Source)
	}
	if p1.DocType != "journal-article" {
		t.Errorf("DocType：got %q", p1.DocType)
	}
	if len(p1.Authors) != 1 || p1.Authors[0].Family != "张伟" {
		t.Errorf("作者应入 Family，got %+v", p1.Authors)
	}
	if p1.URL == "" {
		t.Errorf("有 DOI 应填 URL")
	}

	// 第二篇无摘要/无 DOI
	p2 := papers[1]
	if p2.Abstract != "" || p2.DOI != "" || len(p2.Authors) != 0 {
		t.Errorf("空字段应保留空值：%+v", p2)
	}
}

func TestParseCrossrefSearch_invalidJSON(t *testing.T) {
	if _, err := parseCrossrefSearch([]byte("not json")); err == nil {
		t.Fatal("非法 JSON 应报错")
	}
}

func TestCrossrefCrossrefYearFilter(t *testing.T) {
	if got := crossrefYearFilter(2020, 2020); got != "from-pub-date:2020-01-01,until-pub-date:2020-12-31" {
		t.Errorf("精确年份：got %q", got)
	}
	if got := crossrefYearFilter(2020, 0); got != "from-pub-date:2020-01-01" {
		t.Errorf("自 since 起：got %q", got)
	}
}

func TestCrossrefSource_Search_endToEnd(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("query") != "糖尿病 中医药" {
			t.Errorf("中文 query 应传递，got %q", r.URL.Query().Get("query"))
		}
		if r.URL.Query().Get("rows") != "3" {
			t.Errorf("rows=3，got %q", r.URL.Query().Get("rows"))
		}
		if got := r.URL.Query().Get("filter"); got != "from-pub-date:2020-01-01,until-pub-date:2020-12-31" {
			t.Errorf("Year 应转 filter，got %q", got)
		}
		if ua := r.Header.Get("User-Agent"); !strings.Contains(ua, "mailto:") {
			t.Errorf("Crossref polite pool 需要 mailto UA，got %q", ua)
		}
		_, _ = w.Write([]byte(crossrefSample))
	}))
	defer srv.Close()

	src := NewCrossrefSource(newHTTPClient(2000, 1), ratelimit.New(100, 5))
	src.BaseURL = srv.URL + "/works"

	papers, err := src.Search(context.Background(), "糖尿病 中医药", SearchOptions{MaxResults: 3, Year: 2020})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(papers) != 2 {
		t.Fatalf("应返回 2 篇，got %d", len(papers))
	}
}

func TestCrossrefSource_Search_HTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer srv.Close()

	src := NewCrossrefSource(newHTTPClient(1000, 0), ratelimit.New(100, 5))
	src.BaseURL = srv.URL + "/works"

	_, err := src.Search(context.Background(), "x", SearchOptions{MaxResults: 1})
	if err == nil || !strings.Contains(err.Error(), "HTTP") {
		t.Fatalf("502 应返回含 HTTP 错误，got %v", err)
	}
}
