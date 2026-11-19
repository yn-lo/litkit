// Package config 加载 litkit 运行配置（叶子层）。
//
// 所有密钥经本包统一读取，禁止硬编码（FR-CONFIG-01、NFR-SEC-01）。
// .env 自动发现顺序（FR-CONFIG-02）：
//
//	LITKIT_ENV_FILE > WORK_DIR/.env > CWD/.env > 项目根 .env
//
// 配置仅作用于项目：不加载全局配置（如 %AppData%\litkit\.env），
// 每个项目通过 `litkit init` 生成自己的 .env。
// .env 中声明的相对 LITKIT_WORK_DIR 锚定到该 .env 所在目录（而非进程 cwd），
// 避免从其他目录（如 Start-Job / 子进程）加载同一 .env 时静默落到错误目录的空库。
// 进程环境变量优先于 .env。
// .env.example 模板见仓库根（FR-CONFIG-03）。
package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/joho/godotenv"
)

// Defaults 默认值常量。
const (
	DefaultLang            = "zh"
	DefaultHTTPTimeoutMS   = 15000
	DefaultHTTPRetries     = 2
	DefaultMaxResults      = 5      // 每源默认检索条数
	DefaultRecentYears     = 3      // 默认检索时间范围（最近 N 年，FR-SEARCH-13）
	DefaultSearchMode      = "tiab" // 默认检索等级：tiab=题目+摘要（+关键词，源支持时）；full=全文
	DefaultSearchTimeoutMS = 60000  // 默认整体检索超时（含全部源并发 + 重试）
	DefaultLLMTimeoutMS    = 30000  // 默认 LLM 单次评分超时（FR-LINT-08）
)

// Config litkit 运行配置。全部经环境变量读取（FR-CONFIG-01）。
type Config struct {
	EnvFile               string // 实际加载的 .env 路径（空表示未加载文件）
	WorkDir               string
	Lang                  string   // 默认 zh（FR-CONFIG-04）
	HTTPTimeoutMS         int      // 默认 15000
	HTTPRetries           int      // 默认 2
	DefaultMaxResults     int      // 每源默认检索条数，默认 5
	RecentYears           int      // 默认检索时间范围（最近 N 年），默认 3
	SearchMode            string   // 默认检索等级，tiab|full，默认 tiab
	SearchSources         []string // 默认检索源白名单（空=全部注册源）
	SearchExcludeSources  []string // 默认排除源（仅作用于未显式 -s 的检索）
	SearchTimeoutMS       int      // 整体检索超时（ms），默认 60000
	ProxyURL              string   // 显式代理（http/https/socks5）；空=直连并尊重标准 HTTPS_PROXY 环境变量
	SemanticScholarAPIKey string
	IEEEAPIKey            string
	ACMAPIKey             string
	UnpaywallEmail        string // 全文 OA 解析（Unpaywall，FR-FETCH-02）
	SciHubURL             string // Sci-Hub 兜底镜像（默认 https://sci-hub.se，FR-FETCH-03）
	FetchDownloadDir      string // 全文 PDF 落盘目录（默认 <WorkDir>/downloads）
	LLMAPIKey             string // LLM 评分 API key（FR-LINT-08，全局回落）
	LLMBaseURL            string // LLM API base URL（自托管 / 代理 endpoint，全局回落）
	LLMTimeoutMS          int    // LLM 单次评分超时，默认 30000
}

// Load 发现并加载项目级 .env，返回填充好的 Config。
//
// 已存在的进程环境变量优先于 .env 文件（不覆盖已设置变量）。
func Load() (*Config, error) {
	return loadFrom(discoverProjectEnvFile())
}

// DiscoverEnvFile 按 FR-CONFIG-02 优先级查找项目级 .env 路径。
// 未找到返回空串（不视为错误）。
func DiscoverEnvFile() string {
	return discoverProjectEnvFile()
}

