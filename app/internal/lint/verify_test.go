package lint

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"litkit/internal/model"
	"litkit/internal/storage"
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

func zhChapter() Options {
	return Options{Lang: "zh", Mode: ModeChapter, PaperType: PaperTypeEmpirical}
}
func zhDraft() Options { return Options{Lang: "zh", Mode: ModeDraft, PaperType: PaperTypeEmpirical} }
func zhFinal() Options { return Options{Lang: "zh", Mode: ModeFinal, PaperType: PaperTypeEmpirical} }

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

// TestRule_R7_2_customBoastWords 自定义词表覆盖默认词表。
func TestRule_R7_2_customBoastWords(t *testing.T) {
	spec := DefaultSpec()
	spec.BoastWords = []string{"自定义夸大词"} // 仅含自定义词
	// 默认词表中的"首次证实"不应再触发
	fr := runContent(t, "本研究首次证实了该假设。\n", spec, zhDraft())
	if got := violationsOf(fr, "R7.2"); len(got) != 0 {
		t.Errorf("自定义词表应覆盖默认，'首次证实'不应违规，got %v", got)
	}
	// 自定义词应触发
	fr = runContent(t, "这是自定义夸大词的表述。\n", spec, zhDraft())
	if got := violationsOf(fr, "R7.2"); len(got) != 1 {
		t.Errorf("自定义夸大词应违规，got %v", got)
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
	// 标题行不应计为段落（R8.3 只检内容段落）
	fr = runContent(t, "## 引言\n\n短段落。\n", DefaultSpec(), zhFinal())
	got = violationsOf(fr, "R8.3")
	if len(got) != 1 || got[0].Line != 3 {
		t.Errorf("标题行不应触发 R8.3，仅内容段落短应违规，got %v", got)
	}
	fr = runContent(t, "## 引言\n\n# 结论\n", DefaultSpec(), zhFinal())
	if got := violationsOf(fr, "R8.3"); len(got) != 0 {
		t.Errorf("纯标题行不应触发 R8.3，got %v", got)
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
	fr := runContent(t, content, DefaultSpec(), Options{Lang: "en", Mode: ModeChapter, PaperType: PaperTypeEmpirical})
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

func TestRun_checkAndSkipCheck(t *testing.T) {
	content := "# 标题：冒号\n\n【注意】内容。\n"
	// --check=todo：仅跑 todo 类别（R9.1）
	opts := zhChapter()
	opts.OnlyCategories = []Category{CatTodo}
	fr := runContent(t, content, DefaultSpec(), opts)
	if got := violationsOf(fr, "R9.1"); len(got) == 0 {
		t.Error("--check=todo 应保留 R9.1")
	}
	if got := violationsOf(fr, "R7.1"); len(got) != 0 {
		t.Errorf("--check=todo 应排除 R7.1（heading 类别），got %v", got)
	}
	// --skip-check=todo：跳过 todo 类别
	opts = zhChapter()
	opts.SkipCategories = []Category{CatTodo}
	fr = runContent(t, content, DefaultSpec(), opts)
	if got := violationsOf(fr, "R9.1"); len(got) != 0 {
		t.Errorf("--skip-check=todo 应排除 R9.1，got %v", got)
	}
	if got := violationsOf(fr, "R7.1"); len(got) == 0 {
		t.Error("--skip-check=todo 不应影响 R7.1")
	}
	// --check=citation,word_counts：仅跑引用+字数类别
	opts = zhFinal()
	opts.OnlyCategories = []Category{CatCitation, CatWordCounts}
	fr = runContent(t, "短文。\n", DefaultSpec(), opts)
	// R8.1（word_counts）应跑
	if got := violationsOf(fr, "R8.1"); len(got) == 0 {
		t.Error("--check=citation,word_counts 应保留 R8.1")
	}
	// R9.1（todo）不应跑
	if got := violationsOf(fr, "R9.1"); len(got) != 0 {
		t.Errorf("--check=citation,word_counts 应排除 R9.1，got %v", got)
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

func TestRun_typeFilter(t *testing.T) {
	// R2.1 P值格式仅 empirical；review 不应跑
	content := "结果显示 p=0.03 显著。\n" // 小写 p → R2.1 违规
	fr := runContent(t, content, DefaultSpec(), Options{Lang: "zh", Mode: ModeDraft, PaperType: PaperTypeReview})
	if got := violationsOf(fr, "R2.1"); len(got) != 0 {
		t.Errorf("review 不应跑 R2.1（P值格式），got %v", got)
	}
	// empirical 应跑
	fr = runContent(t, content, DefaultSpec(), Options{Lang: "zh", Mode: ModeDraft, PaperType: PaperTypeEmpirical})
	if got := violationsOf(fr, "R2.1"); len(got) == 0 {
		t.Error("empirical 应跑 R2.1")
	}
	// PaperType 空=不过滤（兼容旧调用）
	fr = runContent(t, content, DefaultSpec(), Options{Lang: "zh", Mode: ModeDraft})
	if got := violationsOf(fr, "R2.1"); len(got) == 0 {
		t.Error("PaperType 空=不过滤，应跑 R2.1")
	}
}

func TestRunFiles_autoPaperType(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "doc.md")
	if err := os.WriteFile(path, []byte("结果显示 P=0.03 显著。\n"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	// review spec + 空 PaperType → 自动从 spec 取 review → R2.1 不跑
	reviewSpec := SpecForType(PaperTypeReview, LangZH)
	rep, err := RunFiles([]string{path}, reviewSpec, Options{Lang: "zh", Mode: ModeDraft})
	if err != nil {
		t.Fatalf("RunFiles: %v", err)
	}
	if got := violationsOf(rep.Files[0], "R2.1"); len(got) != 0 {
		t.Errorf("review spec 自动过滤 R2.1，got %v", got)
	}
}

func TestSpecForType_review(t *testing.T) {
	spec := SpecForType(PaperTypeReview, LangZH)
	if spec.WordCount.Total[0] != 3500 || spec.WordCount.Total[1] != 5000 {
		t.Errorf("review 全文应为 3500-5000，got %v", spec.WordCount.Total)
	}
	if spec.Citation.Count[0] != 20 || spec.Citation.Count[1] != 40 {
		t.Errorf("review 引用应为 20-40，got %v", spec.Citation.Count)
	}
	if spec.SectionList()[0] != "引言" || spec.SectionList()[1] != "文献检索方法" {
		t.Errorf("review 章节应含文献检索方法，got %v", spec.SectionList())
	}
}

func TestSpecForType_enDefault(t *testing.T) {
	spec := SpecForType(PaperTypeEmpirical, LangEN)
	if spec.Citation.Style != "apa" {
		t.Errorf("en 默认引用样式应为 apa，got %s", spec.Citation.Style)
	}
}

// ---- R5.6 引用存在性校验（引用防伪，查库）----

func newTestStore(t *testing.T) *storage.Store {
	t.Helper()
	s, err := storage.Open(filepath.Join(t.TempDir(), "litkit.db"))
	if err != nil {
		t.Fatalf("Open store: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

func TestCheckCiteKeys_ReportsMissingAndPresent(t *testing.T) {
	s := newTestStore(t)
	// 入库一篇，得到真实 citeKey
	realKey, _, err := s.UpsertPaper(model.Paper{Title: "Real", DOI: "10.1/real"})
	if err != nil {
		t.Fatalf("UpsertPaper: %v", err)
	}
	content := "正文引用真实文献[@aBc]与缺失文献[" + realKey + "]。\n"
	src := mustParse(t, content)
	violations := CheckCiteKeys(src, s)
	// 只报缺失的 [@aBc]，真实 key 不报
	if len(violations) != 1 {
		t.Fatalf("应只报 1 条缺失违规，got %d: %+v", len(violations), violations)
	}
	if violations[0].RuleID != ruleCiteExists {
		t.Errorf("RuleID 应为 %s，got %s", ruleCiteExists, violations[0].RuleID)
	}
	if !strings.Contains(violations[0].Problem, "aBc") {
		t.Errorf("Problem 应提及缺失 key aBc，got %q", violations[0].Problem)
	}
}

func TestCheckCiteKeys_NilStore_Skips(t *testing.T) {
	src := mustParse(t, "正文引用[@abc]。\n")
	if got := CheckCiteKeys(src, nil); len(got) != 0 {
		t.Errorf("store 为 nil 应跳过，got %v", got)
	}
}

func TestCheckCiteKeys_DedupePerKey(t *testing.T) {
	s := newTestStore(t)
	content := "第一处[@xyz]，第二处[@xyz]。\n"
	src := mustParse(t, content)
	violations := CheckCiteKeys(src, s)
	if len(violations) != 1 {
		t.Fatalf("同一缺失 key 应只报一次，got %d: %+v", len(violations), violations)
	}
}

func TestRunFilesWithStore_ExitHintPromotesToFix(t *testing.T) {
	s := newTestStore(t)
	dir := t.TempDir()
	path := filepath.Join(dir, "doc.md")
	// 内容本身合规（无 A 违规），但含缺失引用 → 应提升为 fix_and_rerun
	if err := os.WriteFile(path, []byte("这是正文，引用[@missing]文献。\n"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	rep, err := RunFilesWithStore([]string{path}, DefaultSpec(), Options{Lang: "zh", Mode: ModeDraft}, s)
	if err != nil {
		t.Fatalf("RunFilesWithStore: %v", err)
	}
	if rep.ExitHint != "fix_and_rerun" || rep.Passed {
		t.Errorf("缺失引用应 fix_and_rerun，got %s passed=%v", rep.ExitHint, rep.Passed)
	}
	if got := violationsOf(rep.Files[0], ruleCiteExists); len(got) == 0 {
		t.Error("报告应含 R5.6 违规")
	}
}

func TestRunFilesWithStore_NilStore_NoR56(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "doc.md")
	if err := os.WriteFile(path, []byte("这是正文，引用[@missing]文献。\n"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	rep, err := RunFilesWithStore([]string{path}, DefaultSpec(), Options{Lang: "zh", Mode: ModeDraft}, nil)
	if err != nil {
		t.Fatalf("RunFilesWithStore: %v", err)
	}
	if got := violationsOf(rep.Files[0], ruleCiteExists); len(got) != 0 {
		t.Errorf("store 为 nil 不应报 R5.6，got %v", got)
	}
}

func mustParse(t *testing.T, content string) *Source {
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
	return src
}

// ---- R1.3 标题编号（顺序 + 强制编号 + 层级对齐）----

// reviewRun 以综述类型运行验证（R1.3 仅 review 生效）。
func reviewRun(t *testing.T, content string) FileReport {
	t.Helper()
	return runContent(t, content, SpecForType(PaperTypeReview, LangZH),
		Options{Lang: "zh", Mode: ModeChapter, PaperType: PaperTypeReview})
}

func TestRule_R1_3_SkipTopLevel(t *testing.T) {
	// 章节跳号：1 → 3（漏 2）
	fr := reviewRun(t, "# 大标题\n# 1 引言\n正文。\n# 3 结论\n正文。\n")
	if got := violationsOf(fr, "R1.3"); len(got) == 0 {
		t.Error("章节跳号（1→3）应报 R1.3")
	}
}

func TestRule_R1_3_SkipSubLevel(t *testing.T) {
	// 子级跳号：1.1 → 1.3（漏 1.2）
	fr := reviewRun(t, "# 大标题\n# 1 引言\n正文。\n## 1.1 背景\n正文。\n## 1.3 现状\n正文。\n")
	if got := violationsOf(fr, "R1.3"); len(got) == 0 {
		t.Error("子级跳号（1.1→1.3）应报 R1.3")
	}
}

func TestRule_R1_3_HierarchyJump(t *testing.T) {
	// 层级错乱：1 → 1.1.1（跳两级）
	fr := reviewRun(t, "# 大标题\n# 1 引言\n正文。\n### 1.1.1 深层\n正文。\n")
	if got := violationsOf(fr, "R1.3"); len(got) == 0 {
		t.Error("层级跳变（1→1.1.1）应报 R1.3")
	}
	// 子级出现在错误父级下：1.1 → 2.1
	fr = reviewRun(t, "# 大标题\n# 1 引言\n正文。\n## 1.1 背景\n正文。\n## 2.1 错误子级\n正文。\n")
	if got := violationsOf(fr, "R1.3"); len(got) == 0 {
		t.Error("子级挂错父级（1.1→2.1）应报 R1.3")
	}
}

func TestRule_R1_3_Descending(t *testing.T) {
	// 倒序：1.2 → 1.1
	fr := reviewRun(t, "# 大标题\n# 1 引言\n正文。\n## 1.2 背景\n正文。\n## 1.1 倒序\n正文。\n")
	if got := violationsOf(fr, "R1.3"); len(got) == 0 {
		t.Error("编号倒序（1.2→1.1）应报 R1.3")
	}
}

func TestRule_R1_3_ValidSequence_Passes(t *testing.T) {
	// 合规序列：大标题 → 1 → 1.1 → 1.2 → 2 → 2.1 → 2.1.1
	content := "# 大标题\n# 1 引言\n正文。\n## 1.1 背景\n正文。\n## 1.2 现状\n正文。\n" +
		"# 2 方法\n正文。\n## 2.1 设计\n正文。\n### 2.1.1 流程\n正文。\n"
	if got := violationsOf(reviewRun(t, content), "R1.3"); len(got) != 0 {
		t.Errorf("合规编号序列不应报 R1.3，got %+v", got)
	}
}

func TestRule_R1_3_FirstNumberedChecks(t *testing.T) {
	// 首个编号章节非 1 开头
	fr := reviewRun(t, "# 大标题\n# 5 引言\n正文。\n")
	if got := violationsOf(fr, "R1.3"); len(got) == 0 {
		t.Error("首个顶层编号非 1 应报 R1.3")
	}
	// 首个编号章节为深层，缺少父级
	fr = reviewRun(t, "# 大标题\n## 1.1 背景\n正文。\n")
	if got := violationsOf(fr, "R1.3"); len(got) == 0 {
		t.Error("首个编号为子级应报 R1.3（缺少父级）")
	}
}

func TestRule_R1_3_TitleChecks(t *testing.T) {
	// 首个标题带编号 → 缺大标题
	fr := reviewRun(t, "# 1 引言\n正文。\n")
	if got := violationsOf(fr, "R1.3"); len(got) == 0 {
		t.Error("首个标题带编号应报 R1.3（缺大标题）")
	}
	// 无任何标题 → 缺大标题
	fr = reviewRun(t, "只有正文没有标题。\n")
	if got := violationsOf(fr, "R1.3"); len(got) == 0 {
		t.Error("无任何标题应报 R1.3（缺大标题）")
	}
	// 大标题（无编号）+ 合规编号章节 → 通过
	ok := "# 外科术后疼痛恐惧研究进展\n# 1 引言\n正文。\n# 2 结论\n正文。\n"
	if got := violationsOf(reviewRun(t, ok), "R1.3"); len(got) != 0 {
		t.Errorf("大标题+合规章节不应报 R1.3，got %+v", got)
	}
	// 大标题后的无编号标题仍报未编号（只有第一个无编号标题是大标题）
	fr = reviewRun(t, "# 大标题\n# 引言\n正文。\n")
	if got := violationsOf(fr, "R1.3"); len(got) == 0 {
		t.Error("大标题之后的无编号标题应报 R1.3（未编号）")
	}
}

func TestRule_R1_3_MissingNumbering(t *testing.T) {
	// require_numbering=true 时，大标题之后的无编号标题应报 R1.3（review.md 真实场景）
	content := "# 疼痛恐惧综述\n# 引言\n正文。\n## 概念与理论框架\n正文。\n"
	fr := reviewRun(t, content)
	if got := violationsOf(fr, "R1.3"); len(got) == 0 {
		t.Error("无编号章节标题应报 R1.3（require_numbering）")
	}
}

func TestRule_R1_3_HeadingLevelAlignment(t *testing.T) {
	// 层级与编号深度不对齐：## 配顶层编号 1
	fr := reviewRun(t, "# 大标题\n## 1 引言\n正文。\n")
	if got := violationsOf(fr, "R1.3"); len(got) == 0 {
		t.Error("## 配编号 1（层级不对齐）应报 R1.3")
	}
	// # 配二级编号 1.1
	fr = reviewRun(t, "# 大标题\n# 1.1 背景\n正文。\n")
	if got := violationsOf(fr, "R1.3"); len(got) == 0 {
		t.Error("# 配编号 1.1（层级不对齐）应报 R1.3")
	}
	// 对齐序列通过：大标题 → # 1 → ## 1.1 → ### 1.1.1 → # 2
	ok := "# 大标题\n# 1 引言\n正文。\n## 1.1 背景\n正文。\n### 1.1.1 定义\n正文。\n# 2 方法\n正文。\n"
	if got := violationsOf(reviewRun(t, ok), "R1.3"); len(got) != 0 {
		t.Errorf("对齐编号序列不应报 R1.3，got %+v", got)
	}
}

func TestRule_R1_3_English(t *testing.T) {
	// 英文综述：编号检查与语言无关，跳号同样拦截
	content := "# Title\n# 1 Introduction\nText.\n# 3 Methods\nText.\n"
	fr := runContent(t, content, SpecForType(PaperTypeReview, LangEN),
		Options{Lang: "en", Mode: ModeChapter, PaperType: PaperTypeReview})
	if got := violationsOf(fr, "R1.3"); len(got) == 0 {
		t.Error("英文标题跳号（1→3）应报 R1.3")
	}
	// 英文合规序列不报
	ok := "# Title\n# 1 Introduction\nText.\n# 2 Methods\nText.\n"
	if got := violationsOf(runContent(t, ok, SpecForType(PaperTypeReview, LangEN),
		Options{Lang: "en", Mode: ModeChapter, PaperType: PaperTypeReview}), "R1.3"); len(got) != 0 {
		t.Errorf("英文合规序列不应报 R1.3，got %+v", got)
	}
}

func TestRule_R1_3_AppliesToAllTypes(t *testing.T) {
	// 实证（empirical）与综述共用同一套编号检查
	content := "# 标题\n# 1 引言\n正文。\n# 3 方法\n正文。\n"
	fr := runContent(t, content, SpecForType(PaperTypeEmpirical, LangZH),
		Options{Lang: "zh", Mode: ModeChapter, PaperType: PaperTypeEmpirical})
	if got := violationsOf(fr, "R1.3"); len(got) == 0 {
		t.Error("实证类型也应跑 R1.3（章节跳号 1→3）")
	}
	// 实证合规序列不报
	okContent := "# 标题\n# 1 引言\n正文。\n# 2 方法\n正文。\n"
	fr = runContent(t, okContent, SpecForType(PaperTypeEmpirical, LangZH),
		Options{Lang: "zh", Mode: ModeChapter, PaperType: PaperTypeEmpirical})
	if got := violationsOf(fr, "R1.3"); len(got) != 0 {
		t.Errorf("实证合规序列不应报 R1.3，got %+v", got)
	}
}

// ---- R1.5 章节完整性（spec.Sections 比对）----

// reviewFullContent 覆盖 review-zh 全部章节的合规文稿骨架。
const reviewFullContent = "# 疼痛恐惧研究进展\n" +
	"# 1 引言\n正文。\n" +
	"# 2 文献检索方法\n正文。\n" +
	"# 3 主题分析\n正文。\n" +
	"## 3.1 评估工具\n正文。\n" +
	"# 4 讨论与展望\n正文。\n" +
	"# 5 结论\n正文。\n"

func TestRule_R1_5_MissingSections(t *testing.T) {
	// 只有引言和结论：缺 文献检索方法/主题分析/讨论与展望
	fr := reviewRun(t, "# 标题\n# 1 引言\n正文。\n# 2 结论\n正文。\n")
	got := violationsOf(fr, "R1.5")
	if len(got) != 3 {
		t.Fatalf("应报 3 个缺失章节，got %d: %+v", len(got), got)
	}
	joined := got[0].Problem + got[1].Problem + got[2].Problem
	if !strings.Contains(joined, "文献检索方法") || !strings.Contains(joined, "主题分析") {
		t.Errorf("应指明缺失章节名，got %+v", got)
	}
}

func TestRule_R1_5_AllPresent_Passes(t *testing.T) {
	if got := violationsOf(reviewRun(t, reviewFullContent), "R1.5"); len(got) != 0 {
		t.Errorf("全部章节齐全不应报 R1.5，got %+v", got)
	}
}

// ---- R1.6 空章节 ----

func TestRule_R1_6_EmptySection(t *testing.T) {
	// 标题下无正文（下一非空行直接是另一个标题）
	fr := reviewRun(t, "# 标题\n# 1 引言\n# 2 结论\n正文。\n")
	if got := violationsOf(fr, "R1.6"); len(got) != 1 {
		t.Fatalf("空章节应报 1 次 R1.6，got %+v", got)
	}
	// 文末标题无正文也算空章节
	fr = reviewRun(t, "# 标题\n# 1 引言\n正文。\n# 2 结论\n")
	if got := violationsOf(fr, "R1.6"); len(got) != 1 {
		t.Fatalf("文末空章节应报 R1.6，got %+v", got)
	}
	// 有正文的章节不报；大标题豁免
	if got := violationsOf(reviewRun(t, "# 标题\n# 1 引言\n正文。\n"), "R1.6"); len(got) != 0 {
		t.Errorf("正常章节不应报 R1.6，got %+v", got)
	}
}

// ---- R1.7 空/重复标题 ----

func TestRule_R1_7_EmptyHeading(t *testing.T) {
	fr := reviewRun(t, "# 标题\n#\n# 1 引言\n正文。\n")
	if got := violationsOf(fr, "R1.7"); len(got) == 0 {
		t.Error("空标题（# 后无文字）应报 R1.7")
	}
}

func TestRule_R1_7_DuplicateHeading(t *testing.T) {
	// 重复编号：两个 1.1
	fr := reviewRun(t, "# 标题\n# 1 引言\n## 1.1 背景\n正文。\n## 1.1 重复\n正文。\n")
	if got := violationsOf(fr, "R1.7"); len(got) == 0 {
		t.Error("重复编号标题应报 R1.7")
	}
	// 重复文字：两个"结论"
	fr = reviewRun(t, "# 标题\n# 1 结论\n正文。\n# 2 结论\n正文。\n")
	if got := violationsOf(fr, "R1.7"); len(got) == 0 {
		t.Error("重复文字标题应报 R1.7")
	}
	// 不重复则不报
	if got := violationsOf(reviewRun(t, "# 标题\n# 1 引言\n正文。\n# 2 结论\n正文。\n"), "R1.7"); len(got) != 0 {
		t.Errorf("不重复标题不应报 R1.7，got %+v", got)
	}
}

// ---- R1.8 图表交叉引用 ----

func TestRule_R1_8_FigureTableXref(t *testing.T) {
	// 引用表2但全文无表2定义 → 违规
	fr := reviewRun(t, "# 标题\n# 1 引言\n如表2所示。\n")
	if got := violationsOf(fr, "R1.8"); len(got) == 0 {
		t.Error("引用不存在的表2应报 R1.8")
	}
	// 引用表1且表1定义存在（题注紧邻表格）→ 通过
	ok := "# 标题\n# 1 引言\n如表1所示。\n表1 患者基线资料\n| 项目 | 值 |\n|---|---|\n| A | 1 |\n"
	if got := violationsOf(reviewRun(t, ok), "R1.8"); len(got) != 0 {
		t.Errorf("表1有定义不应报 R1.8，got %+v", got)
	}
	// 引用图1且图1定义存在（题注紧邻图片）→ 通过
	okFig := "# 标题\n# 1 引言\n见图1。\n图1 流程图\n![流程图](fig1.png)\n"
	if got := violationsOf(reviewRun(t, okFig), "R1.8"); len(got) != 0 {
		t.Errorf("图1有定义不应报 R1.8，got %+v", got)
	}
	// 英文：Table 2 无定义 → 违规
	frEn := runContent(t, "# Title\n# 1 Introduction\nAs shown in Table 2.\n",
		SpecForType(PaperTypeReview, LangEN),
		Options{Lang: "en", Mode: ModeChapter, PaperType: PaperTypeReview})
	if got := violationsOf(frEn, "R1.8"); len(got) == 0 {
		t.Error("英文引用不存在的 Table 2 应报 R1.8")
	}
}

// ---- R1.3 book 中文编号体系（yueshu.md 二、标题层次）----

// bookRun 以书籍类型运行验证（R1.3 走中文编号分支）。
func bookRun(t *testing.T, content string) FileReport {
	t.Helper()
	return runContent(t, content, SpecForType(PaperTypeBook, LangZH),
		Options{Lang: "zh", Mode: ModeChapter, PaperType: PaperTypeBook})
}

// bookFullContent 覆盖 7 级标题体系的合规书稿骨架。
const bookFullContent = "# 腹部创伤外科学\n" +
	"# 第一章 腹部创伤外科应用解剖学\n正文内容。\n" +
	"## 第一节 腹壁和腹膜腔的解剖\n正文内容。\n" +
	"### 一、腹壁的解剖特点\n正文内容。\n" +
	"#### （一）腹前外侧腹壁层次\n正文内容。\n" +
	"## 第二节 腹膜腔的解剖\n正文内容。\n" +
	"# 第二章 腹部创伤的临床特点\n正文内容。\n"

func TestRule_R1_3_Book_ValidChineseNumbering(t *testing.T) {
	if got := violationsOf(bookRun(t, bookFullContent), "R1.3"); len(got) != 0 {
		t.Errorf("合规中文编号不应报 R1.3，got %+v", got)
	}
}

func TestRule_R1_3_Book_SkipChapterNumber(t *testing.T) {
	fr := bookRun(t, "# 书名\n# 第一章 概述\n正文。\n# 第三章 跳号\n正文。\n")
	if got := violationsOf(fr, "R1.3"); len(got) == 0 {
		t.Error("章号跳号（第一章→第三章）应报 R1.3")
	}
}

func TestRule_R1_3_Book_BanDotNumbering(t *testing.T) {
	fr := bookRun(t, "# 书名\n# 第一章 概述\n正文。\n## 1.1 背景\n正文。\n")
	if got := violationsOf(fr, "R1.3"); len(got) == 0 {
		t.Error("点分编号（1.1）应报 R1.3（禁用 1.1.1 国际编号）")
	}
}

func TestRule_R1_3_Book_BanPureNumber(t *testing.T) {
	fr := bookRun(t, "# 书名\n# 1 概述\n正文。\n")
	if got := violationsOf(fr, "R1.3"); len(got) == 0 {
		t.Error("纯数字编号（1）应报 R1.3（应改用中文编号体系）")
	}
}

func TestRule_R1_3_Book_MissingNumbering(t *testing.T) {
	fr := bookRun(t, "# 书名\n# 第一章 概述\n正文。\n## 背景\n正文。\n")
	if got := violationsOf(fr, "R1.3"); len(got) == 0 {
		t.Error("无编号标题应报 R1.3")
	}
}

func TestRule_R1_3_Book_HierarchyJump(t *testing.T) {
	fr := bookRun(t, "# 书名\n# 第一章 概述\n正文。\n### 一、跳级\n正文。\n")
	if got := violationsOf(fr, "R1.3"); len(got) == 0 {
		t.Error("层级跳变（#→###）应报 R1.3")
	}
}

func TestRule_R1_3_Book_AutoTopLevel_SectionFile(t *testing.T) {
	// auto（默认）：一节一个文件，首个标题为"第一节"（合法编号），不应误报缺书名
	fr := bookRun(t, "## 第一节 腹壁和腹膜腔的解剖\n正文。\n### 一、腹壁的解剖特点\n正文。\n#### （一）腹前外侧腹壁层次\n正文。\n")
	if got := violationsOf(fr, "R1.3"); len(got) != 0 {
		t.Errorf("节文件（首个标题为第一节）不应报 R1.3，got %+v", got)
	}
}

func TestRule_R1_3_Book_AutoTopLevel_ChapterFile(t *testing.T) {
	// auto（默认）：一章一个文件，首个标题为"第一章"
	fr := bookRun(t, "# 第一章 腹部创伤外科应用解剖学\n正文。\n## 第一节 腹壁和腹膜腔的解剖\n正文。\n")
	if got := violationsOf(fr, "R1.3"); len(got) != 0 {
		t.Errorf("章文件（首个标题为第一章）不应报 R1.3，got %+v", got)
	}
}

func TestRule_R1_3_Book_AutoTopLevel_NoTitle(t *testing.T) {
	// auto：完全无标题仍报缺失
	fr := bookRun(t, "只有正文没有标题。\n")
	if got := violationsOf(fr, "R1.3"); len(got) == 0 {
		t.Error("auto：全文无标题应报 R1.3")
	}
}

func TestRule_R1_3_Book_RequireBookTitle(t *testing.T) {
	spec := SpecForType(PaperTypeBook, LangZH)
	spec.BookTopLevel = "book"
	opts := Options{Lang: "zh", Mode: ModeChapter, PaperType: PaperTypeBook}
	// 首标题带编号 → 缺书名
	fr := runContent(t, "# 第一章 概述\n正文。\n", spec, opts)
	if got := violationsOf(fr, "R1.3"); len(got) == 0 {
		t.Error("book 模式：首个标题带编号应报 R1.3（缺书名）")
	}
	// 书名 + 章 → 通过
	ok := runContent(t, "# 书名\n# 第一章 概述\n正文。\n", spec, opts)
	if got := violationsOf(ok, "R1.3"); len(got) != 0 {
		t.Errorf("book 模式：书名+章不应报 R1.3，got %+v", got)
	}
}

func TestRule_R1_3_Book_RequireChapter(t *testing.T) {
	spec := SpecForType(PaperTypeBook, LangZH)
	spec.BookTopLevel = "chapter"
	opts := Options{Lang: "zh", Mode: ModeChapter, PaperType: PaperTypeBook}
	// 首标题为节级 → 违规（须从篇/章级开始）
	fr := runContent(t, "## 第一节 概述\n正文。\n", spec, opts)
	if got := violationsOf(fr, "R1.3"); len(got) == 0 {
		t.Error("chapter 模式：首个标题为节级应报 R1.3")
	}
	// 首标题为章级 → 通过
	ok := runContent(t, "# 第一章 概述\n正文。\n", spec, opts)
	if got := violationsOf(ok, "R1.3"); len(got) != 0 {
		t.Errorf("chapter 模式：首个标题为章级不应报 R1.3，got %+v", got)
	}
}

// ---- R3.3 数字范围规范（yueshu.md 八、数字）----

func TestRule_R3_3(t *testing.T) {
	cases := []struct {
		content string
		want    int
	}{
		{"约占10-16%左右。\n", 1},     // → 占10%～16%
		{"体重10kg～15kg。\n", 1},    // → 10～15kg
		{"温度3℃～5℃。\n", 1},        // → 3～5℃
		{"1988年～1998年。\n", 1},    // → 1988—1998年
		{"创伤后24—48小时。\n", 1},     // → 24～48小时
		{"剂量为5 mg/kg/d。\n", 1},   // → 5 mg/(kg·d)
		{"占10%～16%。\n", 0},       // 合规
		{"体重10～15kg。\n", 0},      // 合规
		{"温度3～5℃。\n", 0},         // 合规
		{"1988—1998年。\n", 0},     // 合规
		{"创伤后24～48小时。\n", 0},     // 合规
		{"剂量为5 mg/(kg·d)。\n", 0}, // 合规
	}
	for _, c := range cases {
		fr := runContent(t, c.content, DefaultSpec(), zhDraft())
		if got := len(violationsOf(fr, "R3.3")); got != c.want {
			t.Errorf("%q 期望 %d 条 R3.3 违规，got %d：%v", c.content, c.want, got, violationsOf(fr, "R3.3"))
		}
	}
}

// ---- R3.4 计量单位（yueshu.md 五、计量单位）----

func TestRule_R3_4(t *testing.T) {
	cases := []struct {
		content string
		want    int
	}{
		{"每次5毫克。\n", 1},          // 毫克→mg
		{"长约3厘米。\n", 1},          // 厘米→cm
		{"重约2磅。\n", 1},           // 英制：磅
		{"长约2英寸。\n", 1},          // 英制：英寸
		{"每次5 mg。\n", 0},         // 合规
		{"长约3 cm。\n", 0},         // 合规
		{"5～8天。\n", 0},           // 时间单位中文合规（yueshu.md 例外）
		{"剂量为5 mg/(kg·d)。\n", 0}, // 合规
	}
	for _, c := range cases {
		fr := runContent(t, c.content, DefaultSpec(), zhDraft())
		if got := len(violationsOf(fr, "R3.4")); got != c.want {
			t.Errorf("%q 期望 %d 条 R3.4 违规，got %d：%v", c.content, c.want, got, violationsOf(fr, "R3.4"))
		}
	}
}
