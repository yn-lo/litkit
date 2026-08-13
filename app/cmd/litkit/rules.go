// rules.go 实现 `litkit rules` 子命令。
//
// 输出规则注册表的精简视图（ID/名称/类别/适用语言与类型/模式/方法），
// 供使用者查询"有哪些规则、各自干什么"，为 spec 的 skip_rules 配置提供依据。
package main

import (
	"github.com/spf13/cobra"

	"litkit/internal/lint"
)

// ruleInfo 规则的可 JSON 序列化视图（lint.Rule 含 func 字段无法直接序列化）。
type ruleInfo struct {
	ID       string   `json:"id"`
	Name     string   `json:"name"`
	Category string   `json:"category"` // language|structure|statistics|punctuation|style|citation|heading|boast_words|word_counts|todo
	Langs    []string `json:"langs"`    // 适用语言（zh/en）
	Types    []string `json:"types"`    // 适用论文类型（空=全部）
	Method   string   `json:"method"`   // A=全自动 | S=脚本初筛+人工
	From     string   `json:"from"`     // 启用模式：chapter|draft|final
	Fixable  bool     `json:"fixable"`  // 是否可自动修正（litkit fix）
}

// buildRulesOutput 投影规则注册表为可序列化视图。
func buildRulesOutput() []ruleInfo {
	rules := lint.AllRules()
	out := make([]ruleInfo, len(rules))
	for i, r := range rules {
		out[i] = ruleInfo{
			ID:       r.ID,
			Name:     r.Name,
			Category: string(r.Category),
			Langs:    r.Langs,
			Types:    r.Types,
			Method:   string(r.Method),
			From:     string(r.From),
			Fixable:  r.Fix != nil,
		}
	}
	return out
}

// newRulesCmd 构造 `litkit rules` 子命令（纯查询，不依赖工作目录/文献库）。
func newRulesCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "rules",
		Short: "列出全部验证规则（ID/名称/类别/适用类型/模式）",
		Long: `litkit rules —— 规则注册表查询

输出每条验证规则的 ID、名称、检查类别、适用语言与论文类型、执行方法与启用模式。
为空（[]）的 types 表示全部类型适用；from 表示该规则从哪个模式起启用
（chapter → draft → final 递增）。

配置 manuscript-spec.yaml 的 skip_rules 前，可先运行本命令确认规则 ID。`,
		Args: cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			return printJSON(struct {
				Rules []ruleInfo `json:"rules"`
			}{Rules: buildRulesOutput()})
		},
	}
}
