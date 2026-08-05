// Package config 加载 litkit 运行配置（叶子层）。
//
// 所有密钥经本包统一读取，禁止硬编码（FR-CONFIG-01、NFR-SEC-01）。
// .env 自动发现顺序（FR-CONFIG-02）：
//
//	LITKIT_ENV_FILE > WORK_DIR/.env > CWD/.env > 项目根 .env
//
// .env.example 模板见仓库根（FR-CONFIG-03）。
package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"

	"github.com/joho/godotenv"
)

// Defaults 默认值常量。
const (
	DefaultLang              = "zh"
	DefaultHTTPTimeoutMS     = 15000
	DefaultHTTPRetries       = 2
	DefaultEmbeddingProvider = "local"
	DefaultMaxResults        = 5      // 每源默认检索条数
	DefaultRecentYears       = 3      // 默认检索时间范围（最近 N 年，FR-SEARCH-13）
	DefaultSearchMode        = "tiab" // 默认检索等级：tiab=题目+摘要（+关键词，源支持时）；full=全文
	DefaultSearchTimeoutMS   = 60000  // 默认整体检索超时（含全部源并发 + 重试）
)

// Config litkit 运行配置。全部经环境变量读取（FR-CONFIG-01）。
type Config struct {
	EnvFile               string // 实际加载的 .env 路径（空表示未加载文件）
	WorkDir               string
	Lang                  string // 默认 zh（FR-CONFIG-04）
	HTTPTimeoutMS         int    // 默认 15000
	HTTPRetries           int    // 默认 2
	DefaultMaxResults     int    // 每源默认检索条数，默认 5
	RecentYears           int    // 默认检索时间范围（最近 N 年），默认 3
	SearchMode            string // 默认检索等级，tiab|full，默认 tiab
	SearchTimeoutMS       int    // 整体检索超时（ms），默认 60000
	SemanticScholarAPIKey string
	IEEEAPIKey            string
	ACMAPIKey             string
	EmbeddingProvider     string // 默认 local
	EmbeddingAPIKey       string
	UnpaywallEmail        string // 全文 OA 解析（Unpaywall，FR-FETCH-02）
	SciHubURL             string // Sci-Hub 兜底镜像（默认 https://sci-hub.se，FR-FETCH-03）
	FetchDownloadDir      string // 全文 PDF 落盘目录（默认 <WorkDir>/downloads）
}

// Load 发现并加载 .env，返回填充好的 Config。
//
// 已存在的进程环境变量优先于 .env 文件（godotenv.Load 不覆盖已设置变量）。
func Load() (*Config, error) {
	envFile := DiscoverEnvFile()
	return loadFrom(envFile)
}

// DiscoverEnvFile 按 FR-CONFIG-02 优先级查找 .env 路径。
// 未找到返回空串（不视为错误）。
func DiscoverEnvFile() string {
	// 1. LITKIT_ENV_FILE 显式指定
	if p := os.Getenv("LITKIT_ENV_FILE"); p != "" {
		if fileExists(p) {
			return p
		}
	}
	// 2. WORK_DIR/.env
	if wd := os.Getenv("LITKIT_WORK_DIR"); wd != "" {
		p := filepath.Join(wd, ".env")
		if fileExists(p) {
			return p
		}
	}
	// 3. CWD/.env，并向项目根上溯
	return discoverEnvFileFrom(getCwd())
}

// discoverEnvFileFrom 从指定目录起查找 .env，未命中时向父目录上溯。
func discoverEnvFileFrom(start string) string {
	dir := start
	for {
		p := filepath.Join(dir, ".env")
		if fileExists(p) {
			return p
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return ""
}

// loadFrom 加载指定 .env（若非空）并填充 Config。
func loadFrom(envFile string) (*Config, error) {
	if envFile != "" {
		if err := godotenv.Load(envFile); err != nil {
			return nil, fmt.Errorf("load .env %q: %w", envFile, err)
		}
	}
	cfg := &Config{
		EnvFile:               envFile,
		WorkDir:               os.Getenv("LITKIT_WORK_DIR"),
		Lang:                  getenvDefault("LITKIT_LANG", DefaultLang),
		HTTPTimeoutMS:         getenvInt("LITKIT_HTTP_TIMEOUT_MS", DefaultHTTPTimeoutMS),
		HTTPRetries:           getenvInt("LITKIT_HTTP_RETRIES", DefaultHTTPRetries),
		DefaultMaxResults:     getenvInt("LITKIT_DEFAULT_MAX_RESULTS", DefaultMaxResults),
		RecentYears:           getenvInt("LITKIT_DEFAULT_RECENT_YEARS", DefaultRecentYears),
		SearchMode:            getenvDefault("LITKIT_DEFAULT_SEARCH_MODE", DefaultSearchMode),
		SearchTimeoutMS:       getenvInt("LITKIT_SEARCH_TIMEOUT_MS", DefaultSearchTimeoutMS),
		SemanticScholarAPIKey: os.Getenv("LITKIT_SEMANTIC_SCHOLAR_API_KEY"),
		IEEEAPIKey:            os.Getenv("LITKIT_IEEE_API_KEY"),
		ACMAPIKey:             os.Getenv("LITKIT_ACM_API_KEY"),
		EmbeddingProvider:     getenvDefault("LITKIT_EMBEDDING_PROVIDER", DefaultEmbeddingProvider),
		EmbeddingAPIKey:       os.Getenv("LITKIT_EMBEDDING_API_KEY"),
		UnpaywallEmail:        os.Getenv("LITKIT_UNPAYWALL_EMAIL"),
		SciHubURL:             getenvDefault("LITKIT_SCI_HUB_URL", "https://sci-hub.se"),
		FetchDownloadDir:      os.Getenv("LITKIT_FETCH_DOWNLOAD_DIR"),
	}
	// 负数/零值下界钳制：负重试次数会使 httpclient 重试循环不执行而返回 nil 响应；
	// 非正超时会退化为无超时
	if cfg.HTTPRetries < 0 {
		cfg.HTTPRetries = 0
	}
	if cfg.HTTPTimeoutMS <= 0 {
		cfg.HTTPTimeoutMS = DefaultHTTPTimeoutMS
	}
	return cfg, nil
}

func fileExists(p string) bool {
	info, err := os.Stat(p)
	return err == nil && !info.IsDir()
}

func getCwd() string {
	wd, err := os.Getwd()
	if err != nil {
		return "."
	}
	return wd
}

func getenvDefault(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func getenvInt(key string, def int) int {
	v := os.Getenv(key)
	if v == "" {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return def
	}
	return n
}
