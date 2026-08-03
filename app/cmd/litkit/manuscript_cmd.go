// manuscript_cmd.go 实现 `litkit manuscript` 命令（FR-REF-08/09/11）。
//
// 解析手稿占位符 [@token]，产出 formatted.md + references.txt + refs.bib + refs.ris
// +（可选）formatted.docx（需 Pandoc，缺失优雅降级）。
package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"litkit/internal/config"
	"litkit/internal/core"
	"litkit/internal/model"
	"litkit/internal/storage"
)

// manuscriptOutput 默认输出（精简视图 + 产物清单，FR-IFACE-04）。
type manuscriptOutput struct {
	CitationMap map[string]string    `json:"citationMap"`
	Papers      []model.PaperSummary `json:"papers"`
	Unresolved  []string             `json:"unresolved"`
	Files       map[string]string    `json:"files"`
}

// 产物落盘权限（对齐仓库惯例：目录 0750 / 文件 0600，gosec G301/G306）。
const (
	outDirPerm  = 0o750
	outFilePerm = 0o600
)

// newManuscriptCmd 构造 `litkit manuscript <draft.md>`。
func newManuscriptCmd(st *storage.Store, f *core.MetadataFetcher, cfg *config.Config) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "manuscript <draft.md>",
		Short: "手稿流水线（解析 [@token] 占位符、生成引用与产物）",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := requireWorkDir(cfg); err != nil {
				return err
			}
			if st == nil {
				return errNoStore
			}
			lang, _ := cmd.Flags().GetString("lang")
			styleFlag, _ := cmd.Flags().GetString("style")
			docx, _ := cmd.Flags().GetBool("docx")
			outDir, _ := cmd.Flags().GetString("output-dir")

			style, err := resolveStyle(lang, styleFlag)
			if err != nil {
				return err
			}
			src, err := os.ReadFile(args[0])
			if err != nil {
				return fmt.Errorf("manuscript: 读取手稿失败: %w", err)
			}

			res, err := core.ProcessManuscript(context.Background(), st, f, string(src), style)
			if err != nil {
				return err
			}

			if outDir == "" {
				outDir = cfg.WorkDir
			}
			if err := os.MkdirAll(outDir, outDirPerm); err != nil {
				return fmt.Errorf("manuscript: 创建输出目录失败: %w", err)
			}
			base := strings.TrimSuffix(filepath.Base(args[0]), filepath.Ext(args[0]))
			files, err := writeManuscriptArtifacts(outDir, base, res, style, docx)
			if err != nil {
				return err
			}
			return printJSON(manuscriptOutput{
				CitationMap: res.CitationMap,
				Papers:      model.SummarizePapers(res.Papers),
				Unresolved:  res.Unresolved,
				Files:       files,
			})
		},
	}
	cmd.Flags().String("lang", "zh", "写作语言模式 zh|en")
	cmd.Flags().StringP("style", "s", "", "引用样式（zh: gb7714-2025；en: apa / ieee）")
	cmd.Flags().Bool("docx", false, "生成 Word（需 Pandoc）")
	cmd.Flags().StringP("output-dir", "o", "", "输出目录（默认 WORK_DIR）")
	return cmd
}

// resolveStyle 由 --style / --lang 解析引用样式；无效值返回参数错误（退出码 2）。
func resolveStyle(lang, styleFlag string) (core.Style, error) {
	if styleFlag != "" {
		s := core.Style(styleFlag)
		switch s {
		case core.StyleGB7714, core.StyleAPA, core.StyleIEEE:
			return s, nil
		}
		return "", &paramError{msg: fmt.Sprintf("未知引用样式 %q（支持 gb7714-2025|apa|ieee）", styleFlag)}
	}
	switch lang {
	case "zh":
		return core.StyleGB7714, nil
	case "en":
		return core.StyleAPA, nil
	default:
		return "", &paramError{msg: fmt.Sprintf("未知语言模式 %q（支持 zh|en）", lang)}
	}
}

// writeManuscriptArtifacts 落盘 formatted.md / references.txt / refs.bib / refs.ris
// +（可选）formatted.docx。返回 产物名 → 绝对路径 映射。
func writeManuscriptArtifacts(outDir, base string, res *core.ManuscriptResult, style core.Style, docx bool) (map[string]string, error) {
	files := make(map[string]string)
	write := func(name, content string) error {
		path := filepath.Join(outDir, name)
		if err := os.WriteFile(path, []byte(content), outFilePerm); err != nil {
			return fmt.Errorf("manuscript: 写入 %s 失败: %w", name, err)
		}
		files[name] = path
		return nil
	}

	if err := write("formatted.md", res.Text); err != nil {
		return nil, err
	}
	var refs strings.Builder
	for i, p := range res.Papers {
		if i > 0 {
			refs.WriteString("\n\n")
		}
		line, err := core.FormatReference(p, style, i+1)
		if err != nil {
			return nil, err
		}
		refs.WriteString(line)
	}
	if err := write("references.txt", refs.String()); err != nil {
		return nil, err
	}
	if err := write("refs.bib", core.BibTeXFromPapers(res.Papers)); err != nil {
		return nil, err
	}
	if err := write("refs.ris", core.RISFromPapers(res.Papers)); err != nil {
		return nil, err
	}

	if docx {
		formattedPath := files["formatted.md"]
		docxPath := filepath.Join(outDir, base+".docx")
		if _, err := exec.LookPath("pandoc"); err != nil {
			// FR-REF-11：Pandoc 缺失仅 docx 不可用，其余产物正常
			fmt.Fprintf(os.Stderr, "litkit: 未找到 pandoc，跳过 docx 生成（formatted.md 已就绪）\n")
		} else if err := runPandoc(formattedPath, docxPath); err != nil {
			fmt.Fprintf(os.Stderr, "litkit: pandoc 转换失败，跳过 docx: %v\n", err)
		} else {
			files["formatted.docx"] = docxPath
		}
	}
	return files, nil
}

// runPandoc 调用 pandoc 将 markdown 转为 docx。
func runPandoc(src, dst string) error {
	// #nosec G204 -- 可执行文件固定为 pandoc（LookPath 校验存在），参数为本地文件路径，无 shell 注入面。
	out, err := exec.Command("pandoc", src, "-o", dst).CombinedOutput()
	if err != nil {
		if len(out) > 0 {
			return fmt.Errorf("%v: %s", err, strings.TrimSpace(string(out)))
		}
		return err
	}
	return nil
}
