package model

import "testing"

func TestPaper_FirstAuthor_FamilyGiven(t *testing.T) {
	p := Paper{Authors: []Author{{Given: "Sergey", Family: "Oladyshkin"}}}
	if got := p.FirstAuthor(); got != "Oladyshkin Sergey" {
		t.Fatalf("got %q", got)
	}
}

func TestPaper_FirstAuthor_OnlyFamily(t *testing.T) {
	p := Paper{Authors: []Author{{Family: "Oladyshkin"}}}
	if got := p.FirstAuthor(); got != "Oladyshkin" {
		t.Fatalf("got %q", got)
	}
}

func TestPaper_FirstAuthor_OnlyGiven(t *testing.T) {
	p := Paper{Authors: []Author{{Given: "Sergey"}}}
	if got := p.FirstAuthor(); got != "Sergey" {
		t.Fatalf("got %q", got)
	}
}

func TestPaper_FirstAuthor_Empty(t *testing.T) {
	p := Paper{}
	if got := p.FirstAuthor(); got != "" {
		t.Fatalf("空作者应返回空串，got %q", got)
	}
}

func TestPaper_Summarize(t *testing.T) {
	p := Paper{
		CiteKey:  "Kxq",
		Title:    "Graph Nets",
		Authors:  []Author{{Given: "Sergey", Family: "Oladyshkin"}},
		Abstract: "abs",
		Year:     2023,
		DOI:      "10.1/x", // 不应出现在 summary
	}
	s := p.Summarize()
	if s.CiteKey != "Kxq" || s.Title != "Graph Nets" || s.FirstAuthor != "Oladyshkin Sergey" ||
		s.Year != 2023 || s.Abstract != "abs" {
		t.Fatalf("summary 字段不符：%+v", s)
	}
}

func TestSummarizePapers(t *testing.T) {
	ps := []Paper{
		{CiteKey: "A", Title: "T1", Authors: []Author{{Family: "X"}}, Year: 2020, Abstract: "a1"},
		{CiteKey: "B", Title: "T2", Authors: []Author{{Family: "Y"}}, Year: 2021, Abstract: "a2"},
	}
	sums := SummarizePapers(ps)
	if len(sums) != 2 || sums[0].CiteKey != "A" || sums[1].CiteKey != "B" {
		t.Fatalf("批量转换不符：%+v", sums)
	}
}
