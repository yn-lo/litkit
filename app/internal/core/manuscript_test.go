package core

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"litkit/internal/model"
)

// fakeStore 手稿流水线的内存存储替身：UpsertPaper 模拟入库（无 citeKey 时生成一个）。
type fakeStore struct {
	papers map[string]model.Paper // citeKey → paper
	next   int
}

func newFakeStore() *fakeStore {
	return &fakeStore{papers: make(map[string]model.Paper)}
}

func (f *fakeStore) GetByCiteKey(citeKey string) (*model.Paper, error) {
	p, ok := f.papers[citeKey]
	if !ok {
		return nil, nil
	}
	cp := p
	return &cp, nil
}

func (f *fakeStore) UpsertPaper(p model.Paper) (string, bool, error) {
	if p.CiteKey == "" {
		f.next++
		p.CiteKey = fmt.Sprintf("auto%d", f.next)
	}
	f.papers[p.CiteKey] = p
	return p.CiteKey, true, nil
}

// mustPaper 构造带稳定 ID 的论文（ID 由 ComputeID 生成）。
func mustPaper(citeKey, title string, year int, authors []model.Author, doi string) model.Paper {
	p := model.Paper{CiteKey: citeKey, Title: title, Authors: authors, Year: year, DOI: doi, DocType: model.DocTypeArticle}
	p.ID = p.ComputeID()
	return p
}

var (
	pOne = mustPaper("a1", "Attention Is All You Need", 2017, []model.Author{{Given: "Ashish", Family: "Vaswani"}}, "10.1/one")
	pTwo = mustPaper("a2", "Deep Learning", 2015, []model.Author{{Given: "Yoshua", Family: "Bengio"}}, "10.2/two")
)

// process 解析并渲染手稿，返回正式样式结果。
func process(t *testing.T, store PaperStore, fetcher Fetcher, src, styleName string) *ManuscriptResult {
	t.Helper()
	resolved, err := ResolveManuscript(context.Background(), store, fetcher, src)
	if err != nil {
		t.Fatalf("ResolveManuscript err = %v", err)
	}
	res, err := FormatManuscript(resolved, mustStyle(t, "zh", styleName))
	if err != nil {
		t.Fatalf("FormatManuscript err = %v", err)
	}
	return res
}

func TestFormatManuscript_GB7714(t *testing.T) {
	store := newFakeStore()
	store.papers["a1"] = pOne
	store.papers["a2"] = pTwo

	res := process(t, store, nil, "前文 [@a1] 与 [@a2] 后文。", "gb7714-2025")
	if res.Text != "前文 [1] 与 [2] 后文。" {
		t.Errorf("Text = %q，期望 %q", res.Text, "前文 [1] 与 [2] 后文。")
	}
	if len(res.Papers) != 2 || res.Papers[0].CiteKey != "a1" || res.Papers[1].CiteKey != "a2" {
		t.Errorf("Papers 顺序错误：%+v", res.Papers)
	}
	if res.CitationMap["a1"] != "[1]" || res.CitationMap["a2"] != "[2]" {
		t.Errorf("CitationMap = %v，期望 {a1:[1] a2:[2]}", res.CitationMap)
	}
	if len(res.Unresolved) != 0 {
		t.Errorf("Unresolved 应为空：%v", res.Unresolved)
	}
	if len(res.Bibliography) != 2 {
		t.Errorf("Bibliography 应有 2 条，实际 %d", len(res.Bibliography))
	}
}

func TestFormatManuscript_DuplicateToken(t *testing.T) {
	store := newFakeStore()
	store.papers["a1"] = pOne

	res := process(t, store, nil, "[@a1] 与 [@a1]", "gb7714-2025")
	if res.Text != "[1] 与 [1]" {
		t.Errorf("Text = %q，期望 %q", res.Text, "[1] 与 [1]")
	}
	if len(res.Papers) != 1 {
		t.Errorf("Papers 应去重为 1 篇，实际 %d 篇", len(res.Papers))
	}
	if res.CitationMap["a1"] != "[1]" {
		t.Errorf("CitationMap = %v，期望 {a1:[1]}", res.CitationMap)
	}
}

