package main

import (
	"github.com/spf13/cobra"
)

// 以下子命令为后续里程碑（M3/M4）实现，M1 仅占位以保证 --help 完整。

func newMetadataCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "metadata <id_type> <identifier>",
		Short: "按标识符取回论文元数据（doi | pmid | arxiv | title）",
		Args:  cobra.ExactArgs(2),
		RunE: func(_ *cobra.Command, _ []string) error {
			return notImplemented("metadata")
		},
	}
}

func newManuscriptCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "manuscript <draft.md>",
		Short: "手稿流水线（解析占位符、生成引用与产物）",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, _ []string) error {
			return notImplemented("manuscript")
		},
	}
	cmd.Flags().String("lang", "zh", "写作语言模式 zh|en")
	cmd.Flags().StringP("style", "s", "", "引用样式（zh: gb7714-2025；en: apa / ieee）")
	cmd.Flags().Bool("docx", false, "生成 Word（需 Pandoc）")
	cmd.Flags().StringP("output-dir", "o", "", "输出目录（默认 WORK_DIR）")
	return cmd
}

func newExportCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "export <papers.json>",
		Short: "批量导出引用（bibtex | ris | text）",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, _ []string) error {
			return notImplemented("export")
		},
	}
	cmd.Flags().StringP("format", "f", "bibtex", "导出格式 bibtex|ris|text")
	cmd.Flags().StringP("style", "s", "", "引用样式")
	return cmd
}

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
