package lint

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// makeProposalSpec 构造带字数区间的 proposal 测试 spec。
func makeProposalSpec() *ManuscriptSpec {
	spec := SpecForType(PaperTypeProposal, LangZH)
	spec.Sections = []string{"中文摘要", "立项依据"}
	spec.SectionLimits = map[string][2]int{
		"中文摘要": {10, 20},
	}
	return spec
}

func proposalDraft() Options {
	return Options{Lang: "zh", Mode: ModeDraft, PaperType: PaperTypeProposal}
}

// TestSpecForType_proposal proposal preset 应可加载且引用模式为 endnote。
func TestSpecForType_proposal(t *testing.T) {
	spec := SpecForType(PaperTypeProposal, LangZH)
	if spec.PaperType != PaperTypeProposal {
		t.Errorf("paper_type 应为 proposal，got %q", spec.PaperType)
	}
	if spec.CitationMode != CitationModeEndnote {
		t.Errorf("proposal 引用模式应为 endnote，got %q", spec.CitationMode)
	}
	if len(spec.SectionList()) == 0 {
		t.Error("proposal 应有章节清单")
	}
}

// TestSpec_Validate_citationMode citation_mode 非法值应被拒绝；空=inline 向后兼容。
func TestSpec_Validate_citationMode(t *testing.T) {
	spec := DefaultSpec()
	spec.CitationMode = "bogus"
	if err := spec.Validate(); err == nil {
		t.Error("citation_mode 非法值应校验失败")
	}
	spec2 := DefaultSpec()
	spec2.CitationMode = ""
	if err := spec2.Validate(); err != nil {
		t.Errorf("空 citation_mode 应视为 inline 合法: %v", err)
	}
	spec3 := DefaultSpec()
	spec3.CitationMode = CitationModeInline
	if err := spec3.Validate(); err != nil {
		t.Errorf("合法 citation_mode=inline 不应失败: %v", err)
	}
	spec4 := DefaultSpec()
	spec4.CitationMode = CitationModeEndnote
	if err := spec4.Validate(); err != nil {
		t.Errorf("合法 citation_mode=endnote 不应失败: %v", err)
	}
}

// TestParseSource_ProposalScope sections 定义时，首个 section 标题之前的
// 非标题行（封面字段）不进 Body，标题行保留；无 sections 的 spec 行为不变。
func TestParseSource_ProposalScope(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "doc.md")
	content := "# 协和医院申请书\n\n课题负责人：罗洋\n\n联系电话：〔　　〕\n\n## 中文摘要\n\n摘要正文。\n\n## 立项依据\n\n立项正文。\n"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	spec := SpecForType(PaperTypeProposal, LangZH)
	spec.Sections = []string{"中文摘要", "立项依据"}
	src, err := parseSourceWithSpec(path, spec)
	if err != nil {
		t.Fatalf("ParseSource: %v", err)
	}
	if !bodyHas(src, "# 协和医院申请书") {
		t.Errorf("主标题应保留在 Body，got %v", src.Body)
	}
	if !bodyHas(src, "摘要正文。") || !bodyHas(src, "立项正文。") {
		t.Errorf("section 正文应在 Body，got %v", src.Body)
	}
	if bodyHas(src, "课题负责人：罗洋") {
		t.Errorf("封面字段不应在 Body，got %v", src.Body)
	}
	// 无 sections：行为不变（封面行仍在 Body）
	spec2 := DefaultSpec()
	spec2.Sections = nil
	src2, err := parseSourceWithSpec(path, spec2)
	if err != nil {
		t.Fatalf("ParseSource: %v", err)
	}
	if !bodyHas(src2, "课题负责人：罗洋") {
		t.Errorf("无 sections 时封面行应保留（向后兼容），got %v", src2.Body)
	}
}

// runContentWithSpec 以 spec 解析（封面裁剪生效）后执行验证。
func runContentWithSpec(t *testing.T, content string, spec *ManuscriptSpec, opts Options) FileReport {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "doc.md")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	src, err := parseSourceWithSpec(path, spec)
	if err != nil {
		t.Fatalf("parseSourceWithSpec: %v", err)
	}
	return Run(src, spec, opts)
}

