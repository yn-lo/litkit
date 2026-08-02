package sources

import (
	"context"
	"testing"

	"litkit/internal/model"
)

// fakeSource 测试用 PaperSource 实现。
type fakeSource struct {
	name string
}

func (f fakeSource) Name() string { return f.name }
func (f fakeSource) Search(ctx context.Context, q string, opts SearchOptions) ([]model.Paper, error) {
	return nil, nil
}
func (f fakeSource) HasAbstract() bool { return true }

func TestRegistry_RegisterAndGet(t *testing.T) {
	r := NewRegistry()
	r.Register(fakeSource{name: "arxiv"})
	r.Register(fakeSource{name: "pubmed"})

	if s, ok := r.Get("arxiv"); !ok || s.Name() != "arxiv" {
		t.Fatal("Get(arxiv) 失败")
	}
	if _, ok := r.Get("nope"); ok {
		t.Fatal("Get(nope) 应返回 false")
	}
}

func TestRegistry_ListSorted(t *testing.T) {
	r := NewRegistry()
	// 故意乱序注册
	r.Register(fakeSource{name: "pubmed"})
	r.Register(fakeSource{name: "arxiv"})
	r.Register(fakeSource{name: "openalex"})

	list := r.List()
	got := make([]string, len(list))
	for i, s := range list {
		got[i] = s.Name()
	}
	want := []string{"arxiv", "openalex", "pubmed"}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("List 应按字典序，got %v want %v", got, want)
		}
	}
}

func TestRegistry_RegisterOverwrites(t *testing.T) {
	r := NewRegistry()
	r.Register(fakeSource{name: "arxiv"})
	r.Register(fakeSource{name: "arxiv"}) // 重名覆盖
	if len(r.List()) != 1 {
		t.Fatalf("重名覆盖后应仍为 1 个，got %d", len(r.List()))
	}
}
