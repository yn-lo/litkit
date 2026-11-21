// metadata_cmd.go 实现 `litkit metadata` 命令（FR-REF-02）。
//
// 按标识符（doi | pmid | arxiv | title）反查论文元数据，输出 Paper | null。
package main

import (
	"errors"

	"github.com/spf13/cobra"

	"litkit/internal/core"
)

// errNoFetcher 元数据反查器不可用（理论不可达，防御性提示）。
var errNoFetcher = errors.New("元数据反查器不可用")

// newMetadataCmd 构造 `litkit metadata <id_type> <identifier>`。
// 未命中输出 null；网络/解析错误退出码 1。
func newMetadataCmd(f *core.MetadataFetcher) *cobra.Command {
	return &cobra.Command{
		Use:   "metadata <id_type> <identifier>",
		Short: "按标识符取回论文元数据（doi | pmid | arxiv | title）",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			if f == nil {
				return errNoFetcher
			}
			if !validIDType(args[0]) {
				return &paramError{msg: "metadata: 未知 id_type \"" + args[0] + "\"（可选 doi|pmid|arxiv|title）"}
			}
			p, err := f.Fetch(cmd.Context(), args[0], args[1])
			if err != nil {
				return err
			}
			return printJSON(p) // 未命中时 p 为 nil → 输出 null
		},
	}
}

// validIDType 校验 metadata 标识符类型（入口层参数校验，退出码 2）。
func validIDType(idType string) bool {
	switch idType {
	case "doi", "pmid", "arxiv", "title":
		return true
	default:
		return false
	}
}
