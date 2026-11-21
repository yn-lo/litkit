package csl

import (
	"strings"
	"testing"

	"litkit/internal/model"
)

func testPapers() []model.Paper {
	return []model.Paper{
		{
			CiteKey: "Vaswani2017",
			Title:   "Attention Is All You Need",
			Authors: []model.Author{{Given: "Ashish", Family: "Vaswani"}, {Given: "Noam", Family: "Shazeer"}, {Given: "Niki", Family: "Parmar"}, {Given: "Jakob", Family: "Uszkoreit"}},
			Year:    2017, Venue: "Advances in Neural Information Processing Systems",
			DOI: "10.5555/3295222.3295349", DocType: model.DocTypeArticle,
			Volume: "30", Pages: "5998-6008",
		},
		{
			CiteKey: "Wang2024",
			Title:   "大语言模型在学术写作中的应用研究",
			Authors: []model.Author{{Given: "小明", Family: "王"}, {Given: "丽", Family: "李"}, {Given: "强", Family: "张"}, {Given: "华", Family: "刘"}},
			Year:    2024, Venue: "情报学报", DOI: "10.1234/abc.2024.001",
			DocType: model.DocTypeJournalArticle, Volume: "43", Number: "2", Pages: "123-135",
		},
	}
}

func mustBuiltin(t *testing.T, name string) *Style {
	t.Helper()
	st, err := builtinStyle(name)
	if err != nil {
		t.Fatalf("builtinStyle(%s) err = %v", name, err)
	}
	return st
}

