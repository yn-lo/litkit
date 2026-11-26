package lint

import (
	"fmt"
	"sort"
	"strings"
	"time"
)

// 报告文本常量（goconst：多处复用）。
const (
	mdEmptyCell  = "—"
	mdRangeUnset = "未设置"
	reportTitle  = "# litkit 格式审查报告"
	reportNotice = "> 由 `litkit verify` 自动生成并覆盖，请勿手工编辑。"
	timeLayout   = "2006-01-02 15:04:05"
)

// categoryLabels 检查类别中文名（报告分组标题）。
var categoryLabels = map[Category]string{
	CatLanguage:    "语言合规",
	CatStructure:   "章节结构",
	CatStatistics:  "统计格式",
	CatPunctuation: "标点符号",
	CatStyle:       "行文风格",
	CatCitation:    "引用规范",
	CatHeading:     "标题规范",
	CatBoastWords:  "自我夸大",
	CatWordCounts:  "字数统计",
	CatTodo:        "用户标记",
}

// categoryLabel 检查类别中文名；未知类别回退为原始值。
func categoryLabel(c Category) string {
	if l, ok := categoryLabels[c]; ok {
		return l
	}
	return string(c)
}

// ReportMeta 人类可读报告的头部元信息。
type ReportMeta struct {
	Generated time.Time // 生成时间
	Mode      Mode      // 验证模式
	Lang      string    // 写作语言
	PaperType string    // 论文类型
}

// RenderMarkdown 渲染人类可读的验证报告（供 verify 写盘，不参与 JSON 契约）。
//
// 报告只做事实呈现，不新增判定：数据来自 lint.Report 与 manuscript-spec.yaml，
// 字数口径与 R8.1/R8.2/R10.2 一致。
func RenderMarkdown(rep *Report, spec *ManuscriptSpec, meta ReportMeta) string {
	if spec == nil {
		spec = &ManuscriptSpec{}
	}
	ruleByID := make(map[string]Rule, len(AllRules()))
	for _, r := range AllRules() {
		ruleByID[r.ID] = r
	}
	stats := make([]fileWords, len(rep.Files))
	totalWords := 0
	for i, f := range rep.Files {
		src, err := parseSourceWithSpec(f.Path, spec)
		if err != nil {
			continue
		}
		stats[i] = wordStats(src, spec)
		totalWords += stats[i].total
	}

	var b strings.Builder
	writef(&b, "%s\n\n%s\n\n", reportTitle, reportNotice)
	writeReportHeader(&b, rep, spec, meta, totalWords, ruleByID)
	for i, f := range rep.Files {
		writeFileSection(&b, f, stats[i], spec, ruleByID)
	}
	writeRecency(&b, rep.Recency)
	writeCitationRefs(&b, rep.CitationRefs)
	writeManualChecklist(&b, rep.ManualChecklist)
	writeNextSteps(&b, rep)
	return b.String()
}

// writef 写入格式化文本；strings.Builder 仅内存写入，永不失败。
func writef(b *strings.Builder, format string, args ...any) {
	_, _ = fmt.Fprintf(b, format, args...)
}

// writeReportHeader 输出总览表格。
func writeReportHeader(b *strings.Builder, rep *Report, spec *ManuscriptSpec, meta ReportMeta, totalWords int, ruleByID map[string]Rule) {
	total, aCount, sCount, fixable := countViolations(rep, ruleByID)
	b.WriteString("| 项目 | 值 |\n|---|---|\n")
	writef(b, "| 生成时间 | %s |\n", meta.Generated.Format(timeLayout))
	writef(b, "| 结论 | %s |\n", verdictLabel(rep))
	writef(b, "| 模式 | %s |\n", modeLabel(meta.Mode))
	writef(b, "| 论文类型 / 语言 | %s / %s |\n", paperTypeLabel(meta.PaperType, spec), langLabel(meta.Lang))
	writef(b, "| 文件数 | %d |\n", len(rep.Files))
	writef(b, "| 违规 | %d 处（A 类 %d / S 类 %d），可自动修正 %d 处 |\n", total, aCount, sCount, fixable)
	writef(b, "| 正文合计 | %d 字 |\n\n", totalWords)
}

// countViolations 汇总违规总数、A/S 类数与可自动修正数。
func countViolations(rep *Report, ruleByID map[string]Rule) (total, aCount, sCount, fixable int) {
	for _, f := range rep.Files {
		for _, v := range f.Violations {
			total++
			if v.Fixable {
				fixable++
			}
			switch ruleByID[v.RuleID].Method {
			case MethodA:
				aCount++
			case MethodS:
				sCount++
			}
		}
	}
	return
}

