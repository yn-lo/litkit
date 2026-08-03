package lint

import (
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"unicode"
)

// Method 规则执行方式。
type Method string

// 规则执行方式常量。
const (
	MethodA Method = "A" // 全自动
	MethodS Method = "S" // 脚本初筛+人工
)

// Mode 验证模式（递增：chapter < draft < final）。
type Mode string

// 验证模式常量（递增启用规则）。
const (
	ModeChapter Mode = "chapter"
	ModeDraft   Mode = "draft"
	ModeFinal   Mode = "final"
)

// P 值格式阈值（R2.1）与引用密度阈值（R5.3）。
const (
	pThreshold001  = 0.001 // P<0.001 不给出具体数值
	pThreshold01   = 0.01  // 0.001≤P<0.01 保留 3 位
	pDecimalsMid   = 3     // 中间区间小数位
	pDecimalsHigh  = 2     // P≥0.01 小数位
	maxCitePerLine = 3     // 单行最大引用数

	// R0.1 英文占比阈值
	enWordRatioLimit = 0.40
	percent          = 100.0

	// 高频规则 ID（goconst）
	ruleR21 = "R2.1"
)

// modeRank 用于比较模式递增。
func modeRank(m Mode) int {
	switch m {
	case ModeChapter:
		return 0
	case ModeDraft:
		return 1
	default:
		return 2 // final
	}
}

// Violation 单条违规。
type Violation struct {
	RuleID     string `json:"ruleId"`
	Line       int    `json:"line,omitempty"`
	Problem    string `json:"problem"`
	Suggestion string `json:"suggestion"`
}

// Rule 一条验证规则。
type Rule struct {
	ID     string
	Name   string
	Langs  []string // ["zh"] / ["en"] / ["zh","en"]
	Method Method
	From   Mode // 从该模式起启用
	Check  func(src *Source, spec *ManuscriptSpec) []Violation
}

// 预编译正则（mnd：集中管理，避免规则函数内重复编译）。
var (
	parenNoteRe     = regexp.MustCompile(`（[^）]*）`)                                   // 术语括注
	headingNumRe    = regexp.MustCompile(`^(\d+(?:\.\d+)*)`)                          // 标题编号
	headingEndRe    = regexp.MustCompile(`[。，、；：！？.]$`)                               // 标题末尾标点
	boldRe          = regexp.MustCompile(`\*\*[^*]+\*\*`)                             // 加粗
	pValueRe        = regexp.MustCompile(`([Pp])\s*(=|<=|>=|<|>|≤|≥)\s*(0?\.\d+)`)    // P 值
	halfWidthRe     = regexp.MustCompile(`[\x{4e00}-\x{9fff}][,.;:]`)                 // 中文后半角标点
	straightQuoteRe = regexp.MustCompile(`[\x{4e00}-\x{9fff}]"|"[\x{4e00}-\x{9fff}]`) // 中文上下文直引号
	apaPlaceholder  = regexp.MustCompile(`\([A-Z][a-z]+,\s*\d{4}\)`)                  // (Author, 2024)
	numCiteRe       = regexp.MustCompile(`\[\d+\]`)                                   // [数字]
	citePunctRe     = regexp.MustCompile(`[。，,.]\s*\[@[^\]]+\]`)                      // 标点后紧跟引用（违规）
	citeRe          = regexp.MustCompile(`\[@[^\]]+\]`)                               // 引用占位符 [@citeKey]
	redundantRes    = []*regexp.Regexp{
		regexp.MustCompile(`进行`),
		regexp.MustCompile(`通过.*使`),
		regexp.MustCompile(`对.*进行`),
	}
)

// boastWords 自我夸大关键词表。
var boastWords = []string{
	"首次证实", "颠覆性", "极其创新", "前所未有",
	"revolutionary", "groundbreaking", "unprecedented",
}

