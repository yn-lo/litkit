package sources

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"litkit/internal/util/ratelimit"
)

const pubmedESearchSample = `<?xml version="1.0"?>
<!DOCTYPE eSearchResult>
<eSearchResult>
  <Count>2</Count>
  <IdList>
    <Id>11111111</Id>
    <Id>22222222</Id>
  </IdList>
</eSearchResult>`

const pubmedEFetchSample = `<?xml version="1.0"?>
<!DOCTYPE PubmedArticleSet>
<PubmedArticleSet>
  <PubmedArticle>
    <MedlineCitation>
      <PMID Version="1">11111111</PMID>
      <Article>
        <ArticleTitle>Deep Learning for Healthcare</ArticleTitle>
        <Abstract>
          <AbstractText>This paper explores deep learning in healthcare.</AbstractText>
        </Abstract>
        <AuthorList>
          <Author><ForeName>Alice</ForeName><LastName>Smith</LastName></Author>
          <Author><ForeName>Bob</ForeName><LastName>Jones</LastName></Author>
        </AuthorList>
        <Journal>
          <Title>Nature Medicine</Title>
          <JournalIssue>
            <PubDate><Year>2023</Year></PubDate>
          </JournalIssue>
        </Journal>
      </Article>
    </MedlineCitation>
    <PubmedData>
      <ArticleIdList>
        <ArticleId IdType="doi">10.1000/abc</ArticleId>
      </ArticleIdList>
    </PubmedData>
  </PubmedArticle>
  <PubmedArticle>
    <MedlineCitation>
      <PMID Version="1">22222222</PMID>
      <Article>
        <ArticleTitle>Transformer Architectures</ArticleTitle>
        <Abstract>
          <AbstractText>Transformers have revolutionized NLP.</AbstractText>
        </Abstract>
        <AuthorList>
          <Author><LastName>Wang</LastName></Author>
        </AuthorList>
        <Journal>
          <Title>IEEE TPAMI</Title>
          <JournalIssue>
            <PubDate><Year>2024</Year></PubDate>
          </JournalIssue>
        </Journal>
      </Article>
    </MedlineCitation>
  </PubmedArticle>
</PubmedArticleSet>`

func TestParsePubmedESearch(t *testing.T) {
	ids, err := parsePubmedESearch([]byte(pubmedESearchSample))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(ids) != 2 || ids[0] != "11111111" || ids[1] != "22222222" {
		t.Fatalf("应返回 [11111111 22222222]，got %v", ids)
	}
}

func TestParsePubmedESearch_empty(t *testing.T) {
	ids, err := parsePubmedESearch([]byte(`<?xml version="1.0"?>
<eSearchResult><IdList></IdList></eSearchResult>`))
	if err != nil {
		t.Fatalf("空 IdList 不应报错： %v", err)
	}
	if len(ids) != 0 {
		t.Fatalf("应返回 0 个 id，got %d", len(ids))
	}
}

func TestParsePubmedEFetch(t *testing.T) {
	papers, err := parsePubmedEFetch([]byte(pubmedEFetchSample))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(papers) != 2 {
		t.Fatalf("应返回 2 篇，got %d", len(papers))
	}

	p1 := papers[0]
	if p1.PMID != "11111111" {
		t.Errorf("PMID：got %q want 11111111", p1.PMID)
	}
	if p1.Title != "Deep Learning for Healthcare" {
		t.Errorf("Title：got %q", p1.Title)
	}
	if p1.Abstract == "" {
		t.Errorf("Abstract 不应为空（FR-SRC-19）")
	}
	if p1.DOI != "10.1000/abc" {
		t.Errorf("DOI：got %q want 10.1000/abc", p1.DOI)
	}
	if p1.Year != 2023 {
		t.Errorf("Year：got %d want 2023", p1.Year)
	}
	if p1.Venue != "Nature Medicine" {
		t.Errorf("Venue：got %q want Nature Medicine", p1.Venue)
	}
	if p1.Source != "pubmed" {
		t.Errorf("Source：got %q want pubmed", p1.Source)
	}
	if len(p1.Authors) != 2 {
		t.Fatalf("应有 2 位作者，got %d", len(p1.Authors))
	}
	if p1.Authors[0].Given != "Alice" || p1.Authors[0].Family != "Smith" {
		t.Errorf("第 1 位作者：got %+v", p1.Authors[0])
	}

	// 第二篇无 DOI，ID 仍应基于 PMID 生成
	p2 := papers[1]
	if p2.PMID != "22222222" {
		t.Errorf("p2 PMID：got %q", p2.PMID)
	}
	if !strings.HasPrefix(p2.ID, "sha256:") {
		t.Errorf("p2 ID 应 sha256: 前缀，got %q", p2.ID)
	}
	if len(p2.Authors) != 1 || p2.Authors[0].Family != "Wang" {
		t.Errorf("p2 作者应仅 Wang，got %+v", p2.Authors)
	}
}

