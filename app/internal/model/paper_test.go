package model

import "testing"

func TestPaper_ComputeID_priority(t *testing.T) {
	// DOI 优先于其他标识符
	p := Paper{DOI: "10.1000/xyz", PMID: "123", ArXivID: "2304.1", Title: "T"}
	got := p.ComputeID()
	want := (Paper{DOI: "10.1000/xyz"}).ComputeID()
	if got != want {
		t.Fatalf("DOI 优先级失败：got %s want %s", got, want)
	}
	if !startsWith(got, "sha256:") {
		t.Fatalf("ID 应以 sha256: 前缀，got %s", got)
	}
}

func TestPaper_ComputeID_deterministic(t *testing.T) {
	a := Paper{DOI: "10.1000/abc"}
	b := Paper{DOI: "10.1000/abc"}
	if a.ComputeID() != b.ComputeID() {
		t.Fatal("同 DOI 应产生同 ID（确定性）")
	}
}

func TestPaper_ComputeID_fallbacks(t *testing.T) {
	// 无 DOI → PMID → ArXivID → Title → source
	cases := []struct {
		name string
		p    Paper
	}{
		{"pmid", Paper{PMID: "123"}},
		{"arxiv", Paper{ArXivID: "2304.1"}},
		{"title", Paper{Title: "Some Title"}},
		{"source-only", Paper{Source: "arxiv"}},
	}
	seen := map[string]bool{}
	for _, c := range cases {
		id := c.p.ComputeID()
		if !startsWith(id, "sha256:") {
			t.Fatalf("%s: ID 应以 sha256: 前缀，got %s", c.name, id)
		}
		if seen[id] {
			t.Fatalf("%s: 与前序案例 ID 冲突", c.name)
		}
		seen[id] = true
	}
}

func TestPaper_ComputeID_titleNormalization(t *testing.T) {
	// Title 大小写与首尾空格不影响 ID
	a := Paper{Title: "Graph Neural Networks"}
	b := Paper{Title: "  graph neural networks  "}
	if a.ComputeID() != b.ComputeID() {
		t.Fatal("Title 归一化失败：大小写与首尾空格应等价")
	}
}

func TestPaper_HasAbstract(t *testing.T) {
	if (Paper{}).HasAbstract() {
		t.Fatal("空串应视为无摘要")
	}
	if !(Paper{Abstract: "..."}).HasAbstract() {
		t.Fatal("非空串应视为有摘要")
	}
	if (Paper{Abstract: "   "}).HasAbstract() {
		t.Fatal("纯空白应视为无摘要")
	}
}

func startsWith(s, prefix string) bool {
	return len(s) >= len(prefix) && s[:len(prefix)] == prefix
}
