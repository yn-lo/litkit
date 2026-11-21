// lint.go 实现 `litkit lint` 子命令（M4 撰写约束基础设施，FR-LINT-01/02）。
//
// 与 `litkit init` 区别：lint 仅生成 .litkit/ 论文类型目录，
// 不生成 .env、不初始化文献库（用于已有工作目录补齐约束设施）。
package main

import (
	"fmt"
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
		Use:   "init [project_dir] --type review|empirical|book --lang zh|en",
		Short: "初始化论文类型目录（.litkit/<type>/ 的 manuscript-spec.yaml）",
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
			if paperType != "" && !lint.IsValidPaperType(paperType) {
				return &paramError{msg: fmt.Sprintf("lint init: 无效 --type %q（可选 %s）", paperType, lint.PaperTypesLabel())}
			}
			// 相对 project_dir 锚定工作目录（与其余路径「相对 .env 目录解析」一致），
			// 避免从其他 cwd 调用时把 spec 写到错误位置。
			dir := cfg.WorkDir
			if len(args) > 0 {
				dir = args[0]
				if !filepath.IsAbs(dir) {
					dir = filepath.Join(cfg.WorkDir, dir)
				}
			}

			// 论文类型目录
			created, err := lint.InitPaperType(dir, paperType, lang, force)
			if err != nil {
				return err
			}

			// journal 覆盖（只替换 journal 行，保留模板顶部撰写规定注释）
			if journal != "" {
				if err := lint.SetSpecJournal(lint.SpecPath(dir, paperType, lang), journal); err != nil {
					return fmt.Errorf("lint init: 写入期刊配置失败: %w", err)
				}
			}

			return printJSON(struct {
				Status    string   `json:"status"`
				Files     []string `json:"files"`
				NextSteps []string `json:"nextSteps"`
			}{
				Status:    "ok",
				Files:     created,
				NextSteps: []string{"阅读 .litkit/<type>/manuscript-spec.yaml 获取撰写规定", "成稿后运行 litkit verify 检查"},
			})
		},
	}
	cmd.Flags().Bool("force", false, "覆盖已存在的文件")
	cmd.Flags().String("lang", defaultLang(cfg), "撰写语言 zh|en（默认 LITKIT_LANG）")
	cmd.Flags().String("type", lint.PaperTypeEmpirical, fmt.Sprintf("论文类型 %s", lint.PaperTypesLabel()))
	cmd.Flags().String("journal", "", "目标期刊名称（写入 spec）")
	return cmd
}
