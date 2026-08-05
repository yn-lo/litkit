package sources

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"litkit/internal/model"
	"litkit/internal/util/ratelimit"
)

// 真实 arXiv Atom Feed 片段（字段命名与命名空间保持与生产一致）。
const arxivAtomSample = `<?xml version="1.0" encoding="UTF-8"?>
<feed xmlns="http://www.w3.org/2005/Atom" xmlns:arxiv="http://arxiv.org/schemas/atom">
  <entry>
    <id>http://arxiv.org/abs/2301.00001v1</id>
    <updated>2023-01-02T00:00:00Z</updated>
    <published>2023-01-01T00:00:00Z</published>
    <title>Graph Neural Networks: A Review of Methods and Applications</title>
    <summary>This paper reviews graph neural networks.</summary>
    <author><name>Zhou Jie</name></author>
    <author><name>Shan Cui</name></author>
    <link href="http://arxiv.org/abs/2301.00001v1" rel="alternate" type="text/html"/>
    <link href="http://arxiv.org/pdf/2301.00001v1" rel="related" type="application/pdf"/>
    <arxiv:doi>10.1000/xyz</arxiv:doi>
    <arxiv:primary_category term="cs.AI" xmlns:arxiv="http://arxiv.org/schemas/atom"/>
  </entry>
  <entry>
    <id>http://arxiv.org/abs/2402.00002v3</id>
    <published>2024-02-15T00:00:00Z</published>
    <title>Attention Is All You Need</title>
    <summary>Transformers architecture.</summary>
    <author><name>Vaswani Ashish</name></author>
    <link href="http://arxiv.org/abs/2402.00002v3" rel="alternate" type="text/html"/>
  </entry>
</feed>`

func TestParseArxivAtom(t *testing.T) {
	papers, err := ParseArxivAtom([]byte(arxivAtomSample))
	if err != nil {
		t.Fatalf("ParseArxivAtom: %v", err)
	}
	if len(papers) != 2 {
		t.Fatalf("应有 2 篇，got %d", len(papers))
	}

	// 第一篇：含 DOI，年份 2023
	p1 := papers[0]
	if p1.ArXivID != "2301.00001" {
		t.Errorf("ArXivID：got %q want 2301.00001（应去版本号）", p1.ArXivID)
	}
	if p1.Title == "" {
		t.Errorf("Title 不应为空")
	}
	if p1.Abstract == "" {
		t.Errorf("Abstract 不应为空（FR-SRC-19）")
	}
	if p1.DOI != "10.1000/xyz" {
		t.Errorf("DOI：got %q want 10.1000/xyz", p1.DOI)
	}
	if p1.Year != 2023 {
		t.Errorf("Year：got %d want 2023", p1.Year)
	}
	if p1.Source != "arxiv" {
		t.Errorf("Source：got %q want arxiv", p1.Source)
	}
	if p1.URL == "" {
		t.Errorf("URL 不应为空")
	}
	if !strings.HasPrefix(p1.ID, "sha256:") {
		t.Errorf("ID 应 sha256: 前缀，got %q", p1.ID)
	}
	if len(p1.Authors) != 2 {
		t.Fatalf("应有 2 位作者，got %d", len(p1.Authors))
	}
	if p1.Authors[0].Family == "" {
		t.Errorf("作者 Family 不应为空（姓名应拆分）")
	}
}

