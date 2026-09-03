package lint

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"litkit/internal/model"
	"litkit/internal/storage"
)

// fakeResolver 测试用撤稿解析器：按 DOI 查表返回预置状态。
type fakeResolver struct {
	byDOI map[string]RetractionStatus
	err   error
}

func (f fakeResolver) Resolve(_ context.Context, doi string) (RetractionStatus, error) {
	if f.err != nil {
		return RetractionStatus{}, f.err
	}
	return f.byDOI[doi], nil
}

// ---- R5.7 撤稿校验 ----

func TestCheckRetractions_ReportsRetracted(t *testing.T) {
	s := newTestStore(t)
	key, _, err := s.UpsertPaper(model.Paper{Title: "Retracted", DOI: "10.1/retracted", Authors: []model.Author{{Family: "A"}}})
	if err != nil {
		t.Fatalf("UpsertPaper: %v", err)
	}
	resolver := fakeResolver{byDOI: map[string]RetractionStatus{
		"10.1/retracted": {Retracted: true, RetractionDOI: "10.1/decl", Source: "retraction-watch"},
	}}
	src := mustParse(t, "本文引用[@"+key+"]。\n")
	v := checkRetractions([]*Source{src}, s, resolver)
	if len(v) != 1 {
		t.Fatalf("已撤稿应报 1 条 R5.7，got %d: %+v", len(v), v)
	}
	if v[0].v.RuleID != ruleRetracted {
		t.Errorf("RuleID 应为 %s，got %s", ruleRetracted, v[0].v.RuleID)
	}
}

// TestCheckRetractions_DedupAcrossFiles 同一被撤稿文献在多文件多次引用 → 仅首次出现处报一条。
func TestCheckRetractions_DedupAcrossFiles(t *testing.T) {
	s := newTestStore(t)
	key, _, err := s.UpsertPaper(model.Paper{Title: "Retracted", DOI: "10.1/retracted"})
	if err != nil {
		t.Fatalf("UpsertPaper: %v", err)
	}
	resolver := fakeResolver{byDOI: map[string]RetractionStatus{"10.1/retracted": {Retracted: true, RetractionDOI: "10.1/decl"}}}
	// 两个文件都引用同一篇被撤稿文献
	file1 := mustParse(t, "第一章引用[@"+key+"]。\n")
	file2 := mustParse(t, "第二章再次引用[@"+key+"]。\n")
	v := checkRetractions([]*Source{file1, file2}, s, resolver)
	if len(v) != 1 {
		t.Fatalf("同一撤稿文献跨文件应只报 1 条，got %d: %+v", len(v), v)
	}
	if v[0].file != 0 {
		t.Errorf("应在首次出现文件(file=0)归属，got file=%d", v[0].file)
	}
}

func TestCheckRetractions_SkipsUnretractedAndNoDOI(t *testing.T) {
	s := newTestStore(t)
	okKey, _, _ := s.UpsertPaper(model.Paper{Title: "Fine", DOI: "10.1/fine"})
	// 无 DOI 论文（获取 citeKey 但无 DOI 无法查撤稿）
	noDOIKey, _, _ := s.UpsertPaper(model.Paper{Title: "NoDOI", Year: 2020})
	resolver := fakeResolver{byDOI: map[string]RetractionStatus{}}
	src := mustParse(t, "引用[@"+okKey+"]与[@"+noDOIKey+"]。\n")
	if got := checkRetractions([]*Source{src}, s, resolver); len(got) != 0 {
		t.Errorf("未撤稿/无 DOI 不应报 R5.7，got %v", got)
	}
}

func TestCheckRetractions_NilStoreNoResolve(t *testing.T) {
	src := mustParse(t, "引用[@k]。\n")
	if got := checkRetractions([]*Source{src}, nil, fakeResolver{}); len(got) != 0 {
		t.Errorf("store 为 nil 应跳过，got %v", got)
	}
	if got := checkRetractions([]*Source{src}, newTestStore(t), nil); len(got) != 0 {
		t.Errorf("resolver 为 nil 应跳过，got %v", got)
	}
}

func TestCheckRetractions_NetworkErrorSkips(t *testing.T) {
	s := newTestStore(t)
	key, _, _ := s.UpsertPaper(model.Paper{Title: "X", DOI: "10.1/x"})
	src := mustParse(t, "引用[@"+key+"]。\n")
	// 网络失败应静默跳过，不阻断
	if got := checkRetractions([]*Source{src}, s, fakeResolver{err: context.DeadlineExceeded}); len(got) != 0 {
		t.Errorf("网络失败应静默跳过，got %v", got)
	}
}

