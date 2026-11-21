// export_cmd.go 实现 `litkit export` 命令（FR-REF-10）。
//
// 读取 papers.json（数组或 {"papers":[...]}），按格式导出：
// bibtex | ris | text（text 需 --style 指定引用样式，CSL 渲染）。
package main

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"litkit/internal/core"
	"litkit/internal/model"
)

// exportOutput 导出结果。
type exportOutput struct {
	Format  string `json:"format"`
	Count   int    `json:"count"`
	Content string `json:"content"`
}

// newExportCmd 构造 `litkit export [papers.json | -] [--keys | --all]`。
func newExportCmd(d *deps) *cobra.Command {
	cfg := d.cfg
	cmd := &cobra.Command{
		Use:   "export [papers.json | -] [--keys K1,K2 | --all] [-f bibtex|ris|text] [-s style]",
		Short: "批量导出引用（bibtex | ris | text）",
		Long: `批量导出引用。数据源三选一：

  <papers.json | ->   读取 []Paper 或 {"papers":[...]}；传 - 从 stdin 读取
  --keys K1,K2         直接从本地文献库按 citeKey 取（免中间文件）
  --all                导出本地文献库全部条目

text 格式由 CSL 引擎渲染（默认 APA），bibtex/ris 为结构化导出。`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			format, _ := cmd.Flags().GetString("format")
			styleFlag, _ := cmd.Flags().GetString("style")
			keysFlag, _ := cmd.Flags().GetString("keys")
			allFlag, _ := cmd.Flags().GetBool("all")
			switch format {
			case "bibtex", "ris", "text":
			default:
				return &paramError{msg: fmt.Sprintf("未知导出格式 %q（支持 bibtex|ris|text）", format)}
			}

			papers, err := exportPapers(d, args, keysFlag, allFlag)
			if err != nil {
				return err
			}

			var content string
			switch format {
			case "bibtex":
				content = core.BibTeXFromPapers(papers)
			case "ris":
				content = core.RISFromPapers(papers)
			case "text":
				// text 需要引用样式；缺省回落 APA（api.md：-s 可选）
				style, err := core.ResolveStyle("en", styleFlag, userStylesDir(cfg))
				if err != nil {
					return &paramError{msg: err.Error()}
				}
				content, err = core.RenderReferenceList(papers, style)
				if err != nil {
					return err
				}
			}
			return printJSON(exportOutput{Format: format, Count: len(papers), Content: content})
		},
	}
	cmd.Flags().StringP("format", "f", "bibtex", "导出格式 bibtex|ris|text")
	cmd.Flags().StringP("style", "s", "", "引用样式（text 格式用）：gb7714-2025|apa|ieee 或 .csl 路径")
	cmd.Flags().String("keys", "", "从本地文献库按 citeKey 导出（逗号分隔）")
	cmd.Flags().Bool("all", false, "从本地文献库导出全部条目")
	return cmd
}

// exportPapers 解析导出数据源：库内 citeKey / 全库 / JSON 文件（或 stdin）。
// 三者互斥；未指定任一来源返回参数错误（退出码 2）。
func exportPapers(d *deps, args []string, keysFlag string, allFlag bool) ([]model.Paper, error) {
	keys := splitCSV(keysFlag)
	if allFlag || len(keys) > 0 {
		if len(args) > 0 {
			return nil, &paramError{msg: "export: --keys/--all 不接受位置参数"}
		}
		if allFlag && len(keys) > 0 {
			return nil, &paramError{msg: "export: --keys 与 --all 只能指定一个"}
		}
		st := d.Store()
		if st == nil {
			return nil, noStoreError()
		}
		if allFlag {
			return st.AllPapers()
		}
		found, miss, err := st.GetByCiteKeys(keys)
		if err != nil {
			return nil, err
		}
		if len(miss) > 0 {
			fmt.Fprintf(os.Stderr, "litkit: export: 库中未找到 citeKey %v（已跳过）\n", miss)
		}
		return found, nil
	}
	if len(args) != 1 {
		return nil, &paramError{msg: "export 需要 <papers.json | -> 或 --keys/--all"}
	}
	return readPapersFile(args[0])
}

// readPapersFile 读取 papers.json：兼容裸数组与 {"papers": [...]} 两种形态。
// 兼容 Windows 工具常见的 UTF-8 BOM 前缀。
func readPapersFile(path string) ([]model.Paper, error) {
	data, err := readInputTrimBOM(path)
	if err != nil {
		return nil, fmt.Errorf("export: 读取 %s 失败: %w", path, err)
	}
	var papers []model.Paper
	if err := json.Unmarshal(data, &papers); err == nil {
		return dedupPapers(papers), nil
	}
	var wrapper struct {
		Papers []model.Paper `json:"papers"`
	}
	if err := json.Unmarshal(data, &wrapper); err != nil {
		return nil, fmt.Errorf("export: 解析 %s 失败（期望 []Paper 或 {\"papers\":[...]}）: %w", path, err)
	}
	// 单个 Paper 对象也能反序列化成空 wrapper（未知字段被忽略）；
	// nil 说明输入里没有 papers 数组，直接报错而不是静默导出 0 条。
	if wrapper.Papers == nil {
		return nil, fmt.Errorf("export: 解析 %s 失败（期望 []Paper 或 {\"papers\":[...]}，单个对象请用数组包裹）", path)
	}
	return dedupPapers(wrapper.Papers), nil
}

// dedupPapers 按 citeKey 去重（保留首次出现），空 citeKey 不参与去重，
// 避免相同文献在导出中出现重复 BibTeX 键。
func dedupPapers(ps []model.Paper) []model.Paper {
	seen := make(map[string]bool, len(ps))
	out := make([]model.Paper, 0, len(ps))
	for _, p := range ps {
		if p.CiteKey == "" || !seen[p.CiteKey] {
			if p.CiteKey != "" {
				seen[p.CiteKey] = true
			}
			out = append(out, p)
		}
	}
	return out
}
