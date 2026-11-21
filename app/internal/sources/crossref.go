package sources

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"regexp"
	"strconv"
	"strings"

	"litkit/internal/model"
	"litkit/internal/util/httpclient"
	"litkit/internal/util/ratelimit"
)

// CrossrefSource Crossref 检索适配器。
//
// 端点：https://api.crossref.org/works（JSON，/works 检索）。
// 与 core 的 DOI 反查共用同一上游，但这里是独立检索源：用中文关键词 query
// 可命中已注册 DOI 的中文学报/期刊，部分条目带中文摘要（JATS 格式，需去标签）。
//
// 中文语料覆盖动机（FR-SRC）：知网/万方/维普无公共 API；Crossref 是目前少数
// 免费的、结构化的中文文献入口之一。
type CrossrefSource struct {
	BaseSource
	BaseURL string // 默认 https://api.crossref.org/works
}

// NewCrossrefSource 创建 Crossref 检索适配器。
func NewCrossrefSource(httpClient *httpclient.Client, limiter *ratelimit.Limiter) *CrossrefSource {
	return &CrossrefSource{
		BaseSource: NewBaseSource("crossref", httpClient, limiter),
		BaseURL:    "https://api.crossref.org/works",
	}
}

// crossrefSearchResponse Crossref /works 检索响应。
type crossrefSearchResponse struct {
	Message struct {
		Items []crossrefWork `json:"items"`
	} `json:"message"`
}

type crossrefWork struct {
	Title          []string           `json:"title"`
	Author         []crossrefWorkAuth `json:"author"`
	Abstract       string             `json:"abstract"`
	Issued         crossrefWorkDate   `json:"issued"`
	ContainerTitle []string           `json:"container-title"`
	DOI            string             `json:"DOI"`
	Type           string             `json:"type"`
	Volume         string             `json:"volume"`
	Issue          string             `json:"issue"`
	Page           string             `json:"page"`
	URL            string             `json:"URL"`
	UpdatedBy      []CrossrefUpdate   `json:"updated-by"`
}

// CrossrefUpdate 一条更新声明（updated-by）：撤稿/更正等。
type CrossrefUpdate struct {
	DOI    string `json:"DOI"`
	Type   string `json:"type"`   // retraction | correction | ...
	Source string `json:"source"` // publisher | retraction-watch
	Label  string `json:"label"`  // Retraction / Correction / ...
}

type crossrefWorkAuth struct {
	Given  string `json:"given"`
	Family string `json:"family"`
}

type crossrefWorkDate struct {
	DateParts [][]int `json:"date-parts"`
}

// Search 调用 Crossref /works 检索并解析 JSON。
func (c *CrossrefSource) Search(ctx context.Context, query string, opts SearchOptions) ([]model.Paper, error) {
	return c.search(ctx, "crossref", mailUserAgent,
		func() (string, error) { return c.buildURL(query, opts) }, parseCrossrefSearch)
}

func (c *CrossrefSource) buildURL(query string, opts SearchOptions) (string, error) {
	q := url.Values{}
	q.Set("query", query)
	q.Set("rows", strconv.Itoa(ensureMax(opts.MaxResults, defaultMaxResults)))
	filters := make([]string, 0, 2)
	if opts.Year != 0 {
		filters = append(filters, crossrefYearFilter(opts.Year, opts.Year))
	} else if opts.Since != 0 {
		filters = append(filters, crossrefYearFilter(opts.Since, 0))
	}
	if opts.RequireAbstract {
		// 无摘要过滤下推到服务端：Crossref 大量条目无摘要，先截断再本地过滤会整源脱落
		filters = append(filters, "has-abstract:true")
	}
	if len(filters) > 0 {
		q.Set("filter", strings.Join(filters, ","))
	}
	return c.BaseURL + "?" + q.Encode(), nil
}

// crossrefYearFilter 构造 Crossref filter 年份区间。
// untilYear=0 表示不设上界（自 since 起至今）。
func crossrefYearFilter(from, until int) string {
	s := fmt.Sprintf("from-pub-date:%04d-01-01", from)
	if until != 0 {
		s += ",until-pub-date:" + fmt.Sprintf("%04d-12-31", until)
	}
	return s
}

// parseCrossrefSearch 解析 /works 检索 JSON 为 []Paper。
func parseCrossrefSearch(data []byte) ([]model.Paper, error) {
	var r crossrefSearchResponse
	if err := json.Unmarshal(data, &r); err != nil {
		return nil, err
	}
	papers := make([]model.Paper, 0, len(r.Message.Items))
	for _, w := range r.Message.Items {
		papers = append(papers, crossrefWorkToPaper(w))
	}
	return papers, nil
}

// crossrefWorkToPaper 将单条 Crossref work 转为 Paper（检索与 DOI/标题反查共用，
// 保证 DocType 规范化、DOI URL 回退等口径一致）。
func crossrefWorkToPaper(w crossrefWork) model.Paper {
	p := model.Paper{
		Title:    firstString(w.Title),
		Abstract: stripJATSTags(w.Abstract),
		Venue:    firstString(w.ContainerTitle),
		DOI:      w.DOI,
		DocType:  strings.ToLower(strings.TrimSpace(w.Type)),
		Volume:   w.Volume,
		Number:   w.Issue,
		Pages:    w.Page,
		URL:      w.URL,
		Source:   "crossref",
	}
	if len(w.Issued.DateParts) > 0 && len(w.Issued.DateParts[0]) > 0 {
		p.Year = w.Issued.DateParts[0][0]
	}
	for _, a := range w.Author {
		if a.Given == "" && a.Family == "" {
			continue
		}
		p.Authors = append(p.Authors, model.Author{Given: a.Given, Family: a.Family})
	}
	if p.DOI != "" && p.URL == "" {
		p.URL = "https://doi.org/" + p.DOI
	}
	p.ID = p.ComputeID()
	return p
}

// ParseCrossrefWork 解析 /works/{doi} 响应（{"message":{...}}）为 Paper + 更新声明。
// 供 core 的标识符反查复用，避免服务层重复实现平台解析。
func ParseCrossrefWork(data []byte) (model.Paper, []CrossrefUpdate, error) {
	var r struct {
		Message crossrefWork `json:"message"`
	}
	if err := json.Unmarshal(data, &r); err != nil {
		return model.Paper{}, nil, err
	}
	return crossrefWorkToPaper(r.Message), r.Message.UpdatedBy, nil
}

// ParseCrossrefSearchFirst 解析 /works 检索响应并返回首条；无结果返回 ok=false。
func ParseCrossrefSearchFirst(data []byte) (model.Paper, bool, error) {
	var r crossrefSearchResponse
	if err := json.Unmarshal(data, &r); err != nil {
		return model.Paper{}, false, err
	}
	if len(r.Message.Items) == 0 {
		return model.Paper{}, false, nil
	}
	return crossrefWorkToPaper(r.Message.Items[0]), true, nil
}

func firstString(ss []string) string {
	for _, s := range ss {
		if s = strings.TrimSpace(s); s != "" {
			return s
		}
	}
	return ""
}

// stripJATSTags 去除摘要中的 JATS/HTML 标签（如 <jats:p>、<italic>）。
func stripJATSTags(s string) string {
	s = strings.TrimSpace(xmlTagReJATS.ReplaceAllString(s, ""))
	return collapseSpace(s)
}

var xmlTagReJATS = regexp.MustCompile(`<[^>]+>`)

// collapseSpace 将连续空白（含换行）折叠为单个空格；CJK 无空白，字号不受影响。
func collapseSpace(s string) string {
	return strings.Join(strings.Fields(s), " ")
}
