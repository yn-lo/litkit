package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"litkit/internal/config"
	"litkit/internal/core"
	"litkit/internal/lint"
	"litkit/internal/model"
	"litkit/internal/storage"
)

// newVerifyCmd 构造 verify 子命令（FR-LINT-05 事后验证；R5.6 引用防伪查库；FR-LINT-08 引用评分）。
//
// 退出码（verify 专用语义）：
//   - 0 全部通过（exitHint=pass）或仅 S 类需人工（exitHint=manual_review）
//   - 1 有 A 类违规（exitHint=fix_and_rerun），AI 应自动修复后重跑
func newVerifyCmd(cfg *config.Config, store *storage.Store) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "verify <file.md> [file2.md ...] --type review|empirical --lang zh|en",
		Short: "验证文稿合规性（A/S 类规则自动检查）",
		Long: `litkit verify —— 事后合规验证

对 Markdown 文稿执行规则检查，输出 JSON 报告。
规则按模式递增启用：chapter（结构）→ draft（+数据/标点/引用）→ final（+字数/行文）。

使用 --check 可仅运行指定检查类别（逗号分隔），如 --check citation,word_counts。
可用类别：language, structure, statistics, punctuation, style, citation, heading, boast_words, word_counts, todo

使用 --report citation-refs 可额外输出引用相关性评分（需 LITKIT_VERIFY_LINT_LLM=true）。

退出码：0=通过或仅需人工复核；1=有 A 类违规需修复。
AI 应读取 JSON 中 exitHint 字段决定下一步动作。`,
		Args: cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := requireWorkDir(cfg); err != nil {
				return err
			}
			lang, _ := cmd.Flags().GetString("lang")
			modeStr, _ := cmd.Flags().GetString("mode")
			paperType, _ := cmd.Flags().GetString("type")
			ruleFlag, _ := cmd.Flags().GetString("rule")
			skipFlag, _ := cmd.Flags().GetString("skip")
			checkFlag, _ := cmd.Flags().GetString("check")
			skipCheckFlag, _ := cmd.Flags().GetString("skip-check")
			reportFlag, _ := cmd.Flags().GetString("report")

			mode := lint.Mode(modeStr)
			switch mode {
			case lint.ModeChapter, lint.ModeDraft, lint.ModeFinal:
			default:
				return &paramError{msg: fmt.Sprintf("verify: 无效 mode %q（可选 chapter|draft|final）", modeStr)}
			}
			if lang != "zh" && lang != "en" {
				return &paramError{msg: fmt.Sprintf("verify: 无效 lang %q（可选 zh|en）", lang)}
			}
			if paperType != "" && paperType != lint.PaperTypeReview && paperType != lint.PaperTypeEmpirical {
				return &paramError{msg: fmt.Sprintf("verify: 无效 type %q（可选 review|empirical）", paperType)}
			}

			// 加载阈值配置
			spec := loadVerifySpec(cfg.WorkDir, paperType, lang)

			onlyCats, err := parseCategories(checkFlag)
			if err != nil {
				return &paramError{msg: fmt.Sprintf("verify: %v", err)}
			}
			skipCats, err := parseCategories(skipCheckFlag)
			if err != nil {
				return &paramError{msg: fmt.Sprintf("verify: %v", err)}
			}

			opts := lint.Options{
				Lang:           lang,
				Mode:           mode,
				PaperType:      paperType,
				Only:           splitCSV(ruleFlag),
				Skip:           splitCSV(skipFlag),
				OnlyCategories: onlyCats,
				SkipCategories: skipCats,
			}

			report, err := lint.RunFilesWithStore(args, spec, opts, store)
			if err != nil {
				return fmt.Errorf("verify: %w", err)
			}

			// 引用评分（FR-LINT-08）
			if strings.Contains(reportFlag, "citation-refs") && store != nil {
				cr := runCitationRelevance(args, store, cfg)
				report.CitationRefs = cr
			}

			out, _ := json.MarshalIndent(report, "", "  ")
			fmt.Println(string(out))

			if report.ExitHint == "fix_and_rerun" {
				os.Exit(1)
			}
			return nil
		},
	}
	cmd.Flags().String("lang", "zh", "写作语言 zh|en")
	cmd.Flags().String("mode", "draft", "验证模式 chapter|draft|final（递增启用规则）")
	cmd.Flags().String("type", "", "论文类型 review|empirical（空=从已有 spec 自动检测）")
	cmd.Flags().String("rule", "", "仅运行指定规则（逗号分隔，如 R2.1,R7.1）")
	cmd.Flags().String("skip", "", "跳过指定规则（逗号分隔）")
	cmd.Flags().String("check", "", "仅运行指定检查类别（逗号分隔，如 citation,word_counts）")
	cmd.Flags().String("skip-check", "", "跳过指定检查类别（逗号分隔）")
	cmd.Flags().String("report", "json", "报告格式：json（默认）| citation-refs（引用评分）")
	return cmd
}

