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

func TestPromptWorkDir(t *testing.T) {
	cwd := filepath.Join("E:", "paper")
	if got := promptWorkDir(strings.NewReader("\n"), cwd); got != cwd {
		t.Errorf("留空应使用当前目录，got %q", got)
	}
	if got := promptWorkDir(strings.NewReader("  D:\\tmp  \n"), cwd); got != `D:\tmp` {
		t.Errorf("应使用输入并去除空白，got %q", got)
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
	// .agents/skills/ Agent Skills（仓库级标准位置）
	skillPath := filepath.Join(dir, ".agents", "skills", "litkit", "SKILL.md")
	if _, err := os.Stat(skillPath); err != nil {
		t.Errorf("应生成 .agents/skills/litkit/SKILL.md：%v", err)
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
	// 参考文献文件
	for _, rel := range []string{
		".agents/skills/litkit/references/literature-search.md",
		".agents/skills/litkit/references/manuscript-writing.md",
		".agents/skills/academic-style/SKILL.md",
		".agents/skills/manuscript-format/SKILL.md",
		".agents/skills/manuscript-format/references/empirical.md",
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
	for _, want := range []string{"litkit search", "--mode", "litkit verify", "--check", "ls .litkit/", "不写摘要", "proposal", "citation_mode"} {
		if !strings.Contains(got, want) {
			t.Errorf("AGENTS.md 应含 %q", want)
		}
	}
}

func TestInitWorkdir_createsMissingDir(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "papers", "new-project")
	if err := initWorkdir(dir, false, lint.PaperTypeEmpirical, lint.LangZH, ""); err != nil {
		t.Fatalf("initWorkdir: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, ".env")); err != nil {
		t.Errorf("应在新目录生成 .env：%v", err)
	}
}

// TestInitWorkdir_appendsWorkDirToExistingEnv 项目已有 .env（其他工具）时，
// 只补 LITKIT_WORK_DIR，不覆盖原内容，且重复 init 不重复追加。
func TestInitWorkdir_appendsWorkDirToExistingEnv(t *testing.T) {
	dir := t.TempDir()
	envPath := filepath.Join(dir, ".env")
	if err := os.WriteFile(envPath, []byte("DATABASE_URL=sqlite://local"), 0o600); err != nil {
		t.Fatalf("write .env: %v", err)
	}
	if err := initWorkdir(dir, false, lint.PaperTypeEmpirical, lint.LangZH, ""); err != nil {
		t.Fatalf("initWorkdir: %v", err)
	}
	data, err := os.ReadFile(envPath)
	if err != nil {
		t.Fatalf("read .env: %v", err)
	}
	got := string(data)
	if !strings.Contains(got, "DATABASE_URL=sqlite://local") {
		t.Errorf("不应覆盖已有配置：%s", got)
	}
	if !strings.Contains(got, "\nLITKIT_WORK_DIR=.\n") {
		t.Errorf("应追加 LITKIT_WORK_DIR：%q", got)
	}
	// 幂等：再次 init 不应重复追加
	if err := initWorkdir(dir, false, lint.PaperTypeEmpirical, lint.LangZH, ""); err != nil {
		t.Fatalf("initWorkdir again: %v", err)
	}
	data, _ = os.ReadFile(envPath)
	if n := strings.Count(string(data), "LITKIT_WORK_DIR="); n != 1 {
		t.Errorf("LITKIT_WORK_DIR 应只出现一次，got %d：%q", n, string(data))
	}
}

// TestInitWorkdir_forceKeepsEnv --force 不得覆盖已有 .env（可能含密钥）。
func TestInitWorkdir_forceKeepsEnv(t *testing.T) {
	dir := t.TempDir()
	envPath := filepath.Join(dir, ".env")
	original := "LITKIT_WORK_DIR=.\nLITKIT_LLM_API_KEY=sk-secret\n"
	if err := os.WriteFile(envPath, []byte(original), 0o600); err != nil {
		t.Fatalf("write .env: %v", err)
	}
	if err := initWorkdir(dir, true, lint.PaperTypeEmpirical, lint.LangZH, ""); err != nil {
		t.Fatalf("initWorkdir: %v", err)
	}
	data, _ := os.ReadFile(envPath)
	if string(data) != original {
		t.Errorf("--force 不应改动 .env：got %q", string(data))
	}
}

// TestInitWorkdir_forceKeepsVerifierModels --force 不得清空可能含 API key 的 verifier_models.json。
func TestInitWorkdir_forceKeepsVerifierModels(t *testing.T) {
	dir := t.TempDir()
	if err := initWorkdir(dir, false, lint.PaperTypeEmpirical, lint.LangZH, ""); err != nil {
		t.Fatalf("initWorkdir: %v", err)
	}
	vmPath := filepath.Join(dir, ".litkit", "verifier_models.json")
	custom := `{"models":[{"id":"x","enabled":true,"api_key":"sk-secret"}]}`
	if err := os.WriteFile(vmPath, []byte(custom), 0o600); err != nil {
		t.Fatalf("write verifier_models: %v", err)
	}
	if err := initWorkdir(dir, true, lint.PaperTypeEmpirical, lint.LangZH, ""); err != nil {
		t.Fatalf("initWorkdir force: %v", err)
	}
	data, _ := os.ReadFile(vmPath)
	if string(data) != custom {
		t.Errorf("--force 不应覆盖 verifier_models.json：got %q", string(data))
	}
}

// TestEnsureEnvWorkDir_bomKeyDetected BOM 后的 LITKIT_WORK_DIR 应被识别，不重复追加。
func TestEnsureEnvWorkDir_bomKeyDetected(t *testing.T) {
	dir := t.TempDir()
	envPath := filepath.Join(dir, ".env")
	if err := os.WriteFile(envPath, []byte("\uFEFFLITKIT_WORK_DIR=.\n"), 0o600); err != nil {
		t.Fatalf("write .env: %v", err)
	}
	added, err := ensureEnvWorkDir(envPath)
	if err != nil {
		t.Fatalf("ensureEnvWorkDir: %v", err)
	}
	if added {
		t.Error("BOM 后的工作目录应被识别，不应重复追加")
	}
}

// TestEnsureEnvWorkDir_emptyValueTreatedAsMissing 空值的 LITKIT_WORK_DIR 视为未配置。
func TestEnsureEnvWorkDir_emptyValueTreatedAsMissing(t *testing.T) {
	dir := t.TempDir()
	envPath := filepath.Join(dir, ".env")
	if err := os.WriteFile(envPath, []byte("LITKIT_WORK_DIR=\n"), 0o600); err != nil {
		t.Fatalf("write .env: %v", err)
	}
	added, err := ensureEnvWorkDir(envPath)
	if err != nil {
		t.Fatalf("ensureEnvWorkDir: %v", err)
	}
	if !added {
		t.Fatal("空值应补写 LITKIT_WORK_DIR")
	}
	data, _ := os.ReadFile(envPath)
	if !strings.HasSuffix(string(data), "\nLITKIT_WORK_DIR=.\n") {
		t.Errorf("应追加非空工作目录：%q", string(data))
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

// TestInitWorkdir_journalKeepsSpecComments --journal 不得抹掉模板顶部的撰写规定注释。
func TestInitWorkdir_journalKeepsSpecComments(t *testing.T) {
	dir := t.TempDir()
	if err := initWorkdir(dir, false, lint.PaperTypeReview, lint.LangZH, "测试期刊"); err != nil {
		t.Fatalf("initWorkdir: %v", err)
	}
	path := lint.SpecPath(dir, lint.PaperTypeReview, lint.LangZH)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read spec: %v", err)
	}
	if !strings.Contains(string(data), "撰写硬性规定") {
		t.Errorf("--journal 后模板注释丢失：%s", data)
	}
	spec, err := lint.LoadSpec(path)
	if err != nil {
		t.Fatalf("LoadSpec: %v", err)
	}
	if spec.Journal != "测试期刊" {
		t.Errorf("journal 应为 测试期刊，got %q", spec.Journal)
	}
}

func TestInitWorkdir_typeProposal(t *testing.T) {
	dir := t.TempDir()
	if err := initWorkdir(dir, false, lint.PaperTypeProposal, lint.LangZH, ""); err != nil {
		t.Fatalf("initWorkdir: %v", err)
	}
	spec, err := lint.LoadSpec(lint.SpecPath(dir, lint.PaperTypeProposal, lint.LangZH))
	if err != nil {
		t.Fatalf("LoadSpec: %v", err)
	}
	if spec.PaperType != lint.PaperTypeProposal {
		t.Errorf("paper_type 应为 proposal，got %s", spec.PaperType)
	}
	if spec.CitationMode != lint.CitationModeEndnote {
		t.Errorf("proposal 引用模式应为 endnote，got %s", spec.CitationMode)
	}
	if len(spec.SectionList()) == 0 {
		t.Error("proposal 应有章节清单")
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
	got := string(after)
	if !strings.HasPrefix(got, "# 用户自定义\n") {
		t.Errorf("无 --force 不应覆盖用户 .env：%q", got)
	}
	if !strings.Contains(got, "LITKIT_WORK_DIR=.\n") {
		t.Errorf("应补充 LITKIT_WORK_DIR：%q", got)
	}
}