// TestParseSource_AbstractBodyKept 摘要节正文不能被封面裁剪吞掉
// （empirical/review 的 sections 从「引言」开始，摘要排在其前）。
func TestParseSource_AbstractBodyKept(t *testing.T) {
	spec := SpecForType(PaperTypeEmpirical, LangZH)
	dir := t.TempDir()
	path := filepath.Join(dir, "doc.md")
	content := "# 研究标题\n\n## 摘要\n\n这是摘要正文。\n\n# 1 引言\n\n这是引言正文。\n"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	src, err := parseSourceWithSpec(path, spec)
	if err != nil {
		t.Fatalf("parseSourceWithSpec: %v", err)
	}
	if !bodyHas(src, "这是摘要正文。") {
		t.Errorf("摘要正文应保留在 Body，got %v", src.Body)
	}
	if !bodyHas(src, "这是引言正文。") {
		t.Errorf("引言正文应保留在 Body，got %v", src.Body)
	}
}

// TestR82_AbstractCounted 摘要字数应统计实际正文，而非恒为 0。
func TestR82_AbstractCounted(t *testing.T) {
	spec := SpecForType(PaperTypeEmpirical, LangZH)
	content := "# 研究标题\n\n## 摘要\n\n这是摘要正文。\n\n# 1 引言\n\n这是引言正文。\n"
	rep := runContentWithSpec(t, content, spec, zhFinal())
	vs := violationsOf(rep, "R8.2")
	if len(vs) != 1 {
		t.Fatalf("短摘要应报 R8.2，got %d 条：%+v", len(vs), vs)
	}
	if strings.Contains(vs[0].Problem, "摘要字数 0") {
		t.Errorf("摘要字数不应为 0：%q", vs[0].Problem)
	}
}

// TestR13_AbstractHeadingExempt 摘要标题惯例不编号，不应触发 R1.3。
func TestR13_AbstractHeadingExempt(t *testing.T) {
	spec := SpecForType(PaperTypeEmpirical, LangZH)
	content := "# 研究标题\n\n## 摘要\n\n摘要正文。\n\n# 1 引言\n\n正文。\n"
	rep := runContentWithSpec(t, content, spec, zhChapter())
	for _, v := range violationsOf(rep, "R1.3") {
		if strings.Contains(v.Problem, "标题未带数字编号") {
			t.Errorf("摘要标题不应要求编号：%+v", v)
		}
	}
}

// TestR49_AbstractCitation proposal 的「中文摘要」也应接受摘要禁引用检查。
func TestR49_AbstractCitation(t *testing.T) {
	spec := makeProposalSpec()
	content := "# 标书\n\n## 中文摘要\n\n摘要含引用[@abc]。\n\n## 立项依据\n\n正文。\n"
	rep := runContentWithSpec(t, content, spec, Options{Lang: LangZH, Mode: ModeFinal, PaperType: PaperTypeProposal})
	if len(violationsOf(rep, "R4.9")) == 0 {
		t.Errorf("中文摘要含引用应报 R4.9，got %+v", rep.Violations)
	}
}

// TestR82_ChineseAbstract proposal 的「中文摘要」字数应受 spec 约束。
func TestR82_ChineseAbstract(t *testing.T) {
	spec := makeProposalSpec()
	content := "# 标书\n\n## 中文摘要\n\n太短。\n\n## 立项依据\n\n正文。\n"
	rep := runContentWithSpec(t, content, spec, Options{Lang: LangZH, Mode: ModeFinal, PaperType: PaperTypeProposal})
	if len(violationsOf(rep, "R8.2")) == 0 {
		t.Errorf("过短的中文摘要应报 R8.2，got %+v", rep.Violations)
	}
}

// TestR102_SubsectionCountedInParent 父节含子标题时，子节正文计入父节字数。
func TestR102_SubsectionCountedInParent(t *testing.T) {
	spec := SpecForType(PaperTypeProposal, LangZH)
	spec.Sections = []string{"立项依据"}
	spec.SectionLimits = map[string][2]int{"立项依据": {100, 200}}
	content := "# 标书\n\n## 立项依据\n\n### 研究内容\n\n子节正文内容。\n"
	rep := runContentWithSpec(t, content, spec, proposalDraft())
	vs := violationsOf(rep, "R10.2")
	if len(vs) != 1 {
		t.Fatalf("应检出 1 条 R10.2，got %+v", vs)
	}
	if strings.Contains(vs[0].Problem, "字数 0 ") {
		t.Errorf("子节正文应计入父节字数：%q", vs[0].Problem)
	}
}

