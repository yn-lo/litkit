// search.go 实现跨源并发检索编排（FR-SEARCH-01/02/03/06）。
//
// 职责：
//   - 并发扇出到 registry 中所有（或指定）源（FR-SEARCH-01）
//   - 单源失败隔离：归入 SearchResult.Errors，不中断整体
//   - 三级去重：DOI > 归一化标题 > ID（FR-SEARCH-02）
//   - 无摘要过滤：默认丢弃 Abstract 空串的论文（FR-SEARCH-03）
//   - 结果入库：去重后的论文 upsert 进本地文献库（FR-LIB-01/06），
//     带摘要的论文自动分配 cite_key 并回填到输出；入库失败不阻断检索

package core

import (
	"context"
	"regexp"
	"sort"
	"strings"
	"sync"

	"litkit/internal/model"
	"litkit/internal/sources"
	"litkit/internal/storage"
	"litkit/internal/util/textutil"
)

// SearchOptions 跨源检索参数（服务层视角，对应 api.md §1.2）。
//
// 与 sources.SearchOptions 不同：core.SearchOptions 面向跨源编排，
// 包含源过滤与无摘要策略；sources.SearchOptions 是单源适配参数。
type SearchOptions struct {
	// Sources 指定源名列表；空表示 registry 中全部源。
	Sources []string `json:"sources,omitempty"`
	// MaxResults 每源最大条数；0 应用默认值 5。
	MaxResults int `json:"maxResults,omitempty"`
	// Year 年份过滤；0 表示不过滤。
	Year int `json:"year,omitempty"`
	// Since 起始年份（含）范围过滤；0 表示不过滤。与 Year 互斥，Since 优先。
	Since int `json:"since,omitempty"`
	// Mode 检索等级："" 或 "tiab"（默认，题目+摘要+关键词，源支持时）；"full"（全文）。
	Mode string `json:"mode,omitempty"`
	// KeepNoAbstract 保留无摘要论文（默认过滤，FR-SEARCH-03）。
	KeepNoAbstract bool `json:"keepNoAbstract,omitempty"`
}

// applyDefaults 填充默认值。
func (o *SearchOptions) applyDefaults(defaultMax int) {
	if o.MaxResults <= 0 {
		o.MaxResults = defaultMax
	}
}

// Searcher 跨源检索编排器。
//
// 持有源注册表与可选文献库；并发安全（每次 Search 调用独立）。
type Searcher struct {
	registry          *sources.Registry
	store             *storage.Store
	defaultMaxResults int
}

// NewSearcher 创建检索器。
//   - registry：源注册表（必填，nil 时 Search 直接返回空结果）
//   - store：可选本地文献库；nil 表示检索结果不入库
//   - defaultMaxResults：每源默认检索条数（≤0 时用 5 兜底）
func NewSearcher(registry *sources.Registry, store *storage.Store, defaultMaxResults int) *Searcher {
	if defaultMaxResults <= 0 {
		defaultMaxResults = 5
	}
	return &Searcher{registry: registry, store: store, defaultMaxResults: defaultMaxResults}
}

