package lint

import (
	"bytes"
	"fmt"
	"os"
	"strings"
)

// Source 表示一个待验证的 Markdown 文件。
type Source struct {
	Path    string
	Lines   []string // 原始行（行号从 1 开始，索引 0 对应第 1 行）
	Body    []string // 正文行（排除代码块/参考文献/表格）
	HasRefs bool     // 是否含参考文献段

	// bodyIdx 记录 Body[i] 对应的原始行号（1 起），供规则定位违规行。
	bodyIdx []int
}

// isRefsHeading 判断是否为参考文献段标题：标题文字包含"参考文献"或"references"即认定。
func isRefsHeading(trimmed string) bool {
	if !strings.HasPrefix(trimmed, "#") {
		return false
	}
	title := strings.ToLower(strings.TrimSpace(strings.TrimLeft(trimmed, "#")))
	return strings.Contains(title, "参考文献") || strings.Contains(title, "references")
}

// ParseSource 从文件路径解析 Source。
// 分段逻辑：
//   - 代码块：``` 到 ``` 之间的行排除
//   - 参考文献：从参考文献标题行开始到文件末尾排除
//   - 表格：以 | 开头的连续行排除
//   - 其余为 Body
func ParseSource(path string) (*Source, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("lint: read source %s: %w", path, err)
	}
	data = bytes.TrimPrefix(data, []byte{0xEF, 0xBB, 0xBF}) // 去除 UTF-8 BOM
	text := strings.ReplaceAll(string(data), "\r\n", "\n")
	lines := strings.Split(text, "\n")

	src := &Source{Path: path, Lines: lines}
	inCode := false
	inRefs := false
	for i, ln := range lines {
		trimmed := strings.TrimSpace(ln)
		// 代码块围栏切换（围栏行本身排除）
		if strings.HasPrefix(trimmed, "```") {
			inCode = !inCode
			continue
		}
		if inCode {
			continue
		}
		// 参考文献标题：自此到文件末尾排除
		if !inRefs && isRefsHeading(trimmed) {
			inRefs = true
			src.HasRefs = true
			continue
		}
		if inRefs {
			continue
		}
		// 表格行排除
		if strings.HasPrefix(trimmed, "|") {
			continue
		}
		src.Body = append(src.Body, ln)
		src.bodyIdx = append(src.bodyIdx, i+1)
	}
	return src, nil
}

// BodyContent 返回以 \n 连接的正文文本（供 core.ExtractCiteSentences 使用）。
func (s *Source) BodyContent() string {
	return strings.Join(s.Body, "\n")
}

// BodyLineNumbers 返回 Body 各行对应的原始行号。
func (s *Source) BodyLineNumbers() []int {
	return s.bodyIdx
}
