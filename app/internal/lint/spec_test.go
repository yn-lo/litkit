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
