package core

import (
	"encoding/json"
	"testing"

	"litkit/internal/model"
)

// TestAddManualPaper_valid 合法输入构造 Paper（摘要必填、source=manual、ID 稳定）。
func TestAddManualPaper_valid(t *testing.T) {
	in := ManualPaperInput{
		Title:    "手动录入的测试文献",
		Authors:  []ManualAuthor{{Family: "张三"}, {Family: "Zhang", Given: "San"}},
		Abstract: "  这是手动录入的摘要内容。  ",
		Year:     2024,
		DOI:      "10.1000/manual",
		DocType:  model.DocTypeArticle,
	}
	p, err := AddManualPaper(in)
	if err != nil {
		t.Fatalf("AddManualPaper: %v", err)
	}
	if p.Source != SourceManual {
		t.Errorf("source 应为 manual，got %q", p.Source)
	}
	if p.Abstract != "这是手动录入的摘要内容。" {
		t.Errorf("摘要应 trim，got %q", p.Abstract)
	}
	if len(p.Authors) != 2 || p.Authors[0].Family != "张三" || p.Authors[1].Given != "San" {
		t.Errorf("作者应正确映射，got %+v", p.Authors)
	}
	if p.ID != p.ComputeID() {
		t.Error("ID 应由 ComputeID 生成（DOI 优先）")
	}
}

// TestAddManualPaper_missingAbstract 摘要必填（摘要工作流：无摘要不入库）。
func TestAddManualPaper_missingAbstract(t *testing.T) {
	in := ManualPaperInput{Title: "无摘要文献"}
	if _, err := AddManualPaper(in); err == nil {
		t.Error("摘要缺失应报错")
	}
	in = ManualPaperInput{Title: "无摘要文献", Abstract: "   "}
	if _, err := AddManualPaper(in); err == nil {
		t.Error("纯空白摘要应报错")
	}
}

// TestAddManualPaper_missingTitle 标题必填（入库去重与引用都需要）。
func TestAddManualPaper_missingTitle(t *testing.T) {
	in := ManualPaperInput{Abstract: "摘要内容。"}
	if _, err := AddManualPaper(in); err == nil {
		t.Error("标题缺失应报错")
	}
}

// TestAddManualPaper_invalidYear 年份超出合理范围报错。
func TestAddManualPaper_invalidYear(t *testing.T) {
	in := ManualPaperInput{Title: "文献", Abstract: "摘要", Year: 3000}
	if _, err := AddManualPaper(in); err == nil {
		t.Error("year=3000 应报错")
	}
}

// TestAddManualPaper_invalidDocType 非法 docType 回退 article。
func TestAddManualPaper_invalidDocType(t *testing.T) {
	in := ManualPaperInput{Title: "文献", Abstract: "摘要", DocType: "typo"}
	p, err := AddManualPaper(in)
	if err != nil {
		t.Fatalf("AddManualPaper: %v", err)
	}
	if p.DocType != model.DocTypeArticle {
		t.Errorf("非法 docType 应回退 article，got %q", p.DocType)
	}
}

// TestManualAuthor_unmarshal 作者兼容字符串与对象两种 JSON 形式。
func TestManualAuthor_unmarshal(t *testing.T) {
	var a ManualAuthor
	if err := json.Unmarshal([]byte(`"张三"`), &a); err != nil || a.Family != "张三" {
		t.Errorf("字符串作者应映射到 family，got %+v err=%v", a, err)
	}
	if err := json.Unmarshal([]byte(`{"family":"Zhang","given":"San"}`), &a); err != nil || a.Given != "San" {
		t.Errorf("对象作者应解析，got %+v err=%v", a, err)
	}
}
