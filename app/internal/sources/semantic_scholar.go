package sources

import (
	"context"
	"encoding/json"
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

// SemanticScholarSource Semantic Scholar 适配器（FR-SRC-05）。
//
// 端点：https://api.semanticscholar.org/graph/v1/paper/search（Graph API JSON）。
// 限速：无 key 共享池（保守取 0.5 RPS, burst 1）；有 key 提速。
// 403 自动降级匿名（PRD 4.1 FR-SRC-05、NFR-REL-01）。
type SemanticScholarSource struct {
	BaseSource
	BaseURL string // 默认 https://api.semanticscholar.org/graph/v1/paper/search
	APIKey  string // 可选；有 key 提速（PRD 7.3）
}

// NewSemanticScholarSource 创建 Semantic Scholar 适配器。
func NewSemanticScholarSource(apiKey string, httpClient *httpclient.Client, limiter *ratelimit.Limiter) *SemanticScholarSource {
	return &SemanticScholarSource{
		BaseSource: NewBaseSource("semantic_scholar", httpClient, limiter),
		BaseURL:    "https://api.semanticscholar.org/graph/v1/paper/search",
		APIKey:     apiKey,
	}
}

// semanticScholarResponse Graph API 响应结构。
type semanticScholarResponse struct {
	Total int                    `json:"total"`
	Data  []semanticScholarPaper `json:"data"`
}

type semanticScholarPaper struct {
	PaperID     string                 `json:"paperId"`
	Title       string                 `json:"title"`
	Abstract    *string                `json:"abstract"`
	Year        int                    `json:"year"`
	Venue       string                 `json:"venue"`
	ExternalIDs map[string]interface{} `json:"externalIds"`
	Authors     []struct {
		Name string `json:"name"`
	} `json:"authors"`
	CitationCount int `json:"citationCount"`
}

// Search 调用 Semantic Scholar Graph API。
//
// 403 处理：若已设置 API key，自动降级匿名重试；无 key 时 403 优雅返回空（NFR-REL-02）。
func (s *SemanticScholarSource) Search(ctx context.Context, query string, opts SearchOptions) ([]model.Paper, error) {
	papers, status, err := s.doSearch(ctx, query, opts, true)
	if err != nil {
		return nil, fmt.Errorf("semantic_scholar search: %w", err)
	}
	// 403 + 有 key → 降级匿名重试
	if status == http.StatusForbidden && s.APIKey != "" {
		papers, _, err = s.doSearch(ctx, query, opts, false)
		if err != nil {
			return nil, fmt.Errorf("semantic_scholar search: %w", err)
		}
		return papers, nil
	}
	// 403 + 无 key → 优雅返回空（共享池限速）
	if status == http.StatusForbidden {
		return []model.Paper{}, nil
	}
	// 其他非 2xx（429/500 等）→ 报错，归入单源失败记录（与 arxiv/pubmed 行为一致）
	if status != http.StatusOK {
		return nil, fmt.Errorf("semantic_scholar search: HTTP %d", status)
	}
	return papers, nil
}

// doSearch 执行一次请求。withKey 控制是否带 API key 头。
// 返回 papers、HTTP 状态码（用于 403 降级判断）、error。
func (s *SemanticScholarSource) doSearch(ctx context.Context, query string, opts SearchOptions, withKey bool) ([]model.Paper, int, error) {
	u, err := s.buildURL(query, opts)
	if err != nil {
		return nil, 0, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("User-Agent", defaultUserAgent)
	if withKey && s.APIKey != "" {
		req.Header.Set("x-api-key", s.APIKey)
	}

	resp, err := s.Do(ctx, req)
	if err != nil {
		return nil, 0, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		// 403 等非 2xx 返回状态码，调用方判断是否降级
		return nil, resp.StatusCode, nil
	}
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, resp.StatusCode, fmt.Errorf("read body: %w", err)
	}
	papers, err := parseSemanticScholarJSON(data)
	if err != nil {
		return nil, resp.StatusCode, err
	}
	return papers, resp.StatusCode, nil
}

func (s *SemanticScholarSource) buildURL(query string, opts SearchOptions) (string, error) {
	q := url.Values{}
	q.Set("query", query)
	q.Set("limit", strconv.Itoa(ensureMax(opts.MaxResults, defaultMaxResults)))
	q.Set("fields", "title,abstract,year,venue,externalIds,authors,citationCount")
	if opts.Year != 0 {
		// S2 年份过滤：YYYY-YYYY 范围（Year 优先于 Since）
		q.Set("year", fmt.Sprintf("%d-%d", opts.Year, opts.Year))
	} else if opts.Since != 0 {
		// S2 支持开区间写法：YYYY- 表示 Since 年及以后
		q.Set("year", fmt.Sprintf("%d-", opts.Since))
	}
	return s.BaseURL + "?" + q.Encode(), nil
}

// parseSemanticScholarJSON 解析 Graph API 响应为 []Paper。
func parseSemanticScholarJSON(data []byte) ([]model.Paper, error) {
	var r semanticScholarResponse
	if err := json.Unmarshal(data, &r); err != nil {
		return nil, fmt.Errorf("parse semantic_scholar: %w", err)
	}
	papers := make([]model.Paper, 0, len(r.Data))
	for _, sp := range r.Data {
		p := model.Paper{
			Title:     strings.TrimSpace(sp.Title),
			Year:      sp.Year,
			Venue:     strings.TrimSpace(sp.Venue),
			Authors:   semanticScholarAuthorsToModel(sp.Authors),
			Citations: sp.CitationCount,
			Source:    "semantic_scholar",
			DocType:   DocTypeArticle,
		}
		if sp.Abstract != nil {
			p.Abstract = strings.TrimSpace(*sp.Abstract)
		}
		// externalIds 是动态对象，按 key 取
		if v, ok := sp.ExternalIDs["DOI"].(string); ok {
			p.DOI = strings.TrimSpace(v)
		}
		if v, ok := sp.ExternalIDs["PubMed"].(string); ok {
			p.PMID = strings.TrimSpace(v)
		} else if v, ok := sp.ExternalIDs["PubMed"].(float64); ok {
			p.PMID = strconv.Itoa(int(v))
		}
		if v, ok := sp.ExternalIDs["ArXiv"].(string); ok {
			p.ArXivID = strings.TrimSpace(v)
		}
		if p.DOI != "" {
			p.URL = "https://doi.org/" + p.DOI
		} else if sp.PaperID != "" {
			p.URL = "https://www.semanticscholar.org/paper/" + sp.PaperID
		}
		p.ID = p.ComputeID()
		papers = append(papers, p)
	}
	return papers, nil
}

// semanticScholarAuthorsToModel 将 S2 authors 列表（含 name 字段）转为 []Author。
func semanticScholarAuthorsToModel(authors []struct {
	Name string `json:"name"`
}) []model.Author {
	out := make([]model.Author, 0, len(authors))
	for _, a := range authors {
		name := strings.TrimSpace(a.Name)
		if name == "" {
			continue
		}
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
