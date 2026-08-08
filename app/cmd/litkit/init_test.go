package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"litkit/internal/config"
	"litkit/internal/lint"
)

func TestComputeSince_sincePriority(t *testing.T) {
	if got := computeSince(2020, 3); got != 2020 {
		t.Fatalf("--since 应优先，got %d", got)
	}
}

func TestComputeSince_years(t *testing.T) {
	want := time.Now().Year() - 3 + 1
	if got := computeSince(0, 3); got != want {
		t.Fatalf("years=3 应得 %d，got %d", want, got)
	}
}

func TestComputeSince_bothZero(t *testing.T) {
	if got := computeSince(0, 0); got != 0 {
		t.Fatalf("应返回 0（不过滤），got %d", got)
	}
}

func TestRequireWorkDir(t *testing.T) {
	if err := requireWorkDir(&config.Config{}); err == nil {
		t.Error("WorkDir 为空应拒绝（errNoWorkDir）")
	}
	if err := requireWorkDir(&config.Config{WorkDir: "e:\\Codes\\litkit\\workspace"}); err != nil {
		t.Errorf("WorkDir 非空应通过，got %v", err)
	}
}

func TestInitCmd_rejectsWithoutWorkDir(t *testing.T) {
	cmd := newInitCmd(&config.Config{})
	if err := cmd.Execute(); err == nil {
		t.Error("未设置 LITKIT_WORK_DIR 时 init 应拒绝执行")
	}
}

func TestInitWorkdir_createsFiles(t *testing.T) {
	dir := t.TempDir()
	if err := initWorkdir(dir, false, lint.PaperTypeEmpirical, lint.LangZH, ""); err != nil {
		t.Fatalf("initWorkdir: %v", err)
	}
	// .env / .litkit/AGENTS.md / .litkit/litkit.db
	for _, name := range []string{".env", ".litkit/AGENTS.md", ".litkit/litkit.db"} {
		if _, err := os.Stat(filepath.Join(dir, name)); err != nil {
			t.Errorf("应生成 %s：%v", name, err)
		}
	}
	// .litkit/ 目录共享文件 + 类型文件
	for _, rel := range []string{
		".litkit/verifier_models.json",
		".litkit/empirical-zh/manuscript-spec.yaml",
	} {
		if _, err := os.Stat(filepath.Join(dir, rel)); err != nil {
			t.Errorf("应生成 %s：%v", rel, err)
		}
	}
	// .litkit/skills/ Agent Skills
	skillPath := filepath.Join(dir, ".litkit", "skills", "litkit", "SKILL.md")
	if _, err := os.Stat(skillPath); err != nil {
		t.Errorf("应生成 .litkit/skills/litkit/SKILL.md：%v", err)
	}
	skillData, err := os.ReadFile(skillPath)
	if err != nil {
		t.Fatalf("read SKILL.md: %v", err)
	}
	skillContent := string(skillData)
	for _, want := range []string{"name: litkit", "litkit search", "litkit verify", "--check"} {
		if !strings.Contains(skillContent, want) {
			t.Errorf("SKILL.md 应含 %q", want)
		}
	}
	// references 文件
	for _, rel := range []string{
		".litkit/skills/litkit/references/literature-search.md",
		".litkit/skills/litkit/references/manuscript-writing.md",
	} {
		if _, err := os.Stat(filepath.Join(dir, rel)); err != nil {
			t.Errorf("应生成 %s：%v", rel, err)
		}
	}
	// AGENTS.md 应含检索命令 + 论文类型清单
	data, err := os.ReadFile(filepath.Join(dir, ".litkit", "AGENTS.md"))
	if err != nil {
		t.Fatalf("read AGENTS.md: %v", err)
	}
	got := string(data)
	for _, want := range []string{"litkit search", "--mode", "litkit verify", "--check", "ls .litkit/", "不写摘要"} {
		if !strings.Contains(got, want) {
			t.Errorf("AGENTS.md 应含 %q", want)
		}
	}
}

func TestInitWorkdir_typeReview(t *testing.T) {
	dir := t.TempDir()
	if err := initWorkdir(dir, false, lint.PaperTypeReview, lint.LangZH, ""); err != nil {
		t.Fatalf("initWorkdir: %v", err)
	}
	spec, err := lint.LoadSpec(lint.SpecPath(dir, lint.PaperTypeReview, lint.LangZH))
	if err != nil {
		t.Fatalf("LoadSpec: %v", err)
	}
	if spec.PaperType != lint.PaperTypeReview {
		t.Errorf("paper_type 应为 review，got %s", spec.PaperType)
	}
}

func TestInitWorkdir_noOverwriteWithoutForce(t *testing.T) {
	dir := t.TempDir()
	if err := initWorkdir(dir, false, lint.PaperTypeEmpirical, lint.LangZH, ""); err != nil {
		t.Fatalf("initWorkdir: %v", err)
	}
	envPath := filepath.Join(dir, ".env")
	if err := os.WriteFile(envPath, []byte("# 用户自定义\n"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := initWorkdir(dir, false, lint.PaperTypeEmpirical, lint.LangZH, ""); err != nil {
		t.Fatalf("再次 init: %v", err)
	}
	after, _ := os.ReadFile(envPath)
	if string(after) != "# 用户自定义\n" {
		t.Errorf("无 --force 不应覆盖用户 .env")
	}
}
