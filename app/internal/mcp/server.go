// Package mcp 实现 litkit 的 MCP Server（可选第二接口，FR-IFACE-03）。
//
// 传输：stdio。全部工具直接调用 core/lint/storage 共享核心，与 CLI 同输入同输出，
// 保证接口一致（C8）。新增 CLI 功能须在此同步注册工具（FR-IFACE-03）。
package mcp

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	gomcp "github.com/modelcontextprotocol/go-sdk/mcp"

	"litkit/internal/buildinfo"
	"litkit/internal/config"
	"litkit/internal/core"
	"litkit/internal/lint"
	"litkit/internal/model"
	"litkit/internal/sources"
	"litkit/internal/storage"
)

// Deps MCP 服务器依赖（入口层注入；与 CLI 共用核心实例，FR-IFACE-03）。
type Deps struct {
	Cfg      *config.Config
	Registry *sources.Registry
	Store    *storage.Store
	Searcher *core.Searcher
	Fetcher  core.Fetcher
	Fulltext *core.FulltextFetcher // 全文获取（fetch_paper，FR-FETCH）
}

// errNoStore 文献库不可用时 lib/manuscript 类工具的公共错误。
var errNoStore = errors.New("本地文献库不可用：请确认已设置 LITKIT_WORK_DIR（可用 lint_init 初始化）")

// 产物目录/文件权限（与 lint/init 层一致，C6）。
const (
	outputDirPerm  = 0o750
	agentsFilePerm = 0o600
)

// Run 在 stdio 传输上运行服务器，直至客户端断开或 ctx 取消。
func Run(ctx context.Context, deps Deps) error {
	s := New(deps)
	return s.Run(ctx, &gomcp.StdioTransport{})
}

// New 构造 MCP 服务器并注册全部工具（工具清单与 api.md §2 一致）。
func New(deps Deps) *gomcp.Server {
	s := gomcp.NewServer(&gomcp.Implementation{
		Name:        "litkit",
		Version:     buildinfo.Version,
		Description: "国内学术写作场景的论文工具包：跨源检索/规范引用/排版手稿/撰写合规门禁",
	}, nil)

	registerSearchPapers(s, deps)
	registerGetPaperMetadata(s, deps)
	registerFetchPaper(s, deps)
	registerProcessManuscript(s, deps)
	registerExportReferences(s, deps)
	registerLintInit(s, deps)
	registerVerifyManuscript(s, deps)
	registerLibraryTools(s, deps)
	if deps.Registry != nil {
		for _, src := range deps.Registry.List() {
			registerSourceTool(s, deps, src.Name())
		}
	}
	return s
}

// ---- search_papers ----

type searchPapersIn struct {
	Query               string   `json:"query" jsonschema:"检索关键词（英文命中率更高，FR-SEARCH-11）"`
	Sources             []string `json:"sources,omitempty" jsonschema:"源名过滤（如 arxiv,pubmed；空表示全部源）"`
	MaxResultsPerSource int      `json:"maxResultsPerSource,omitempty" jsonschema:"每源最大条数（默认 5）"`
	Year                int      `json:"year,omitempty" jsonschema:"年份过滤（精确匹配，0 表示不过滤）"`
	Since               int      `json:"since,omitempty" jsonschema:"起始年份（含）；与 year 互斥，year 优先"`
	Mode                string   `json:"mode,omitempty" jsonschema:"检索等级 tiab|full（默认 tiab）"`
	KeepNoAbstract      bool     `json:"keepNoAbstract,omitempty" jsonschema:"保留无摘要论文（默认过滤，FR-SEARCH-03）"`
	Exclude             []string `json:"exclude,omitempty" jsonschema:"排除词：标题或摘要命中任一排除词即剔除（本地召回后筛查，先排除后入库）"`
	Full                bool     `json:"full,omitempty" jsonschema:"返回完整元数据（默认精简视图 PaperSummary，FR-IFACE-04）"`
}