// ---- R5.8 引用时效 + R5.9 自引比例 ----

// makeStoreWith 建库并入库若干论文，返回 citeKey→paper 的关系与全部 key。
func makeStoreWith(t *testing.T, papers []model.Paper) (*storage.Store, []string) {
	t.Helper()
	s := newTestStore(t)
	keys := make([]string, 0, len(papers))
	for _, p := range papers {
		k, _, err := s.UpsertPaper(p)
		if err != nil {
			t.Fatalf("UpsertPaper: %v", err)
		}
		keys = append(keys, k)
	}
	return s, keys
}

func TestCheckCitationHealth_Age(t *testing.T) {
	s, keys := makeStoreWith(t, []model.Paper{
		{Title: "Old", Year: 2000, DOI: "10.1/old"},
		{Title: "Recent", Year: 2021, DOI: "10.1/recent"},
	})
	spec := DefaultSpec()
	spec.Citation.MaxAgeYears = 10 // 截止 baselineYear-10
	src := mustParse(t, "引用[@"+keys[0]+"]与[@"+keys[1]+"]。\n")
	got := checkCitationHealth([]*Source{src}, s, spec, 2026)
	if len(got) != 1 {
		t.Fatalf("应报 1 条 R5.8，got %d: %+v", len(got), got)
	}
	if got[0].v.RuleID != ruleCurrency {
		t.Errorf("RuleID 应为 %s，got %s", ruleCurrency, got[0].v.RuleID)
	}
}

func TestCheckCitationHealth_SelfCite(t *testing.T) {
	s, keys := makeStoreWith(t, []model.Paper{
		{Title: "Mine", Year: 2021, Authors: []model.Author{{Family: "Wang"}, {Given: "Xia", Family: "Li"}}},
		{Title: "Other", Year: 2021, Authors: []model.Author{{Family: "Smith"}}},
		{Title: "Other2", Year: 2021, Authors: []model.Author{{Family: "Jones"}}},
	})
	spec := DefaultSpec()
	spec.Citation.MaxAgeYears = 10 // 避免年份干扰 R5.8
	spec.Citation.SelfCitationAuthors = []string{"Wang"}
	src := mustParse(t, "引用[@"+keys[0]+"]、[@"+keys[1]+"]、[@"+keys[2]+"]。\n")
	got := checkCitationHealth([]*Source{src}, s, spec, 2026)
	// 自引 1/3≈33% > 默认 15% → 1 条 R5.9
	found := false
	for _, h := range got {
		if h.v.RuleID == ruleSelfCite {
			found = true
		}
	}
	if !found {
		t.Fatalf("应报 R5.9，got %+v", got)
	}
}

func TestCheckCitationHealth_NoSelfCiteConfigured(t *testing.T) {
	s, keys := makeStoreWith(t, []model.Paper{
		{Title: "Mine", Year: 2021, Authors: []model.Author{{Family: "Wang"}}},
		{Title: "O2", Year: 2021, Authors: []model.Author{{Family: "Smith"}}},
	})
	spec := DefaultSpec()
	spec.Citation.SelfCitationAuthors = []string{} // 未启用
	src := mustParse(t, "引用[@"+keys[0]+"]、[@"+keys[1]+"]。\n")
	got := checkCitationHealth([]*Source{src}, s, spec, 2026)
	for _, h := range got {
		if h.v.RuleID == ruleSelfCite {
			t.Fatalf("未配置自引作者不应报 R5.9，got %+v", got)
		}
	}
}

// ---- RunFilesWithStore + fake resolver：R5.7→A 提升 exitHint ----

func TestRunFilesWithStore_RetractionPromotesToFix(t *testing.T) {
	s, keys := makeStoreWith(t, []model.Paper{{Title: "Retracted", DOI: "10.1/retracted"}})
	resolver := fakeResolver{byDOI: map[string]RetractionStatus{"10.1/retracted": {Retracted: true}}}
	dir := t.TempDir()
	path := filepath.Join(dir, "doc.md")
	if err := os.WriteFile(path, []byte("正文引用[@"+keys[0]+"]。\n"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	rep, err := RunFilesWithStore([]string{path}, DefaultSpec(), Options{Lang: "zh", Mode: ModeDraft}, s, resolver)
	if err != nil {
		t.Fatalf("RunFilesWithStore: %v", err)
	}
	if rep.ExitHint != "fix_and_rerun" || rep.Passed {
		t.Errorf("撤稿应提升为 fix_and_rerun，got %s passed=%v", rep.ExitHint, rep.Passed)
	}
	_ = keys
}
