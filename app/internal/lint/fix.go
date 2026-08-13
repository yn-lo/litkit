package lint

import (
	"regexp"
	"strconv"
	"strings"
	"unicode"
)

// FixReport 一次自动修正的统计。
type FixReport struct {
	Applied map[string]int `json:"applied"` // 规则 ID → 修正行数
}

// Total 返回修正总行数。
func (r FixReport) Total() int {
	n := 0
	for _, c := range r.Applied {
		n += c
	}
	return n
}

// 修正用正则（集中管理）。
var (
	halfWidthCnRe   = regexp.MustCompile(`[\x{4e00}-\x{9fff}][,.;:]`) // 中文后半角标点（R3.1）
	boldContentRe   = regexp.MustCompile(`\*\*([^*]+)\*\*`)           // 加粗标记（R1.4）
	citePunctMoveRe = regexp.MustCompile(`([。，,.])\s*(\[@[^\]]+\])`)  // 标点后紧跟引用（R6.1）

	// R3.3 数字范围子模式
	fixPctRangeRe  = regexp.MustCompile(`(\d+(?:\.\d+)?)\s*[-–]\s*(\d+(?:\.\d+)?)%`)                                // 10-16% → 10%～16%
	fixUnitRangeRe = regexp.MustCompile(`(\d+(?:\.\d+)?)\s*([a-zA-Z℃°]+)\s*[～~]\s*(\d+(?:\.\d+)?)\s*([a-zA-Z℃°]+)`) // 10kg～15kg → 10～15kg
	fixYearRangeRe = regexp.MustCompile(`(\d{4})年\s*[～~]\s*(\d{4})年`)                                               // 1988年～1998年 → 1988—1998年
	fixHourRangeRe = regexp.MustCompile(`(\d+(?:\.\d+)?)\s*—\s*(\d+(?:\.\d+)?)\s*(小时|分钟|天|周)`)                      // 24—48小时 → 24～48小时
	fixSlashUnitRe = regexp.MustCompile(`([a-zA-Z℃°]+)\s*/\s*([a-zA-Z℃°]+)\s*/\s*([a-zA-Z℃°]+)`)                    // mg/kg/d → mg/(kg·d)
)

// fixR31 中文后半角标点转全角（R3.1）。
func fixR31(line string) (string, bool) {
	changed := false
	out := halfWidthCnRe.ReplaceAllStringFunc(line, func(m string) string {
		changed = true
		r := []rune(m)
		switch r[1] {
		case ',':
			r[1] = '，'
		case '.':
			r[1] = '。'
		case ';':
			r[1] = '；'
		case ':':
			r[1] = '：'
		}
		return string(r)
	})
	return out, changed
}

// fixR32 中文上下文直引号转弯引号（R3.2，开合交替配对）。
func fixR32(line string) (string, bool) {
	runes := []rune(line)
	changed, open := false, true
	for i, r := range runes {
		if r != '"' {
			continue
		}
		prevCn := i > 0 && unicode.Is(unicode.Han, runes[i-1])
		nextCn := i < len(runes)-1 && unicode.Is(unicode.Han, runes[i+1])
		if !prevCn && !nextCn {
			continue // 非中文上下文（如英文引号）不动
		}
		if open {
			runes[i] = '“'
		} else {
			runes[i] = '”'
		}
		open, changed = !open, true
	}
	return string(runes), changed
}

// fixR21 P 值格式（R2.1）：大写、前导零、小数位、P<0.001。
func fixR21(line string) (string, bool) {
	changed := false
	out := pValueRe.ReplaceAllStringFunc(line, func(m string) string {
		sub := pValueRe.FindStringSubmatch(m)
		op, num := sub[2], sub[3]
		if strings.HasPrefix(num, ".") {
			num = "0" + num
		}
		val, _ := strconv.ParseFloat(num, 64)
		var fixed string
		switch {
		case val < pThreshold001:
			fixed = "P<0.001"
		case val < pThreshold01:
			fixed = "P" + op + strconv.FormatFloat(val, 'f', pDecimalsMid, 64)
		default:
			fixed = "P" + op + strconv.FormatFloat(val, 'f', pDecimalsHigh, 64)
		}
		if fixed != m {
			changed = true
		}
		return fixed
	})
	return out, changed
}

