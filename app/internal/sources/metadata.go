// metadata.go 实现按标识符反查论文元数据（FR-REF-02），适配层职责。
//
// 反查源：
//   - doi → CrossRef /works/{doi}（摘要缺失时回落 OpenAlex 补齐）
//   - pmid → NCBI eutils efetch（XML，含摘要与卷期页）
//   - arxiv → arXiv API（Atom XML）
//   - title → CrossRef works 检索（rows=1 取首条）
//
// 平台端点、请求构造、响应解析与撤稿判定全部收敛在适配层；
// 服务层 core 仅通过 MetadataFetcher 调用，新增反查源不修改核心层。
//
// 未命中（404 / 空结果）返回 (nil, nil)；网络或解析错误返回 error。
// 说明：本文件注释不构成 package doc（package doc 见各包 doc/主文件）。

package sources

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"unicode"

	"litkit/internal/model"
	"litkit/internal/util/httpclient"
)

// MetadataFetcher 按标识符反查论文元数据。
type MetadataFetcher struct {
	client *httpclient.Client

	CrossrefBase string // 测试可覆盖
	EutilsBase   string
	ArxivBase    string
	OpenAlexBase string // 摘要兜底（Crossref 常缺摘要）
}

// NewMetadataFetcher 创建反查器。
func NewMetadataFetcher(client *httpclient.Client) *MetadataFetcher {
	return &MetadataFetcher{
		client:       client,
		CrossrefBase: "https://api.crossref.org",
		EutilsBase:   "https://eutils.ncbi.nlm.nih.gov",
		ArxivBase:    "https://export.arxiv.org",
		OpenAlexBase: "https://api.openalex.org",
	}
}

// metadataUserAgent 元数据反查 User-Agent（Crossref polite pool 要求带联系方式）。
const metadataUserAgent = "litkit/0.1 (https://github.com/litkit/litkit)"

// Fetch 按 idType（doi|pmid|arxiv|title）与 identifier 反查论文。
//
// 未命中返回 (nil, nil)；网络/解析错误返回 error；未知 idType 返回错误。
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
	data, err := httpclient.ReadAllLimit(resp, maxResponseBytes)
	if err != nil {
		return nil, fmt.Errorf("metadata %s: 读取响应: %w", idType, err)
	}
	return data, nil
}

func (f *MetadataFetcher) fetchDOI(ctx context.Context, doi string) (*model.Paper, error) {
	u := f.CrossrefBase + "/works/" + url.PathEscape(doi)
	data, err := f.get(ctx, "doi", u)
	if err != nil || data == nil {
		return nil, err
	}
	p, _, err := ParseCrossrefWork(data)
	if err != nil {
		return nil, fmt.Errorf("metadata doi: 解析 CrossRef 响应: %w", err)
	}
	f.fillAbstract(ctx, &p)
	return &p, nil
}

// fillAbstract 摘要兜底：Crossref 部分出版商不上传摘要，而摘要工作流（入库/引用评分）必需，
// 故 DOI 路径在摘要为空时回落 OpenAlex 查一次（按 DOI 直接取作品，无需搜索）。
// 兜底失败不报错：摘要缺失不是身份错误，由调用方按 soft 策略告警入库。
func (f *MetadataFetcher) fillAbstract(ctx context.Context, p *model.Paper) {
	if p == nil || p.HasAbstract() || p.DOI == "" || f.OpenAlexBase == "" {
		return
	}
	data, err := f.get(ctx, "doi", f.OpenAlexBase+"/works/https://doi.org/"+p.DOI)
	if err != nil || data == nil {
		return
	}
	if ap, aerr := ParseOpenAlexWork(data); aerr == nil && ap.HasAbstract() {
		p.Abstract = ap.Abstract
	}
}

// RetractionInfo 撤稿判定结果。
type RetractionInfo struct {
	Retracted     bool   `json:"retracted"`
	RetractionDOI string `json:"retractionDoi,omitempty"` // 撤稿声明 DOI
	Source        string `json:"source,omitempty"`        // publisher | retraction-watch
	Label         string `json:"label,omitempty"`         // 声明类型（Retraction）
}

// retractionTypes 判定为「已撤稿」的 updated-by 类型。
// Crossref 原论文通过 updated-by 指向撤稿/撤回声明。
var retractionTypes = map[string]bool{
	"retraction":      true,
	"withdrawal":      true,
	"withdraw":        true,
	"is-retracted-by": true, // 关系型直接撤稿链接
}

