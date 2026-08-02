package textutil

import "testing"

func TestNormalizeTitle(t *testing.T) {
	cases := []struct {
		name, in, want string
	}{
		{"empty", "", ""},
		{"trim", "  Graph Neural Networks  ", "graph neural networks"},
		{"collapse_double_spaces", "graph  neural\tnetworks", "graph neural networks"},
		{"lowercase", "GRAPH Neural NETWORKS", "graph neural networks"},
		{"preserve_punctuation", "Graph: A Survey!", "graph: a survey!"},
		{"mixed_whitespace", "\n\r Graph\tNeural  Networks \n", "graph neural networks"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := NormalizeTitle(c.in); got != c.want {
				t.Errorf("NormalizeTitle(%q) = %q, want %q", c.in, got, c.want)
			}
		})
	}
}

func TestNormalizeTitle_deterministic(t *testing.T) {
	// 同一标题的不同呈现应归一化为同一键（跨源去重前提）
	a := NormalizeTitle("  Graph   Neural  Networks  ")
	b := NormalizeTitle("graph neural networks")
	if a != b {
		t.Fatalf("同一标题不同呈现应产生同键： %q != %q", a, b)
	}
}
