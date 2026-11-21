package core

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"litkit/internal/model"
	"litkit/internal/util/httpclient"
)

// 真实 CrossRef works/{doi} 响应片段（字段命名与生产保持一致）。
const crossrefWorkSample = `{
  "status": "ok",
  "message": {
    "title": ["Graph Neural Networks: A Review"],
    "author": [
      {"given": "Jie", "family": "Zhou"},
      {"given": "Ganqu", "family": "Cui"}
    ],
    "abstract": "<jats:p>We review graph neural networks.</jats:p><jats:p>Methods and applications.</jats:p>",
    "issued": {"date-parts": [[2020, 7, 1]]},
    "container-title": ["IEEE Transactions on Knowledge and Data Engineering"],
    "DOI": "10.1000/xyz",
    "type": "journal-article",
    "volume": "35",
    "issue": "4",
    "page": "123-135",
    "URL": "https://doi.org/10.1000/xyz"
  }
}`

// 真实 NCBI efetch 响应片段（wm RETTYPE=abstract，含摘要与卷期页）。
const pubmedEFetchSample = `<?xml version="1.0"?>
<PubmedArticleSet>
  <PubmedArticle>
    <MedlineCitation>
      <PMID Version="1">12345</PMID>
      <Article>
        <ArticleTitle>Deep Learning</ArticleTitle>
        <Abstract>
          <AbstractText>Deep learning allows computational models of multiple processing layers.</AbstractText>
        </Abstract>
        <AuthorList>
          <Author><ForeName>Yann</ForeName><LastName>LeCun</LastName></Author>
          <Author><ForeName>Yoshua</ForeName><LastName>Bengio</LastName></Author>
        </AuthorList>
        <Journal>
          <Title>Nature</Title>
          <JournalIssue>
            <Volume>521</Volume>
            <Issue>7553</Issue>
            <PubDate><Year>2015</Year></PubDate>
          </JournalIssue>
        </Journal>
        <Pagination><MedlinePgn>436-444</MedlinePgn></Pagination>
      </Article>
    </MedlineCitation>
    <PubmedData>
      <ArticleIdList>
        <ArticleId IdType="doi">10.1038/nature14539</ArticleId>
      </ArticleIdList>
    </PubmedData>
  </PubmedArticle>
</PubmedArticleSet>`

// 真实 arXiv Atom Feed 片段（journal_ref / doi 在 arxiv 命名空间）。
const arxivEntrySample = `<?xml version="1.0" encoding="UTF-8"?>
<feed xmlns="http://www.w3.org/2005/Atom" xmlns:arxiv="http://arxiv.org/schemas/atom">
  <entry>
    <id>http://arxiv.org/abs/2001.00001v2</id>
    <published>2020-01-01T00:00:00Z</published>
    <title>Attention Is All You Need</title>
    <summary>We propose the Transformer architecture.</summary>
    <author><name>Vaswani Ashish</name></author>
    <author><name>Shazeer Noam</name></author>
    <arxiv:journal_ref>NeurIPS 2017</arxiv:journal_ref>
    <arxiv:doi>10.5555/123</arxiv:doi>
  </entry>
</feed>`

// CrossRef works 检索响应片段（两条候选，应取第一条；标题与查询 "graph neural networks" 相似）。
const crossrefSearchSample = `{
  "status": "ok",
  "message": {
    "items": [
      {
        "title": ["Graph Neural Networks: A Review"],
        "author": [{"given": "Ann", "family": "Author"}],
        "issued": {"date-parts": [[2019]]},
        "container-title": ["Some Journal"],
        "DOI": "10.2000/first",
        "type": "journal-article",
        "URL": "https://doi.org/10.2000/first"
      },
      {
        "title": ["Second Result"],
        "DOI": "10.2000/second"
      }
    ]
  }
}`

// CrossRef works 检索响应片段（首条标题与查询明显不符，应判未命中）。
const crossrefSearchMismatchSample = `{
  "status": "ok",
  "message": {
    "items": [
      {
        "title": ["Found via Title Search"],
        "DOI": "10.2000/first"
      }
    ]
  }
}`

// newMetadataFetcher 构造测试用反查器（短超时，指向真实 base，各测试覆盖对应字段）。
func newMetadataFetcher() *MetadataFetcher {
	return NewMetadataFetcher(httpclient.New(httpclient.Options{TimeoutMS: 5000}))
}