// searchPapersOut 默认输出（PaperSummary 精简视图，FR-IFACE-04）。
type searchPapersOut struct {
	Total         int                  `json:"total"`
	SourceResults map[string]int       `json:"sourceResults"`
	Errors        []model.SourceError  `json:"errors,omitempty"`
	Papers        []model.PaperSummary `json:"papers"`
}

// searchPapersFullOut --full 模式输出（完整元数据）。
type searchPapersFullOut struct {
	Total         int                      `json:"total"`
	SourceResults map[string][]model.Paper `json:"sourceResults"`
	Errors        []model.SourceError      `json:"errors,omitempty"`
	Papers        []model.Paper            `json:"papers"`
}

func registerSearchPapers(s *gomcp.Server, deps Deps) {
	gomcp.AddTool[searchPapersIn, any](s, &gomcp.Tool{
		Name:        "search_papers",
		Description: "跨源并发检索文献（摘要工作流），自动去重并入库。默认返回 PaperSummary 精简视图（FR-IFACE-04）",
	}, func(ctx context.Context, _ *gomcp.CallToolRequest, in searchPapersIn) (*gomcp.CallToolResult, any, error) {
		res, err := deps.Searcher.Search(ctx, in.Query, core.SearchOptions{
			Sources:        in.Sources,
			MaxResults:     in.MaxResultsPerSource,
			Year:           in.Year,
			Since:          in.Since,
			Mode:           in.Mode,
			KeepNoAbstract: in.KeepNoAbstract,
			Exclude:        in.Exclude,
		})
		if err != nil {
			return nil, nil, err
		}
		if in.Full {
			out := searchPapersFullOut{
				Total:         res.Total,
				SourceResults: res.SourceResults,
				Errors:        res.Errors,
				Papers:        res.Papers,
			}
			return nil, out, nil
		}
		// 默认：精简视图 + sourceResults 降为条数 + errors 精简（与 CLI 一致，FR-IFACE-04）
		srcCounts := make(map[string]int, len(res.SourceResults))
		for src, ps := range res.SourceResults {
			srcCounts[src] = len(ps)
		}
		out := searchPapersOut{
			Total:         res.Total,
			SourceResults: srcCounts,
			Errors:        shortErrors(res.Errors),
			Papers:        model.SummarizePapers(res.Papers),
		}
		return nil, out, nil
	})
}

// ---- get_paper_metadata ----

type getPaperMetadataIn struct {
	IDType     string `json:"idType" jsonschema:"标识符类型 doi|pmid|arxiv|title"`
	Identifier string `json:"identifier" jsonschema:"标识符值（如 DOI / PMID / arXiv ID / 题名）"`
}

func registerGetPaperMetadata(s *gomcp.Server, deps Deps) {
	gomcp.AddTool[getPaperMetadataIn, any](s, &gomcp.Tool{
		Name:        "get_paper_metadata",
		Description: "按标识符反查论文完整元数据；未命中返回 null",
	}, func(ctx context.Context, _ *gomcp.CallToolRequest, in getPaperMetadataIn) (*gomcp.CallToolResult, any, error) {
		p, err := deps.Fetcher.Fetch(ctx, in.IDType, in.Identifier)
		if err != nil {
			return nil, nil, err
		}
		if p == nil {
			return &gomcp.CallToolResult{Content: []gomcp.Content{&gomcp.TextContent{Text: "null"}}}, nil, nil
		}
		return nil, p, nil
	})
}

// ---- fetch_paper（FR-FETCH）----

type fetchPaperIn struct {
	Ref string `json:"ref" jsonschema:"论文引用：cite_key（3 字母）或 DOI；论文须已入库"`
}

