// library.go 实现 `litkit lib` 本地文献库命令（FR-LIB-02/04/06）。
//
// 检索结果自动入库（FR-LIB-01）；cite_key 为 3 字母引用标识，是 AI 引用的唯一入口。
package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"litkit/internal/core"
	"litkit/internal/model"
	"litkit/internal/storage"
)

// libPapersOutput lib list/search 默认输出（FR-IFACE-04：精简视图）。
type libPapersOutput struct {
	Total  int                  `json:"total"`
	Papers []model.PaperSummary `json:"papers"`
}

// libPapersOutputFull lib list/search --full 输出（完整元数据）。
type libPapersOutputFull struct {
	Total  int           `json:"total"`
	Papers []model.Paper `json:"papers"`
}

// errNoStore 文献库不可用时 lib 子命令的公共错误。
// 原因可能是未设置 LITKIT_WORK_DIR 或库初始化失败。
var errNoStore = errors.New("本地文献库不可用：请确认已设置 LITKIT_WORK_DIR（可用 litkit init 初始化）")

// newLibraryCmd 构造 `litkit lib` 子命令。
func newLibraryCmd(st *storage.Store) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "lib",
		Short: "本地文献库管理（add | list | search | rm | stats | path）",
	}
	cmd.AddCommand(
		newLibAddCmd(st),
		newLibListCmd(st),
		newLibSearchCmd(st),
		newLibRmCmd(st),
		newLibStatsCmd(st),
		newLibPathCmd(st),
	)
	return cmd
}

// libAddResult 单篇手动录入结果。
type libAddResult struct {
	CiteKey  string `json:"citeKey"`
	Title    string `json:"title"`
	Inserted bool   `json:"inserted"` // true=首次入库；false=更新已有文献（citeKey 不变）
}

func newLibAddCmd(st *storage.Store) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "add <metadata.json>",
		Short: "手动录入文献元数据（标题+摘要必填，source=manual）",
		Long: `litkit lib add —— 手动录入文献

读取元数据 JSON 文件（单个对象或对象数组），校验后入库并分配 citeKey。
用于 AI 手动添加检索源覆盖不到（或无法在线获取）的文献。

必填字段：title、abstract（摘要工作流：入库文献必须携带摘要，AI 需手动撰写）。
可选字段：authors（字符串数组 ["张三"] 或对象数组 [{"family","given"}]）、
year、venue、doi、pmid、arxivId、url、docType、volume、number、pages、publisher、city。

同一 DOI（无 DOI 则按标题）重复录入时更新字段、返回原 citeKey（inserted=false）。
入库文献 source 标记为 manual，可用 lib list --source manual 或 lib stats 区分。`,
		Args: cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			if st == nil {
				return errNoStore
			}
			inputs, err := readManualPapers(args[0])
			if err != nil {
				return err
			}
			results := make([]libAddResult, 0, len(inputs))
			for _, in := range inputs {
				p, err := core.AddManualPaper(in)
				if err != nil {
					return &paramError{msg: err.Error()}
				}
				citeKey, inserted, err := st.UpsertPaper(p)
				if err != nil {
					return err
				}
				results = append(results, libAddResult{CiteKey: citeKey, Title: p.Title, Inserted: inserted})
			}
			return printJSON(struct {
				Added  int            `json:"added"`
				Papers []libAddResult `json:"papers"`
			}{Added: len(results), Papers: results})
		},
	}
	return cmd
}

// readManualPapers 读取元数据 JSON 文件（单对象或数组；容忍 UTF-8 BOM）。
func readManualPapers(path string) ([]core.ManualPaperInput, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("lib add: 读取 %s 失败: %w", path, err)
	}
	data = bytes.TrimPrefix(data, []byte{0xEF, 0xBB, 0xBF}) // 去除 UTF-8 BOM
	trimmed := strings.TrimSpace(string(data))
	if strings.HasPrefix(trimmed, "[") {
		var inputs []core.ManualPaperInput
		if err := json.Unmarshal(data, &inputs); err != nil {
			return nil, fmt.Errorf("lib add: 解析 %s 失败: %w", path, err)
		}
		return inputs, nil
	}
	var in core.ManualPaperInput
	if err := json.Unmarshal(data, &in); err != nil {
		return nil, fmt.Errorf("lib add: 解析 %s 失败: %w", path, err)
	}
	return []core.ManualPaperInput{in}, nil
}