// TestR102_SectionWordCount 超字数/不足字数被检出；未配置区间的节不报。
func TestR102_SectionWordCount(t *testing.T) {
	spec := makeProposalSpec()
	over := "# 标书\n\n## 中文摘要\n\n" + strings.Repeat("摘要内容测试。", 10) + "\n\n## 立项依据\n\n" + strings.Repeat("立项正文，随便写。", 5) + "\n"
	rep := runContent(t, over, spec, proposalDraft())
	if got := violationsOf(rep, "R10.2"); len(got) == 0 {
		t.Errorf("摘要超字数应报 R10.2，violations=%v", rep.Violations)
	}
	// 边界内通过
	ok := "# 标书\n\n## 中文摘要\n\n" + strings.Repeat("字。", 15) + "\n\n## 立项依据\n\n" + strings.Repeat("立项正文，随便写。", 5) + "\n"
	rep2 := runContent(t, ok, spec, proposalDraft())
	if got := violationsOf(rep2, "R10.2"); len(got) != 0 {
		t.Errorf("区间内不应报 R10.2，got %v", got)
	}
}

// TestR103_EndnoteCitationMode endnote 模式：正文内联引用 [1]/[1,2] 违规，
// 文末无参考文献节违规；inline 模式不检查（向后兼容）。
func TestR103_EndnoteCitationMode(t *testing.T) {
	spec := makeProposalSpec()
	spec.CitationMode = CitationModeEndnote
	bad := "# 标书\n\n## 中文摘要\n\n术后疼痛发生率高[1]，研究显示[1,2]下降。\n\n## 立项依据\n\n正文。\n"
	rep := runContent(t, bad, spec, proposalDraft())
	if got := violationsOf(rep, "R10.3"); len(got) < 2 {
		t.Errorf("endnote 模式正文内联引用应报 R10.3（2 处），got %v", got)
	}
	// 文末无参考文献节
	noRefs := "# 标书\n\n## 中文摘要\n\n正文。\n\n## 立项依据\n\n正文。\n"
	rep2 := runContent(t, noRefs, spec, proposalDraft())
	if got := violationsOf(rep2, "R10.3"); len(got) == 0 {
		t.Error("endnote 模式文末无参考文献节应报 R10.3")
	}
	// 有参考文献节且正文无内联引用：通过
	good := "# 标书\n\n## 中文摘要\n\n术后疼痛发生率高。\n\n## 立项依据\n\n正文。\n\n# 主要参考文献\n\n1. 某文献\n"
	rep3 := runContent(t, good, spec, proposalDraft())
	if got := violationsOf(rep3, "R10.3"); len(got) != 0 {
		t.Errorf("合规 endnote 文稿不应报 R10.3，got %v", got)
	}
	// inline 模式不检查
	specInline := makeProposalSpec()
	specInline.CitationMode = CitationModeInline
	rep4 := runContent(t, bad, specInline, proposalDraft())
	if got := violationsOf(rep4, "R10.3"); len(got) != 0 {
		t.Errorf("inline 模式不应报 R10.3，got %v", got)
	}
}

// TestR104_Placeholder 占位符检查规则 R10.4 应为 S 类（提示不判 A 类违规），
// 检出 `〔　〕` 全角占位符。
func TestR104_Placeholder(t *testing.T) {
	var rule *Rule
	for _, r := range AllRules() {
		if r.ID == "R10.4" {
			rule = &r
			break
		}
	}
	if rule == nil {
		t.Fatal("R10.4 未注册")
	}
	if rule.Method != MethodS {
		t.Errorf("R10.4 应为 S 类（半自动提示），got %s", rule.Method)
	}
	spec := makeProposalSpec()
	doc := "# 标书\n\n## 中文摘要\n\n经费预算〔　　〕万元。\n\n## 立项依据\n\n正文。\n"
	rep := runContent(t, doc, spec, proposalDraft())
	if got := violationsOf(rep, "R10.4"); len(got) == 0 {
		t.Errorf("占位符应被 R10.4 提示，violations=%v", rep.Violations)
	}
	clean := "# 标书\n\n## 中文摘要\n\n经费预算十万元。\n\n## 立项依据\n\n正文。\n"
	rep2 := runContent(t, clean, spec, proposalDraft())
	if got := violationsOf(rep2, "R10.4"); len(got) != 0 {
		t.Errorf("无占位符不应报 R10.4，got %v", got)
	}
}
