package core

import (
	"context"
	"errors"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"litkit/internal/model"
	"litkit/internal/sources"
	"litkit/internal/storage"
)

// ---- 测试用 fake source ----

// fakeSource 计数 + 可注入返回值/错误的 PaperSource 实现。
type fakeSource struct {
	name   string
	papers []model.Paper
	err    error
	calls  int32 // Search 调用次数
	delay  time.Duration
}

func (f *fakeSource) Name() string      { return f.name }
func (f *fakeSource) HasAbstract() bool { return true }
func (f *fakeSource) Search(ctx context.Context, query string, opts sources.SearchOptions) ([]model.Paper, error) {
	atomic.AddInt32(&f.calls, 1)
	if f.delay > 0 {
		select {
		case <-time.After(f.delay):
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	if f.err != nil {
		return nil, f.err
	}
	// 返回副本，避免被调用方 mutate
	out := make([]model.Paper, len(f.papers))
	copy(out, f.papers)
	return out, nil
}

func (f *fakeSource) Calls() int { return int(atomic.LoadInt32(&f.calls)) }

func newFakeRegistry(srcs ...sources.PaperSource) *sources.Registry {
	r := sources.NewRegistry()
	for _, s := range srcs {
		r.Register(s)
	}
	return r
}

func paperWithAbstract(title, doi, abstract string) model.Paper {
	p := model.Paper{Title: title, DOI: doi, Abstract: abstract, Source: "fake"}
	p.ID = p.ComputeID()
	return p
}

func paperNoAbstract(title, doi string) model.Paper {
	p := model.Paper{Title: title, DOI: doi, Source: "fake"} // 无 Abstract
	p.ID = p.ComputeID()
	return p
}

// ---- SearchOptions ----

func TestSearchOptions_Defaults(t *testing.T) {
	o := SearchOptions{}
	o.applyDefaults(5)
	if o.MaxResults != 5 {
		t.Fatalf("默认 MaxResults 应为 5，got %d", o.MaxResults)
	}
}

// ---- 并发多源 ----

func TestSearch_ConcurrentMultiSource(t *testing.T) {
	srcA := &fakeSource{name: "alpha", papers: []model.Paper{paperWithAbstract("A1", "10.1/a1", "abs-a1")}}
	srcB := &fakeSource{name: "beta", papers: []model.Paper{paperWithAbstract("B1", "10.1/b1", "abs-b1")}}
	reg := newFakeRegistry(srcA, srcB)
	s := NewSearcher(reg, nil, 0)

	res, err := s.Search(context.Background(), "query", SearchOptions{MaxResults: 5})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(res.Papers) != 2 {
		t.Fatalf("应返回 2 篇去重后论文，got %d", len(res.Papers))
	}
	if len(res.SourceResults["alpha"]) != 1 || len(res.SourceResults["beta"]) != 1 {
		t.Fatalf("SourceResults 应分源保留： %+v", res.SourceResults)
	}
	if len(res.Errors) != 0 {
		t.Fatalf("不应有错误： %+v", res.Errors)
	}
	if srcA.Calls() != 1 || srcB.Calls() != 1 {
		t.Fatalf("每源应仅调用 1 次： alpha=%d beta=%d", srcA.Calls(), srcB.Calls())
	}
}

// ---- 单源失败隔离 ----

func TestSearch_SingleSourceFailureIsolation(t *testing.T) {
	srcOK := &fakeSource{name: "ok", papers: []model.Paper{paperWithAbstract("OK", "10.1/ok", "abs")}}
	srcBad := &fakeSource{name: "bad", err: errors.New("upstream 503")}
	reg := newFakeRegistry(srcOK, srcBad)
	s := NewSearcher(reg, nil, 0)

	res, err := s.Search(context.Background(), "q", SearchOptions{MaxResults: 5})
	if err != nil {
		t.Fatalf("单源失败不应导致整体 error： %v", err)
	}
	if len(res.Papers) != 1 {
		t.Fatalf("应仍返回 1 篇可用论文，got %d", len(res.Papers))
	}
	if len(res.Errors) != 1 || res.Errors[0].Source != "bad" {
		t.Fatalf("错误应归入 Errors： %+v", res.Errors)
	}
}

// ---- 三级去重 ----

func TestSearch_DedupByDOI(t *testing.T) {
	// 两个源返回同 DOI（标题可不同），应合并为一条
	srcA := &fakeSource{name: "alpha", papers: []model.Paper{
		paperWithAbstract("Graph Networks A", "10.1/x", "abs-from-alpha"),
	}}
	srcB := &fakeSource{name: "beta", papers: []model.Paper{
		paperWithAbstract("Graph Networks B", "10.1/x", "abs-from-beta"),
	}}
	reg := newFakeRegistry(srcA, srcB)
	s := NewSearcher(reg, nil, 0)

	res, _ := s.Search(context.Background(), "q", SearchOptions{MaxResults: 5})
	if len(res.Papers) != 1 {
		t.Fatalf("同 DOI 应去重为 1 条，got %d", len(res.Papers))
	}
	// 合并后 DOI 必须保留
	if res.Papers[0].DOI != "10.1/x" {
		t.Fatalf("去重后 DOI 应保留，got %q", res.Papers[0].DOI)
	}
}

func TestSearch_DedupByTitleWhenNoDOI(t *testing.T) {
	// 两个源返回同标题（无 DOI），应合并为一条
	srcA := &fakeSource{name: "alpha", papers: []model.Paper{
		{Title: "Same Title", Abstract: "abs-a", Source: "alpha"},
	}}
	srcB := &fakeSource{name: "beta", papers: []model.Paper{
		{Title: "  Same  Title  ", Abstract: "abs-b", Source: "beta"}, // 归一化后应相同
	}}
	reg := newFakeRegistry(srcA, srcB)
	s := NewSearcher(reg, nil, 0)

	res, _ := s.Search(context.Background(), "q", SearchOptions{MaxResults: 5})
	if len(res.Papers) != 1 {
		t.Fatalf("同标题（无 DOI）应去重为 1 条，got %d", len(res.Papers))
	}
}

// ---- 无摘要过滤 ----

func TestSearch_NoAbstractFilteredByDefault(t *testing.T) {
	src := &fakeSource{name: "alpha", papers: []model.Paper{
		paperWithAbstract("Has", "10.1/has", "abs"),
		paperNoAbstract("NoAbs", "10.1/noabs"),
	}}
	reg := newFakeRegistry(src)
	s := NewSearcher(reg, nil, 0)

	res, _ := s.Search(context.Background(), "q", SearchOptions{MaxResults: 5})
	if len(res.Papers) != 1 {
		t.Fatalf("无摘要默认过滤后应剩 1 篇，got %d", len(res.Papers))
	}
	if res.Papers[0].Title != "Has" {
		t.Fatalf("保留的应是有摘要的，got %q", res.Papers[0].Title)
	}
}

func TestSearch_KeepNoAbstractFlag(t *testing.T) {
	src := &fakeSource{name: "alpha", papers: []model.Paper{
		paperWithAbstract("Has", "10.1/has", "abs"),
		paperNoAbstract("NoAbs", "10.1/noabs"),
	}}
	reg := newFakeRegistry(src)
	s := NewSearcher(reg, nil, 0)

	res, _ := s.Search(context.Background(), "q", SearchOptions{MaxResults: 5, KeepNoAbstract: true})
	if len(res.Papers) != 2 {
		t.Fatalf("KeepNoAbstract 应保留全部 2 篇，got %d", len(res.Papers))
	}
}

// ---- 源过滤 ----

func TestSearch_SourcesFilter(t *testing.T) {
	srcA := &fakeSource{name: "alpha", papers: []model.Paper{paperWithAbstract("A", "10.1/a", "abs")}}
	srcB := &fakeSource{name: "beta", papers: []model.Paper{paperWithAbstract("B", "10.1/b", "abs")}}
	reg := newFakeRegistry(srcA, srcB)
	s := NewSearcher(reg, nil, 0)

	res, _ := s.Search(context.Background(), "q", SearchOptions{Sources: []string{"alpha"}, MaxResults: 5})
	if len(res.Papers) != 1 || res.Papers[0].Title != "A" {
		t.Fatalf("源过滤后应仅 alpha 的 A： %+v", res.Papers)
	}
	if srcB.Calls() != 0 {
		t.Fatalf("未指定的源不应被调用，beta calls=%d", srcB.Calls())
	}
}

func TestSearch_SourcesFilter_UnknownIgnored(t *testing.T) {
	srcA := &fakeSource{name: "alpha", papers: []model.Paper{paperWithAbstract("A", "10.1/a", "abs")}}
	reg := newFakeRegistry(srcA)
	s := NewSearcher(reg, nil, 0)

	res, err := s.Search(context.Background(), "q", SearchOptions{Sources: []string{"alpha", "unknown"}, MaxResults: 5})
	if err != nil {
		t.Fatalf("未知源不应导致整体 error： %v", err)
	}
	if len(res.Papers) != 1 {
		t.Fatalf("应仅返回 alpha 的 1 篇，got %d", len(res.Papers))
	}
}

// ---- 检索结果入库 ----

func TestSearch_UpsertsPapersIntoStore(t *testing.T) {
	srcA := &fakeSource{name: "alpha", papers: []model.Paper{paperWithAbstract("A1", "10.1/a1", "abs")}}
	reg := newFakeRegistry(srcA)
	store, err := storage.Open(filepath.Join(t.TempDir(), "litkit.db"))
	if err != nil {
		t.Fatalf("Open store: %v", err)
	}
	defer store.Close()
	s := NewSearcher(reg, store, 0)

	res1, _ := s.Search(context.Background(), "query", SearchOptions{MaxResults: 5})
	if len(res1.Papers) != 1 {
		t.Fatalf("应返回 1 篇，got %d", len(res1.Papers))
	}
	if len(res1.Papers[0].CiteKey) != 3 {
		t.Fatalf("入库论文应回填 3 字母 cite_key，got %q", res1.Papers[0].CiteKey)
	}
	// 库中应能查到
	got, err := store.GetByCiteKey(res1.Papers[0].CiteKey)
	if err != nil || got == nil {
		t.Fatalf("库中应能按 cite_key 查到：got=%+v err=%v", got, err)
	}
	// 重复检索（同 DOI）不重复入库
	n, err := store.Stats()
	if err != nil {
		t.Fatalf("Stats: %v", err)
	}
	if n.Total != 1 {
		t.Fatalf("重复入库应去重为 1 篇，got %d", n.Total)
	}
}

func TestSearch_StoreNil_NoUpsert(t *testing.T) {
	srcA := &fakeSource{name: "alpha", papers: []model.Paper{paperWithAbstract("A1", "10.1/a1", "abs")}}
	reg := newFakeRegistry(srcA)
	s := NewSearcher(reg, nil, 0)
	res, _ := s.Search(context.Background(), "query", SearchOptions{MaxResults: 5})
	if len(res.Papers) != 1 {
		t.Fatalf("store=nil 时应正常返回 1 篇，got %d", len(res.Papers))
	}
	if res.Papers[0].CiteKey != "" {
		t.Fatalf("store=nil 时不应有 cite_key，got %q", res.Papers[0].CiteKey)
	}
}

func TestSearch_KeepNoAbstract_NoUpsert(t *testing.T) {
	srcA := &fakeSource{name: "alpha", papers: []model.Paper{paperNoAbstract("NoAbs", "10.1/noabs")}}
	reg := newFakeRegistry(srcA)
	store, err := storage.Open(filepath.Join(t.TempDir(), "litkit.db"))
	if err != nil {
		t.Fatalf("Open store: %v", err)
	}
	defer store.Close()
	s := NewSearcher(reg, store, 0)

	res, _ := s.Search(context.Background(), "q", SearchOptions{MaxResults: 5, KeepNoAbstract: true})
	if len(res.Papers) != 1 {
		t.Fatalf("KeepNoAbstract 应保留 1 篇，got %d", len(res.Papers))
	}
	st, _ := store.Stats()
	if st.Total != 0 {
		t.Fatalf("无摘要论文不应入库（FR-LIB-01），got %d", st.Total)
	}
}

// ---- 上下文取消 ----

func TestSearch_ContextCancelIsolatesErrors(t *testing.T) {
	// 慢源 + 取消 ctx：错误归入 Errors，不 panic
	srcSlow := &fakeSource{
		name:   "slow",
		delay:  200 * time.Millisecond,
		papers: []model.Paper{paperWithAbstract("S", "10.1/s", "abs")},
	}
	reg := newFakeRegistry(srcSlow)
	s := NewSearcher(reg, nil, 0)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	res, err := s.Search(ctx, "q", SearchOptions{MaxResults: 5})
	if err != nil {
		t.Fatalf("ctx 取消不应整体 error： %v", err)
	}
	if len(res.Errors) != 1 {
		t.Fatalf("慢源失败应归入 1 条 error，got %d", len(res.Errors))
	}
}

// ---- 空 registry ----

func TestSearch_EmptyRegistry(t *testing.T) {
	reg := sources.NewRegistry()
	s := NewSearcher(reg, nil, 0)
	res, err := s.Search(context.Background(), "q", SearchOptions{MaxResults: 5})
	if err != nil {
		t.Fatalf("空 registry 不应 error： %v", err)
	}
	if res == nil {
		t.Fatal("应返回非 nil 结果")
	}
	if len(res.Papers) != 0 || res.Total != 0 {
		t.Fatalf("空结果应 0 篇，got total=%d papers=%d", res.Total, len(res.Papers))
	}
}

// ---- 默认年份倒序排序（FR-SEARCH-10）----

func TestSearch_SortedByYearDesc(t *testing.T) {
	src := &fakeSource{name: "alpha", papers: []model.Paper{
		{Title: "Old", Abstract: "a", Source: "alpha", Year: 2018},
		{Title: "Newest", Abstract: "a", Source: "alpha", Year: 2024},
		{Title: "Mid", Abstract: "a", Source: "alpha", Year: 2021},
		{Title: "Unknown", Abstract: "a", Source: "alpha", Year: 0},
	}}
	reg := newFakeRegistry(src)
	s := NewSearcher(reg, nil, 0)

	res, _ := s.Search(context.Background(), "q", SearchOptions{MaxResults: 5})
	if len(res.Papers) != 4 {
		t.Fatalf("应返回 4 篇，got %d", len(res.Papers))
	}
	// 期望：2024 > 2021 > 2018 > 0
	years := []int{res.Papers[0].Year, res.Papers[1].Year, res.Papers[2].Year, res.Papers[3].Year}
	want := []int{2024, 2021, 2018, 0}
	for i := range want {
		if years[i] != want[i] {
			t.Fatalf("排序不符，got %v want %v", years, want)
		}
	}
}

// ---- mergePapers 行为 ----

func TestMergePapers_PrefersNonEmptyFields(t *testing.T) {
	a := model.Paper{Title: "T", DOI: "10.1/x", Abstract: "", Year: 2020, Source: "alpha"}
	b := model.Paper{Title: "T", DOI: "10.1/x", Abstract: "filled", Year: 0, Source: "beta"}
	merged := mergePapers(a, b)
	if merged.Abstract != "filled" {
		t.Fatalf("应取非空 Abstract，got %q", merged.Abstract)
	}
	if merged.Year != 2020 {
		t.Fatalf("应取非空 Year，got %d", merged.Year)
	}
}
