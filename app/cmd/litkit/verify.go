package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"litkit/internal/config"
	"litkit/internal/lint"
)

// newVerifyCmd 构造 verify 子命令（FR-LINT-05 事后验证）。
//
// 退出码（verify 专用语义）：
//   - 0 全部通过（exitHint=pass）或仅 S 类需人工（exitHint=manual_review）
//   - 1 有 A 类违规（exitHint=fix_and_rerun），AI 应自动修复后重跑
func newVerifyCmd(cfg *config.Config) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "verify <file.md> [file2.md ...]",
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

			// 加载阈值配置（.litkit/specs/manuscript-spec.yaml）
			spec := loadVerifySpec(cfg.WorkDir)

			opts := lint.Options{
				Lang: lang,
				Mode: mode,
				Only: splitCSV(ruleFlag),
				Skip: splitCSV(skipFlag),
			}

			report, err := lint.RunFiles(args, spec, opts)
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
	cmd.Flags().String("rule", "", "仅运行指定规则（逗号分隔，如 R2.1,R7.1）")
	cmd.Flags().String("skip", "", "跳过指定规则（逗号分隔）")
	return cmd
}

// loadVerifySpec 从工作目录加载 manuscript-spec.yaml；不存在时用默认值。
func loadVerifySpec(workDir string) *lint.ManuscriptSpec {
	p := lint.SpecPath(workDir)
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
