// Package lint 实现撰写约束（harness）基础设施：.litkit 目录生成、
// manuscript-spec.yaml 解析。
//
// 对应 PRD FR-LINT；架构约束：服务层，仅被入口层调用。
package lint

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// 论文类型常量（FR-LINT：preset 阈值切换，规则代码零差异）。
const (
	PaperTypeReview    = "review"    // 综述
	PaperTypeEmpirical = "empirical" // 四段式实证
	PaperTypeBook      = "book"      // 书籍（医学书稿编校细则，yueshu.md）
)

// 撰写语言常量（C1 中文优先，zh 为默认）。
const (
	LangZH = "zh"
	LangEN = "en"
)

// 标题默认阈值（mnd：避免魔法值）。

// ManuscriptSpec 撰写规范配置（.litkit/<type>/manuscript-spec.yaml）。
// 用户可手动修改；AI 直接读取此文件，无需额外步骤。
type ManuscriptSpec struct {
	PaperType  string        `yaml:"paper_type"` // review | empirical | book
	Lang       string        `yaml:"lang"`       // zh | en
	Journal    string        `yaml:"journal"`    // 目标期刊（影响引用格式默认值与 checklist）
	Sections   []string      `yaml:"sections"`   // 当前论文类型的章节清单
	WordCount  WordCounts    `yaml:"word_counts"`
	Citation   CitationSpec  `yaml:"citation"`
	Heading    HeadingLimits `yaml:"heading"`
	BoastWords []string      `yaml:"boast_words"` // 自我夸大/AI痕迹禁用词表（空=使用默认词表）
	// BookTopLevel 书籍文件顶层标题级别（仅 book 生效）：auto|book|chapter|section。
	// auto（默认）自动兼容整本书/单章/单节文件；book 强制首标题为书名。
	BookTopLevel string `yaml:"book_top_level"`
}

// WordCounts 字数阈值 [min, max]。
type WordCounts struct {
	Total     []int `yaml:"total"`
	Abstract  []int `yaml:"abstract"`
	Paragraph []int `yaml:"paragraph"`
}

// CitationSpec 引用阈值与样式。
type CitationSpec struct {
	Count []int  `yaml:"count"`
	Style string `yaml:"style"` // gbt7714 | apa | ieee
}

// HeadingLimits 标题层级、长度与编号要求。
type HeadingLimits struct {
	MaxLevel  int `yaml:"max_level"`
	MaxLength int `yaml:"max_length"`
	// RequireNumbering 标题是否必须带数字编号（1 / 1.1 / 1.1.1）。
	// 指针语义：yaml 未指定时默认 true（全类型强制编号），显式 false 可关闭。
	RequireNumbering *bool `yaml:"require_numbering"`
}

// NumberingRequired 返回是否强制标题编号（未配置时默认 true）。
func (h *HeadingLimits) NumberingRequired() bool {
	if h.RequireNumbering == nil {
		return true
	}
	return *h.RequireNumbering
}

// DefaultSpec 返回默认撰写规范（empirical/zh，从 embed 模板读取）。
func DefaultSpec() *ManuscriptSpec {
	return SpecForType(PaperTypeEmpirical, LangZH)
}

// SpecForType 按论文类型与语言返回预设撰写规范（从 embed 模板读取）。
// 模板文件由 //go:embed 编译时嵌入，运行时不会失败。
func SpecForType(paperType, lang string) *ManuscriptSpec {
	spec, err := LoadDefaultSpec(paperType, lang)
	if err != nil {
		panic(fmt.Sprintf("lint: embed 模板缺失（编译时错误）: %v", err))
	}
	return spec
}

// LoadSpec 从 yaml 文件加载撰写规范；文件不存在时返回默认模板值。
func LoadSpec(path string) (*ManuscriptSpec, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return DefaultSpec(), nil
		}
		return DefaultSpec(), fmt.Errorf("lint: read spec %s: %w", path, err)
	}
	var spec ManuscriptSpec
	if err := yaml.Unmarshal(data, &spec); err != nil {
		return DefaultSpec(), fmt.Errorf("lint: parse spec %s: %w", path, err)
	}
	if err := spec.Validate(); err != nil {
		return DefaultSpec(), fmt.Errorf("lint: invalid spec %s: %w", path, err)
	}
	return &spec, nil
}

// Validate 校验规范字段合法性（信任边界输入校验）。
func (s *ManuscriptSpec) Validate() error {
	switch s.PaperType {
	case PaperTypeReview, PaperTypeEmpirical, PaperTypeBook:
	default:
		return fmt.Errorf("paper_type 必须为 review|empirical|book，got %q", s.PaperType)
	}
	switch s.Lang {
	case LangZH, LangEN:
	default:
		return fmt.Errorf("lang 必须为 zh|en，got %q", s.Lang)
	}
	if err := validateRange("word_counts.total", s.WordCount.Total); err != nil {
		return err
	}
	if err := validateRange("word_counts.abstract", s.WordCount.Abstract); err != nil {
		return err
	}
	if err := validateRange("word_counts.paragraph", s.WordCount.Paragraph); err != nil {
		return err
	}
	if err := validateRange("citation.count", s.Citation.Count); err != nil {
		return err
	}
	if s.Heading.MaxLevel <= 0 {
		return fmt.Errorf("heading.max_level 必须 > 0")
	}
	if s.Heading.MaxLength <= 0 {
		return fmt.Errorf("heading.max_length 必须 > 0")
	}
	if !IsValidBookTopLevel(s.BookTopLevel) {
		return fmt.Errorf("book_top_level 必须为 auto|book|chapter|section（空=auto），got %q", s.BookTopLevel)
	}
	return nil
}

// IsValidBookTopLevel 判断书籍文件顶层标题级别是否合法（book_top_level）。
func IsValidBookTopLevel(l string) bool {
	switch l {
	case "", "auto", "book", "chapter", "section":
		return true
	}
	return false
}

func validateRange(name string, r []int) error {
	if len(r) != 2 {
		return fmt.Errorf("%s 必须为 [min, max]，got %v", name, r)
	}
	if r[0] <= 0 || r[1] <= 0 || r[0] >= r[1] {
		return fmt.Errorf("%s 须满足 0 < min < max，got %v", name, r)
	}
	return nil
}

// SectionList 返回当前论文类型的章节清单。
func (s *ManuscriptSpec) SectionList() []string {
	return s.Sections
}

// BoastWordList 返回夸大词表；spec 为空时返回 nil（verify 跳过）。
func (s *ManuscriptSpec) BoastWordList() []string {
	return s.BoastWords
}

// StyleLabel 返回引用样式的 AI 可读标签。
func (s *ManuscriptSpec) StyleLabel() string {
	switch s.Citation.Style {
	case "apa":
		return "APA"
	case "ieee":
		return "IEEE"
	default:
		return "GB/T 7714"
	}
}

// TypeLangDir 返回 .litkit 下的论文类型目录名（如 "empirical-zh"）。
func TypeLangDir(paperType, lang string) string {
	return paperType + "-" + lang
}

// IsValidPaperType 判断是否为已注册论文类型。
func IsValidPaperType(t string) bool {
	switch t {
	case PaperTypeReview, PaperTypeEmpirical, PaperTypeBook:
		return true
	}
	return false
}

// PaperTypesLabel 返回论文类型枚举的展示标签（用于帮助文本与参数错误提示）。
func PaperTypesLabel() string {
	return "review|empirical|book"
}