// writeFileSection 输出单个文件的字数与违规明细。
func writeFileSection(b *strings.Builder, f FileReport, fw fileWords, spec *ManuscriptSpec, ruleByID map[string]Rule) {
	writef(b, "## %s\n\n", f.Path)
	writeWordLines(b, fw, spec)
	writeSectionTable(b, fw.sections)
	writeViolations(b, f.Violations, ruleByID)
}

// writeWordLines 输出正文字数与摘要字数（通过时也展示）。
func writeWordLines(b *strings.Builder, fw fileWords, spec *ManuscriptSpec) {
	if !fw.parsed {
		b.WriteString("- 字数统计：读取文件失败\n\n")
		return
	}
	writef(b, "- 正文字数：**%d 字**（spec %s%s）\n",
		fw.total, rangeLabel(spec.WordCount.Total), rangeSuffix(fw.total, spec.WordCount.Total))
	if fw.hasAbstract {
		writef(b, "- 摘要字数：**%d 字**（spec %s%s）\n",
			fw.abstract, rangeLabel(spec.WordCount.Abstract), rangeSuffix(fw.abstract, spec.WordCount.Abstract))
	} else {
		b.WriteString("- 摘要字数：未找到摘要标题\n")
	}
	b.WriteString("\n")
}

// writeSectionTable 输出 spec.section_limits 配置的章节字数。
func writeSectionTable(b *strings.Builder, sections []sectionWords) {
	if len(sections) == 0 {
		return
	}
	b.WriteString("### 章节字数\n\n| 章节 | 行 | 字数 | 区间 | 状态 |\n|---|---|---|---|---|\n")
	for _, s := range sections {
		if !s.found {
			writef(b, "| %s | %s | %s | %s | 未找到该节标题 |\n",
				mdCell(s.name), mdEmptyCell, mdEmptyCell, rangeLabel(s.limit[:]))
			continue
		}
		writef(b, "| %s | %d | %d | %s | %s |\n",
			mdCell(s.name), s.line, s.words, rangeLabel(s.limit[:]), sectionStatus(s.words, s.limit))
	}
	b.WriteString("\n")
}

// writeViolations 按类别分组输出违规明细。
func writeViolations(b *strings.Builder, vs []Violation, ruleByID map[string]Rule) {
	if len(vs) == 0 {
		b.WriteString("未发现违规。\n\n")
		return
	}
	sorted := append([]Violation(nil), vs...)
	sort.SliceStable(sorted, func(i, j int) bool {
		if sorted[i].Line != sorted[j].Line {
			return sorted[i].Line < sorted[j].Line
		}
		return sorted[i].RuleID < sorted[j].RuleID
	})
	writef(b, "### 违规明细（%d）\n\n", len(sorted))
	for _, g := range groupByCategory(sorted, ruleByID) {
		writef(b, "#### %s（%d）\n\n| 行 | 规则 | 问题 | 建议 |\n|---|---|---|---|\n", g.label, len(g.vs))
		for _, v := range g.vs {
			suggestion := mdCell(v.Suggestion)
			if v.Fixable {
				suggestion += "（可自动修正）"
			}
			writef(b, "| %s | %s | %s | %s |\n",
				lineCell(v.Line), ruleCell(v.RuleID, ruleByID), mdCell(v.Problem), suggestion)
		}
		b.WriteString("\n")
	}
}

// violationGroup 同一检查类别的违规集合。
type violationGroup struct {
	label string
	vs    []Violation
}

// groupByCategory 按检查类别分组，顺序与 ValidCategories 一致；未知类别排最后。
func groupByCategory(vs []Violation, ruleByID map[string]Rule) []violationGroup {
	groups := make(map[Category][]Violation, len(categoryLabels))
	for _, v := range vs {
		c := ruleByID[v.RuleID].Category
		groups[c] = append(groups[c], v)
	}
	out := make([]violationGroup, 0, len(groups))
	seen := make(map[Category]bool, len(groups))
	for _, c := range ValidCategories() {
		if g := groups[c]; len(g) > 0 {
			out = append(out, violationGroup{label: categoryLabel(c), vs: g})
			seen[c] = true
		}
	}
	var unknown []Violation
	for c, g := range groups {
		if !seen[c] {
			unknown = append(unknown, g...)
		}
	}
	if len(unknown) > 0 {
		sort.SliceStable(unknown, func(i, j int) bool { return unknown[i].Line < unknown[j].Line })
		out = append(out, violationGroup{label: "其他", vs: unknown})
	}
	return out
}

