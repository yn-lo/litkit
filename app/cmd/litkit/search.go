package main

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"litkit/internal/config"
	"litkit/internal/core"
	"litkit/internal/model"
	"litkit/internal/sources"
)

// exitCodePartialFailure 部分源失败但部分成功的退出码（api.md §1.1）。
const exitCodePartialFailure = 3

// searchOutput 默认输出（FR-IFACE-04：精简视图，面向 AI agent）。
type searchOutput struct {
	Total         int                  `json:"total"`
	SourceResults map[string]int       `json:"sourceResults"` // 源 → 命中条数（降噪）
	Errors        []model.SourceError  `json:"errors,omitempty"`
	Papers        []model.PaperSummary `json:"papers"`
}

// searchOutputFull --full 模式输出（完整元数据，供人类调试或特殊场景）。
type searchOutputFull struct {
	Total         int                      `json:"total"`
	SourceResults map[string][]model.Paper `json:"sourceResults"`
	Errors        []model.SourceError      `json:"errors,omitempty"`
	Papers        []model.Paper            `json:"papers"`
}

// searchOutcomeError 依据检索结果映射退出码：
// 部分源失败但仍有源成功（含零结果）→ 3；全部源失败 → 1；否则 nil。
func searchOutcomeError(res *model.SearchResult) error {
	srcErrs := sourceErrors(res.Errors)
	if len(srcErrs) == 0 {
		return nil
	}
	if len(res.SourceResults) > 0 {
		return &exitError{code: exitCodePartialFailure}
	}
	// 网络类失败时追加代理提示（AI-first：错误消息即文档，agent 只读得到 stderr）。
	if hasNetworkError(srcErrs) {
		return fmt.Errorf("search: 所有源均失败（检测到网络类错误；网络受限环境可在 .env 设 LITKIT_PROXY_URL=http://127.0.0.1:7890 启用代理，详见 litkit --help）")
	}
	return fmt.Errorf("search: 所有源均失败")
}

// sourceErrors 过滤掉本地入库（storage）错误，只保留检索源错误。
func sourceErrors(errs []model.SourceError) []model.SourceError {
	out := make([]model.SourceError, 0, len(errs))
	for _, e := range errs {
		if e.Source != "storage" {
			out = append(out, e)
		}
	}
	return out
}

// shortErrors 将完整源错误压缩为精简原因（FR-IFACE-04）。
// 完整错误保留在 --full 输出与日志中；AI 需要细查时重跑 search --full。
func shortErrors(errs []model.SourceError) []model.SourceError {
	out := make([]model.SourceError, len(errs))
	for i, e := range errs {
		out[i] = model.SourceError{Source: e.Source, Error: core.ShortError(e.Error)}
	}
	return out
}

// netErrorMarkers 网络类错误特征（小写子串匹配）。
var netErrorMarkers = []string{
	"timeout", "tls", "handshake", "connection refused", "connection reset",
	"proxyconnect", "no such host", "network", "unreachable",
}

// hasNetworkError 判断源错误中是否存在网络类失败（用于追加代理提示）。
func hasNetworkError(errs []model.SourceError) bool {
	for _, e := range errs {
		msg := strings.ToLower(e.Error)
		for _, m := range netErrorMarkers {
			if strings.Contains(msg, m) {
				return true
			}
		}
	}
	return false
}

// resolveSearchSources 计算本次检索实际使用的源集合。
//
// 显式 -s（explicit 非空）完全覆盖 .env 配置；否则从白名单（未设=全部注册源）
// 中减去排除列表。warnings 为 .env 中未注册的源名（调用方输出到 stderr）。
func resolveSearchSources(explicit string, cfg *config.Config, registered []string) (names, warnings []string) {
	if names := config.SplitList(explicit); len(names) > 0 {
		return names, nil
	}
	known := make(map[string]struct{}, len(registered))
	for _, n := range registered {
		known[n] = struct{}{}
	}
	base := cfg.SearchSources
	if len(base) == 0 {
		base = registered
	} else {
		kept := make([]string, 0, len(base))
		for _, n := range base {
			if _, ok := known[n]; ok {
				kept = append(kept, n)
			} else {
				warnings = append(warnings, fmt.Sprintf("LITKIT_SEARCH_SOURCES 中的未知源 %q 已忽略（可用源见 litkit sources）", n))
			}
		}
		base = kept
	}
	exclude := make(map[string]struct{}, len(cfg.SearchExcludeSources))
	for _, n := range cfg.SearchExcludeSources {
		if _, ok := known[n]; !ok {
			warnings = append(warnings, fmt.Sprintf("LITKIT_SEARCH_EXCLUDE_SOURCES 中的未知源 %q 已忽略（可用源见 litkit sources）", n))
			continue
		}
		exclude[n] = struct{}{}
	}
	out := make([]string, 0, len(base))
	for _, n := range base {
		if _, skip := exclude[n]; skip {
			continue
		}
		out = append(out, n)
	}
	return out, warnings
}

