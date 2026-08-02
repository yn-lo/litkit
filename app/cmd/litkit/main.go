// litkit 是国内学术写作场景的论文工具包 CLI。
//
// 子命令对应 PRD 7.1 节定义的接口；输出默认 JSON（FR-IFACE-01）。
// 退出码：0 成功 / 1 运行错误 / 2 参数错误 / 3 部分源失败但部分成功（api.md §1.1）。
package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

func main() {
	if err := newRootCmd().Execute(); err != nil {
		// cobra 已自行输出错误信息；按 api.md 退出码语义
		os.Exit(1)
	}
}

// newRootCmd 构造根命令并注册全部子命令。
//
// 子命令清单与 PRD 7.1 一致；新增功能须同步 MCP 注册（FR-IFACE-03）。
func newRootCmd() *cobra.Command {
	d := loadDeps()
	root := &cobra.Command{
		Use:   "litkit",
		Short: "国内学术写作场景的论文工具包",
		Long: `litkit —— 国内学术写作场景的论文工具包

跨源检索文献（摘要工作流）、生成规范引用（GB/T 7714—2025 / APA / IEEE）、
排版手稿、AI 撰写合规门禁。

CLI 为第一接口（FR-IFACE-01）；MCP Server 为可选第二接口。
输出默认 JSON，可被 AI shell 调用。`,
		SilenceUsage: true,
	}
	root.AddCommand(
		newSearchCmd(d.searcher),
		newSourcesCmd(d.registry),
		newMetadataCmd(),
		newManuscriptCmd(),
		newExportCmd(),
		newLibraryCmd(d.store),
		newLintCmd(),
		newVerifyCmd(),
	)
	return root
}

// notImplemented 返回一个统一形态的"未实现"错误。
// 用于后续里程碑（M3/M4）的占位子命令。
func notImplemented(name string) error {
	return fmt.Errorf("litkit %s: 暂未实现（参见 .harness/specs/plans/roadmap.md）", name)
}
