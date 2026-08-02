// wireup.go 在 CLI 入口层组装依赖：config → registry/store/searcher。
//
// 入口层负责依赖注入（boundaries.md）；业务逻辑全部下沉 internal/core。
// CLI 与 MCP（M5）将共享同一份 wireup，保证接口一致性（FR-IFACE-03）。
package main

import (
	"fmt"
	"os"
	"path/filepath"

	"litkit/internal/config"
	"litkit/internal/core"
	"litkit/internal/sources"
	"litkit/internal/storage"
)

// deps 持有一次 CLI 调用所需的全部依赖。
type deps struct {
	cfg      *config.Config
	registry *sources.Registry
	store    *storage.Store
	searcher *core.Searcher
}

// loadDeps 加载配置并组装依赖。
// 配置加载失败时退化为默认值并打印告警，不阻断 CLI（优雅降级）。
func loadDeps() *deps {
	cfg, err := config.Load()
	if err != nil {
		// 配置加载失败不阻断：用默认值继续，让 CLI 仍可用
		cfg = &config.Config{
			Lang:              config.DefaultLang,
			HTTPTimeoutMS:     config.DefaultHTTPTimeoutMS,
			HTTPRetries:       config.DefaultHTTPRetries,
			DefaultMaxResults: config.DefaultMaxResults,
		}
		fmt.Fprintf(os.Stderr, "litkit: config load failed, using defaults: %v\n", err)
	}
	reg := sources.NewDefaultRegistry(cfg)
	store, err := storage.Open(dbPath(cfg))
	if err != nil {
		// 库初始化失败不阻断检索：退化为不入库（检索仍可用）
		fmt.Fprintf(os.Stderr, "litkit: storage init failed, search 将不入库: %v\n", err)
		store = nil
	}
	return &deps{
		cfg:      cfg,
		registry: reg,
		store:    store,
		searcher: core.NewSearcher(reg, store, cfg.DefaultMaxResults),
	}
}

// dbPath 解析文献库路径（FR-LIB-03）。
// 优先 WORK_DIR/litkit.db；WORK_DIR 未设置时退化为 CWD/litkit.db。
func dbPath(cfg *config.Config) string {
	base := cfg.WorkDir
	if base == "" {
		base, _ = os.Getwd()
	}
	return filepath.Join(base, storage.DefaultDBName)
}
