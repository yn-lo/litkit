package main

import (
	"encoding/json"
	"os"

	"github.com/spf13/cobra"

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
// 输出注册表中全部源及其能力（检索/摘要/启用），与平台能力矩阵保持一致
// （platform-matrix.md，NFR-MAINT-04）。
func newSourcesCmd(reg *sources.Registry) *cobra.Command {
	return &cobra.Command{
		Use:   "sources",
		Short: "列出已注册的检索源",
		Long: `列出已注册的检索源及其能力（检索/摘要/启用）。

输出：{ sources: [{ name, searchable, hasAbstract, enabled }] }

平台能力矩阵详见 .harness/specs/reference/platform-matrix.md。`,
		RunE: func(_ *cobra.Command, _ []string) error {
			list := reg.List()
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
					Enabled:     true, // 一期源默认启用；二期可选源按 key 启用
				})
			}
			enc := json.NewEncoder(os.Stdout)
			enc.SetIndent("", "  ")
			return enc.Encode(out)
		},
	}
}
