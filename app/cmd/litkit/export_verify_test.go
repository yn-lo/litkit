package main

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"litkit/internal/model"
)

// TestReadPapersFile 仅接受 []Paper 或 {"papers":[...]}；
// 单个 Paper 对象须报错，不能静默导出 0 条（未知字段反序列化会得到空 wrapper）。
func TestReadPapersFile(t *testing.T) {
	dir := t.TempDir()
	cases := []struct {
		name    string
		content string
		wantN   int
		wantErr bool
	}{
		{"裸数组", `[{"title":"A"},{"title":"B"}]`, 2, false},
		{"包装对象", `{"papers":[{"title":"A"}]}`, 1, false},
		{"包装空数组", `{"papers":[]}`, 0, false},
		{"单个对象", `{"title":"A"}`, 0, true},
		{"非 JSON", `not json`, 0, true},
	}
	for _, tc := range cases {
		p := filepath.Join(dir, "papers.json")
		if err := os.WriteFile(p, []byte(tc.content), 0o600); err != nil {
			t.Fatalf("write fixture: %v", err)
		}
		got, err := readPapersFile(p)
		if tc.wantErr {
			if err == nil {
				t.Errorf("%s: 应报错，got %d 条", tc.name, len(got))
			}
			continue
		}
		if err != nil {
			t.Errorf("%s: 不应报错：%v", tc.name, err)
			continue
		}
		if len(got) != tc.wantN {
			t.Errorf("%s: got %d 条 want %d", tc.name, len(got), tc.wantN)
		}
	}
}

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

// TestExitCodeOfUsageErrors cobra 的参数/子命令错误应映射为退出码 2。
func TestExitCodeOfUsageErrors(t *testing.T) {
	cases := []error{
		errors.New("accepts 1 arg(s), received 0"),
		errors.New(`unknown command "foo" for "litkit styles"`),
		errors.New("unknown flag: --x"),
		errors.New("flag needs an argument: --style"),
	}
	for _, err := range cases {
		if got := exitCodeOf(err); got != 2 {
			t.Errorf("exitCodeOf(%q) = %d, want 2", err, got)
		}
	}
	if got := exitCodeOf(errors.New("network down")); got != 1 {
		t.Errorf("普通运行错误 exitCodeOf = %d, want 1", got)
	}
}