// countWords 统计字数：中文字符（Han）按 1 字计，英文按空格分词计。
// 返回 (总字数, 英文词数)；英文词定义为含至少一个 ASCII 字母的 token。
// 纯标点 token（不含字母或数字）不计字。
func countWords(s string) (total, en int) {
	var sb strings.Builder
	hasLetter := false
	hasAlnum := false
	flush := func() {
		if sb.Len() == 0 {
			return
		}
		if hasAlnum {
			total++
		}
		if hasLetter {
			en++
		}
		sb.Reset()
		hasLetter = false
		hasAlnum = false
	}
	for _, r := range s {
		switch {
		case unicode.Is(unicode.Han, r):
			flush()
			total++
		case unicode.IsSpace(r):
			flush()
		default:
			if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') {
				hasLetter = true
			}
			if unicode.IsLetter(r) || unicode.IsDigit(r) {
				hasAlnum = true
			}
			sb.WriteRune(r)
		}
	}
	flush()
	return
}

// wordCount 返回字数（countWords 的总数部分）。
func wordCount(s string) int {
	total, _ := countWords(s)
	return total
}

// headingLevel 解析标题行：返回层级、编号、去编号后的标题文字。
// 层级取 markdown # 数与编号深度（如 2.3.1 为 3）的较大者。
func headingLevel(trimmed string) (level int, num string, text string) {
	hashes := 0
	for hashes < len(trimmed) && trimmed[hashes] == '#' {
		hashes++
	}
	rest := strings.TrimSpace(trimmed[hashes:])
	level = hashes
	if m := headingNumRe.FindString(rest); m != "" {
		num = m
		if depth := strings.Count(m, ".") + 1; depth > level {
			level = depth
		}
		rest = strings.TrimSpace(rest[len(m):])
	}
	return level, num, rest
}

// isHeading 判断（已 trim 的）行是否为标题行。
func isHeading(trimmed string) bool { return strings.HasPrefix(trimmed, "#") }

// --- A 类规则 ---

// checkR01 全文中文：Body 英文单词占比（排除术语括注）>40% 违规。
func checkR01(src *Source, _ *ManuscriptSpec) []Violation {
	total, en := 0, 0
	for _, ln := range src.Body {
		t, e := countWords(parenNoteRe.ReplaceAllString(ln, ""))
		total += t
		en += e
	}
	if total == 0 {
		return nil
	}
	ratio := float64(en) / float64(total)
	if ratio > enWordRatioLimit {
		return []Violation{{
			RuleID:     "R0.1",
			Problem:    fmt.Sprintf("英文单词占比 %.0f%% 超过 40%%", ratio*percent),
			Suggestion: "全文应以中文撰写，英文仅限专业术语首次括注",
		}}
	}
	return nil
}

