package lint

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// writeReportFixture 写临时 Markdown 文稿。
func writeReportFixture(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "doc.md")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	return path
}

// TestRenderMarkdown_CountsViolationsAndNextSteps 报告应展示通过时也可见的字数、
// 违规明细（按类别分组）与下一步动作。
func TestRenderMarkdown_CountsViolationsAndNextSteps(t *testing.T) {
	path := writeReportFixture(t, "# 标题\n\n## 摘要\n\n本研究纳入患者共十例。\n\n## 方法\n\n本文进行一次分析。\n")
	spec := &ManuscriptSpec{
		PaperType:     PaperTypeEmpirical,
		Lang:          LangZH,
		WordCount:     WordCounts{Total: []int{100, 200}, Abstract: []int{5, 20}},
		SectionLimits: map[string][2]int{"方法": {1, 5}},
	}
	rep := &Report{
		Files: []FileReport{{
			Path: path,
			Violations: []Violation{
				{RuleID: "R3.1", Line: 7, Problem: "中文后半角标点", Suggestion: "改为全角", Fixable: true},
				{RuleID: "R4.2", Line: 3, Problem: "句式可能冗余 | 含竖线", Suggestion: "需人工确认"},
			},
		}},
		ExitHint:        "fix_and_rerun",
		FixHint:         "1 处可自动修正，先运行：litkit fix doc.md",
		ManualChecklist: []string{"数据一致性：核对正文数据与表格/图片是否一致"},
	}
	out := RenderMarkdown(rep, spec, ReportMeta{
		Generated: time.Date(2026, 11, 26, 15, 4, 5, 0, time.UTC),
		Mode:      ModeFinal,
		Lang:      LangZH,
		PaperType: PaperTypeEmpirical,
	})

	for _, want := range []string{
		"2026-11-26 15:04:05",
		"**存在可自动修正的违规**",
		"final（成稿）",
		"empirical / 中文（zh）",
		"**24 字**", // 正文合计含标题行（与 R8.1 同口径）
		"**10 字**", // 摘要段
		"偏少",       // 22 < 100
		"合格",       // 摘要 10 ∈ [5,20]
		"超出",       // 方法 8 > 5
		"R3.1 全半角",
		"（可自动修正）",
		"#### 标点符号（1）",
		"#### 行文风格（1）",
		`含竖线`, // md 单元格转义后仍保留原文
		`\|`,  // 表格竖线被转义
		"litkit fix doc.md",
		"人工核对清单",
		"数据一致性",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("报告缺少 %q\n---\n%s", want, out)
		}
	}
}

// TestRenderMarkdown_Pass 全部通过时报告应明确写"通过"，且不虚构下一步动作。
func TestRenderMarkdown_Pass(t *testing.T) {
	path := writeReportFixture(t, "# 标题\n\n短文。\n")
	spec := &ManuscriptSpec{PaperType: PaperTypeEmpirical, Lang: LangZH}
	rep := &Report{Files: []FileReport{{Path: path}}, ExitHint: exitPass}
	out := RenderMarkdown(rep, spec, ReportMeta{Generated: time.Now(), Mode: ModeDraft, Lang: LangZH})

	for _, want := range []string{"**通过**", "未发现违规。", "无需处理", "未找到摘要标题"} {
		if !strings.Contains(out, want) {
			t.Errorf("通过报告缺少 %q\n---\n%s", want, out)
		}
	}
}

// TestRenderMarkdown_RecencyAndCitationRefs 引用时效与评分结果应进入报告。
func TestRenderMarkdown_RecencyAndCitationRefs(t *testing.T) {
	path := writeReportFixture(t, "# 标题\n\n短文。\n")
	spec := &ManuscriptSpec{PaperType: PaperTypeEmpirical, Lang: LangZH}
	rep := &Report{
		Files:    []FileReport{{Path: path}},
		ExitHint: exitPass,
		Recency:  &RecencySummary{Total: 10, Within5: 7, Over5: 3, Over10: 1, RecentRatio: 0.7},
		CitationRefs: &CitationRelevanceReport{
			Enabled: true,
			Models:  []string{"model-a"},
			Results: []CitationRelevanceItem{{
				File: "doc.md", Line: 3, CiteKey: "key2024",
				MeanScore: 0.2, Consensus: 0.4, LowScore: true, LowConsensus: true,
				ScoredModels: 1, UnverifiedNumbers: []string{"42"},
			}},
		},
	}
	out := RenderMarkdown(rep, spec, ReportMeta{Generated: time.Now(), Mode: ModeFinal, Lang: LangZH})

	for _, want := range []string{"引用时效", "70.0%", "低吻合度", "低一致率", "摘要中未见数字：42", "model-a"} {
		if !strings.Contains(out, want) {
			t.Errorf("报告缺少 %q\n---\n%s", want, out)
		}
	}
}

// TestWordStats_SectionSubheadingCounted 子标题内容计入父节（与 R10.2 同口径）。
func TestWordStats_SectionSubheadingCounted(t *testing.T) {
	path := writeReportFixture(t, "# 标题\n\n## 方法\n\n### 子节\n\n子节正文内容。\n")
	src, err := ParseSource(path)
	if err != nil {
		t.Fatalf("ParseSource: %v", err)
	}
	spec := &ManuscriptSpec{SectionLimits: map[string][2]int{"方法": {1, 100}}}
	fw := wordStats(src, spec)
	if len(fw.sections) != 1 || !fw.sections[0].found {
		t.Fatalf("应定位到方法节，got %+v", fw.sections)
	}
	if fw.sections[0].words != 8 {
		t.Errorf("方法节应含子标题与子节正文共 8 字，got %d", fw.sections[0].words)
	}
}
