package lint

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadSpec_defaults(t *testing.T) {
	dir := t.TempDir()
	// 文件缺失 → 返回默认值（无 error，调用方可忽略）
	spec, err := LoadSpec(filepath.Join(dir, "missing.yaml"))
	if err != nil {
		t.Fatalf("文件缺失应返回默认值，got error: %v", err)
	}
	if spec.PaperType != PaperTypeEmpirical {
		t.Errorf("默认 paper_type 应为 empirical，got %s", spec.PaperType)
	}
}

func TestLoadSpec_custom(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "s.yaml")
	content := `paper_type: review
lang: en
sections:
  - Introduction
  - Methods
word_counts:
  total: [3000, 8000]
  abstract: [200, 500]
  paragraph: [30, 500]
citation:
  count: [50, 100]
heading:
  max_level: 4
  max_length: 15
`
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	spec, err := LoadSpec(path)
	if err != nil {
		t.Fatalf("LoadSpec: %v", err)
	}
	if spec.PaperType != PaperTypeReview || spec.Lang != LangEN {
		t.Errorf("应解析 review/en，got %s/%s", spec.PaperType, spec.Lang)
	}
	if spec.Citation.Count[0] != 50 || spec.Citation.Count[1] != 100 {
		t.Errorf("citation 区间应 50-100，got %v", spec.Citation.Count)
	}
	// 未指定字段保留默认
	if spec.Heading.MaxLevel != 4 {
		t.Errorf("heading.max_level 默认应为 4，got %d", spec.Heading.MaxLevel)
	}
}

func TestLoadSpec_invalid(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bad.yaml")
	// 非法 paper_type
	if err := os.WriteFile(path, []byte("paper_type: typo\n"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	if _, err := LoadSpec(path); err == nil {
		t.Error("非法 paper_type 应报错")
	}
	// 非法区间
	content := "word_counts:\n  total: [8000, 3000]\n"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	if _, err := LoadSpec(path); err == nil {
		t.Error("min>max 应报错")
	}
}

func TestInitProjectInfra_createsSharedFiles(t *testing.T) {
	dir := t.TempDir()
	created, err := InitProjectInfra(dir, false)
	if err != nil {
		t.Fatalf("InitProjectInfra: %v", err)
	}
	if len(created) != 1 {
		t.Errorf("应创建 1 个共享文件，got %d: %v", len(created), created)
	}
	for _, rel := range []string{"verifier_models.json"} {
		if _, err := os.Stat(filepath.Join(dir, LitkitDir, rel)); err != nil {
			t.Errorf("应生成 .litkit/%s：%v", rel, err)
		}
	}
	// 再次调用（无 force）不应覆盖
	created2, err := InitProjectInfra(dir, false)
	if err != nil {
		t.Fatalf("InitProjectInfra: %v", err)
	}
	if len(created2) != 0 {
		t.Errorf("无 force 不应重复创建，got %v", created2)
	}
}

func TestInitPaperType_createsYaml(t *testing.T) {
	dir := t.TempDir()
	created, err := InitPaperType(dir, PaperTypeReview, LangZH, false)
	if err != nil {
		t.Fatalf("InitPaperType: %v", err)
	}
	if len(created) != 1 {
		t.Errorf("应创建 1 个文件，got %d: %v", len(created), created)
	}
	specPath := SpecPath(dir, PaperTypeReview, LangZH)
	if _, err := os.Stat(specPath); err != nil {
		t.Errorf("应生成 .litkit/review-zh/manuscript-spec.yaml：%v", err)
	}
	// 再次调用（无 force）不应覆盖
	created2, err := InitPaperType(dir, PaperTypeReview, LangZH, false)
	if err != nil {
		t.Fatalf("InitPaperType: %v", err)
	}
	if len(created2) != 0 {
		t.Errorf("无 force 不应重复创建，got %v", created2)
	}
}

func TestRenderWritingRules_zh(t *testing.T) {
	spec := DefaultSpec()
	got := RenderWritingRules(spec)
	for _, want := range []string{
		"## 撰写硬性规定",
		"论文类型：四段式实证（empirical）",
		"P 值：≥0.01 保留 2 位",
		"章节结构（empirical）",
		"引言 → 资料与方法",
		"全文 3000-8000",
		"30-45 篇",
		"GB/T 7714",
		"标题层级 ≤4 级",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("zh 渲染应含 %q\n%s", want, got)
		}
	}
	// journal 为空时不应出现"目标期刊"
	if strings.Contains(got, "目标期刊") {
		t.Error("journal 为空不应渲染目标期刊行")
	}
}

func TestRenderWritingRules_en(t *testing.T) {
	spec := DefaultSpec()
	spec.Lang = LangEN
	spec.PaperType = PaperTypeReview
	spec.Sections = []string{"Introduction", "Search Strategy", "Thematic Analysis", "Discussion and Outlook", "Conclusion"}
	got := RenderWritingRules(spec)
	for _, want := range []string{
		"论文类型：综述（review）",
		"academic English",
		"章节结构（review）",
		"Introduction → Search Strategy",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("en 渲染应含 %q", want)
		}
	}
	if strings.Contains(got, "P 值") {
		t.Error("en 渲染不应含 zh 专属 P 值规则")
	}
}

func TestRenderWritingRules_journal(t *testing.T) {
	spec := DefaultSpec()
	spec.Journal = "中华医学杂志"
	got := RenderWritingRules(spec)
	if !strings.Contains(got, "目标期刊：中华医学杂志") {
		t.Errorf("journal 非空应渲染目标期刊行\n%s", got)
	}
}
