package storage

import (
	"database/sql"
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
	for _, tbl := range []string{"papers", "paper_refs", "citation_scores"} {
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
	defer func() { _ = s2.Close() }()
	got, err := s2.GetByDOI("10.1/persist")
	if err != nil {
		t.Fatalf("GetByDOI: %v", err)
	}
	if got == nil || got.Title != "Persist" {
		t.Fatalf("重开后数据应仍在，got %+v", got)
	}
}

// TestOpen_OldSchemaAddsNewColumns 旧库迁移：Open 前手工建不含
// volume/number/pages 的旧版 papers 表，迁移后应自动补列。
func TestOpen_OldSchemaAddsNewColumns(t *testing.T) {
	path := filepath.Join(t.TempDir(), DefaultDBName)
	old, err := sql.Open("sqlite", filepath.ToSlash(path))
	if err != nil {
		t.Fatalf("open old db: %v", err)
	}
	if _, err := old.Exec(`CREATE TABLE papers (
		id         INTEGER PRIMARY KEY AUTOINCREMENT,
		dedup_key  TEXT UNIQUE NOT NULL,
		cite_key   TEXT UNIQUE NOT NULL,
		doi        TEXT NOT NULL DEFAULT '',
		paper_id   TEXT NOT NULL DEFAULT '',
		title      TEXT NOT NULL,
		authors    TEXT NOT NULL DEFAULT '[]',
		abstract   TEXT NOT NULL DEFAULT '',
		year       INTEGER NOT NULL DEFAULT 0,
		venue      TEXT NOT NULL DEFAULT '',
		source     TEXT NOT NULL DEFAULT '',
		doc_type   TEXT NOT NULL DEFAULT '',
		url        TEXT NOT NULL DEFAULT '',
		pmid       TEXT NOT NULL DEFAULT '',
		arxiv_id   TEXT NOT NULL DEFAULT '',
		citations  INTEGER NOT NULL DEFAULT 0,
		fetched_at TEXT NOT NULL
	)`); err != nil {
		_ = old.Close()
		t.Fatalf("create old papers: %v", err)
	}
	if err := old.Close(); err != nil {
		t.Fatalf("close old db: %v", err)
	}

	s, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer func() { _ = s.Close() }()

	for _, col := range []string{"volume", "number", "pages"} {
		var n int
		if err := s.db.QueryRow(
			"SELECT COUNT(*) FROM pragma_table_info('papers') WHERE name = ?", col,
		).Scan(&n); err != nil {
			t.Fatalf("查列 %s: %v", col, err)
		}
		if n != 1 {
			t.Fatalf("旧库迁移后应含列 %s，got %d", col, n)
		}
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
		if (c < 'a' || c > 'z') && (c < 'A' || c > 'Z') {
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

func TestUpsertPaper_SameTitleDifferentAuthors_Dedup(t *testing.T) {
	s := newTestStore(t)
	_, inserted1, err := s.UpsertPaper(paper("T", "", "Alice"))
	if err != nil {
		t.Fatalf("首次: %v", err)
	}
	if !inserted1 {
		t.Fatal("首次应 inserted=true")
	}
	// 同标题不同作者（无 DOI）：按归一化标题去重（与检索层 dedupPapers 一致）
	_, inserted2, err := s.UpsertPaper(paper("T", "", "Bob"))
	if err != nil {
		t.Fatalf("二次: %v", err)
	}
	if inserted2 {
		t.Fatal("同标题不同作者（无 DOI）应按标题去重，不应重复插入")
	}
}

func TestUpsertPaper_TitleWhitespaceNormalized_Dedup(t *testing.T) {
	s := newTestStore(t)
	_, inserted1, err := s.UpsertPaper(paper("Deep  Learning   Pain", "", "Lee"))
	if err != nil || !inserted1 {
		t.Fatalf("首次: inserted=%v err=%v", inserted1, err)
	}
	// 仅内部空白不同：归一化后应命中同 dedup_key（与内存去重 NormalizeTitle 一致）
	_, inserted2, err := s.UpsertPaper(paper("deep learning pain", "", "Lee"))
	if err != nil {
		t.Fatalf("二次: %v", err)
	}
	if inserted2 {
		t.Fatal("标题内部空白归一化后应去重，inserted=false")
	}
}

// TestUpsertPaper_RoundTripVolumeNumberPages 卷/期/页 round-trip：
// 入库带卷期页的论文，取回应一致；二次入库空值不擦除旧值。
func TestUpsertPaper_RoundTripVolumeNumberPages(t *testing.T) {
	s := newTestStore(t)
	p := paper("Vol", "10.1/vol")
	p.Volume = "42"
	p.Number = "3"
	p.Pages = "123-135"
	k, _, err := s.UpsertPaper(p)
	if err != nil {
		t.Fatalf("UpsertPaper: %v", err)
	}
	got, err := s.GetByCiteKey(k)
	if err != nil {
		t.Fatalf("GetByCiteKey: %v", err)
	}
	if got.Volume != "42" || got.Number != "3" || got.Pages != "123-135" {
		t.Fatalf("round-trip 后卷期页应一致，got %+v", got)
	}
	// UPDATE 路径：非空 volume 更新、空 number/pages 保留旧值
	p2 := paper("Vol", "10.1/vol")
	p2.Volume = "43"
	if _, _, err := s.UpsertPaper(p2); err != nil {
		t.Fatalf("二次 UpsertPaper: %v", err)
	}
	got, err = s.GetByCiteKey(k)
	if err != nil {
		t.Fatalf("GetByCiteKey: %v", err)
	}
	if got.Volume != "43" {
		t.Fatalf("非空 volume 应更新，got %q", got.Volume)
	}
	if got.Number != "3" || got.Pages != "123-135" {
		t.Fatalf("空 number/pages 不应擦除旧值，got %+v", got)
	}
}

func TestUpsertPaper_UpdateKeepsOldValueForEmptyFields(t *testing.T) {
	s := newTestStore(t)
	k, _, err := s.UpsertPaper(paper("Full", "10.1/full"))
	if err != nil {
		t.Fatalf("首次: %v", err)
	}
	// 再次入库：摘要为空、venue 非空
	p := paper("Full", "10.1/full")
	p.Abstract = ""
	p.Venue = "Nature"
	if _, _, err := s.UpsertPaper(p); err != nil {
		t.Fatalf("二次: %v", err)
	}
	got, err := s.GetByCiteKey(k)
	if err != nil {
		t.Fatalf("GetByCiteKey: %v", err)
	}
	if got.Abstract != "abs-Full" {
		t.Fatalf("空摘要不应擦除旧值，got %q", got.Abstract)
	}
	if got.Venue != "Nature" {
		t.Fatalf("非空 venue 应正常更新，got %q", got.Venue)
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
	papers, err := s.ListPapers("", "id", 10, 0)
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

func TestListPapers_OrderByYear(t *testing.T) {
	s := newTestStore(t)
	old := paper("Old", "10.1/old")
	old.Year = 2010
	recent := paper("Recent", "10.1/recent")
	recent.Year = 2024
	nodate := paper("NoDate", "10.1/nodate")
	// 入库顺序：old → recent → nodate
	for _, p := range []model.Paper{old, recent, nodate} {
		if _, _, err := s.UpsertPaper(p); err != nil {
			t.Fatalf("UpsertPaper: %v", err)
		}
	}
	papers, err := s.ListPapers("", "year", 10, 0)
	if err != nil {
		t.Fatalf("ListPapers: %v", err)
	}
	if len(papers) != 3 {
		t.Fatalf("应列出 3 篇，got %d", len(papers))
	}
	if papers[0].Title != "Recent" {
		t.Fatalf("年份倒序：最新年份应在最前，got %q (year=%d)", papers[0].Title, papers[0].Year)
	}
	if papers[2].Title != "NoDate" {
		t.Fatalf("年份倒序：year=0 应排末尾，got %q (year=%d)", papers[2].Title, papers[2].Year)
	}
}

func TestListPapers_BadSort(t *testing.T) {
	s := newTestStore(t)
	_, err := s.ListPapers("", "invalid", 10, 0)
	if err == nil {
		t.Fatal("无效排序应返回错误")
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
	papers, err := s.ListPapers("arxiv", "id", 10, 0)
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
	got, err := s.SearchLocal("neural", 10, 0)
	if err != nil {
		t.Fatalf("SearchLocal: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("关键词应命中 1 篇，got %d", len(got))
	}
	miss, err := s.SearchLocal("nonexistent-token", 10, 0)
	if err != nil {
		t.Fatalf("SearchLocal miss: %v", err)
	}
	if len(miss) != 0 {
		t.Fatalf("无关关键词应 0 命中，got %d", len(miss))
	}
}

// ---- 删除 ----

func TestForget_RemovesPaper(t *testing.T) {
	s := newTestStore(t)
	citeKey, _, err := s.UpsertPaper(paper("Doomed", "10.1/d"))
	if err != nil {
		t.Fatalf("UpsertPaper: %v", err)
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

// ---- 全文缓存（FR-FETCH-04）----

func TestFulltext_SetGet_ByCiteKey(t *testing.T) {
	s := newTestStore(t)
	k, _, err := s.UpsertPaper(paper("Full", "10.1/fulltext"))
	if err != nil {
		t.Fatalf("UpsertPaper: %v", err)
	}
	// 未设置时返回空
	if got, err := s.GetFulltext(k); err != nil || got != "" {
		t.Fatalf("初始应无全文，got %q err=%v", got, err)
	}
	text := "This is the full text of the paper.\nSecond paragraph."
	if err := s.SetFulltext(k, text); err != nil {
		t.Fatalf("SetFulltext: %v", err)
	}
	got, err := s.GetFulltext(k)
	if err != nil {
		t.Fatalf("GetFulltext: %v", err)
	}
	if got != text {
		t.Fatalf("全文 round-trip 应一致，got %q", got)
	}
}

func TestFulltext_UpdateOverwrites(t *testing.T) {
	s := newTestStore(t)
	k, _, err := s.UpsertPaper(paper("Full", "10.1/fulltext2"))
	if err != nil {
		t.Fatalf("UpsertPaper: %v", err)
	}
	if err := s.SetFulltext(k, "v1"); err != nil {
		t.Fatalf("SetFulltext v1: %v", err)
	}
	if err := s.SetFulltext(k, "v2"); err != nil {
		t.Fatalf("SetFulltext v2: %v", err)
	}
	got, _ := s.GetFulltext(k)
	if got != "v2" {
		t.Fatalf("全文应被覆盖为 v2，got %q", got)
	}
}

func TestFulltext_MissingCiteKey_ReturnsEmpty(t *testing.T) {
	s := newTestStore(t)
	got, err := s.GetFulltext("zzz")
	if err != nil {
		t.Fatalf("GetFulltext: %v", err)
	}
	if got != "" {
		t.Fatalf("未入库 cite_key 全文应为空，got %q", got)
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

// ---- paper_refs 引用标记 ----

func TestAddRef_InsertsIdempotent(t *testing.T) {
	s := newTestStore(t)
	k, _, err := s.UpsertPaper(paper("RefTest", "10.1/ref"))
	if err != nil {
		t.Fatalf("UpsertPaper: %v", err)
	}

	ref := model.PaperRef{CiteKey: k, SentenceHash: "abc123", Manuscript: "ch1.md", Sentence: "该方法[@Kxq]有效"}
	if err := s.AddRef(ref); err != nil {
		t.Fatalf("AddRef: %v", err)
	}
	// 同句幂等
	if err := s.AddRef(ref); err != nil {
		t.Fatalf("AddRef 二次: %v", err)
	}

	refs, err := s.GetRefsByManuscript("ch1.md")
	if err != nil {
		t.Fatalf("GetRefsByManuscript: %v", err)
	}
	if len(refs) != 1 {
		t.Fatalf("应只有 1 条引用标记，got %d", len(refs))
	}
	if refs[0].CiteKey != k || refs[0].SentenceHash != "abc123" {
		t.Fatalf("引用标记内容 round-trip 不一致，got %+v", refs[0])
	}
}

func TestAddRef_MultipleManuscripts(t *testing.T) {
	s := newTestStore(t)
	k, _, err := s.UpsertPaper(paper("MultiMs", "10.1/multi"))
	if err != nil {
		t.Fatalf("UpsertPaper: %v", err)
	}
	// 同一句 hash 出现在不同手稿 → 两条独立记录
	if err := s.AddRef(model.PaperRef{CiteKey: k, SentenceHash: "h1", Manuscript: "a.md", Sentence: "t1"}); err != nil {
		t.Fatalf("AddRef a.md: %v", err)
	}
	if err := s.AddRef(model.PaperRef{CiteKey: k, SentenceHash: "h1", Manuscript: "b.md", Sentence: "t1"}); err != nil {
		t.Fatalf("AddRef b.md: %v", err)
	}

	all, err := s.GetRefsByCiteKey(k)
	if err != nil {
		t.Fatalf("GetRefsByCiteKey: %v", err)
	}
	if len(all) != 2 {
		t.Fatalf("两个手稿应有 2 条引用标记，got %d", len(all))
	}
}

func TestRemoveRefsByManuscript_Scoped(t *testing.T) {
	s := newTestStore(t)
	k, _, err := s.UpsertPaper(paper("RemoveMs", "10.1/rm"))
	if err != nil {
		t.Fatalf("UpsertPaper: %v", err)
	}
	if err := s.AddRef(model.PaperRef{CiteKey: k, SentenceHash: "h1", Manuscript: "a.md", Sentence: "t1"}); err != nil {
		t.Fatalf("AddRef a.md: %v", err)
	}
	if err := s.AddRef(model.PaperRef{CiteKey: k, SentenceHash: "h2", Manuscript: "b.md", Sentence: "t2"}); err != nil {
		t.Fatalf("AddRef b.md: %v", err)
	}
	if err := s.RemoveRefsByManuscript("a.md"); err != nil {
		t.Fatalf("RemoveRefsByManuscript: %v", err)
	}
	refsA, _ := s.GetRefsByManuscript("a.md")
	if len(refsA) != 0 {
		t.Fatal("a.md 的引用标记应被删除")
	}
	refsB, _ := s.GetRefsByManuscript("b.md")
	if len(refsB) != 1 {
		t.Fatal("b.md 的引用标记应保留")
	}
}

func TestGetRefsByCiteKey_Empty(t *testing.T) {
	s := newTestStore(t)
	refs, err := s.GetRefsByCiteKey("zzz")
	if err != nil {
		t.Fatalf("GetRefsByCiteKey: %v", err)
	}
	if len(refs) != 0 {
		t.Fatalf("不存在的 cite_key 应返回空列表，got %d", len(refs))
	}
}

// ---- citation_scores 评分缓存 ----

func TestGetCitationScore_MissingReturnsNil(t *testing.T) {
	s := newTestStore(t)
	cs, err := s.GetCitationScore("zzz", "hash", "gpt-4o", "v1")
	if err != nil {
		t.Fatalf("GetCitationScore: %v", err)
	}
	if cs != nil {
		t.Fatal("未命中应返回 nil")
	}
}

func TestSaveAndGetCitationScore(t *testing.T) {
	s := newTestStore(t)
	k, _, err := s.UpsertPaper(paper("ScoreTest", "10.1/score"))
	if err != nil {
		t.Fatalf("UpsertPaper: %v", err)
	}

	cs := model.CitationScore{
		CiteKey:       k,
		SentenceHash:  "hash1",
		ModelID:       "gpt-4o",
		PromptVersion: "v1",
		Score:         0.85,
		Rationale:     "该方法与摘要一致",
	}
	if err := s.SaveCitationScore(cs); err != nil {
		t.Fatalf("SaveCitationScore: %v", err)
	}

	got, err := s.GetCitationScore(k, "hash1", "gpt-4o", "v1")
	if err != nil {
		t.Fatalf("GetCitationScore: %v", err)
	}
	if got == nil {
		t.Fatal("应命中评分缓存")
	}
	if got.Score != 0.85 || got.Rationale != "该方法与摘要一致" {
		t.Fatalf("评分 round-trip 不一致，got %+v", got)
	}
}

func TestCitationScore_KeyInvalidates(t *testing.T) {
	s := newTestStore(t)
	k, _, err := s.UpsertPaper(paper("KeyInv", "10.1/keyinv"))
	if err != nil {
		t.Fatalf("UpsertPaper: %v", err)
	}

	cs := model.CitationScore{CiteKey: k, SentenceHash: "h1", ModelID: "m1", PromptVersion: "v1", Score: 0.9}
	if err := s.SaveCitationScore(cs); err != nil {
		t.Fatalf("SaveCitationScore: %v", err)
	}

	// 同句同模型不同 prompt_version → 不命中
	miss, err := s.GetCitationScore(k, "h1", "m1", "v2")
	if err != nil {
		t.Fatalf("GetCitationScore: %v", err)
	}
	if miss != nil {
		t.Fatal("prompt_version 不同应不命中")
	}

	// 同句同 prompt 不同模型 → 不命中
	miss2, err := s.GetCitationScore(k, "h1", "m2", "v1")
	if err != nil {
		t.Fatalf("GetCitationScore: %v", err)
	}
	if miss2 != nil {
		t.Fatal("model_id 不同应不命中")
	}
}

// ---- 级联删除 ----

func TestForget_CascadesToRefsAndScores(t *testing.T) {
	s := newTestStore(t)
	k, _, err := s.UpsertPaper(paper("CascadeTest", "10.1/cascade"))
	if err != nil {
		t.Fatalf("UpsertPaper: %v", err)
	}
	if err := s.AddRef(model.PaperRef{CiteKey: k, SentenceHash: "h1", Manuscript: "ms.md", Sentence: "t"}); err != nil {
		t.Fatalf("AddRef: %v", err)
	}
	if err := s.SaveCitationScore(model.CitationScore{CiteKey: k, SentenceHash: "h1", ModelID: "m1", PromptVersion: "v1", Score: 0.5}); err != nil {
		t.Fatalf("SaveCitationScore: %v", err)
	}

	if _, err := s.Forget(k); err != nil {
		t.Fatalf("Forget: %v", err)
	}

	refs, _ := s.GetRefsByCiteKey(k)
	if len(refs) != 0 {
		t.Fatal("论文删除后 paper_refs 应级联删除")
	}
	cs, _ := s.GetCitationScore(k, "h1", "m1", "v1")
	if cs != nil {
		t.Fatal("论文删除后 citation_scores 应级联删除")
	}
}
