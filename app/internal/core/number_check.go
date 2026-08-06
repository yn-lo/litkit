// number_check.go 引用句数字一致性检查（Layer 0，免费确定性规则）。
//
// 在引用句中提取所有数字（百分比、样本量、年份、p 值、系数等），
// 检查该数字是否在摘要中出现。如果引用句中的数字不存在于摘要中，
// 这可能是"幻觉引用"的信号——AI 编造了摘要中不存在的数据。
//
// 这是三层漏斗中最轻量的一层：O(n) 扫描，零外部依赖，零网络。
// 标记结果进入 verify 报告，触发 Layer 2 LLM 评分。

package core

import (
	"regexp"
	"strings"
)

// NumberIssue 数字一致性检查结果。
type NumberIssue struct {
	Number     string `json:"number"`     // 引用句中出现的数字原文
	InAbstract bool   `json:"inAbstract"` // 该数字是否在摘要中出现
}

// numberRe 匹配常见学术数字格式。
// 优先级：
//  1. 百分比（32.7%）
//  2. p 值（p < 0.05, p=0.01）
//  3. 带小数/千分位的数字（3,000, 3.14, 0.05）
//  4. 纯数字（42, 2024）
//
// ponytail: 不设尾部 \b，否则 % 结尾时 \b 不匹配会回退到不包含 % 的子串。
var numberRe = regexp.MustCompile(`\b\d+(?:,\d{3})*(?:\.\d+)?%?`)

// pValueRe 匹配 p 值表达式（p < 0.05, p=0.01, p < .001）。
var pValueRe = regexp.MustCompile(`(?i)p\s*[<>=]\s*\.?\d+(?:\.\d+)?`)

// sampleStatRe 匹配常见统计量模式（F(1,2)=3.45, t(28)=2.1, χ²=4.5）。
var sampleStatRe = regexp.MustCompile(`(?i)[Ftrχzw]+\s*\([^)]*\)\s*[=≈]\s*\d+(?:\.\d+)?`)

// CheckNumericConsistency 检查引用句中的数字是否在摘要中出现。
//
// 返回引用句中"摘要中不存在"的数字列表。
// 空列表表示所有数字均可从摘要中找到（或引用句中无数字）。
func CheckNumericConsistency(sentence, abstract string) []NumberIssue {
	abstractLower := strings.ToLower(abstract)
	var issues []NumberIssue

	// 提取引用句中的数字
	numbers := extractNumbers(sentence)
	seen := map[string]bool{}

	for _, num := range numbers {
		if seen[num] {
			continue
		}
		seen[num] = true

		inAbstract := strings.Contains(abstractLower, strings.ToLower(num))
		if !inAbstract {
			issues = append(issues, NumberIssue{Number: num, InAbstract: false})
		}
	}

	return issues
}

// extractNumbers 从文本中提取所有数字表示（去重保留首个位置）。
func extractNumbers(text string) []string {
	var out []string
	seen := map[string]bool{}

	// 统计量表达式优先匹配
	stats := sampleStatRe.FindAllString(text, -1)
	for _, s := range stats {
		key := strings.ToLower(s)
		if !seen[key] {
			seen[key] = true
			out = append(out, s)
		}
	}

	// p 值表达式
	pvals := pValueRe.FindAllString(text, -1)
	for _, p := range pvals {
		key := strings.ToLower(p)
		if !seen[key] {
			seen[key] = true
			out = append(out, p)
		}
	}

	// 普通数字
	nums := numberRe.FindAllString(text, -1)
	for _, n := range nums {
		// 忽略个位数（可能是一般计数，如 "2 种方法"）
		if len(n) == 1 && !strings.ContainsRune(n, '%') {
			continue
		}
		key := strings.ToLower(n)
		if !seen[key] {
			seen[key] = true
			out = append(out, n)
		}
	}

	return out
}