// CheckRetraction 查询 DOI 是否已被撤稿（R5.7，Crossref updated-by 机制）。
//
// 未命中（404）/ 未发现撤稿事件返回 (false, nil)；网络或解析错误返回 error。
func (f *MetadataFetcher) CheckRetraction(ctx context.Context, doi string) (RetractionInfo, error) {
	u := f.CrossrefBase + "/works/" + url.PathEscape(doi)
	data, err := f.get(ctx, "doi", u)
	if err != nil || data == nil {
		return RetractionInfo{}, err
	}
	_, updates, err := ParseCrossrefWork(data)
	if err != nil {
		return RetractionInfo{}, fmt.Errorf("metadata doi: 解析 CrossRef 响应: %w", err)
	}
	// update-to 从声明指向原论文；只有反向的 updated-by 表示原论文已被更新。
	for _, up := range updates {
		if retractionTypes[strings.ToLower(up.Type)] {
			return RetractionInfo{
				Retracted:     true,
				RetractionDOI: up.DOI,
				Source:        up.Source,
				Label:         up.Label,
			}, nil
		}
	}
	return RetractionInfo{}, nil
}

func (f *MetadataFetcher) fetchTitle(ctx context.Context, title string) (*model.Paper, error) {
	q := url.Values{}
	q.Set("query.bibliographic", title)
	q.Set("rows", "1")
	u := f.CrossrefBase + "/works?" + q.Encode()
	data, err := f.get(ctx, "title", u)
	if err != nil || data == nil {
		return nil, err
	}
	p, ok, err := ParseCrossrefSearchFirst(data)
	if err != nil {
		return nil, fmt.Errorf("metadata title: 解析 CrossRef 检索响应: %w", err)
	}
	if !ok {
		return nil, nil
	}
	// 标题相似度校验：CrossRef 检索首条可能与查询标题不符（张冠李戴），
	// 低于阈值视为未命中（返回 nil），由调用方归入 Unresolved。
	if titleSimilar(title, p.Title) < titleMatchThreshold {
		return nil, nil
	}
	f.fillAbstract(ctx, &p)
	return &p, nil
}

// titleMatchThreshold 标题反查相似度阈值：CrossRef 检索首条低于此值视为匹配错误（防张冠李戴）。
const titleMatchThreshold = 0.5

// titleStopwords 标题归一化时剔除的英文停用词。
var titleStopwords = map[string]bool{
	"a": true, "an": true, "the": true, "in": true, "of": true, "for": true,
	"on": true, "to": true, "and": true, "with": true, "by": true, "et": true, "al": true,
}

// titleTokens 将标题归一化为 token 集合：小写、非字母数字替换为空格、去停用词。
func titleTokens(s string) map[string]bool {
	var b strings.Builder
	for _, r := range strings.ToLower(s) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
		} else {
			b.WriteByte(' ')
		}
	}
	tokens := make(map[string]bool)
	for _, w := range strings.Fields(b.String()) {
		if !titleStopwords[w] {
			tokens[w] = true
		}
	}
	return tokens
}

// titleSimilar 计算两个标题的 Jaccard 相似度（token 集合交集 / 并集）。
// 任一标题归一化后为空返回 0。
func titleSimilar(a, b string) float64 {
	ta, tb := titleTokens(a), titleTokens(b)
	if len(ta) == 0 || len(tb) == 0 {
		return 0
	}
	inter := 0
	for w := range ta {
		if tb[w] {
			inter++
		}
	}
	union := len(ta) + len(tb) - inter
	return float64(inter) / float64(union)
}

// fetchPMID 按 PMID 反查：走 efetch（XML）而非 esummary——esummary 响应不含 Abstract，
// 会让反查入库的文献永远无摘要（违反入库元数据必须含摘要，且无法参与引用评分）。
func (f *MetadataFetcher) fetchPMID(ctx context.Context, pmid string) (*model.Paper, error) {
	q := url.Values{}
	q.Set("db", "pubmed")
	q.Set("id", pmid)
	q.Set("rettype", "abstract")
	q.Set("retmode", "xml")
	u := f.EutilsBase + "/entrez/eutils/efetch.fcgi?" + q.Encode()
	data, err := f.get(ctx, "pmid", u)
	if err != nil || data == nil {
		return nil, err
	}
	papers, err := ParsePubmedEFetch(data)
	if err != nil {
		return nil, fmt.Errorf("metadata pmid: 解析 efetch 响应: %w", err)
	}
	for i := range papers {
		if papers[i].PMID == strings.TrimSpace(pmid) {
			return &papers[i], nil
		}
	}
	// 单条 efetch 正常只回一条；未匹配视为未命中（返回 nil, nil）。
	if len(papers) == 1 {
		return &papers[0], nil
	}
	return nil, nil
}

func (f *MetadataFetcher) fetchArxiv(ctx context.Context, id string) (*model.Paper, error) {
	q := url.Values{}
	q.Set("id_list", id)
	u := f.ArxivBase + "/api/query?" + q.Encode()
	data, err := f.get(ctx, "arxiv", u)
	if err != nil || data == nil {
		return nil, err
	}
	papers, err := ParseArxivAtom(data)
	if err != nil {
		return nil, fmt.Errorf("metadata arxiv: %w", err)
	}
	if len(papers) == 0 {
		return nil, nil
	}
	p := papers[0]
	return &p, nil
}
