package lint

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// runContent 写临时 md 文件并执行验证（ParseSource + Run 集成）。
func runContent(t *testing.T, content string, spec *ManuscriptSpec, opts Options) FileReport {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "doc.md")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	src, err := ParseSource(path)
	if err != nil {
		t.Fatalf("ParseSource: %v", err)
	}
	return Run(src, spec, opts)
}

// violationsOf 过滤出指定规则的违规。
func violationsOf(fr FileReport, id string) []Violation {
	var out []Violation
	for _, v := range fr.Violations {
		if v.RuleID == id {
			out = append(out, v)
		}
	}
	return out
}

func bodyHas(src *Source, want string) bool {
	for _, ln := range src.Body {
		if ln == want {
			return true
		}
	}
	return false
}

func zhChapter() Options { return Options{Lang: "zh", Mode: ModeChapter} }
func zhDraft() Options   { return Options{Lang: "zh", Mode: ModeDraft} }
func zhFinal() Options   { return Options{Lang: "zh", Mode: ModeFinal} }

func TestParseSource(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "doc.md")
	content := "# 标题\n\n正文一行。\n\n```go\ncode line\n```\n\n| 表头 | 值 |\n|---|---|\n\n# 参考文献\n\n[1] 某文献\n"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	src, err := ParseSource(path)
	if err != nil {
		t.Fatalf("ParseSource: %v", err)
	}
	if !src.HasRefs {
		t.Error("应检测到参考文献段")
	}
	if !bodyHas(src, "# 标题") || !bodyHas(src, "正文一行。") {
		t.Errorf("Body 应含标题与正文行，got %v", src.Body)
	}
	if bodyHas(src, "code line") {
		t.Error("Body 不应含代码块内容")
	}
	if bodyHas(src, "[1] 某文献") {
		t.Error("Body 不应含参考文献内容")
	}
	for _, ln := range src.Body {
		if strings.HasPrefix(strings.TrimSpace(ln), "|") {
			t.Errorf("Body 不应含表格行：%q", ln)
		}
	}
}

func TestParseSource_BOMAndRefsHeading(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "doc.md")
	// 含 UTF-8 BOM；参考文献标题带附加文字（包含匹配）
	content := "\uFEFF# 标题\n\n正文。\n\n# 主要参考文献（References）\n\n[1] 某文献\n"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	src, err := ParseSource(path)
	if err != nil {
		t.Fatalf("ParseSource: %v", err)
	}
	if !bodyHas(src, "# 标题") {
		t.Errorf("应去除 BOM，首行为标题，got %v", src.Body)
	}
	if !src.HasRefs || bodyHas(src, "[1] 某文献") {
		t.Errorf("含参考文献字样的标题应被识别并排除，got HasRefs=%v Body=%v", src.HasRefs, src.Body)
	}
}

func TestRule_R0_1(t *testing.T) {
	pos := "This is an English sentence with many words here and more words overall.\nAnother English line with plenty of words to push the ratio high enough.\n"
	fr := runContent(t, pos, DefaultSpec(), zhChapter())
	if got := violationsOf(fr, "R0.1"); len(got) != 1 {
		t.Errorf("英文占比高应违规 1 条，got %d", len(got))
	}
	neg := "这是一段中文内容，用于测试全文中文规则是否通过。\n"
	fr = runContent(t, neg, DefaultSpec(), zhChapter())
	if got := violationsOf(fr, "R0.1"); len(got) != 0 {
		t.Errorf("纯中文不应违规，got %v", got)
	}
}

func TestRule_R0_2(t *testing.T) {
	fr := runContent(t, "# This Is An English Heading\n", DefaultSpec(), zhChapter())
	got := violationsOf(fr, "R0.2")
	if len(got) != 1 || got[0].Line != 1 {
		t.Errorf("英文标题应在第 1 行违规，got %v", got)
	}
	fr = runContent(t, "# 中文标题\n", DefaultSpec(), zhChapter())
	if got := violationsOf(fr, "R0.2"); len(got) != 0 {
		t.Errorf("中文标题不应违规，got %v", got)
	}
}

