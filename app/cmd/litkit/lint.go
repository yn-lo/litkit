// lint.go 实现 `litkit lint` 子命令（M4 撰写约束基础设施，FR-LINT-01/02）。
//
// 与 `litkit init` 区别：lint 仅生成 .litkit/ 论文类型目录与 AGENTS.md 撰写段，
// 不生成 .env、不初始化文献库（用于已有工作目录补齐约束设施）。
package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"litkit/internal/config"
	"litkit/internal/lint"
)

// newLintCmd 构造 `litkit lint` 子命令。
func newLintCmd(cfg *config.Config) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "lint",
		Short: "撰写约束基础设施（init）",
	}
	cmd.AddCommand(newLintInitCmd(cfg))
	return cmd
}

// newLintInitCmd 构造 `litkit lint init [project_dir]`。
func newLintInitCmd(cfg *config.Config) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "init [project_dir] --type review|empirical --lang zh|en",
		Short: "初始化论文类型目录（.litkit/<type>/ 的 yaml + AGENTS.md）",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := requireWorkDir(cfg); err != nil {
				return err
			}
			force, _ := cmd.Flags().GetBool("force")
			lang, _ := cmd.Flags().GetString("lang")
			paperType, _ := cmd.Flags().GetString("type")
			journal, _ := cmd.Flags().GetString("journal")
			if lang != lint.LangZH && lang != lint.LangEN {
				return &paramError{msg: fmt.Sprintf("lint init: 无效 --lang %q（可选 zh|en）", lang)}
			}
			if paperType != "" && paperType != lint.PaperTypeReview && paperType != lint.PaperTypeEmpirical {
				return &paramError{msg: fmt.Sprintf("lint init: 无效 --type %q（可选 review|empirical）", paperType)}
			}
			dir := cfg.WorkDir
			if len(args) > 0 {
				dir = args[0]
			}

			// 论文类型目录
			created, err := lint.InitPaperType(dir, paperType, lang, force)
			if err != nil {
				return err
			}

			// 加载 spec
			specPath := lint.SpecPath(dir, paperType, lang)
			spec, err := lint.LoadSpec(specPath)
			if err != nil {
				spec = lint.SpecForType(paperType, lang)
			}
			if journal != "" {
				spec.Journal = journal
				_ = lint.WriteSpec(specPath, spec)
			}

			// 类型 AGENTS.md
			agentsDir := lint.PapersDirPath(dir, paperType, lang)
			if err := os.MkdirAll(agentsDir, initDirPerm); err != nil {
				return err
			}
			agentsPath := filepath.Join(agentsDir, "AGENTS.md")
			agentsContent := lint.TypeAgentsMD(spec)
			if !fileExists(agentsPath) || force {
				if err := os.WriteFile(agentsPath, []byte(agentsContent), initFilePerm); err != nil {
					return err
				}
				created = append(created, filepath.Join(lint.LitkitDir, lint.TypeLangDir(paperType, lang), "AGENTS.md"))
			}

			return printJSON(struct {
				Status    string   `json:"status"`
				Files     []string `json:"files"`
				NextSteps []string `json:"nextSteps"`
			}{
				Status:    "ok",
				Files:     created,
				NextSteps: []string{"阅读 .litkit/<type>/AGENTS.md 获取撰写规定", "成稿后运行 litkit verify 检查"},
			})
		},
	}
	cmd.Flags().Bool("force", false, "覆盖已存在的文件")
	cmd.Flags().String("lang", lint.LangZH, "撰写语言 zh|en")
	cmd.Flags().String("type", lint.PaperTypeEmpirical, "论文类型 review|empirical")
	cmd.Flags().String("journal", "", "目标期刊名称（写入 spec）")
	return cmd
}
