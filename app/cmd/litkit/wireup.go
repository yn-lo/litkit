// wireup.go 在 CLI 入口层组装依赖：config → registry/store/searcher。
//
// 入口层负责依赖注入（boundaries.md）；业务逻辑全部下沉 internal/core。
// CLI 与 MCP（M5）将共享同一份 wireup，保证接口一致性（FR-IFACE-03）。
package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"litkit/internal/config"
	"litkit/internal/core"
	"litkit/internal/sources"
	"litkit/internal/storage"
)

// errNoWorkDir 未设置 LITKIT_WORK_DIR 的公共错误。
// 工作目录必须显式设置：所有需要文献库的命令（init/search/lib）拒绝执行，
// 避免生成物污染任意 CWD（FR-LIB-03）。
var errNoWorkDir = errors.New("未设置 LITKIT_WORK_DIR：请先设置工作目录（如 $env:LITKIT_WORK_DIR = \"<dir>\"）")

// deps 持有一次 CLI 调用所需的全部依赖。
type deps struct {
	cfg      *config.Config
	registry *sources.Registry
	store    *storage.Store
	searcher *core.Searcher
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
	reg := sources.NewDefaultRegistry(cfg)

	store := (*storage.Store)(nil)
	if cfg.WorkDir != "" {
		store, err = storage.Open(filepath.Join(cfg.WorkDir, storage.DefaultDBName))
		if err != nil {
			// 库初始化失败不阻断检索：退化为不入库（检索仍可用）
			fmt.Fprintf(os.Stderr, "litkit: storage init failed, search 将不入库: %v\n", err)
			store = nil
		}
	}
	return &deps{
		cfg:      cfg,
		registry: reg,
		store:    store,
		searcher: core.NewSearcher(reg, store, cfg.DefaultMaxResults),
	}
}

// requireWorkDir 校验工作目录已设置；未设置时返回 errNoWorkDir（FR-LIB-03）。
func requireWorkDir(cfg *config.Config) error {
	if cfg.WorkDir == "" {
		return errNoWorkDir
	}
	return nil
}
