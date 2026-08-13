package lint

import (
	"strings"
	"testing"
)

// fixContent 对内容应用全部可修规则的自动修正。
func fixContent(content string) (string, FixReport) {
	return ApplyFixes(content, FixableRules())
}

func TestFix_R31_HalfWidthPunct(t *testing.T) {
	got, rep := fixContent("中文,内容。中文;分号。\n")
	if !strings.Contains(got, "中文，内容。中文；分号。") {
		t.Errorf("半角标点应转全角，got %q", got)
	}
	if rep.Applied["R3.1"] != 1 {
		t.Errorf("R3.1 应修正 1 行，got %v", rep.Applied)
	}
	if !strings.Contains(got, "中文，内容") {
		t.Errorf("逗号应转全角，got %q", got)
	}
}

func TestFix_R32_StraightQuote(t *testing.T) {
	got, rep := fixContent("他说\"你好\"。\n")
	if !strings.Contains(got, "他说“你好”。") {
		t.Errorf("直引号应转弯引号，got %q", got)
	}
	if rep.Applied["R3.2"] != 1 {
		t.Errorf("R3.2 应修正 1 行，got %v", rep.Applied)
	}
}

func TestFix_R21_PValue(t *testing.T) {
	cases := []struct{ in, want string }{
		{"结果显示 p=.03 显著。\n", "结果显示 P=0.03 显著。\n"},
		{"结果显示 p=0.3 显著。\n", "结果显示 P=0.30 显著。\n"},
		{"结果显示 P=0.0003 显著。\n", "结果显示 P<0.001 显著。\n"},
		{"结果显示 P=0.03 显著。\n", "结果显示 P=0.03 显著。\n"}, // 合规不变
	}
	for _, c := range cases {
		got, _ := fixContent(c.in)
		if got != c.want {
			t.Errorf("%q → %q，期望 %q", c.in, got, c.want)
		}
	}
}

func TestFix_R14_Bold(t *testing.T) {
	got, _ := fixContent("正文含**加粗**内容。\n")
	if !strings.Contains(got, "正文含加粗内容。") {
		t.Errorf("加粗标记应删除保留内容，got %q", got)
	}
}

func TestFix_R71_HeadingColon(t *testing.T) {
	got, _ := fixContent("# 方法：实验设计\n")
	if !strings.Contains(got, "# 方法实验设计") {
		t.Errorf("标题冒号应删除，got %q", got)
	}
}

func TestFix_R12_HeadingEndPunct(t *testing.T) {
	got, _ := fixContent("# 结果。\n")
	if !strings.Contains(got, "# 结果\n") {
		t.Errorf("标题末尾标点应删除，got %q", got)
	}
}

func TestFix_R61_CitePosition(t *testing.T) {
	got, _ := fixContent("结论成立。[@smith2020]\n")
	if !strings.Contains(got, "结论成立[@smith2020]。") {
		t.Errorf("引用应移到标点前，got %q", got)
	}
}

func TestFix_R33_NumberRange(t *testing.T) {
	cases := []struct{ in, want string }{
		{"约占10-16%左右。\n", "约占10%～16%左右。\n"},
		{"体重10kg～15kg。\n", "体重10～15kg。\n"},
		{"1988年～1998年。\n", "1988—1998年。\n"},
		{"创伤后24—48小时。\n", "创伤后24～48小时。\n"},
		{"剂量为5 mg/kg/d。\n", "剂量为5 mg/(kg·d)。\n"},
	}
	for _, c := range cases {
		got, _ := fixContent(c.in)
		if got != c.want {
			t.Errorf("%q → %q，期望 %q", c.in, got, c.want)
		}
	}
}

func TestFix_ComposesAcrossRules(t *testing.T) {
	// 多行分别命中不同规则（引号挡在汉字与标点之间时 R3.1 不检测，属规则本身范围）
	got, rep := fixContent("他说\"你好\"。\n中文,朋友。\n")
	if !strings.Contains(got, "他说“你好”。") || !strings.Contains(got, "中文，朋友。") {
		t.Errorf("多规则应组合修正，got %q", got)
	}
	if rep.Applied["R3.1"] != 1 || rep.Applied["R3.2"] != 1 {
		t.Errorf("应同时修正 R3.1/R3.2，got %v", rep.Applied)
	}
}

func TestFix_UnfixableRulesNotIncluded(t *testing.T) {
	// 不可修规则（如 R7.2 自我夸大）不应出现在 FixableRules
	for _, r := range FixableRules() {
		if r.ID == "R7.2" || r.ID == "R1.5" {
			t.Errorf("不可修规则 %s 不应在 FixableRules", r.ID)
		}
	}
}

func TestFix_BOMPreserved(t *testing.T) {
	// 含 BOM 的文件：标题行规则（R1.2/R7.1）也应生效，BOM 保留
	in := "\uFEFF# 方法：设计。\n正文。\n"
	got, rep := fixContent(in)
	if !strings.HasPrefix(got, "\uFEFF") {
		t.Error("BOM 应保留")
	}
	if !strings.Contains(got, "# 方法设计\n") {
		t.Errorf("标题冒号与末尾标点应删除，got %q", got)
	}
	if rep.Applied["R7.1"] != 1 || rep.Applied["R1.2"] != 1 {
		t.Errorf("标题行规则应生效，got %v", rep.Applied)
	}
}