func registerFetchPaper(s *gomcp.Server, deps Deps) {
	gomcp.AddTool[fetchPaperIn, any](s, &gomcp.Tool{
		Name:        "fetch_paper",
		Description: "取回论文全文：Unpaywall OA 优先 → Sci-Hub 兜底（失败静默）→ PDF 落盘 + 全文缓存入库；库中已缓存直接返回（via=cache）",
	}, func(ctx context.Context, _ *gomcp.CallToolRequest, in fetchPaperIn) (*gomcp.CallToolResult, any, error) {
		if deps.Store == nil || deps.Fulltext == nil {
			return nil, nil, errNoStore
		}
		res, err := deps.Fulltext.Fetch(ctx, in.Ref)
		if err != nil {
			return nil, nil, err
		}
		return nil, res, nil
	})
}

// ---- process_manuscript ----

type processManuscriptIn struct {
	Text         string `json:"text" jsonschema:"手稿正文，引用写 [@citeKey] 或 [@doi:]/[@pmid:]/[@arxiv:]/[@title:] 占位符"`
	Lang         string `json:"lang,omitempty" jsonschema:"写作语言模式 zh|en（默认 zh）"`
	Style        string `json:"style,omitempty" jsonschema:"引用样式 gb7714-2025|apa|ieee（默认按 lang：zh→gb7714-2025、en→apa）"`
	Preview      bool   `json:"preview,omitempty" jsonschema:"预览模式：内联标记自描述，不生成引用列表"`
	GenerateDocx bool   `json:"generateDocx,omitempty" jsonschema:"是否生成 docx（需安装 pandoc；缺 pandoc 时静默跳过）"`
	OutputDir    string `json:"outputDir,omitempty" jsonschema:"产物输出目录（默认工作目录；目录为空时只返回文本不写文件）"`
}

type processManuscriptOut struct {
	ProcessedText string            `json:"processedText"`
	ReferenceList string            `json:"referenceList"`
	CitationMap   map[string]string `json:"citationMap"`
	Unresolved    []string          `json:"unresolved"`
	Files         map[string]string `json:"files,omitempty"`
}

func registerProcessManuscript(s *gomcp.Server, deps Deps) {
	gomcp.AddTool[processManuscriptIn, processManuscriptOut](s, &gomcp.Tool{
		Name:        "process_manuscript",
		Description: "解析手稿占位符并生成规范引用（formatted 正文 + 引用列表 + 可选产物落盘）",
	}, func(ctx context.Context, _ *gomcp.CallToolRequest, in processManuscriptIn) (*gomcp.CallToolResult, processManuscriptOut, error) {
		out := processManuscriptOut{}
		if deps.Store == nil {
			return nil, out, errNoStore
		}
		lang := in.Lang
		if lang == "" {
			lang = "zh"
		}
		style, err := core.ResolveStyle(lang, in.Style)
		if err != nil {
			return nil, out, err
		}
		if in.Preview {
			style = core.StylePreview
		}
		res, err := core.ProcessManuscript(ctx, deps.Store, deps.Fetcher, in.Text, style)
		if err != nil {
			return nil, out, err
		}
		out = processManuscriptOut{
			ProcessedText: res.Text,
			CitationMap:   res.CitationMap,
			Unresolved:    res.Unresolved,
		}
		// 文末引用列表（含 preview：自描述标记 + 编号列表供人工核对）。
		refList, err := core.RenderReferenceList(res.Papers, style)
		if err != nil {
			return nil, out, err
		}
		out.ReferenceList = refList

		outDir := in.OutputDir
		if outDir == "" && deps.Cfg != nil {
			outDir = filepath.Join(deps.Cfg.WorkDir, "outputs")
		}
		if outDir == "" {
			return nil, out, nil // 无目录：只返回文本（LLM 直接可用）
		}
		if err := os.MkdirAll(outDir, outputDirPerm); err != nil {
			return nil, out, fmt.Errorf("mcp: process_manuscript: 创建输出目录失败: %w", err)
		}
		base := "manuscript"
		ts := core.ManuscriptStamp(time.Now())
		files, err := core.WriteManuscriptOutputs(outDir, base, ts, res, style)
		if err != nil {
			return nil, out, err
		}
		if in.GenerateDocx {
			if _, err := exec.LookPath("pandoc"); err == nil {
				docxPath := filepath.Join(outDir, base+"_"+ts+".docx")
				if err := core.PandocToDocx(files[core.ManuscriptFormatted], docxPath); err == nil {
					files["formatted.docx"] = docxPath
				}
			} // 缺 pandoc：优雅降级（FR-REF-11），主产物不受影响
		}
		out.Files = files
		return nil, out, nil
	})
}

