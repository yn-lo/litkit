package model

// SearchResult 跨源检索结果。
//
// 单源失败归入 Errors，不中断整体（FR-SEARCH-01）。
// Papers 是去重合并后的最终结果集（DOI→title→id 三级去重，FR-SEARCH-02）。
type SearchResult struct {
	Total         int                `json:"total"`
	SourceResults map[string][]Paper `json:"sourceResults"`
	Errors        []SourceError      `json:"errors"`
	Papers        []Paper            `json:"papers"` // 去重合并后
}

// SourceError 单源失败记录（不中断整体检索）。
type SourceError struct {
	Source string `json:"source"`
	Error  string `json:"error"`
}

// NewSearchResult 构造空 SearchResult，初始化内部切片与 map。
func NewSearchResult() *SearchResult {
	return &SearchResult{
		SourceResults: map[string][]Paper{},
		Errors:        []SourceError{},
		Papers:        []Paper{},
	}
}
