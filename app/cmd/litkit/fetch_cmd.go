// fetch_cmd.go 实现 `litkit fetch` 命令（FR-FETCH-01）。
//
// 按 cite_key（3 字母）或 DOI 取回论文全文：Unpaywall OA 优先 → Sci-Hub 兜底 →
// 下载 PDF 落盘 + 纯 Go 抽取全文并缓存入库（FR-FETCH-04/05）。
package main

import (
	"context"
	"errors"

	"github.com/spf13/cobra"

	"litkit/internal/core"
	"litkit/internal/storage"
)

// errNoFulltextFetcher 全文获取器不可用（无工作目录时）。
var errNoFulltextFetcher = errors.New("全文获取需要工作目录（LITKIT_WORK_DIR），且论文须先入库")

// newFetchCmd 构造 `litkit fetch <cite_key|doi>`。
func newFetchCmd(store *storage.Store, f *core.FulltextFetcher) *cobra.Command {
	return &cobra.Command{
		Use:   "fetch <cite_key|doi>",
		Short: "取回论文全文（Unpaywall OA → Sci-Hub 兜底，PDF 落盘 + 全文入库）",
		Long: `按 cite_key（3 字母）或 DOI 取回论文全文。

流程：Unpaywall 按 DOI 解析 OA PDF（需 LITKIT_UNPAYWALL_EMAIL）→ 未命中则
Sci-Hub 兜底（默认开启，LITKIT_SCI_HUB_URL 可配，失败静默）→ 下载 PDF 落盘
<WORK_DIR>/downloads/<citeKey>.pdf → 抽取全文缓存入库（papers.fulltext）。

输出 JSON：{ citeKey, pdfPath, fulltext, via }；via = cache | unpaywall | scihub。
库中已有全文缓存时直接返回（零网络）。`,
		Args: cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			if store == nil || f == nil {
				return errNoFulltextFetcher
			}
			res, err := f.Fetch(context.Background(), args[0])
			if err != nil {
				return err
			}
			return printJSON(res)
		},
	}
}
