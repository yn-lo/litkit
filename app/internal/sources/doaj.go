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

// DoajSource DOAJ 检索适配器。
//
// 端点：https://doaj.org/api/v2/search/articles/{query}。
// 检索词置于 URL 路径（DOAJ 约定），pageSize 控制条数；query 支持中文。
// DOAJ 收录的符合质量标准的开放获取期刊中含中文 OA 期刊，记录带
// journal.language=["ZH"]、country，多数携带中文标题与中文摘要——
// 是中文语料免费检索的可靠入口。
type DoajSource struct {
	BaseSource
	BaseURL string // 默认 https://doaj.org/api/v2
}

// NewDoajSource 创建 DOAJ 检索适配器。
func NewDoajSource(httpClient *httpclient.Client, limiter *ratelimit.Limiter) *DoajSource {
	return &DoajSource{
		BaseSource: NewBaseSource("doaj", httpClient, limiter),
		BaseURL:    "https://doaj.org/api/v2",
	}
}

// doajResponse DOAJ 检索响应结构。
type doajResponse struct {
	Results []doajItem `json:"results"`
}

type doajItem struct {
	BibJSON doajBibJSON `json:"bibjson"`
}

type doajBibJSON struct {
	Title      string       `json:"title"`
	Abstract   string       `json:"abstract"`
	Year       any          `json:"year"` // DOAJ 返回可能是数字或字符串（"2021"）
	Author     []doajAuthor `json:"author"`
	Journal    doajJournal  `json:"journal"`
	Volume     string       `json:"volume"`
	Number     string       `json:"number"`
	StartPage  string       `json:"start_page"`
	EndPage    string       `json:"end_page"`
	Identifier []struct {
		ID   string `json:"id"`
		Type string `json:"type"`
	} `json:"identifier"`
	Link []struct {
		URL  string `json:"url"`
		Type string `json:"type"`
	} `json:"link"`
}

type doajAuthor struct {
	Name string `json:"name"`
}

type doajJournal struct {
	Title string `json:"title"`
}

// Search 调用 DOAJ 检索并解析 JSON。
func (d *DoajSource) Search(ctx context.Context, query string, opts SearchOptions) ([]model.Paper, error) {
	return d.search(ctx, "doaj", defaultUserAgent,
		func() (string, error) { return d.buildURL(query, opts), nil }, parseDoajJSON)
}

func (d *DoajSource) buildURL(query string, opts SearchOptions) string {
	q := url.PathEscape(query)
	if opts.Year != 0 {
		q += url.PathEscape(" AND year:" + strconv.Itoa(opts.Year))
	} else if opts.Since != 0 {
		q += url.PathEscape(fmt.Sprintf(" AND year:[%d TO *]", opts.Since))
	}
	return fmt.Sprintf("%s/search/articles/%s?pageSize=%d",
		d.BaseURL, q, ensureMax(opts.MaxResults, defaultMaxResults))
}

// doajYear 将 DOAJ 的年份（数字或字符串）转为 int；无法解析按 0。
func doajYear(v any) int {
	switch t := v.(type) {
	case int:
		return t
	case float64:
		return int(t)
	case string:
		if n, err := strconv.Atoi(strings.TrimSpace(t)); err == nil {
			return n
		}
	}
	return 0
}

// parseDoajJSON 解析 DOAJ JSON 为 []Paper。
func parseDoajJSON(data []byte) ([]model.Paper, error) {
	var r doajResponse
	if err := json.Unmarshal(data, &r); err != nil {
		return nil, err
	}
	papers := make([]model.Paper, 0, len(r.Results))
	for _, it := range r.Results {
		b := it.BibJSON
		p := model.Paper{
			Title:    strings.TrimSpace(b.Title),
			Abstract: collapseSpace(b.Abstract),
			Year:     doajYear(b.Year),
			Venue:    strings.TrimSpace(b.Journal.Title),
			Volume:   b.Volume,
			Number:   b.Number,
			DocType:  DocTypeArticle,
			Source:   "doaj",
		}
		if b.StartPage != "" || b.EndPage != "" {
			p.Pages = strings.TrimSpace(b.StartPage + "-" + b.EndPage)
		}
		for _, id := range b.Identifier {
			if id.Type == "doi" {
				p.DOI = id.ID
				break
			}
		}
		for _, l := range b.Link {
			if l.URL != "" {
				p.URL = l.URL
				break
			}
		}
		for _, a := range b.Author {
			name := strings.TrimSpace(a.Name)
			if name == "" {
				continue
			}
			p.Authors = append(p.Authors, splitAuthorName(name))
		}
		if p.DOI != "" && p.URL == "" {
			p.URL = "https://doi.org/" + p.DOI
		}
		p.ID = p.ComputeID()
		papers = append(papers, p)
	}
	return papers, nil
}
