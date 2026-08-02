package main

import (
	"encoding/json"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"litkit/internal/core"
)

// exitCodePartialFailure 部分源失败但部分成功的退出码（api.md §1.1）。
const exitCodePartialFailure = 3

// newSearchCmd 构造 `litkit search` 子命令。
//
// 接口契约见 PRD 7.1 与 api.md §1.2。
// 退出码：3 表示部分源失败但部分成功（api.md §1.1）。
func newSearchCmd(s *core.Searcher) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "search <query> [-s sources] [-n N] [-y year] [--keep-no-abstract]",
		Short: "跨源并发检索文献",
		Long: `跨源并发检索文献，输出标准化元数据 + 摘要，结果按 DOI→title→id 三级去重。

输出：{ total, sourceResults, errors, papers[] }`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			query := args[0]
			sourcesFlag, _ := cmd.Flags().GetString("sources")
			maxResults, _ := cmd.Flags().GetInt("max-results")
			year, _ := cmd.Flags().GetInt("year")
			keepNoAbstract, _ := cmd.Flags().GetBool("keep-no-abstract")

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
			enc := json.NewEncoder(os.Stdout)
			enc.SetIndent("", "  ")
			if err := enc.Encode(res); err != nil {
				return err
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
	return cmd
}
