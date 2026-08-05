package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"litkit/internal/config"
	"litkit/internal/lint"
	"litkit/internal/storage"
)

// newVerifyCmd 构造 verify 子命令（FR-LINT-05 事后验证；R5.6 引用防伪查库）。
//
// 退出码（verify 专用语义）：
//   - 0 全部通过（exitHint=pass）或仅 S 类需人工（exitHint=manual_review）
//   - 1 有 A 类违规（exitHint=fix_and_rerun），AI 应自动修复后重跑
func newVerifyCmd(cfg *config.Config, store *storage.Store) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "verify <file.md> [file2.md ...] --type review|empirical --lang zh|en",
		Short: "验证文稿合规性（A/S 类规则自动检查）",
		Long: `litkit verify —— 事后合规验证

对 Markdown 文稿执行规则检查，输出 JSON 报告。
规则按模式递增启用：chapter（结构）→ draft（+数据/标点/引用）→ final（+字数/行文）。

退出码：0=通过或仅需人工复核；1=有 A 类违规需修复。
AI 应读取 JSON 中 exitHint 字段决定下一步动作。`,
		Args: cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := requireWorkDir(cfg); err != nil {
				return err
			}
			lang, _ := cmd.Flags().GetString("lang")
			modeStr, _ := cmd.Flags().GetString("mode")
			paperType, _ := cmd.Flags().GetString("type")
			ruleFlag, _ := cmd.Flags().GetString("rule")
			skipFlag, _ := cmd.Flags().GetString("skip")

			mode := lint.Mode(modeStr)
			switch mode {
			case lint.ModeChapter, lint.ModeDraft, lint.ModeFinal:
			default:
				return &paramError{msg: fmt.Sprintf("verify: 无效 mode %q（可选 chapter|draft|final）", modeStr)}
			}
			if lang != "zh" && lang != "en" {
				return &paramError{msg: fmt.Sprintf("verify: 无效 lang %q（可选 zh|en）", lang)}
			}
			if paperType != "" && paperType != lint.PaperTypeReview && paperType != lint.PaperTypeEmpirical {
				return &paramError{msg: fmt.Sprintf("verify: 无效 type %q（可选 review|empirical）", paperType)}
			}

			// 加载阈值配置
			spec := loadVerifySpec(cfg.WorkDir, paperType, lang)

			opts := lint.Options{
				Lang:      lang,
				Mode:      mode,
				PaperType: paperType,
				Only:      splitCSV(ruleFlag),
				Skip:      splitCSV(skipFlag),
			}

			report, err := lint.RunFilesWithStore(args, spec, opts, store)
			if err != nil {
				return fmt.Errorf("verify: %w", err)
			}

			out, _ := json.MarshalIndent(report, "", "  ")
			fmt.Println(string(out))

			if report.ExitHint == "fix_and_rerun" {
				os.Exit(1)
			}
			return nil
		},
	}
	cmd.Flags().String("lang", "zh", "写作语言 zh|en")
	cmd.Flags().String("mode", "draft", "验证模式 chapter|draft|final（递增启用规则）")
	cmd.Flags().String("type", "", "论文类型 review|empirical（空=从已有 spec 自动检测）")
	cmd.Flags().String("rule", "", "仅运行指定规则（逗号分隔，如 R2.1,R7.1）")
	cmd.Flags().String("skip", "", "跳过指定规则（逗号分隔）")
	return cmd
}

// loadVerifySpec 从工作目录加载 manuscript-spec.yaml。
// paperType/lang 为空时自动检测（仅一个类型时使用）。
func loadVerifySpec(workDir, paperType, lang string) *lint.ManuscriptSpec {
	if paperType == "" || lang == "" {
		types, err := lint.ListPaperTypes(workDir)
		if err == nil && len(types) == 1 {
			// 自动检测到唯一类型
			p := lint.TypeSpecPath(workDir, types[0])
			spec, err := lint.LoadSpec(p)
			if err == nil {
				return spec
			}
		}
		// 无法确定类型，用默认值
		fmt.Fprintln(os.Stderr, "litkit: 未指定 --type/--lang 且无法自动检测，使用默认阈值")
		return lint.DefaultSpec()
	}
	p := lint.SpecPath(workDir, paperType, lang)
	spec, err := lint.LoadSpec(p)
	if err != nil {
		fmt.Fprintf(os.Stderr, "litkit: 未找到 %s，使用默认阈值（可 litkit init 生成）\n", p)
		return lint.DefaultSpec()
	}
	return spec
}

// splitCSV 将逗号分隔字符串拆为切片（空串返回 nil）。
func splitCSV(s string) []string {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}
