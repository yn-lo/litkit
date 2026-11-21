// map.go 把 model.Paper 映射为 CSL-JSON 条目（citeproc-js 的输入）。
//
// 字段名遵循 CSL-JSON 规范；缺失字段直接不写入，交由样式自行降级。

package csl

import (
	"strings"
	"time"

	"litkit/internal/model"
)

// ItemID 返回论文在 CSL 会话中的稳定 ID：CiteKey > 内部 ID > ComputeID。
func ItemID(p model.Paper) string {
	if id := strings.TrimSpace(p.CiteKey); id != "" {
		return id
	}
	if id := strings.TrimSpace(p.ID); id != "" {
		return id
	}
	return p.ComputeID()
}

// ItemFromPaper 将论文元数据转换为 CSL-JSON 条目。
func ItemFromPaper(p model.Paper) Item {
	id := ItemID(p)
	m := map[string]interface{}{
		"id":       id,
		"type":     cslType(p.DocType),
		"title":    strings.Join(strings.Fields(p.Title), " "),
		"language": paperLanguage(p),
	}
	if authors := cslAuthors(p.Authors); len(authors) > 0 {
		m["author"] = authors
	}
	if p.Year > 0 {
		m["issued"] = map[string]interface{}{"date-parts": [][]int{{p.Year}}}
	}
	if p.Venue != "" {
		m["container-title"] = p.Venue
	}
	if p.Volume != "" {
		m["volume"] = p.Volume
	}
	if p.Number != "" {
		m["issue"] = p.Number
	}
	if p.Pages != "" {
		m["page"] = cleanPages(p.Pages)
	}
	if p.DOI != "" {
		m["DOI"] = p.DOI
	}
	if p.URL != "" {
		m["URL"] = p.URL
	}
	if p.Publisher != "" {
		m["publisher"] = p.Publisher
	}
	if p.City != "" {
		m["publisher-place"] = p.City
	}
	// 学位论文的授予机构在 CSL 中走 publisher（Zotero 同款映射）。
	if p.Institution != "" && isThesis(p.DocType) {
		m["publisher"] = p.Institution
	}
	if p.AccessDate != "" {
		m["accessed"] = parseDate(p.AccessDate)
	}
	return Item{ID: id, JSON: m}
}

// ItemsFromPapers 批量转换。
func ItemsFromPapers(ps []model.Paper) []Item {
	items := make([]Item, 0, len(ps))
	for _, p := range ps {
		items = append(items, ItemFromPaper(p))
	}
	return items
}

// cslType 文献类型 → CSL type。
func cslType(docType string) string {
	switch strings.ToLower(strings.TrimSpace(docType)) {
	case model.DocTypeArticle, model.DocTypeJournalArticle:
		return "article-journal"
	case model.DocTypePreprint:
		return "article"
	case model.DocTypeDataset:
		return "dataset"
	case model.DocTypeBook, model.DocTypeMonograph:
		return "book"
	case model.DocTypeConference, model.DocTypeProceedings:
		return "paper-conference"
	case model.DocTypeReport:
		return "report"
	case model.DocTypeThesis:
		return "thesis"
	case model.DocTypePatent:
		return "patent"
	case model.DocTypeSoftware:
		return "software"
	case model.DocTypeWebpage:
		return "webpage"
	default:
		// 空/未知类型走通用 document，避免被 GB/T 当作预印本 [PP]。
		return "document"
	}
}

// cslAuthors 作者列表 → CSL name 对象；family/given 均空时跳过。
func cslAuthors(as []model.Author) []map[string]string {
	out := make([]map[string]string, 0, len(as))
	for _, a := range as {
		f := strings.TrimSpace(a.Family)
		g := strings.TrimSpace(a.Given)
		if f == "" && g == "" {
			continue
		}
		name := make(map[string]string, 2)
		if f != "" {
			name["family"] = f
		}
		if g != "" {
			name["given"] = g
		}
		out = append(out, name)
	}
	return out
}

// paperLanguage 返回 CSL language：显式声明优先，否则按中英文特征推断。
func paperLanguage(p model.Paper) string {
	if l := strings.TrimSpace(p.Language); l != "" {
		return l
	}
	if isCJKPaper(p) {
		return "zh-CN"
	}
	return "en"
}

// isCJKPaper 首位作者或标题含 CJK 即视为中文文献。
func isCJKPaper(p model.Paper) bool {
	if len(p.Authors) > 0 {
		a := p.Authors[0]
		if containsCJK(a.Family + a.Given) {
			return true
		}
	}
	return containsCJK(p.Title)
}

// containsCJK 判断字符串是否含中日韩统一表意文字。
func containsCJK(s string) bool {
	for _, r := range s {
		if r >= 0x4E00 && r <= 0x9FFF {
			return true
		}
	}
	return false
}

func isThesis(docType string) bool {
	return strings.EqualFold(strings.TrimSpace(docType), model.DocTypeThesis)
}

// parseDate 把日期字符串转为 CSL 日期；无法解析时退化为 literal。
func parseDate(s string) map[string]interface{} {
	s = strings.TrimSpace(s)
	for _, layout := range []string{"2006-01-02", "2006/01/02", "2006-1-2", "2006/1/2", "2006年1月2日"} {
		if t, err := time.Parse(layout, s); err == nil {
			return map[string]interface{}{"date-parts": [][]int{{t.Year(), int(t.Month()), t.Day()}}}
		}
	}
	return map[string]interface{}{"literal": s}
}

// cleanPages 归一化页码：-- → -，去首尾空白。
func cleanPages(pages string) string {
	return strings.ReplaceAll(strings.TrimSpace(pages), "--", "-")
}