func TestExtractArxivID(t *testing.T) {
	cases := map[string]string{
		"http://arxiv.org/abs/2301.00001v1":       "2301.00001",
		"http://arxiv.org/pdf/2402.00002v3":       "2402.00002",
		"http://arxiv.org/abs/solv-int/9612001v2": "solv-int/9612001", // 旧式归档名含 'v'，不得误切归档名
		"http://arxiv.org/abs/solv-int/9612001":   "solv-int/9612001", // 无版本号时归档名保持完整
	}
	for in, want := range cases {
		if got := ExtractArxivID(in); got != want {
			t.Errorf("ExtractArxivID(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestParseArxivAtom_emptyFeed(t *testing.T) {
	papers, err := ParseArxivAtom([]byte(`<?xml version="1.0"?>
<feed xmlns="http://www.w3.org/2005/Atom"></feed>`))
	if err != nil {
		t.Fatalf("空 feed 不应报错： %v", err)
	}
	if len(papers) != 0 {
		t.Fatalf("空 feed 应返回 0 篇，got %d", len(papers))
	}
}

func TestParseArxivAtom_malformedXML(t *testing.T) {
	_, err := ParseArxivAtom([]byte("not xml <"))
	if err == nil {
		t.Fatal("格式错误的 XML 应报错")
	}
}

func TestArxivSource_Search_viaHTTP(t *testing.T) {
	// httptest 模拟 arXiv 端点，验证 Search 端到端（限速 + HTTP + 解析）
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// 校验请求参数
		q := r.URL.Query().Get("search_query")
		if !strings.Contains(q, "graph neural") {
			t.Errorf("请求应含 query，got %q", q)
		}
		if r.URL.Query().Get("max_results") != "2" {
			t.Errorf("max_results=2，got %q", r.URL.Query().Get("max_results"))
		}
		w.Header().Set("Content-Type", "application/atom+xml")
		_, _ = w.Write([]byte(arxivAtomSample))
	}))
	defer srv.Close()

	// 构造 ArxivSource，覆盖 BaseURL 指向 httptest
	src := NewArxivSource(
		newHTTPClient(2000, 1),
		ratelimit.New(100, 5),
	)
	src.BaseURL = srv.URL + "/api/query"

	papers, err := src.Search(context.Background(), "graph neural", SearchOptions{MaxResults: 2})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(papers) != 2 {
		t.Fatalf("应返回 2 篇，got %d", len(papers))
	}
	if papers[0].Source != "arxiv" {
		t.Errorf("Source 应为 arxiv，got %q", papers[0].Source)
	}
}

func TestArxivSearchQuery_tiabDefault(t *testing.T) {
	// 默认/空 mode → tiab：仅题目+摘要
	got := arxivSearchQuery("fear of pain", "")
	if got != `ti:"fear of pain" OR abs:"fear of pain"` {
		t.Fatalf("tiab 检索式不符：%q", got)
	}
}

func TestArxivSearchQuery_full(t *testing.T) {
	got := arxivSearchQuery("fear of pain", "full")
	if got != "all:fear of pain" {
		t.Fatalf("full 检索式不符：%q", got)
	}
}

func TestFilterSince(t *testing.T) {
	papers := []model.Paper{
		{Title: "a", Year: 2020},
		{Title: "b", Year: 2022},
		{Title: "c", Year: 2019},
	}
	out := filterSince(papers, 2021)
	if len(out) != 1 || out[0].Title != "b" {
		t.Fatalf("应只留 2022，got %+v", out)
	}
}

func TestArxivSource_Search_yearFilter(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/atom+xml")
		_, _ = w.Write([]byte(arxivAtomSample))
	}))
	defer srv.Close()

	src := NewArxivSource(newHTTPClient(2000, 1), ratelimit.New(100, 5))
	src.BaseURL = srv.URL + "/api/query"

	// year=2024 应只保留第二篇
	papers, err := src.Search(context.Background(), "x", SearchOptions{MaxResults: 5, Year: 2024})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(papers) != 1 {
		t.Fatalf("year=2024 应过滤为 1 篇，got %d", len(papers))
	}
	if papers[0].Year != 2024 {
		t.Errorf("保留篇应为 2024，got %d", papers[0].Year)
	}
}

func TestArxivSource_Search_HTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	src := NewArxivSource(newHTTPClient(1000, 0), ratelimit.New(100, 5))
	src.BaseURL = srv.URL + "/api/query"

	_, err := src.Search(context.Background(), "x", SearchOptions{MaxResults: 1})
	if err == nil {
		t.Fatal("HTTP 500 应返回错误")
	}
}
