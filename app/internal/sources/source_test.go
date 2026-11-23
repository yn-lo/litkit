package sources

import (
	"reflect"
	"testing"
)

func TestSplitAuthorName_lastSpace(t *testing.T) {
	// "Given [Middle] Family"：必须按最后一个空白切分，否则中间名会被当成姓氏一部分
	cases := []struct {
		in   string
		want struct{ given, family string }
	}{
		{"John A. Smith", struct{ given, family string }{"John A.", "Smith"}},
		{"Alice Smith", struct{ given, family string }{"Alice", "Smith"}},
		{"Smith, John A.", struct{ given, family string }{"John A.", "Smith"}},
		{"张三", struct{ given, family string }{"", "张三"}},
		{"", struct{ given, family string }{"", ""}},
	}
	for _, c := range cases {
		got := splitAuthorName(c.in)
		if got.Given != c.want.given || got.Family != c.want.family {
			t.Errorf("splitAuthorName(%q) = %+v，want given=%q family=%q",
				c.in, got, c.want.given, c.want.family)
		}
	}
}

func TestSearchTerms(t *testing.T) {
	cases := []struct {
		in   string
		want []string
	}{
		{"fear of pain", []string{"fear", "pain"}},          // 虚词剔除，否则多词 AND 召回骤降
		{"  sepsis  ", []string{"sepsis"}},                  // 去空白
		{`"gut microbiota"`, []string{"gut", "microbiota"}}, // 引号剥离
		{"of the", []string{"of the"}},                      // 全虚词回退原文，避免清空检索词
	}
	for _, c := range cases {
		if got := searchTerms(c.in); !reflect.DeepEqual(got, c.want) {
			t.Errorf("searchTerms(%q) = %v，want %v", c.in, got, c.want)
		}
	}
}

func TestHasQuerySyntax(t *testing.T) {
	cases := []struct {
		in   string
		want bool
	}{
		{"fear of pain", false},
		{`"exact phrase"`, true},
		{"a AND b", true},
		{"a or b", true},
		{"cancer NOT diabetes", true}, // 独立 NOT 操作符不得被拆词
		{"cancer[Title]", true},
	}
	for _, c := range cases {
		if got := hasQuerySyntax(c.in); got != c.want {
			t.Errorf("hasQuerySyntax(%q) = %v，want %v", c.in, got, c.want)
		}
	}
}
