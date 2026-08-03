// export_cmd.go 实现 `litkit export` 命令（FR-REF-10）。
//
// 读取 papers.json（数组或 {"papers":[...]}），按格式导出：
// bibtex | ris | text（text 需 --style 指定引用样式）。
package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"strings"

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

// newExportCmd 构造 `litkit export <papers.json>`。
func newExportCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "export <papers.json>",
		Short: "批量导出引用（bibtex | ris | text）",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			format, _ := cmd.Flags().GetString("format")
			styleFlag, _ := cmd.Flags().GetString("style")
			switch format {
			case "bibtex", "ris", "text":
			default:
				return &paramError{msg: fmt.Sprintf("未知导出格式 %q（支持 bibtex|ris|text）", format)}
			}

			papers, err := readPapersFile(args[0])
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
				style, err := resolveStyle("en", styleFlag)
				if err != nil {
					return err
				}
				var b strings.Builder
				for i, p := range papers {
					if i > 0 {
						b.WriteString("\n\n")
					}
					line, err := core.FormatReference(p, style, i+1)
					if err != nil {
						return err
					}
					b.WriteString(line)
				}
				content = b.String()
			}
			return printJSON(exportOutput{Format: format, Count: len(papers), Content: content})
		},
	}
	cmd.Flags().StringP("format", "f", "bibtex", "导出格式 bibtex|ris|text")
	cmd.Flags().StringP("style", "s", "", "引用样式（text 格式用）")
	return cmd
}

// readPapersFile 读取 papers.json：兼容裸数组与 {"papers": [...]} 两种形态。
// 兼容 Windows 工具常见的 UTF-8 BOM 前缀。
func readPapersFile(path string) ([]model.Paper, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("export: 读取 %s 失败: %w", path, err)
	}
	data = bytes.TrimPrefix(data, []byte("\xEF\xBB\xBF"))
	var papers []model.Paper
	if err := json.Unmarshal(data, &papers); err == nil {
		return papers, nil
	}
	var wrapper struct {
		Papers []model.Paper `json:"papers"`
	}
	if err := json.Unmarshal(data, &wrapper); err != nil {
		return nil, fmt.Errorf("export: 解析 %s 失败（期望 []Paper 或 {\"papers\":[...]}）: %w", path, err)
	}
	return wrapper.Papers, nil
}
