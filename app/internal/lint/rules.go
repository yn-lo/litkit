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

// Category 规则检查类别（对应 spec yaml 顶层分组，用于 --check/--skip-check 筛选）。
type Category string

// 检查类别常量。
const (
	CatLanguage    Category = "language"    // 语言合规（R0.x）
	CatStructure   Category = "structure"   // 章节结构（R1.x）
	CatStatistics  Category = "statistics"  // 统计格式（R2.x）
	CatPunctuation Category = "punctuation" // 标点符号（R3.x）
	CatStyle       Category = "style"       // 行文风格（R4.x）
	CatCitation    Category = "citation"    // 引用规范（R5.x, R6.x）
	CatHeading     Category = "heading"     // 标题规范（R7.1）
	CatBoastWords  Category = "boast_words" // 自我夸大/AI痕迹（R7.2）
	CatWordCounts  Category = "word_counts" // 字数统计（R8.x）
	CatTodo        Category = "todo"        // 用户标记（R9.x）
)

// ruleIDHeadingOrder R1.3 标题规范规则的 ID。
const ruleIDHeadingOrder = "R1.3"

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
	ruleR17        = "R1.7"
	ruleR21        = "R2.1"
	ruleCiteExists = "R5.6" // 引用存在性校验（[@citeKey] 须在本地库中）
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
	RuleID     string `json:"-"` // 内部使用，不输出到 JSON
	Line       int    `json:"line,omitempty"`
	Problem    string `json:"problem"`
	Suggestion string `json:"suggestion"`
}

// Rule 一条验证规则。
type Rule struct {
	ID       string
	Name     string
	Category Category // 检查类别（对应 spec yaml 顶层分组）
	Langs    []string // ["zh"] / ["en"] / ["zh","en"]
	Types    []string // 适用论文类型，空=全部（如 ["empirical"] 仅实证）
	Method   Method
	From     Mode // 从该模式起启用
	Check    func(src *Source, spec *ManuscriptSpec) []Violation
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
	figTableRefRe   = regexp.MustCompile(`(?i)(图|表|table|figure)s?\s*(\d+)`)          // 图表引用/题注
	redundantRes    = []*regexp.Regexp{
		regexp.MustCompile(`进行`),
		regexp.MustCompile(`通过.*使`),
		regexp.MustCompile(`对.*进行`),
	}

	// book 中文编号体系（yueshu.md 二、标题层次）：第一篇/第一章/第一节/一、/（一）/1./（1）
	bookCNNumRe    = regexp.MustCompile(`^第[一二三四五六七八九十百零]+[篇章节]`)   // 第X篇/第X章/第X节
	bookCNOrderRe  = regexp.MustCompile(`^[一二三四五六七八九十百零]+、`)        // 一、
	bookCNParenRe  = regexp.MustCompile(`^[（(][一二三四五六七八九十百零]+[)）]`) // （一）
	bookNumDotRe   = regexp.MustCompile(`^\d+\.\s`)                 // 1. 六级
	bookNumParenRe = regexp.MustCompile(`^[（(]\d+[)）]`)             // （1）七级
	bookDotNumRe   = regexp.MustCompile(`^\d+(?:\.\d+)+`)           // 1.1.1 点分编号（禁用）
	bookPureNumRe  = regexp.MustCompile(`^\d+[、．.]?\s`)             // 纯数字编号（应改用中文体系）

	// R3.3 数字范围规范（yueshu.md 八、数字）
	rangeHyphenPctRe  = regexp.MustCompile(`\d+(?:\.\d+)?\s*[-–]\s*\d+(?:\.\d+)?%`)                            // 10-16% → 10%～16%
	rangeUnitBothRe   = regexp.MustCompile(`\d+(?:\.\d+)?\s*[a-zA-Z℃°]+\s*[～~]\s*\d+(?:\.\d+)?\s*[a-zA-Z℃°]+`) // 10kg～15kg → 10～15kg
	yearRangeWaveRe   = regexp.MustCompile(`\d{4}年\s*[～~]\s*\d{4}年`)                                           // 1988年～1998年 → 1988—1998年
	hourRangeDashRe   = regexp.MustCompile(`\d+(?:\.\d+)?\s*—\s*\d+(?:\.\d+)?\s*(?:小时|分钟|天|周)`)                // 24—48小时 → 24～48小时
	doubleSlashUnitRe = regexp.MustCompile(`[a-zA-Z℃°]+\s*/\s*[a-zA-Z℃°]+\s*/\s*[a-zA-Z℃°]+`)                  // mg/kg/d → mg/(kg·d)

	// R3.4 计量单位（yueshu.md 五、计量单位）：数字后的中文单位词 / 英制旧制单位
	cnUnitRe = regexp.MustCompile(`\d+(?:\.\d+)?\s*(?:毫米汞柱|毫米水柱|毫克|微克|纳克|千克|公斤|克|厘米|毫米|千米|公里|米|毫升|升|毫摩尔|摩尔|兆帕|千帕|帕|千瓦|瓦|千伏|伏|毫安|安|兆赫|千赫|赫兹|焦耳|牛顿|摄氏度|华氏度|英寸|磅|英尺|尺|寸|两|钱|卡|石)`)
)