func TestRule_R1_1(t *testing.T) {
	fr := runContent(t, "##### 五级标题\n", DefaultSpec(), zhChapter())
	got := violationsOf(fr, "R1.1")
	if len(got) != 1 || got[0].Line != 1 {
		t.Errorf("五级标题应在第 1 行违规，got %v", got)
	}
	fr = runContent(t, "## 二级标题\n", DefaultSpec(), zhChapter())
	if got := violationsOf(fr, "R1.1"); len(got) != 0 {
		t.Errorf("二级标题不应违规，got %v", got)
	}
}

func TestRule_R1_2(t *testing.T) {
	fr := runContent(t, "# 这是一个非常非常非常非常非常非常非常长的标题\n", DefaultSpec(), zhChapter())
	if got := violationsOf(fr, "R1.2"); len(got) == 0 {
		t.Error("超长标题应违规")
	}
	fr = runContent(t, "# 结果。\n", DefaultSpec(), zhChapter())
	got := violationsOf(fr, "R1.2")
	if len(got) != 1 || got[0].Line != 1 {
		t.Errorf("末尾标点应在第 1 行违规，got %v", got)
	}
	fr = runContent(t, "# 结果\n", DefaultSpec(), zhChapter())
	if got := violationsOf(fr, "R1.2"); len(got) != 0 {
		t.Errorf("规范标题不应违规，got %v", got)
	}
}

func TestRule_R1_4(t *testing.T) {
	fr := runContent(t, "这是正文含**加粗**内容。\n", DefaultSpec(), zhDraft())
	got := violationsOf(fr, "R1.4")
	if len(got) != 1 || got[0].Line != 1 {
		t.Errorf("加粗应在第 1 行违规，got %v", got)
	}
	fr = runContent(t, "这是普通正文。\n", DefaultSpec(), zhDraft())
	if got := violationsOf(fr, "R1.4"); len(got) != 0 {
		t.Errorf("无加粗不应违规，got %v", got)
	}
}

func TestRule_R2_1(t *testing.T) {
	cases := []struct {
		content string
		want    int // 期望 R2.1 违规数
	}{
		{"结果显示 p=0.03 显著。\n", 1},   // 小写 p
		{"结果显示 P=0.3 显著。\n", 1},    // 小数位不足
		{"结果显示 P=.03 显著。\n", 1},    // 缺前导零
		{"结果显示 P=0.0003 显著。\n", 1}, // 应写 P<0.001
		{"结果显示 P=0.03 显著。\n", 0},   // 正确
		{"结果显示 P<0.001 显著。\n", 0},  // 正确
		{"结果显示 P=0.006 显著。\n", 0},  // 正确（3 位）
	}
	for _, c := range cases {
		fr := runContent(t, c.content, DefaultSpec(), zhDraft())
		if got := len(violationsOf(fr, "R2.1")); got != c.want {
			t.Errorf("%q 期望 %d 条违规，got %d：%v", c.content, c.want, got, violationsOf(fr, "R2.1"))
		}
	}
}

func TestRule_R3_1(t *testing.T) {
	fr := runContent(t, "中文,内容\n", DefaultSpec(), zhDraft())
	got := violationsOf(fr, "R3.1")
	if len(got) != 1 || got[0].Line != 1 {
		t.Errorf("半角逗号应在第 1 行违规，got %v", got)
	}
	fr = runContent(t, "中文，内容\n", DefaultSpec(), zhDraft())
	if got := violationsOf(fr, "R3.1"); len(got) != 0 {
		t.Errorf("全角逗号不应违规，got %v", got)
	}
}

