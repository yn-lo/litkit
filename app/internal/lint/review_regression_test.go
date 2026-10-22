package lint

import (
	"strings"
	"testing"
)

func TestRound2_FenceParsing(t *testing.T) {
	for _, doc := range []string{"~~~text\n结论[@AAA]。\n~~~\n正文。\n", "````text\n```\n结论[@AAA]。\n````\n正文。\n", "```text\n    ```\n结论[@AAA]。\n```\n正文。\n"} {
		src := mustParse(t, doc)
		if strings.Contains(src.BodyContent(), "[@AAA]") || !strings.Contains(src.BodyContent(), "正文") {
			t.Errorf("code fence not respected: Body=%q", src.BodyContent())
		}
	}
}

func TestBuiltinSpecsMatchTypeAndLanguage(t *testing.T) {
	for _, tc := range []struct{ typ, lang string }{
		{PaperTypeEmpirical, LangZH}, {PaperTypeEmpirical, LangEN},
		{PaperTypeReview, LangZH}, {PaperTypeReview, LangEN},
		{PaperTypeBook, LangZH}, {PaperTypeProposal, LangZH},
	} {
		spec, err := LoadDefaultSpec(tc.typ, tc.lang)
		if err != nil {
			t.Fatal(err)
		}
		if spec.PaperType != tc.typ || spec.Lang != tc.lang {
			t.Fatalf("wrong built-in spec for %s/%s: %+v", tc.typ, tc.lang, spec)
		}
	}
}

func TestRound2_FixProtectedCodeVariants(t *testing.T) {
	for _, doc := range []string{"正文。\n\n    {\"中文\":\"内容\"}\n", "正文 `中文,\n原样` 结束。\n", "正文 `中文,\n    中文,\n原样` 结束。\n"} {
		got, _ := ApplyFixes(doc, DefaultSpec(), FixableRules())
		if got != doc {
			t.Errorf("Markdown code changed: before=%q, after=%q", doc, got)
		}
	}
}
