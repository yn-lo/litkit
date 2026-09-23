package sources

import (
	"context"
	"encoding/xml"
	"fmt"
	"html"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"litkit/internal/model"
	"litkit/internal/util/httpclient"
	"litkit/internal/util/ratelimit"
)

// PubmedSource PubMed 适配器（FR-SRC-03）。
//
// 端点：NCBI EUtils（esearch.fcgi + efetch.fcgi）。
// 限速：无 key 3 req/s（保守取 2 RPS, burst 3，NFR-PERF-04）。
// 年份过滤走服务端 mindate/maxdate（PubMed 原生支持）。
type PubmedSource struct {
	BaseSource
	ESearchURL string // 默认 https://eutils.ncbi.nlm.nih.gov/entrez/eutils/esearch.fcgi
	EFetchURL  string // 默认 https://eutils.ncbi.nlm.nih.gov/entrez/eutils/efetch.fcgi
	APIKey     string // 可选；有 key 提速（PRD 7.3）
}

// NewPubmedSource 创建 PubMed 适配器。
func NewPubmedSource(httpClient *httpclient.Client, limiter *ratelimit.Limiter) *PubmedSource {
	return &PubmedSource{
		BaseSource: NewBaseSource("pubmed", httpClient, limiter),
		ESearchURL: "https://eutils.ncbi.nlm.nih.gov/entrez/eutils/esearch.fcgi",
		EFetchURL:  "https://eutils.ncbi.nlm.nih.gov/entrez/eutils/efetch.fcgi",
	}
}

// eSearchResult EUtils esearch 响应结构。
type eSearchResult struct {
	XMLName xml.Name `xml:"eSearchResult"`
	Count   string   `xml:"Count"`
	IDList  struct {
		IDs []string `xml:"Id"`
	} `xml:"IdList"`
}

// pubmedArticleSet EUtils efetch 响应结构（PubmedArticleSet）。
type pubmedArticleSet struct {
	XMLName        xml.Name        `xml:"PubmedArticleSet"`
	PubmedArticles []pubmedArticle `xml:"PubmedArticle"`
}

// pubmedAbstractText AbstractText 段：Label 属性 + 混合内容。
// Value 用 innerxml 而非 chardata——PubMed 标题/摘要内嵌 <sup>/<sub>/<italic> 等
// 标签，chardata 会丢弃子元素内文本（10<sup>3</sup> 变 10），破坏科学内容。
type pubmedAbstractText struct {
	Label string `xml:"Label,attr"`
	Value string `xml:",innerxml"`
}

type pubmedArticle struct {
	MedlineCitation struct {
		PMID    string `xml:"PMID"`
		Article struct {
			Title struct {
				Inner string `xml:",innerxml"` // innerxml 不能与元素名同用，借包装字段取混合内容
			} `xml:"ArticleTitle"`
			Abstract struct {
				Texts []pubmedAbstractText `xml:"AbstractText"`
			} `xml:"Abstract"`
			Authors []struct {
				ForeName string `xml:"ForeName"`
				LastName string `xml:"LastName"`
			} `xml:"AuthorList>Author"`
			Journal struct {
				Title string `xml:"Title"`
				Issue struct {
					PubDate struct {
						Year  string `xml:"Year"`
						Month string `xml:"Month"`
						Day   string `xml:"Day"`
					} `xml:"PubDate"`
				} `xml:"JournalIssue"`
			} `xml:"Journal"`
		} `xml:"Article"`
	} `xml:"MedlineCitation"`
	PubmedData struct {
		ArticleIDs []struct {
			IDType string `xml:"IdType,attr"`
			Value  string `xml:",chardata"`
		} `xml:"ArticleIdList>ArticleId"`
	} `xml:"PubmedData"`
}