func TestRule_R3_2(t *testing.T) {
	fr := runContent(t, "他说\"你好\"。\n", DefaultSpec(), zhDraft())
	got := violationsOf(fr, "R3.2")
	if len(got) != 1 || got[0].Line != 1 {
		t.Errorf("直引号应在第 1 行违规，got %v", got)
	}
	fr = runContent(t, "他说“你好”。\n", DefaultSpec(), zhDraft())
	if got := violationsOf(fr, "R3.2"); len(got) != 0 {
		t.Errorf("弯引号不应违规，got %v", got)
	}
}

func TestRule_R5_1(t *testing.T) {
	fr := runContent(t, "已有研究表明该结论成立 (Smith, 2020)。\n", DefaultSpec(), zhDraft())
	if got := violationsOf(fr, "R5.1"); len(got) != 1 {
		t.Errorf("(Author, 2024) 模式应违规，got %v", got)
	}
	fr = runContent(t, "已有研究表明该结论成立[1]。\n", DefaultSpec(), zhDraft())
	if got := violationsOf(fr, "R5.1"); len(got) != 1 {
		t.Errorf("[数字] 模式应违规，got %v", got)
	}
	fr = runContent(t, "已有研究表明该结论成立[@smith2020]\n", DefaultSpec(), zhDraft())
	if got := violationsOf(fr, "R5.1"); len(got) != 0 {
		t.Errorf("[@] 不应违规，got %v", got)
	}
}

func TestRule_R5_2(t *testing.T) {
	fr := runContent(t, "此处需要补充证据[待引证]。\n", DefaultSpec(), zhDraft())
	got := violationsOf(fr, "R5.2")
	if len(got) != 1 || got[0].Line != 1 {
		t.Errorf("待引证标记应在第 1 行违规，got %v", got)
	}
	fr = runContent(t, "此处证据充分[@key2021]。\n", DefaultSpec(), zhDraft())
	if got := violationsOf(fr, "R5.2"); len(got) != 0 {
		t.Errorf("无标记不应违规，got %v", got)
	}
}

func TestRule_R6_1(t *testing.T) {
	// 违规：标点后紧跟引用
	fr := runContent(t, "结论成立。[@smith2020]\n", DefaultSpec(), zhDraft())
	got := violationsOf(fr, "R6.1")
	if len(got) != 1 || got[0].Line != 1 {
		t.Errorf("标点后引用应在第 1 行违规，got %v", got)
	}
	// 正确：引用在标点前
	fr = runContent(t, "结论成立[@smith2020]。\n", DefaultSpec(), zhDraft())
	if got := violationsOf(fr, "R6.1"); len(got) != 0 {
		t.Errorf("引用在标点前不应违规，got %v", got)
	}
}

func TestRule_R7_1(t *testing.T) {
	fr := runContent(t, "# 方法：实验设计\n", DefaultSpec(), zhChapter())
	got := violationsOf(fr, "R7.1")
	if len(got) != 1 || got[0].Line != 1 {
		t.Errorf("标题冒号应在第 1 行违规，got %v", got)
	}
	fr = runContent(t, "# 方法与实验设计\n", DefaultSpec(), zhChapter())
	if got := violationsOf(fr, "R7.1"); len(got) != 0 {
		t.Errorf("无冒号不应违规，got %v", got)
	}
}

func TestRule_R7_2(t *testing.T) {
	fr := runContent(t, "本研究首次证实了该假设。\n", DefaultSpec(), zhDraft())
	got := violationsOf(fr, "R7.2")
	if len(got) != 1 || got[0].Line != 1 {
		t.Errorf("夸大词应在第 1 行违规，got %v", got)
	}
	fr = runContent(t, "本研究验证了该假设。\n", DefaultSpec(), zhDraft())
	if got := violationsOf(fr, "R7.2"); len(got) != 0 {
		t.Errorf("客观表述不应违规，got %v", got)
	}
}

