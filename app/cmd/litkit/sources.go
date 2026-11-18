package main

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"litkit/internal/config"
	"litkit/internal/sources"
)

// sourceInfo sources 命令的单个源描述条目，对齐 api.md §1.2 输出契约。
type sourceInfo struct {
	Name        string `json:"name"`
	Searchable  bool   `json:"searchable"`
	HasAbstract bool   `json:"hasAbstract"`
	Enabled     bool   `json:"enabled"`
}

// newSourcesCmd 构造 `litkit sources` 子命令。
//
// 输出注册表中全部源及其能力（检索/摘要/是否参与默认检索），与平台能力矩阵保持一致
// （platform-matrix.md，NFR-MAINT-04）。enabled 反映 .env 的
// LITKIT_SEARCH_SOURCES / LITKIT_SEARCH_EXCLUDE_SOURCES 配置（未显式 -s 时的默认源）。
func newSourcesCmd(reg *sources.Registry, cfg *config.Config) *cobra.Command {
	return &cobra.Command{
		Use:   "sources",
		Short: "列出已注册的检索源",
		Long: `列出已注册的检索源及其能力（检索/摘要/是否参与默认检索）。

输出：{ sources: [{ name, searchable, hasAbstract, enabled }] }

enabled 表示该源是否参与未显式 -s 的默认检索，受 .env 的
LITKIT_SEARCH_SOURCES / LITKIT_SEARCH_EXCLUDE_SOURCES 控制。

平台能力矩阵详见 .harness/specs/reference/platform-matrix.md。`,
		RunE: func(_ *cobra.Command, _ []string) error {
			list := reg.List()
			enabled := make(map[string]bool, len(list))
			defaults, warns := resolveSearchSources(nil, cfg, reg.Names())
			for _, w := range warns {
				fmt.Fprintf(os.Stderr, "litkit: %s\n", w)
			}
			for _, n := range defaults {
				enabled[n] = true
			}
			out := struct {
				Sources []sourceInfo `json:"sources"`
			}{
				Sources: make([]sourceInfo, 0, len(list)),
			}
			for _, s := range list {
				out.Sources = append(out.Sources, sourceInfo{
					Name:        s.Name(),
					Searchable:  true, // 注册到 registry 即视为可检索
					HasAbstract: s.HasAbstract(),
					Enabled:     enabled[s.Name()],
				})
			}
			enc := json.NewEncoder(os.Stdout)
			enc.SetIndent("", "  ")
			return enc.Encode(out)
		},
	}
}