// discoverProjectEnvFile 查找项目级 .env（不读取全局配置）。
func discoverProjectEnvFile() string {
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

// loadFrom 加载项目级 .env（进程环境变量最高优先）并填充 Config。
func loadFrom(projectFile string) (*Config, error) {
	workDirFile, err := loadEnvFile(projectFile)
	if err != nil {
		return nil, err
	}
	cfg := &Config{
		EnvFile:               projectFile,
		WorkDir:               resolveWorkDir(os.Getenv("LITKIT_WORK_DIR"), workDirFile),
		Lang:                  getenvDefault("LITKIT_LANG", DefaultLang),
		HTTPTimeoutMS:         getenvInt("LITKIT_HTTP_TIMEOUT_MS", DefaultHTTPTimeoutMS),
		HTTPRetries:           getenvInt("LITKIT_HTTP_RETRIES", DefaultHTTPRetries),
		DefaultMaxResults:     getenvInt("LITKIT_DEFAULT_MAX_RESULTS", DefaultMaxResults),
		RecentYears:           getenvInt("LITKIT_DEFAULT_RECENT_YEARS", DefaultRecentYears),
		SearchMode:            getenvDefault("LITKIT_DEFAULT_SEARCH_MODE", DefaultSearchMode),
		SearchSources:         SplitList(os.Getenv("LITKIT_SEARCH_SOURCES")),
		SearchExcludeSources:  SplitList(os.Getenv("LITKIT_SEARCH_EXCLUDE_SOURCES")),
		SearchTimeoutMS:       getenvInt("LITKIT_SEARCH_TIMEOUT_MS", DefaultSearchTimeoutMS),
		ProxyURL:              os.Getenv("LITKIT_PROXY_URL"),
		SemanticScholarAPIKey: os.Getenv("LITKIT_SEMANTIC_SCHOLAR_API_KEY"),
		IEEEAPIKey:            os.Getenv("LITKIT_IEEE_API_KEY"),
		ACMAPIKey:             os.Getenv("LITKIT_ACM_API_KEY"),
		UnpaywallEmail:        os.Getenv("LITKIT_UNPAYWALL_EMAIL"),
		SciHubURL:             getenvDefault("LITKIT_SCI_HUB_URL", "https://sci-hub.se"),
		FetchDownloadDir:      os.Getenv("LITKIT_FETCH_DOWNLOAD_DIR"),
		LLMAPIKey:             os.Getenv("LITKIT_LLM_API_KEY"),
		LLMBaseURL:            os.Getenv("LITKIT_LLM_BASE_URL"),
		LLMTimeoutMS:          getenvInt("LITKIT_LLM_TIMEOUT_MS", DefaultLLMTimeoutMS),
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

// loadEnvFile 加载项目级 .env，且不覆盖已存在的进程环境变量。
// 返回为 LITKIT_WORK_DIR 提供值的 .env 路径；该值来自进程环境变量或文件未声明时为空。
// 兼容 Windows 记事本 / PowerShell 5 保存的 UTF-8 BOM（否则首个键名会被污染）。
func loadEnvFile(f string) (string, error) {
	if f == "" {
		return "", nil
	}
	data, err := os.ReadFile(f)
	if err != nil {
		return "", fmt.Errorf("load .env %q: %w", f, err)
	}
	text := strings.TrimPrefix(string(data), "\uFEFF")
	kv, err := godotenv.Unmarshal(text)
	if err != nil {
		return "", fmt.Errorf("load .env %q: %w", f, err)
	}
	workDirFile := ""
	for k, v := range kv {
		if _, ok := os.LookupEnv(k); ok {
			continue
		}
		if err := os.Setenv(k, v); err != nil {
			return "", fmt.Errorf("set env %s: %w", k, err)
		}
		if k == "LITKIT_WORK_DIR" {
			workDirFile = f
		}
	}
	return workDirFile, nil
}

// resolveWorkDir 把 .env 声明的相对工作目录锚定到该 .env 所在目录；
// 进程环境变量中的相对值仍按 cwd 解析（符合环境变量语义）。
func resolveWorkDir(wd, envFile string) string {
	if wd == "" || filepath.IsAbs(wd) {
		return wd
	}
	base := getCwd()
	if envFile != "" {
		base = filepath.Dir(envFile)
	}
	if abs, err := filepath.Abs(base); err == nil {
		base = abs
	}
	return filepath.Clean(filepath.Join(base, wd))
}

// SplitList 解析逗号分隔列表：trim、去掉空项、按出现顺序去重。
// 同时供 CLI 解析 -s / --rule 等逗号分隔参数复用。
func SplitList(s string) []string {
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	seen := make(map[string]struct{}, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		if _, ok := seen[p]; ok {
			continue
		}
		seen[p] = struct{}{}
		out = append(out, p)
	}
	return out
}

func fileExists(p string) bool {
	info, err := os.Stat(p)
	return err == nil && !info.IsDir()
}

// LLMCredentials 按模型 ID 解析该模型的 API key 与 base URL（FR-LINT-08）。
//
// 命名约定：模型 ID 归一化（大写、非字母数字转下划线）后查
// LITKIT_LLM_API_KEY_<ID> / LITKIT_LLM_BASE_URL_<ID>；
// 未设置时回落全局 LITKIT_LLM_API_KEY / LITKIT_LLM_BASE_URL。
// 例：deepseek-chat → LITKIT_LLM_API_KEY_DEEPSEEK_CHAT。
func (c *Config) LLMCredentials(modelID string) (apiKey, baseURL string) {
	norm := normalizeModelID(modelID)
	apiKey = os.Getenv("LITKIT_LLM_API_KEY_" + norm)
	if apiKey == "" {
		apiKey = c.LLMAPIKey
	}
	baseURL = os.Getenv("LITKIT_LLM_BASE_URL_" + norm)
	if baseURL == "" {
		baseURL = c.LLMBaseURL
	}
	return apiKey, baseURL
}

// normalizeModelID 模型 ID → env 变量后缀（大写，非字母数字转下划线）。
// 例：gpt-4o → GPT_4O；qwen-plus → QWEN_PLUS。
func normalizeModelID(id string) string {
	up := strings.ToUpper(id)
	return strings.Map(func(r rune) rune {
		if (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') {
			return r
		}
		return '_'
	}, up)
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