// newSearchCmd 构造 `litkit search` 子命令。
//
// 默认输出 PaperSummary（FR-IFACE-04）；--full 输出完整 Paper。
// 默认检索等级 tiab（题目+摘要+关键词）、默认最近 N 年（FR-SEARCH-12/13）。
// 退出码：3 表示部分源失败但部分成功（api.md §1.1）。
func newSearchCmd(s *core.Searcher, reg *sources.Registry, cfg *config.Config) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "search <query> [-s sources] [-n N] [--mode tiab|full] [--years N|--since YEAR] [-y year] [--keep-no-abstract] [--exclude TERM,...] [--no-store] [--full]",
		Short: "跨源并发检索文献",
		Long: `跨源并发检索文献，默认输出精简视图（citeKey/title/firstAuthor/year/abstract），
面向 AI agent 调用（FR-IFACE-04）。结果按 DOI→title→id 三级去重，年份倒序。

检索词：英文关键词命中更高（arXiv/PubMed/OpenAlex/S2/bioRxiv/medRxiv 多为英文语料）；
如需检索中文语料，用 -s crossref,doaj（中文关键词可命中中文标题/中文摘要，FR-SRC-20/21）。

检索等级（FR-SEARCH-12）：
  --mode tiab（默认）  题目+摘要+关键词（源支持时），误检率低
  --mode full          全文检索，作高级选项

时间范围（FR-SEARCH-13）：
  --years N（默认 3）  最近 N 年；0 表示不限
  --since YEAR         显式起始年份（优先于 --years）
  -y YEAR              精确单年过滤

--full 输出完整元数据（含 doi/pmid/arxivId/url/venue/全部作者）与完整错误。

--no-store 探索模式：结果只返回、不入库，citeKey 为空。检索词拿不准时先试跑，
确认相关后再去掉本选项正式检索入库，避免弱相关文献污染文献库。
本模式无需 LITKIT_WORK_DIR。

网络：检索源默认直连。网络受限环境（报 TLS handshake timeout / connection
timeout）在 .env 设 LITKIT_PROXY_URL（如 http://127.0.0.1:7890）使全部外呼走代理，
配置详见 litkit --help。`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			query := args[0]
			sourcesFlag, _ := cmd.Flags().GetString("sources")
			maxResults, _ := cmd.Flags().GetInt("max-results")
			year, _ := cmd.Flags().GetInt("year")
			mode, _ := cmd.Flags().GetString("mode")
			sinceFlag, _ := cmd.Flags().GetInt("since")
			years, _ := cmd.Flags().GetInt("years")
			keepNoAbstract, _ := cmd.Flags().GetBool("keep-no-abstract")
			excludeFlag, _ := cmd.Flags().GetString("exclude")
			noStore, _ := cmd.Flags().GetBool("no-store")
			full, _ := cmd.Flags().GetBool("full")

			if err := validateSearchParams(query, maxResults, years, sinceFlag, year, mode); err != nil {
				return err
			}

			// 工作目录必须显式设置（FR-LIB-03）：结果需自动入库回填 citeKey；
			// --no-store 不入库，无需工作目录。
			if !noStore {
				if err := requireWorkDir(cfg); err != nil {
					return err
				}
			}

			opts := core.SearchOptions{
				MaxResults:     maxResults,
				Year:           year,
				Since:          computeSince(sinceFlag, years),
				Mode:           mode,
				KeepNoAbstract: keepNoAbstract,
				Exclude:        config.SplitList(excludeFlag),
				NoStore:        noStore,
			}
			names, warns := resolveSearchSources(sourcesFlag, cfg, reg.Names())
			for _, w := range warns {
				fmt.Fprintf(os.Stderr, "litkit: %s\n", w)
			}
			// 显式 -s 的未知源直接报错（env 中的未知源已在上一步过滤并告警）
			for _, name := range names {
				if _, ok := reg.Get(name); !ok {
					return &paramError{msg: fmt.Sprintf("search: 未知源 %q（可用源见 litkit sources）", name)}
				}
			}
			if len(names) == 0 {
				return fmt.Errorf("search: 无可检索源（检查 LITKIT_SEARCH_SOURCES / LITKIT_SEARCH_EXCLUDE_SOURCES，可用源见 litkit sources）")
			}
			opts.Sources = names

			res, err := s.Search(cmd.Context(), query, opts)
			if err != nil {
				return err
			}
			if noStore {
				fmt.Fprintln(os.Stderr, "litkit: --no-store 探索模式：结果未入库，citeKey 为空")
			}

			if full {
				if perr := printJSON(searchOutputFull{
					Total:         res.Total,
					SourceResults: res.SourceResults,
					Errors:        res.Errors,
					Papers:        res.Papers,
				}); perr != nil {
					return perr
				}
			} else {
				// 默认：精简视图 + sourceResults 降为条数（降噪）
				srcCounts := make(map[string]int, len(res.SourceResults))
				for src, ps := range res.SourceResults {
					srcCounts[src] = len(ps)
				}
				if perr := printJSON(searchOutput{
					Total:         res.Total,
					SourceResults: srcCounts,
					Errors:        shortErrors(res.Errors),
					Papers:        model.SummarizePapers(res.Papers),
				}); perr != nil {
					return perr
				}
			}
			// 部分源失败但部分成功：退出码 3（api.md §1.1）。
			// 仅统计检索源错误；本地入库（storage）错误已在输出中可见，不算源失败。
			return searchOutcomeError(res)
		},
	}
	cmd.Flags().StringP("sources", "s", "", "逗号分隔源列表（litkit sources 查看）")
	cmd.Flags().IntP("max-results", "n", 0, "每源最大条数（0=源默认 5）")
	cmd.Flags().String("mode", cfg.SearchMode, "检索等级：tiab（题目+摘要+关键词）| full（全文）")
	cmd.Flags().Int("years", cfg.RecentYears, "最近 N 年（0=不限）")
	cmd.Flags().Int("since", 0, "显式起始年份（优先于 --years）")
	cmd.Flags().IntP("year", "y", 0, "精确年份过滤（源支持时）")
	cmd.Flags().Bool("keep-no-abstract", false, "保留无摘要论文（默认过滤，FR-SEARCH-03）")
	cmd.Flags().String("exclude", "", "逗号分隔排除词：标题或摘要命中任一排除词即剔除（本地召回后筛查，先排除后入库）")
	cmd.Flags().Bool("no-store", false, "只检索不入库（探索模式：不分配 citeKey；无需 LITKIT_WORK_DIR）")
	cmd.Flags().Bool("full", false, "输出完整元数据（默认精简视图，FR-IFACE-04）")
	return cmd
}

// computeSince 计算起始年份（含）：--since 优先；否则 --years N → 当前年-N+1。
// 两者均为 0 时返回 0（不过滤）。
func computeSince(sinceFlag, years int) int {
	if sinceFlag > 0 {
		return sinceFlag
	}
	if years > 0 {
		return time.Now().Year() - years + 1
	}
	return 0
}

// validateSearchParams 校验 search 命令参数（无效返回 paramError，退出码 2）。
func validateSearchParams(query string, maxResults, years, sinceFlag, year int, mode string) error {
	if query == "" {
		return &paramError{msg: "search: 检索词不能为空"}
	}
	if maxResults < 0 {
		return &paramError{msg: "search: -n 不能为负数"}
	}
	if years < 0 {
		return &paramError{msg: "search: --years 不能为负数"}
	}
	if sinceFlag < 0 {
		return &paramError{msg: "search: --since 不能为负数"}
	}
	if year < 0 {
		return &paramError{msg: "search: -y 不能为负数"}
	}
	if mode != "" && mode != "tiab" && mode != "full" {
		return &paramError{msg: fmt.Sprintf("search: 无效 --mode %q（支持 tiab|full）", mode)}
	}
	return nil
}
