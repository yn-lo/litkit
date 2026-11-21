package sources

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"strings"

	"litkit/internal/model"
	"litkit/internal/util/httpclient"
	"litkit/internal/util/ratelimit"
)

// OpenAlexSource OpenAlex 适配器（FR-SRC-06）。
//
// 端点：https://api.openalex.org/works（JSON）。
// 限速：10 req/s（保守取 5 RPS, burst 5，NFR-PERF-04）。
// 摘要以倒排索引（abstract_inverted_index）形式提供，需客户端重建。
// 按关键词与 DOI 均可查（PRD 4.1 FR-SRC-06）。
type OpenAlexSource struct {
	BaseSource
	BaseURL string // 默认 https://api.openalex.org/works
}

// NewOpenAlexSource 创建 OpenAlex 适配器。
func NewOpenAlexSource(httpClient *httpclient.Client, limiter *ratelimit.Limiter) *OpenAlexSource {
	return &OpenAlexSource{
		BaseSource: NewBaseSource("openalex", httpClient, limiter),
		BaseURL:    "https://api.openalex.org/works",
	}
}

// openAlexResponse OpenAlex API 响应结构。
type openAlexResponse struct {
	Results []openAlexWork `json:"results"`
}

type openAlexWork struct {
	ID                    string                   `json:"id"`  // https://openalex.org/W123
	DOI                   *string                  `json:"doi"` // https://doi.org/10.x 或 null
	Title                 string                   `json:"title"`
	AbstractInvertedIndex map[string][]int         `json:"abstract_inverted_index"`
	PublicationYear       int                      `json:"publication_year"`
	Type                  string                   `json:"type"`
	CitedByCount          int                      `json:"cited_by_count"`
	Authorships           []openAlexAuthorship     `json:"authorships"`
	PrimaryLocation       *openAlexPrimaryLocation `json:"primary_location"`
}

type openAlexAuthorship struct {
	Author struct {
		DisplayName string `json:"display_name"`
	} `json:"author"`
}

type openAlexPrimaryLocation struct {
	Source *struct {
		DisplayName string `json:"display_name"`
	} `json:"source"`
}

// Search 调用 OpenAlex API 并解析 JSON。
func (o *OpenAlexSource) Search(ctx context.Context, query string, opts SearchOptions) ([]model.Paper, error) {
	return o.search(ctx, "openalex", mailUserAgent,
		func() (string, error) { return o.buildURL(query, opts) }, parseOpenAlexJSON)
}

// openAlexTiabFilter 返回题录+摘要检索的 filter 条件（tiab 模式）。
//
// OpenAlex 的 search= 是全字段检索（含全文/机构等），tiab 应用 title_and_abstract.search
// 下推，提高精度；filter 值以逗号分隔，查询含逗号时只能回落 search=（否则会被拆成两个 filter）。
// 返回空串表示应使用 search=。
func openAlexTiabFilter(query, mode string) string {
	if mode == modeFull || strings.ContainsAny(query, ",") {
		return ""
	}
	return "title_and_abstract.search:" + query
}

func (o *OpenAlexSource) buildURL(query string, opts SearchOptions) (string, error) {
	q := url.Values{}
	parts := make([]string, 0, len(q)+1) // 检索条件 + 年份 + 摘要过滤
	tiab := openAlexTiabFilter(query, opts.Mode)
	if tiab != "" {
		parts = append(parts, tiab)
	} else {
		q.Set("search", query)
	}
	if opts.Year != 0 {
		// OpenAlex 原生年份过滤（filter 参数）
		parts = append(parts, fmt.Sprintf("publication_year:%d", opts.Year))
	} else if opts.Since != 0 {
		// 最近 N 年范围过滤（FR-SEARCH-13）
		parts = append(parts, fmt.Sprintf("from_publication_date:%d-01-01", opts.Since))
	}
	if opts.RequireAbstract {
		// 无摘要过滤下推到服务端：否则先按 per-page 截断再本地过滤，容易整流为空
		parts = append(parts, "has_abstract:true")
	}
	if len(parts) > 0 {
		q.Set("filter", strings.Join(parts, ","))
	}
	q.Set("per-page", strconv.Itoa(ensureMax(opts.MaxResults, defaultMaxResults)))
	return o.BaseURL + "?" + q.Encode(), nil
}

