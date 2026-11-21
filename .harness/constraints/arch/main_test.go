package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// setupModule 创建临时 Go 模块（module litkit），返回根目录。
func setupModule(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	writeTestFile(t, filepath.Join(root, "go.mod"), "module litkit\n\ngo 1.26\n")
	return root
}

func writeTestFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func violationsFor(t *testing.T, root, rel string) []string {
	t.Helper()
	var v []string
	checkFile(root, rel, &v)
	return v
}

// 反向依赖（叶子层 import 服务层）必须被发现；否则检查器形同虚设。
func TestCheckFile_detectsReverseDependency(t *testing.T) {
	root := setupModule(t)
	rel := filepath.Join("internal", "model", "model.go")
	writeTestFile(t, filepath.Join(root, rel), "package model\n\nimport _ \"litkit/internal/core\"\n")
	v := violationsFor(t, root, rel)
	if len(v) == 0 {
		t.Fatal("叶子层依赖服务层应报违规，实际未报")
	}
	if !strings.Contains(v[0], "不得依赖") {
		t.Fatalf("违规信息应说明依赖方向，got %q", v[0])
	}
}

// 正向依赖（服务层 import 叶子层）必须通过。
func TestCheckFile_allowsForwardDependency(t *testing.T) {
	root := setupModule(t)
	rel := filepath.Join("internal", "core", "core.go")
	writeTestFile(t, filepath.Join(root, rel), "package core\n\nimport _ \"litkit/internal/model\"\n")
	if v := violationsFor(t, root, rel); len(v) != 0 {
		t.Fatalf("服务层依赖叶子层不应报违规，got %v", v)
	}
}

// 未登记层归属的 internal 包必须报违规，避免新包绕过约束。
func TestCheckFile_unknownInternalPackageFails(t *testing.T) {
	root := setupModule(t)
	rel := filepath.Join("internal", "newpkg", "new.go")
	writeTestFile(t, filepath.Join(root, rel), "package newpkg\n")
	if v := violationsFor(t, root, rel); len(v) == 0 {
		t.Fatal("未登记的 internal 包应报违规")
	}
}

// 第三方模块路径中的 internal 片段不得被误判为项目内部依赖。
func TestCheckFile_ignoresExternalInternalPaths(t *testing.T) {
	root := setupModule(t)
	rel := filepath.Join("internal", "model", "model.go")
	writeTestFile(t, filepath.Join(root, rel), "package model\n\nimport _ \"example.com/x/internal/y\"\n")
	if v := violationsFor(t, root, rel); len(v) != 0 {
		t.Fatalf("外部模块不应纳入分层约束，got %v", v)
	}
}

// 入口层（cmd/）允许依赖服务层。
func TestCheckFile_entryMayDependOnService(t *testing.T) {
	root := setupModule(t)
	rel := filepath.Join("cmd", "litkit", "main.go")
	writeTestFile(t, filepath.Join(root, rel), "package main\n\nimport _ \"litkit/internal/core\"\n")
	if v := violationsFor(t, root, rel); len(v) != 0 {
		t.Fatalf("入口层依赖服务层不应报违规，got %v", v)
	}
}

// 模块位于子目录（本仓库的真实结构 app/go.mod + app/internal/...）时仍必须检查。
func TestCheckFile_nestedModuleDetectsReverseDependency(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, filepath.Join(root, "app", "go.mod"), "module litkit\n\ngo 1.26\n")
	rel := filepath.Join("app", "internal", "model", "model.go")
	writeTestFile(t, filepath.Join(root, rel), "package model\n\nimport _ \"litkit/internal/core\"\n")
	if v := violationsFor(t, root, rel); len(v) == 0 {
		t.Fatal("子目录模块的反向依赖应报违规")
	}
}

// 分层目录下缺少 go.mod 时不能静默放行。
func TestCheckFile_missingModuleFileFails(t *testing.T) {
	root := t.TempDir()
	rel := filepath.Join("internal", "model", "model.go")
	writeTestFile(t, filepath.Join(root, rel), "package model\n")
	if v := violationsFor(t, root, rel); len(v) == 0 {
		t.Fatal("缺少 go.mod 时应报违规")
	}
}