func TestFormatManuscript_Unresolved(t *testing.T) {
	res := process(t, newFakeStore(), nil, "见 [@zzz]。", "gb7714-2025")
	if len(res.Unresolved) != 1 || res.Unresolved[0] != "zzz" {
		t.Errorf("Unresolved = %v，期望 [zzz]", res.Unresolved)
	}
	if res.Text != "见 [@zzz]。" {
		t.Errorf("Text = %q，期望保留原占位符不动", res.Text)
	}
	if len(res.Papers) != 0 || len(res.CitationMap) != 0 {
		t.Errorf("Papers/CitationMap 应为空：%v %v", res.Papers, res.CitationMap)
	}
}

func TestFormatManuscript_APAInline(t *testing.T) {
	store := newFakeStore()
	store.papers["v3"] = mustPaper("v3", "Attention", 2017, []model.Author{{Family: "Vaswani"}, {Family: "Shazeer"}, {Family: "Parmar"}}, "10.3/v3")
	store.papers["v2"] = mustPaper("v2", "Neural", 2017, []model.Author{{Family: "Vaswani"}, {Family: "Shazeer"}}, "10.4/v2")

	res := process(t, store, nil, "[@v3] 与 [@v2]", "apa")
	want := "(Vaswani et al., 2017) 与 (Vaswani & Shazeer, 2017)"
	if res.Text != want {
		t.Errorf("Text = %q，期望 %q", res.Text, want)
	}
}

func TestFormatManuscript_CaseInsensitiveCiteKey(t *testing.T) {
	store := newFakeStore()
	store.papers["ABC"] = mustPaper("ABC", "Alpha", 2020, []model.Author{{Family: "A"}}, "10.5/alpha")

	res := process(t, store, nil, "[@abc]", "gb7714-2025")
	if res.Text != "[1]" {
		t.Errorf("Text = %q，期望 %q", res.Text, "[1]")
	}
	if len(res.Unresolved) != 0 {
		t.Errorf("Unresolved = %v，期望空", res.Unresolved)
	}
}

func TestFormatPreview(t *testing.T) {
	store := newFakeStore()
	store.papers["a1"] = pOne // 有 DOI
	store.papers["a2"] = mustPaper("a2", "No DOI Paper", 2019, []model.Author{{Family: "Smith"}}, "")

	resolved, err := ResolveManuscript(context.Background(), store, nil, "前文 [@a1] 与 [@a2]。")
	if err != nil {
		t.Fatalf("ResolveManuscript err = %v", err)
	}
	res := FormatPreview(resolved)
	want := "前文 [@doi:10.1/one — Attention Is All You Need] 与 [@No DOI Paper]。"
	if res.Text != want {
		t.Errorf("Text = %q，期望 %q", res.Text, want)
	}
	if res.CitationMap["a1"] != "[@doi:10.1/one — Attention Is All You Need]" {
		t.Errorf("CitationMap[a1] = %q", res.CitationMap["a1"])
	}
	if len(res.Bibliography) != 2 {
		t.Errorf("preview 核对清单应有 2 条，实际 %d", len(res.Bibliography))
	}
}

