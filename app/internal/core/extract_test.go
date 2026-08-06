package core

import (
	"strings"
	"testing"
)

func TestExtractCiteSentences_Simple(t *testing.T) {
	body := `本文提出了一种新方法，在 Graph Neural Networks 的实验中取得了显著效果[@Kxq]。`
	got := ExtractCiteSentences(body)
	if len(got) != 1 {
		t.Fatalf("应提取 1 句，got %d", len(got))
	}
	if got[0].CiteKey != "Kxq" {
		t.Fatalf("CiteKey 应为 Kxq，got %q", got[0].CiteKey)
	}
	if len(got[0].SentenceHash) != 16 {
		t.Fatalf("SentenceHash 应为 16 字符，got %d", len(got[0].SentenceHash))
	}
	if !strings.Contains(got[0].Sentence, "Graph Neural Networks") {
		t.Fatalf("句子应含原文，got %q", got[0].Sentence)
	}
}

func TestExtractCiteSentences_MultipleCiteKeys(t *testing.T) {
	body := `方法 A[@Kxq] 在数据集 B[@Kzr] 上表现优异。`
	got := ExtractCiteSentences(body)
	if len(got) != 2 {
		t.Fatalf("应提取 2 句（一句一引），got %d", len(got))
	}
	if got[0].CiteKey != "Kxq" || got[1].CiteKey != "Kzr" {
		t.Fatalf("citeKey 顺序错误：%q %q", got[0].CiteKey, got[1].CiteKey)
	}
}

func TestExtractCiteSentences_SameKeySameHash_Dedup(t *testing.T) {
	body := `该方法[@Kxq]在实验中有效。另一处也提到[@Kxq]。`
	got := ExtractCiteSentences(body)
	if len(got) != 2 {
		t.Fatalf("不同句的同 citeKey 应各一条，got %d", len(got))
	}
	if got[0].SentenceHash == got[1].SentenceHash {
		t.Fatal("不同句的 hash 应不同")
	}
}

func TestExtractCiteSentences_SkipCodeBlock(t *testing.T) {
	body := `正文引用[@Kxq]。
` + "```" + `
代码块引用[@Kxq]。
` + "```" + `
`
	got := ExtractCiteSentences(body)
	if len(got) != 1 {
		t.Fatalf("代码块中的引用应被跳过，got %d", len(got))
	}
}

func TestExtractCiteSentences_SkipReferences(t *testing.T) {
	body := `正文引用[@Kxq]。
## 参考文献
[@Kzr] 的一些工作。`
	got := ExtractCiteSentences(body)
	if len(got) != 1 {
		t.Fatalf("参考文献区的引用应被跳过，got %d", len(got))
	}
	if got[0].CiteKey != "Kxq" {
		t.Fatalf("应仅提取正文中的 Kxq，got %q", got[0].CiteKey)
	}
}

func TestExtractCiteSentences_NoCitation(t *testing.T) {
	got := ExtractCiteSentences("这是一段没有引用的文字。")
	if len(got) != 0 {
		t.Fatalf("无引用时应返回空，got %d", len(got))
	}
}

func TestExtractCiteSentences_SentenceBoundaryChinese(t *testing.T) {
	body := `前文结束。该模型[@Kxq]在测试中表现优异。后文开始。`
	got := ExtractCiteSentences(body)
	if len(got) != 1 {
		t.Fatalf("应提取 1 句，got %d", len(got))
	}
	if !strings.Contains(got[0].Sentence, "该模型") {
		t.Fatalf("句子应从'该模型'开始，got %q", got[0].Sentence)
	}
	if !strings.Contains(got[0].Sentence, "表现优异") {
		t.Fatalf("句子应含'表现优异'，got %q", got[0].Sentence)
	}
}

func TestExtractCiteSentences_EnglishPeriod(t *testing.T) {
	body := `Previous work. This method[@Kxq] achieves state-of-the-art results. Next work.`
	got := ExtractCiteSentences(body)
	if len(got) != 1 {
		t.Fatalf("应提取 1 句，got %d", len(got))
	}
	if !strings.Contains(got[0].Sentence, "This method") {
		t.Fatalf("句子应从'This method'开始，got %q", got[0].Sentence)
	}
}

func TestExtractCiteSentences_MidSentenceCite(t *testing.T) {
	body := `Smith等人[@Kxq]提出了重要方法，而Jones[@Kzr]则改进了它。`
	got := ExtractCiteSentences(body)
	if len(got) != 2 {
		t.Fatalf("句中引用应提取 2 句，got %d", len(got))
	}
	// 同一句中的两个 citeKey 共享同一 sentence_hash
	if got[0].SentenceHash != got[1].SentenceHash {
		t.Fatal("同一句中的多个 citeKey 应共享 sentence_hash")
	}
}
