package sources

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"litkit/internal/util/ratelimit"
)

// DOAJ 中文检索响应片段（含中文标题与中文摘要，journal.language=ZH）。
const doajSample = `{
  "results": [
    {
      "bibjson": {
        "title": "糖尿病肾病的中西医结合治疗进展",
        "abstract": "目的  探讨中西医结合治疗糖尿病肾病的效果。\n结论  疗效显著。",
        "year": "2021",
        "author": [
          {"name": "GUO Ru (郭茹)"},
          {"name": "张琳"}
        ],
        "journal": {
          "title": "Zhongshan Daxue xuebao. Yixue kexue ban",
          "volume": "45",
          "number": "2"
        },
        "start_page": "446",
        "end_page": "456",
        "identifier": [
          {"id": "2709-1961", "type": "pissn"},
          {"id": "10.55111/j.issn2709-1961.202012073", "type": "doi"}
        ],
        "link": [
          {"type": "fulltext", "url": "http://xuebaoyx.sysu.edu.cn/zh/article/doi/10.13471/x"}
        ]
      }
    },
    {
      "bibjson": {
        "title": "No abstract paper",
        "abstract": "",
        "year": 2020,
        "author": [],
        "identifier": [],
        "link": []
      }
    }
  ]
}`

func TestParseDoajJSON(t *testing.T) {
	papers, err := parseDoajJSON([]byte(doajSample))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(papers) != 2 {
		t.Fatalf("应返回 2 篇，got %d", len(papers))
	}

	p1 := papers[0]
	if p1.Title != "糖尿病肾病的中西医结合治疗进展" {
		t.Errorf("中文标题：got %q", p1.Title)
	}
	if p1.Abstract != "目的 探讨中西医结合治疗糖尿病肾病的效果。 结论 疗效显著。" {
		t.Errorf("摘要应折叠空白，got %q", p1.Abstract)
	}
	if p1.DOI != "10.55111/j.issn2709-1961.202012073" {
		t.Errorf("DOI 取 doi 类型标识，got %q", p1.DOI)
	}
	if p1.Year != 2021 {
		t.Errorf("Year：got %d", p1.Year)
	}
	if p1.Source != "doaj" {
		t.Errorf("Source：got %q", p1.Source)
	}
	if p1.DocType != "article" {
		t.Errorf("DocType：got %q", p1.DocType)
	}
	if p1.Pages != "446-456" {
		t.Errorf("Pages：got %q", p1.Pages)
	}
	if len(p1.Authors) != 2 {
		t.Fatalf("应有 2 位作者，got %d", len(p1.Authors))
	}
	if p1.Authors[1].Family != "张琳" {
		t.Errorf("纯中文名应整体入 Family，got %+v", p1.Authors[1])
	}
	if p1.URL == "" {
		t.Errorf("应取 fulltext link 作为 URL")
	}

	p2 := papers[1]
	if p2.Abstract != "" || p2.DOI != "" || len(p2.Authors) != 0 || p2.URL != "" {
		t.Errorf("空字段应保留空值：%+v", p2)
	}
}

func TestParseDoajJSON_invalidJSON(t *testing.T) {
	if _, err := parseDoajJSON([]byte("not json")); err == nil {
		t.Fatal("非法 JSON 应报错")
	}
}

func TestDoajYear(t *testing.T) {
	if got := doajYear("2021"); got != 2021 {
		t.Errorf("字符串年份：got %d", got)
	}
	if got := doajYear(2020); got != 2020 {
		t.Errorf("数字年份：got %d", got)
	}
	if got := doajYear(2019.0); got != 2019 {
		t.Errorf("float 年份：got %d", got)
	}
	if got := doajYear(""); got != 0 {
		t.Errorf("空串：got %d", got)
	}
	if got := doajYear(nil); got != 0 {
		t.Errorf("nil：got %d", got)
	}
	if got := doajYear("abc"); got != 0 {
		t.Errorf("非数字：got %d", got)
	}
}

func TestDoajSource_buildURL(t *testing.T) {
	d := NewDoajSource(nil, nil)
	got := d.buildURL("糖尿病", SearchOptions{MaxResults: 5})
	// query 在路径中，应 URL 编码中文
	if !strings.Contains(got, "/search/articles/") || !strings.Contains(got, "pageSize=5") {
		t.Fatalf("URL 结构：got %q", got)
	}
	if _, err := url.ParseQuery(strings.TrimPrefix(got[strings.LastIndex(got, "?")+1:], "")); err != nil {
		t.Fatalf("query 部分应合法：%v", err)
	}
}

func TestDoajSource_Search_endToEnd(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "糖尿病") && !strings.Contains(url.PathEscape("糖尿病"), r.URL.Path) {
			t.Errorf("中文 query 应在路径中，got %q", r.URL.Path)
		}
		if r.URL.Query().Get("pageSize") != "3" {
			t.Errorf("pageSize=3，got %q", r.URL.Query().Get("pageSize"))
		}
		_, _ = w.Write([]byte(doajSample))
	}))
	defer srv.Close()

	src := NewDoajSource(newHTTPClient(2000, 1), ratelimit.New(100, 5))
	src.BaseURL = srv.URL + "/api/v2"

	papers, err := src.Search(context.Background(), "糖尿病", SearchOptions{MaxResults: 3})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(papers) != 2 {
		t.Fatalf("应返回 2 篇，got %d", len(papers))
	}
}

func TestDoajSource_Search_HTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer srv.Close()

	src := NewDoajSource(newHTTPClient(1000, 0), ratelimit.New(100, 5))
	src.BaseURL = srv.URL + "/api/v2"

	_, err := src.Search(context.Background(), "x", SearchOptions{MaxResults: 1})
	if err == nil || !strings.Contains(err.Error(), "HTTP") {
		t.Fatalf("502 应返回含 HTTP 错误，got %v", err)
	}
}
