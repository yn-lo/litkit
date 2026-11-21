// Package sources 实现学术源适配层（FR-SRC）。
//
// 所有源统一实现 PaperSource 接口（FR-SRC-01），通过 registry 注册（FR-SRC-18）。
// 新增源只需放适配文件 + 在 NewDefaultRegistry 注册，不修改核心层。
//
// 分层归属：适配层，仅可依赖叶子层（model / config / util）。
package sources

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"litkit/internal/model"
	"litkit/internal/util/httpclient"
	"litkit/internal/util/ratelimit"
)

// SearchOptions 检索参数（远程检索仅 keyword 模式，FR-SEARCH-07）。
type SearchOptions struct {
	MaxResults int    // 每源最大条数；0 表示由源决定默认
	Year       int    // 精确年份过滤；0 表示不过滤
	Since      int    // 起始年份（含）范围过滤；0 表示不过滤。与 Year 互斥，Year 优先
	Mode       string // 检索等级："" 或 "tiab"（题目+摘要+关键词，源支持时）；"full"（全文）
	// RequireAbstract 要求源端只返回带摘要的记录。调用方默认丢弃无摘要论文时开启，
	// 支持的源把该条件下推到服务端，避免「先按 -n 截断、后本地过滤」导致整源召回为空
	// （FR-SEARCH-03）。不支持该条件的源忽略本字段。
	RequireAbstract bool
}

// 文档类型常量（FR-REF-03 引用渲染按类型区分；goconst 避免字面量重复）。
const (
	DocTypeArticle  = "article"
	DocTypePreprint = "preprint"
)

// 检索等级常量（FR-SEARCH-12）。
// tiab 为默认：题目+摘要+关键词（源支持时）；full 为全文（高级选项，误检率较高）。
const (
	modeTiab = "tiab"
	modeFull = "full"
)

// 检索默认值。
const (
	defaultMaxResults = 5 // 未指定 MaxResults 时每源返回条数（兜底；实际由 config 注入）
)

// HTTP 请求头常量。
const (
	defaultUserAgent = "litkit/0.1 (https://github.com/litkit/litkit)"
	// mailUserAgent 带 mailto 的 UA：Crossref/OpenAlex 的 polite pool 要求
	mailUserAgent = "litkit/0.1 (mailto:litkit-tool@users.noreply.github.com)"
)

// DefaultSource 可选接口：源是否参与未显式 -s 的默认检索。
//
// 未实现该接口视为 true。召回能力天然受限的源（bioRxiv/medRxiv 无关键词检索端点，
// 只能拉最近批次再本地过滤）返回 false：它们仍需 -s 显式指定才参与，避免默认检索
// 因「召回不足」产生假 0 命中与噪声。
type DefaultSource interface {
	DefaultEnabled() bool
}

// IsDefaultEnabled 报告源是否参与默认检索（未实现 DefaultSource 视为 true）。
func IsDefaultEnabled(s PaperSource) bool {
	if d, ok := s.(DefaultSource); ok {
		return d.DefaultEnabled()
	}
	return true
}

// PaperSource 学术源统一抽象（FR-SRC-01）。
//
// 新增源只需实现本接口并在 registry 注册（FR-SRC-18）。
// 检索源必须提供摘要（HasAbstract 返回 true）；无摘要源不纳入检索（FR-SRC-19）。
type PaperSource interface {
	// Name 源标识（如 "arxiv"、"pubmed"），全小写，用作注册键与缓存键。
	Name() string
	// Search 检索（ctx 控制超时与取消）；单源失败由调用方归入 errors（FR-SEARCH-01）。
	Search(ctx context.Context, query string, opts SearchOptions) ([]model.Paper, error)
	// HasAbstract 该源是否提供摘要。检索源必须返回 true（FR-SRC-19）。
	HasAbstract() bool
}

// BaseSource 提供源适配器公共逻辑：HTTP 客户端、每源限速器、统一 Do 方法。
//
// 各源以嵌入方式复用；自身只实现 Name/Search/HasAbstract 与解析逻辑。
type BaseSource struct {
	name    string
	http    *httpclient.Client
	limiter *ratelimit.Limiter
	// defaultEnabled 是否参与未显式 -s 的默认检索（默认 true；见 DefaultSource）。
	defaultEnabled bool
}

// NewBaseSource 创建 BaseSource。
//   - name：源标识
//   - httpClient：HTTP 客户端（含超时与 429/503 重试）
//   - limiter：每源令牌桶限速器（NFR-PERF-04、NFR-REL-04）；nil 表示不限速
func NewBaseSource(name string, httpClient *httpclient.Client, limiter *ratelimit.Limiter) BaseSource {
	return BaseSource{name: name, http: httpClient, limiter: limiter, defaultEnabled: true}
}

// Name 返回源标识。
func (b BaseSource) Name() string { return b.name }

// DefaultEnabled 报告该源是否参与默认检索（BaseSource 默认 true）。
func (b BaseSource) DefaultEnabled() bool { return b.defaultEnabled }

// HasAbstract 默认 true（检索源必须提供摘要，FR-SRC-19）。
// 个别源（如反查源 CrossRef）可覆盖返回 false。
func (b BaseSource) HasAbstract() bool { return true }

