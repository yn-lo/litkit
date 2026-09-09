package lint

import (
	"strings"
	"testing"
)

func mustSpec(lang string) *ManuscriptSpec { return &ManuscriptSpec{Lang: lang} }

// ---- R4.5 AI 高频术语 ----

func TestCheckR45_ChineseAndExempt(t *testing.T) {
	pos := mustParse(t, "综上所述，有必要深入探讨该机制。\n")
	vs := checkR45(pos, mustSpec(LangZH))
	if len(vs) != 2 {
		t.Fatalf("中文黑名单应报 2 处（综上所述/深入探讨），got %d: %+v", len(vs), vs)
	}
	// 配置豁免「综上所述」后只剩 1 处
	exempt := mustSpec(LangZH)
	exempt.StyleExemptTerms = []string{"综上所述"}
	if got := checkR45(pos, exempt); len(got) != 1 {
		t.Fatalf("豁免后应报 1 处，got %d: %+v", len(got), got)
	}
	// 英文文本跑 zh 词表：无中文套路词，应 0
	if got := checkR45(mustParse(t, "This is a delve into things.\n"), mustSpec(LangZH)); len(got) != 0 {
		t.Fatalf("英文字符不应命中中文词表，got %v", got)
	}
}

func TestCheckR45_EnglishFlag(t *testing.T) {
	src := mustParse(t, "This paper delve into multifaceted aspect and leverage the finding.\n")
	vs := checkR45(src, mustSpec(LangEN))
	if len(vs) != 3 {
		t.Fatalf("英文黑名单应报 delve/multifaceted/leverage 3 处，got %d: %+v", len(vs), vs)
	}
	// robust 属带学科义词，默认词表不含，不应误报
	if got := checkR45(mustParse(t, "The robust estimator is used.\n"), mustSpec(LangEN)); len(got) != 0 {
		t.Fatalf("robust 不应被默认黑名单误报，got %v", got)
	}
}

// ---- R4.7 句长突变 ----

func TestCheckR47_UniformSentencesFlag(t *testing.T) {
	uniform := "句长一致。句长一致。句长一致。句长一致。句长一致。句长一致。\n"
	if got := checkR47(mustParse(t, uniform), mustSpec(LangZH)); len(got) != 1 {
		t.Fatalf("连续 6 句等长应报 R4.7，got %d: %+v", len(got), got)
	}
	var varied strings.Builder
	for i := 0; i < 8; i++ {
		varied.WriteString("这是长度明显不一致的一长串句子demo。短。\n")
	}
	if got := checkR47(mustParse(t, varied.String()), mustSpec(LangZH)); len(got) != 0 {
		t.Fatalf("长短交替不应报 R4.7，got %v", got)
	}
}

// ---- R4.8 分号过密 ----

func TestCheckR48_SemicolonDensity(t *testing.T) {
	zh := "甲；乙；丙；丁；戊。\n"
	if got := checkR48(mustParse(t, zh), mustSpec(LangZH)); len(got) != 1 {
		t.Fatalf("中文分号过密应报 R4.8，got %d: %+v", len(got), got)
	}
	enOk := "first clause and second clause are combined. That is all there is.\n"
	if got := checkR48(mustParse(t, enOk), mustSpec(LangEN)); len(got) != 0 {
		t.Fatalf("无分号不应报 R4.8，got %v", got)
	}
	enBad := "a; b; c; d; e; f; g; h; i; j; k; l; m; n.\n"
	if got := checkR48(mustParse(t, enBad), mustSpec(LangEN)); len(got) != 1 {
		t.Fatalf("英文分号过密应报 R4.8，got %d: %+v", len(got), got)
	}
}

// ---- R4.9 摘要禁引用 ----