// writeRecency 输出引用时效分档统计（仅报告，不是替换建议）。
func writeRecency(b *strings.Builder, r *RecencySummary) {
	if r == nil || r.Total == 0 {
		return
	}
	b.WriteString("## 引用时效\n\n")
	writef(b, "- 已解析年份的被引文献：%d 篇\n", r.Total)
	writef(b, "- 近 5 年：%d 篇（%.1f%%）\n", r.Within5, r.RecentRatio*percent)
	writef(b, "- 超过 5 年：%d 篇；超过 10 年：%d 篇\n", r.Over5, r.Over10)
	if r.Note != "" {
		writef(b, "- %s\n", r.Note)
	}
	b.WriteString("\n")
}

// writeCitationRefs 输出引用相关性评分（--report citation-refs）。
func writeCitationRefs(b *strings.Builder, cr *CitationRelevanceReport) {
	if cr == nil {
		return
	}
	b.WriteString("## 引用相关性评分\n\n")
	if !cr.Enabled {
		b.WriteString("未启用（需在 `.litkit/verifier_models.json` 配置可用模型与凭据）。\n\n")
		return
	}
	writef(b, "- 参与模型：%s\n", strings.Join(cr.Models, "、"))
	writef(b, "- 评分条目：%d\n\n", len(cr.Results))
	if len(cr.Results) == 0 {
		b.WriteString("本次没有可评分的引用。\n\n")
		return
	}
	b.WriteString("| 文件 | 行 | 引用 | 吻合度 | 一致率 | 提示 |\n|---|---|---|---|---|---|\n")
	for _, it := range cr.Results {
		writef(b, "| %s | %s | %s | %.2f | %.2f | %s |\n",
			mdCell(it.File), lineCell(it.Line), mdCell(it.CiteKey),
			it.MeanScore, it.Consensus, mdCell(citationFlags(it)))
	}
	b.WriteString("\n")
}

// citationFlags 汇总单条评分的提示标记。
func citationFlags(it CitationRelevanceItem) string {
	var flags []string
	if it.ScoredModels == 0 {
		flags = append(flags, "模型全部失败")
	}
	if it.LowScore {
		flags = append(flags, "低吻合度")
	}
	if it.LowConsensus {
		flags = append(flags, "低一致率")
	}
	if len(it.UnverifiedNumbers) > 0 {
		flags = append(flags, "摘要中未见数字："+strings.Join(it.UnverifiedNumbers, "、"))
	}
	if len(flags) == 0 {
		return mdEmptyCell
	}
	return strings.Join(flags, "；")
}

// writeManualChecklist 输出无法自动判定的人工核对项。
func writeManualChecklist(b *strings.Builder, list []string) {
	if len(list) == 0 {
		return
	}
	b.WriteString("## 人工核对清单\n\n")
	for _, c := range list {
		writef(b, "- [ ] %s\n", c)
	}
	b.WriteString("\n")
}

// writeNextSteps 输出下一步动作（fixHint / 人工复核 / 无需处理）。
func writeNextSteps(b *strings.Builder, rep *Report) {
	b.WriteString("## 下一步\n\n")
	if rep.FixHint != "" {
		writef(b, "- %s\n", rep.FixHint)
	}
	switch rep.ExitHint {
	case exitManualReview:
		b.WriteString("- S 类提示需人工判断，不影响验证通过\n")
	case exitPass:
		if rep.FixHint == "" {
			b.WriteString("- 无需处理\n")
		}
	default:
		if rep.FixHint == "" {
			b.WriteString("- 按违规明细逐项处理\n")
		}
	}
	b.WriteString("\n")
}

// fileWords 单文件字数摘要。
type fileWords struct {
	parsed      bool
	total       int
	hasAbstract bool
	abstract    int
	sections    []sectionWords
}

// sectionWords 单个配置章节的字数与区间。
type sectionWords struct {
	name  string
	line  int
	words int
	limit [2]int
	found bool
}

// wordStats 统计正文字数、摘要字数与配置章节字数（与 R8.1/R8.2/R10.2 同口径）。
func wordStats(src *Source, spec *ManuscriptSpec) fileWords {
	fw := fileWords{parsed: true}
	for _, ln := range src.Body {
		fw.total += wordCount(ln)
	}
	fw.hasAbstract, fw.abstract = abstractWords(src)
	if spec == nil || len(spec.SectionLimits) == 0 {
		return fw
	}
	names := make([]string, 0, len(spec.SectionLimits))
	for name := range spec.SectionLimits {
		names = append(names, name)
	}
	sort.Slice(names, func(a, b2 int) bool {
		if len(names[a]) != len(names[b2]) {
			return len(names[a]) > len(names[b2])
		}
		return names[a] < names[b2]
	})
	for _, name := range names {
		sw := sectionWords{name: name, limit: spec.SectionLimits[name]}
		sw.found, sw.line, sw.words = sectionWordsAt(src, name)
		fw.sections = append(fw.sections, sw)
	}
	sort.SliceStable(fw.sections, func(a, b2 int) bool {
		if fw.sections[a].found != fw.sections[b2].found {
			return fw.sections[a].found
		}
		return fw.sections[a].line < fw.sections[b2].line
	})
	return fw
}

