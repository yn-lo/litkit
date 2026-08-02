package storage

import (
	"path/filepath"
	"testing"

	"litkit/internal/model"
)

// newTestStore 打开临时目录中的 Store，测试结束自动关闭。
func newTestStore(t *testing.T) *Store {
	t.Helper()
	s, err := Open(filepath.Join(t.TempDir(), DefaultDBName))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

func paper(title, doi string, authors ...string) model.Paper {
	p := model.Paper{Title: title, DOI: doi, Abstract: "abs-" + title, Source: "fake"}
	for _, a := range authors {
		p.Authors = append(p.Authors, model.Author{Family: a})
	}
	p.ID = p.ComputeID()
	return p
}

// ---- Open / 迁移 ----

func TestOpen_CreatesTables(t *testing.T) {
	s := newTestStore(t)
	for _, tbl := range []string{"papers", "paper_refs"} {
		var n int
		err := s.db.QueryRow(
			"SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name=?", tbl,
		).Scan(&n)
		if err != nil {
			t.Fatalf("查表 %s: %v", tbl, err)
		}
		if n != 1 {
			t.Fatalf("表 %s 应存在，got %d", tbl, n)
		}
	}
}

func TestOpen_ReopenKeepsData(t *testing.T) {
	path := filepath.Join(t.TempDir(), DefaultDBName)
	s, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if _, _, err := s.UpsertPaper(paper("Persist", "10.1/persist")); err != nil {
		t.Fatalf("UpsertPaper: %v", err)
	}
	_ = s.Close()

	s2, err := Open(path)
	if err != nil {
		t.Fatalf("重开 Open: %v", err)
	}
	defer s2.Close()
	got, err := s2.GetByDOI("10.1/persist")
	if err != nil {
		t.Fatalf("GetByDOI: %v", err)
	}
	if got == nil || got.Title != "Persist" {
		t.Fatalf("重开后数据应仍在，got %+v", got)
	}
}

// ---- Upsert / 去重 ----

func TestUpsertPaper_NewInsert_AssignsCiteKey(t *testing.T) {
	s := newTestStore(t)
	citeKey, inserted, err := s.UpsertPaper(paper("Graph Nets", "10.1/x"))
	if err != nil {
		t.Fatalf("UpsertPaper: %v", err)
	}
	if !inserted {
		t.Fatal("首次入库应 inserted=true")
	}
	if len(citeKey) != 3 {
		t.Fatalf("cite_key 应为 3 字母，got %q", citeKey)
	}
	for _, c := range citeKey {
		if !((c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')) {
			t.Fatalf("cite_key 应仅含字母 a-zA-Z，got %q", citeKey)
		}
	}
}

func TestUpsertPaper_SameDOI_UpdatesNotInserts(t *testing.T) {
	s := newTestStore(t)
	k1, inserted1, err := s.UpsertPaper(paper("Title A", "10.1/same"))
	if err != nil {
		t.Fatalf("首次: %v", err)
	}
	if !inserted1 {
		t.Fatal("首次应 inserted=true")
	}
	k2, inserted2, err := s.UpsertPaper(paper("Title B", "10.1/same"))
	if err != nil {
		t.Fatalf("二次: %v", err)
	}
	if inserted2 {
		t.Fatal("同 DOI 二次应 inserted=false")
	}
	if k1 != k2 {
		t.Fatalf("同 DOI 应复用 cite_key：%q vs %q", k1, k2)
	}
	// 标题应被覆盖更新
	got, err := s.GetByCiteKey(k2)
	if err != nil {
		t.Fatalf("GetByCiteKey: %v", err)
	}
	if got.Title != "Title B" {
		t.Fatalf("更新后标题应为 Title B，got %q", got.Title)
	}
}

func TestUpsertPaper_SameTitleNoDOI_Dedup(t *testing.T) {
	s := newTestStore(t)
	_, inserted1, err := s.UpsertPaper(paper("Same Title", "", "Smith"))
	if err != nil {
		t.Fatalf("首次: %v", err)
	}
	if !inserted1 {
		t.Fatal("首次应 inserted=true")
	}
	// 同标题同作者、无 DOI：应命中同 dedup_key
	_, inserted2, err := s.UpsertPaper(paper("same title", "", "smith"))
	if err != nil {
		t.Fatalf("二次: %v", err)
	}
	if inserted2 {
		t.Fatal("同标题+同作者（无 DOI）应去重，inserted=false")
	}
}

func TestUpsertPaper_SameTitleDifferentAuthors_NotDedup(t *testing.T) {
	s := newTestStore(t)
	_, inserted1, err := s.UpsertPaper(paper("T", "", "Alice"))
	if err != nil {
		t.Fatalf("首次: %v", err)
	}
	if !inserted1 {
		t.Fatal("首次应 inserted=true")
	}
	// 同标题但不同作者：不同论文，不应去重
	_, inserted2, err := s.UpsertPaper(paper("T", "", "Bob"))
	if err != nil {
		t.Fatalf("二次: %v", err)
	}
	if !inserted2 {
		t.Fatal("同标题不同作者（无 DOI）应视为不同论文")
	}
}

func TestUpsertPapers_ReturnsNewCount(t *testing.T) {
	s := newTestStore(t)
	papers := []model.Paper{
		paper("A", "10.1/a"),
		paper("B", "10.1/b"),
		paper("A2", "10.1/a"), // 与 A 同 DOI
	}
	n, err := s.UpsertPapers(papers)
	if err != nil {
		t.Fatalf("UpsertPapers: %v", err)
	}
	if n != 2 {
		t.Fatalf("应新增 2 篇（A、B），got %d", n)
	}
}

// ---- 查询 ----

func TestGetByCiteKey_MissingReturnsNil(t *testing.T) {
	s := newTestStore(t)
	got, err := s.GetByCiteKey("zzz")
	if err != nil {
		t.Fatalf("GetByCiteKey: %v", err)
	}
	if got != nil {
		t.Fatalf("未入库 cite_key 应返回 nil，got %+v", got)
	}
}

func TestGetByDOI_CaseInsensitive(t *testing.T) {
	s := newTestStore(t)
	if _, _, err := s.UpsertPaper(paper("D", "10.1/DOIMIX")); err != nil {
		t.Fatalf("UpsertPaper: %v", err)
	}
	got, err := s.GetByDOI("10.1/doimix")
	if err != nil {
		t.Fatalf("GetByDOI: %v", err)
	}
	if got == nil {
		t.Fatal("DOI 查询应大小写不敏感命中")
	}
}

func TestListPapers_OrderedByNewest(t *testing.T) {
	s := newTestStore(t)
	for _, p := range []model.Paper{paper("First", "10.1/f"), paper("Second", "10.1/s")} {
		if _, _, err := s.UpsertPaper(p); err != nil {
			t.Fatalf("UpsertPaper: %v", err)
		}
	}
	papers, err := s.ListPapers("", 10, 0)
	if err != nil {
		t.Fatalf("ListPapers: %v", err)
	}
	if len(papers) != 2 {
		t.Fatalf("应列出 2 篇，got %d", len(papers))
	}
	if papers[0].Title != "Second" {
		t.Fatalf("最新入库应在最前，got %q", papers[0].Title)
	}
}

func TestListPapers_FilterBySource(t *testing.T) {
	s := newTestStore(t)
	a := paper("A", "10.1/a")
	a.Source = "arxiv"
	b := paper("B", "10.1/b")
	b.Source = "pubmed"
	if _, _, err := s.UpsertPaper(a); err != nil {
		t.Fatalf("UpsertPaper: %v", err)
	}
	if _, _, err := s.UpsertPaper(b); err != nil {
		t.Fatalf("UpsertPaper: %v", err)
	}
	papers, err := s.ListPapers("arxiv", 10, 0)
	if err != nil {
		t.Fatalf("ListPapers: %v", err)
	}
	if len(papers) != 1 || papers[0].Source != "arxiv" {
		t.Fatalf("应按源过滤，got %+v", papers)
	}
}

func TestSearchLocal_MatchesTitleAbstract(t *testing.T) {
	s := newTestStore(t)
	if _, _, err := s.UpsertPaper(model.Paper{
		Title: "Graph Neural Networks for Pain", DOI: "10.1/g", Abstract: "a study", Source: "fake",
	}); err != nil {
		t.Fatalf("UpsertPaper: %v", err)
	}
	got, err := s.SearchLocal("neural", 10)
	if err != nil {
		t.Fatalf("SearchLocal: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("关键词应命中 1 篇，got %d", len(got))
	}
	miss, err := s.SearchLocal("nonexistent-token", 10)
	if err != nil {
		t.Fatalf("SearchLocal miss: %v", err)
	}
	if len(miss) != 0 {
		t.Fatalf("无关关键词应 0 命中，got %d", len(miss))
	}
}

// ---- 删除 ----

func TestForget_RemovesPaperAndRefs(t *testing.T) {
	s := newTestStore(t)
	citeKey, _, err := s.UpsertPaper(paper("Doomed", "10.1/d"))
	if err != nil {
		t.Fatalf("UpsertPaper: %v", err)
	}
	if err := s.AddRef(Ref{CiteKey: citeKey, SentenceHash: "h1", SentenceText: "s"}); err != nil {
		t.Fatalf("AddRef: %v", err)
	}
	removed, err := s.Forget(citeKey)
	if err != nil {
		t.Fatalf("Forget: %v", err)
	}
	if !removed {
		t.Fatal("Forget 应返回 removed=true")
	}
	if got, _ := s.GetByCiteKey(citeKey); got != nil {
		t.Fatal("删除后论文不应再查到")
	}
	refs, err := s.ListRefs(citeKey)
	if err != nil {
		t.Fatalf("ListRefs: %v", err)
	}
	if len(refs) != 0 {
		t.Fatalf("删除后引用应级联清理，got %d", len(refs))
	}
}

func TestForget_Missing(t *testing.T) {
	s := newTestStore(t)
	removed, err := s.Forget("zzz")
	if err != nil {
		t.Fatalf("Forget: %v", err)
	}
	if removed {
		t.Fatal("不存在的 cite_key 应返回 removed=false")
	}
}

// ---- 引用标记 ----

func TestAddRef_DuplicateIgnored(t *testing.T) {
	s := newTestStore(t)
	citeKey, _, err := s.UpsertPaper(paper("Cited", "10.1/c"))
	if err != nil {
		t.Fatalf("UpsertPaper: %v", err)
	}
	ref := Ref{CiteKey: citeKey, SentenceHash: "h1", SentenceText: "句1", Manuscript: "draft.md"}
	if err := s.AddRef(ref); err != nil {
		t.Fatalf("AddRef 首次: %v", err)
	}
	if err := s.AddRef(ref); err != nil {
		t.Fatalf("AddRef 重复应不报错: %v", err)
	}
	refs, err := s.ListRefs(citeKey)
	if err != nil {
		t.Fatalf("ListRefs: %v", err)
	}
	if len(refs) != 1 {
		t.Fatalf("重复引用应仅 1 条，got %d", len(refs))
	}
	if refs[0].SentenceText != "句1" || refs[0].Manuscript != "draft.md" {
		t.Fatalf("引用字段不符：%+v", refs[0])
	}
}

func TestAddRef_UnknownCiteKey_Rejected(t *testing.T) {
	s := newTestStore(t)
	if err := s.AddRef(Ref{CiteKey: "nope", SentenceHash: "h", SentenceText: "s"}); err == nil {
		t.Fatal("引用不存在的 cite_key 应报错（外键约束）")
	}
}

// ---- Stats ----

func TestStats_Counts(t *testing.T) {
	s := newTestStore(t)
	a := paper("A", "10.1/a")
	a.Source = "arxiv"
	b := paper("B", "10.1/b")
	b.Source = "arxiv"
	if _, _, err := s.UpsertPaper(a); err != nil {
		t.Fatalf("UpsertPaper: %v", err)
	}
	if _, _, err := s.UpsertPaper(b); err != nil {
		t.Fatalf("UpsertPaper: %v", err)
	}
	st, err := s.Stats()
	if err != nil {
		t.Fatalf("Stats: %v", err)
	}
	if st.Total != 2 {
		t.Fatalf("Total 应 2，got %d", st.Total)
	}
	if st.WithAbstract != 2 {
		t.Fatalf("WithAbstract 应 2，got %d", st.WithAbstract)
	}
	if st.WithDOI != 2 {
		t.Fatalf("WithDOI 应 2，got %d", st.WithDOI)
	}
	if st.BySource["arxiv"] != 2 {
		t.Fatalf("BySource[arxiv] 应 2，got %d", st.BySource["arxiv"])
	}
	if st.DBPath == "" {
		t.Fatal("Stats 应含 DBPath")
	}
}
