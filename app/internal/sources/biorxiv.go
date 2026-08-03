package sources

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
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
func NewBiorxivSource(server string, httpClient *httpclient.Client, limiter *ratelimit.Limiter) *BiorxivSource {
	return &BiorxivSource{
		BaseSource: NewBaseSource(server, httpClient, limiter),
		Server:     server,
		BaseURL:    "https://api.biorxiv.org",
	}
}

// biorxivDefaultResults MaxResults 未指定时的默认拉取条数（interval 取数字 N 形式）。
const biorxivDefaultResults = 100

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

// Search 拉取 bioRxiv/medRxiv 最近批次并按关键词过滤。
func (b *BiorxivSource) Search(ctx context.Context, query string, opts SearchOptions) ([]model.Paper, error) {
	// interval 用数字 N（最近 N 篇）；MaxResults=0 回退 100，而非不截断
	u := fmt.Sprintf("%s/details/%s/%d", b.BaseURL, b.Server, ensureMax(opts.MaxResults, biorxivDefaultResults))
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
	data, err := io.ReadAll(resp.Body)
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

// parseBiorxivAuthors 解析 "First Last;First2 Last2" 为 []Author。
func parseBiorxivAuthors(s string) []model.Author {
	parts := strings.Split(s, ";")
	out := make([]model.Author, 0, len(parts))
	for _, p := range parts {
		name := strings.TrimSpace(p)
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

// filterBiorxivByKeyword 按关键词做大小写不敏感子串匹配（标题或摘要）。
func filterBiorxivByKeyword(papers []model.Paper, query string) []model.Paper {
	q := strings.ToLower(strings.TrimSpace(query))
	if q == "" {
		return papers
	}
	out := papers[:0]
	for _, p := range papers {
		if strings.Contains(strings.ToLower(p.Title), q) ||
			strings.Contains(strings.ToLower(p.Abstract), q) {
			out = append(out, p)
		}
	}
	return out
}
