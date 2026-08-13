package main

import (
	"os"
	"path/filepath"
	"testing"

	"litkit/internal/storage"
)

// newTestStoreCmd 创建临时文献库（cmd 包测试用）。
func newTestStoreCmd(t *testing.T) *storage.Store {
	t.Helper()
	s, err := storage.Open(filepath.Join(t.TempDir(), "litkit.db"))
	if err != nil {
		t.Fatalf("Open store: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

func TestLibAddCmd_manualPaper(t *testing.T) {
	s := newTestStoreCmd(t)
	dir := t.TempDir()
	p := filepath.Join(dir, "paper.json")
	content := `{"title":"手动录入文献","authors":["张三"],"abstract":"这是手动录入的摘要内容。","year":2024,"doi":"10.1000/manual"}`
	if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	cmd := newLibAddCmd(s)
	cmd.SetArgs([]string{p})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("lib add 执行失败: %v", err)
	}
	paper, err := s.GetByDOI("10.1000/manual")
	if err != nil || paper == nil {
		t.Fatalf("论文应入库，err=%v", err)
	}
	if paper.Source != "manual" {
		t.Errorf("source 应为 manual，got %q", paper.Source)
	}
	if paper.Abstract != "这是手动录入的摘要内容。" {
		t.Errorf("摘要应入库，got %q", paper.Abstract)
	}
}

func TestLibAddCmd_rejectsMissingAbstract(t *testing.T) {
	s := newTestStoreCmd(t)
	dir := t.TempDir()
	p := filepath.Join(dir, "bad.json")
	if err := os.WriteFile(p, []byte(`{"title":"无摘要"}`), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	cmd := newLibAddCmd(s)
	cmd.SetArgs([]string{p})
	if err := cmd.Execute(); err == nil {
		t.Error("缺摘要应拒绝入库")
	}
	st, _ := s.Stats()
	if st.Total != 0 {
		t.Errorf("不应入库，total=%d", st.Total)
	}
}

func TestLibAddCmd_arrayBatch(t *testing.T) {
	s := newTestStoreCmd(t)
	dir := t.TempDir()
	p := filepath.Join(dir, "papers.json")
	content := `[{"title":"文献一","abstract":"摘要一。","doi":"10.1/a"},{"title":"文献二","abstract":"摘要二。","doi":"10.1/b"}]`
	if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	cmd := newLibAddCmd(s)
	cmd.SetArgs([]string{p})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("lib add 批量执行失败: %v", err)
	}
	st, _ := s.Stats()
	if st.Total != 2 {
		t.Errorf("应入库 2 篇，total=%d", st.Total)
	}
}

func TestLibAddCmd_upsertKeepsCiteKey(t *testing.T) {
	s := newTestStoreCmd(t)
	dir := t.TempDir()
	p := filepath.Join(dir, "paper.json")
	content := `{"title":"同一文献","abstract":"摘要内容。","doi":"10.1/same"}`
	if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	cmd := newLibAddCmd(s)
	cmd.SetArgs([]string{p})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("首次入库失败: %v", err)
	}
	first, err := s.GetByDOI("10.1/same")
	if err != nil || first == nil {
		t.Fatalf("首次入库失败: %v", err)
	}
	// 同 DOI 再次入库：更新字段，citeKey 不变
	if err := cmd.Execute(); err != nil {
		t.Fatalf("二次入库失败: %v", err)
	}
	second, _ := s.GetByDOI("10.1/same")
	if second.CiteKey != first.CiteKey {
		t.Errorf("重复入库应保持 citeKey，got %q vs %q", second.CiteKey, first.CiteKey)
	}
}

func TestLibAddCmd_BOMTolerant(t *testing.T) {
	s := newTestStoreCmd(t)
	dir := t.TempDir()
	p := filepath.Join(dir, "paper.json")
	// 带 UTF-8 BOM 的 JSON（Windows 编辑器常见），应正常解析
	content := "\uFEFF" + `{"title":"BOM 文献","abstract":"摘要内容。","doi":"10.1/bom"}`
	if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	cmd := newLibAddCmd(s)
	cmd.SetArgs([]string{p})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("lib add 解析带 BOM 文件失败: %v", err)
	}
	paper, err := s.GetByDOI("10.1/bom")
	if err != nil || paper == nil {
		t.Fatalf("论文应入库，err=%v", err)
	}
}