// ParseOpenAlexWork 解析单个 work（如 /works/https://doi.org/{doi}）为 Paper。
// 供 core 的摘要兜底复用（Crossref 记录常缺摘要）。
func ParseOpenAlexWork(data []byte) (model.Paper, error) {
	var w openAlexWork
	if err := json.Unmarshal(data, &w); err != nil {
		return model.Paper{}, fmt.Errorf("parse openalex work: %w", err)
	}
	return openAlexWorkToPaper(w), nil
}

// parseOpenAlexJSON 解析 OpenAlex JSON 为 []Paper。
func parseOpenAlexJSON(data []byte) ([]model.Paper, error) {
	var r openAlexResponse
	if err := json.Unmarshal(data, &r); err != nil {
		return nil, fmt.Errorf("parse openalex: %w", err)
	}
	papers := make([]model.Paper, 0, len(r.Results))
	for _, w := range r.Results {
		papers = append(papers, openAlexWorkToPaper(w))
	}
	return papers, nil
}

// openAlexWorkToPaper 将单条 OpenAlex work 转为 Paper（检索与摘要兜底共用）。
func openAlexWorkToPaper(w openAlexWork) model.Paper {
	p := model.Paper{
		Title:     strings.TrimSpace(w.Title),
		Abstract:  reconstructOpenAlexAbstract(w.AbstractInvertedIndex),
		Year:      w.PublicationYear,
		DocType:   normalizeDocType(w.Type),
		Citations: w.CitedByCount,
		Authors:   openAlexAuthorsToModel(w.Authorships),
		Source:    "openalex",
	}
	if w.DOI != nil {
		p.DOI = strings.TrimPrefix(*w.DOI, "https://doi.org/")
		p.DOI = strings.TrimPrefix(p.DOI, "http://doi.org/")
	}
	if w.PrimaryLocation != nil && w.PrimaryLocation.Source != nil {
		p.Venue = strings.TrimSpace(w.PrimaryLocation.Source.DisplayName)
	}
	if p.DOI != "" {
		p.URL = "https://doi.org/" + p.DOI
	} else if w.ID != "" {
		p.URL = w.ID
	}
	p.ID = p.ComputeID()
	return p
}

// maxAbstractPosition 摘要倒排位置上限：防御异常/恶意响应触发超大切片分配。
const maxAbstractPosition = 10000

// reconstructOpenAlexAbstract 由倒排索引重建摘要原文。
//
// 输入：{word: [positions]}；输出：按位置拼接的字符串。
// ponytail: 用最大位置分配切片，O(N) 重建，适合 OpenAlex 摘要规模（< 1k 词）；
// 位置超过 maxAbstractPosition 的项直接忽略。
func reconstructOpenAlexAbstract(idx map[string][]int) string {
	if len(idx) == 0 {
		return ""
	}
	maxPos := -1
	for _, positions := range idx {
		for _, p := range positions {
			if p >= 0 && p <= maxAbstractPosition && p > maxPos {
				maxPos = p
			}
		}
	}
	if maxPos < 0 {
		return ""
	}
	words := make([]string, maxPos+1)
	for word, positions := range idx {
		for _, p := range positions {
			if p >= 0 && p <= maxPos {
				words[p] = word
			}
		}
	}
	return strings.Join(words, " ")
}

// openAlexAuthorsToModel 将 Authorship 列表转为 []Author（display_name 按首空格拆分）。
func openAlexAuthorsToModel(authorships []openAlexAuthorship) []model.Author {
	out := make([]model.Author, 0, len(authorships))
	for _, a := range authorships {
		if name := strings.TrimSpace(a.Author.DisplayName); name != "" {
			out = append(out, splitAuthorName(name))
		}
	}
	return out
}

// normalizeDocType 归一化 OpenAlex 文献类型到 GB/T 7714 取值。
// ponytail: 仅处理常见类型，未识别留空（不阻塞检索）。
func normalizeDocType(t string) string {
	switch strings.ToLower(strings.TrimSpace(t)) {
	case DocTypeArticle:
		return DocTypeArticle
	case DocTypePreprint:
		return DocTypePreprint
	case "dataset":
		return "dataset"
	case "book-chapter", "book":
		return "book"
	case "dissertation":
		return "dissertation"
	default:
		return strings.ToLower(strings.TrimSpace(t))
	}
}
