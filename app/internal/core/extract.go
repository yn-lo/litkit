// extract.go 从手稿正文中提取引用句（FR-LINT-08 输入构造）。
//
// 引用句抽取规则：
//   - 以 [@citeKey] 占位符为锚点，向前后找句末标点确定句子边界
//   - 多语种句末标点：.  。  ！  ？  ？  ；  ．
//   - 一句多引：同一句含多个 [@citeKey] → 每个 citeKey 各一条 PaperRef，共享 sentence_hash
//   - 跳过非正文区（代码块、参考文献列表、表格——与 lint verify 的 body 提取一致）
//
// 与 manuscript.go 共用 placeholderRe 正则。

package core

import (
	"crypto/sha256"
	"encoding/hex"
	"regexp"
	"strings"
	"unicode/utf8"
)

// CiteSentence 引用句信息。
type CiteSentence struct {
	CiteKey      string // 被引论文 cite_key（3 字母）
	SentenceHash string // 引用句 sha256 前缀指纹
	Sentence     string // 引用句原文
	Line         int    // 在正文中的行号（1 起）
}

// bodyCiteRe 匹配正文中的裸 [@citeKey] 占位符（与 manuscript.go placeholderRe 一致）。
// 3-5 字母以支持 cite_key 空间耗尽升 4/5。
var bodyCiteRe = regexp.MustCompile(`\[@([a-zA-Z]{3,5})\]`)

// codeBlockRe 匹配 Markdown 代码块（``` 或 ~~~），排除其中内容。
var codeBlockRe = regexp.MustCompile("(?s)```.*?```|~~~.*?~~~")

// skipBlockRe 匹配应跳过的非正文段：参考文献、表格。
var skipBlockRe = regexp.MustCompile(`(?i)(?sm)^#+\s*(参考文献|references|bibliography)\s*$.*`)

// isSentenceEnd 判断 rune 是否为句末标点。
func isSentenceEnd(r rune) bool {
	switch r {
	case '。', '．', '！', '？', '!', '?', '；', ';':
		return true
	case '.':
		return true // 句点，调用方做上下文判断
	}
	return false
}

// isLetter 判断 rune 是否为字母。
func isLetter(r rune) bool {
	return (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z')
}

// ExtractCiteSentences 从手稿正文中提取所有引用句。
//
// body 应为排除代码块/参考文献/表格后的正文（与 lint verify body 提取一致）。
// 返回 slice 按 citeKey 出现顺序排列。
func ExtractCiteSentences(body string) []CiteSentence {
	// 跳过代码块、参考文献段
	body = codeBlockRe.ReplaceAllString(body, "")
	body = skipBlockRe.ReplaceAllString(body, "")

	matches := bodyCiteRe.FindAllStringSubmatchIndex(body, -1)
	if len(matches) == 0 {
		return nil
	}

	var result []CiteSentence
	seen := map[string]bool{} // "citeKey:hash" 去重

	for _, m := range matches {
		citeKey := body[m[2]:m[3]] // 第二个捕获组 = citeKey

		// 向前找句末标点（字节偏移）
		sentStart := 0
		before := body[:m[0]]
		if idx := lastSentenceEndByte(before); idx >= 0 {
			sentStart = idx
		}

		// 向后找句末标点（字节偏移）
		after := body[m[1]:]
		sentEnd := len(body)
		if idx := nextSentenceEndByte(after); idx >= 0 {
			sentEnd = m[1] + idx
		}

		sentence := strings.TrimSpace(body[sentStart:sentEnd])
		if sentence == "" {
			continue
		}

		// hash 去掉末尾标点，保持稳定
		hashInput := sentence
		runes := []rune(sentence)
		if len(runes) > 0 && isSentenceEnd(runes[len(runes)-1]) {
			hashInput = strings.TrimSpace(string(runes[:len(runes)-1]))
		}

		sum := sha256.Sum256([]byte(hashInput))
		hash := hex.EncodeToString(sum[:])[:16]

		key := citeKey + ":" + hash
		if seen[key] {
			continue
		}
		seen[key] = true

		// 计算行号：统计锚点前的换行数
		lineNo := 1
		for _, b := range body[:m[0]] {
			if b == '\n' {
				lineNo++
			}
		}

		result = append(result, CiteSentence{
			CiteKey:      citeKey,
			SentenceHash: hash,
			Sentence:     sentence,
			Line:         lineNo,
		})
	}
	return result
}

// lastSentenceEndByte 在 before 中从后向前找句末标点，返回标点后的下一个字节位置。
// 未找到返回 -1。
//
//nolint:gocyclo // 标点判断涉及多种字符，展开为 switch 是惯用做法。
func lastSentenceEndByte(before string) int {
	// 从后向前遍历 rune
	runes := []rune(before)
	for i := len(runes) - 1; i >= 0; i-- {
		c := runes[i]
		switch c {
		case '。', '．', '！', '？', '!', '?', '；', ';':
			// 跳过标点及后续空白
			j := i + 1
			for j < len(runes) && (runes[j] == ' ' || runes[j] == '\t' || runes[j] == '\n' || runes[j] == '\r') {
				j++
			}
			// 将 rune 偏移转为字节偏移
			return runeOffsetToByte(before, j)
		case '.':
			// 英文句点：前面是字母且后面是空格/大写 → 句点
			if i > 0 && isLetter(runes[i-1]) {
				if i+1 < len(runes) && (runes[i+1] == ' ' || runes[i+1] == '\t' || runes[i+1] == '\n' || runes[i+1] == '\r') {
					j := i + 1
					for j < len(runes) && (runes[j] == ' ' || runes[j] == '\t' || runes[j] == '\n' || runes[j] == '\r') {
						j++
					}
					return runeOffsetToByte(before, j)
				}
			}
		}
	}
	return -1
}

// nextSentenceEndByte 在 after 中从前往后找句末标点，返回标点后的字节偏移（含标点）。
// 未找到返回 -1。
func nextSentenceEndByte(after string) int {
	runes := []rune(after)
	for i, c := range runes {
		switch c {
		case '。', '．', '！', '？', '!', '?', '；', ';':
			return runeOffsetToByte(after, i+1)
		case '.':
			// 句点：后面有非字母（或字符串结尾）→ 句点
			if i+1 >= len(runes) || !isLetter(runes[i+1]) {
				return runeOffsetToByte(after, i+1)
			}
		}
	}
	return -1
}

// runeOffsetToByte 将字符串中第 n 个 rune 的偏移转为字节偏移。
func runeOffsetToByte(s string, n int) int {
	pos := 0
	for i := 0; i < n && pos < len(s); i++ {
		_, size := utf8.DecodeRuneInString(s[pos:])
		pos += size
	}
	return pos
}