func TestRule_R9_1(t *testing.T) {
	fr := runContent(t, "【注意】此处待修改。\n", DefaultSpec(), zhChapter())
	got := violationsOf(fr, "R9.1")
	if len(got) != 1 || got[0].Line != 1 {
		t.Errorf("用户标记应在第 1 行违规，got %v", got)
	}
	fr = runContent(t, "此处待修改。\n", DefaultSpec(), zhChapter())
	if got := violationsOf(fr, "R9.1"); len(got) != 0 {
		t.Errorf("无标记不应违规，got %v", got)
	}
}

func TestRule_R4_2(t *testing.T) {
	fr := runContent(t, "我们对数据进行了分析。\n", DefaultSpec(), zhFinal())
	got := violationsOf(fr, "R4.2")
	if len(got) != 1 || got[0].Suggestion != "需人工确认是否冗余" {
		t.Errorf("冗余句式应违规且提示人工确认，got %v", got)
	}
	fr = runContent(t, "我们分析了数据。\n", DefaultSpec(), zhFinal())
	if got := violationsOf(fr, "R4.2"); len(got) != 0 {
		t.Errorf("简洁句式不应违规，got %v", got)
	}
}

func TestRule_R5_3(t *testing.T) {
	spec := DefaultSpec()
	spec.Citation.Count = []int{2, 5}
	// 总数不足
	fr := runContent(t, "引用一篇[@a]。\n", spec, zhFinal())
	if got := violationsOf(fr, "R5.3"); len(got) == 0 {
		t.Error("引用总数不足应违规")
	}
	// 单行 >3
	fr = runContent(t, "[@a][@b][@c][@d]\n", spec, zhFinal())
	got := violationsOf(fr, "R5.3")
	if len(got) != 1 || got[0].Line != 1 {
		t.Errorf("单行超 3 引用应在第 1 行违规，got %v", got)
	}
	// 合规
	fr = runContent(t, "[@a][@b]\n\n[@c]\n", spec, zhFinal())
	if got := violationsOf(fr, "R5.3"); len(got) != 0 {
		t.Errorf("引用密度合规不应违规，got %v", got)
	}
	// 同一 key 引用 3 处仅计 1 篇：去重后 1 < 2，总数不足违规（无行号）
	fr = runContent(t, "第一处[@a]，第二处[@a]。\n\n第三处[@a]。\n", spec, zhFinal())
	got = violationsOf(fr, "R5.3")
	if len(got) != 1 || got[0].Line != 0 {
		t.Errorf("同一 key 应去重计 1 篇而总数不足违规，got %v", got)
	}
}

func TestRule_R8_1(t *testing.T) {
	fr := runContent(t, "短文。\n", DefaultSpec(), zhFinal())
	if got := violationsOf(fr, "R8.1"); len(got) != 1 {
		t.Errorf("全文过短应违规，got %v", got)
	}
	fr = runContent(t, strings.Repeat("字", 4000)+"\n", DefaultSpec(), zhFinal())
	if got := violationsOf(fr, "R8.1"); len(got) != 0 {
		t.Errorf("全文字数合规不应违规，got %v", got)
	}
}

func TestRule_R8_2(t *testing.T) {
	pos := "# 摘要\n\n短摘要。\n\n# 引言\n\n正文内容。\n"
	fr := runContent(t, pos, DefaultSpec(), zhFinal())
	got := violationsOf(fr, "R8.2")
	if len(got) != 1 || got[0].Line != 1 {
		t.Errorf("摘要过短应在第 1 行违规，got %v", got)
	}
	neg := "# 摘要\n\n" + strings.Repeat("字", 300) + "\n\n# 引言\n\n正文。\n"
	fr = runContent(t, neg, DefaultSpec(), zhFinal())
	if got := violationsOf(fr, "R8.2"); len(got) != 0 {
		t.Errorf("摘要字数合规不应违规，got %v", got)
	}
}