func TestCheckR49_AbstractNoCitation(t *testing.T) {
	bad := "# 摘要\n\n本文综述了[@foo]\n见相关研究[@foo]\n。\n\n# 引言\n\n正文。\n"
	if got := checkR49(mustParse(t, bad), mustSpec(LangZH)); len(got) != 2 {
		t.Fatalf("摘要内 2 行各含引用应报 R4.9（按行计 2 条），got %d: %+v", len(got), got)
	}
	clean := "# 摘要\n\n本文介绍机制。\n\n# 引言\n\n正文引用[@foo]。\n"
	if got := checkR49(mustParse(t, clean), mustSpec(LangZH)); len(got) != 0 {
		t.Fatalf("引言中的引用不应触发摘要禁引用，got %v", got)
	}
	noAbs := "# 引言\n\n正文引用[@foo]。\n"
	if got := checkR49(mustParse(t, noAbs), mustSpec(LangZH)); len(got) != 0 {
		t.Fatalf("无摘要段不应报 R4.9，got %v", got)
	}
	enAbs := "# Abstract\n\nWe review [@foo].\n"
	if got := checkR49(mustParse(t, enAbs), mustSpec(LangEN)); len(got) != 1 {
		t.Fatalf("英文 Abstract 内引用应报 R4.9，got %d: %+v", len(got), got)
	}
}

// ---- R4.10 重复句段 ----

func TestCheckR410_CrossParagraphRepeat(t *testing.T) {
	src := mustParse(t, "# 1 引言\n\n本研究采用术后疼痛恐惧面部评估工具进行测量。\n\n# 2 讨论\n\n如前所述，本研究采用术后疼痛恐惧面部评估工具进行测量。\n")
	vs := checkR410(src, mustSpec(LangZH))
	if len(vs) != 1 {
		t.Fatalf("跨段重复片段应报 1 条，got %d: %+v", len(vs), vs)
	}
	if vs[0].Line != 3 {
		t.Fatalf("应报首次出现行 3，got %d", vs[0].Line)
	}
	if !strings.Contains(vs[0].Problem, "重复出现 2 次") {
		t.Fatalf("应报出现次数 2，got %q", vs[0].Problem)
	}
}

func TestCheckR410_LongestOnly(t *testing.T) {
	// 整句重复：句内子片段的重复由最长段覆盖，只报 1 条
	sent := "本研究采用术后疼痛恐惧面部评估工具进行测量与分析。结果可靠。"
	src := mustParse(t, sent+"\n\n第二部分："+sent+"\n")
	vs := checkR410(src, mustSpec(LangZH))
	if len(vs) != 1 {
		t.Fatalf("整句重复应只报最长 1 条，got %d: %+v", len(vs), vs)
	}
	if !strings.Contains(vs[0].Problem, "重复出现 2 次") {
		t.Fatalf("应报出现次数 2，got %q", vs[0].Problem)
	}
}

func TestCheckR410_ThresholdConfigurable(t *testing.T) {
	src := mustParse(t, "术后疼痛恐惧面部显著影响生活质量。\n\n因此术后疼痛恐惧面部值得关注。\n")
	// 默认 10：8 字片段不报
	if got := checkR410(src, mustSpec(LangZH)); len(got) != 0 {
		t.Fatalf("8 字片段低于默认阈值 10 不应报，got %v", got)
	}
	spec := mustSpec(LangZH)
	spec.RepeatMinLen = 5
	if vs := checkR410(src, spec); len(vs) != 1 {
		t.Fatalf("repeat_min_len=5 时 8 字片段应报 1 条，got %d: %+v", len(vs), vs)
	}
}

func TestCheckR410_TripleOccurrence(t *testing.T) {
	src := mustParse(t, "术前评估包括影像学检查与实验室检验。术后评估包括影像学检查与实验室检验。随访评估包括影像学检查与实验室检验。\n")
	vs := checkR410(src, mustSpec(LangZH))
	if len(vs) != 1 {
		t.Fatalf("同一片段 3 次出现应合并报 1 条，got %d: %+v", len(vs), vs)
	}
	if !strings.Contains(vs[0].Problem, "重复出现 3 次") {
		t.Fatalf("应报出现次数 3，got %q", vs[0].Problem)
	}
}