// Search 执行跨源并发检索。
//
// 返回 (*SearchResult, error)：error 仅在 registry 不可用时非 nil；
// 单源失败归入 result.Errors（FR-SEARCH-01）。
func (s *Searcher) Search(ctx context.Context, query string, opts SearchOptions) (*model.SearchResult, error) {
	opts.applyDefaults(s.defaultMaxResults)
	result := model.NewSearchResult()

	if s.registry == nil {
		return result, nil
	}

	srcs := s.selectSources(opts.Sources)
	if len(srcs) == 0 {
		return result, nil
	}

	var (
		mu        sync.Mutex
		wg        sync.WaitGroup
		allPapers []model.Paper
	)
	for _, src := range srcs {
		wg.Add(1)
		go func(src sources.PaperSource) {
			defer wg.Done()
			papers, srcErr := s.searchOne(ctx, src, query, opts)
			mu.Lock()
			defer mu.Unlock()
			if srcErr != nil {
				result.Errors = append(result.Errors, model.SourceError{
					Source: src.Name(),
					Error:  srcErr.Error(),
				})
				return
			}
			result.SourceResults[src.Name()] = papers
			allPapers = append(allPapers, papers...)
		}(src)
	}
	wg.Wait()

	// 三级去重
	merged := dedupPapers(allPapers)
	// 无摘要过滤
	if !opts.KeepNoAbstract {
		merged = filterByAbstract(merged)
	}
	// 入库：仅带摘要论文（FR-LIB-01）；失败不阻断检索（透明降级）
	if s.store != nil {
		merged = s.persist(merged)
	}
	// 默认年份倒序（最新在前）；year=0 排末尾（FR-SEARCH-05）
	sortPapersByYearDesc(merged)
	result.Papers = merged
	result.Total = len(merged)
	return result, nil
}

// sortPapersByYearDesc 按年份倒序排列；year=0（未知）排末尾，保持稳定排序。
//
// 学术写作偏好近年成果，故默认年份倒序。各源原始返回顺序语义不一
// （arXiv 按相关性、PubMed 按日期），跨源混合后无意义，必须显式排序。
func sortPapersByYearDesc(papers []model.Paper) {
	sort.SliceStable(papers, func(i, j int) bool {
		return papers[i].Year > papers[j].Year
	})
}

// httpStatusRe 匹配错误信息中的 "HTTP <3位码>"。
var httpStatusRe = regexp.MustCompile(`HTTP (\d{3})`)

// ShortError 将完整源错误压缩为 AI 可读的简短原因（FR-IFACE-04）。
//
// 完整错误（含 URL / 内部细节）是调试噪声，默认输出只保留"失败类型"，
// 源名由 SourceError.Source 字段负责。需要细查时用 search --full。
func ShortError(errMsg string) string {
	switch {
	case strings.Contains(errMsg, "context deadline exceeded"),
		strings.Contains(errMsg, "Client.Timeout"):
		return "timeout"
	case strings.Contains(errMsg, "HTTP 429"):
		return "rate limited"
	case strings.Contains(errMsg, "HTTP 403"):
		return "forbidden"
	case httpStatusRe.MatchString(errMsg):
		return "HTTP " + httpStatusRe.FindStringSubmatch(errMsg)[1]
	default:
		return errMsg // 无网络噪声可精简，保留原文
	}
}

// persist 将去重后的论文 upsert 进文献库，并回填 cite_key（FR-LIB-06）。
//
// 入库失败静默忽略（检索结果不受影响）。
func (s *Searcher) persist(papers []model.Paper) []model.Paper {
	for i := range papers {
		if !papers[i].HasAbstract() {
			continue
		}
		citeKey, _, err := s.store.UpsertPaper(papers[i])
		if err == nil {
			papers[i].CiteKey = citeKey
		}
	}
	return papers
}

// selectSources 按 names 过滤；names 为空时返回全部。
// 未知名静默忽略（不视为错误）。
func (s *Searcher) selectSources(names []string) []sources.PaperSource {
	all := s.registry.List()
	if len(names) == 0 {
		return all
	}
	want := make(map[string]struct{}, len(names))
	for _, n := range names {
		want[strings.ToLower(strings.TrimSpace(n))] = struct{}{}
	}
	out := make([]sources.PaperSource, 0, len(all))
	for _, src := range all {
		if _, ok := want[src.Name()]; ok {
			out = append(out, src)
		}
	}
	return out
}

// searchOne 单源检索（含参数装配）。
func (s *Searcher) searchOne(ctx context.Context, src sources.PaperSource, query string, opts SearchOptions) ([]model.Paper, error) {
	sourceOpts := sources.SearchOptions{
		MaxResults: opts.MaxResults,
		Year:       opts.Year,
		Since:      opts.Since,
		Mode:       opts.Mode,
	}
	return src.Search(ctx, query, sourceOpts)
}

