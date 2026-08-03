// Package sources 实现学术源适配层（FR-SRC）。
//
// 所有源统一实现 PaperSource 接口（FR-SRC-01），通过 registry 注册（FR-SRC-18）。
// 新增源只需放适配文件 + 在 NewDefaultRegistry 注册，不修改核心层。
//
// 分层归属：适配层，仅可依赖叶子层（model / config / util）。
package sources

import (
	"context"
	"net/http"

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
)

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
}

// NewBaseSource 创建 BaseSource。
//   - name：源标识
//   - httpClient：HTTP 客户端（含超时与 429/503 重试）
//   - limiter：每源令牌桶限速器（NFR-PERF-04、NFR-REL-04）；nil 表示不限速
func NewBaseSource(name string, httpClient *httpclient.Client, limiter *ratelimit.Limiter) BaseSource {
	return BaseSource{name: name, http: httpClient, limiter: limiter}
}

// Name 返回源标识。
func (b BaseSource) Name() string { return b.name }

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
	if !b.limiter.Allow(ctx) {
		return nil, ctx.Err()
	}
	return b.http.Do(ctx, req)
}
