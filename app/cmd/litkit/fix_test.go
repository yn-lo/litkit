package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"litkit/internal/lint"
)

func TestFilterFixRules(t *testing.T) {
	rules := lint.FixableRules()
	if len(rules) == 0 {
		t.Fatal("FixableRules 不应为空")
	}
	// --rule 仅保留指定规则
	got := filterFixRules(rules, []string{"R3.1"}, nil)
	if len(got) != 1 || got[0].ID != "R3.1" {
		t.Errorf("--rule R3.1 应仅保留 R3.1，got %+v", got)
	}
	// --skip 排除指定规则
	got = filterFixRules(rules, nil, []string{"R3.1"})
	for _, r := range got {
		if r.ID == "R3.1" {
			t.Error("--skip R3.1 应排除 R3.1")
		}
	}
}

func TestFixCmd_rewritesFile(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "doc.md")
	content := "# 方法：设计\n\n正文,内容。他说\"你好\"。\n"
	if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	cmd := newFixCmd()
	cmd.SetArgs([]string{p})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("fix 命令执行失败: %v", err)
	}
	got, err := os.ReadFile(p)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	for _, want := range []string{"# 方法设计", "正文，内容", "他说“你好”"} {
		if !strings.Contains(string(got), want) {
			t.Errorf("文件应含 %q，got %q", want, string(got))
		}
	}
}

func TestFixCmd_noChangesSkipsRewrite(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "ok.md")
	content := "# 方法设计\n\n正文内容。\n"
	if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	cmd := newFixCmd()
	cmd.SetArgs([]string{p})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("fix 命令执行失败: %v", err)
	}
	got, err := os.ReadFile(p)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if string(got) != content {
		t.Errorf("无违规文件不应被改写，got %q", string(got))
	}
}