// dedupPapers 三级去重：DOI > 归一化标题 > ID（FR-SEARCH-02）。
//
// 合并时调用 mergePapers 取非空字段。
func dedupPapers(papers []model.Paper) []model.Paper {
	type slot struct {
		idx int
	}
	byDOI := make(map[string]slot)
	byTitle := make(map[string]slot)
	byID := make(map[string]slot)
	out := make([]model.Paper, 0, len(papers))

	mergeInto := func(idx int, p model.Paper) {
		out[idx] = mergePapers(out[idx], p)
		// 合并后更新索引键，避免漏掉新出现的更强键
		if p.DOI != "" {
			byDOI[p.DOI] = slot{idx}
		}
		tk := textutil.NormalizeTitle(p.Title)
		if tk != "" {
			byTitle[tk] = slot{idx}
		}
		if p.ID != "" {
			byID[p.ID] = slot{idx}
		}
	}

	for _, p := range papers {
		if p.ID == "" {
			p.ID = p.ComputeID()
		}
		// 1. DOI
		if p.DOI != "" {
			if s, ok := byDOI[p.DOI]; ok {
				mergeInto(s.idx, p)
				continue
			}
		}
		// 2. 归一化标题
		tk := textutil.NormalizeTitle(p.Title)
		if tk != "" {
			if s, ok := byTitle[tk]; ok {
				mergeInto(s.idx, p)
				continue
			}
		}
		// 3. ID
		if p.ID != "" {
			if s, ok := byID[p.ID]; ok {
				mergeInto(s.idx, p)
				continue
			}
		}
		// 新条目
		idx := len(out)
		out = append(out, p)
		if p.DOI != "" {
			byDOI[p.DOI] = slot{idx}
		}
		if tk != "" {
			byTitle[tk] = slot{idx}
		}
		if p.ID != "" {
			byID[p.ID] = slot{idx}
		}
	}
	return out
}

// mergePapers 合并两篇同源/异源论文：取非空字段；冲突时保留已有值。
//
// 用于 dedupPapers 中将多源信息汇入一条记录。
//
//nolint:gocyclo // 字段逐一合并，圈复杂度结构性偏高（ponytail: 上限 30，重构不增益可读性）
func mergePapers(existing, incoming model.Paper) model.Paper {
	if existing.Abstract == "" && incoming.Abstract != "" {
		existing.Abstract = incoming.Abstract
	}
	if existing.DOI == "" && incoming.DOI != "" {
		existing.DOI = incoming.DOI
	}
	if existing.PMID == "" && incoming.PMID != "" {
		existing.PMID = incoming.PMID
	}
	if existing.ArXivID == "" && incoming.ArXivID != "" {
		existing.ArXivID = incoming.ArXivID
	}
	if existing.URL == "" && incoming.URL != "" {
		existing.URL = incoming.URL
	}
	if existing.Venue == "" && incoming.Venue != "" {
		existing.Venue = incoming.Venue
	}
	if existing.Year == 0 && incoming.Year != 0 {
		existing.Year = incoming.Year
	}
	if existing.DocType == "" && incoming.DocType != "" {
		existing.DocType = incoming.DocType
	}
	if existing.Source == "" {
		existing.Source = incoming.Source
	}
	if len(existing.Authors) == 0 && len(incoming.Authors) > 0 {
		existing.Authors = incoming.Authors
	}
	if existing.Citations == 0 && incoming.Citations != 0 {
		existing.Citations = incoming.Citations
	}
	return existing
}

// filterByAbstract 移除无摘要的论文（FR-SEARCH-03）。
func filterByAbstract(papers []model.Paper) []model.Paper {
	out := make([]model.Paper, 0, len(papers))
	for _, p := range papers {
		if p.HasAbstract() {
			out = append(out, p)
		}
	}
	return out
}
