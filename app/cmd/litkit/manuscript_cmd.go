// manuscript_cmd.go 实现 `litkit manuscript` 命令（FR-REF-08/09/11）。
//
// 解析手稿占位符 [@token]，产出 formatted.md + refs.bib + refs.ris
// +（可选）formatted.docx（需 Pandoc，缺失优雅降级）。
// --preview 额外产出独立的草稿/调试视图，不经过 CSL 引擎。
package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"litkit/internal/config"
	"litkit/internal/core"
	"litkit/internal/csl"
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

// 产物目录权限（对齐仓库惯例 0750，gosec G301）。
const outDirPerm = 0o750

// lookPath 可注入，便于测试 Pandoc 缺失时的降级路径。
var lookPath = exec.LookPath

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
				return noStoreError()
			}
			lang, _ := cmd.Flags().GetString("lang")
			styleFlag, _ := cmd.Flags().GetString("style")
			preview, _ := cmd.Flags().GetBool("preview")
			docx, _ := cmd.Flags().GetBool("docx")
			outDir, _ := cmd.Flags().GetString("output-dir")

			style, err := resolveStyle(cfg, lang, styleFlag)
			if err != nil {
				return err
			}
			src, err := readFileTrimBOM(args[0])
			if err != nil {
				return fmt.Errorf("manuscript: 读取手稿失败: %w", err)
			}

			resolved, err := core.ResolveManuscript(context.Background(), st, f, string(src))
			if err != nil {
				return err
			}
			res, err := core.FormatManuscript(resolved, style)
			if err != nil {
				return err
			}

			if outDir == "" {
				outDir = filepath.Join(cfg.WorkDir, "outputs")
			}
			if err := os.MkdirAll(outDir, outDirPerm); err != nil {
				return fmt.Errorf("manuscript: 创建输出目录失败: %w", err)
			}
			base := strings.TrimSuffix(filepath.Base(args[0]), filepath.Ext(args[0]))
			ts := core.ManuscriptStamp(time.Now())
			files, err := writeManuscriptArtifacts(outDir, base, ts, res, core.RefListHeading(lang), docx)
			if err != nil {
				return err
			}

			// --preview：额外产出 preview 版本 md，与正常 formatted.md 并存。
			// 正文用自描述标记（[@doi:{DOI} — 标题]），便于人工核查引用指向。
			if preview {
				pres := core.FormatPreview(resolved)
				p, err := core.WritePreviewOutput(outDir, base, ts, pres, core.RefListHeading(lang))
				if err != nil {
					return err
				}
				files["formatted.preview.md"] = p
			}

			return printJSON(manuscriptOutput{
				CitationMap: res.CitationMap,
				Papers:      model.SummarizePapers(res.Papers),
				Unresolved:  res.Unresolved,
				Files:       files,
			})
		},
	}
	cmd.Flags().String("lang", defaultLang(cfg), "写作语言模式 zh|en（默认 LITKIT_LANG）")
	cmd.Flags().StringP("style", "s", "", "引用样式：gb7714-2025|apa|ieee 或 .csl 路径（默认按 --lang）")
	cmd.Flags().Bool("preview", false, "额外生成草稿/调试视图 md：内联自描述标记 [@doi:{DOI} — 标题]/[@标题]")
	cmd.Flags().Bool("docx", false, "生成 Word（需 Pandoc）")
	cmd.Flags().StringP("output-dir", "o", "", "输出目录（默认 WORK_DIR/outputs）")
	return cmd
}

// resolveStyle 由 --style / --lang 解析引用样式；无效值返回参数错误（退出码 2）。
func resolveStyle(cfg *config.Config, lang, styleFlag string) (*csl.Style, error) {
	s, err := core.ResolveStyle(lang, styleFlag, userStylesDir(cfg))
	if err != nil {
		return nil, &paramError{msg: err.Error()}
	}
	return s, nil
}

// writeManuscriptArtifacts 落盘 {base}_{ts}.md（正文+文末引用列表）/ .bib / .ris
// +（可选）{base}_{ts}.docx（需 Pandoc，缺失优雅降级）。
func writeManuscriptArtifacts(outDir, base, ts string, res *core.ManuscriptResult, heading string, docx bool) (map[string]string, error) {
	files, err := core.WriteManuscriptOutputs(outDir, base, ts, res, heading)
	if err != nil {
		return nil, err
	}

	if docx {
		docxPath := filepath.Join(outDir, base+"_"+ts+".docx")
		if _, err := lookPath("pandoc"); err != nil {
			// FR-REF-11：Pandoc 缺失仅 docx 不可用，其余产物正常
			fmt.Fprintf(os.Stderr, "litkit: 未找到 pandoc，跳过 docx 生成（formatted.md 已就绪）。\n")
			fmt.Fprintf(os.Stderr, "litkit: 如需 docx，请安装 pandoc 后重试：https://pandoc.org/installing.html\n")
		} else if err := core.PandocToDocx(files[core.ManuscriptFormatted], docxPath); err != nil {
			fmt.Fprintf(os.Stderr, "litkit: pandoc 转换失败，跳过 docx: %v\n", err)
		} else {
			files["formatted.docx"] = docxPath
		}
	}
	return files, nil
}