// ---- export_references ----

type exportReferencesIn struct {
	Papers []model.Paper `json:"papers" jsonschema:"论文列表（完整元数据，可取自 search_papers 结果）"`
	Format string        `json:"format" jsonschema:"导出格式 bibtex|ris|text"`
	Style  string        `json:"style,omitempty" jsonschema:"引用样式（text 格式用；缺省 apa）"`
}

type exportReferencesOut struct {
	Success bool   `json:"success"`
	Content string `json:"content"`
}

func registerExportReferences(s *gomcp.Server, deps Deps) {
	gomcp.AddTool[exportReferencesIn, exportReferencesOut](s, &gomcp.Tool{
		Name:        "export_references",
		Description: "批量导出引用（BibTeX / RIS / 格式化文本）",
	}, func(_ context.Context, _ *gomcp.CallToolRequest, in exportReferencesIn) (*gomcp.CallToolResult, exportReferencesOut, error) {
		out := exportReferencesOut{}
		switch in.Format {
		case "bibtex":
			out.Success = true
			out.Content = core.BibTeXFromPapers(in.Papers)
			return nil, out, nil
		case "ris":
			out.Success = true
			out.Content = core.RISFromPapers(in.Papers)
			return nil, out, nil
		case "text":
			style, err := core.ResolveStyle("en", in.Style)
			if err != nil {
				return nil, out, err
			}
			content, err := core.RenderReferenceList(in.Papers, style)
			if err != nil {
				return nil, out, err
			}
			out.Success = true
			out.Content = content
			return nil, out, nil
		default:
			return nil, out, fmt.Errorf("mcp: export_references: 未知格式 %q（支持 bibtex|ris|text）", in.Format)
		}
	})
}

// ---- lint_init ----

type lintInitIn struct {
	ProjectDir string `json:"projectDir,omitempty" jsonschema:"项目目录（默认工作目录）"`
	Force      bool   `json:"force,omitempty" jsonschema:"覆盖已存在的 .litkit/ 与 AGENTS.md"`
	Lang       string `json:"lang,omitempty" jsonschema:"撰写语言 zh|en（默认 zh，仅首次生成 yaml 时生效）"`
	PaperType  string `json:"paperType,omitempty" jsonschema:"论文类型 review|empirical（默认 empirical，仅首次生成 yaml 时生效）"`
	Journal    string `json:"journal,omitempty" jsonschema:"目标期刊名称（写入 spec）"`
}

type lintInitOut struct {
	Status    string   `json:"status"`
	Files     []string `json:"files"`
	NextSteps []string `json:"nextSteps"`
}

