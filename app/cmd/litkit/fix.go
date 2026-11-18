// fix.go 实现 `litkit fix` 子命令：自动修正可修的格式违规（原地覆盖）。
package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"litkit/internal/config"
	"litkit/internal/lint"
)

// newFixCmd 构造 `litkit fix <file.md> [file2.md ...]`。
func newFixCmd(cfg *config.Config) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "fix <file.md> [file2.md ...]",
		Short: "自动修正可修的格式违规（原地覆盖）",
		Long: `litkit fix —— 自动修正

对 Markdown 文稿应用可自动修正的规则（字符级确定性替换）：
全半角标点（R3.1）、直引号（R3.2）、P 值格式（R2.1）、加粗标记（R1.4）、
标题冒号（R7.1）、标题末尾标点（R1.2）、引用位置（R6.1）、数字范围（R3.3）。
语义/结构类规则不可自动修，需人工处理（litkit verify 会提示）。

文件原地覆盖；修复后建议再跑 litkit verify 验证。`,
		Args: cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ruleFlag, _ := cmd.Flags().GetString("rule")
			skipFlag, _ := cmd.Flags().GetString("skip")
			// 加载阈值配置（供 R2.1 P 值小数位等规则读取；未配置时自动回落默认）。
			spec, err := loadVerifySpec(cfg.WorkDir, "", "")
			if err != nil {
				return err
			}
			// spec 级 skip_rules 与 verify 保持一致：永久跳过，等效每次携带 --skip
			skip := append(splitCSV(skipFlag), spec.SkipRules...)
			rules := filterFixRules(lint.FixableRules(), splitCSV(ruleFlag), skip, spec)
			if len(rules) == 0 {
				return &paramError{msg: "fix: 无可用的可修规则（检查 --rule/--skip 与 spec 的 skip_rules）"}
			}
			files := map[string]lint.FixReport{}
			for _, p := range args {
				data, err := os.ReadFile(p)
				if err != nil {
					return fmt.Errorf("fix: 读取 %s 失败: %w", p, err)
				}
				content := string(data)
				// 统一按 LF 应用修复（verify 也按 LF 解析），写回时恢复原行尾风格。
				hadCRLF := strings.Contains(content, "\r\n")
				if hadCRLF {
					content = strings.ReplaceAll(content, "\r\n", "\n")
				}
				fixed, rep := lint.ApplyFixes(content, spec, rules)
				if rep.Total() == 0 {
					continue
				}
				if hadCRLF {
					fixed = strings.ReplaceAll(fixed, "\n", "\r\n")
				}
				var perm os.FileMode = initFilePerm
				if info, err := os.Stat(p); err == nil {
					perm = info.Mode()
				}
				if err := os.WriteFile(p, []byte(fixed), perm); err != nil {
					return fmt.Errorf("fix: 写入 %s 失败: %w", p, err)
				}
				files[p] = rep
			}
			return printJSON(struct {
				Files map[string]lint.FixReport `json:"files"` // 被修正的文件 → 规则修正统计
			}{Files: files})
		},
	}
	cmd.Flags().String("rule", "", "仅修正指定规则（逗号分隔，如 R3.1,R3.2）")
	cmd.Flags().String("skip", "", "跳过指定规则（逗号分隔）")
	return cmd
}

// filterFixRules 按 --rule/--skip 与 spec 的 lang/type 过滤可修规则。
//
// 与 verify 的规则筛选口径一致：zh 专属规则（如 R3.1/R3.2/R3.3）不得在 en 文稿上执行，
// 否则英文数字范围会被改成全角波浪线等中文排版形式。
func filterFixRules(rules []lint.Rule, only, skip []string, spec *lint.ManuscriptSpec) []lint.Rule {
	out := make([]lint.Rule, 0, len(rules))
	for _, r := range rules {
		if !fixRuleApplies(r, spec) {
			continue
		}
		if len(only) > 0 && !containsStr(only, r.ID) {
			continue
		}
		if containsStr(skip, r.ID) {
			continue
		}
		out = append(out, r)
	}
	return out
}

// fixRuleApplies 判断规则是否适用于 spec 的语言与论文类型（空类型=全部适用）。
func fixRuleApplies(r lint.Rule, spec *lint.ManuscriptSpec) bool {
	if spec == nil {
		return true
	}
	if len(r.Langs) > 0 && !containsStr(r.Langs, spec.Lang) {
		return false
	}
	if len(r.Types) > 0 && !containsStr(r.Types, spec.PaperType) {
		return false
	}
	return true
}

// containsStr 判断切片是否含目标字符串。
func containsStr(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}
