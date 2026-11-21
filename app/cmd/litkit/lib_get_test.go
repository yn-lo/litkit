package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"litkit/internal/config"
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
	add := newLibAddCmd(&deps{store: s})
	add.SetArgs([]string{p})
	if err := add.Execute(); err != nil {
		t.Fatalf("lib add: %v", err)
	}
	a, _ := s.GetByDOI("10.1/ga")
	b, _ := s.GetByDOI("10.1/gb")

	out := captureStdout(t, func() {
		cmd := newLibGetCmd(&deps{store: s})
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
	cmd := newLibGetCmd(&deps{store: s})
	cmd.SetArgs(nil)
	if err := cmd.Execute(); err == nil {
		t.Error("不带 citeKey 应报错")
	}
}

// TestLibRmCmd_removesDownloadedPDF lib rm 应同时清理该论文已落盘的 PDF（避免孤儿产物）。
func TestLibRmCmd_removesDownloadedPDF(t *testing.T) {
	s := newTestStoreCmd(t)
	dir := t.TempDir()
	meta := filepath.Join(dir, "paper.json")
	if err := os.WriteFile(meta, []byte(`{"title":"待删文献","abstract":"摘要。","doi":"10.1/rm"}`), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	add := newLibAddCmd(&deps{store: s})
	add.SetArgs([]string{meta})
	if err := add.Execute(); err != nil {
		t.Fatalf("lib add: %v", err)
	}
	paper, err := s.GetByDOI("10.1/rm")
	if err != nil || paper == nil {
		t.Fatalf("论文应入库，err=%v", err)
	}

	dl := filepath.Join(dir, "downloads")
	if err := os.MkdirAll(dl, 0o750); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	pdf := filepath.Join(dl, paper.CiteKey+".pdf")
	if err := os.WriteFile(pdf, []byte("%PDF-1.4"), 0o600); err != nil {
		t.Fatalf("write pdf: %v", err)
	}

	out := captureStdout(t, func() {
		cmd := newLibRmCmd(&deps{store: s, cfg: &config.Config{WorkDir: dir, FetchDownloadDir: dl}})
		cmd.SetArgs([]string{paper.CiteKey})
		if err := cmd.Execute(); err != nil {
			t.Fatalf("lib rm: %v", err)
		}
	})
	var got struct {
		Removed    bool `json:"removed"`
		PDFRemoved bool `json:"pdfRemoved"`
	}
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("解析输出: %v\n输出: %s", err, out)
	}
	if !got.Removed || !got.PDFRemoved {
		t.Errorf("应删除条目与 PDF：%+v（输出 %s）", got, out)
	}
	if _, err := os.Stat(pdf); !os.IsNotExist(err) {
		t.Errorf("PDF 应被删除，stat err=%v", err)
	}
	if p, _ := s.GetByCiteKey(paper.CiteKey); p != nil {
		t.Error("条目应已从库中删除")
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