func TestMetadataFetch_DOI(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// DOI 中的斜杠应被 PathEscape 编码（服务端解码后仍是原始 DOI）
		if r.URL.Path != "/works/10.1000/xyz" {
			t.Errorf("请求路径不符（应已 PathEscape DOI）：got %q", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(crossrefWorkSample))
	}))
	defer srv.Close()

	f := newMetadataFetcher()
	f.CrossrefBase = srv.URL

	p, err := f.Fetch(context.Background(), "doi", "10.1000/xyz")
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if p == nil {
		t.Fatal("应命中返回论文")
	}
	if p.Title != "Graph Neural Networks: A Review" {
		t.Errorf("Title：got %q", p.Title)
	}
	if len(p.Authors) != 2 {
		t.Fatalf("应 2 位作者，got %d", len(p.Authors))
	}
	if p.Authors[0] != (model.Author{Given: "Jie", Family: "Zhou"}) {
		t.Errorf("Authors[0]：got %+v", p.Authors[0])
	}
	// abstract 的 JATS XML 标签应被剥除
	if p.Abstract != "We review graph neural networks.Methods and applications." {
		t.Errorf("Abstract（应去 XML 标签）：got %q", p.Abstract)
	}
	if p.Year != 2020 {
		t.Errorf("Year：got %d", p.Year)
	}
	if p.Venue != "IEEE Transactions on Knowledge and Data Engineering" {
		t.Errorf("Venue：got %q", p.Venue)
	}
	if p.DOI != "10.1000/xyz" {
		t.Errorf("DOI：got %q", p.DOI)
	}
	if p.Volume != "35" || p.Number != "4" || p.Pages != "123-135" {
		t.Errorf("Volume/Number/Pages：got %q/%q/%q", p.Volume, p.Number, p.Pages)
	}
	if p.URL != "https://doi.org/10.1000/xyz" {
		t.Errorf("URL：got %q", p.URL)
	}
	if p.DocType != "journal-article" {
		t.Errorf("DocType：got %q", p.DocType)
	}
	if p.Source != "crossref" {
		t.Errorf("Source：got %q", p.Source)
	}
}

func TestMetadataFetch_DOI_Missing(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r) // CrossRef 未知 DOI 返回 404
	}))
	defer srv.Close()

	f := newMetadataFetcher()
	f.CrossrefBase = srv.URL

	p, err := f.Fetch(context.Background(), "doi", "10.1000/nope")
	if err != nil {
		t.Fatalf("未命中不应报错： %v", err)
	}
	if p != nil {
		t.Fatalf("未命中应返回 nil，got %+v", p)
	}
}

func TestMetadataFetch_PMID(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/entrez/eutils/efetch.fcgi" {
			t.Errorf("请求路径不符（esummary 不含摘要，必须走 efetch）：got %q", r.URL.Path)
		}
		q := r.URL.Query()
		if q.Get("db") != "pubmed" || q.Get("id") != "12345" || q.Get("retmode") != "xml" {
			t.Errorf("请求参数不符：%s", r.URL.RawQuery)
		}
		w.Header().Set("Content-Type", "application/xml")
		_, _ = w.Write([]byte(pubmedEFetchSample))
	}))
	defer srv.Close()

	f := newMetadataFetcher()
	f.EutilsBase = srv.URL

	p, err := f.Fetch(context.Background(), "pmid", "12345")
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if p == nil {
		t.Fatal("应命中返回论文")
	}
	if p.Title != "Deep Learning" {
		t.Errorf("Title：got %q", p.Title)
	}
	if p.PMID != "12345" {
		t.Errorf("PMID：got %q", p.PMID)
	}
	// efetch 直接给出 ForeName/LastName
	if len(p.Authors) != 2 {
		t.Fatalf("应 2 位作者，got %d", len(p.Authors))
	}
	if p.Authors[0] != (model.Author{Family: "LeCun", Given: "Yann"}) {
		t.Errorf("Authors[0]：got %+v", p.Authors[0])
	}
	if p.Year != 2015 {
		t.Errorf("Year（应取 pubdate 前 4 位）：got %d", p.Year)
	}
	if p.Venue != "Nature" {
		t.Errorf("Venue：got %q", p.Venue)
	}
	// DOI 应来自 articleids 中 idtype=="doi"
	if p.DOI != "10.1038/nature14539" {
		t.Errorf("DOI（应取自 articleids）：got %q", p.DOI)
	}
	if p.Volume != "521" || p.Number != "7553" || p.Pages != "436-444" {
		t.Errorf("Volume/Number/Pages：got %q/%q/%q", p.Volume, p.Number, p.Pages)
	}
	if p.DocType != "article" {
		t.Errorf("DocType：got %q", p.DocType)
	}
}