func TestWriteManuscriptOutputs_AllStylesGetReferenceList(t *testing.T) {
	store := newFakeStore()
	store.papers["a1"] = pOne
	resolved, err := ResolveManuscript(context.Background(), store, nil, "[@a1]")
	if err != nil {
		t.Fatalf("ResolveManuscript err = %v", err)
	}
	res := FormatPreview(resolved)
	ts := "20260805_120000"
	files, err := WriteManuscriptOutputs(t.TempDir(), "manuscript", ts, res, "参考文献")
	if err != nil {
		t.Fatalf("WriteManuscriptOutputs err = %v", err)
	}
	path, ok := files[ManuscriptFormatted]
	if !ok {
		t.Fatalf("应生成 %s", ManuscriptFormatted)
	}
	b, _ := os.ReadFile(path)
	content := string(b)
	if !strings.Contains(content, "参考文献") {
		t.Errorf("preview 模式 formatted.md 也应含参考文献列表")
	}
	if !strings.Contains(content, "[1]") {
		t.Errorf("preview 模式 formatted.md 应含编号引用")
	}
	refsFile := filepath.Join(filepath.Dir(path), "manuscript_"+ts+".txt")
	if _, err := os.Stat(refsFile); err == nil {
		t.Errorf("references.txt 不应生成")
	}
}

// 英文模式的文末文献节标题应为 References 而非中文标题。
func TestFormattedHeadingEnglishStyles(t *testing.T) {
	store := newFakeStore()
	store.papers["a1"] = pOne

	for _, name := range []string{"apa", "ieee"} {
		resolved, err := ResolveManuscript(context.Background(), store, nil, "前文 [@a1] 后文。")
		if err != nil {
			t.Fatalf("%s: ResolveManuscript err = %v", name, err)
		}
		res, err := FormatManuscript(resolved, mustStyle(t, "en", name))
		if err != nil {
			t.Fatalf("%s: FormatManuscript err = %v", name, err)
		}
		files, err := WriteManuscriptOutputs(t.TempDir(), "manuscript", "20260924_120000", res, RefListHeading("en"))
		if err != nil {
			t.Fatalf("%s: WriteManuscriptOutputs err = %v", name, err)
		}
		b, _ := os.ReadFile(files[ManuscriptFormatted])
		content := string(b)
		if !strings.Contains(content, "# References") {
			t.Errorf("%s: formatted.md 应含 # References 标题，got:\n%s", name, content)
		}
		if strings.Contains(content, "参考文献") {
			t.Errorf("%s: 英文模式不应出现中文标题“参考文献”", name)
		}
	}
}

func TestWritePreviewOutput(t *testing.T) {
	store := newFakeStore()
	store.papers["a1"] = pOne
	store.papers["a2"] = mustPaper("a2", "No DOI Paper", 2019, []model.Author{{Family: "Smith"}}, "")

	resolved, err := ResolveManuscript(context.Background(), store, nil, "前文 [@a1] 与 [@a2]。")
	if err != nil {
		t.Fatalf("ResolveManuscript err = %v", err)
	}
	res := FormatPreview(resolved)
	ts := "20260830_120000"
	p, err := WritePreviewOutput(t.TempDir(), "manuscript", ts, res, "参考文献")
	if err != nil {
		t.Fatalf("WritePreviewOutput err = %v", err)
	}
	if base := filepath.Base(p); base != "manuscript_"+ts+".preview.md" {
		t.Errorf("preview 文件名 = %q，期望 %s", base, "manuscript_"+ts+".preview.md")
	}
	b, _ := os.ReadFile(p)
	content := string(b)
	if !strings.Contains(content, "[@doi:10.1/one") {
		t.Errorf("preview md 正文应含自描述标记，got:\n%s", content)
	}
	if !strings.Contains(content, "参考文献") {
		t.Errorf("preview md 文末应含参考文献列表，got:\n%s", content)
	}
}

// ---- 引用簇（相邻引用由 CSL 合并/排序）----

func TestFormatManuscript_ClusterConsecutive(t *testing.T) {
	store := newFakeStore()
	store.papers["a1"] = pOne
	store.papers["a2"] = pTwo
	store.papers["a3"] = mustPaper("a3", "Paper Three", 2019, []model.Author{{Family: "Wang"}}, "10.3/three")

	res := process(t, store, nil, "[@a1][@a2][@a3]", "gb7714-2025")
	if res.Text != "[1-3]" {
		t.Errorf("Text = %q, want [1-3]", res.Text)
	}
	if res.CitationMap["a1"] != "[1-3]" {
		t.Errorf("CitationMap[a1] = %q, want [1-3]", res.CitationMap["a1"])
	}
}

