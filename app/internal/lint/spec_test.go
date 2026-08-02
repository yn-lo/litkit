package lint

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadSpec_defaults(t *testing.T) {
	dir := t.TempDir()
	// 文件缺失 → 回退默认并报错（调用方可忽略）
	spec, err := LoadSpec(filepath.Join(dir, "missing.yaml"))
	if err == nil {
		t.Fatal("文件缺失应返回 error")
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
citation:
  count: [50, 100]
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

func TestInitHarness_createsLitkit(t *testing.T) {
	dir := t.TempDir()
	created, err := InitHarness(dir, false)
	if err != nil {
		t.Fatalf("InitHarness: %v", err)
	}
	if len(created) != 4 {
		t.Errorf("应创建 4 个文件，got %d: %v", len(created), created)
	}
	for _, rel := range []string{
		"rules.md", "checklist.md",
		"specs/manuscript-spec.yaml", "verifier_models.json",
	} {
		if _, err := os.Stat(filepath.Join(dir, LitkitDir, rel)); err != nil {
			t.Errorf("应生成 .litkit/%s：%v", rel, err)
		}
	}
	// 再次调用（无 force）不应覆盖
	created2, err := InitHarness(dir, false)
	if err != nil {
		t.Fatalf("InitHarness: %v", err)
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
}

func TestRenderWritingRules_en(t *testing.T) {
	spec := DefaultSpec()
	spec.Lang = LangEN
	spec.PaperType = PaperTypeReview
	got := RenderWritingRules(spec)
	for _, want := range []string{
		"academic English",
		"章节结构（review）",
		"引言 → 文献检索方法",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("en 渲染应含 %q", want)
		}
	}
	if strings.Contains(got, "P 值") {
		t.Error("en 渲染不应含 zh 专属 P 值规则")
	}
}