func newLibListCmd(st *storage.Store) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "列出库内论文（默认按年份倒序，精简视图）",
		RunE: func(cmd *cobra.Command, _ []string) error {
			if st == nil {
				return errNoStore
			}
			source, _ := cmd.Flags().GetString("source")
			sortBy, _ := cmd.Flags().GetString("sort")
			limit, _ := cmd.Flags().GetInt("limit")
			offset, _ := cmd.Flags().GetInt("offset")
			full, _ := cmd.Flags().GetBool("full")
			papers, err := st.ListPapers(source, sortBy, limit, offset)
			if err != nil {
				return err
			}
			if full {
				return printJSON(libPapersOutputFull{Total: len(papers), Papers: papers})
			}
			return printJSON(libPapersOutput{Total: len(papers), Papers: model.SummarizePapers(papers)})
		},
	}
	cmd.Flags().String("source", "", "按来源过滤")
	cmd.Flags().String("sort", "year", "排序方式：year（年份倒序）| id（入库倒序）")
	cmd.Flags().Int("limit", 0, "最大条数（默认 100）")
	cmd.Flags().Int("offset", 0, "偏移量")
	cmd.Flags().Bool("full", false, "输出完整元数据（默认精简视图，FR-IFACE-04）")
	return cmd
}

func newLibSearchCmd(st *storage.Store) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "search <keyword>",
		Short: "本地库关键词检索（标题/作者/摘要，FR-LIB-04；默认精简视图）",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if st == nil {
				return errNoStore
			}
			limit, _ := cmd.Flags().GetInt("limit")
			offset, _ := cmd.Flags().GetInt("offset")
			full, _ := cmd.Flags().GetBool("full")
			papers, err := st.SearchLocal(args[0], limit, offset)
			if err != nil {
				return err
			}
			if full {
				return printJSON(libPapersOutputFull{Total: len(papers), Papers: papers})
			}
			return printJSON(libPapersOutput{Total: len(papers), Papers: model.SummarizePapers(papers)})
		},
	}
	cmd.Flags().Int("limit", 0, "最大条数（默认 50）")
	cmd.Flags().Int("offset", 0, "偏移量")
	cmd.Flags().Bool("full", false, "输出完整元数据（默认精简视图，FR-IFACE-04）")
	return cmd
}

func newLibRmCmd(st *storage.Store) *cobra.Command {
	return &cobra.Command{
		Use:     "rm <cite_key>",
		Aliases: []string{"forget"},
		Short:   "删除一篇论文及其引用标记",
		Args:    cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			if st == nil {
				return errNoStore
			}
			removed, err := st.Forget(args[0])
			if err != nil {
				return err
			}
			return printJSON(struct {
				CiteKey string `json:"citeKey"`
				Removed bool   `json:"removed"`
			}{CiteKey: args[0], Removed: removed})
		},
	}
}

func newLibStatsCmd(st *storage.Store) *cobra.Command {
	return &cobra.Command{
		Use:   "stats",
		Short: "文献库统计",
		RunE: func(_ *cobra.Command, _ []string) error {
			if st == nil {
				return errNoStore
			}
			s, err := st.Stats()
			if err != nil {
				return err
			}
			return printJSON(s)
		},
	}
}

func newLibPathCmd(st *storage.Store) *cobra.Command {
	return &cobra.Command{
		Use:   "path",
		Short: "打印文献库文件路径",
		RunE: func(_ *cobra.Command, _ []string) error {
			if st == nil {
				return errNoStore
			}
			return printJSON(struct {
				Path string `json:"path"`
			}{Path: st.Path()})
		},
	}
}

// printJSON 以缩进 JSON 输出任意值（FR-IFACE-01）。
func printJSON(v any) error {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}