func TestRule_R8_3(t *testing.T) {
	fr := runContent(t, "短段落。\n", DefaultSpec(), zhFinal())
	got := violationsOf(fr, "R8.3")
	if len(got) != 1 || got[0].Line != 1 {
		t.Errorf("段落过短应在第 1 行违规，got %v", got)
	}
	fr = runContent(t, strings.Repeat("字", 100)+"\n", DefaultSpec(), zhFinal())
	if got := violationsOf(fr, "R8.3"); len(got) != 0 {
		t.Errorf("段落字数合规不应违规，got %v", got)
	}
}

func TestRun_modeFilter(t *testing.T) {
	content := "正文含**加粗**内容,中文。\n"
	// chapter 模式不跑 draft 规则（R1.4 加粗 / R3.1 半角）
	fr := runContent(t, content, DefaultSpec(), zhChapter())
	if got := violationsOf(fr, "R1.4"); len(got) != 0 {
		t.Errorf("chapter 不应跑 R1.4，got %v", got)
	}
	if got := violationsOf(fr, "R3.1"); len(got) != 0 {
		t.Errorf("chapter 不应跑 R3.1，got %v", got)
	}
	// draft 模式启用
	fr = runContent(t, content, DefaultSpec(), zhDraft())
	if got := violationsOf(fr, "R1.4"); len(got) == 0 {
		t.Error("draft 应跑 R1.4")
	}
	if got := violationsOf(fr, "R3.1"); len(got) == 0 {
		t.Error("draft 应跑 R3.1")
	}
}

func TestRun_langFilter(t *testing.T) {
	content := "This is English text with many words to trigger the ratio check here.\n"
	// en 模式不跑 zh 专属规则 R0.1
	fr := runContent(t, content, DefaultSpec(), Options{Lang: "en", Mode: ModeChapter})
	if got := violationsOf(fr, "R0.1"); len(got) != 0 {
		t.Errorf("en 不应跑 R0.1，got %v", got)
	}
	// zh 模式启用
	fr = runContent(t, content, DefaultSpec(), zhChapter())
	if got := violationsOf(fr, "R0.1"); len(got) == 0 {
		t.Error("zh 应跑 R0.1 并违规")
	}
}

func TestRun_skipAndOnly(t *testing.T) {
	content := "# 标题：冒号\n\n【注意】内容。\n"
	// Only：仅跑 R9.1
	opts := zhChapter()
	opts.Only = []string{"R9.1"}
	fr := runContent(t, content, DefaultSpec(), opts)
	if got := violationsOf(fr, "R9.1"); len(got) == 0 {
		t.Error("Only 应保留 R9.1")
	}
	if got := violationsOf(fr, "R7.1"); len(got) != 0 {
		t.Errorf("Only 应排除 R7.1，got %v", got)
	}
	// Skip：跳过 R9.1
	opts = zhChapter()
	opts.Skip = []string{"R9.1"}
	fr = runContent(t, content, DefaultSpec(), opts)
	if got := violationsOf(fr, "R9.1"); len(got) != 0 {
		t.Errorf("Skip 应排除 R9.1，got %v", got)
	}
	if got := violationsOf(fr, "R7.1"); len(got) == 0 {
		t.Error("Skip 不应影响 R7.1")
	}
}

func TestRunFiles_exitHint(t *testing.T) {
	dir := t.TempDir()
	// A 类违规文件
	aPath := filepath.Join(dir, "a.md")
	if err := os.WriteFile(aPath, []byte("【注意】内容。\n"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	rep, err := RunFiles([]string{aPath}, DefaultSpec(), zhChapter())
	if err != nil {
		t.Fatalf("RunFiles: %v", err)
	}
	if rep.ExitHint != "fix_and_rerun" || rep.Passed {
		t.Errorf("A 类违规应 fix_and_rerun，got %s passed=%v", rep.ExitHint, rep.Passed)
	}
	if len(rep.ManualChecklist) == 0 {
		t.Error("应固定输出人工核对清单")
	}
}