func registerLintInit(s *gomcp.Server, deps Deps) {
	gomcp.AddTool[lintInitIn, lintInitOut](s, &gomcp.Tool{
		Name:        "lint_init",
		Description: "初始化撰写约束（.litkit/<type>/ 的 manuscript-spec.yaml）",
	}, func(_ context.Context, _ *gomcp.CallToolRequest, in lintInitIn) (*gomcp.CallToolResult, lintInitOut, error) {
		out := lintInitOut{}
		dir := in.ProjectDir
		if dir == "" {
			if deps.Cfg == nil || deps.Cfg.WorkDir == "" {
				return nil, out, errors.New("mcp: lint_init: 未指定 projectDir 且未设置工作目录")
			}
			dir = deps.Cfg.WorkDir
		}
		lang := in.Lang
		if lang == "" {
			lang = lint.LangZH
		}
		if lang != lint.LangZH && lang != lint.LangEN {
			return nil, out, fmt.Errorf("mcp: lint_init: 无效 lang %q（可选 zh|en）", lang)
		}
		paperType := in.PaperType
		if paperType == "" {
			paperType = lint.PaperTypeEmpirical
		}
		if paperType != lint.PaperTypeReview && paperType != lint.PaperTypeEmpirical {
			return nil, out, fmt.Errorf("mcp: lint_init: 无效 paperType %q（可选 review|empirical）", paperType)
		}

		// 项目基础设施（幂等）
		created, err := lint.InitProjectInfra(dir, in.Force)
		if err != nil {
			return nil, out, err
		}

		// 论文类型目录
		typeCreated, err := lint.InitPaperType(dir, paperType, lang, in.Force)
		if err != nil {
			return nil, out, err
		}
		created = append(created, typeCreated...)

		// 加载 spec，写入 journal
		specPath := lint.SpecPath(dir, paperType, lang)
		spec, err := lint.LoadSpec(specPath)
		if err != nil {
			spec = lint.SpecForType(paperType, lang)
		}
		if in.PaperType != "" {
			spec.PaperType = paperType
		}
		if in.Journal != "" {
			spec.Journal = in.Journal
		}
		if err := lint.WriteSpec(specPath, spec); err != nil {
			return nil, out, fmt.Errorf("mcp: lint_init: 写入 spec: %w", err)
		}

		out.Status = "ok"
		out.Files = created
		out.NextSteps = []string{"阅读 .litkit/<type>/manuscript-spec.yaml 获取撰写规定", "成稿后运行 verify_manuscript 检查"}
		return nil, out, nil
	})
}

// ---- verify_manuscript ----

type verifyManuscriptIn struct {
	Files     []string `json:"files" jsonschema:"待验证的 Markdown 文件路径"`
	Lang      string   `json:"lang,omitempty" jsonschema:"写作语言 zh|en（默认 zh）"`
	Mode      string   `json:"mode,omitempty" jsonschema:"验证模式 chapter|draft|final（默认 draft）"`
	PaperType string   `json:"paperType,omitempty" jsonschema:"论文类型 review|empirical（空=从 spec 自动取）"`
	Rule      string   `json:"rule,omitempty" jsonschema:"仅运行指定规则（逗号分隔，如 R2.1,R7.1）"`
	Skip      string   `json:"skip,omitempty" jsonschema:"跳过指定规则（逗号分隔）"`
}

func registerVerifyManuscript(s *gomcp.Server, deps Deps) {
	gomcp.AddTool[verifyManuscriptIn, lint.Report](s, &gomcp.Tool{
		Name:        "verify_manuscript",
		Description: "成稿后机械化验证（A/S 类规则检查，三要素报告；A 类违规需修复后重跑）",
	}, func(_ context.Context, _ *gomcp.CallToolRequest, in verifyManuscriptIn) (*gomcp.CallToolResult, lint.Report, error) {
		if len(in.Files) == 0 {
			return nil, lint.Report{}, errors.New("mcp: verify_manuscript: 缺少 files")
		}
		lang := in.Lang
		if lang == "" {
			lang = "zh"
		}
		if lang != "zh" && lang != "en" {
			return nil, lint.Report{}, fmt.Errorf("mcp: verify_manuscript: 无效 lang %q（可选 zh|en）", lang)
		}
		modeStr := in.Mode
		if modeStr == "" {
			modeStr = "draft"
		}
		mode := lint.Mode(modeStr)
		switch mode {
		case lint.ModeChapter, lint.ModeDraft, lint.ModeFinal:
		default:
			return nil, lint.Report{}, fmt.Errorf("mcp: verify_manuscript: 无效 mode %q（可选 chapter|draft|final）", modeStr)
		}
		paperType := in.PaperType
		if paperType != "" && paperType != lint.PaperTypeReview && paperType != lint.PaperTypeEmpirical {
			return nil, lint.Report{}, fmt.Errorf("mcp: verify_manuscript: 无效 paperType %q（可选 review|empirical）", paperType)
		}

		spec := lint.DefaultSpec()
		if deps.Cfg != nil && deps.Cfg.WorkDir != "" {
			if paperType != "" {
				if s, err := lint.LoadSpec(lint.SpecPath(deps.Cfg.WorkDir, paperType, in.Lang)); err == nil {
					spec = s
				}
			} else {
				// 自动检测唯一类型
				if types, err := lint.ListPaperTypes(deps.Cfg.WorkDir); err == nil && len(types) == 1 {
					if s, err := lint.LoadSpec(lint.TypeSpecPath(deps.Cfg.WorkDir, types[0])); err == nil {
						spec = s
					}
				}
			}
		}
		report, err := lint.RunFilesWithStore(in.Files, spec, lint.Options{
			Lang:      lang,
			Mode:      mode,
			PaperType: paperType,
			Only:      splitCSV(in.Rule),
			Skip:      splitCSV(in.Skip),
		}, deps.Store)
		if err != nil {
			return nil, lint.Report{}, err
		}
		return nil, report, nil
	})
}

