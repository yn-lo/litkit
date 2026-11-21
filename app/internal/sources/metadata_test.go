package sources

import (
	"math"
	"testing"
)

func TestTitleSimilar(t *testing.T) {
	cases := []struct {
		name string
		a, b string
		want float64
	}{
		{"完全匹配", "Attention Is All You Need", "Attention Is All You Need", 1},
		{"大小写标点差异", "Graph Neural Networks: A Review", "graph neural networks a review", 1},
		{"查询为结果子集", "graph neural networks", "Graph Neural Networks for Node Classification", 0.6},
		{"完全不相关", "graph neural networks", "Found via Title Search", 0},
		{"空标题", "", "Graph Neural Networks", 0},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := titleSimilar(c.a, c.b)
			if math.Abs(got-c.want) > 1e-9 {
				t.Errorf("titleSimilar(%q, %q) = %v，期望 %v", c.a, c.b, got, c.want)
			}
		})
	}
}