func TestFormatManuscript_ClusterNonConsecutive(t *testing.T) {
	store := newFakeStore()
	store.papers["a1"] = pOne
	store.papers["a2"] = pTwo
	store.papers["a3"] = mustPaper("a3", "Paper Three", 2019, []model.Author{{Family: "Wang"}}, "10.3/three")

	res := process(t, store, nil, "[@a1] x [@a2] x [@a1][@a3]", "gb7714-2025")
	want := "[1] x [2] x [1,3]"
	if res.Text != want {
		t.Errorf("Text = %q, want %q", res.Text, want)
	}
}

func TestFormatManuscript_ClusterMixed(t *testing.T) {
	store := newFakeStore()
	store.papers["a1"] = pOne
	store.papers["a2"] = pTwo
	store.papers["a3"] = mustPaper("a3", "Paper Three", 2019, []model.Author{{Family: "Wang"}}, "10.3/three")
	store.papers["a4"] = mustPaper("a4", "Paper Four", 2020, []model.Author{{Family: "Li"}}, "10.4/four")
	store.papers["a5"] = mustPaper("a5", "Paper Five", 2021, []model.Author{{Family: "Zhang"}}, "10.5/five")

	res := process(t, store, nil, "[@a1] x [@a2] x [@a1][@a3][@a4][@a5]", "gb7714-2025")
	want := "[1] x [2] x [1,3-5]"
	if res.Text != want {
		t.Errorf("Text = %q, want %q", res.Text, want)
	}
}

func TestFormatManuscript_ClusterSpaceSeparated(t *testing.T) {
	store := newFakeStore()
	store.papers["a1"] = pOne
	store.papers["a2"] = pTwo

	res := process(t, store, nil, "[@a1] [@a2]", "gb7714-2025")
	if res.Text != "[1,2]" {
		t.Errorf("Text = %q, want [1,2]", res.Text)
	}
}

func TestFormatManuscript_ClusterUnresolvedBreaksGroup(t *testing.T) {
	store := newFakeStore()
	store.papers["a1"] = pOne
	store.papers["a2"] = pTwo

	res := process(t, store, nil, "[@a1][@zzz][@a2]", "gb7714-2025")
	want := "[1][@zzz][2]"
	if res.Text != want {
		t.Errorf("Text = %q, want %q", res.Text, want)
	}
}

func TestFormatManuscript_ClusterIEEE(t *testing.T) {
	store := newFakeStore()
	store.papers["a1"] = pOne
	store.papers["a2"] = pTwo
	store.papers["a3"] = mustPaper("a3", "Paper Three", 2019, []model.Author{{Family: "Wang"}}, "10.3/three")

	res := process(t, store, nil, "[@a1][@a2][@a3]", "ieee")
	if res.Text != "[1], [2], [3]" {
		t.Errorf("Text = %q, want %q", res.Text, "[1], [2], [3]")
	}
}

func TestFormatManuscript_ClusterAPA(t *testing.T) {
	store := newFakeStore()
	store.papers["a1"] = pOne
	store.papers["a2"] = pTwo

	res := process(t, store, nil, "[@a1][@a2]", "apa")
	// APA 会按作者字母序重排同一引用簇。
	want := "(Bengio, 2015; Vaswani, 2017)"
	if res.Text != want {
		t.Errorf("Text = %q, want %q", res.Text, want)
	}
}

func TestResolveManuscript_NilStore(t *testing.T) {
	if _, err := ResolveManuscript(context.Background(), nil, nil, "[@a1]"); err == nil {
		t.Fatal("nil store 应返回错误而不是 panic")
	}
}

func TestFormatManuscript_ClusterDuplicateToken(t *testing.T) {
	store := newFakeStore()
	store.papers["a1"] = pOne

	res := process(t, store, nil, "[@a1] [@a1]", "gb7714-2025")
	if res.Text != "[1]" {
		t.Errorf("Text = %q，同一论文的重复引用应去重为 [1]", res.Text)
	}
}

