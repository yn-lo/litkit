package lint

import "testing"

// TestSpec_Validate_forbiddenTerms 空 term 应被拒绝（R7.3 禁用字词校验）。
func TestSpec_Validate_forbiddenTerms(t *testing.T) {
	spec := DefaultSpec()
	spec.ForbiddenTerms = []ForbiddenTerm{{Term: "   "}}
	if err := spec.Validate(); err == nil {
		t.Error("forbidden_terms 空 term 应校验失败")
	}
	spec2 := DefaultSpec()
	spec2.ForbiddenTerms = []ForbiddenTerm{{Term: "Ⓡ", Note: ""}}
	if err := spec2.Validate(); err != nil {
		t.Errorf("合法 forbidden_terms 不应失败: %v", err)
	}
}
