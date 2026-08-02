package main

import (
	"os"
	"strings"

	"github.com/spf13/cobra"

	"litkit/internal/core"
	"litkit/internal/model"
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

// newSearchCmd 构造 `litkit search` 子命令。
//
// 默认输出 PaperSummary（FR-IFACE-04）；--full 输出完整 Paper。
// 退出码：3 表示部分源失败但部分成功（api.md §1.1）。
func newSearchCmd(s *core.Searcher) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "search <query> [-s sources] [-n N] [-y year] [--keep-no-abstract] [--full]",
		Short: "跨源并发检索文献",
		Long: `跨源并发检索文献，默认输出精简视图（citeKey/title/firstAuthor/year/abstract），
面向 AI agent 调用（FR-IFACE-04）。结果按 DOI→title→id 三级去重，年份倒序。

--full 输出完整元数据（含 doi/pmid/arxivId/url/venue/全部作者等），供调试。`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			query := args[0]
			sourcesFlag, _ := cmd.Flags().GetString("sources")
			maxResults, _ := cmd.Flags().GetInt("max-results")
			year, _ := cmd.Flags().GetInt("year")
			keepNoAbstract, _ := cmd.Flags().GetBool("keep-no-abstract")
			full, _ := cmd.Flags().GetBool("full")

			opts := core.SearchOptions{
				MaxResults:     maxResults,
				Year:           year,
				KeepNoAbstract: keepNoAbstract,
			}
			if sourcesFlag != "" {
				for _, s := range strings.Split(sourcesFlag, ",") {
					s = strings.TrimSpace(s)
					if s != "" {
						opts.Sources = append(opts.Sources, s)
					}
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
					Errors:        res.Errors,
					Papers:        model.SummarizePapers(res.Papers),
				}); perr != nil {
					return perr
				}
			}
			// 部分源失败：退出码 3（api.md §1.1）
			if len(res.Errors) > 0 {
				os.Exit(exitCodePartialFailure)
			}
			return nil
		},
	}
	cmd.Flags().StringP("sources", "s", "", "逗号分隔源列表（litkit sources 查看）")
	cmd.Flags().IntP("max-results", "n", 0, "每源最大条数（0=源默认 5）")
	cmd.Flags().IntP("year", "y", 0, "年份过滤（源支持时）")
	cmd.Flags().Bool("keep-no-abstract", false, "保留无摘要论文（默认过滤，FR-SEARCH-03）")
	cmd.Flags().Bool("full", false, "输出完整元数据（默认精简视图，FR-IFACE-04）")
	return cmd
}
