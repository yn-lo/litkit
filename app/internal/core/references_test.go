package core

import (
	"strings"
	"testing"

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

func TestFormatReference_GB7714_English(t *testing.T) {
	got, err := FormatReference(sampleJournal(), StyleGB7714, 1)
	if err != nil {
		t.Fatalf("FormatReference err = %v", err)
	}
	for _, want := range []string{"[1] ", "VASWANI A", "Attention Is All You Need[J]", "Advances in Neural Information Processing Systems", "2017", "30", "5998-6008", "DOI:10.5555/3295222.3295349."} {
		if !strings.Contains(got, want) {
			t.Errorf("GB7714 条目缺少 %q：\n%s", want, got)
		}
	}
}

func TestFormatReference_GB7714_CJK(t *testing.T) {
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
	got, err := FormatReference(p, StyleGB7714, 2)
	if err != nil {
		t.Fatalf("FormatReference err = %v", err)
	}
	for _, want := range []string{"[2] ", "王小明, 李丽, 张强, 等", "大语言模型在学术写作中的应用研究[J]", "情报学报", "2024", "43(2)", "123-135"} {
		if !strings.Contains(got, want) {
			t.Errorf("GB7714 中文条目缺少 %q：\n%s", want, got)
		}
	}
}

func TestFormatReference_GB7714_NewDocTypes(t *testing.T) {
	cases := []struct {
		docType string
		mark    string
	}{
		{"preprint", "[PP]"},
		{"dataset", "[DS]"},
		{"software", "[CP]"},
		{"", "[Z]"},
	}
	for _, c := range cases {
		p := sampleJournal()
		p.DocType = c.docType
		got, err := FormatReference(p, StyleGB7714, 0)
		if err != nil {
			t.Fatalf("docType %q err = %v", c.docType, err)
		}
		if !strings.Contains(got, c.mark) {
			t.Errorf("docType %q 期望标志 %q，得到：\n%s", c.docType, c.mark, got)
		}
	}
}

func TestFormatReference_APA(t *testing.T) {
	got, err := FormatReference(sampleJournal(), StyleAPA, 0)
	if err != nil {
		t.Fatalf("FormatReference err = %v", err)
	}
	for _, want := range []string{"Vaswani, A., Shazeer, N., Parmar, N., Uszkoreit, J., & Jones, L.", "(2017)", "Attention Is All You Need.", "Advances in Neural Information Processing Systems, 30, 5998-6008.", "https://doi.org/10.5555/3295222.3295349"} {
		if !strings.Contains(got, want) {
			t.Errorf("APA 条目缺少 %q：\n%s", want, got)
		}
	}
}

func TestFormatReference_IEEE(t *testing.T) {
	got, err := FormatReference(sampleJournal(), StyleIEEE, 3)
	if err != nil {
		t.Fatalf("FormatReference err = %v", err)
	}
	for _, want := range []string{"[3] A. Vaswani, N. Shazeer, N. Parmar, J. Uszkoreit, and L. Jones,", "\"Attention Is All You Need,\"", "Advances in Neural Information Processing Systems", "vol. 30", "pp. 5998-6008", "2017"} {
		if !strings.Contains(got, want) {
			t.Errorf("IEEE 条目缺少 %q：\n%s", want, got)
		}
	}
}

func TestFormatReference_UnknownStyle(t *testing.T) {
	if _, err := FormatReference(sampleJournal(), Style("chicago"), 0); err == nil {
		t.Fatal("未知样式应返回错误")
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
