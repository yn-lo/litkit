// Package textutil 提供文本归一化工具（叶子层）。
//
// 主要服务跨源去重（FR-SEARCH-02）：不同源返回的标题在大小写/空白上
// 可能不一致，NormalizeTitle 给出确定性归一化形式作为去重键。
package textutil

import (
	"strings"
	"unicode"
)

// NormalizeTitle 归一化标题：转小写、去除首尾空白、合并内部连续空白。
//
// 用于跨源去重的 title 级键（PRD 4.2 FR-SEARCH-02、data-model.md §3）。
// 不改变标点与字符，仅处理大小写与空白。
func NormalizeTitle(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	if s == "" {
		return ""
	}
	var b strings.Builder
	b.Grow(len(s))
	inSpace := false
	for _, r := range s {
		if unicode.IsSpace(r) {
			if !inSpace {
				b.WriteRune(' ')
				inSpace = true
			}
			continue
		}
		inSpace = false
		b.WriteRune(r)
	}
	return b.String()
}