func TestMetadataFetch_Arxiv(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/query" {
			t.Errorf("请求路径不符：got %q", r.URL.Path)
		}
		if q := r.URL.Query().Get("id_list"); q != "2001.00001" {
			t.Errorf("id_list 参数不符：got %q", q)
		}
		w.Header().Set("Content-Type", "application/atom+xml")
		_, _ = w.Write([]byte(arxivEntrySample))
	}))
	defer srv.Close()

	f := newMetadataFetcher()
	f.ArxivBase = srv.URL

	p, err := f.Fetch(context.Background(), "arxiv", "2001.00001")
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if p == nil {
		t.Fatal("应命中返回论文")
	}
	if p.Title != "Attention Is All You Need" {
		t.Errorf("Title：got %q", p.Title)
	}
	if p.Abstract != "We propose the Transformer architecture." {
		t.Errorf("Abstract：got %q", p.Abstract)
	}
	if p.Year != 2020 {
		t.Errorf("Year：got %d", p.Year)
	}
	// ArXivID 应去 /abs/ 前缀与 v 版本后缀
	if p.ArXivID != "2001.00001" {
		t.Errorf("ArXivID（应去版本号）：got %q", p.ArXivID)
	}
	// arXiv 姓名格式 "Given Family"：首空格前为 Given，后为 Family
	if len(p.Authors) != 2 {
		t.Fatalf("应 2 位作者，got %d", len(p.Authors))
	}
	if p.Authors[0] != (model.Author{Given: "Vaswani", Family: "Ashish"}) {
		t.Errorf("Authors[0]：got %+v", p.Authors[0])
	}
	if p.Venue != "NeurIPS 2017" {
		t.Errorf("Venue（journal_ref）：got %q", p.Venue)
	}
	if p.DOI != "10.5555/123" {
		t.Errorf("DOI（arxiv:doi）：got %q", p.DOI)
	}
	if p.DocType != "preprint" {
		t.Errorf("DocType：got %q", p.DocType)
	}
}

func TestMetadataFetch_Title(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/works" {
			t.Errorf("请求路径不符：got %q", r.URL.Path)
		}
		q := r.URL.Query()
		if q.Get("query.bibliographic") != "graph neural networks" {
			t.Errorf("query.bibliographic 不符：got %q", q.Get("query.bibliographic"))
		}
		if q.Get("rows") != "1" {
			t.Errorf("rows 应为 1：got %q", q.Get("rows"))
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(crossrefSearchSample))
	}))
	defer srv.Close()

	f := newMetadataFetcher()
	f.CrossrefBase = srv.URL

	p, err := f.Fetch(context.Background(), "title", "graph neural networks")
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if p == nil {
		t.Fatal("应命中返回论文")
	}
	// 应取 items[0]（标题与查询相似，通过校验）
	if p.Title != "Graph Neural Networks: A Review" {
		t.Errorf("Title（应取第一条）：got %q", p.Title)
	}
	if p.DOI != "10.2000/first" {
		t.Errorf("DOI：got %q", p.DOI)
	}
	if p.Year != 2019 {
		t.Errorf("Year：got %d", p.Year)
	}
}

func TestMetadataFetch_Title_Mismatch(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(crossrefSearchMismatchSample))
	}))
	defer srv.Close()

	f := newMetadataFetcher()
	f.CrossrefBase = srv.URL

	// 首条标题与查询明显不符：应视为未命中（nil），由调用方归入 Unresolved。
	p, err := f.Fetch(context.Background(), "title", "graph neural networks")
	if err != nil {
		t.Fatalf("标题不匹配不应报错：%v", err)
	}
	if p != nil {
		t.Fatalf("标题不匹配应返回 nil，got %+v", p)
	}
}

func TestMetadataFetch_UnknownType(t *testing.T) {
	f := newMetadataFetcher()
	p, err := f.Fetch(context.Background(), "isbn", "978-0-13-110362-7")
	if err == nil {
		t.Fatal("未知 id_type 应返回错误")
	}
	if !strings.Contains(err.Error(), "未知 id_type") {
		t.Errorf("错误信息应提示未知 id_type：%v", err)
	}
	if p != nil {
		t.Fatalf("错误时不应返回论文，got %+v", p)
	}
}

func TestMetadataFetch_HTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	f := newMetadataFetcher()
	f.CrossrefBase = srv.URL

	p, err := f.Fetch(context.Background(), "doi", "10.1000/xyz")
	if err == nil {
		t.Fatal("HTTP 500 应返回错误")
	}
	if !strings.Contains(err.Error(), "HTTP 500") {
		t.Errorf("错误信息应含 HTTP 500：%v", err)
	}
	if p != nil {
		t.Fatalf("错误时不应返回论文，got %+v", p)
	}
}

