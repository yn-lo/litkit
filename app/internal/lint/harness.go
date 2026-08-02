package lint

import (
	"embed"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

//go:embed templates/*
var templatesFS embed.FS

// LitkitDir 宿主工作目录的约束目录名。
// 独立于 litkit 自身开发约束 .harness/；统一 litkit 在宿主目录的命名空间。
const LitkitDir = ".litkit"

// 文件权限常量（mnd：避免魔法值；0600：verifier_models.json 可能含 API key）。
const (
	harnessDirPerm  = 0o750
	harnessFilePerm = 0o600
)

// harnessFiles .litkit 目录生成清单（相对路径 → embed 模板）。
var harnessFiles = []struct{ rel, tmpl string }{
	{"rules.md", "templates/rules.md"},
	{"checklist.md", "templates/checklist.md"},
	{"specs/manuscript-spec.yaml", "templates/manuscript-spec.yaml"},
	{"verifier_models.json", "templates/verifier_models.json"},
}

// SpecPath 返回 .litkit/specs/manuscript-spec.yaml 的绝对路径。
func SpecPath(dir string) string {
	return filepath.Join(dir, LitkitDir, "specs", "manuscript-spec.yaml")
}

// WriteSpec 序列化 spec 写回 yaml（flag 指定类型/语言与模板默认不一致时调用）。
func WriteSpec(path string, spec *ManuscriptSpec) error {
	data, err := yaml.Marshal(spec)
	if err != nil {
		return fmt.Errorf("lint: marshal spec: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), harnessDirPerm); err != nil {
		return fmt.Errorf("lint: mkdir %s: %w", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, data, harnessFilePerm); err != nil {
		return fmt.Errorf("lint: write spec %s: %w", path, err)
	}
	return nil
}

func fileExists(p string) bool {
	info, err := os.Stat(p)
	return err == nil && !info.IsDir()
}

// InitHarness 生成 .litkit/ 目录（rules/checklist/specs/verifier 模板，go:embed）。
// 返回创建的相对路径；已存在且未 force 时跳过。
func InitHarness(dir string, force bool) ([]string, error) {
	created := []string{}
	for _, f := range harnessFiles {
		data, err := templatesFS.ReadFile(f.tmpl)
		if err != nil {
			return created, fmt.Errorf("lint: embed %s: %w", f.tmpl, err)
		}
		path := filepath.Join(dir, LitkitDir, f.rel)
		if err := os.MkdirAll(filepath.Dir(path), harnessDirPerm); err != nil {
			return created, fmt.Errorf("lint: mkdir %s: %w", filepath.Dir(path), err)
		}
		if fileExists(path) && !force {
			continue
		}
		if err := os.WriteFile(path, data, harnessFilePerm); err != nil {
			return created, fmt.Errorf("lint: write %s: %w", path, err)
		}
		created = append(created, filepath.Join(LitkitDir, filepath.ToSlash(f.rel)))
	}
	return created, nil
}

// 固定高频硬性规定（zh/en 各一套，写死模板；阈值类从 spec 取可变值）。
var (
	zhFixedRules = []string{
		"全文中文撰写；英文仅限专业术语首次括注",
		"正文禁止：列表、加粗、回引（如\"见本文2.3节\"）、AI 痕迹词（\"首次证实\"\"颠覆性\"）",
		"P 值：≥0.01 保留 2 位（P=0.03）；0.001≤P<0.01 保留 3 位（P=0.006）；P<0.001 写 \"P<0.001\"",
		"统计量保留 2 位小数（χ²=68.40）；表格 % 提取到表头统一标注",
	}
	enFixedRules = []string{
		"Write in academic English; define abbreviations at first use",
		"No lists or bold in body text; no back-references or self-promotion",
		"Report statistics with consistent decimals; percentages rounded to 1 decimal",
	}
)

// RenderWritingRules 渲染 AGENTS.md 的「撰写硬性规定」段（事前指导）。
//
// 固定高频规则 + spec 可变阈值（字数/引用数/章节/标题）→ 精简祈使句。
// 不是 yaml 的翻译，而是 AI 写稿时可执行的硬性规定。
func RenderWritingRules(spec *ManuscriptSpec) string {
	rules := zhFixedRules
	if spec.Lang == LangEN {
		rules = enFixedRules
	}

	var b strings.Builder
	b.WriteString("## 撰写硬性规定（事前指导，源自 .litkit/specs/manuscript-spec.yaml）\n\n")
	for _, r := range rules {
		b.WriteString("- " + r + "\n")
	}
	fmt.Fprintf(&b, "- 章节结构（%s）：%s\n",
		spec.PaperType, strings.Join(spec.SectionList(), " → "))
	fmt.Fprintf(&b, "- 标题层级 ≤%d 级，标题 ≤%d 字，末尾无标点\n",
		spec.Heading.MaxLevel, spec.Heading.MaxLength)
	fmt.Fprintf(&b, "- 引用：全文 %d-%d 篇，用 [cite:citeKey] 占位符（%s），不展开元数据\n",
		spec.Citation.Count[0], spec.Citation.Count[1], spec.StyleLabel())
	fmt.Fprintf(&b, "- 字数：全文 %d-%d；摘要 %d-%d；段落 %d-%d\n",
		spec.WordCount.Total[0], spec.WordCount.Total[1],
		spec.WordCount.Abstract[0], spec.WordCount.Abstract[1],
		spec.WordCount.Paragraph[0], spec.WordCount.Paragraph[1])
	return b.String()
}
