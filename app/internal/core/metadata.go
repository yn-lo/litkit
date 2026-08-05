// metadata.go 实现按标识符反查论文元数据（FR-REF-02）。
//
// 反查源：
//   - doi → CrossRef /works/{doi}
//   - pmid → NCBI eutils esummary（JSON）
//   - arxiv → arXiv API（Atom XML）
//   - title → CrossRef works 检索（rows=1 取首条）
//
// 未命中（404 / 空结果）返回 (nil, nil)；网络或解析错误返回 error。

package core

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"

	"litkit/internal/model"
	"litkit/internal/sources"
	"litkit/internal/util/httpclient"
)

// MetadataFetcher 按标识符反查论文元数据（FR-REF-02）。
//
// 依赖注入 httpclient.Client，便于测试 mock。
type MetadataFetcher struct {
	client *httpclient.Client

	crossrefBase string // 测试可覆盖
	eutilsBase   string
	arxivBase    string
}

// NewMetadataFetcher 创建反查器。
func NewMetadataFetcher(client *httpclient.Client) *MetadataFetcher {
	return &MetadataFetcher{
		client:       client,
		crossrefBase: "https://api.crossref.org",
		eutilsBase:   "https://eutils.ncbi.nlm.nih.gov",
		arxivBase:    "http://export.arxiv.org",
	}
}

// Fetch 按 id_type（doi|pmid|arxiv|title）与 identifier 反查论文。
//
// 未命中返回 (nil, nil)；网络/解析错误返回 error。
// 未知 id_type 返回错误。
func (f *MetadataFetcher) Fetch(ctx context.Context, idType, identifier string) (*model.Paper, error) {
	switch idType {
	case "doi":
		return f.fetchDOI(ctx, identifier)
	case "pmid":
		return f.fetchPMID(ctx, identifier)
	case "arxiv":
		return f.fetchArxiv(ctx, identifier)
	case "title":
		return f.fetchTitle(ctx, identifier)
	default:
		return nil, fmt.Errorf("metadata: 未知 id_type %q（可选 doi|pmid|arxiv|title）", idType)
	}
}

// get 执行 GET 请求并返回响应体。
//
// 404 视为未命中返回 (nil, nil)；其余非 200 返回 HTTP 错误。
func (f *MetadataFetcher) get(ctx context.Context, idType, u string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, fmt.Errorf("metadata %s: %w", idType, err)
	}
	req.Header.Set("User-Agent", metadataUserAgent)
	resp, err := f.client.Do(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("metadata %s: %w", idType, err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode == http.StatusNotFound {
		return nil, nil
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("metadata %s: HTTP %d", idType, resp.StatusCode)
	}
	data, err := httpclient.ReadAll(resp)
	if err != nil {
		return nil, fmt.Errorf("metadata %s: 读取响应: %w", idType, err)
	}
	return data, nil
}

func (f *MetadataFetcher) fetchDOI(ctx context.Context, doi string) (*model.Paper, error) {
	u := f.crossrefBase + "/works/" + url.PathEscape(doi)
	data, err := f.get(ctx, "doi", u)
	if err != nil || data == nil {
		return nil, err
	}
	var resp crossrefResponse
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, fmt.Errorf("metadata doi: 解析 CrossRef 响应: %w", err)
	}
	return crossrefMessageToPaper(resp.Message), nil
}

func (f *MetadataFetcher) fetchTitle(ctx context.Context, title string) (*model.Paper, error) {
	q := url.Values{}
	q.Set("query.bibliographic", title)
	q.Set("rows", "1")
	u := f.crossrefBase + "/works?" + q.Encode()
	data, err := f.get(ctx, "title", u)
	if err != nil || data == nil {
		return nil, err
	}
	var resp crossrefSearchResponse
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, fmt.Errorf("metadata title: 解析 CrossRef 检索响应: %w", err)
	}
	if len(resp.Message.Items) == 0 {
		return nil, nil
	}
	return crossrefMessageToPaper(resp.Message.Items[0]), nil
}

func (f *MetadataFetcher) fetchPMID(ctx context.Context, pmid string) (*model.Paper, error) {
	q := url.Values{}
	q.Set("db", "pubmed")
	q.Set("id", pmid)
	q.Set("retmode", "json")
	u := f.eutilsBase + "/entrez/eutils/esummary.fcgi?" + q.Encode()
	data, err := f.get(ctx, "pmid", u)
	if err != nil || data == nil {
		return nil, err
	}
	var resp esummaryResponse
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, fmt.Errorf("metadata pmid: 解析 esummary 响应: %w", err)
	}
	doc, ok := resp.Result[pmid]
	if !ok {
		return nil, nil
	}
	var d esummaryDoc
	if err := json.Unmarshal(doc, &d); err != nil {
		return nil, fmt.Errorf("metadata pmid: 解析 esummary 记录: %w", err)
	}
	return esummaryToPaper(d, pmid), nil
}

func (f *MetadataFetcher) fetchArxiv(ctx context.Context, id string) (*model.Paper, error) {
	q := url.Values{}
	q.Set("id_list", id)
	u := f.arxivBase + "/api/query?" + q.Encode()
	data, err := f.get(ctx, "arxiv", u)
	if err != nil || data == nil {
		return nil, err
	}
	papers, err := sources.ParseArxivAtom(data)
	if err != nil {
		return nil, fmt.Errorf("metadata arxiv: %w", err)
	}
	if len(papers) == 0 {
		return nil, nil
	}
	p := papers[0]
	return &p, nil
}

