package sources

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"litkit/internal/model"
	"litkit/internal/util/httpclient"
	"litkit/internal/util/ratelimit"
)

// BiorxivSource bioRxiv/medRxiv 适配器（FR-SRC-04）。
//
// 端点：https://api.biorxiv.org/details/{server}/{interval}[/{cursor}]
// server ∈ {biorxiv, medrxiv}；interval 仅支持：数字 N（最近 N 篇）、
// YYYY-MM-DD/YYYY-MM-DD、Nd（最近 N 天）。
// bioRxiv API 无原生关键词检索，采用：拉取最近批次 + 客户端关键词过滤。
// ponytail: 仅取首页（cursor 省略，默认 0），适合"返回结果"的验收场景；
// 升级路径：分页拉取多页 + 索引服务，避免单次首页召回不足。
type BiorxivSource struct {
	BaseSource
	Server  string // "biorxiv" 或 "medrxiv"
	BaseURL string // 默认 https://api.biorxiv.org
}

// NewBiorxivSource 创建 bioRxiv/medRxiv 适配器；server 决定源标识与端点路径。
//
// 默认不参与未显式 -s 的检索：官方 API 无关键词端点，只能拉最近批次再本地过滤，
// 召回能力与其余源不可比，默认启用只会带来假 0 命中（见 DefaultSource）。
func NewBiorxivSource(server string, httpClient *httpclient.Client, limiter *ratelimit.Limiter) *BiorxivSource {
	s := &BiorxivSource{
		BaseSource: NewBaseSource(server, httpClient, limiter),
		Server:     server,
		BaseURL:    "https://api.biorxiv.org",
	}
	s.defaultEnabled = false
	return s
}

// biorxivResponse bioRxiv API 响应结构。
type biorxivResponse struct {
	Messages   []biorxivMessage `json:"messages"`
	Collection []biorxivEntry   `json:"collection"`
}

// biorxivMessage API 级状态消息；status 为 "ok" 表示成功。
type biorxivMessage struct {
	Status string `json:"status"`
}

type biorxivEntry struct {
	DOI      string `json:"doi"`
	Title    string `json:"title"`
	Authors  string `json:"authors"`
	Date     string `json:"date"`
	Category string `json:"category"`
	Abstract string `json:"abstract"`
	Server   string `json:"server"`
}

// biorxivFetchWindow 单次拉取窗口：API 无关键词端点，窗口取默认拉取条数
// 而非用户要的条数，否则"最近 5 篇里搜关键词"几乎必然漏检。
const biorxivFetchWindow = 100

// biorxivInterval 计算请求的 interval：至少覆盖默认窗口，用户要更多时按需放大。
func biorxivInterval(maxResults int) int {
	if maxResults > biorxivFetchWindow {
		return maxResults
	}
	return biorxivFetchWindow
}

// Search 拉取 bioRxiv/medRxiv 最近批次并按关键词过滤。
func (b *BiorxivSource) Search(ctx context.Context, query string, opts SearchOptions) ([]model.Paper, error) {
	interval := biorxivInterval(opts.MaxResults)
	u := fmt.Sprintf("%s/details/%s/%d", b.BaseURL, b.Server, interval)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, fmt.Errorf("biorxiv search: %w", err)
	}
	req.Header.Set("User-Agent", defaultUserAgent)

	resp, err := b.Do(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("biorxiv search: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("biorxiv search: HTTP %d", resp.StatusCode)
	}
	data, err := httpclient.ReadAllLimit(resp, maxResponseBytes)
	if err != nil {
		return nil, fmt.Errorf("biorxiv search: read body: %w", err)
	}
	papers, err := parseBiorxivJSON(data, b.Server)
	if err != nil {
		return nil, fmt.Errorf("biorxiv search: %w", err)
	}
	papers = filterBiorxivByKeyword(papers, query)
	if opts.Year != 0 {
		papers = filterByYear(papers, opts.Year)
	} else if opts.Since != 0 {
		papers = filterSince(papers, opts.Since)
	}
	if limit := opts.MaxResults; limit > 0 && len(papers) > limit {
		papers = papers[:limit]
	}
	// 未命中不报错：该 API 无关键词端点，本地过滤为空既可能是真无结果也可能是召回不足，
	// 两者都无法归因到故障。此处报错会让默认检索带 3 号退出码（其他源正常时），
	// 使调用方误以为命令失败。该源已默认不参与检索，需显式 -s 指定。
	return papers, nil
}

// parseBiorxivJSON 解析 bioRxiv JSON 为 []Paper。
// server 决定 Source 字段与 URL 路径前缀。
func parseBiorxivJSON(data []byte, server string) ([]model.Paper, error) {
	var r biorxivResponse
	if err := json.Unmarshal(data, &r); err != nil {
		return nil, fmt.Errorf("parse biorxiv: %w", err)
	}
	// messages[0].status 为 API 级状态（HTTP 200 也可能携带错误，如参数非法）
	if len(r.Messages) == 0 || r.Messages[0].Status != "ok" {
		return nil, fmt.Errorf("biorxiv api status not ok: %v", r.Messages)
	}
	papers := make([]model.Paper, 0, len(r.Collection))
	for _, e := range r.Collection {
		p := model.Paper{
			Title:    strings.TrimSpace(e.Title),
			Abstract: strings.TrimSpace(e.Abstract),
			DOI:      strings.TrimSpace(e.DOI),
			Year:     parseYearFromISO(e.Date),
			Authors:  parseBiorxivAuthors(e.Authors),
			Source:   server,
			DocType:  DocTypePreprint,
		}
		if p.DOI != "" {
			p.URL = fmt.Sprintf("https://www.%s.org/content/%s", server, p.DOI)
		}
		p.ID = p.ComputeID()
		papers = append(papers, p)
	}
	return papers, nil
}

// parseBiorxivAuthors 解析 "Family, Given;Family2, Given2" 为 []Author。
// 真实 API 为 "Last, First" 逗号格式；无逗号时回退按空白拆分（兼容单名/旧格式）。
func parseBiorxivAuthors(s string) []model.Author {
	parts := strings.Split(s, ";")
	out := make([]model.Author, 0, len(parts))
	for _, p := range parts {
		name := strings.TrimSpace(p)
		if name == "" {
			continue
		}
		if i := strings.Index(name, ","); i > 0 {
			out = append(out, model.Author{
				Family: strings.TrimSpace(name[:i]),
				Given:  strings.TrimSpace(name[i+1:]),
			})
			continue
		}
		out = append(out, splitAuthorName(name))
	}
	return out
}

// filterBiorxivByKeyword 按关键词做大小写不敏感匹配（标题或摘要）。
//
// 多词查询按空白拆词，标题/摘要文本同时包含全部词项即命中（AND 语义）；
// 整串当子串匹配会让 "gut microbiota depression" 这类常规检索几乎必然 0 命中。
func filterBiorxivByKeyword(papers []model.Paper, query string) []model.Paper {
	terms := searchTerms(query)
	if len(terms) == 0 {
		return papers
	}
	for i := range terms {
		terms[i] = strings.ToLower(terms[i])
	}
	out := papers[:0]
	for _, p := range papers {
		haystack := strings.ToLower(p.Title) + "\n" + strings.ToLower(p.Abstract)
		matched := true
		for _, t := range terms {
			if !strings.Contains(haystack, t) {
				matched = false
				break
			}
		}
		if matched {
			out = append(out, p)
		}
	}
	return out
}
