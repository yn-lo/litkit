// wireup.go 在 CLI 入口层组装依赖：config → registry/store/searcher。
//
// 入口层负责依赖注入（boundaries.md）；业务逻辑全部下沉 internal/core。
// 文献库按需打开：只读命令（rules/styles/export/--help 等）不创建库文件、
// 不触发 schema 迁移，避免并行调用时无谓争锁。
package main

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"

	"litkit/internal/config"
	"litkit/internal/core"
	"litkit/internal/sources"
	"litkit/internal/storage"
	"litkit/internal/util/httpclient"
)

// errNoWorkDir 未设置 LITKIT_WORK_DIR 的公共错误。
// 工作目录必须显式设置：所有需要文献库的命令（init/search/lib）拒绝执行，
// 避免生成物污染任意 CWD（FR-LIB-03）。
// 错误消息即文档：首次使用（尚无可发现的 .env）时必须给出可直接粘贴的命令。
var errNoWorkDir = errors.New(`未设置 LITKIT_WORK_DIR（也未找到可自动发现的项目 .env）。
  首次使用请显式指定后重跑，例如：
    PowerShell: $env:LITKIT_WORK_DIR = "<项目目录>"; litkit init --type empirical --lang zh
    bash:       LITKIT_WORK_DIR="<项目目录>" litkit init --type empirical --lang zh
  之后该目录的 .env 会写 LITKIT_WORK_DIR=.，在目录内调用无需再设。
  .env 自 CWD 向上溯源发现（LITKIT_ENV_FILE > WORK_DIR/.env > CWD/.env → 父目录）`)

// deps 持有一次 CLI 调用所需的全部依赖。
type deps struct {
	cfg      *config.Config
	registry *sources.Registry
	client   *httpclient.Client
	fetcher  *core.MetadataFetcher // 标识符反查（metadata/manuscript 用）

	storeOnce sync.Once
	store     *storage.Store
}

// Store 返回本地文献库；仅首次调用时打开（未设置工作目录或打开失败返回 nil）。
func (d *deps) Store() *storage.Store {
	if d.store != nil {
		return d.store // 测试注入或已打开
	}
	d.storeOnce.Do(func() {
		if d.cfg == nil || d.cfg.WorkDir == "" {
			return
		}
		s, err := storage.Open(storage.DBPath(d.cfg.WorkDir))
		if err != nil {
			// 库初始化失败不阻断只读能力：检索仍可退化为不入库
			fmt.Fprintf(os.Stderr, "litkit: storage init failed, 文献库不可用: %v\n", err)
			return
		}
		d.store = s
	})
	return d.store
}

// Searcher 构造检索器（会按需打开文献库）。
func (d *deps) Searcher() *core.Searcher {
	return core.NewSearcher(d.registry, d.Store(), d.cfg.DefaultMaxResults, d.cfg.SearchTimeoutMS)
}

// Fulltext 构造全文获取器；文献库不可用时返回 nil。
func (d *deps) Fulltext() *core.FulltextFetcher {
	st := d.Store()
	if st == nil {
		return nil
	}
	return core.NewFulltextFetcher(st, d.client, d.cfg.UnpaywallEmail, d.cfg.SciHubURL, d.cfg.DownloadDir())
}

// Close 释放依赖持有的资源（文献库连接）。
func (d *deps) Close() {
	if d.store != nil {
		_ = d.store.Close()
	}
}

// loadDeps 加载配置并组装依赖。
// 配置加载失败时退化为默认值并打印告警，不阻断 CLI（优雅降级）。
// 文献库延迟到命令实际需要时再打开。
func loadDeps() *deps {
	cfg, err := config.Load()
	if err != nil {
		// 配置加载失败不阻断：用默认值继续，让 CLI 仍可用；
		// WorkDir 仍从环境变量读取，避免丢失用户已声明的工作目录
		cfg = &config.Config{
			Lang:              config.DefaultLang,
			HTTPTimeoutMS:     config.DefaultHTTPTimeoutMS,
			HTTPRetries:       config.DefaultHTTPRetries,
			DefaultMaxResults: config.DefaultMaxResults,
			RecentYears:       config.DefaultRecentYears,
			SearchMode:        config.DefaultSearchMode,
			WorkDir:           os.Getenv("LITKIT_WORK_DIR"),
		}
		fmt.Fprintf(os.Stderr, "litkit: 配置加载失败，使用默认值: %v\n", err)
	}
	// 显式代理（LITKIT_PROXY_URL）：入口层一次性解析校验，非法时告警并降级直连
	proxy, perr := httpclient.ParseProxyURL(cfg.ProxyURL)
	if perr != nil {
		fmt.Fprintf(os.Stderr, "litkit: LITKIT_PROXY_URL 无效，忽略代理: %v\n", perr)
		proxy = nil
	}
	client := httpclient.New(httpclient.Options{
		TimeoutMS:  cfg.HTTPTimeoutMS,
		MaxRetries: cfg.HTTPRetries,
		Proxy:      proxy,
	})
	return &deps{
		cfg:      cfg,
		registry: sources.NewDefaultRegistry(cfg, proxy),
		client:   client,
		fetcher:  core.NewMetadataFetcher(client),
	}
}

// requireWorkDir 校验工作目录已设置；未设置时返回 errNoWorkDir（FR-LIB-03）。
func requireWorkDir(cfg *config.Config) error {
	if cfg.WorkDir == "" {
		return errNoWorkDir
	}
	return nil
}

// defaultLang 返回命令 --lang 的默认值（LITKIT_LANG；为空时回落 zh）。
// config.normalize 已保证 Load 后的值合法；手动构造的零值 Config 也可安全使用。
func defaultLang(cfg *config.Config) string {
	if cfg != nil && cfg.Lang != "" {
		return cfg.Lang
	}
	return config.DefaultLang
}

// userStylesDir 返回用户 CSL 样式目录（WORK_DIR/styles）；未设置工作目录时为空。
func userStylesDir(cfg *config.Config) string {
	if cfg == nil || cfg.WorkDir == "" {
		return ""
	}
	return filepath.Join(cfg.WorkDir, "styles")
}

// readFileTrimBOM 读取文本文件并去除 UTF-8 BOM（Windows 记事本 / PowerShell 5 常见）。
// 所有面向用户的文本输入统一走此函数，避免 BOM 泄漏到产物或解析失败。
func readFileTrimBOM(path string) ([]byte, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return bytes.TrimPrefix(data, []byte{0xEF, 0xBB, 0xBF}), nil
}

// readInputTrimBOM 读取 path（"-" 表示 stdin）并去除 UTF-8 BOM。
func readInputTrimBOM(path string) ([]byte, error) {
	if path != "-" {
		return readFileTrimBOM(path)
	}
	data, err := io.ReadAll(os.Stdin)
	if err != nil {
		return nil, err
	}
	return bytes.TrimPrefix(data, []byte{0xEF, 0xBB, 0xBF}), nil
}
