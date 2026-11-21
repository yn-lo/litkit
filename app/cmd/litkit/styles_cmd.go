// styles_cmd.go 实现 `litkit styles` 子命令：列出、查看、导出内置 CSL 样式。
//
// 内置样式随二进制分发；用户样式放在 WORK_DIR/styles/*.csl，按文件名 stem 引用。
package main

import (
	"github.com/spf13/cobra"

	"litkit/internal/config"
	"litkit/internal/csl"
)

type stylesListOutput struct {
	Styles []csl.StyleInfo `json:"styles"`
}

type styleShowOutput struct {
	Name    string `json:"name"`
	Builtin bool   `json:"builtin"`
	Content string `json:"content"`
}

type styleExportOutput struct {
	Name string `json:"name"`
	Path string `json:"path"`
}

// newStylesCmd 构造 `litkit styles`。
func newStylesCmd(cfg *config.Config) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "styles",
		Short: "引用样式管理（list | show | export）",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return &paramError{msg: "请使用 styles list | show | export"}
		},
	}
	cmd.AddCommand(
		newStylesListCmd(cfg),
		newStylesShowCmd(cfg),
		newStylesExportCmd(cfg),
	)
	return cmd
}

func newStylesListCmd(cfg *config.Config) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "列出内置与用户样式",
		Args:  cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			return printJSON(stylesListOutput{Styles: csl.List(userStylesDir(cfg))})
		},
	}
}

func newStylesShowCmd(cfg *config.Config) *cobra.Command {
	return &cobra.Command{
		Use:   "show <name>",
		Short: "输出样式原始 CSL 内容",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			style, err := csl.Show(args[0], userStylesDir(cfg))
			if err != nil {
				return &paramError{msg: err.Error()}
			}
			return printJSON(styleShowOutput{Name: style.Name, Builtin: style.Builtin, Content: string(style.Content)})
		},
	}
}

func newStylesExportCmd(cfg *config.Config) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "export <name>",
		Short: "导出内置样式为 <name>-custom.csl（便于自定义）",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			outDir, _ := cmd.Flags().GetString("out")
			if outDir == "" {
				outDir = userStylesDir(cfg)
			}
			if outDir == "" {
				return &paramError{msg: "未设置 LITKIT_WORK_DIR：请用 --out 指定导出目录"}
			}
			path, err := csl.Export(args[0], outDir)
			if err != nil {
				return &paramError{msg: err.Error()}
			}
			return printJSON(styleExportOutput{Name: args[0], Path: path})
		},
	}
	cmd.Flags().StringP("out", "o", "", "导出目录（默认 WORK_DIR/styles）")
	return cmd
}
