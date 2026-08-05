package model

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
)

// Author 论文作者。
type Author struct {
	Given  string `json:"given,omitempty"`
	Family string `json:"family,omitempty"`
}

// DocType 文献类型标识（GB/T 7714-2025 新文献类型，C3）。
const (
	DocTypeArticle        = "article"
	DocTypeJournalArticle = "journal-article"
	DocTypePreprint       = "preprint"
	DocTypeDataset        = "dataset"
	DocTypeBook           = "book"
	DocTypeMonograph      = "monograph"
	DocTypeConference     = "conference"
	DocTypeProceedings    = "proceedings"
	DocTypeReport         = "report"
	DocTypeThesis         = "thesis"
	DocTypePatent         = "patent"
	DocTypeSoftware       = "software"
	DocTypeWebpage        = "webpage"
)

// Paper 论文元数据，摘要工作流的核心载体。
//
// 可选字段用零值（空串 / 0）表示"不可用"，JSON 输出中空串等价于 null 语义。
// Abstract 空串 = 无摘要（检索默认过滤，A6 假设）。
type Paper struct {
	ID          string   `json:"id"`                // 内部稳定唯一 id（hash，ComputeID 生成）
	CiteKey     string   `json:"citeKey,omitempty"` // 3 字母引用标识，入库时分配（FR-LIB-06）
	Title       string   `json:"title"`
	Authors     []Author `json:"authors"`
	Abstract    string   `json:"abstract"` // 检索源必须提供；空串 = 无摘要
	Year        int      `json:"year"`     // 0 表示未知
	Venue       string   `json:"venue"`
	DOI         string   `json:"doi"`
	PMID        string   `json:"pmid"`
	ArXivID     string   `json:"arxivId"`
	URL         string   `json:"url"`
	Source      string   `json:"source"`                // 来源平台标识
	DocType     string   `json:"docType"`               // 文献类型：article/preprint/dataset/...（GB/T 7714-2025）
	Volume      string   `json:"volume,omitempty"`      // 卷（GB/T 7714 卷）
	Number      string   `json:"number,omitempty"`      // 期（GB/T 7714 期）
	Pages       string   `json:"pages,omitempty"`       // 页码（如 "123-135" 或 "e1234"）
	Citations   int      `json:"citations"`             // 源提供时
	Publisher   string   `json:"publisher,omitempty"`   // 出版社（书籍 [M] 用）
	City        string   `json:"city,omitempty"`        // 出版地（书籍 [M] 用）
	Institution string   `json:"institution,omitempty"` // 学位授予机构（学位论文 [D] 用）
	AccessDate  string   `json:"accessDate,omitempty"`  // 网页访问日期（网页用）
}

// ComputeID 按可用标识符优先级生成稳定内部 ID（sha256 前缀）。
//
// 优先级：DOI > PMID > ArXivID > Title。同源同标识符必然命中同 ID，
// 是缓存键与跨源去重的稳定性基础。
func (p Paper) ComputeID() string {
	var seed string
	switch {
	case p.DOI != "":
		seed = "doi:" + p.DOI
	case p.PMID != "":
		seed = "pmid:" + p.PMID
	case p.ArXivID != "":
		seed = "arxiv:" + p.ArXivID
	case p.Title != "":
		seed = "title:" + strings.ToLower(strings.TrimSpace(p.Title))
	default:
		seed = "source:" + p.Source
	}
	sum := sha256.Sum256([]byte(seed))
	return "sha256:" + hex.EncodeToString(sum[:])
}

// HasAbstract 报告该论文是否携带摘要（空串视为无摘要）。
func (p Paper) HasAbstract() bool { return strings.TrimSpace(p.Abstract) != "" }
