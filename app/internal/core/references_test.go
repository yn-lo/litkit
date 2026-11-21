package core

import (
	"strings"
	"testing"

	"litkit/internal/csl"
	"litkit/internal/model"
)

// sampleJournal 一篇典型期刊论文（英文）。
func sampleJournal() model.Paper {
	return model.Paper{
		CiteKey:  "Vaswani2017",
		Title:    "Attention Is All You Need",
		Authors:  []model.Author{{Given: "Ashish", Family: "Vaswani"}, {Given: "Noam", Family: "Shazeer"}, {Given: "Niki", Family: "Parmar"}, {Given: "Jakob", Family: "Uszkoreit"}, {Given: "Llion", Family: "Jones"}},
		Abstract: "The dominant sequence transduction models are based on complex recurrent or convolutional neural networks.",
		Year:     2017,
		Venue:    "Advances in Neural Information Processing Systems",
		DOI:      "10.5555/3295222.3295349",
		URL:      "https://proceedings.neurips.cc/paper/2017",
		DocType:  "article",
		Volume:   "30",
		Pages:    "5998-6008",
	}
}

func mustStyle(t *testing.T, lang, name string) *csl.Style {
	t.Helper()
	st, err := ResolveStyle(lang, name, "")
	if err != nil {
		t.Fatalf("ResolveStyle(%s, %s) err = %v", lang, name, err)
	}
	return st
}

func TestRenderReferenceList_GB7714(t *testing.T) {
	got, err := RenderReferenceList([]model.Paper{sampleJournal()}, mustStyle(t, "zh", "gb7714-2025"))
	if err != nil {
		t.Fatalf("RenderReferenceList err = %v", err)
	}
	for _, want := range []string{"[1] ", "Vaswani A", "Shazeer N", "Attention Is All You Need[J/OL]", "Advances in Neural Information Processing Systems", "2017", "30", "5998-6008", "DOI:10.5555/3295222.3295349"} {
		if !strings.Contains(got, want) {
			t.Errorf("GB7714 条目缺少 %q：\n%s", want, got)
		}
	}
}

func TestRenderReferenceList_GB7714_CJK(t *testing.T) {
	p := model.Paper{
		CiteKey: "Wang2024",
		Title:   "大语言模型在学术写作中的应用研究",
		Authors: []model.Author{{Given: "小明", Family: "王"}, {Given: "丽", Family: "李"}, {Given: "强", Family: "张"}, {Given: "华", Family: "刘"}},
		Year:    2024,
		Venue:   "情报学报",
		DocType: "journal-article",
		Volume:  "43",
		Number:  "2",
		Pages:   "123-135",
		DOI:     "10.1234/abc.2024.001",
	}
	got, err := RenderReferenceList([]model.Paper{p}, mustStyle(t, "zh", "gb7714-2025"))
	if err != nil {
		t.Fatalf("RenderReferenceList err = %v", err)
	}
	for _, want := range []string{"[1] ", "王小明", "李丽", "张强", "等", "大语言模型在学术写作中的应用研究[J/OL]", "情报学报", "2024", "43（2）", "123-135"} {
		if !strings.Contains(got, want) {
			t.Errorf("GB7714 中文条目缺少 %q：\n%s", want, got)
		}
	}
	if !strings.Contains(got, "，") {
		t.Errorf("GB7714 中文条目应使用全角逗号：\n%s", got)
	}
}

func TestRenderReferenceList_APA(t *testing.T) {
	got, err := RenderReferenceList([]model.Paper{sampleJournal()}, mustStyle(t, "en", "apa"))
	if err != nil {
		t.Fatalf("RenderReferenceList err = %v", err)
	}
	for _, want := range []string{
		"Vaswani, A., Shazeer, N., Parmar, N., Uszkoreit, J., & Jones, L.",
		"(2017)",
		"Attention Is All You Need",
		"Advances in Neural Information Processing Systems, 30, 5998–6008",
		"https://doi.org/10.5555/3295222.3295349",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("APA 条目缺少 %q：\n%s", want, got)
		}
	}
}

func TestRenderReferenceList_IEEE(t *testing.T) {
	got, err := RenderReferenceList([]model.Paper{sampleJournal()}, mustStyle(t, "en", "ieee"))
	if err != nil {
		t.Fatalf("RenderReferenceList err = %v", err)
	}
	for _, want := range []string{"[1] A. Vaswani", "N. Shazeer", "“Attention Is All You Need,”", "Advances in Neural Information Processing Systems", "vol. 30", "pp. 5998–6008", "2017"} {
		if !strings.Contains(got, want) {
			t.Errorf("IEEE 条目缺少 %q：\n%s", want, got)
		}
	}
}

func TestRenderReferenceList_APANoVenue(t *testing.T) {
	// 无 venue 时不得出现 "Title.." 双句点
	got, err := RenderReferenceList([]model.Paper{{
		Title:   "A Venue-Less Study",
		Authors: []model.Author{{Family: "Doe", Given: "Jane"}},
		Year:    2023,
	}}, mustStyle(t, "en", "apa"))
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if strings.Contains(got, "..") {
		t.Fatalf("APA 无 venue 出现双句点：%q", got)
	}
	if !strings.Contains(got, "A Venue-Less Study.") {
		t.Fatalf("APA 无 venue 应以单句点结尾：%q", got)
	}
}