func TestPubmedSource_Search_endToEnd(t *testing.T) {
	// httptest 同时模拟 esearch + efetch；按 path 分流
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Path, "esearch"):
			if r.URL.Query().Get("retmax") != "2" {
				t.Errorf("esearch retmax=2，got %q", r.URL.Query().Get("retmax"))
			}
			if r.URL.Query().Get("term") == "" {
				t.Errorf("esearch term 不应为空")
			}
			_, _ = w.Write([]byte(pubmedESearchSample))
		case strings.Contains(r.URL.Path, "efetch"):
			ids := r.URL.Query().Get("id")
			if !strings.Contains(ids, "11111111") {
				t.Errorf("efetch id 应含 esearch 返回的 PMID，got %q", ids)
			}
			_, _ = w.Write([]byte(pubmedEFetchSample))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	src := NewPubmedSource(newHTTPClient(2000, 1), ratelimit.New(100, 5))
	src.ESearchURL = srv.URL + "/esearch.fcgi"
	src.EFetchURL = srv.URL + "/efetch.fcgi"

	papers, err := src.Search(context.Background(), "deep learning", SearchOptions{MaxResults: 2})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(papers) != 2 {
		t.Fatalf("应返回 2 篇，got %d", len(papers))
	}
	if papers[0].Source != "pubmed" {
		t.Errorf("Source 应为 pubmed，got %q", papers[0].Source)
	}
}

func TestPubmedSource_Search_yearServerSideFilter(t *testing.T) {
	var capturedMindate, capturedMaxdate string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "esearch") {
			capturedMindate = r.URL.Query().Get("mindate")
			capturedMaxdate = r.URL.Query().Get("maxdate")
			_, _ = w.Write([]byte(pubmedESearchSample))
		} else {
			_, _ = w.Write([]byte(pubmedEFetchSample))
		}
	}))
	defer srv.Close()

	src := NewPubmedSource(newHTTPClient(2000, 1), ratelimit.New(100, 5))
	src.ESearchURL = srv.URL + "/esearch.fcgi"
	src.EFetchURL = srv.URL + "/efetch.fcgi"

	_, err := src.Search(context.Background(), "x", SearchOptions{MaxResults: 1, Year: 2023})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if capturedMindate != "2023" || capturedMaxdate != "2023" {
		t.Errorf("year=2023 应转换为 mindate/maxdate=2023，got mindate=%s maxdate=%s", capturedMindate, capturedMaxdate)
	}
}

func TestPubmedSource_Search_emptyIDListReturnsEmpty(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`<?xml version="1.0"?><eSearchResult><IdList></IdList></eSearchResult>`))
	}))
	defer srv.Close()

	src := NewPubmedSource(newHTTPClient(2000, 1), ratelimit.New(100, 5))
	src.ESearchURL = srv.URL + "/esearch.fcgi"
	src.EFetchURL = srv.URL + "/efetch.fcgi"

	papers, err := src.Search(context.Background(), "x", SearchOptions{MaxResults: 5})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(papers) != 0 {
		t.Fatalf("空 IdList 不应调 efetch，应返回 0 篇，got %d", len(papers))
	}
}
