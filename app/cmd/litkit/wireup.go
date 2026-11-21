// wireup.go 在 CLI 入口层组装依赖：config → registry/store/searcher。
//
// 入口层负责依赖注入（boundaries.md）；业务逻辑全部下沉 internal/core。
package main

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"litkit/internal/config"
	"litkit/internal/core"
	"litkit/internal/sources"
	"litkit/internal/storage"
	"litkit/internal/util/httpclient"
)

// errNoWorkDir 未设置 LITKIT_WORK_DIR 的公共错误。
// 工作目录必须显式设置：所有需要文献库的命令（init/search/lib）拒绝执行，
// 避免生成物污染任意 CWD（FR-LIB-03）。
var errNoWorkDir = errors.New("未设置 LITKIT_WORK_DIR：请在项目目录运行 litkit init 生成项目 .env，或设置环境变量（如 $env:LITKIT_WORK_DIR = \"<dir>\"）")

// deps 持有一次 CLI 调用所需的全部依赖。
type deps struct {
	cfg      *config.Config
	registry *sources.Registry
	store    *storage.Store
	searcher *core.Searcher
	fetcher  *core.MetadataFetcher // 标识符反查（metadata/manuscript 用）
	fulltext *core.FulltextFetcher // 全文获取（fetch 用，FR-FETCH）
}

// Close 释放依赖持有的资源（文献库连接）。
func (d *deps) Close() {
	if d.store != nil {
		_ = d.store.Close()
	}
}

// loadDeps 加载配置并组装依赖。
// 配置加载失败时退化为默认值并打印告警，不阻断 CLI（优雅降级）。
// 未设置 LITKIT_WORK_DIR 时 store 为空：init/search/lib 在命令层拒绝（errNoWorkDir）。
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
	reg := sources.NewDefaultRegistry(cfg, proxy)

	store := (*storage.Store)(nil)
	if cfg.WorkDir != "" {
		store, err = storage.Open(storage.DBPath(cfg.WorkDir))
		if err != nil {
			// 库初始化失败不阻断检索：退化为不入库（检索仍可用）
			fmt.Fprintf(os.Stderr, "litkit: storage init failed, search 将不入库: %v\n", err)
			store = nil
		}
	}
	// 全文获取（FR-FETCH）：PDF 默认落盘 <WorkDir>/downloads，可由 LITKIT_FETCH_DOWNLOAD_DIR 覆盖
	fulltext := (*core.FulltextFetcher)(nil)
	if store != nil {
		downloadDir := cfg.DownloadDir()
		fulltext = core.NewFulltextFetcher(
			store,
			httpclient.New(httpclient.Options{TimeoutMS: cfg.HTTPTimeoutMS, MaxRetries: cfg.HTTPRetries, Proxy: proxy}),
			cfg.UnpaywallEmail,
			cfg.SciHubURL,
			downloadDir,
		)
	}
	return &deps{
		cfg:      cfg,
		registry: reg,
		store:    store,
		searcher: core.NewSearcher(reg, store, cfg.DefaultMaxResults, cfg.SearchTimeoutMS),
		fetcher: core.NewMetadataFetcher(httpclient.New(httpclient.Options{
			TimeoutMS:  cfg.HTTPTimeoutMS,
			MaxRetries: cfg.HTTPRetries,
			Proxy:      proxy,
		})),
		fulltext: fulltext,
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