// PubMed 混合内容转平文用的标签与上下标映射。
var (
	supTagRe   = regexp.MustCompile(`<sup>(.*?)</sup>`)
	subTagRe   = regexp.MustCompile(`<sub>(.*?)</sub>`)
	otherTagRe = regexp.MustCompile(`<[^>]+>`)

	supMap = map[rune]rune{'0': '⁰', '1': '¹', '2': '²', '3': '³', '4': '⁴', '5': '⁵', '6': '⁶', '7': '⁷', '8': '⁸', '9': '⁹', '+': '⁺', '-': '⁻', '−': '⁻', 'n': 'ⁿ'}
	subMap = map[rune]rune{'0': '₀', '1': '₁', '2': '₂', '3': '₃', '4': '₄', '5': '₅', '6': '₆', '7': '₇', '8': '₈', '9': '₉', '+': '₊', '-': '₋', '−': '₋'}
)

// scriptToUnicode 上下标内容整体转 Unicode 上下标字符；含不可映射字符时原样返回（退化为平文内联）。
func scriptToUnicode(s string, m map[rune]rune) string {
	out := make([]rune, 0, len(s))
	for _, r := range s {
		mapped, ok := m[r]
		if !ok {
			return s
		}
		out = append(out, mapped)
	}
	return string(out)
}

// stripInlineXML 将 PubMed 标题/摘要的混合 XML 内容转平文：
// sup/sub 内容转 Unicode 上下标保留语义（10<sup>3</sup> → 10³、H<sub>2</sub>O → H₂O、
// CD<sup>4</sup> → CD⁴），其余标签剥离保留内文，实体反转义，空白规范化。
func stripInlineXML(s string) string {
	s = supTagRe.ReplaceAllStringFunc(s, func(m string) string {
		return scriptToUnicode(supTagRe.FindStringSubmatch(m)[1], supMap)
	})
	s = subTagRe.ReplaceAllStringFunc(s, func(m string) string {
		return scriptToUnicode(subTagRe.FindStringSubmatch(m)[1], subMap)
	})
	s = otherTagRe.ReplaceAllString(s, "")
	s = html.UnescapeString(s)
	return strings.Join(strings.Fields(s), " ")
}

// pubmedTerm 按检索等级构造 PubMed 检索词（FR-SEARCH-12）。
//
//   - tiab（默认）：[Title/Abstract] + [Keyword]，题目+摘要+关键词三字段
//   - full：裸词（NCBI 全字段，含全文/MeSH 等）
func pubmedTerm(query, mode string) string {
	if mode == modeFull {
		return query
	}
	return "(" + query + `[Title/Abstract]) OR (` + query + "[Keyword])"
}

// Search 执行 PubMed 两阶段检索：esearch 取 PMID → efetch 取详情。
func (p *PubmedSource) Search(ctx context.Context, query string, opts SearchOptions) ([]model.Paper, error) {
	ids, err := p.esearch(ctx, query, opts)
	if err != nil {
		return nil, fmt.Errorf("pubmed search: %w", err)
	}
	if len(ids) == 0 {
		return []model.Paper{}, nil
	}
	data, err := p.efetch(ctx, ids)
	if err != nil {
		return nil, fmt.Errorf("pubmed search: %w", err)
	}
	papers, err := parsePubmedEFetch(data)
	if err != nil {
		return nil, fmt.Errorf("pubmed search: %w", err)
	}
	return papers, nil
}

