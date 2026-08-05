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

// shortErrors 将完整源错误压缩为精简原因（FR-IFACE-04）。
// 完整错误保留在 --full 输出与日志中；AI 需要细查时重跑 search --full。
func shortErrors(errs []model.SourceError) []model.SourceError {
	out := make([]model.SourceError, len(errs))
	for i, e := range errs {
		out[i] = model.SourceError{Source: e.Source, Error: core.ShortError(e.Error)}
	}
	return out
}

// newSearchCmd 构造 `litkit search` 子命令。
//
// 默认输出 PaperSummary（FR-IFACE-04）；--full 输出完整 Paper。
// 默认检索等级 tiab（题目+摘要+关键词）、默认最近 N 年（FR-SEARCH-12/13）。
// 退出码：3 表示部分源失败但部分成功（api.md §1.1）。
func newSearchCmd(s *core.Searcher, reg *sources.Registry, cfg *config.Config) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "search <query> [-s sources] [-n N] [--mode tiab|full] [--years N|--since YEAR] [-y year] [--keep-no-abstract] [--exclude TERM,...] [--full]",
		Short: "跨源并发检索文献",
		Long: `跨源并发检索文献，默认输出精简视图（citeKey/title/firstAuthor/year/abstract），
面向 AI agent 调用（FR-IFACE-04）。结果按 DOI→title→id 三级去重，年份倒序。

检索词建议使用英文：各源（arXiv/PubMed/OpenAlex/S2/bioRxiv/medRxiv）
均为英文语料为主，中文检索词命中率极低（FR-SEARCH-11）。

检索等级（FR-SEARCH-12）：
  --mode tiab（默认）  题目+摘要+关键词（源支持时），误检率低
  --mode full          全文检索，作高级选项

时间范围（FR-SEARCH-13）：
  --years N（默认 3）  最近 N 年；0 表示不限
  --since YEAR         显式起始年份（优先于 --years）
  -y YEAR              精确单年过滤

--full 输出完整元数据（含 doi/pmid/arxivId/url/venue/全部作者）与完整错误。`,
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
			full, _ := cmd.Flags().GetBool("full")

			if err := validateSearchParams(query, maxResults, years, sinceFlag, year, mode); err != nil {
				return err
			}

			// 工作目录必须显式设置（FR-LIB-03）：结果需自动入库回填 citeKey
			if err := requireWorkDir(cfg); err != nil {
				return err
			}

			opts := core.SearchOptions{
				MaxResults:     maxResults,
				Year:           year,
				Since:          computeSince(sinceFlag, years),
				Mode:           mode,
				KeepNoAbstract: keepNoAbstract,
			}
			if excludeFlag != "" {
				for _, e := range strings.Split(excludeFlag, ",") {
					e = strings.TrimSpace(e)
					if e != "" {
						opts.Exclude = append(opts.Exclude, e)
					}
				}
			}
			if sourcesFlag != "" {
				for _, s := range strings.Split(sourcesFlag, ",") {
					s = strings.TrimSpace(s)
					if s != "" {
						opts.Sources = append(opts.Sources, s)
					}
				}
			}
			// 源名校验：未知源直接报错，避免静默忽略（返回空结果误导调用方）
			for _, name := range opts.Sources {
				if _, ok := reg.Get(name); !ok {
					return &paramError{msg: fmt.Sprintf("search: 未知源 %q（可用源见 litkit sources）", name)}
				}
			}

			res, err := s.Search(cmd.Context(), query, opts)
			if err != nil {
				return err
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
			// 部分源失败但部分成功：退出码 3（api.md §1.1）
			if len(res.Errors) > 0 && res.Total > 0 {
				os.Exit(exitCodePartialFailure)
			}
			// 全部源失败：返回错误，由 main 以退出码 1 结束
			if len(res.Errors) > 0 {
				return fmt.Errorf("search: 所有源均失败")
			}
			return nil
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
