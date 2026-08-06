package config

import (
	"os"
	"path/filepath"
	"testing"
)

// writeEnv 在临时目录创建 .env 并返回其路径。
func writeEnv(t *testing.T, dir, content string) string {
	t.Helper()
	p := filepath.Join(dir, ".env")
	if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
		t.Fatalf("write .env: %v", err)
	}
	return p
}

func TestDiscoverEnvFile_explicitEnvFile(t *testing.T) {
	dir := t.TempDir()
	p := writeEnv(t, dir, "LITKIT_LANG=en\n")
	t.Setenv("LITKIT_ENV_FILE", p)
	// 即使 WORK_DIR 与 CWD 也有 .env，显式指定优先
	wd := t.TempDir()
	writeEnv(t, wd, "LITKIT_LANG=zh\n")
	t.Setenv("LITKIT_WORK_DIR", wd)

	got := DiscoverEnvFile()
	if got != p {
		t.Fatalf("LITKIT_ENV_FILE 应优先：got %s want %s", got, p)
	}
}

func TestDiscoverEnvFile_workDirEnv(t *testing.T) {
	wd := t.TempDir()
	want := writeEnv(t, wd, "LITKIT_LANG=en\n")
	t.Setenv("LITKIT_ENV_FILE", "")
	t.Setenv("LITKIT_WORK_DIR", wd)

	got := DiscoverEnvFile()
	if got != want {
		t.Fatalf("WORK_DIR/.env 应命中：got %s want %s", got, want)
	}
}

func TestDiscoverEnvFile_cwdEnv(t *testing.T) {
	cwd := t.TempDir()
	want := writeEnv(t, cwd, "LITKIT_LANG=en\n")
	t.Setenv("LITKIT_ENV_FILE", "")
	t.Setenv("LITKIT_WORK_DIR", "")

	got := discoverEnvFileFrom(cwd)
	if got != want {
		t.Fatalf("CWD/.env 应命中：got %s want %s", got, want)
	}
}

func TestDiscoverEnvFile_notFound(t *testing.T) {
	t.Setenv("LITKIT_ENV_FILE", "")
	t.Setenv("LITKIT_WORK_DIR", "")
	got := discoverEnvFileFrom(t.TempDir())
	if got != "" {
		t.Fatalf("无 .env 时应返回空串，got %s", got)
	}
}

func TestLoad_defaults(t *testing.T) {
	t.Setenv("LITKIT_ENV_FILE", "")
	t.Setenv("LITKIT_WORK_DIR", "")
	t.Setenv("LITKIT_LANG", "")
	t.Setenv("LITKIT_HTTP_TIMEOUT_MS", "")
	t.Setenv("LITKIT_HTTP_RETRIES", "")
	t.Setenv("LITKIT_EMBEDDING_PROVIDER", "")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Lang != "zh" {
		t.Errorf("默认 Lang 应为 zh，got %q", cfg.Lang)
	}
	if cfg.HTTPTimeoutMS != 15000 {
		t.Errorf("默认 HTTPTimeoutMS 应为 15000，got %d", cfg.HTTPTimeoutMS)
	}
	if cfg.HTTPRetries != 2 {
		t.Errorf("默认 HTTPRetries 应为 2，got %d", cfg.HTTPRetries)
	}
	if cfg.EmbeddingProvider != "local" {
		t.Errorf("默认 EmbeddingProvider 应为 local，got %q", cfg.EmbeddingProvider)
	}
	if cfg.RecentYears != 3 {
		t.Errorf("默认 RecentYears 应为 3，got %d", cfg.RecentYears)
	}
	if cfg.SearchMode != "tiab" {
		t.Errorf("默认 SearchMode 应为 tiab，got %q", cfg.SearchMode)
	}
}

func TestLoad_recentYearsAndSearchModeFromEnv(t *testing.T) {
	t.Setenv("LITKIT_ENV_FILE", "")
	t.Setenv("LITKIT_WORK_DIR", "")
	t.Setenv("LITKIT_DEFAULT_RECENT_YEARS", "10")
	t.Setenv("LITKIT_DEFAULT_SEARCH_MODE", "full")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.RecentYears != 10 {
		t.Errorf("RecentYears 应为 10，got %d", cfg.RecentYears)
	}
	if cfg.SearchMode != "full" {
		t.Errorf("SearchMode 应为 full，got %q", cfg.SearchMode)
	}
}

