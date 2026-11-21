package csl

import (
	"testing"

	"litkit/internal/model"
)

func TestCSLType(t *testing.T) {
	cases := map[string]string{
		model.DocTypeArticle:        "article-journal",
		model.DocTypeJournalArticle: "article-journal",
		model.DocTypePreprint:       "article",
		model.DocTypeDataset:        "dataset",
		model.DocTypeBook:           "book",
		model.DocTypeMonograph:      "book",
		model.DocTypeConference:     "paper-conference",
		model.DocTypeProceedings:    "paper-conference",
		model.DocTypeReport:         "report",
		model.DocTypeThesis:         "thesis",
		model.DocTypePatent:         "patent",
		model.DocTypeSoftware:       "software",
		model.DocTypeWebpage:        "webpage",
		"":                          "document",
	}
	for docType, want := range cases {
		if got := cslType(docType); got != want {
			t.Errorf("cslType(%q) = %q, want %q", docType, got, want)
		}
	}
}

func TestItemID(t *testing.T) {
	p := model.Paper{ID: "sha256:abc"}
	if got := ItemID(p); got != "sha256:abc" {
		t.Errorf("ItemID = %q", got)
	}
	p.CiteKey = "ABC2024"
	if got := ItemID(p); got != "ABC2024" {
		t.Errorf("CiteKey 应优先，got %q", got)
	}
}

func TestItemFromPaper_Mapping(t *testing.T) {
	p := model.Paper{
		CiteKey: "T1", Title: "A  Title", Year: 2024, Venue: "Journal", Volume: "3", Number: "2",
		Pages: "1--9", DOI: "10.1/x", URL: "https://example.org", Publisher: "Press", City: "Beijing",
		Authors: []model.Author{{Family: "Wang", Given: "Xiao"}, {Family: "", Given: ""}},
		DocType: model.DocTypeArticle,
	}
	it := ItemFromPaper(p)
	if it.ID != "T1" {
		t.Errorf("ID = %q", it.ID)
	}
	if it.JSON["title"] != "A Title" {
		t.Errorf("title = %v", it.JSON["title"])
	}
	if it.JSON["page"] != "1-9" {
		t.Errorf("page = %v", it.JSON["page"])
	}
	if it.JSON["issue"] != "2" || it.JSON["volume"] != "3" {
		t.Errorf("volume/issue = %v/%v", it.JSON["volume"], it.JSON["issue"])
	}
	authors, ok := it.JSON["author"].([]map[string]string)
	if !ok || len(authors) != 1 || authors[0]["family"] != "Wang" {
		t.Errorf("author = %#v", it.JSON["author"])
	}
}

func TestItemFromPaper_ThesisPublisher(t *testing.T) {
	p := model.Paper{CiteKey: "T2", Title: "Thesis", DocType: model.DocTypeThesis, Institution: "MIT"}
	it := ItemFromPaper(p)
	if it.JSON["publisher"] != "MIT" {
		t.Errorf("学位论文 publisher = %v，期望 MIT", it.JSON["publisher"])
	}
}

func TestPaperLanguage(t *testing.T) {
	if got := paperLanguage(model.Paper{Language: "en-GB"}); got != "en-GB" {
		t.Errorf("显式 language = %q", got)
	}
	if got := paperLanguage(model.Paper{Title: "中文标题"}); got != "zh-CN" {
		t.Errorf("中文标题 language = %q", got)
	}
	if got := paperLanguage(model.Paper{Title: "English title"}); got != "en" {
		t.Errorf("英文标题 language = %q", got)
	}
}

func TestParseDate(t *testing.T) {
	if got, ok := parseDate("2024-06-01")["date-parts"].([][]int); !ok || got[0][0] != 2024 || got[0][1] != 6 || got[0][2] != 1 {
		t.Errorf("ISO 日期解析失败: %#v", parseDate("2024-06-01"))
	}
	if got, ok := parseDate("2024/6/1")["date-parts"].([][]int); !ok || got[0][0] != 2024 || got[0][1] != 6 || got[0][2] != 1 {
		t.Errorf("斜杠日期解析失败: %#v", parseDate("2024/6/1"))
	}
	if got := parseDate("unknown")["literal"]; got != "unknown" {
		t.Errorf("不可解析日期应回落 literal: %#v", parseDate("unknown"))
	}
}