// ---- 动态单源检索 search_<source> ----

func registerSourceTool(s *gomcp.Server, deps Deps, name string) {
	type sourceSearchIn struct {
		Query      string `json:"query" jsonschema:"检索关键词"`
		MaxResults int    `json:"maxResults,omitempty" jsonschema:"最大条数（默认 5）"`
	}
	gomcp.AddTool[sourceSearchIn, any](s, &gomcp.Tool{
		Name:        "search_" + name,
		Description: fmt.Sprintf("在 %s 源内单源检索文献（摘要工作流，结果入库）", name),
	}, func(ctx context.Context, _ *gomcp.CallToolRequest, in sourceSearchIn) (*gomcp.CallToolResult, any, error) {
		res, err := deps.Searcher.Search(ctx, in.Query, core.SearchOptions{
			Sources:    []string{name},
			MaxResults: in.MaxResults,
		})
		if err != nil {
			return nil, nil, err
		}
		srcCounts := make(map[string]int, len(res.SourceResults))
		for src, ps := range res.SourceResults {
			srcCounts[src] = len(ps)
		}
		out := searchPapersOut{
			Total:         res.Total,
			SourceResults: srcCounts,
			Errors:        shortErrors(res.Errors),
			Papers:        model.SummarizePapers(res.Papers),
		}
		return nil, out, nil
	})
}

// ---- lib_* ----

type libListIn struct {
	Source string `json:"source,omitempty" jsonschema:"按来源过滤"`
	Sort   string `json:"sort,omitempty" jsonschema:"排序方式 year（年份倒序，默认）| id（入库倒序）"`
	Limit  int    `json:"limit,omitempty" jsonschema:"最大条数（默认 100）"`
	Offset int    `json:"offset,omitempty" jsonschema:"偏移量（分页用）"`
	Full   bool   `json:"full,omitempty" jsonschema:"返回完整元数据（默认精简视图 PaperSummary，FR-IFACE-04）"`
}

type libSearchIn struct {
	Keyword string `json:"keyword" jsonschema:"本地库关键词（标题/作者/摘要 LIKE 匹配）"`
	Limit   int    `json:"limit,omitempty" jsonschema:"最大条数（默认 50）"`
	Offset  int    `json:"offset,omitempty" jsonschema:"偏移量（分页用）"`
	Full    bool   `json:"full,omitempty" jsonschema:"返回完整元数据（默认精简视图 PaperSummary，FR-IFACE-04）"`
}

type libRmIn struct {
	CiteKey string `json:"citeKey" jsonschema:"3 字母引用标识"`
}

type libPapersOut struct {
	Total  int                  `json:"total"`
	Papers []model.PaperSummary `json:"papers"`
}

type libPapersFullOut struct {
	Total  int           `json:"total"`
	Papers []model.Paper `json:"papers"`
}

