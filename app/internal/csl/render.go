// render.go 面向业务层的渲染器：注册论文、按文档顺序渲染引用簇、生成文末列表。

package csl

import (
	"fmt"

	"litkit/internal/model"
)

// Renderer 一次渲染会话（正文引用标记 + 文末参考文献）。
type Renderer struct {
	eng  *Engine
	byID map[string]Item
}

// NewRenderer 用样式创建渲染会话。
func NewRenderer(st *Style) (*Renderer, error) {
	if st == nil {
		return nil, fmt.Errorf("csl: 样式为空")
	}
	eng, err := NewEngine(st.Content)
	if err != nil {
		return nil, err
	}
	return &Renderer{eng: eng, byID: make(map[string]Item)}, nil
}

// Add 注册论文条目；后续 Cite 用 ItemFromPaper 生成的 ID 引用。
func (r *Renderer) Add(papers []model.Paper) error {
	items := ItemsFromPapers(papers)
	for _, it := range items {
		r.byID[it.ID] = it
	}
	return r.eng.Add(items...)
}

// Cite 渲染一个引用簇（按传入顺序），返回正文标记。
func (r *Renderer) Cite(ids ...string) (string, error) {
	items := make([]Item, 0, len(ids))
	for _, id := range ids {
		it, ok := r.byID[id]
		if !ok {
			return "", fmt.Errorf("csl: 未注册的条目 %q", id)
		}
		items = append(items, it)
	}
	return r.eng.Cite(items...)
}

// Bibliography 返回文末参考文献条目（按样式排序/编号）。
func (r *Renderer) Bibliography() ([]string, error) {
	return r.eng.Bibliography()
}

// RenderBibliography 一次性渲染整份参考文献列表（按 papers 顺序登记并编号）。
func RenderBibliography(papers []model.Paper, st *Style) ([]string, error) {
	r, err := NewRenderer(st)
	if err != nil {
		return nil, err
	}
	if err := r.Add(papers); err != nil {
		return nil, err
	}
	for _, p := range papers {
		if _, err := r.Cite(ItemFromPaper(p).ID); err != nil {
			return nil, err
		}
	}
	return r.Bibliography()
}
