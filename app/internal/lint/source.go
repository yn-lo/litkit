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

// ParseSource 从文件路径解析 Source（无 spec，全部行参与分段）。
// 分段逻辑：
//   - 代码块：匹配的反引号/波浪号围栏及缩进代码行排除
//   - 参考文献：从参考文献标题行开始到文件末尾排除
//   - 表格：以 | 开头的连续行排除
//   - 其余为 Body
func ParseSource(path string) (*Source, error) {
	return parseSourceWithSpec(path, nil)
}

// parseSourceWithSpec 从文件路径解析 Source；spec 定义 sections 时，
// 首个 section 标题之前的非标题行（封面/简表字段）不进入 Body，
// 标题行保留（主标题仍受 R1.2 等检查）。spec 为 nil 或无 sections 时行为与旧版一致。
func parseSourceWithSpec(path string, spec *ManuscriptSpec) (*Source, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("lint: read source %s: %w", path, err)
	}
	data = bytes.TrimPrefix(data, []byte{0xEF, 0xBB, 0xBF}) // 去除 UTF-8 BOM
	text := strings.ReplaceAll(string(data), "\r\n", "\n")
	lines := strings.Split(text, "\n")

	// 作用域收口：首个 section 标题行号（1 起），0=无 sections 或未命中
	coverEnd := firstSectionLine(lines, spec)

	src := &Source{Path: path, Lines: lines}
	codeLines := markdownCodeLines(lines)
	inRefs := false
	for i, ln := range lines {
		lineNo := i + 1
		trimmed := strings.TrimSpace(ln)
		if codeLines[i] {
			continue
		}
		// 封面区（首个 section 标题之前）：标题行保留，其余排除
		if coverEnd > 0 && lineNo < coverEnd && !isHeading(trimmed) {
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
		src.bodyIdx = append(src.bodyIdx, lineNo)
	}
	return src, nil
}

// markdownCodeLines 为验证和修复提供相同的代码块范围（含围栏行）。
// ponytail: 仅按顶层块分段；嵌套列表/引用的精确范围需升级为 CommonMark 块解析器。
func markdownCodeLines(lines []string) []bool {
	const codeIndentWidth = 4
	protected := make([]bool, len(lines))
	var fence byte
	fenceLen := 0
	paragraph := false
	for i, line := range lines {
		char, length, tail := markdownFence(line)
		if fence != 0 {
			protected[i] = true
			if char == fence && length >= fenceLen && tail == "" {
				fence = 0
			}
			continue
		}
		indent := 0
	indentLoop:
		for _, c := range line {
			switch c {
			case ' ':
				indent++
			case '\t':
				indent += codeIndentWidth - indent%codeIndentWidth
			default:
				break indentLoop
			}
		}
		if indent >= codeIndentWidth && !paragraph {
			protected[i] = true
			continue
		}
		if char != 0 {
			protected[i] = true
			fence, fenceLen = char, length
			paragraph = false
			continue
		}
		// 缩进代码不能中断已开始的段落，跨行代码跨度可含缩进续行。
		trimmed := strings.TrimSpace(line)
		paragraph = trimmed != "" && !isHeading(trimmed)
	}
	return protected
}

// firstSectionLine 返回首个命中 spec.Sections 的标题行号（1 起）；
// spec 为 nil / 无 sections / 无命中时返回 0。
func firstSectionLine(lines []string, spec *ManuscriptSpec) int {
	if spec == nil || len(spec.Sections) == 0 {
		return 0
	}
	for i, ln := range lines {
		t := strings.TrimSpace(ln)
		if !isHeading(t) {
			continue
		}
		if _, _, text := headingLevel(t); text != "" {
			textLower := strings.ToLower(text)
			for _, sec := range spec.Sections {
				if strings.Contains(textLower, strings.ToLower(sec)) {
					return i + 1
				}
			}
		}
	}
	return 0
}

// BodyContent 返回以 \n 连接的正文文本（供 core.ExtractCiteSentences 使用）。
func (s *Source) BodyContent() string {
	return strings.Join(s.Body, "\n")
}

// BodyLineNumbers 返回 Body 各行对应的原始行号。
func (s *Source) BodyLineNumbers() []int {
	return s.bodyIdx
}