// checkR02 标题中文：标题行英文字母占比 >30% 违规。
func checkR02(src *Source, _ *ManuscriptSpec) []Violation {
	var vs []Violation
	for i, ln := range src.Body {
		t := strings.TrimSpace(ln)
		if !isHeading(t) {
			continue
		}
		letters, han := 0, 0
		for _, r := range t {
			switch {
			case (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z'):
				letters++
			case unicode.Is(unicode.Han, r):
				han++
			}
		}
		if denom := letters + han; denom > 0 && float64(letters)/float64(denom) > 0.30 {
			vs = append(vs, Violation{
				RuleID:     "R0.2",
				Line:       src.bodyIdx[i],
				Problem:    "标题英文字母占比超过 30%",
				Suggestion: "标题应以中文为主",
			})
		}
	}
	return vs
}

// checkR11 章节层级：标题层级 > spec.Heading.MaxLevel 违规。
func checkR11(src *Source, spec *ManuscriptSpec) []Violation {
	var vs []Violation
	for i, ln := range src.Body {
		t := strings.TrimSpace(ln)
		if !isHeading(t) {
			continue
		}
		level, _, _ := headingLevel(t)
		if level > spec.Heading.MaxLevel {
			vs = append(vs, Violation{
				RuleID:     "R1.1",
				Line:       src.bodyIdx[i],
				Problem:    fmt.Sprintf("标题层级 %d 超过上限 %d", level, spec.Heading.MaxLevel),
				Suggestion: "降低标题层级",
			})
		}
	}
	return vs
}

// checkR12 标题长度：去编号标题字数 > MaxLength 违规；末尾有标点违规。
func checkR12(src *Source, spec *ManuscriptSpec) []Violation {
	var vs []Violation
	for i, ln := range src.Body {
		t := strings.TrimSpace(ln)
		if !isHeading(t) {
			continue
		}
		_, _, text := headingLevel(t)
		if n := wordCount(text); n > spec.Heading.MaxLength {
			vs = append(vs, Violation{
				RuleID:     "R1.2",
				Line:       src.bodyIdx[i],
				Problem:    fmt.Sprintf("标题字数 %d 超过上限 %d", n, spec.Heading.MaxLength),
				Suggestion: "精简标题",
			})
		}
		if headingEndRe.MatchString(text) {
			vs = append(vs, Violation{
				RuleID:     "R1.2",
				Line:       src.bodyIdx[i],
				Problem:    "标题末尾有标点",
				Suggestion: "删除标题末尾标点",
			})
		}
	}
	return vs
}

// checkR14 正文禁止加粗：Body 行含 **...** 违规（表格已在解析阶段排除）。
func checkR14(src *Source, _ *ManuscriptSpec) []Violation {
	var vs []Violation
	for i, ln := range src.Body {
		if boldRe.MatchString(ln) {
			vs = append(vs, Violation{
				RuleID:     "R1.4",
				Line:       src.bodyIdx[i],
				Problem:    "正文含加粗",
				Suggestion: "移除 ** 加粗标记",
			})
		}
	}
	return vs
}

// checkR21 P 值格式：大写/前导零/小数位规范。
func checkR21(src *Source, _ *ManuscriptSpec) []Violation {
	var vs []Violation
	for i, ln := range src.Body {
		line := src.bodyIdx[i]
		for _, m := range pValueRe.FindAllStringSubmatch(ln, -1) {
			letter, num := m[1], m[3]
			if letter == "p" {
				vs = append(vs, Violation{
					RuleID:     ruleR21,
					Line:       line,
					Problem:    "P 值应大写",
					Suggestion: "将 p 改为 P",
				})
			}
			if strings.HasPrefix(num, ".") {
				vs = append(vs, Violation{
					RuleID:     ruleR21,
					Line:       line,
					Problem:    "P 值缺少前导零",
					Suggestion: "写为 0.xx 形式",
				})
				continue
			}
			val, _ := strconv.ParseFloat(num, 64)
			decimals := len(num) - strings.Index(num, ".") - 1
			switch {
			case val < pThreshold001:
				vs = append(vs, Violation{
					RuleID:     ruleR21,
					Line:       line,
					Problem:    "P<0.001 不应给出具体数值",
					Suggestion: "写为 P<0.001",
				})
			case val < pThreshold01:
				if decimals != pDecimalsMid {
					vs = append(vs, Violation{
						RuleID:     ruleR21,
						Line:       line,
						Problem:    fmt.Sprintf("0.001≤P<0.01 应保留 3 位小数，got %s", num),
						Suggestion: "保留 3 位小数",
					})
				}
			default: // val >= 0.01
				if decimals != pDecimalsHigh {
					vs = append(vs, Violation{
						RuleID:     ruleR21,
						Line:       line,
						Problem:    fmt.Sprintf("P≥0.01 应保留 2 位小数，got %s", num),
						Suggestion: "保留 2 位小数",
					})
				}
			}
		}
	}
	return vs
}

// checkR31 全半角：中文字符后紧跟半角标点违规。
func checkR31(src *Source, _ *ManuscriptSpec) []Violation {
	var vs []Violation
	for i, ln := range src.Body {
		if halfWidthRe.MatchString(ln) {
			vs = append(vs, Violation{
				RuleID:     "R3.1",
				Line:       src.bodyIdx[i],
				Problem:    "中文后使用了半角标点",
				Suggestion: "改用全角标点",
			})
		}
	}
	return vs
}

// checkR32 中文引号：中文上下文出现直引号违规。
func checkR32(src *Source, _ *ManuscriptSpec) []Violation {
	var vs []Violation
	for i, ln := range src.Body {
		if straightQuoteRe.MatchString(ln) {
			vs = append(vs, Violation{
				RuleID:     "R3.2",
				Line:       src.bodyIdx[i],
				Problem:    "中文上下文使用了直引号",
				Suggestion: "改用中文弯引号",
			})
		}
	}
	return vs
}

// checkR51 引用占位符：(Author, 2024) 与 [数字] 违规（参考文献段已排除）。
func checkR51(src *Source, _ *ManuscriptSpec) []Violation {
	var vs []Violation
	for i, ln := range src.Body {
		if apaPlaceholder.MatchString(ln) || numCiteRe.MatchString(ln) {
			vs = append(vs, Violation{
				RuleID:     "R5.1",
				Line:       src.bodyIdx[i],
				Problem:    "检测到非规范引用占位符",
				Suggestion: "改用 [@citeKey] 占位符",
			})
		}
	}
	return vs
}

// checkR52 待引证标记：[待引证] / [TODO] / [TBD] 违规。
func checkR52(src *Source, _ *ManuscriptSpec) []Violation {
	var vs []Violation
	for i, ln := range src.Body {
		if strings.Contains(ln, "[待引证]") || strings.Contains(ln, "[TODO]") || strings.Contains(ln, "[TBD]") {
			vs = append(vs, Violation{
				RuleID:     "R5.2",
				Line:       src.bodyIdx[i],
				Problem:    "含待引证标记",
				Suggestion: "补充引用后移除标记",
			})
		}
	}
	return vs
}

// checkR61 引用位置：标点后紧跟 [@xxx] 违规（引用应在标点前）。
func checkR61(src *Source, _ *ManuscriptSpec) []Violation {
	var vs []Violation
	for i, ln := range src.Body {
		if citePunctRe.MatchString(ln) {
			vs = append(vs, Violation{
				RuleID:     "R6.1",
				Line:       src.bodyIdx[i],
				Problem:    "标点后紧跟引用标记",
				Suggestion: "引用标记应置于标点之前，如 ...发展[@Kxq]。",
			})
		}
	}
	return vs
}

// checkR71 标题冒号：标题行含 ： 或 : 违规。
func checkR71(src *Source, _ *ManuscriptSpec) []Violation {
	var vs []Violation
	for i, ln := range src.Body {
		t := strings.TrimSpace(ln)
		if !isHeading(t) {
			continue
		}
		if strings.Contains(t, "：") || strings.Contains(t, ":") {
			vs = append(vs, Violation{
				RuleID:     "R7.1",
				Line:       src.bodyIdx[i],
				Problem:    "标题含冒号",
				Suggestion: "移除标题中的冒号",
			})
		}
	}
	return vs
}

// checkR72 自我夸大：关键词表扫描违规。
func checkR72(src *Source, _ *ManuscriptSpec) []Violation {
	var vs []Violation
	for i, ln := range src.Body {
		lower := strings.ToLower(ln)
		for _, w := range boastWords {
			if strings.Contains(lower, strings.ToLower(w)) {
				vs = append(vs, Violation{
					RuleID:     "R7.2",
					Line:       src.bodyIdx[i],
					Problem:    fmt.Sprintf("含自我夸大词 %q", w),
					Suggestion: "改用客观表述",
				})
				break // 一行报一次
			}
		}
	}
	return vs
}

// checkR91 用户标记：含 【 或 】 违规。
func checkR91(src *Source, _ *ManuscriptSpec) []Violation {
	var vs []Violation
	for i, ln := range src.Body {
		if strings.Contains(ln, "【") || strings.Contains(ln, "】") {
			vs = append(vs, Violation{
				RuleID:     "R9.1",
				Line:       src.bodyIdx[i],
				Problem:    "含用户标记【】",
				Suggestion: "移除【】标记",
			})
		}
	}
	return vs
}

// --- 字数规则（A 类，from:final） ---

// checkR81 全文字数：Body 总字数不在 spec.WordCount.Total 区间违规。
func checkR81(src *Source, spec *ManuscriptSpec) []Violation {
	total := 0
	for _, ln := range src.Body {
		total += wordCount(ln)
	}
	lo, hi := spec.WordCount.Total[0], spec.WordCount.Total[1]
	if total < lo || total > hi {
		return []Violation{{
			RuleID:     "R8.1",
			Problem:    fmt.Sprintf("全文字数 %d 不在 %d-%d 区间", total, lo, hi),
			Suggestion: "调整全文篇幅",
		}}
	}
	return nil
}

// checkR82 摘要字数：摘要/Abstract 标题后段落字数不在 spec.WordCount.Abstract 区间违规。
func checkR82(src *Source, spec *ManuscriptSpec) []Violation {
	idx := -1
	for i, ln := range src.Body {
		t := strings.TrimSpace(ln)
		if !isHeading(t) {
			continue
		}
		title := strings.TrimSpace(strings.TrimLeft(t, "#"))
		if title == "摘要" || strings.EqualFold(title, "abstract") {
			idx = i
			break
		}
	}
	if idx < 0 {
		return nil
	}
	words := 0
	for j := idx + 1; j < len(src.Body); j++ {
		if isHeading(strings.TrimSpace(src.Body[j])) {
			break
		}
		words += wordCount(src.Body[j])
	}
	lo, hi := spec.WordCount.Abstract[0], spec.WordCount.Abstract[1]
	if words < lo || words > hi {
		return []Violation{{
			RuleID:     "R8.2",
			Line:       src.bodyIdx[idx],
			Problem:    fmt.Sprintf("摘要字数 %d 不在 %d-%d 区间", words, lo, hi),
			Suggestion: "调整摘要篇幅",
		}}
	}
	return nil
}

// --- S 类规则（初筛输出） ---

// checkR42 句式冗余：匹配冗余句式，命中提示人工确认。
func checkR42(src *Source, _ *ManuscriptSpec) []Violation {
	var vs []Violation
	for i, ln := range src.Body {
		for _, re := range redundantRes {
			if re.MatchString(ln) {
				vs = append(vs, Violation{
					RuleID:     "R4.2",
					Line:       src.bodyIdx[i],
					Problem:    "句式可能冗余",
					Suggestion: "需人工确认是否冗余",
				})
				break
			}
		}
	}
	return vs
}

// checkR53 引用密度：全文 [@...] 去重篇数不在区间违规；单行 >3 处违规（按出现次数）。
func checkR53(src *Source, spec *ManuscriptSpec) []Violation {
	var vs []Violation
	seen := map[string]bool{} // citeKey 去重，总数按去重篇数计
	total := 0
	for i, ln := range src.Body {
		matches := citeRe.FindAllString(ln, -1)
		if c := len(matches); c > maxCitePerLine {
			vs = append(vs, Violation{
				RuleID:     "R5.3",
				Line:       src.bodyIdx[i],
				Problem:    fmt.Sprintf("单行 %d 个引用超过 3", c),
				Suggestion: "拆分引用到多处",
			})
		}
		for _, m := range matches {
			key := m[2 : len(m)-1] // 去掉 [@ 与 ]
			if !seen[key] {
				seen[key] = true
				total++
			}
		}
	}
	lo, hi := spec.Citation.Count[0], spec.Citation.Count[1]
	if total < lo || total > hi {
		vs = append(vs, Violation{
			RuleID:     "R5.3",
			Problem:    fmt.Sprintf("全文引用 %d 篇，不在 %d-%d 区间", total, lo, hi),
			Suggestion: "调整引用数量",
		})
	}
	return vs
}

// checkR83 段长：Body 连续非空行为一段，字数不在 spec.WordCount.Paragraph 区间违规。
func checkR83(src *Source, spec *ManuscriptSpec) []Violation {
	var vs []Violation
	lo, hi := spec.WordCount.Paragraph[0], spec.WordCount.Paragraph[1]
	inPara := false
	paraStart, paraWords := 0, 0
	flush := func() {
		if inPara && (paraWords < lo || paraWords > hi) {
			vs = append(vs, Violation{
				RuleID:     "R8.3",
				Line:       paraStart,
				Problem:    fmt.Sprintf("段落字数 %d 不在 %d-%d 区间", paraWords, lo, hi),
				Suggestion: "调整段落长度",
			})
		}
		inPara = false
		paraWords = 0
	}
	for i, ln := range src.Body {
		if strings.TrimSpace(ln) == "" {
			flush()
			continue
		}
		if !inPara {
			inPara = true
			paraStart = src.bodyIdx[i]
		}
		paraWords += wordCount(ln)
	}
	flush()
	return vs
}

// AllRules 返回全部已注册规则（按 ID 排序）。
func AllRules() []Rule {
	rules := []Rule{
		{ID: "R0.1", Name: "全文中文", Langs: []string{"zh"}, Method: MethodA, From: ModeChapter, Check: checkR01},
		{ID: "R0.2", Name: "标题中文", Langs: []string{"zh"}, Method: MethodA, From: ModeChapter, Check: checkR02},
		{ID: "R1.1", Name: "章节层级", Langs: []string{"zh", "en"}, Method: MethodA, From: ModeChapter, Check: checkR11},
		{ID: "R1.2", Name: "标题长度", Langs: []string{"zh", "en"}, Method: MethodA, From: ModeChapter, Check: checkR12},
		{ID: "R1.4", Name: "正文禁止加粗", Langs: []string{"zh"}, Method: MethodA, From: ModeDraft, Check: checkR14},
		{ID: ruleR21, Name: "P值格式", Langs: []string{"zh", "en"}, Method: MethodA, From: ModeDraft, Check: checkR21},
		{ID: "R3.1", Name: "全半角", Langs: []string{"zh"}, Method: MethodA, From: ModeDraft, Check: checkR31},
		{ID: "R3.2", Name: "中文引号", Langs: []string{"zh"}, Method: MethodA, From: ModeDraft, Check: checkR32},
		{ID: "R4.2", Name: "句式冗余", Langs: []string{"zh"}, Method: MethodS, From: ModeFinal, Check: checkR42},
		{ID: "R5.1", Name: "引用占位符", Langs: []string{"zh", "en"}, Method: MethodA, From: ModeDraft, Check: checkR51},
		{ID: "R5.2", Name: "待引证标记", Langs: []string{"zh", "en"}, Method: MethodA, From: ModeDraft, Check: checkR52},
		{ID: "R5.3", Name: "引用密度", Langs: []string{"zh", "en"}, Method: MethodS, From: ModeFinal, Check: checkR53},
		{ID: "R6.1", Name: "引用位置", Langs: []string{"zh", "en"}, Method: MethodA, From: ModeDraft, Check: checkR61},
		{ID: "R7.1", Name: "标题冒号", Langs: []string{"zh", "en"}, Method: MethodA, From: ModeChapter, Check: checkR71},
		{ID: "R7.2", Name: "自我夸大", Langs: []string{"zh", "en"}, Method: MethodA, From: ModeDraft, Check: checkR72},
		{ID: "R8.1", Name: "全文字数", Langs: []string{"zh", "en"}, Method: MethodA, From: ModeFinal, Check: checkR81},
		{ID: "R8.2", Name: "摘要字数", Langs: []string{"zh", "en"}, Method: MethodA, From: ModeFinal, Check: checkR82},
		{ID: "R8.3", Name: "段长", Langs: []string{"zh", "en"}, Method: MethodS, From: ModeFinal, Check: checkR83},
		{ID: "R9.1", Name: "用户标记", Langs: []string{"zh", "en"}, Method: MethodA, From: ModeChapter, Check: checkR91},
	}
	sort.Slice(rules, func(i, j int) bool { return rules[i].ID < rules[j].ID })
	return rules
}
