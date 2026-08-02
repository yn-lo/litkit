package sources

import (
	"sort"
	"sync"

	"litkit/internal/config"
	"litkit/internal/util/httpclient"
	"litkit/internal/util/ratelimit"
)

// Registry 源注册表（FR-SRC-18、FR-IFACE-03）。
//
// CLI 与 MCP 共用同一注册表实例，保证接口一致性（C8）。
type Registry struct {
	mu      sync.RWMutex
	sources map[string]PaperSource
}

// NewRegistry 创建空注册表。
func NewRegistry() *Registry {
	return &Registry{sources: map[string]PaperSource{}}
}

// Register 注册一个源；重名覆盖（后注册者生效）。
func (r *Registry) Register(s PaperSource) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.sources[s.Name()] = s
}

// Get 按名取源；未注册返回 (nil, false)。
func (r *Registry) Get(name string) (PaperSource, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	s, ok := r.sources[name]
	return s, ok
}

// List 返回全部已注册源（按名排序，便于稳定输出）。
func (r *Registry) List() []PaperSource {
	r.mu.RLock()
	defer r.mu.RUnlock()
	names := make([]string, 0, len(r.sources))
	for name := range r.sources {
		names = append(names, name)
	}
	sort.Strings(names)
	out := make([]PaperSource, 0, len(names))
	for _, n := range names {
		out = append(out, r.sources[n])
	}
	return out
}

// Names 返回全部已注册源名（按名字典序）。
func (r *Registry) Names() []string {
	list := r.List()
	out := make([]string, len(list))
	for i, s := range list {
		out[i] = s.Name()
	}
	return out
}

// 默认 HTTP 退避基准与各源限速（platform-matrix.md，NFR-PERF-04）。
const (
	defaultBackoffBaseMS = 200  // 指数退避基准 200ms
	arxivRPS             = 0.33 // arXiv: 1 req/3s（官方要求）
	pubmedRPS            = 2.0  // PubMed: 3 req/s 无 key，保守取 2
	pubmedBurst          = 3    // PubMed: 令牌桶 burst
	openalexRPS          = 2.0  // OpenAlex: polite pool 10 RPS，保守取 2
)

// newHTTPClient 构造标准 HTTP 客户端（按 config 注入超时与重试次数）。
// 注册表构造时统一调用，保证各源 HTTP 行为一致。
func newHTTPClient(timeoutMS, retries int) *httpclient.Client {
	return httpclient.New(httpclient.Options{
		TimeoutMS:     timeoutMS,
		MaxRetries:    retries,
		BackoffBaseMS: defaultBackoffBaseMS,
	})
}

// NewDefaultRegistry 按配置创建并填充默认源注册表（FR-SRC-18）。
//
// 一期默认源（platform-matrix.md）：arxiv、pubmed、biorxiv、medrxiv、
// semantic_scholar、openalex。新增源在此登记，CLI 与 MCP 共用此构造
// 以保证接口同步（FR-IFACE-03）。
//
// 限速取合规保守值（platform-matrix.md），避免触发上游 429（NFR-PERF-04）。
func NewDefaultRegistry(cfg *config.Config) *Registry {
	if cfg == nil {
		// nil cfg 退化为默认值，避免入口层调用方必须构造 cfg
		cfg = &config.Config{
			HTTPTimeoutMS: config.DefaultHTTPTimeoutMS,
			HTTPRetries:   config.DefaultHTTPRetries,
		}
	}
	r := NewRegistry()
	httpc := newHTTPClient(cfg.HTTPTimeoutMS, cfg.HTTPRetries)

	// arXiv：1 req/3s ≈ 0.33 RPS, burst 1（官方要求）
	r.Register(NewArxivSource(httpc, ratelimit.New(arxivRPS, 1)))
	// PubMed：3 req/s（无 key），保守取 2 RPS, burst 3
	r.Register(NewPubmedSource(httpc, ratelimit.New(pubmedRPS, pubmedBurst)))
	// bioRxiv/medRxiv：官方无明确限速文档，取保守 1 RPS, burst 1
	r.Register(NewBiorxivSource("biorxiv", httpc, ratelimit.New(1.0, 1)))
	r.Register(NewBiorxivSource("medrxiv", httpc, ratelimit.New(1.0, 1)))
	// Semantic Scholar：有 key 5000 req/h ≈ 1.4 RPS；无 key 取 1 RPS, burst 1
	r.Register(NewSemanticScholarSource(cfg.SemanticScholarAPIKey, httpc, ratelimit.New(1.0, 1)))
	// OpenAlex：polite pool 10 RPS 可用，保守取 2 RPS, burst 2
	r.Register(NewOpenAlexSource(httpc, ratelimit.New(openalexRPS, 2)))
	return r
}