type libRmOut struct {
	CiteKey string `json:"citeKey"`
	Removed bool   `json:"removed"`
}

type libPathOut struct {
	Path string `json:"path"`
}

func registerLibraryTools(s *gomcp.Server, deps Deps) {
	gomcp.AddTool[libListIn, any](s, &gomcp.Tool{
		Name:        "lib_list",
		Description: "列出本地文献库论文（默认按年份倒序，精简视图）",
	}, func(_ context.Context, _ *gomcp.CallToolRequest, in libListIn) (*gomcp.CallToolResult, any, error) {
		if deps.Store == nil {
			return nil, nil, errNoStore
		}
		sortBy := in.Sort
		if sortBy == "" {
			sortBy = "year"
		}
		papers, err := deps.Store.ListPapers(in.Source, sortBy, in.Limit, in.Offset)
		if err != nil {
			return nil, nil, err
		}
		if in.Full {
			return nil, libPapersFullOut{Total: len(papers), Papers: papers}, nil
		}
		return nil, libPapersOut{Total: len(papers), Papers: model.SummarizePapers(papers)}, nil
	})

	gomcp.AddTool[libSearchIn, any](s, &gomcp.Tool{
		Name:        "lib_search",
		Description: "本地文献库关键词检索（标题/作者/摘要，FR-LIB-04）",
	}, func(_ context.Context, _ *gomcp.CallToolRequest, in libSearchIn) (*gomcp.CallToolResult, any, error) {
		if deps.Store == nil {
			return nil, nil, errNoStore
		}
		papers, err := deps.Store.SearchLocal(in.Keyword, in.Limit, in.Offset)
		if err != nil {
			return nil, nil, err
		}
		if in.Full {
			return nil, libPapersFullOut{Total: len(papers), Papers: papers}, nil
		}
		return nil, libPapersOut{Total: len(papers), Papers: model.SummarizePapers(papers)}, nil
	})

	gomcp.AddTool[libRmIn, libRmOut](s, &gomcp.Tool{
		Name:        "lib_rm",
		Description: "删除一篇论文及其全部引用标记",
	}, func(_ context.Context, _ *gomcp.CallToolRequest, in libRmIn) (*gomcp.CallToolResult, libRmOut, error) {
		out := libRmOut{CiteKey: in.CiteKey}
		if deps.Store == nil {
			return nil, out, errNoStore
		}
		removed, err := deps.Store.Forget(in.CiteKey)
		if err != nil {
			return nil, out, err
		}
		out.Removed = removed
		return nil, out, nil
	})

	gomcp.AddTool[any, storage.Stats](s, &gomcp.Tool{
		Name:        "lib_stats",
		Description: "文献库统计（总量/有摘要/有 DOI/按源分布/库路径）",
	}, func(_ context.Context, _ *gomcp.CallToolRequest, _ any) (*gomcp.CallToolResult, storage.Stats, error) {
		if deps.Store == nil {
			return nil, storage.Stats{}, errNoStore
		}
		st, err := deps.Store.Stats()
		if err != nil {
			return nil, storage.Stats{}, err
		}
		return nil, st, nil
	})

	gomcp.AddTool[any, libPathOut](s, &gomcp.Tool{
		Name:        "lib_path",
		Description: "返回文献库文件绝对路径",
	}, func(_ context.Context, _ *gomcp.CallToolRequest, _ any) (*gomcp.CallToolResult, libPathOut, error) {
		if deps.Store == nil {
			return nil, libPathOut{}, errNoStore
		}
		return nil, libPathOut{Path: deps.Store.Path()}, nil
	})
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
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

// shortErrors 将完整源错误压缩为精简原因（FR-IFACE-04，与 CLI 一致）。
func shortErrors(errs []model.SourceError) []model.SourceError {
	out := make([]model.SourceError, len(errs))
	for i, e := range errs {
		out[i] = model.SourceError{Source: e.Source, Error: core.ShortError(e.Error)}
	}
	return out
}