func TestLoad_readsEnvFile(t *testing.T) {
	dir := t.TempDir()
	p := writeEnv(t, dir, "LITKIT_LANG=en\nLITKIT_HTTP_TIMEOUT_MS=8000\n")
	t.Setenv("LITKIT_ENV_FILE", p)
	// 已加载的 .env 变量应被识别
	t.Setenv("LITKIT_HTTP_RETRIES", "5")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Lang != "en" {
		t.Errorf("Lang 应从 .env 读 en，got %q", cfg.Lang)
	}
	if cfg.HTTPTimeoutMS != 8000 {
		t.Errorf("HTTPTimeoutMS 应从 .env 读 8000，got %d", cfg.HTTPTimeoutMS)
	}
	if cfg.HTTPRetries != 5 {
		t.Errorf("HTTPRetries 应从环境读 5，got %d", cfg.HTTPRetries)
	}
}

func TestLoad_invalidIntFallsBackToDefault(t *testing.T) {
	t.Setenv("LITKIT_ENV_FILE", "")
	t.Setenv("LITKIT_WORK_DIR", "")
	t.Setenv("LITKIT_HTTP_TIMEOUT_MS", "not-a-number")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load 不应因 HTTP_TIMEOUT_MS 非法而报错： %v", err)
	}
	if cfg.HTTPTimeoutMS != 15000 {
		t.Errorf("非法值应回退默认 15000，got %d", cfg.HTTPTimeoutMS)
	}
}

func TestLoad_negativeValuesClamped(t *testing.T) {
	// 负重试次数/非正超时应钳制到安全下界，避免 httpclient panic 或无超时
	t.Setenv("LITKIT_ENV_FILE", "")
	t.Setenv("LITKIT_WORK_DIR", "")
	t.Setenv("LITKIT_HTTP_RETRIES", "-1")
	t.Setenv("LITKIT_HTTP_TIMEOUT_MS", "-5")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.HTTPRetries != 0 {
		t.Errorf("HTTPRetries=-1 应钳制为 0，got %d", cfg.HTTPRetries)
	}
	if cfg.HTTPTimeoutMS != DefaultHTTPTimeoutMS {
		t.Errorf("负超时应回退默认 %d，got %d", DefaultHTTPTimeoutMS, cfg.HTTPTimeoutMS)
	}
}

func TestLoad_secretsFromEnv(t *testing.T) {
	t.Setenv("LITKIT_ENV_FILE", "")
	t.Setenv("LITKIT_SEMANTIC_SCHOLAR_API_KEY", "key-xyz")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.SemanticScholarAPIKey != "key-xyz" {
		t.Errorf("SemanticScholarAPIKey 读取失败：got %q", cfg.SemanticScholarAPIKey)
	}
}

func TestLoad_llmConfig(t *testing.T) {
	t.Setenv("LITKIT_ENV_FILE", "")
	t.Setenv("LITKIT_LLM_API_KEY", "sk-abc123")
	t.Setenv("LITKIT_LLM_BASE_URL", "https://llm.example.com/v1")
	t.Setenv("LITKIT_LLM_TIMEOUT_MS", "60000")
	t.Setenv("LITKIT_VERIFY_LINT_LLM", "true")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.LLMAPIKey != "sk-abc123" {
		t.Errorf("LLMAPIKey 读取失败：got %q", cfg.LLMAPIKey)
	}
	if cfg.LLMBaseURL != "https://llm.example.com/v1" {
		t.Errorf("LLMBaseURL 读取失败：got %q", cfg.LLMBaseURL)
	}
	if cfg.LLMTimeoutMS != 60000 {
		t.Errorf("LLMTimeoutMS 应为 60000，got %d", cfg.LLMTimeoutMS)
	}
	if !cfg.VerifyLLMEnabled {
		t.Errorf("VerifyLLMEnabled 应为 true")
	}
}

func TestLoad_llmDefaults(t *testing.T) {
	t.Setenv("LITKIT_ENV_FILE", "")
	t.Setenv("LITKIT_LLM_API_KEY", "")
	t.Setenv("LITKIT_LLM_BASE_URL", "")
	t.Setenv("LITKIT_LLM_TIMEOUT_MS", "")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.LLMAPIKey != "" {
		t.Errorf("LLMAPIKey 默认应为空，got %q", cfg.LLMAPIKey)
	}
	if cfg.LLMBaseURL != "" {
		t.Errorf("LLMBaseURL 默认应为空，got %q", cfg.LLMBaseURL)
	}
	if cfg.LLMTimeoutMS != 30000 {
		t.Errorf("LLMTimeoutMS 默认应为 30000，got %d", cfg.LLMTimeoutMS)
	}
	if cfg.VerifyLLMEnabled {
		t.Errorf("VerifyLLMEnabled 默认应为 false")
	}
}
