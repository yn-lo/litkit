package main

import (
	"testing"

	"litkit/internal/model"
)

// TestDedupPapers 重复 citeKey 应仅保留首次出现（report 问题2）。
func TestDedupPapers(t *testing.T) {
	ps := []model.Paper{
		{CiteKey: "a", Title: "A"},
		{CiteKey: "a", Title: "A-dup"},
		{CiteKey: "b", Title: "B"},
		{CiteKey: "", Title: "NoKey1"},
		{CiteKey: "", Title: "NoKey2"},
	}
	got := dedupPapers(ps)
	if len(got) != 4 {
		t.Fatalf("去重后应为 4 篇（空 citeKey 不去重），got %d", len(got))
	}
	if got[0].Title != "A" {
		t.Errorf("应保留首次出现的 A（got %q）", got[0].Title)
	}
	if got[0].Title == "A-dup" || got[1].Title == "A-dup" {
		t.Errorf("重复 citeKey=a 的第二条应被丢弃：%+v", got)
	}
}

// TestValidateRuleIDs 未知规则 ID 应报错（report 问题3）。
func TestValidateRuleIDs(t *testing.T) {
	if err := validateRuleIDs("R9.9"); err == nil {
		t.Fatal("未知规则 R9.9 应报错")
	}
	if err := validateRuleIDs("R3.3,R1.3"); err != nil {
		t.Fatalf("合法规则不应报错：%v", err)
	}
	if err := validateRuleIDs(""); err != nil {
		t.Fatalf("空 --rule 不应报错：%v", err)
	}
}
