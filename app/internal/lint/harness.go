package lint

import (
	"embed"
	"fmt"
	"os"
	"path/filepath"

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