// numberPatterns R3.3 子模式表（命中任一即违规，每行报一次）。
var numberPatterns = []struct {
	re         *regexp.Regexp
	problem    string
	suggestion string
}{
	{rangeHyphenPctRe, "范围连接符用了连字符且百分号位置不当", "写为 10%～16% 形式（百分号只在范围末）"},
	{rangeUnitBothRe, "单位重复出现在范围两端", "写为 10～15kg 形式（单位只在范围末）"},
	{yearRangeWaveRe, "年份范围使用了波浪线", "年份范围用一字线：1988—1998年"},
	{hourRangeDashRe, "时间跨度使用了一字线", "时间范围用波浪线：24～48小时"},
	{doubleSlashUnitRe, "复合单位分母含多个斜线", "分母用圆括号且只保留一条斜线：5 mg/(kg·d)"},
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

// checkR13 标题审查：大标题存在性 + 强制编号（require_numbering）+ 层级对齐 + 顺序排列。
//
// 判定模型：
//   - 第一个标题是论文大标题，必须无编号；首个标题带编号或全文无标题 → 缺大标题
//   - 大标题之后的标题须带编号（require_numbering 开启时），且从 1 开始
//   - 有编号：markdown # 数须与编号深度一致（# 1、## 1.1、### 1.1.1）
//   - 有编号：编号递增、不跳号、层级挂接合法（编号栈）
func checkR13(src *Source, spec *ManuscriptSpec) []Violation {
	// book 采用中文编号体系（第一篇/第一章/一、/（一）），与论文点分编号不同，走独立分支
	if spec.PaperType == PaperTypeBook {
		return checkBookHeading(src, spec)
	}
	var vs []Violation
	prev := []int{}
	firstHeading := true  // 尚未处理第一个标题
	firstNumbered := true // 尚未处理第一个编号标题
	requireNum := spec.Heading.NumberingRequired()
	for i, ln := range src.Body {
		t := strings.TrimSpace(ln)
		if !isHeading(t) {
			continue
		}
		hashes := 0
		for hashes < len(t) && t[hashes] == '#' {
			hashes++
		}
		_, num, _ := headingLevel(t)
		line := src.bodyIdx[i]
		if num == "" {
			if firstHeading {
				// 首个无编号标题 = 大标题，合法，不参与编号栈
				firstHeading = false
				continue
			}
			// 大标题之后的无编号标题：强制编号时违规（不参与顺序栈）
			if requireNum {
				vs = append(vs, Violation{
					RuleID: ruleIDHeadingOrder, Line: line,
					Problem:    "标题未带数字编号",
					Suggestion: "按层级添加编号：# 1、## 1.1、### 1.1.1（编号深度与 # 数对齐）",
				})
			}
			continue
		}
		if firstHeading {
			// 首个标题带编号 → 缺大标题
			vs = append(vs, Violation{
				RuleID: ruleIDHeadingOrder, Line: line,
				Problem:    "缺少大标题：首个标题带编号",
				Suggestion: "根据文章内容设计一个合适的大标题置于文首（不编号），章节从 1 开始编号",
			})
		}
		firstHeading = false
		parts := parseHeadingNum(num)
		// 层级对齐：markdown # 数 == 编号深度
		if hashes != len(parts) {
			vs = append(vs, Violation{
				RuleID: ruleIDHeadingOrder, Line: line,
				Problem:    fmt.Sprintf("标题 %s 的 markdown 层级（%d 个 #）与编号深度（%d）不对齐", joinHeadingNum(parts), hashes, len(parts)),
				Suggestion: "编号深度应与 # 数一致：# 1、## 1.1、### 1.1.1",
			})
		}
		if firstNumbered {
			if len(parts) > 1 {
				vs = append(vs, Violation{
					RuleID: ruleIDHeadingOrder, Line: line,
					Problem:    fmt.Sprintf("首个编号标题 %s 缺少父级标题", joinHeadingNum(parts)),
					Suggestion: "顶层章节应从单级编号（如 1）开始",
				})
			}
			if parts[0] != 1 {
				vs = append(vs, Violation{
					RuleID: ruleIDHeadingOrder, Line: line,
					Problem:    fmt.Sprintf("顶层章节编号应从 1 开始，got %d", parts[0]),
					Suggestion: "顶层章节编号从 1 开始递增",
				})
			}
			firstNumbered = false
		} else if !validNextHeading(prev, parts) {
			vs = append(vs, Violation{
				RuleID: ruleIDHeadingOrder, Line: line,
				Problem:    fmt.Sprintf("标题编号 %s 顺序异常（上一编号 %s）", joinHeadingNum(parts), joinHeadingNum(prev)),
				Suggestion: "编号须按层级递增排列：同级顺延、子级在父级下从 1 开始",
			})
		}
		prev = parts
	}
	if firstHeading {
		// 全文无任何标题
		vs = append(vs, Violation{
			RuleID: ruleIDHeadingOrder, Line: 1,
			Problem:    "缺少大标题：全文未找到任何标题",
			Suggestion: "根据文章内容设计一个合适的大标题置于文首（不编号）",
		})
	}
	return vs
}

// checkBookHeading 书籍标题审查（yueshu.md 二、标题层次）。
//
// 判定模型（book 中文编号体系）：
//   - 首个标题为文件顶层：默认 auto 下可为书名（无编号）或任意合法编号，
//     兼容整本书/单章/单节成文件；book_top_level 可强制指定（book=须书名，chapter=须篇/章级）
//   - 禁用 1.1.1 点分编号；纯数字编号应改用中文体系
//   - 篇/章/节编号同类连续递进（第一章→第二章）
//   - 标题层级（# 数）逐级递进，不可跳级
func checkBookHeading(src *Source, spec *ManuscriptSpec) []Violation {
	var vs []Violation
	firstHeading := true     // 首个标题 = 文件顶层（书名或最高编号标题）
	prevHashes := 0          // 上一标题的 markdown # 数
	topSeq := map[byte]int{} // 篇/章/节 → 上一个数字（同类连续）
	for i, ln := range src.Body {
		t := strings.TrimSpace(ln)
		if !isHeading(t) {
			continue
		}
		hashes := 0
		for hashes < len(t) && t[hashes] == '#' {
			hashes++
		}
		rest := strings.TrimSpace(t[hashes:])
		line := src.bodyIdx[i]
		if firstHeading {
			firstHeading = false
			if rest == "" {
				vs = append(vs, Violation{RuleID: ruleIDHeadingOrder, Line: line,
					Problem: "空标题（# 后无文字）", Suggestion: "删除空标题或补充标题"})
				continue
			}
			if !bookTopLevelOK(rest, spec.BookTopLevel) {
				problem, suggestion := bookTopLevelHint(spec.BookTopLevel)
				vs = append(vs, Violation{RuleID: ruleIDHeadingOrder, Line: line,
					Problem: problem, Suggestion: suggestion})
			}
			prevHashes = hashes
			continue
		}
		// 点分编号（1.1.1）禁用
		if bookDotNumRe.MatchString(rest) {
			vs = append(vs, Violation{RuleID: ruleIDHeadingOrder, Line: line,
				Problem:    "标题使用了点分编号（1.1.1 国际编号方式）",
				Suggestion: "改用中文编号体系：第一篇/第一章/第一节/一、/（一）/1./（1）"})
			continue
		}
		// 未编号 / 纯数字编号
		if !isBookNumbered(rest) {
			problem, suggestion := "标题未带中文编号", "按层级添加中文编号：第一章/第一节/一、/（一）/1./（1）"
			if bookPureNumRe.MatchString(rest) {
				problem, suggestion = "标题使用了纯数字编号", "改用中文编号体系（如 第一章、第一节、一、）"
			}
			vs = append(vs, Violation{RuleID: ruleIDHeadingOrder, Line: line,
				Problem: problem, Suggestion: suggestion})
			continue
		}
		// 层级跳变（# 数只能逐级递进）
		if hashes > prevHashes+1 {
			vs = append(vs, Violation{RuleID: ruleIDHeadingOrder, Line: line,
				Problem:    fmt.Sprintf("标题层级从 %d 跳至 %d", prevHashes, hashes),
				Suggestion: "标题层级应逐级递进，不可跳级"})
		}
		prevHashes = hashes
		// 篇/章/节编号连续（同类各自递进）
		if m := bookCNNumRe.FindString(rest); m != "" {
			kind := m[len(m)-1] // 篇/章/节
			n := cnToInt(m[1 : len(m)-1])
			if prev, ok := topSeq[kind]; ok && n != prev+1 {
				vs = append(vs, Violation{RuleID: ruleIDHeadingOrder, Line: line,
					Problem:    fmt.Sprintf("%s编号不连续（上一%s为 %d，got %d）", string(kind), string(kind), prev, n),
					Suggestion: "编号须连续递进"})
			}
			topSeq[kind] = n
		}
	}
	if firstHeading {
		vs = append(vs, Violation{RuleID: ruleIDHeadingOrder, Line: 1,
			Problem: "缺少标题：全文未找到任何标题", Suggestion: "根据书稿内容补充文件顶层标题（书名或章节标题）"})
	}
	return vs
}

// bookTopLevelOK 判断首个标题是否符合 book_top_level 要求的文件顶层级别。
func bookTopLevelOK(rest, level string) bool {
	switch level {
	case "book":
		return !isBookNumbered(rest) // 整本书单文件：首标题须为书名（无编号）
	case "chapter":
		m := bookCNNumRe.FindString(rest)
		return m != "" && (strings.HasSuffix(m, "章") || strings.HasSuffix(m, "篇"))
	case "section":
		return isBookNumbered(rest) // 节文件：任意合法中文编号
	default: // auto / 空
		return true // 书名或任意合法编号均可
	}
}

// bookTopLevelHint 返回 book_top_level 校验未通过时的违规提示。
func bookTopLevelHint(level string) (problem, suggestion string) {
	switch level {
	case "book":
		return "缺少书名：首个标题带编号", "设计一个不编号的书名置于文首"
	case "chapter":
		return "首个标题不是篇/章级", "文件顶层标题应为 第X篇/第X章（如 第一章 ×××）"
	default: // section
		return "首个标题未带中文编号", "文件顶层标题应为 第X节 或更低级中文编号（第一节/一、/（一））"
	}
}

// isBookNumbered 判断标题文字是否带合法的中文编号（book 七级体系之一）。
func isBookNumbered(rest string) bool {
	return bookCNNumRe.MatchString(rest) || bookCNOrderRe.MatchString(rest) ||
		bookCNParenRe.MatchString(rest) || bookNumDotRe.MatchString(rest) || bookNumParenRe.MatchString(rest)
}

// cnToInt 中文数字转阿拉伯数字（支持 一~九、十、百、零 组合，如 十五=15、一百零二=102）。
func cnToInt(s string) int {
	section, digit := 0, 0
	for _, r := range s {
		switch r {
		case '零':
			digit = 0
		case '一':
			digit = 1
		case '二':
			digit = 2
		case '三':
			digit = 3
		case '四':
			digit = 4
		case '五':
			digit = 5
		case '六':
			digit = 6
		case '七':
			digit = 7
		case '八':
			digit = 8
		case '九':
			digit = 9
		case '十':
			if digit == 0 {
				digit = 1
			}
			section += digit * 10
			digit = 0
		case '百':
			if digit == 0 {
				digit = 1
			}
			section += digit * 100
			digit = 0
		}
	}
	return section + digit
}

// parseHeadingNum 将编号串 "1.2.3" 拆为 []int{1,2,3}。
func parseHeadingNum(s string) []int {
	raw := strings.Split(s, ".")
	out := make([]int, 0, len(raw))
	for _, p := range raw {
		if n, err := strconv.Atoi(p); err == nil {
			out = append(out, n)
		}
	}
	return out
}

// joinHeadingNum 将 []int{1,2} 拼为 "1.2"。
func joinHeadingNum(parts []int) string {
	ss := make([]string, len(parts))
	for i, p := range parts {
		ss[i] = strconv.Itoa(p)
	}
	return strings.Join(ss, ".")
}

// validNextHeading 判断 parts 是否为 prev 的一个合法后续编号。
func validNextHeading(prev, parts []int) bool {
	// 深入一层：parts == prev + [1]
	if len(parts) == len(prev)+1 && parts[len(prev)] == 1 {
		ok := true
		for i := range prev {
			if parts[i] != prev[i] {
				ok = false
				break
			}
		}
		if ok {
			return true
		}
	}
	// 同级续 / 回退到某祖先续：parts == prev[:k-1] + (prev[k-1]+1)
	for k := 1; k <= len(prev); k++ {
		if len(parts) != k {
			continue
		}
		ok := parts[k-1] == prev[k-1]+1
		for j := 0; ok && j < k-1; j++ {
			if parts[j] != prev[j] {
				ok = false
			}
		}
		if ok {
			return true
		}
	}
	return false
}

// checkR15 章节完整性：spec.Sections 每项须出现在某个标题文字中（去编号、忽略大小写）。
//
// 缺口防护：AI 漏写整章时，编号/层级检查均无法发现，需与章节清单显式比对。
func checkR15(src *Source, spec *ManuscriptSpec) []Violation {
	if len(spec.Sections) == 0 {
		return nil
	}
	var headings []string
	for _, ln := range src.Body {
		t := strings.TrimSpace(ln)
		if !isHeading(t) {
			continue
		}
		if _, _, text := headingLevel(t); text != "" {
			headings = append(headings, strings.ToLower(text))
		}
	}
	var vs []Violation
	for _, sec := range spec.Sections {
		secLower := strings.ToLower(sec)
		found := false
		for _, h := range headings {
			if strings.Contains(h, secLower) {
				found = true
				break
			}
		}
		if !found {
			vs = append(vs, Violation{
				RuleID:     "R1.5",
				Line:       1,
				Problem:    "缺少章节：" + sec,
				Suggestion: "按 manuscript-spec.yaml 的 sections 清单补齐该章节",
			})
		}
	}
	return vs
}

// checkR16 空章节：编号标题到下一个标题之间无任何内容违规。
//
// 大标题（首个无编号标题）豁免；判定基于原始 Lines，章节内的表格/图片行算有内容。
func checkR16(src *Source, _ *ManuscriptSpec) []Violation {
	var vs []Violation
	first := true
	for i, ln := range src.Body {
		t := strings.TrimSpace(ln)
		if !isHeading(t) {
			continue
		}
		_, num, _ := headingLevel(t)
		if first {
			first = false
			if num == "" {
				continue // 大标题豁免
			}
		}
		if num == "" {
			continue // 未编号标题由 R1.3 处理
		}
		start := src.bodyIdx[i]   // 标题行号（1 起）
		end := len(src.Lines) + 1 // 下一标题行号；无则视为文件末尾后一行
		for j := i + 1; j < len(src.Body); j++ {
			if isHeading(strings.TrimSpace(src.Body[j])) {
				end = src.bodyIdx[j]
				break
			}
		}
		hasContent := false
		for k := start; k < end-1; k++ { // 0-based：标题行之后到下一标题之前
			if strings.TrimSpace(src.Lines[k]) != "" {
				hasContent = true
				break
			}
		}
		if !hasContent {
			vs = append(vs, Violation{
				RuleID:     "R1.6",
				Line:       start,
				Problem:    "标题下无正文内容",
				Suggestion: "补充该章节正文，或删除空标题",
			})
		}
	}
	return vs
}

// checkR17 空/重复标题：# 后无文字违规；编号或文字重复在第二次出现处违规。
func checkR17(src *Source, _ *ManuscriptSpec) []Violation {
	var vs []Violation
	seenNum := map[string]int{}  // 编号 → 首现行号
	seenText := map[string]int{} // 文字（小写）→ 首现行号
	for i, ln := range src.Body {
		t := strings.TrimSpace(ln)
		if !isHeading(t) {
			continue
		}
		_, num, text := headingLevel(t)
		line := src.bodyIdx[i]
		if num == "" && text == "" {
			vs = append(vs, Violation{
				RuleID:     ruleR17,
				Line:       line,
				Problem:    "空标题（# 后无文字）",
				Suggestion: "删除空标题或补充标题文字",
			})
			continue
		}
		if num != "" {
			if prevLine, dup := seenNum[num]; dup {
				vs = append(vs, Violation{
					RuleID:     ruleR17,
					Line:       line,
					Problem:    fmt.Sprintf("标题编号 %s 重复（首次出现在第 %d 行）", num, prevLine),
					Suggestion: "修正编号，保证唯一且按层级递增",
				})
			} else {
				seenNum[num] = line
			}
		}
		if text != "" {
			key := strings.ToLower(text)
			if prevLine, dup := seenText[key]; dup {
				vs = append(vs, Violation{
					RuleID:     ruleR17,
					Line:       line,
					Problem:    fmt.Sprintf("标题文字“%s”重复（首次出现在第 %d 行）", text, prevLine),
					Suggestion: "修改标题文字，避免重复",
				})
			} else {
				seenText[key] = line
			}
		}
	}
	return vs
}

// figTableKey 归一化图表键：表/table → tableN，图/figure → figureN。
func figTableKey(kind, num string) string {
	switch strings.ToLower(kind) {
	case "表", "table":
		return "table" + num
	default:
		return "figure" + num
	}
}

// nearTableOrImage 判断第 i 行 ±2 行内是否有 markdown 表格行（| 开头）或图片（![）。
func nearTableOrImage(lines []string, i int) bool {
	for j := i - 2; j <= i+2 && j < len(lines); j++ {
		if j < 0 {
			continue
		}
		t := strings.TrimSpace(lines[j])
		if strings.HasPrefix(t, "|") || strings.Contains(t, "![") {
			return true
		}
	}
	return false
}

// checkR18 图表交叉引用：正文引用图/表 N 但全文找不到题注定义违规。
//
// 题注定义 = 题注行（图N/表N/Figure N/Table N）满足之一：标题行、含图片 ![、
// 或 ±2 行内紧邻表格行（| 开头）。普通行文中的"如表1所示"不算定义。
func checkR18(src *Source, _ *ManuscriptSpec) []Violation {
	defined := map[string]bool{}
	for i, ln := range src.Lines {
		t := strings.TrimSpace(ln)
		for _, m := range figTableRefRe.FindAllStringSubmatch(t, -1) {
			if isHeading(t) || strings.Contains(t, "![") || nearTableOrImage(src.Lines, i) {
				defined[figTableKey(m[1], m[2])] = true
			}
		}
	}
	var vs []Violation
	reported := map[string]bool{}
	for i, ln := range src.Body {
		for _, m := range figTableRefRe.FindAllStringSubmatch(ln, -1) {
			key := figTableKey(m[1], m[2])
			if defined[key] || reported[key] {
				continue
			}
			reported[key] = true
			vs = append(vs, Violation{
				RuleID:     "R1.8",
				Line:       src.bodyIdx[i],
				Problem:    fmt.Sprintf("引用了%s%s，但全文未找到对应题注定义", m[1], m[2]),
				Suggestion: "补充该图表及题注（题注须紧邻表格或图片），或修正引用编号",
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

// checkR33 数字范围规范（yueshu.md 八、数字）：范围用波浪线、百分号只在范围末、
// 年份范围用一字线、复合单位分母用圆括号。
func checkR33(src *Source, _ *ManuscriptSpec) []Violation {
	var vs []Violation
	for i, ln := range src.Body {
		for _, p := range numberPatterns {
			if p.re.MatchString(ln) {
				vs = append(vs, Violation{RuleID: "R3.3", Line: src.bodyIdx[i],
					Problem: p.problem, Suggestion: p.suggestion})
				break
			}
		}
	}
	return vs
}

// checkR34 计量单位（yueshu.md 五、计量单位）：禁用中文单位词与英制/旧制单位。
// 时间单位例外（5～8天、6小时单独使用用中文）。
func checkR34(src *Source, _ *ManuscriptSpec) []Violation {
	var vs []Violation
	for i, ln := range src.Body {
		if cnUnitRe.MatchString(ln) {
			vs = append(vs, Violation{RuleID: "R3.4", Line: src.bodyIdx[i],
				Problem:    "使用了中文计量单位词或英制/旧制单位",
				Suggestion: "改用法定计量单位符号（如 mg、cm、mmHg）；时间单位可用中文（5～8天）"})
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

// checkR72 自我夸大：关键词表扫描违规（词表从 spec 读取，可用户自定义）。
func checkR72(src *Source, spec *ManuscriptSpec) []Violation {
	words := spec.BoastWordList()
	var vs []Violation
	for i, ln := range src.Body {
		lower := strings.ToLower(ln)
		for _, w := range words {
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
		t := strings.TrimSpace(ln)
		if t == "" {
			flush()
			continue
		}
		// 标题行不计入段落（R8.3 只检内容段落）
		if isHeading(t) {
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
//
// Types 维度：空=全部类型适用；仅特定类型时标注（如 R2.1 P值仅 empirical）。
func AllRules() []Rule {
	rules := []Rule{
		{ID: "R0.1", Name: "全文中文", Category: CatLanguage, Langs: []string{"zh"}, Types: nil, Method: MethodA, From: ModeChapter, Check: checkR01},
		{ID: "R0.2", Name: "标题中文", Category: CatLanguage, Langs: []string{"zh"}, Types: nil, Method: MethodA, From: ModeChapter, Check: checkR02},
		{ID: "R1.1", Name: "章节层级", Category: CatStructure, Langs: []string{"zh", "en"}, Types: nil, Method: MethodA, From: ModeChapter, Check: checkR11},
		{ID: "R1.2", Name: "标题长度", Category: CatHeading, Langs: []string{"zh", "en"}, Types: nil, Method: MethodA, From: ModeChapter, Check: checkR12},
		{ID: ruleIDHeadingOrder, Name: "标题规范", Category: CatHeading, Langs: []string{"zh", "en"}, Types: nil, Method: MethodA, From: ModeChapter, Check: checkR13},
		{ID: "R1.4", Name: "正文禁止加粗", Category: CatStructure, Langs: []string{"zh"}, Types: nil, Method: MethodA, From: ModeDraft, Check: checkR14},
		{ID: "R1.5", Name: "章节完整性", Category: CatStructure, Langs: []string{"zh", "en"}, Types: nil, Method: MethodA, From: ModeChapter, Check: checkR15},
		{ID: "R1.6", Name: "空章节", Category: CatStructure, Langs: []string{"zh", "en"}, Types: nil, Method: MethodA, From: ModeChapter, Check: checkR16},
		{ID: ruleR17, Name: "空/重复标题", Category: CatStructure, Langs: []string{"zh", "en"}, Types: nil, Method: MethodA, From: ModeChapter, Check: checkR17},
		{ID: "R1.8", Name: "图表交叉引用", Category: CatStructure, Langs: []string{"zh", "en"}, Types: nil, Method: MethodA, From: ModeChapter, Check: checkR18},
		{ID: ruleR21, Name: "P值格式", Category: CatStatistics, Langs: []string{"zh", "en"}, Types: []string{PaperTypeEmpirical}, Method: MethodA, From: ModeDraft, Check: checkR21},
		{ID: "R3.1", Name: "全半角", Category: CatPunctuation, Langs: []string{"zh"}, Types: nil, Method: MethodA, From: ModeDraft, Check: checkR31},
		{ID: "R3.2", Name: "中文引号", Category: CatPunctuation, Langs: []string{"zh"}, Types: nil, Method: MethodA, From: ModeDraft, Check: checkR32},
		{ID: "R3.3", Name: "数字范围", Category: CatPunctuation, Langs: []string{"zh"}, Types: nil, Method: MethodA, From: ModeDraft, Check: checkR33},
		{ID: "R3.4", Name: "计量单位", Category: CatPunctuation, Langs: []string{"zh"}, Types: nil, Method: MethodA, From: ModeDraft, Check: checkR34},
		{ID: "R4.2", Name: "句式冗余", Category: CatStyle, Langs: []string{"zh"}, Types: nil, Method: MethodS, From: ModeFinal, Check: checkR42},
		{ID: "R5.1", Name: "引用占位符", Category: CatCitation, Langs: []string{"zh", "en"}, Types: nil, Method: MethodA, From: ModeDraft, Check: checkR51},
		{ID: "R5.2", Name: "待引证标记", Category: CatCitation, Langs: []string{"zh", "en"}, Types: nil, Method: MethodA, From: ModeDraft, Check: checkR52},
		{ID: "R5.3", Name: "引用密度", Category: CatCitation, Langs: []string{"zh", "en"}, Types: nil, Method: MethodS, From: ModeFinal, Check: checkR53},
		{ID: "R6.1", Name: "引用位置", Category: CatCitation, Langs: []string{"zh", "en"}, Types: nil, Method: MethodA, From: ModeDraft, Check: checkR61},
		{ID: "R7.1", Name: "标题冒号", Category: CatHeading, Langs: []string{"zh", "en"}, Types: nil, Method: MethodA, From: ModeChapter, Check: checkR71},
		{ID: "R7.2", Name: "自我夸大", Category: CatBoastWords, Langs: []string{"zh", "en"}, Types: nil, Method: MethodA, From: ModeDraft, Check: checkR72},
		{ID: "R8.1", Name: "全文字数", Category: CatWordCounts, Langs: []string{"zh", "en"}, Types: nil, Method: MethodA, From: ModeFinal, Check: checkR81},
		{ID: "R8.2", Name: "摘要字数", Category: CatWordCounts, Langs: []string{"zh", "en"}, Types: nil, Method: MethodA, From: ModeFinal, Check: checkR82},
		{ID: "R8.3", Name: "段长", Category: CatWordCounts, Langs: []string{"zh", "en"}, Types: nil, Method: MethodS, From: ModeFinal, Check: checkR83},
		{ID: "R9.1", Name: "用户标记", Category: CatTodo, Langs: []string{"zh", "en"}, Types: nil, Method: MethodA, From: ModeChapter, Check: checkR91},
	}
	sort.Slice(rules, func(i, j int) bool { return rules[i].ID < rules[j].ID })
	return rules
}

// ValidCategories 返回全部合法的检查类别（用于 CLI 校验和帮助文本）。
func ValidCategories() []Category {
	return []Category{CatLanguage, CatStructure, CatStatistics, CatPunctuation, CatStyle, CatCitation, CatHeading, CatBoastWords, CatWordCounts, CatTodo}
}
