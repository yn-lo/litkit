package mcp

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	gomcp "github.com/modelcontextprotocol/go-sdk/mcp"

	"litkit/internal/config"
	"litkit/internal/core"
	"litkit/internal/lint"
	"litkit/internal/model"
	"litkit/internal/storage"
	"litkit/internal/util/httpclient"
)

// newTestDeps 构造最小依赖：临时 store + 空 registry（不触网）。
func newTestDeps(t *testing.T) (Deps, *storage.Store) {
	t.Helper()
	store, err := storage.Open(filepath.Join(t.TempDir(), storage.DefaultDBName))
	if err != nil {
		t.Fatalf("storage.Open: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	return Deps{
		Cfg:      &config.Config{},
		Store:    store,
		Searcher: core.NewSearcher(nil, store, 5, 0),
		Fetcher:  core.NewMetadataFetcher(httpclient.New(httpclient.Options{})),
	}, store
}

// newTestClient 通过内存传输连接 server，返回已初始化的客户端会话。
func newTestClient(t *testing.T, deps Deps) *gomcp.ClientSession {
	t.Helper()
	serverT, clientT := gomcp.NewInMemoryTransports()
	srv := New(deps)
	if _, err := srv.Connect(context.Background(), serverT, nil); err != nil {
		t.Fatalf("server.Connect: %v", err)
	}
	client := gomcp.NewClient(&gomcp.Implementation{Name: "litkit-test", Version: "0.0.0"}, nil)
	cs, err := client.Connect(context.Background(), clientT, nil)
	if err != nil {
		t.Fatalf("client.Connect: %v", err)
	}
	t.Cleanup(func() { _ = cs.Close() })
	return cs
}

// callTool 调用工具；JSON-RPC 级错误直接失败（工具内错误以 IsError 返回，由断言处理）。
func callTool(t *testing.T, cs *gomcp.ClientSession, name string, args map[string]any) *gomcp.CallToolResult {
	t.Helper()
	res, err := cs.CallTool(context.Background(), &gomcp.CallToolParams{Name: name, Arguments: args})
	if err != nil {
		t.Fatalf("CallTool(%s): %v", name, err)
	}
	return res
}

// structuredAs 将工具的结构化输出解码到 out。
func structuredAs[T any](t *testing.T, res *gomcp.CallToolResult, out *T) {
	t.Helper()
	b, err := json.Marshal(res.StructuredContent)
	if err != nil {
		t.Fatalf("marshal structuredContent: %v", err)
	}
	if err := json.Unmarshal(b, out); err != nil {
		t.Fatalf("unmarshal structuredContent: %v", err)
	}
}

// textContent 提取结果中的文本内容。
func textContent(t *testing.T, res *gomcp.CallToolResult) string {
	t.Helper()
	for _, c := range res.Content {
		if tc, ok := c.(*gomcp.TextContent); ok {
			return tc.Text
		}
	}
	t.Fatalf("结果无文本内容: %v", res.Content)
	return ""
}

// TestToolsList 工具清单与 api.md §2 一致（FR-IFACE-03）。
func TestToolsList(t *testing.T) {
	deps, _ := newTestDeps(t)
	cs := newTestClient(t, deps)

	want := map[string]bool{
		"search_papers":      true,
		"get_paper_metadata": true,
		"process_manuscript": true,
		"export_references":  true,
		"lint_init":          true,
		"verify_manuscript":  true,
		"lib_list":           true,
		"lib_search":         true,
		"lib_rm":             true,
		"lib_stats":          true,
		"lib_path":           true,
	}
	for tool, err := range cs.Tools(context.Background(), nil) {
		if err != nil {
			t.Fatalf("Tools: %v", err)
		}
		if !want[tool.Name] {
			t.Errorf("意外工具 %q（新增功能须在 api.md §2 登记）", tool.Name)
		}
		delete(want, tool.Name)
	}
	for name := range want {
		t.Errorf("缺少工具 %q", name)
	}
}

// TestCallTool_ExportReferencesText export_references(text) 输出与 core.RenderReferenceList 一致
// （CLI 与 MCP 同输入同输出，FR-IFACE-03）。
func TestCallTool_ExportReferencesText(t *testing.T) {
	deps, _ := newTestDeps(t)
	cs := newTestClient(t, deps)

	papers := []model.Paper{{
		Title:   "Attention Is All You Need",
		Authors: []model.Author{{Given: "Ashish", Family: "Vaswani"}, {Given: "Noam", Family: "Shazeer"}},
		Year:    2017, DOI: "10.5555/3295222.3295349",
	}}
	args := map[string]any{"papers": papers, "format": "text", "style": "apa"}

	res := callTool(t, cs, "export_references", args)
	if res.IsError {
		t.Fatalf("export_references 不应报错: %v", res.Content)
	}
	var out exportReferencesOut
	structuredAs(t, res, &out)
	if !out.Success {
		t.Fatal("Success 应为 true")
	}
	want, err := core.RenderReferenceList(papers, core.StyleAPA)
	if err != nil {
		t.Fatalf("RenderReferenceList: %v", err)
	}
	if out.Content != want {
		t.Errorf("text 输出与 CLI 核心不一致\n got: %q\nwant: %q", out.Content, want)
	}
}

// TestCallTool_ExportReferencesBibtex bibtex 输出与 core.BibTeXFromPapers 一致。
func TestCallTool_ExportReferencesBibtex(t *testing.T) {
	deps, _ := newTestDeps(t)
	cs := newTestClient(t, deps)

	papers := []model.Paper{{Title: "A Paper", Authors: []model.Author{{Family: "Li"}}, Year: 2020, DOI: "10.1/x"}}
	res := callTool(t, cs, "export_references", map[string]any{
		"papers": papers, "format": "bibtex",
	})
	if res.IsError {
		t.Fatalf("export_references 不应报错: %v", res.Content)
	}
	var out exportReferencesOut
	structuredAs(t, res, &out)
	if want := core.BibTeXFromPapers(papers); out.Content != want {
		t.Errorf("bibtex 输出不一致\n got: %q\nwant: %q", out.Content, want)
	}
}

// TestCallTool_ExportReferencesBadFormat 未知格式返回 IsError（参数校验）。
func TestCallTool_ExportReferencesBadFormat(t *testing.T) {
	deps, _ := newTestDeps(t)
	cs := newTestClient(t, deps)

	res := callTool(t, cs, "export_references", map[string]any{"papers": []model.Paper{}, "format": "xml"})
	if !res.IsError {
		t.Fatal("未知格式应返回 IsError")
	}
}

// TestCallTool_VerifyManuscript verify_manuscript 在临时文件上生成报告。
func TestCallTool_VerifyManuscript(t *testing.T) {
	deps, _ := newTestDeps(t)
	cs := newTestClient(t, deps)

	path := filepath.Join(t.TempDir(), "draft.md")
	if err := os.WriteFile(path, []byte("# 标题\n\n正文 [1]。\n"), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	res := callTool(t, cs, "verify_manuscript", map[string]any{
		"files": []string{path}, "lang": "zh", "mode": "chapter",
	})
	if res.IsError {
		t.Fatalf("verify_manuscript 不应报错: %v", res.Content)
	}
	var report lint.Report
	structuredAs(t, res, &report)
	if len(report.Files) != 1 {
		t.Errorf("Report.Files 应含 1 个文件，got %d", len(report.Files))
	}
}

// TestCallTool_VerifyManuscriptMissingFiles 缺必填 files 由 SDK 输入校验拦截。
func TestCallTool_VerifyManuscriptMissingFiles(t *testing.T) {
	deps, _ := newTestDeps(t)
	cs := newTestClient(t, deps)

	res := callTool(t, cs, "verify_manuscript", map[string]any{})
	if !res.IsError {
		t.Fatal("缺 files 应返回 IsError")
	}
}

// TestCallTool_LintInit 初始化生成 AGENTS.md 与 .litkit/ 四件套。
func TestCallTool_LintInit(t *testing.T) {
	deps, _ := newTestDeps(t)
	cs := newTestClient(t, deps)

	dir := t.TempDir()
	res := callTool(t, cs, "lint_init", map[string]any{"projectDir": dir, "lang": "zh"})
	if res.IsError {
		t.Fatalf("lint_init 不应报错: %v", res.Content)
	}
	var out lintInitOut
	structuredAs(t, res, &out)
	if out.Status != "ok" {
		t.Errorf("Status = %q, want ok", out.Status)
	}
	for _, f := range []string{"AGENTS.md", ".litkit/rules.md", ".litkit/checklist.md"} {
		if _, err := os.Stat(filepath.Join(dir, f)); err != nil {
			t.Errorf("lint_init 未生成 %s: %v", f, err)
		}
	}
}

// TestCallTool_LintInitWithPaperType lint_init 传入 paperType/journal 后 spec 包含对应字段。
func TestCallTool_LintInitWithPaperType(t *testing.T) {
	deps, _ := newTestDeps(t)
	cs := newTestClient(t, deps)

	dir := t.TempDir()
	res := callTool(t, cs, "lint_init", map[string]any{
		"projectDir": dir, "lang": "zh", "paperType": "review", "journal": "中华医学杂志",
	})
	if res.IsError {
		t.Fatalf("lint_init 不应报错: %v", res.Content)
	}
	// 验证生成的 spec 包含 paper_type 和 journal
	spec, err := lint.LoadSpec(lint.SpecPath(dir))
	if err != nil {
		t.Fatalf("LoadSpec: %v", err)
	}
	if spec.PaperType != lint.PaperTypeReview {
		t.Errorf("spec.PaperType = %q, want %q", spec.PaperType, lint.PaperTypeReview)
	}
	if spec.Journal != "中华医学杂志" {
		t.Errorf("spec.Journal = %q, want %q", spec.Journal, "中华医学杂志")
	}
	// AGENTS.md 应包含论文类型行
	agents, err := os.ReadFile(filepath.Join(dir, "AGENTS.md"))
	if err != nil {
		t.Fatalf("ReadFile AGENTS.md: %v", err)
	}
	if !strings.Contains(string(agents), "综述（review）") {
		t.Error("AGENTS.md 应包含论文类型行")
	}
	if !strings.Contains(string(agents), "中华医学杂志") {
		t.Error("AGENTS.md 应包含目标期刊行")
	}
}

// TestCallTool_LintInitBadPaperType 无效 paperType 返回 IsError。
func TestCallTool_LintInitBadPaperType(t *testing.T) {
	deps, _ := newTestDeps(t)
	cs := newTestClient(t, deps)

	dir := t.TempDir()
	res := callTool(t, cs, "lint_init", map[string]any{
		"projectDir": dir, "paperType": "case_report",
	})
	if !res.IsError {
		t.Fatal("无效 paperType 应返回 IsError")
	}
}

// TestCallTool_VerifyManuscriptWithPaperType verify_manuscript 传入 paperType 后按类型过滤规则。
func TestCallTool_VerifyManuscriptWithPaperType(t *testing.T) {
	deps, _ := newTestDeps(t)
	cs := newTestClient(t, deps)

	// 写入含小写 p 值的文件（R2.1 仅 empirical 触发）
	path := filepath.Join(t.TempDir(), "draft.md")
	if err := os.WriteFile(path, []byte("# 标题\n\n结果显示 p=0.03 显著。\n"), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	// review 类型：R2.1 不适用，不应有 R2.1 违规
	res := callTool(t, cs, "verify_manuscript", map[string]any{
		"files": []string{path}, "lang": "zh", "mode": "draft", "paperType": "review",
	})
	if res.IsError {
		t.Fatalf("verify_manuscript(review) 不应报错: %v", res.Content)
	}
	var reviewReport lint.Report
	structuredAs(t, res, &reviewReport)
	for _, f := range reviewReport.Files {
		for _, v := range f.Violations {
			if v.RuleID == "R2.1" {
				t.Error("review 类型不应触发 R2.1（P值格式仅实证）")
			}
		}
	}

	// empirical 类型：R2.1 应触发
	res2 := callTool(t, cs, "verify_manuscript", map[string]any{
		"files": []string{path}, "lang": "zh", "mode": "draft", "paperType": "empirical",
	})
	if res2.IsError {
		t.Fatalf("verify_manuscript(empirical) 不应报错: %v", res2.Content)
	}
	var empReport lint.Report
	structuredAs(t, res2, &empReport)
	found := false
	for _, f := range empReport.Files {
		for _, v := range f.Violations {
			if v.RuleID == "R2.1" {
				found = true
			}
		}
	}
	if !found {
		t.Error("empirical 类型应触发 R2.1（P值格式）")
	}
}

// TestCallTool_VerifyManuscriptBadPaperType 无效 paperType 返回 IsError。
func TestCallTool_VerifyManuscriptBadPaperType(t *testing.T) {
	deps, _ := newTestDeps(t)
	cs := newTestClient(t, deps)

	dir := t.TempDir()
	path := filepath.Join(dir, "draft.md")
	if err := os.WriteFile(path, []byte("# 标题\n\n正文\n"), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	res := callTool(t, cs, "verify_manuscript", map[string]any{
		"files": []string{path}, "paperType": "meta_analysis",
	})
	if !res.IsError {
		t.Fatal("无效 paperType 应返回 IsError")
	}
}

// TestCallTool_ProcessManuscript 预置论文后解析 [@citeKey] 占位符（与 core 同管线）。
func TestCallTool_ProcessManuscript(t *testing.T) {
	deps, store := newTestDeps(t)
	cs := newTestClient(t, deps)

	p := model.Paper{Title: "Deep Learning", Authors: []model.Author{{Family: "LeCun"}}, Year: 2015, Abstract: "abs", Source: "fake"}
	p.ID = p.ComputeID()
	citeKey, _, err := store.UpsertPaper(p)
	if err != nil {
		t.Fatalf("UpsertPaper: %v", err)
	}

	res := callTool(t, cs, "process_manuscript", map[string]any{
		"text": "引言 [@" + citeKey + "]，结语 [@" + citeKey + "]",
		"lang": "zh",
	})
	if res.IsError {
		t.Fatalf("process_manuscript 不应报错: %v", res.Content)
	}
	var out processManuscriptOut
	structuredAs(t, res, &out)
	if out.ProcessedText != "引言 [1]，结语 [1]" {
		t.Errorf("ProcessedText = %q, want 引言 [1]，结语 [1]", out.ProcessedText)
	}
	if out.CitationMap[citeKey] != "[1]" {
		t.Errorf("CitationMap[%s] = %q, want [1]", citeKey, out.CitationMap[citeKey])
	}
	if len(out.Unresolved) != 0 {
		t.Errorf("Unresolved = %v, want 空", out.Unresolved)
	}
	if !strings.Contains(out.ReferenceList, "Deep Learning") {
		t.Errorf("ReferenceList 应含论文题名: %q", out.ReferenceList)
	}
}

// TestCallTool_ProcessManuscriptNoStore 无文献库时报错（与 CLI 行为一致）。
func TestCallTool_ProcessManuscriptNoStore(t *testing.T) {
	deps := Deps{Cfg: &config.Config{}, Searcher: core.NewSearcher(nil, nil, 5, 0)}
	cs := newTestClient(t, deps)

	res := callTool(t, cs, "process_manuscript", map[string]any{"text": "x", "lang": "zh"})
	if !res.IsError {
		t.Fatal("无 store 应返回 IsError")
	}
	if !strings.Contains(res.Content[0].(*gomcp.TextContent).Text, "本地文献库不可用") {
		t.Errorf("错误信息应提示文献库: %v", res.Content)
	}
}

// TestCallTool_LibStats 有 store 时正常返回统计。
func TestCallTool_LibStats(t *testing.T) {
	deps, _ := newTestDeps(t)
	cs := newTestClient(t, deps)

	res := callTool(t, cs, "lib_stats", map[string]any{})
	if res.IsError {
		t.Fatalf("lib_stats 不应报错: %v", res.Content)
	}
	var st storage.Stats
	structuredAs(t, res, &st)
	if st.Total != 0 {
		t.Errorf("Stats.Total = %d, want 0", st.Total)
	}
}

// fakeFetcher 固定返回未命中（null 路径，避免触网）。
type fakeFetcher struct{}

func (fakeFetcher) Fetch(context.Context, string, string) (*model.Paper, error) { return nil, nil }

// TestCallTool_GetPaperMetadataNull 未命中返回文本 "null"（AI 可判空）。
func TestCallTool_GetPaperMetadataNull(t *testing.T) {
	deps := Deps{
		Cfg:      &config.Config{},
		Fetcher:  fakeFetcher{},
		Searcher: core.NewSearcher(nil, nil, 5, 0),
	}
	cs := newTestClient(t, deps)

	res := callTool(t, cs, "get_paper_metadata", map[string]any{
		"idType": "doi", "identifier": "10.0000/nonexistent",
	})
	if res.IsError {
		t.Fatalf("get_paper_metadata 不应报错（未命中是正常态）: %v", res.Content)
	}
	got := textContent(t, res)
	if got != "null" {
		t.Errorf("未命中应返回文本 null，got %q", got)
	}
}
