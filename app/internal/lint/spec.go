// Package lint 实现撰写约束（harness）基础设施：.litkit 目录生成、
// manuscript-spec.yaml 解析、AGENTS.md 撰写硬性规定渲染（事前指导）。
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
)

// 撰写语言常量（C1 中文优先，zh 为默认）。
const (
	LangZH = "zh"
	LangEN = "en"
)

// 标题默认阈值（mnd：避免魔法值）。
const (
	defaultMaxHeadingLevel  = 4  // 最大层级
	defaultMaxHeadingLength = 15 // 标题最大字数
)

// ManuscriptSpec 撰写规范配置（.litkit/specs/manuscript-spec.yaml）。
// 用户可手动修改；修改后 `litkit init --refresh` 重新生成 AGENTS.md 撰写段。
type ManuscriptSpec struct {
	PaperType string        `yaml:"paper_type"` // review | empirical
	Lang      string        `yaml:"lang"`       // zh | en
	Sections  Sections      `yaml:"sections"`
	WordCount WordCounts    `yaml:"word_counts"`
	Citation  CitationSpec  `yaml:"citation"`
	Heading   HeadingLimits `yaml:"heading"`
}

// Sections 章节结构（按论文类型）。
type Sections struct {
	Review    []string `yaml:"review"`
	Empirical []string `yaml:"empirical"`
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

// HeadingLimits 标题层级与长度上限。
type HeadingLimits struct {
	MaxLevel  int `yaml:"max_level"`
	MaxLength int `yaml:"max_length"`
}

// DefaultSpec 返回默认撰写规范（与 templates 中 manuscript-spec.yaml 一致）。
func DefaultSpec() *ManuscriptSpec {
	return &ManuscriptSpec{
		PaperType: PaperTypeEmpirical,
		Lang:      LangZH,
		Sections: Sections{
			Review:    []string{"引言", "文献检索方法", "主题分析", "讨论与展望", "结论"},
			Empirical: []string{"引言", "资料与方法", "结果", "讨论", "结论"},
		},
		WordCount: WordCounts{
			Total:     []int{3000, 8000},
			Abstract:  []int{200, 500},
			Paragraph: []int{30, 500},
		},
		Citation: CitationSpec{
			Count: []int{30, 45},
			Style: "gbt7714",
		},
		Heading: HeadingLimits{
			MaxLevel:  defaultMaxHeadingLevel,
			MaxLength: defaultMaxHeadingLength,
		},
	}
}

// LoadSpec 从 yaml 文件加载撰写规范；文件缺失/非法时回退默认值并返回 error 提示。
func LoadSpec(path string) (*ManuscriptSpec, error) {
	spec := DefaultSpec()
	data, err := os.ReadFile(path)
	if err != nil {
		return spec, fmt.Errorf("lint: read spec %s: %w", path, err)
	}
	// 先解到临时对象，缺失字段保留默认值
	loaded := *spec
	if err := yaml.Unmarshal(data, &loaded); err != nil {
		return spec, fmt.Errorf("lint: parse spec %s: %w", path, err)
	}
	if err := loaded.Validate(); err != nil {
		return spec, fmt.Errorf("lint: invalid spec %s: %w", path, err)
	}
	return &loaded, nil
}

// Validate 校验规范字段合法性（信任边界输入校验）。
func (s *ManuscriptSpec) Validate() error {
	switch s.PaperType {
	case PaperTypeReview, PaperTypeEmpirical:
	default:
		return fmt.Errorf("paper_type 必须为 review|empirical，got %q", s.PaperType)
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
	return nil
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
	if s.PaperType == PaperTypeReview {
		return s.Sections.Review
	}
	return s.Sections.Empirical
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
