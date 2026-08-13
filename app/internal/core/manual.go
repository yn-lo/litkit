package core

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"litkit/internal/model"
)

// SourceManual 手动录入文献的来源标识（lib stats / list --source 可区分）。
const SourceManual = "manual"

// ManualAuthor 手动录入的作者：兼容字符串（"张三" → Family）与对象（{"family","given"}）两种 JSON 形式。
type ManualAuthor struct {
	Family string `json:"family"`
	Given  string `json:"given"`
}

// UnmarshalJSON 兼容字符串与对象两种形式。
func (a *ManualAuthor) UnmarshalJSON(data []byte) error {
	var s string
	if err := json.Unmarshal(data, &s); err == nil {
		a.Family = s
		return nil
	}
	type raw ManualAuthor
	var r raw
	if err := json.Unmarshal(data, &r); err != nil {
		return fmt.Errorf("manual author: %w", err)
	}
	a.Family, a.Given = r.Family, r.Given
	return nil
}

// ManualPaperInput 手动录入的文献元数据（title/abstract 必填）。
type ManualPaperInput struct {
	Title       string         `json:"title"`
	Authors     []ManualAuthor `json:"authors"`
	Abstract    string         `json:"abstract"`
	Year        int            `json:"year"`
	Venue       string         `json:"venue"`
	DOI         string         `json:"doi"`
	PMID        string         `json:"pmid"`
	ArXivID     string         `json:"arxivId"`
	URL         string         `json:"url"`
	DocType     string         `json:"docType"`
	Volume      string         `json:"volume"`
	Number      string         `json:"number"`
	Pages       string         `json:"pages"`
	Publisher   string         `json:"publisher"`
	City        string         `json:"city"`
	Institution string         `json:"institution"`
}

// AddManualPaper 校验并构造手动录入的文献元数据（信任边界输入校验）。
// 非法 docType 回退 article；year 须在 [0, 2100]。
func AddManualPaper(in ManualPaperInput) (model.Paper, error) {
	if strings.TrimSpace(in.Title) == "" {
		return model.Paper{}, errors.New("manual: title 必填")
	}
	if strings.TrimSpace(in.Abstract) == "" {
		return model.Paper{}, errors.New("manual: abstract 必填（入库文献必须携带摘要）")
	}
	if in.Year < 0 || in.Year > 2100 {
		return model.Paper{}, fmt.Errorf("manual: year %d 超出合理范围 [0, 2100]", in.Year)
	}
	docType := in.DocType
	if !validDocType(docType) {
		docType = model.DocTypeArticle
	}
	authors := make([]model.Author, len(in.Authors))
	for i, a := range in.Authors {
		authors[i] = model.Author{Family: strings.TrimSpace(a.Family), Given: strings.TrimSpace(a.Given)}
	}
	p := model.Paper{
		Title:       strings.TrimSpace(in.Title),
		Authors:     authors,
		Abstract:    strings.TrimSpace(in.Abstract),
		Year:        in.Year,
		Venue:       in.Venue,
		DOI:         strings.TrimSpace(in.DOI),
		PMID:        in.PMID,
		ArXivID:     in.ArXivID,
		URL:         in.URL,
		DocType:     docType,
		Volume:      in.Volume,
		Number:      in.Number,
		Pages:       in.Pages,
		Publisher:   in.Publisher,
		City:        in.City,
		Institution: in.Institution,
		Source:      SourceManual,
	}
	p.ID = p.ComputeID()
	return p, nil
}

// validDocType 判断文献类型是否为 model 包注册的枚举之一。
func validDocType(dt string) bool {
	switch dt {
	case "", model.DocTypeArticle, model.DocTypeJournalArticle, model.DocTypePreprint,
		model.DocTypeDataset, model.DocTypeBook, model.DocTypeMonograph,
		model.DocTypeConference, model.DocTypeProceedings, model.DocTypeReport,
		model.DocTypeThesis, model.DocTypePatent, model.DocTypeSoftware, model.DocTypeWebpage:
		return true
	}
	return false
}