func TestRenderReferenceList_UnknownStyle(t *testing.T) {
	if _, err := ResolveStyle("zh", "chicago", ""); err == nil {
		t.Fatal("未知样式应返回错误")
	}
}

func TestResolveStyle_Defaults(t *testing.T) {
	zh, err := ResolveStyle("zh", "", "")
	if err != nil || zh.Name != "gb7714-2025" {
		t.Fatalf("zh 默认样式 = %v, err = %v", zh, err)
	}
	en, err := ResolveStyle("en", "", "")
	if err != nil || en.Name != "apa" {
		t.Fatalf("en 默认样式 = %v, err = %v", en, err)
	}
}

func TestFormatBibTeX_FieldComplete(t *testing.T) {
	got := FormatBibTeX(sampleJournal())
	for _, want := range []string{
		"@article{Vaswani2017,",
		"author = {Vaswani, Ashish and Shazeer, Noam and Parmar, Niki and Uszkoreit, Jakob and Jones, Llion}",
		"title = {Attention Is All You Need}",
		"journal = {Advances in Neural Information Processing Systems}",
		"year = {2017}",
		"volume = {30}",
		"pages = {5998-6008}",
		"doi = {10.5555/3295222.3295349}",
		"url = {https://proceedings.neurips.cc/paper/2017}",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("BibTeX 缺少 %q：\n%s", want, got)
		}
	}
}

func TestFormatBibTeX_KeyFallback(t *testing.T) {
	p := sampleJournal()
	p.CiteKey = ""
	got := FormatBibTeX(p)
	if !strings.Contains(got, "@article{Vaswani2017,") {
		t.Errorf("无 CiteKey 时应回落 作者姓+年：\n%s", got)
	}
	// 特殊字符转义
	p.Title = "Data & Models: A 100% Guide #2"
	p.CiteKey = "T1"
	got = FormatBibTeX(p)
	for _, want := range []string{`Data \& Models: A 100\% Guide \#2`, "@article{T1,"} {
		if !strings.Contains(got, want) {
			t.Errorf("转义/类型映射缺失 %q：\n%s", want, got)
		}
	}
}

func TestFormatRIS_FieldComplete(t *testing.T) {
	got := FormatRIS(sampleJournal())
	for _, want := range []string{
		"TY  - JOUR",
		"AU  - Vaswani, Ashish",
		"TI  - Attention Is All You Need",
		"JO  - Advances in Neural Information Processing Systems",
		"PY  - 2017",
		"VL  - 30",
		"SP  - 5998",
		"EP  - 6008",
		"DO  - 10.5555/3295222.3295349",
		"UR  - https://proceedings.neurips.cc/paper/2017",
		"ER  - ",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("RIS 缺少 %q：\n%s", want, got)
		}
	}
}

// TestFormatRIS_FieldNewlinesCollapsed RIS 是逐行记录格式：
// 字段内换行必须折叠，否则可伪造字段或提前终止条目；BibTeX 键同理不得含换行。
func TestFormatRIS_FieldNewlinesCollapsed(t *testing.T) {
	p := sampleJournal()
	p.Title = "Real Title\nER  - \nTY  - JOUR"
	p.Abstract = "line1\r\nline2"
	got := FormatRIS(p)
	if strings.Contains(got, "\r") {
		t.Errorf("RIS 不应含 CR：\n%q", got)
	}
	if c := strings.Count(got, "TY  - "); c != 1 {
		t.Errorf("注入的 TY 应被折叠，got %d 条：\n%s", c, got)
	}
	if c := strings.Count(got, "ER  - "); c != 1 {
		t.Errorf("注入的 ER 应被折叠，got %d 条：\n%s", c, got)
	}
	if !strings.Contains(got, "TI  - Real Title ER - TY - JOUR") {
		t.Errorf("字段应折叠为单行：\n%s", got)
	}
	if !strings.Contains(got, "AB  - line1 line2") {
		t.Errorf("抽象换行应折叠：\n%s", got)
	}

	p.CiteKey = "Bad\nKey"
	if bib := FormatBibTeX(p); !strings.Contains(bib, "@article{BadKey,") {
		t.Errorf("BibTeX 键应剔除换行：\n%s", bib)
	}
}

func TestRISFromPapers_Batch(t *testing.T) {
	p1 := sampleJournal()
	p2 := p1
	p2.CiteKey = "Smith2020"
	p2.Title = "Second Paper"
	p2.DocType = "preprint"
	got := RISFromPapers([]model.Paper{p1, p2})
	if c := strings.Count(got, "TY  - "); c != 2 {
		t.Errorf("期望 2 条 RIS 条目，得到 %d", c)
	}
	if !strings.Contains(got, "TY  - PREP") {
		t.Errorf("preprint 应映射 PREP：\n%s", got)
	}
}
