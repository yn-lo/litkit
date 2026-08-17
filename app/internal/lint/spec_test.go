package lint

import (
	"os"
	"path/filepath"
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

// ---- book 类型 ----

func TestSpecForType_book(t *testing.T) {
	spec := SpecForType(PaperTypeBook, LangZH)
	if spec.PaperType != PaperTypeBook {
		t.Errorf("paper_type 应为 book，got %s", spec.PaperType)
	}
	if spec.Heading.MaxLevel != 7 {
		t.Errorf("book 标题最大层级应为 7（yueshu.md 七级标题体系），got %d", spec.Heading.MaxLevel)
	}
	if len(spec.SectionList()) != 0 {
		t.Errorf("book 章节清单应为空（按章成册，R1.5 不启用），got %v", spec.SectionList())
	}
	if spec.WordCount.Total[0] >= spec.WordCount.Total[1] {
		t.Errorf("total 区间非法：%v", spec.WordCount.Total)
	}
}

func TestInitPaperType_book(t *testing.T) {
	dir := t.TempDir()
	created, err := InitPaperType(dir, PaperTypeBook, LangZH, false)
	if err != nil {
		t.Fatalf("InitPaperType(book): %v", err)
	}
	if len(created) != 1 {
		t.Errorf("应创建 1 个文件，got %d: %v", len(created), created)
	}
	if _, err := os.Stat(SpecPath(dir, PaperTypeBook, LangZH)); err != nil {
		t.Errorf("应生成 .litkit/book-zh/manuscript-spec.yaml：%v", err)
	}
}

// ---- skip_rules（spec 级永久跳过规则）----

func TestLoadSpec_skipRules(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "s.yaml")
	content := `paper_type: book
lang: zh
skip_rules:
  - R3.4
  - R8.1
word_counts:
  total: [500, 20000]
  abstract: [200, 500]
  paragraph: [50, 2000]
citation:
  count: [10, 200]
heading:
  max_level: 7
  max_length: 20
`
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	spec, err := LoadSpec(path)
	if err != nil {
		t.Fatalf("LoadSpec: %v", err)
	}
	if len(spec.SkipRules) != 2 || spec.SkipRules[0] != "R3.4" || spec.SkipRules[1] != "R8.1" {
		t.Errorf("应解析 skip_rules 为 [R3.4 R8.1]，got %v", spec.SkipRules)
	}
}

func TestLoadSpec_invalidSkipRule(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bad.yaml")
	content := `paper_type: book
lang: zh
skip_rules:
  - R9.9
word_counts:
  total: [500, 20000]
  abstract: [200, 500]
  paragraph: [50, 2000]
citation:
  count: [10, 200]
heading:
  max_level: 7
  max_length: 20
`
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	if _, err := LoadSpec(path); err == nil {
		t.Error("未知规则 ID 应报错")
	}
}

func TestLoadSpec_citationRunLimit(t *testing.T) {
	// yaml 显式配 run_limit: 2 → 解析为 2
	dir := t.TempDir()
	path := filepath.Join(dir, "s.yaml")
	content := `paper_type: empirical
lang: zh
citation:
  count: [20, 40]
  run_limit: 2
heading:
  max_level: 4
  max_length: 15
word_counts:
  total: [3500, 5000]
  abstract: [200, 500]
  paragraph: [100, 400]
`
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	spec, err := LoadSpec(path)
	if err != nil {
		t.Fatalf("LoadSpec: %v", err)
	}
	if spec.Citation.RunLimit != 2 {
		t.Errorf("citation.run_limit 应解析为 2，got %d", spec.Citation.RunLimit)
	}
	// 未配 run_limit 的 yaml → 0（规则层回退默认 3）
	noLimit := `paper_type: empirical
lang: zh
citation:
  count: [20, 40]
heading:
  max_level: 4
  max_length: 15
word_counts:
  total: [3500, 5000]
  abstract: [200, 500]
  paragraph: [100, 400]
`
	if err := os.WriteFile(path, []byte(noLimit), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	spec, err = LoadSpec(path)
	if err != nil {
		t.Fatalf("LoadSpec: %v", err)
	}
	if spec.Citation.RunLimit != 0 {
		t.Errorf("未配置 run_limit 应为 0，got %d", spec.Citation.RunLimit)
	}
	// 内置模板显式配 3（行为与缺省默认一致）
	if def := DefaultSpec(); def.Citation.RunLimit != 3 {
		t.Errorf("内置模板 run_limit 应为 3，got %d", def.Citation.RunLimit)
	}
}