// runCitationRelevance 对文稿执行引用相关性评分（FR-LINT-08）。
//
// 在规则验证后额外执行，不产生 lint Violation，以独立报告形式输出。
func runCitationRelevance(paths []string, store *storage.Store, cfg *config.Config) *lint.CitationRelevanceReport {
	cr := &lint.CitationRelevanceReport{Enabled: false}

	if !cfg.VerifyLLMEnabled {
		return cr
	}

	// 加载 verifier_models.json
	vmPath := filepath.Join(cfg.WorkDir, ".litkit", "verifier_models.json")
	vm, err := core.LoadVerifierModels(vmPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "litkit: 加载 verifier_models.json 失败: %v（跳过引用评分）\n", err)
		return cr
	}

	timeout := time.Duration(cfg.LLMTimeoutMS) * time.Millisecond
	engine := core.NewScorerEngine(store, vm, cfg.LLMAPIKey, cfg.LLMBaseURL, timeout, cfg.VerifyLLMEnabled)
	if engine.IsDisabled() {
		return cr
	}

	cr.Enabled = true
	cr.Models = engine.EnabledModels()
	ctx := context.Background()

	for _, p := range paths {
		src, err := lint.ParseSource(p)
		if err != nil {
			fmt.Fprintf(os.Stderr, "litkit: 解析 %s 失败: %v（跳过引用评分）\n", p, err)
			continue
		}

		// 提取引用句
		body := src.BodyContent()
		lineNos := src.BodyLineNumbers()
		refs := core.ExtractCiteSentences(body)
		if len(refs) == 0 {
			continue
		}

		// 重建 paper_refs（先删旧引用，再插新）
		rel := relativePath(p, cfg.WorkDir)
		_ = store.RemoveRefsByManuscript(rel)
		refModels := make([]model.PaperRef, len(refs))
		for i, r := range refs {
			// 映射到原始行号
			origLine := r.Line
			if r.Line > 0 && r.Line <= len(lineNos) {
				origLine = lineNos[r.Line-1]
			}
			refModels[i] = model.PaperRef{
				CiteKey:      r.CiteKey,
				SentenceHash: r.SentenceHash,
				Manuscript:   rel,
				Sentence:     r.Sentence,
				Line:         origLine,
			}
		}
		_ = store.AddRefs(refModels)

		// 逐个评分
		for _, r := range refs {
			paper, err := store.GetByCiteKey(r.CiteKey)
			if err != nil || paper == nil || paper.Abstract == "" {
				continue
			}
			result, err := engine.Score(ctx, r.CiteKey, r.SentenceHash, r.Sentence, paper.Abstract)
			if err != nil || result == nil {
				continue
			}
			// 映射到原始行号
			origLine := r.Line
			if r.Line > 0 && r.Line <= len(lineNos) {
				origLine = lineNos[r.Line-1]
			}
			cr.Results = append(cr.Results, lint.CitationRelevanceItem{
				File:         rel,
				Line:         origLine,
				CiteKey:      r.CiteKey,
				Sentence:     truncate(r.Sentence, 80), //nolint: mnd
				MeanScore:    result.MeanScore,
				Consensus:    result.Consensus,
				Cached:       result.Cached,
				LowScore:     result.MeanScore < 0.3, //nolint: mnd
				LowConsensus: result.Consensus < 0.5, //nolint: mnd
			})
		}
	}
	return cr
}

// relativePath 将绝对路径转为相对工作目录的路径。
func relativePath(path, workDir string) string {
	if workDir == "" {
		return path
	}
	rel, err := filepath.Rel(workDir, path)
	if err != nil {
		return path
	}
	return rel
}

// truncate 截断字符串到指定最大长度（中文按一个字符计）。
func truncate(s string, max int) string {
	runes := []rune(s)
	if len(runes) <= max {
		return s
	}
	return string(runes[:max]) + "..."
}

// loadVerifySpec 从工作目录加载 manuscript-spec.yaml。
// paperType/lang 为空时自动检测（仅一个类型时使用）。
func loadVerifySpec(workDir, paperType, lang string) *lint.ManuscriptSpec {
	if paperType == "" || lang == "" {
		types, err := lint.ListPaperTypes(workDir)
		if err == nil && len(types) == 1 {
			p := lint.TypeSpecPath(workDir, types[0])
			spec, err := lint.LoadSpec(p)
			if err == nil {
				return spec
			}
		}
		fmt.Fprintln(os.Stderr, "litkit: 未指定 --type/--lang 且无法自动检测，使用默认阈值")
		return lint.DefaultSpec()
	}
	p := lint.SpecPath(workDir, paperType, lang)
	spec, err := lint.LoadSpec(p)
	if err != nil {
		fmt.Fprintf(os.Stderr, "litkit: 未找到 %s，使用默认阈值（可 litkit init 生成）\n", p)
		return lint.DefaultSpec()
	}
	return spec
}

// splitCSV 将逗号分隔字符串拆为切片（空串返回 nil）。
func splitCSV(s string) []string {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

// parseCategories 将逗号分隔的类别字符串解析为 []lint.Category，校验合法性。
func parseCategories(s string) ([]lint.Category, error) {
	parts := splitCSV(s)
	if len(parts) == 0 {
		return nil, nil
	}
	valid := map[string]bool{}
	for _, c := range lint.ValidCategories() {
		valid[string(c)] = true
	}
	out := make([]lint.Category, 0, len(parts))
	for _, p := range parts {
		if !valid[p] {
			return nil, fmt.Errorf("无效检查类别 %q（可选 %s）", p, joinCategoryNames())
		}
		out = append(out, lint.Category(p))
	}
	return out, nil
}

// joinCategoryNames 拼接全部合法类别名（用于错误提示）。
func joinCategoryNames() string {
	cats := lint.ValidCategories()
	names := make([]string, len(cats))
	for i, c := range cats {
		names[i] = string(c)
	}
	return strings.Join(names, ",")
}
