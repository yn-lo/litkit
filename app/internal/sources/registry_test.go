package sources

import (
	"context"
	"testing"

	"litkit/internal/model"
	"litkit/internal/util/ratelimit"
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

func TestRegistry_DefaultNames(t *testing.T) {
	// 默认不参与的源（bioRxiv/medRxiv）不出现在 DefaultNames，但仍可通过 -s 显式使用
	r := NewRegistry()
	r.Register(fakeSource{name: "openalex"})
	r.Register(NewBiorxivSource("biorxiv", newHTTPClient(1000, 0, nil), ratelimit.New(100, 1)))
	names := r.DefaultNames()
	if len(names) != 1 || names[0] != "openalex" {
		t.Fatalf("DefaultNames 应只含 openalex，got %v", names)
	}
	if len(r.Names()) != 2 {
		t.Fatalf("Names 应含全部源，got %v", r.Names())
	}
}
