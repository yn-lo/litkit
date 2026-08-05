package sources

import (
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"litkit/internal/model"
	"litkit/internal/util/httpclient"
	"litkit/internal/util/ratelimit"
)

// ArxivSource arXiv 适配器（FR-SRC-02）。
//
// 端点：http://export.arxiv.org/api/query（Atom Feed）。
// 限速：官方 1 req/3s（保守取 0.33 RPS, burst 1，NFR-PERF-04）。
type ArxivSource struct {
	BaseSource
	// BaseURL 可被测试覆盖；默认 http://export.arxiv.org/api/query
	BaseURL string
}

// NewArxivSource 创建 arXiv 适配器。
func NewArxivSource(httpClient *httpclient.Client, limiter *ratelimit.Limiter) *ArxivSource {
	return &ArxivSource{
		BaseSource: NewBaseSource("arxiv", httpClient, limiter),
		BaseURL:    "http://export.arxiv.org/api/query",
	}
}

// atomFeed arXiv Atom Feed 解析结构（仅取必要字段）。
type atomFeed struct {
	XMLName xml.Name    `xml:"feed"`
	Entries []atomEntry `xml:"entry"`
}

type atomEntry struct {
	ID         string       `xml:"id"`
	Title      string       `xml:"title"`
	Summary    string       `xml:"summary"`
	Authors    []atomAuthor `xml:"author"`
	Published  string       `xml:"published"`
	Links      []atomLink   `xml:"link"`
	DOI        string       `xml:"http://arxiv.org/schemas/atom doi"`
	JournalRef string       `xml:"http://arxiv.org/schemas/atom journal_ref"`
}

type atomAuthor struct {
	Name string `xml:"name"`
}

type atomLink struct {
	Href string `xml:"href,attr"`
	Rel  string `xml:"rel,attr"`
	Type string `xml:"type,attr"`
}

