package lint

import (
	"embed"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"sort"

	"gopkg.in/yaml.v3"
)

//go:embed templates/verifier_models.json
//go:embed templates/root-AGENTS.md
//go:embed templates/skills/litkit/SKILL.md
//go:embed templates/skills/litkit/references/literature-search.md
//go:embed templates/skills/litkit/references/manuscript-writing.md
//go:embed templates/empirical-zh/manuscript-spec.yaml
//go:embed templates/review-zh/manuscript-spec.yaml
//go:embed templates/empirical-en/manuscript-spec.yaml
//go:embed templates/review-en/manuscript-spec.yaml
//go:embed templates/book-zh/manuscript-spec.yaml
var templatesFS embed.FS

// LitkitDir 宿主工作目录的约束目录名。
// 独立于 litkit 自身开发约束 .harness/；统一 litkit 在宿主目录的命名空间。
const LitkitDir = ".litkit"

// SkillsSubDir .litkit 下的 Skills 子目录。
// init 时生成到此目录，用户按自己 AI 工具手动复制到对应 skills 路径。
const SkillsSubDir = "skills"

// 文件权限常量（mnd：避免魔法值）。
const (
	harnessDirPerm  = 0o750
	harnessFilePerm = 0o600
)

// sharedFiles 项目级共享文件（复制到 .litkit/ 根目录）。
var sharedFiles = []struct{ rel, tmpl string }{
	{"verifier_models.json", "templates/verifier_models.json"},
}

// skillFiles Agent Skills 文件（复制到 .litkit/skills/ 目录）。
var skillFiles = []struct{ rel, tmpl string }{
	{"litkit/SKILL.md", "templates/skills/litkit/SKILL.md"},
	{"litkit/references/literature-search.md", "templates/skills/litkit/references/literature-search.md"},
	{"litkit/references/manuscript-writing.md", "templates/skills/litkit/references/manuscript-writing.md"},
}

// SpecPath 返回 .litkit/<type-lang>/manuscript-spec.yaml 的绝对路径。
func SpecPath(dir, paperType, lang string) string {
	return filepath.Join(dir, LitkitDir, TypeLangDir(paperType, lang), "manuscript-spec.yaml")
}

// TypeSpecPath 返回 .litkit/<type-lang>/manuscript-spec.yaml 的绝对路径。
// 与 SpecPath 等价，用 typeDir 而非 paperType+lang。
func TypeSpecPath(dir, typeDir string) string {
	return filepath.Join(dir, LitkitDir, typeDir, "manuscript-spec.yaml")
}

// PapersDirPath 返回 .litkit/<type-lang>/ 的绝对路径。
func PapersDirPath(dir, paperType, lang string) string {
	return filepath.Join(dir, LitkitDir, TypeLangDir(paperType, lang))
}

// WriteSpec 序列化 spec 写回 yaml。
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

// InitProjectInfra 生成项目基础设施（.litkit/ 下的共享文件，如 verifier_models.json）。
// 已存在且未 force 时跳过。返回创建的相对路径。
func InitProjectInfra(dir string, force bool) ([]string, error) {
	created := []string{}
	for _, f := range sharedFiles {
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

// InitPaperType 生成论文类型目录（.litkit/<type-lang>/），复制对应模板 yaml。
// 已存在且未 force 时跳过。返回创建的相对路径。
func InitPaperType(dir, paperType, lang string, force bool) ([]string, error) {
	created := []string{}
	typeDir := TypeLangDir(paperType, lang)
	tmpl := path.Join("templates", typeDir, "manuscript-spec.yaml")

	data, err := templatesFS.ReadFile(tmpl)
	if err != nil {
		return created, fmt.Errorf("lint: embed %s: %w", tmpl, err)
	}
	path := filepath.Join(dir, LitkitDir, typeDir, "manuscript-spec.yaml")
	if err := os.MkdirAll(filepath.Dir(path), harnessDirPerm); err != nil {
		return created, fmt.Errorf("lint: mkdir %s: %w", filepath.Dir(path), err)
	}
	if !fileExists(path) || force {
		if err := os.WriteFile(path, data, harnessFilePerm); err != nil {
			return created, fmt.Errorf("lint: write %s: %w", path, err)
		}
		created = append(created, filepath.Join(LitkitDir, typeDir, "manuscript-spec.yaml"))
	}
	return created, nil
}

// ListPaperTypes 扫描 .litkit/ 下的论文类型目录（如 ["empirical-zh", "review-zh"]）。
func ListPaperTypes(dir string) ([]string, error) {
	litkitDir := filepath.Join(dir, LitkitDir)
	entries, err := os.ReadDir(litkitDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var types []string
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		// 仅识别包含 manuscript-spec.yaml 的目录
		specPath := filepath.Join(litkitDir, e.Name(), "manuscript-spec.yaml")
		if fileExists(specPath) {
			types = append(types, e.Name())
		}
	}
	sort.Strings(types)
	return types, nil
}

// LoadDefaultSpec 从 embed 模板加载指定类型的默认规范。
// 模板文件不存在时返回 error（编译时即应确保存在）。
func LoadDefaultSpec(paperType, lang string) (*ManuscriptSpec, error) {
	typeDir := TypeLangDir(paperType, lang)
	tmpl := path.Join("templates", typeDir, "manuscript-spec.yaml")

	data, err := templatesFS.ReadFile(tmpl)
	if err != nil {
		return nil, fmt.Errorf("lint: 默认模板 %s 不存在: %w", tmpl, err)
	}
	var spec ManuscriptSpec
	if err := yaml.Unmarshal(data, &spec); err != nil {
		return nil, fmt.Errorf("lint: 解析默认模板 %s: %w", tmpl, err)
	}
	return &spec, nil
}

// RootAgentsContent 返回 embed 模板中的根 AGENTS.md 内容。
func RootAgentsContent() (string, error) {
	data, err := templatesFS.ReadFile("templates/root-AGENTS.md")
	if err != nil {
		return "", fmt.Errorf("lint: embed root-AGENTS.md: %w", err)
	}
	return string(data), nil
}

// InitSkills 生成 Agent Skills 文件（.litkit/skills/ 目录）。
// 已存在且未 force 时跳过。返回创建的相对路径。
func InitSkills(dir string, force bool) ([]string, error) {
	created := []string{}
	for _, f := range skillFiles {
		data, err := templatesFS.ReadFile(f.tmpl)
		if err != nil {
			return created, fmt.Errorf("lint: embed %s: %w", f.tmpl, err)
		}
		path := filepath.Join(dir, LitkitDir, SkillsSubDir, f.rel)
		if err := os.MkdirAll(filepath.Dir(path), harnessDirPerm); err != nil {
			return created, fmt.Errorf("lint: mkdir %s: %w", filepath.Dir(path), err)
		}
		if fileExists(path) && !force {
			continue
		}
		if err := os.WriteFile(path, data, harnessFilePerm); err != nil {
			return created, fmt.Errorf("lint: write %s: %w", path, err)
		}
		created = append(created, filepath.Join(LitkitDir, SkillsSubDir, filepath.ToSlash(f.rel)))
	}
	return created, nil
}
