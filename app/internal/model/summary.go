package model

import "strings"

// PaperSummary 是面向 AI agent 的精简论文视图（FR-IFACE-04）。
//
// 设计动机：litkit 的输出对象是 AI agent，完整 Paper 字段（DOI/PMID/ArXivID/
// URL/Venue/DocType/Source/全部作者）对相关性判断与引用占位无用，反而是上下文噪声。
// citeKey 是 AI 与本地文献库之间唯一的握手协议——AI 写 [@Kxq] 占位符，
// manuscript 流水线（M3）按 citeKey 从库中取完整 Paper 做引用格式化。
//
// 字段选择依据：仅保留"判断是否引用 + 写占位符"所需的最小集。
type PaperSummary struct {
	CiteKey     string `json:"citeKey"`            // 引用句柄（写 [@Kxq]）
	Title       string `json:"title"`              // 相关性判断主信号
	FirstAuthor string `json:"firstAuthor"`        // "Family Given" 格式；空串表示未知
	Year        int    `json:"year"`               // 相关性 + 默认排序依据；0 表示未知
	Abstract    string `json:"abstract,omitempty"` // 相关性判断次信号
}

// FirstAuthor 按 "Family Given" 格式返回第一作者；空作者列表返回空串。
//
// 各源适配层已将作者解析为 []Author 且顺序与原文一致，因此 [0] 即第一作者，
// 无需源层适配。空 Given 时仅返回 Family（trim 处理）。
func (p Paper) FirstAuthor() string {
	if len(p.Authors) == 0 {
		return ""
	}
	a := p.Authors[0]
	family := strings.TrimSpace(a.Family)
	given := strings.TrimSpace(a.Given)
	if family == "" {
		return given
	}
	if given == "" {
		return family
	}
	return family + " " + given
}

// Summarize 将 Paper 转为精简视图（FR-IFACE-04）。
func (p Paper) Summarize() PaperSummary {
	return PaperSummary{
		CiteKey:     p.CiteKey,
		Title:       p.Title,
		FirstAuthor: p.FirstAuthor(),
		Year:        p.Year,
		Abstract:    p.Abstract,
	}
}

// SummarizePapers 批量转为精简视图。
func SummarizePapers(ps []Paper) []PaperSummary {
	out := make([]PaperSummary, len(ps))
	for i := range ps {
		out[i] = ps[i].Summarize()
	}
	return out
}