func TestRenderBibliography_GB7714(t *testing.T) {
	entries, err := RenderBibliography(testPapers(), mustBuiltin(t, "gb7714-2025"))
	if err != nil {
		t.Fatalf("RenderBibliography err = %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("期望 2 条，得到 %d", len(entries))
	}
	all := strings.Join(entries, "\n")
	for _, want := range []string{"[1]", "Vaswani A", "et al.", "Attention Is All You Need[J/OL]", "DOI:10.5555/3295222.3295349", "[2]", "王小明", "等", "43（2）"} {
		if !strings.Contains(all, want) {
			t.Errorf("GB7714 缺少 %q：\n%s", want, all)
		}
	}
}

func TestRenderBibliography_APA(t *testing.T) {
	entries, err := RenderBibliography(testPapers(), mustBuiltin(t, "apa"))
	if err != nil {
		t.Fatalf("RenderBibliography err = %v", err)
	}
	all := strings.Join(entries, "\n")
	for _, want := range []string{"Vaswani, A.", "(2017)", "Attention Is All You Need.", "Advances in Neural Information Processing Systems, 30, 5998–6008", "https://doi.org/10.5555/3295222.3295349"} {
		if !strings.Contains(all, want) {
			t.Errorf("APA 缺少 %q：\n%s", want, all)
		}
	}
}

func TestRenderBibliography_IEEE(t *testing.T) {
	entries, err := RenderBibliography(testPapers(), mustBuiltin(t, "ieee"))
	if err != nil {
		t.Fatalf("RenderBibliography err = %v", err)
	}
	all := strings.Join(entries, "\n")
	for _, want := range []string{"[1] A. Vaswani", "“Attention Is All You Need,”", "vol. 30", "pp. 5998–6008", "doi: 10.5555/3295222.3295349"} {
		if !strings.Contains(all, want) {
			t.Errorf("IEEE 缺少 %q：\n%s", want, all)
		}
	}
}

func TestCitationClusters(t *testing.T) {
	cases := []struct {
		style string
		want  []string
	}{
		{"gb7714-2025", []string{"[1]", "[2]", "[1-3]"}},
		{"ieee", []string{"[1]", "[2]", "[1], [2], [3]"}},
		{"apa", []string{"(Vaswani et al., 2017)", "(王 et al., 2024)", "(Lee, 2020; Vaswani et al., 2017; 王 et al., 2024)"}},
	}
	for _, tc := range cases {
		t.Run(tc.style, func(t *testing.T) {
			papers := append(testPapers(), model.Paper{
				CiteKey: "Lee2020", Title: "Third Study", Year: 2020, Venue: "Journal of Tests",
				Authors: []model.Author{{Given: "Kai", Family: "Lee"}}, DocType: model.DocTypeArticle,
			})
			r, err := NewRenderer(mustBuiltin(t, tc.style))
			if err != nil {
				t.Fatal(err)
			}
			if err := r.Add(papers); err != nil {
				t.Fatal(err)
			}
			first, err := r.Cite("Vaswani2017")
			if err != nil {
				t.Fatal(err)
			}
			second, err := r.Cite("Wang2024")
			if err != nil {
				t.Fatal(err)
			}
			third, err := r.Cite("Vaswani2017", "Wang2024", "Lee2020")
			if err != nil {
				t.Fatal(err)
			}
			got := []string{first, second, third}
			for i, want := range tc.want {
				if got[i] != want {
					t.Errorf("mark[%d] = %q，期望 %q", i, got[i], want)
				}
			}
		})
	}
}

// TestEngineSessionIsolation 回归：多个会话不能共享 citation 状态。
func TestEngineSessionIsolation(t *testing.T) {
	a, err := NewEngine(mustBuiltin(t, "ieee").Content)
	if err != nil {
		t.Fatal(err)
	}
	pa := ItemFromPaper(testPapers()[0])
	if err := a.Add(pa); err != nil {
		t.Fatal(err)
	}
	if _, err := a.Cite(pa); err != nil {
		t.Fatal(err)
	}

	b, err := NewEngine(mustBuiltin(t, "ieee").Content)
	if err != nil {
		t.Fatal(err)
	}
	pb := ItemFromPaper(testPapers()[1])
	if err := b.Add(pb); err != nil {
		t.Fatal(err)
	}
	mark, err := b.Cite(pb)
	if err != nil {
		t.Fatalf("第二个会话被第一个会话的状态污染: %v", err)
	}
	if mark != "[1]" {
		t.Errorf("第二个会话 mark = %q，期望 [1]", mark)
	}
}

func TestNewEngine_InvalidCSL(t *testing.T) {
	if _, err := NewEngine([]byte("not a csl")); err == nil {
		t.Fatal("非法 CSL 应返回错误")
	}
}

func TestEntryTextPreservesAngleBrackets(t *testing.T) {
	got := entryText(`<div class="csl-entry">Effect of A < B > C &amp; D</div>`)
	if !strings.Contains(got, "Effect of A < B > C & D") {
		t.Fatalf("题名字面尖括号被吞掉: %q", got)
	}
}

func TestCite_DeduplicatesRepeatedID(t *testing.T) {
	r, err := NewRenderer(mustBuiltin(t, "gb7714-2025"))
	if err != nil {
		t.Fatal(err)
	}
	if err := r.Add(testPapers()); err != nil {
		t.Fatal(err)
	}
	mark, err := r.Cite("Vaswani2017", "Vaswani2017")
	if err != nil {
		t.Fatal(err)
	}
	if mark != "[1]" {
		t.Fatalf("重复引用未去重: %q", mark)
	}
}

func TestBibliography_NoBibliographySection(t *testing.T) {
	csl := `<?xml version="1.0" encoding="utf-8"?>
<style xmlns="http://purl.org/net/xbiblio/csl" class="in-text" version="1.0">
  <info><title>No Bib</title><id>https://litkit.local/no-bib</id><updated>2026-01-01T00:00:00+00:00</updated></info>
  <citation><layout><text variable="title"/></layout></citation>
</style>`
	e, err := NewEngine([]byte(csl))
	if err != nil {
		t.Fatal(err)
	}
	it := ItemFromPaper(testPapers()[0])
	if err := e.Add(it); err != nil {
		t.Fatal(err)
	}
	if _, err := e.Cite(it); err != nil {
		t.Fatal(err)
	}
	bib, err := e.Bibliography()
	if err != nil {
		t.Fatalf("无 bibliography 不应报错: %v", err)
	}
	if len(bib) != 0 {
		t.Fatalf("无 bibliography 应返回空列表: %v", bib)
	}
}

func TestCite_NoCitationSection(t *testing.T) {
	csl := `<?xml version="1.0" encoding="utf-8"?>
<style xmlns="http://purl.org/net/xbiblio/csl" class="in-text" version="1.0">
  <info><title>No Cite</title><id>https://litkit.local/no-cite</id><updated>2026-01-01T00:00:00+00:00</updated></info>
  <bibliography><layout><text variable="title"/></layout></bibliography>
</style>`
	e, err := NewEngine([]byte(csl))
	if err != nil {
		t.Fatal(err)
	}
	it := ItemFromPaper(testPapers()[0])
	if err := e.Add(it); err != nil {
		t.Fatal(err)
	}
	if _, err := e.Cite(it); err == nil {
		t.Fatal("缺少 citation 布局应返回错误")
	}
}

func TestRenderBibliography_DocTypes(t *testing.T) {
	cases := []struct {
		docType string
		mark    string
	}{
		{"", "[Z]"},
		{model.DocTypeBook, "[M]"},
		{model.DocTypeThesis, "[D]"},
		{model.DocTypeWebpage, "[EB/OL]"},
		{model.DocTypePreprint, "[PP]"},
		{model.DocTypeDataset, "[DS]"},
		{model.DocTypeSoftware, "[CP]"},
	}
	for _, tc := range cases {
		p := model.Paper{CiteKey: "T1", Title: "Title", Year: 2024, Venue: "Journal", DocType: tc.docType}
		if tc.docType == model.DocTypeWebpage {
			p.URL = "https://example.org"
			p.AccessDate = "2024-06-01"
		}
		entries, err := RenderBibliography([]model.Paper{p}, mustBuiltin(t, "gb7714-2025"))
		if err != nil {
			t.Fatalf("docType %q: %v", tc.docType, err)
		}
		if len(entries) != 1 || !strings.Contains(entries[0], tc.mark) {
			t.Errorf("docType %q 期望 %q，得到 %v", tc.docType, tc.mark, entries)
		}
	}
}
