// mcp_cmd.go 实现 `litkit mcp` 子命令（MCP Server，stdio 传输，可选第二接口 FR-IFACE-03）。
//
// 工具清单与 CLI 子命令一一对应，共享核心实现（C8）：同一输入必然同一输出。
// 供 MCP 客户端（如 Claude Desktop / IDE）通过 stdio 启动。
package main

import (
	"context"
	"errors"
	"os"
	"os/signal"

	"github.com/spf13/cobra"

	"litkit/internal/mcp"
)

// newMcpCmd 构造 `litkit mcp` 子命令。
func newMcpCmd(d *deps) *cobra.Command {
	return &cobra.Command{
		Use:   "mcp",
		Short: "启动 MCP Server（stdio 传输，可选第二接口）",
		Long: `litkit mcp —— 以 MCP 协议暴露 litkit 全部能力

通过标准输入输出（stdio）与 MCP 客户端通信。工具与 CLI 命令一一对应，
共享同一核心实现，保证同输入同输出（FR-IFACE-03）。`,
		Args: cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
			defer stop()
			if err := mcp.Run(ctx, mcp.Deps{
				Cfg:      d.cfg,
				Registry: d.registry,
				Store:    d.store,
				Searcher: d.searcher,
				Fetcher:  d.fetcher,
				Fulltext: d.fulltext,
			}); err != nil && !errors.Is(err, context.Canceled) {
				return err
			}
			return nil
		},
	}
}