// HTTPClient 返回底层 HTTP 客户端（供适配器构造请求时复用）。
func (b BaseSource) HTTPClient() *httpclient.Client { return b.http }

// Do 执行带限速与重试的 HTTP 请求。
//
// 流程：限速器取令牌（Wait）→ HTTP 客户端执行（含 429/503 退避重试）。
// ctx 取消会同时中断限速等待与重试等待。
func (b BaseSource) Do(ctx context.Context, req *http.Request) (*http.Response, error) {
	if err := b.limiter.Wait(ctx); err != nil {
		return nil, fmt.Errorf("%s rate limit: %w", b.name, err)
	}
	return b.http.Do(ctx, req)
}

// maxResponseBytes 单个检索/元数据响应体上限（防超大或恶意响应耗尽内存）。
const maxResponseBytes = 10 << 20

// get 执行已构造的 GET 请求并读回 body（校验 2xx）。
// 非 2xx 返回含状态码错误；ctx 控制取消。
func (b BaseSource) get(ctx context.Context, req *http.Request) ([]byte, error) {
	resp, err := b.Do(ctx, req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	return httpclient.ReadAllLimit(resp, maxResponseBytes)
}

// search 通用「GET JSON → parse」检索骨架，并为各源注入统一的错误前缀。
//
// 各 JSON 语料源（openalex/crossref/doaj）复用，消除逐源雷同的 Search 方法体
// （dupl 检查要求）。name 用作错误前缀；ua 为 User-Agent；build 负责拼 URL；
// parse 负责解析厂商响应。
func (b BaseSource) search(ctx context.Context, name, ua string, build func() (string, error), parse func([]byte) ([]model.Paper, error)) ([]model.Paper, error) {
	u, err := build()
	if err != nil {
		return nil, fmt.Errorf("%s search: %w", name, err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, fmt.Errorf("%s search: %w", name, err)
	}
	req.Header.Set("User-Agent", ua)
	data, err := b.get(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("%s search: %w", name, err)
	}
	papers, err := parse(data)
	if err != nil {
		return nil, fmt.Errorf("%s search: %w", name, err)
	}
	return papers, nil
}

// splitAuthorName 将单个姓名串拆为 given/family。
//
// 供各源适配器复用，消除逐源复制的拆分逻辑（此前 arXiv/OpenAlex/bioRxiv/S2 四处雷同）。
// 接受两种常见形式：
//   - "Given [Middle] Family"（arXiv/OpenAlex/S2/DOAJ）：按**最后一个**空白切分，
//     保证 "John A. Smith" → Given="John A." / Family="Smith"（按首空格会把中间名当前姓）；
//   - "Family, Given"（逗号反转）：逗号前为 Family。
//
// 纯中文名（无空白、无逗号）整体入 Family；空串返回空 Author（调用方自行决定是否跳过）。
func splitAuthorName(name string) model.Author {
	name = strings.TrimSpace(name)
	if name == "" {
		return model.Author{}
	}
	if i := strings.Index(name, ","); i > 0 {
		return model.Author{
			Family: strings.TrimSpace(name[:i]),
			Given:  strings.TrimSpace(name[i+1:]),
		}
	}
	if i := strings.LastIndexAny(name, " \t"); i > 0 {
		return model.Author{
			Given:  strings.TrimSpace(name[:i]),
			Family: strings.TrimSpace(name[i+1:]),
		}
	}
	return model.Author{Family: name}
}

// termStopwords 拆词时剔除的英文虚词：不剔除则多词 AND 检索召回骤降
// （如 "fear of pain" 里的 of 会被当成必需关键词）。
var termStopwords = map[string]bool{
	"a": true, "an": true, "the": true, "of": true, "in": true, "on": true,
	"for": true, "to": true, "with": true, "by": true, "from": true, "as": true,
	"at": true, "and": true, "or": true, "is": true, "are": true,
}

// hasQuerySyntax 判断检索词是否已含上游高级语法（引号短语 / 布尔操作符 / 字段标签）。
// 含语法时各源不拆词，原样交给上游解析，避免破坏用户显式表达的语义。
func hasQuerySyntax(query string) bool {
	if strings.ContainsAny(query, `"[]`) {
		return true
	}
	up := " " + strings.ToUpper(query) + " "
	for _, op := range []string{" AND ", " OR ", " NOT ", " ANDNOT "} {
		if strings.Contains(up, op) {
			return true
		}
	}
	return false
}

// searchTerms 将自由文本检索词拆为词项：去引号/括号、按空白切分、剔除英文虚词。
//
// 各源的原生关键词检索多为合并匹配，把整串多词当短语（或短语化加引号）几乎必然 0 命中；
// 拆词后由各源以 AND 语义组合。拆空时回退原文，避免把检索词清空。
func searchTerms(query string) []string {
	clean := strings.NewReplacer(`"`, " ", `(`, " ", `)`, " ", `[`, " ", `]`, " ").Replace(query)
	out := make([]string, 0, len(query)/4+1)
	for _, f := range strings.Fields(clean) {
		if termStopwords[strings.ToLower(f)] {
			continue
		}
		out = append(out, f)
	}
	if len(out) == 0 {
		if q := strings.TrimSpace(query); q != "" {
			return []string{q}
		}
	}
	return out
}