// abstractWords 返回摘要段字数；未找到摘要标题时 found=false。
func abstractWords(src *Source) (found bool, words int) {
	for i, ln := range src.Body {
		t := strings.TrimSpace(ln)
		if !isHeading(t) || !isAbstractHeading(strings.TrimSpace(strings.TrimLeft(t, "#"))) {
			continue
		}
		for j := i + 1; j < len(src.Body); j++ {
			if isHeading(strings.TrimSpace(src.Body[j])) {
				break
			}
			words += wordCount(src.Body[j])
		}
		return true, words
	}
	return false, 0
}

// sectionWordsAt 定位首个标题文字包含 name 的章节，统计到下一个同级或更高级标题。
func sectionWordsAt(src *Source, name string) (found bool, line, words int) {
	target := strings.ToLower(name)
	for i, ln := range src.Body {
		t := strings.TrimSpace(ln)
		if !isHeading(t) {
			continue
		}
		level, _, text := headingLevel(t)
		if !strings.Contains(strings.ToLower(text), target) {
			continue
		}
		for j := i + 1; j < len(src.Body); j++ {
			tj := strings.TrimSpace(src.Body[j])
			if isHeading(tj) {
				if subLevel, _, _ := headingLevel(tj); subLevel <= level {
					break
				}
			}
			words += wordCount(src.Body[j])
		}
		return true, src.bodyIdx[i], words
	}
	return false, 0, 0
}

// verdictLabel 结论中文标签。
func verdictLabel(rep *Report) string {
	switch rep.ExitHint {
	case exitPass:
		return "**通过**"
	case exitFixAndRerun:
		return "**存在可自动修正的违规**"
	case exitManualReview:
		return "**需人工复核（S 类提示）**"
	default:
		return rep.ExitHint
	}
}

// modeLabel 验证模式中文标签。
func modeLabel(m Mode) string {
	switch m {
	case ModeChapter:
		return "chapter（章节结构）"
	case ModeDraft:
		return "draft（草稿）"
	default:
		return "final（成稿）"
	}
}

// paperTypeLabel 论文类型标签；meta 未指定时回落到 spec。
func paperTypeLabel(paperType string, spec *ManuscriptSpec) string {
	if paperType == "" {
		paperType = spec.PaperType
	}
	if paperType == "" {
		return "未指定"
	}
	return paperType
}

// langLabel 写作语言标签。
func langLabel(lang string) string {
	switch lang {
	case LangZH:
		return "中文（zh）"
	case LangEN:
		return "英文（en）"
	default:
		return lang
	}
}

// rangeLabel 字数区间标签。
func rangeLabel(r []int) string {
	if len(r) != 2 {
		return mdRangeUnset
	}
	return fmt.Sprintf("%d–%d 字", r[0], r[1])
}

// rangeSuffix 字数相对区间的状态后缀（未配置区间时为空）。
func rangeSuffix(n int, r []int) string {
	if len(r) != 2 {
		return ""
	}
	switch {
	case n < r[0]:
		return "，偏少"
	case n > r[1]:
		return "，超出"
	default:
		return "，合格"
	}
}

// sectionStatus 章节字数状态。
func sectionStatus(n int, r [2]int) string {
	switch {
	case n < r[0]:
		return "偏少"
	case n > r[1]:
		return "超出"
	default:
		return "合格"
	}
}

// ruleCell 规则单元格：ID + 规则名。
func ruleCell(id string, ruleByID map[string]Rule) string {
	if name := ruleByID[id].Name; name != "" {
		return id + " " + name
	}
	return id
}

// lineCell 行号单元格；0（全文级违规）显示占位符。
func lineCell(line int) string {
	if line <= 0 {
		return mdEmptyCell
	}
	return fmt.Sprintf("%d", line)
}

// mdCell 转义 Markdown 表格单元格内容。
func mdCell(s string) string {
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.ReplaceAll(s, "|", "\\|")
	return strings.TrimSpace(s)
}
