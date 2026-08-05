// litkit 是国内学术写作场景的论文工具包 CLI。
//
// 子命令对应 PRD 7.1 节定义的接口；输出默认 JSON（FR-IFACE-01）。
// 退出码：0 成功 / 1 运行错误 / 2 参数错误 / 3 部分源失败但部分成功（api.md §1.1）。
package main

import (
	"errors"
	"os"

	"github.com/spf13/cobra"

	"litkit/internal/buildinfo"
)

// paramError 表示参数校验错误（退出码 2）。
type paramError struct{ msg string }

func (e *paramError) Error() string { return e.msg }

func main() {
	os.Exit(run())
}

// run 执行 CLI 并返回退出码（避免 os.Exit 跳过 defer）。
func run() int {
	d := loadDeps()
	defer d.Close()
	if err := newRootCmd(d).Execute(); err != nil {
		var pe *paramError
		if errors.As(err, &pe) {
			return 2
		}
		return 1
	}
	return 0
}

// newRootCmd 构造根命令并注册全部子命令。
//
// 子命令清单与 PRD 7.1 一致；新增功能须同步 MCP 注册（FR-IFACE-03）。
func newRootCmd(d *deps) *cobra.Command {
	root := &cobra.Command{
		Use:   "litkit",
		Short: "国内学术写作场景的论文工具包",
		Long: `litkit —— 国内学术写作场景的论文工具包

跨源检索文献（摘要工作流）、生成规范引用（GB/T 7714—2025 / APA / IEEE）、
排版手稿、AI 撰写合规门禁。

CLI 为第一接口（FR-IFACE-01）；MCP Server 为可选第二接口。
输出默认 JSON，可被 AI shell 调用。`,
		SilenceUsage: true,
		Version:      buildinfo.Version, // 启用 --version / version 子命令（goreleaser ldflags 注入）
	}
	root.AddCommand(
		newInitCmd(d.cfg),
		newSearchCmd(d.searcher, d.registry, d.cfg),
		newSourcesCmd(d.registry),
		newMetadataCmd(d.fetcher),
		newFetchCmd(d.store, d.fulltext),
		newManuscriptCmd(d.store, d.fetcher, d.cfg),
		newExportCmd(),
		newLibraryCmd(d.store),
		newLintCmd(d.cfg),
		newVerifyCmd(d.cfg),
		newMcpCmd(d),
	)
	return root
}