// fakeFetcher 可配置反查结果与错误的 Fetcher 替身。
type fakeFetcher struct {
	paper  *model.Paper
	err    error
	called int
}

func (f *fakeFetcher) Fetch(_ context.Context, idType, identifier string) (*model.Paper, error) {
	f.called++
	if f.err != nil {
		return nil, f.err
	}
	return f.paper, nil
}

// fakeErrStore 在 GetByCiteKey / UpsertPaper 上返回可配置错误的存储替身。
type fakeErrStore struct {
	*fakeStore
	getErr    error
	upsertErr error
}

func (f *fakeErrStore) GetByCiteKey(citeKey string) (*model.Paper, error) {
	if f.getErr != nil {
		return nil, f.getErr
	}
	return f.fakeStore.GetByCiteKey(citeKey)
}

func (f *fakeErrStore) UpsertPaper(p model.Paper) (string, bool, error) {
	if f.upsertErr != nil {
		return "", false, f.upsertErr
	}
	return f.fakeStore.UpsertPaper(p)
}

func TestResolveManuscript_PropagatesRuntimeErrors(t *testing.T) {
	ctx := context.Background()
	cases := []struct {
		name  string
		store PaperStore
		fetch Fetcher
		src   string
	}{
		{
			name:  "GetByCiteKey 错误",
			store: &fakeErrStore{fakeStore: newFakeStore(), getErr: errors.New("db locked")},
			fetch: nil,
			src:   "[@a1]",
		},
		{
			name:  "Fetch 反查错误",
			store: newFakeStore(),
			fetch: &fakeFetcher{err: errors.New("network down")},
			src:   "[@doi:10.1/x]",
		},
		{
			name:  "UpsertPaper 错误",
			store: &fakeErrStore{fakeStore: newFakeStore(), upsertErr: errors.New("disk full")},
			fetch: &fakeFetcher{paper: &pOne},
			src:   "[@doi:10.1/one]",
		},
	}
	for _, tc := range cases {
		_, err := ResolveManuscript(ctx, tc.store, tc.fetch, tc.src)
		if err == nil {
			t.Errorf("%s: 运行错误应向上返回，而不是记为 Unresolved", tc.name)
		}
	}
}

func TestResolveManuscript_FetchMissIsUnresolved(t *testing.T) {
	// 反查返回 (nil, nil)（未命中）仍应记入 Unresolved 而不报错
	store := newFakeStore()
	resolved, err := ResolveManuscript(context.Background(), store, &fakeFetcher{}, "[@doi:10.1/none]")
	if err != nil {
		t.Fatalf("未命中不应报错: %v", err)
	}
	if len(resolved.Unresolved) != 1 || resolved.Unresolved[0] != "doi:10.1/none" {
		t.Errorf("Unresolved = %v, want [doi:10.1/none]", resolved.Unresolved)
	}
}

func TestExpandCiteGroups_PrefixedTokenKeepsComma(t *testing.T) {
	// 带前缀 token 内的逗号（含于标题/DOI）不应被分组拆分
	src := "[@a1][@title:Smith, J. 2024]"
	got := expandCiteGroups(src)
	if !strings.Contains(got, "[@title:Smith, J. 2024]") {
		t.Errorf("带前缀 token 应保持原样，got %q", got)
	}
	if strings.Contains(got, "[@ Smith") {
		t.Errorf("带前缀 token 不应被逗号拆分，got %q", got)
	}
	// 裸 token 分组行为不变（bodyCiteRe 只匹配 3-5 个纯字母的裸 citeKey）
	got = expandCiteGroups("[@abc,def]")
	if !strings.Contains(got, "[@abc]") || !strings.Contains(got, "[@def]") {
		t.Errorf("裸 token 分组应拆分，got %q", got)
	}
}
