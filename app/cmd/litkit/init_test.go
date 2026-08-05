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
	if err := initWorkdir(dir, false, false, lint.PaperTypeEmpirical, lint.LangZH, ""); err != nil {
		t.Fatalf("initWorkdir: %v", err)
	}
	// .env / AGENTS.md / litkit.db
	for _, name := range []string{".env", "AGENTS.md", "litkit.db"} {
		if _, err := os.Stat(filepath.Join(dir, name)); err != nil {
			t.Errorf("应生成 %s：%v", name, err)
		}
	}
	// .litkit/ 目录四件套
	for _, rel := range []string{
		".litkit/rules.md",
		".litkit/checklist.md",
		".litkit/specs/manuscript-spec.yaml",
		".litkit/verifier_models.json",
	} {
		if _, err := os.Stat(filepath.Join(dir, rel)); err != nil {
			t.Errorf("应生成 %s：%v", rel, err)
		}
	}
	// AGENTS.md 应含检索命令 + 撰写硬性规定（事前指导）
	data, err := os.ReadFile(filepath.Join(dir, "AGENTS.md"))
	if err != nil {
		t.Fatalf("read AGENTS.md: %v", err)
	}
	got := string(data)
	for _, want := range []string{"litkit search", "--mode", "撰写硬性规定", "[@citeKey]", "litkit verify"} {
		if !strings.Contains(got, want) {
			t.Errorf("AGENTS.md 应含 %q", want)
		}
	}
}

func TestInitWorkdir_typeReview(t *testing.T) {
	dir := t.TempDir()
	if err := initWorkdir(dir, false, false, lint.PaperTypeReview, lint.LangZH, ""); err != nil {
		t.Fatalf("initWorkdir: %v", err)
	}
	spec, err := lint.LoadSpec(lint.SpecPath(dir))
	if err != nil {
		t.Fatalf("LoadSpec: %v", err)
	}
	if spec.PaperType != lint.PaperTypeReview {
		t.Errorf("paper_type 应为 review，got %s", spec.PaperType)
	}
	agents, _ := os.ReadFile(filepath.Join(dir, "AGENTS.md"))
	if !strings.Contains(string(agents), "文献检索方法") {
		t.Errorf("AGENTS.md 应含综述章节（文献检索方法）")
	}
}

func TestInitWorkdir_refreshRegeneratesAgents(t *testing.T) {
	dir := t.TempDir()
	if err := initWorkdir(dir, false, false, lint.PaperTypeEmpirical, lint.LangZH, ""); err != nil {
		t.Fatalf("initWorkdir: %v", err)
	}
	// 修改 yaml：引用区间改为 10-20
	specPath := lint.SpecPath(dir)
	spec, err := lint.LoadSpec(specPath)
	if err != nil {
		t.Fatalf("LoadSpec: %v", err)
	}
	spec.Citation.Count = []int{10, 20}
	if err := lint.WriteSpec(specPath, spec); err != nil {
		t.Fatalf("WriteSpec: %v", err)
	}
	// refresh：重新生成 AGENTS.md
	if err := initWorkdir(dir, false, true, lint.PaperTypeEmpirical, lint.LangZH, ""); err != nil {
		t.Fatalf("refresh: %v", err)
	}
	agents, _ := os.ReadFile(filepath.Join(dir, "AGENTS.md"))
	if !strings.Contains(string(agents), "全文 10-20 篇") {
		t.Errorf("refresh 后 AGENTS.md 应反映新引用区间 10-20")
	}
	// refresh 不应覆盖用户改的 yaml（10-20 保持）
	reloaded, _ := lint.LoadSpec(specPath)
	if reloaded.Citation.Count[1] != 20 {
		t.Errorf("refresh 不应改 yaml，got %v", reloaded.Citation.Count)
	}
}

func TestInitWorkdir_noOverwriteWithoutForce(t *testing.T) {
	dir := t.TempDir()
	if err := initWorkdir(dir, false, false, lint.PaperTypeEmpirical, lint.LangZH, ""); err != nil {
		t.Fatalf("initWorkdir: %v", err)
	}
	envPath := filepath.Join(dir, ".env")
	if err := os.WriteFile(envPath, []byte("# 用户自定义\n"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := initWorkdir(dir, false, false, lint.PaperTypeEmpirical, lint.LangZH, ""); err != nil {
		t.Fatalf("再次 init: %v", err)
	}
	after, _ := os.ReadFile(envPath)
	if string(after) != "# 用户自定义\n" {
		t.Errorf("无 --force 不应覆盖用户 .env")
	}
}