func (p *PubmedSource) esearch(ctx context.Context, query string, opts SearchOptions) ([]string, error) {
	q := url.Values{}
	q.Set("db", "pubmed")
	q.Set("term", pubmedTerm(query, opts.Mode))
	q.Set("retmax", strconv.Itoa(ensureMax(opts.MaxResults, defaultMaxResults)))
	q.Set("retmode", "xml")
	if opts.Year != 0 {
		// 服务端年份过滤（PubMed 原生支持）
		q.Set("datetype", "pdat")
		q.Set("mindate", strconv.Itoa(opts.Year))
		q.Set("maxdate", strconv.Itoa(opts.Year))
	} else if opts.Since != 0 {
		// 最近 N 年范围过滤（FR-SEARCH-13）
		q.Set("datetype", "pdat")
		q.Set("mindate", strconv.Itoa(opts.Since))
		q.Set("maxdate", strconv.Itoa(time.Now().Year()))
	}
	if p.APIKey != "" {
		q.Set("api_key", p.APIKey)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, p.ESearchURL+"?"+q.Encode(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", defaultUserAgent)
	resp, err := p.Do(ctx, req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("esearch HTTP %d", resp.StatusCode)
	}
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("esearch read body: %w", err)
	}
	return parsePubmedESearch(data)
}

func (p *PubmedSource) efetch(ctx context.Context, ids []string) ([]byte, error) {
	q := url.Values{}
	q.Set("db", "pubmed")
	q.Set("id", strings.Join(ids, ","))
	q.Set("rettype", "abstract")
	q.Set("retmode", "xml")
	if p.APIKey != "" {
		q.Set("api_key", p.APIKey)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, p.EFetchURL+"?"+q.Encode(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", defaultUserAgent)
	resp, err := p.Do(ctx, req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("efetch HTTP %d", resp.StatusCode)
	}
	return io.ReadAll(resp.Body)
}

// parsePubmedESearch 解析 esearch 响应，提取 PMID 列表。
func parsePubmedESearch(data []byte) ([]string, error) {
	var r eSearchResult
	if err := xml.Unmarshal(data, &r); err != nil {
		return nil, fmt.Errorf("parse esearch: %w", err)
	}
	out := make([]string, 0, len(r.IDList.IDs))
	out = append(out, r.IDList.IDs...)
	return out, nil
}

// parsePubmedEFetch 解析 efetch 响应（PubmedArticleSet）为 []Paper。
func parsePubmedEFetch(data []byte) ([]model.Paper, error) {
	var set pubmedArticleSet
	if err := xml.Unmarshal(data, &set); err != nil {
		return nil, fmt.Errorf("parse efetch: %w", err)
	}
	papers := make([]model.Paper, 0, len(set.PubmedArticles))
	for _, art := range set.PubmedArticles {
		ma := art.MedlineCitation
		p := model.Paper{
			Title:    stripInlineXML(ma.Article.Title.Inner),
			Abstract: composePubmedAbstract(ma.Article.Abstract.Texts),
			PMID:     strings.TrimSpace(ma.PMID),
			Year:     atoiSafe(ma.Article.Journal.Issue.PubDate.Year),
			Venue:    strings.TrimSpace(ma.Article.Journal.Title),
			Authors:  pubmedAuthorsToModel(ma.Article.Authors),
			Source:   "pubmed",
			DocType:  DocTypeArticle,
		}
		p.DOI = pubmedPickArticleID(art.PubmedData.ArticleIDs, "doi")
		if p.URL == "" && p.DOI != "" {
			p.URL = "https://doi.org/" + p.DOI
		}
		p.ID = p.ComputeID()
		papers = append(papers, p)
	}
	return papers, nil
}

// composePubmedAbstract 合并 AbstractText 多段（带 Label），段内容经 stripInlineXML 转平文。
func composePubmedAbstract(texts []pubmedAbstractText) string {
	if len(texts) == 0 {
		return ""
	}
	parts := make([]string, 0, len(texts))
	for _, t := range texts {
		v := strings.TrimSpace(stripInlineXML(t.Value))
		if v == "" {
			continue
		}
		if t.Label != "" {
			parts = append(parts, t.Label+": "+v)
		} else {
			parts = append(parts, v)
		}
	}
	return strings.Join(parts, " ")
}

func pubmedAuthorsToModel(authors []struct {
	ForeName string `xml:"ForeName"`
	LastName string `xml:"LastName"`
}) []model.Author {
	out := make([]model.Author, 0, len(authors))
	for _, a := range authors {
		out = append(out, model.Author{
			Given:  strings.TrimSpace(a.ForeName),
			Family: strings.TrimSpace(a.LastName),
		})
	}
	return out
}

func pubmedPickArticleID(ids []struct {
	IDType string `xml:"IdType,attr"`
	Value  string `xml:",chardata"`
}, idType string) string {
	for _, id := range ids {
		if id.IDType == idType {
			return strings.TrimSpace(id.Value)
		}
	}
	return ""
}

func atoiSafe(s string) int {
	n, err := strconv.Atoi(strings.TrimSpace(s))
	if err != nil {
		return 0
	}
	return n
}
