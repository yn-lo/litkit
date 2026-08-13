// litkit 是国内学术写作场景的论文工具包 CLI。
//
// 子命令对应 PRD 7.1 节定义的接口；输出默认 JSON（FR-IFACE-01）。
// 退出码：0 成功 / 1 运行错误 / 2 参数错误 / 3 部分源失败但部分成功（api.md §1.1）。
package main

import (
	"errors"
	"fmt"
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
			fmt.Fprintln(os.Stderr, "错误:", pe.msg)
			return 2
		}
		fmt.Fprintln(os.Stderr, "错误:", err.Error())
		return 1
	}
	return 0
}

// newRootCmd 构造根命令并注册全部子命令。
func newRootCmd(d *deps) *cobra.Command {
	root := &cobra.Command{
		Use:   "litkit",
		Short: "国内学术写作场景的论文工具包",
		Long: `litkit —— 国内学术写作场景的论文工具包

跨源检索文献（摘要工作流）、生成规范引用（GB/T 7714—2025 / APA / IEEE）、
排版手稿、AI 撰写合规门禁。

输出默认 JSON，可被 AI shell 调用。`,
		SilenceUsage:  true,
		SilenceErrors: true,
		Version:       buildinfo.Version,
		RunE: func(cmd *cobra.Command, _ []string) error {
			cmd.Println(cmd.UsageString())
			return &paramError{msg: "请使用 --help 查看使用方法"}
		},
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
		newRulesCmd(),
		newFixCmd(),
		newLintCmd(d.cfg),
		newVerifyCmd(d.cfg, d.store),
	)
	return root
}