// Search 调用 arXiv API 并解析 Atom Feed。
func (a *ArxivSource) Search(ctx context.Context, query string, opts SearchOptions) ([]model.Paper, error) {
	u, err := a.buildURL(query, opts)
	if err != nil {
		return nil, fmt.Errorf("arxiv search: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, fmt.Errorf("arxiv search: %w", err)
	}
	req.Header.Set("User-Agent", defaultUserAgent)

	resp, err := a.Do(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("arxiv search: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("arxiv search: HTTP %d", resp.StatusCode)
	}
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("arxiv search: read body: %w", err)
	}
	papers, err := ParseArxivAtom(data)
	if err != nil {
		return nil, fmt.Errorf("arxiv search: %w", err)
	}
	if opts.Year != 0 {
		papers = filterByYear(papers, opts.Year)
	} else if opts.Since != 0 {
		papers = filterSince(papers, opts.Since)
	}
	return papers, nil
}

// buildURL 构造 arXiv API URL。
//
// 检索等级（FR-SEARCH-12）：
//   - tiab（默认）：ti:<q> OR abs:<q>，仅题目+摘要；引号包裹支持多词短语
//   - full：all:<q>，全字段（含全文，误检率较高，作高级选项）
func (a *ArxivSource) buildURL(query string, opts SearchOptions) (string, error) {
	q := url.Values{}
	q.Set("search_query", arxivSearchQuery(query, opts.Mode))
	q.Set("start", "0")
	q.Set("max_results", strconv.Itoa(ensureMax(opts.MaxResults, defaultMaxResults)))
	q.Set("sortBy", "relevance")
	q.Set("sortOrder", "descending")
	return a.BaseURL + "?" + q.Encode(), nil
}

// arxivSearchQuery 按检索等级构造 arXiv 检索式。
func arxivSearchQuery(query, mode string) string {
	if mode == modeFull {
		return "all:" + query
	}
	return `ti:"` + query + `" OR abs:"` + query + `"`
}

// ParseArxivAtom 解析 arXiv Atom Feed 字节为 []Paper。
//
// 纯函数，便于单元测试；同时供 core/metadata 反查复用。
func ParseArxivAtom(data []byte) ([]model.Paper, error) {
	var feed atomFeed
	if err := xml.Unmarshal(data, &feed); err != nil {
		return nil, fmt.Errorf("parse atom: %w", err)
	}
	papers := make([]model.Paper, 0, len(feed.Entries))
	for _, e := range feed.Entries {
		p := model.Paper{
			Title:    strings.TrimSpace(e.Title),
			Abstract: strings.TrimSpace(e.Summary),
			DOI:      strings.TrimSpace(e.DOI),
			Venue:    strings.TrimSpace(e.JournalRef),
			Year:     parseYearFromISO(e.Published),
			Authors:  atomAuthorsToModel(e.Authors),
			Source:   "arxiv",
			DocType:  DocTypePreprint,
		}
		p.ArXivID = ExtractArxivID(e.ID)
		p.URL = pickArxivURL(e)
		p.ID = p.ComputeID()
		papers = append(papers, p)
	}
	return papers, nil
}

// ExtractArxivID 从 http://arxiv.org/abs/2301.00001v1 提取 2301.00001（去版本号）。
func ExtractArxivID(idURL string) string {
	// 兼容 http(s) 与 abs/pdf 两种路径
	idx := strings.Index(idURL, "/abs/")
	if idx < 0 {
		idx = strings.Index(idURL, "/pdf/")
	}
	if idx < 0 {
		return strings.TrimSpace(idURL)
	}
	s := idURL[idx+5:] // 跳过 "/abs/" 或 "/pdf/"
	// 去除版本号 vN：用 LastIndex 定位最后一个 v，旧式归档名本身可能含 'v'
	// （如 solv-int/9612001v2），且仅当 v 后全为数字时才剥离
	if i := strings.LastIndex(s, "v"); i > 0 {
		rest := s[i+1:]
		if _, err := strconv.Atoi(rest); err == nil {
			s = s[:i]
		}
	}
	return strings.TrimSuffix(s, ".pdf")
}

// pickArxivURL 选择 entry 的 alternate 链接（HTML 页面），缺失回退 id。
func pickArxivURL(e atomEntry) string {
	for _, l := range e.Links {
		if l.Rel == "alternate" {
			return l.Href
		}
	}
	return e.ID
}

// atomAuthorsToModel 将 arXiv 单串姓名拆为 given/family（按首空格切分）。
func atomAuthorsToModel(authors []atomAuthor) []model.Author {
	out := make([]model.Author, 0, len(authors))
	for _, a := range authors {
		name := strings.TrimSpace(a.Name)
		if name == "" {
			continue
		}
		// 中英文混排：首空格前为 given，后为 family
		// 对纯中文名（无空格）整体入 Family
		if i := strings.IndexAny(name, " \t"); i > 0 {
			out = append(out, model.Author{
				Given:  strings.TrimSpace(name[:i]),
				Family: strings.TrimSpace(name[i+1:]),
			})
		} else {
			out = append(out, model.Author{Family: name})
		}
	}
	return out
}

// parseYearFromISO 从 ISO8601 日期串提取年份。
func parseYearFromISO(s string) int {
	const minYearLen = 4 // "2024" 至少 4 字符
	if len(s) < minYearLen {
		return 0
	}
	y, err := strconv.Atoi(s[:minYearLen])
	if err != nil {
		return 0
	}
	return y
}

// ensureMax 返回 limit；若 ≤0 用 def。
func ensureMax(limit, def int) int {
	if limit <= 0 {
		return def
	}
	return limit
}

// filterByYear 客户端按年份过滤（arXiv API 不直接支持）。
func filterByYear(papers []model.Paper, year int) []model.Paper {
	out := papers[:0]
	for _, p := range papers {
		if p.Year == year {
			out = append(out, p)
		}
	}
	return out
}

// filterSince 客户端按起始年份（含）过滤。
func filterSince(papers []model.Paper, since int) []model.Paper {
	out := papers[:0]
	for _, p := range papers {
		if p.Year >= since {
			out = append(out, p)
		}
	}
	return out
}