// fixR14 删除加粗标记保留内容（R1.4）。
func fixR14(line string) (string, bool) {
	out := boldContentRe.ReplaceAllString(line, "$1")
	return out, out != line
}

// fixR71 删除标题中的冒号（R7.1）。
func fixR71(line string) (string, bool) {
	if !isHeading(strings.TrimSpace(line)) {
		return line, false
	}
	out := strings.ReplaceAll(line, "：", "")
	out = strings.ReplaceAll(out, ":", "")
	return out, out != line
}

// fixR12 删除标题末尾标点（R1.2）。
func fixR12(line string) (string, bool) {
	if !isHeading(strings.TrimSpace(line)) {
		return line, false
	}
	if !headingEndRe.MatchString(strings.TrimSpace(line)) {
		return line, false
	}
	out := headingEndRe.ReplaceAllString(line, "")
	return out, out != line
}

// fixR61 引用移到标点前（R6.1）："结论。[@a]" → "结论[@a]。"。
func fixR61(line string) (string, bool) {
	changed := false
	out := citePunctMoveRe.ReplaceAllStringFunc(line, func(m string) string {
		sub := citePunctMoveRe.FindStringSubmatch(m)
		changed = true
		return sub[2] + sub[1]
	})
	return out, changed
}

// fixR33 数字范围格式（R3.3 可修子集）。
func fixR33(line string) (string, bool) {
	changed := false
	apply := func(re *regexp.Regexp, repl string) {
		before := line
		line = re.ReplaceAllString(line, repl)
		if line != before {
			changed = true
		}
	}
	// 替换模板用 ${n} 显式边界：$n 后紧跟汉字时会被 Go 解析为命名组导致替换为空
	apply(fixSlashUnitRe, "${1}/(${2}·${3})") // 先处理复合单位（避免 kg 被误当单位两端）
	apply(fixPctRangeRe, "${1}%～${2}%")
	apply(fixUnitRangeRe, "${1}～${3}${4}")
	apply(fixYearRangeRe, "${1}—${2}年")
	apply(fixHourRangeRe, "${1}～${2}${3}")
	return line, changed
}

// FixableRules 返回可自动修正的规则（Fix 非 nil），按 ID 排序。
// 仅覆盖"字符级、无歧义"规则（全半角/引号/P值/加粗/标题冒号/标题末尾标点/
// 引用位置/数字范围）；语义、结构、数值类规则不可修（Fix 为 nil）。
// Fix 是纯函数、幂等，配合 verify 使用："修完再验"闭环。
func FixableRules() []Rule {
	var out []Rule
	for _, r := range AllRules() {
		if r.Fix != nil {
			out = append(out, r)
		}
	}
	return out
}

// ApplyFixes 对内容逐行应用给定规则的 Fix（按传入顺序），返回修正内容与统计。
// 同一行可被多条规则依次修正；计数按"规则 × 行"。
// BOM（U+FEFF）在开头时先行剥离再逐行处理（否则 isHeading 等判断失效），修复后原样保留。
func ApplyFixes(content string, rules []Rule) (string, FixReport) {
	bom := strings.HasPrefix(content, "\uFEFF")
	if bom {
		content = strings.TrimPrefix(content, "\uFEFF")
	}
	lines := strings.Split(content, "\n")
	applied := map[string]int{}
	for i, ln := range lines {
		for _, r := range rules {
			if r.Fix == nil {
				continue
			}
			out, ok := r.Fix(ln)
			if ok {
				lines[i], ln = out, out
				applied[r.ID]++
			}
		}
	}
	out := strings.Join(lines, "\n")
	if bom {
		out = "\uFEFF" + out
	}
	return out, FixReport{Applied: applied}
}
