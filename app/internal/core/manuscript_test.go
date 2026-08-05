package core

import (
	"context"
	"fmt"
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
	p := model.Paper{CiteKey: citeKey, Title: title, Authors: authors, Year: year, DOI: doi}
	p.ID = p.ComputeID()
	return p
}

var (
	pOne = mustPaper("a1", "Attention Is All You Need", 2017, []model.Author{{Given: "Ashish", Family: "Vaswani"}}, "10.1/one")
	pTwo = mustPaper("a2", "Deep Learning", 2015, []model.Author{{Given: "Yoshua", Family: "Bengio"}}, "10.2/two")
)

func TestProcessManuscript_GB7714(t *testing.T) {
	store := newFakeStore()
	store.papers["a1"] = pOne
	store.papers["a2"] = pTwo

	res, err := ProcessManuscript(context.Background(), store, nil, "前文 [@a1] 与 [@a2] 后文。", StyleGB7714)
	if err != nil {
		t.Fatalf("ProcessManuscript err = %v", err)
	}
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
}

func TestProcessManuscript_DuplicateToken(t *testing.T) {
	store := newFakeStore()
	store.papers["a1"] = pOne

	res, err := ProcessManuscript(context.Background(), store, nil, "[@a1] 与 [@a1]", StyleGB7714)
	if err != nil {
		t.Fatalf("ProcessManuscript err = %v", err)
	}
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

func TestProcessManuscript_Unresolved(t *testing.T) {
	store := newFakeStore()

	res, err := ProcessManuscript(context.Background(), store, nil, "见 [@zzz]。", StyleGB7714)
	if err != nil {
		t.Fatalf("ProcessManuscript err = %v", err)
	}
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

func TestProcessManuscript_APAInline(t *testing.T) {
	store := newFakeStore()
	store.papers["v3"] = mustPaper("v3", "Attention", 2017, []model.Author{{Family: "Vaswani"}, {Family: "Shazeer"}, {Family: "Parmar"}}, "10.3/v3")
	store.papers["v2"] = mustPaper("v2", "Neural", 2017, []model.Author{{Family: "Vaswani"}, {Family: "Shazeer"}}, "10.4/v2")

	res, err := ProcessManuscript(context.Background(), store, nil, "[@v3] 与 [@v2]", StyleAPA)
	if err != nil {
		t.Fatalf("ProcessManuscript err = %v", err)
	}
	if res.Text != "(Vaswani et al., 2017) 与 (Vaswani & Shazeer, 2017)" {
		t.Errorf("Text = %q，期望 %q", res.Text, "(Vaswani et al., 2017) 与 (Vaswani & Shazeer, 2017)")
	}
}

func TestProcessManuscript_UnknownStyle(t *testing.T) {
	store := newFakeStore()
	_, err := ProcessManuscript(context.Background(), store, nil, "x", Style("csl"))
	if err == nil {
		t.Errorf("未知 style 应返回 error")
	}
}

func TestProcessManuscript_CaseInsensitiveCiteKey(t *testing.T) {
	store := newFakeStore()
	store.papers["ABC"] = mustPaper("ABC", "Alpha", 2020, []model.Author{{Family: "A"}}, "10.5/alpha")

	res, err := ProcessManuscript(context.Background(), store, nil, "[@abc]", StyleGB7714)
	if err != nil {
		t.Fatalf("ProcessManuscript err = %v", err)
	}
	if res.Text != "[1]" {
		t.Errorf("Text = %q，期望 %q", res.Text, "[1]")
	}
	if len(res.Unresolved) != 0 {
		t.Errorf("Unresolved = %v，期望空", res.Unresolved)
	}
}

func TestProcessManuscript_Preview(t *testing.T) {
	store := newFakeStore()
	store.papers["a1"] = pOne // 有 DOI
	store.papers["a2"] = mustPaper("a2", "No DOI Paper", 2019, []model.Author{{Family: "Smith"}}, "")

	res, err := ProcessManuscript(context.Background(), store, nil, "前文 [@a1] 与 [@a2]。", StylePreview)
	if err != nil {
		t.Fatalf("ProcessManuscript err = %v", err)
	}
	want := "前文 [@doi:10.1/one — Attention Is All You Need] 与 [@No DOI Paper]。"
	if res.Text != want {
		t.Errorf("Text = %q，期望 %q", res.Text, want)
	}
	if res.CitationMap["a1"] != "[@doi:10.1/one — Attention Is All You Need]" {
		t.Errorf("CitationMap[a1] = %q", res.CitationMap["a1"])
	}
}

func TestWriteManuscriptOutputs_PreviewSkipsReferences(t *testing.T) {
	store := newFakeStore()
	store.papers["a1"] = pOne
	res, err := ProcessManuscript(context.Background(), store, nil, "[@a1]", StylePreview)
	if err != nil {
		t.Fatalf("ProcessManuscript err = %v", err)
	}
	files, err := WriteManuscriptOutputs(t.TempDir(), res, StylePreview)
	if err != nil {
		t.Fatalf("WriteManuscriptOutputs err = %v", err)
	}
	if _, ok := files[ManuscriptReferences]; ok {
		t.Errorf("预览模式不应生成 %s", ManuscriptReferences)
	}
	if _, ok := files[ManuscriptFormatted]; !ok {
		t.Errorf("预览模式应生成 %s", ManuscriptFormatted)
	}
}