// crossrefRetractedSample 原论文通过 updated-by 指向撤稿声明（R5.7）。
const crossrefRetractedSample = `{
  "status": "ok",
  "message": {
    "title": ["Retracted paper"],
    "DOI": "10.1000/retracted",
    "type": "journal-article",
    "updated-by": [
      {"DOI": "10.1000/retraction-1", "type": "retraction", "source": "retraction-watch", "label": "Retraction"}
    ]
  }
}`

// crossrefCleanSample 无 updated-by（未撤稿）。
const crossrefCleanSample = `{
  "status": "ok",
  "message": {
    "title": ["Fine paper"],
    "DOI": "10.1000/fine",
    "type": "journal-article"
  }
}`

func TestCheckRetraction_Retracted(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(crossrefRetractedSample))
	}))
	defer srv.Close()

	f := newMetadataFetcher()
	f.CrossrefBase = srv.URL

	info, err := f.CheckRetraction(context.Background(), "10.1000/retracted")
	if err != nil {
		t.Fatalf("CheckRetraction: %v", err)
	}
	if !info.Retracted {
		t.Fatal("应判定为已撤稿")
	}
	if info.RetractionDOI != "10.1000/retraction-1" {
		t.Errorf("RetractionDOI：got %q", info.RetractionDOI)
	}
	if info.Source != "retraction-watch" {
		t.Errorf("Source：got %q", info.Source)
	}
}

func TestCheckRetraction_Clean(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(crossrefCleanSample))
	}))
	defer srv.Close()

	f := newMetadataFetcher()
	f.CrossrefBase = srv.URL

	info, err := f.CheckRetraction(context.Background(), "10.1000/fine")
	if err != nil {
		t.Fatalf("CheckRetraction: %v", err)
	}
	if info.Retracted {
		t.Fatal("未撤稿文献不应判为已撤稿")
	}
}

func TestCheckRetraction_NoticeIsNotRetracted(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"message":{"DOI":"10.1000/notice","update-to":[{"DOI":"10.1000/original","type":"retraction"}],"updated-by":[{"DOI":"10.1000/correction","type":"correction"}]}}`))
	}))
	defer srv.Close()
	f := newMetadataFetcher()
	f.CrossrefBase = srv.URL
	info, err := f.CheckRetraction(context.Background(), "10.1000/notice")
	if err != nil || info.Retracted {
		t.Fatalf("撤稿声明不等于声明本身已撤稿: %+v, err=%v", info, err)
	}
}

// TestMetadataFetch_DOI_abstractBackfill Crossref 常缺摘要：应回落 OpenAlex 补齐（摘要工作流必需）。
func TestMetadataFetch_DOI_abstractBackfill(t *testing.T) {
	openAlexWork := `{
	  "id": "https://openalex.org/W1",
	  "doi": "https://doi.org/10.1000/xyz",
	  "title": "Graph Neural Networks: A Review",
	  "abstract_inverted_index": {"We": [0], "review": [1], "graphs": [2]},
	  "publication_year": 2020,
	  "type": "article"
	}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/works/10.1000/xyz":
			// Crossref 记录：无 abstract 字段
			_, _ = w.Write([]byte(`{"status":"ok","message":{"title":["Graph Neural Networks: A Review"],"DOI":"10.1000/xyz","type":"journal-article"}}`))
		case strings.HasPrefix(r.URL.Path, "/works/https://doi.org/"):
			_, _ = w.Write([]byte(openAlexWork))
		default:
			t.Errorf("未预期请求路径：%q", r.URL.Path)
		}
	}))
	defer srv.Close()

	f := newMetadataFetcher()
	f.CrossrefBase = srv.URL
	f.OpenAlexBase = srv.URL

	p, err := f.Fetch(context.Background(), "doi", "10.1000/xyz")
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if p == nil {
		t.Fatal("应命中返回论文")
	}
	if p.Abstract != "We review graphs" {
		t.Errorf("摘要应由 OpenAlex 补齐，got %q", p.Abstract)
	}
}

// TestMetadataFetch_DOI_abstractBackfillFailsClosed OpenAlex 兜底失败时不得报错（摘要缺失非身份错误）。
func TestMetadataFetch_DOI_abstractBackfillFailsClosed(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/works/10.1000/xyz" {
			_, _ = w.Write([]byte(`{"status":"ok","message":{"title":["T"],"DOI":"10.1000/xyz"}}`))
			return
		}
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	f := newMetadataFetcher()
	f.CrossrefBase = srv.URL
	f.OpenAlexBase = srv.URL

	p, err := f.Fetch(context.Background(), "doi", "10.1000/xyz")
	if err != nil {
		t.Fatalf("兜底失败不应报错：%v", err)
	}
	if p == nil || p.Abstract != "" {
		t.Fatalf("应返回无摘要论文，got %+v", p)
	}
}
