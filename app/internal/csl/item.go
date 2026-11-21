// item.go CSL-JSON 条目的最小表示。

package csl

// Item 是 citeproc-js 的输入条目：ID 用于引用，JSON 为 CSL-JSON 字段。
type Item struct {
	ID   string
	JSON map[string]interface{}
}
