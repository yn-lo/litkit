package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestLibGetCmd_singleAndBatch(t *testing.T) {
	s := newTestStoreCmd(t)
	dir := t.TempDir()
	// 批量录入两篇（手动 JSON 模式，无需网络）。
	p := filepath.Join(dir, "papers.json")
	content := `[{"title":"文献甲","abstract":"摘要甲。","doi":"10.1/ga"},{"title":"文献乙","abstract":"摘要乙。","doi":"10.1/gb"}]`
	if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	add := newLibAddCmd(s, nil)
	add.SetArgs([]string{p})
	if err := add.Execute(); err != nil {
		t.Fatalf("lib add: %v", err)
	}
	a, _ := s.GetByDOI("10.1/ga")
	b, _ := s.GetByDOI("10.1/gb")

	out := captureStdout(t, func() {
		cmd := newLibGetCmd(s)
		cmd.SetArgs([]string{a.CiteKey, "nope", b.CiteKey})
		if err := cmd.Execute(); err != nil {
			t.Fatalf("lib get: %v", err)
		}
	})
	var got libGetOutput
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("解析输出: %v\n输出: %s", err, out)
	}
	if len(got.Papers) != 2 {
		t.Fatalf("应命中 2 篇，got %d", len(got.Papers))
	}
	if got.Papers[0].CiteKey != a.CiteKey || got.Papers[1].CiteKey != b.CiteKey {
		t.Errorf("应按输入顺序返回，got %s, %s", got.Papers[0].CiteKey, got.Papers[1].CiteKey)
	}
	if got.Papers[0].Abstract != "摘要甲。" {
		t.Errorf("应含摘要，got %q", got.Papers[0].Abstract)
	}
	if len(got.Missing) != 1 || got.Missing[0] != "nope" {
		t.Errorf("missing 应为 [nope]，got %v", got.Missing)
	}
}

func TestLibGetCmd_noArgsFails(t *testing.T) {
	s := newTestStoreCmd(t)
	cmd := newLibGetCmd(s)
	cmd.SetArgs(nil)
	if err := cmd.Execute(); err == nil {
		t.Error("不带 citeKey 应报错")
	}
}

func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	os.Stdout = w
	defer func() { os.Stdout = old }()
	fn()
	_ = w.Close()
	var buf bytes.Buffer
	if _, err := buf.ReadFrom(r); err != nil {
		t.Fatalf("read: %v", err)
	}
	return buf.String()
}
