package main

import (
	"github.com/spf13/cobra"
)

// lint 基础设施（M4 撰写 harness）为占位，M4 实现。
func newLintCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "lint",
		Short: "撰写约束基础设施（init）",
	}
	cmd.AddCommand(&cobra.Command{
		Use:   "init [project_dir]",
		Short: "初始化宿主项目约束基础设施",
		RunE:  func(_ *cobra.Command, _ []string) error { return notImplemented("lint init") },
	})
	return cmd
}