// ---- CrossRef ----

// crossrefResponse /works/{doi} 响应。
type crossrefResponse struct {
	Message crossrefMessage `json:"message"`
}

// crossrefSearchResponse /works 检索响应（items 数组）。
type crossrefSearchResponse struct {
	Message struct {
		Items []crossrefMessage `json:"items"`
	} `json:"message"`
}

type crossrefMessage struct {
	Title          []string         `json:"title"`
	Author         []crossrefAuthor `json:"author"`
	Abstract       string           `json:"abstract"`
	Issued         crossrefDate     `json:"issued"`
	ContainerTitle []string         `json:"container-title"`
	DOI            string           `json:"DOI"`
	Type           string           `json:"type"`
	Volume         string           `json:"volume"`
	Issue          string           `json:"issue"`
	Page           string           `json:"page"`
	URL            string           `json:"URL"`
}

type crossrefAuthor struct {
	Given  string `json:"given"`
	Family string `json:"family"`
}

type crossrefDate struct {
	DateParts [][]int `json:"date-parts"`
}

func crossrefMessageToPaper(m crossrefMessage) *model.Paper {
	p := &model.Paper{
		Title:    firstOrEmpty(m.Title),
		Abstract: stripXMLTags(m.Abstract),
		Venue:    firstOrEmpty(m.ContainerTitle),
		DOI:      m.DOI,
		DocType:  m.Type,
		Volume:   m.Volume,
		Number:   m.Issue,
		Pages:    m.Page,
		URL:      m.URL,
		Source:   "crossref",
	}
	if len(m.Issued.DateParts) > 0 && len(m.Issued.DateParts[0]) > 0 {
		p.Year = m.Issued.DateParts[0][0]
	}
	for _, a := range m.Author {
		if a.Given == "" && a.Family == "" {
			continue
		}
		p.Authors = append(p.Authors, model.Author{Given: a.Given, Family: a.Family})
	}
	p.ID = p.ComputeID()
	return p
}

// firstOrEmpty 取切片首元素（去空白），空切片返回空串。
func firstOrEmpty(ss []string) string {
	if len(ss) == 0 {
		return ""
	}
	return strings.TrimSpace(ss[0])
}

// xmlTagRe 匹配 XML/HTML 标签（CrossRef abstract 可能含 JATS 标签）。
var xmlTagRe = regexp.MustCompile(`<[^>]+>`)

// stripXMLTags 去除 XML 标签后去首尾空白。
func stripXMLTags(s string) string {
	return strings.TrimSpace(xmlTagRe.ReplaceAllString(s, ""))
}

// ---- NCBI eutils esummary ----

type esummaryResponse struct {
	// result 含 "uids"（数组）与各 PMID（对象）两类键，用 RawMessage 按需取目标记录
	Result map[string]json.RawMessage `json:"result"`
}

type esummaryDoc struct {
	Title           string              `json:"title"`
	Authors         []esummaryAuthor    `json:"authors"`
	PubDate         string              `json:"pubdate"`
	FullJournalName string              `json:"fulljournalname"`
	Volume          string              `json:"volume"`
	Issue           string              `json:"issue"`
	Pages           string              `json:"pages"`
	ArticleIDs      []esummaryArticleID `json:"articleids"`
}

type esummaryAuthor struct {
	Name string `json:"name"`
}

type esummaryArticleID struct {
	IDType string `json:"idtype"`
	Value  string `json:"value"`
}

// yearPrefixRe 匹配 pubdate 开头的 4 位年份（如 "2020 Jan 15"）。
var yearPrefixRe = regexp.MustCompile(`^\d{4}`)

func esummaryToPaper(doc esummaryDoc, pmid string) *model.Paper {
	p := &model.Paper{
		Title:   strings.TrimSpace(doc.Title),
		Venue:   strings.TrimSpace(doc.FullJournalName),
		PMID:    pmid,
		DocType: model.DocTypeArticle,
		Volume:  doc.Volume,
		Number:  doc.Issue,
		Pages:   doc.Pages,
		Source:  "pubmed",
	}
	if m := yearPrefixRe.FindString(doc.PubDate); m != "" {
		p.Year, _ = strconv.Atoi(m)
	}
	for _, a := range doc.Authors {
		p.Authors = append(p.Authors, splitFamilyGiven(a.Name))
	}
	for _, id := range doc.ArticleIDs {
		if id.IDType == "doi" && p.DOI == "" {
			p.DOI = strings.TrimSpace(id.Value)
		}
	}
	p.ID = p.ComputeID()
	return p
}

// splitFamilyGiven 将 "Family Given" 形式姓名拆为 Family/Given（esummary 格式）。
func splitFamilyGiven(name string) model.Author {
	name = strings.TrimSpace(name)
	if i := strings.IndexAny(name, " \t"); i > 0 {
		return model.Author{
			Family: strings.TrimSpace(name[:i]),
			Given:  strings.TrimSpace(name[i+1:]),
		}
	}
	return model.Author{Family: name}
}

const metadataUserAgent = "litkit/0.1 (https://github.com/litkit/litkit)"
