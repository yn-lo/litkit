// arch_check 检查 litkit 分层依赖方向（入口层 → 服务层 → 适配层 → 叶子层）。
//
// 运行方式（在 litkit 项目根）：
//
//	go run .harness/constraints/arch/main.go
//
// 退出码：0 = 通过（不输出任何检查信息）；1 = 违规（只输出错误）。
//
// 分层规则（对应 .harness/specs/architecture/boundaries.md）：
//
//	入口层 cmd/litkit                         允许依赖 服务层/适配层/叶子层
//	服务层 internal/core·lint                允许依赖 适配层/叶子层
//	适配层 internal/sources·csl                允许依赖 叶子层
//	叶子层 internal/model·config·storage·util·buildinfo 只允许依赖其他叶子层
//
// 额外规则：
//   - internal/model 不 import 任何非叶子层（数据模型纯净，C5/C6）
//   - cmd/ 下的 main 包视为入口层，允许依赖所有 internal 包
//   - 叶子层只允许依赖标准库与第三方模块，禁止依赖 internal/ 上层
package main

import (
	"fmt"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
)

// layer 层标识。
type layer int

const (
	layerUnknown layer = iota
	layerEntry         // 入口层
	layerService       // 服务层
	layerAdapter       // 适配层
	layerLeaf          // 叶子层
)

// servicePrefixes / adapterPrefixes / leafPrefixes
// 目录前缀 → 层映射（相对模块根）。新增内部包时在此登记，并同步更新 boundaries.md。
// 入口层由 "cmd/" 前缀识别，不在此列表。
var (
	servicePrefixes = []string{"internal/core", "internal/lint"}
	adapterPrefixes = []string{"internal/sources", "internal/csl"}
	leafPrefixes    = []string{"internal/model", "internal/config", "internal/storage", "internal/util", "internal/buildinfo"}
)

func dirLayer(importPath string) layer {
	for _, p := range leafPrefixes {
		if importPath == p || strings.HasPrefix(importPath, p+"/") {
			return layerLeaf
		}
	}
	for _, p := range adapterPrefixes {
		if importPath == p || strings.HasPrefix(importPath, p+"/") {
			return layerAdapter
		}
	}
	for _, p := range servicePrefixes {
		if importPath == p || strings.HasPrefix(importPath, p+"/") {
			return layerService
		}
	}
	return layerUnknown
}

func layerName(l layer) string {
	switch l {
	case layerEntry:
		return "入口层"
	case layerService:
		return "服务层"
	case layerAdapter:
		return "适配层"
	case layerLeaf:
		return "叶子层"
	default:
		return "未知层"
	}
}

// allowed 判断 src 层是否允许依赖 dst 层。依赖方向单向：入口 → 服务 → 适配 → 叶子。
func allowed(src, dst layer) bool {
	switch src {
	case layerEntry:
		return dst == layerService || dst == layerAdapter || dst == layerLeaf
	case layerService:
		return dst == layerAdapter || dst == layerLeaf
	case layerAdapter:
		return dst == layerLeaf
	case layerLeaf:
		return dst == layerLeaf
	default:
		return false
	}
}

func main() {
	// 项目根：以本源码文件位置定位（go run / 编译后均可靠）。
	// 本文件位于 <root>/.harness/constraints/arch/main.go，上溯三级即项目根。
	_, srcFile, _, ok := runtime.Caller(0)
	if !ok {
		fmt.Fprintln(os.Stderr, "arch_check: 无法定位源文件路径")
		os.Exit(1)
	}
	root, err := filepath.Abs(filepath.Join(filepath.Dir(srcFile), "..", "..", ".."))
	if err != nil {
		fmt.Fprintf(os.Stderr, "arch_check: 无法定位项目根: %v\n", err)
		os.Exit(1)
	}

	var violations []string
	walkErr := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		// 跳过约束层自身与 vendor
		if d.IsDir() && (d.Name() == ".harness" || d.Name() == "vendor" || d.Name() == ".git") {
			return filepath.SkipDir
		}
		if d.IsDir() || !strings.HasSuffix(d.Name(), ".go") {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		// 层归属由 checkFile 依据「相对模块根」的路径判断，
		// 不假设模块位于仓库根（本项目源码在 app/ 下）。
		checkFile(root, rel, &violations)
		return nil
	})
	if walkErr != nil {
		fmt.Fprintf(os.Stderr, "arch_check: 遍历失败: %v\n", walkErr)
		os.Exit(1)
	}

	if len(violations) > 0 {
		sort.Strings(violations)
		for _, v := range violations {
			fmt.Println(v)
		}
		os.Exit(1)
	}
}

// checkFile 检查单个 Go 文件的 import 依赖方向。
//
// 以文件所属 go.mod 所在目录为模块根，计算相对模块根的路径判断层归属。
// 这样 Go 代码无论放在仓库根还是子目录（如 app/），均能正确识别层。
func checkFile(root, rel string, violations *[]string) {
	absPath := filepath.Join(root, rel)
	moduleRoot := findModuleRoot(absPath, root)
	if moduleRoot == "" {
		// 分层目录下的源码必须能找到 go.mod；否则检查会静默失效。
		if underLayerDir(rel) {
			*violations = append(*violations, fmt.Sprintf(
				"%s: 未找到所属 go.mod，无法判断分层归属（arch-check 不能静默放行）", rel))
		}
		return
	}
	relToModule, err := filepath.Rel(moduleRoot, absPath)
	if err != nil {
		return
	}

	// 将相对模块根的路径转换为目录形式。
	dir := filepath.Dir(relToModule)
	dir = filepath.ToSlash(dir)
	// cmd/ 下的 main 包视为入口层
	var srcLayer layer
	if strings.HasPrefix(dir, "cmd/") || dir == "cmd" {
		srcLayer = layerEntry
	} else if strings.HasPrefix(dir, "internal/") {
		srcLayer = dirLayer(dir)
		if srcLayer == layerUnknown {
			// 新增内部包必须在层映射中登记，否则它将不受任何约束。
			*violations = append(*violations, fmt.Sprintf(
				"%s: internal 包目录 %q 未登记层归属，请在 .harness/constraints/arch/main.go 登记并同步 boundaries.md",
				rel, dir))
			return
		}
	} else {
		return // 既非入口层也非 internal 层（如模块根包），不纳入分层约束
	}

	modPath := modulePath(moduleRoot)
	if modPath == "" {
		*violations = append(*violations, fmt.Sprintf(
			"%s: %s/go.mod 缺少 module 声明，arch-check 不能静默放行", rel, moduleRoot))
		return
	}

	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, absPath, nil, parser.ImportsOnly)
	if err != nil {
		*violations = append(*violations, fmt.Sprintf("%s: 解析失败: %v", rel, err))
		return
	}

	for _, imp := range f.Imports {
		importPath := strings.Trim(imp.Path.Value, `"`)
		// 只检查项目自身模块内的依赖（分层约束只约束项目内部）
		dstPath, ok := strings.CutPrefix(importPath, modPath+"/")
		if !ok {
			continue
		}
		dstLayer := dirLayer(dstPath)
		if strings.HasPrefix(dstPath, "cmd/") || dstPath == "cmd" {
			dstLayer = layerEntry
		}
		if dstLayer == layerUnknown {
			if strings.HasPrefix(dstPath, "internal/") {
				pos := fset.Position(imp.Pos())
				*violations = append(*violations, fmt.Sprintf(
					"%s: 依赖的 internal 包 %q 未登记层归属，请在 .harness/constraints/arch/main.go 登记",
					pos, dstPath))
			}
			continue
		}
		if !allowed(srcLayer, dstLayer) {
			pos := fset.Position(imp.Pos())
			*violations = append(*violations, fmt.Sprintf(
				"%s: %s(%s) 不得依赖 %s(%s)。方向必须单向：入口 → 服务 → 适配 → 叶子。参见 .harness/specs/architecture/boundaries.md",
				pos, layerName(srcLayer), dir, layerName(dstLayer), dstPath))
		}
	}
}

// modulePath 读取 go.mod 首行的模块路径；读取失败返回空串。
func modulePath(moduleRoot string) string {
	data, err := os.ReadFile(filepath.Join(moduleRoot, "go.mod"))
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if rest, ok := strings.CutPrefix(line, "module "); ok {
			return strings.TrimSpace(rest)
		}
	}
	return ""
}

// underLayerDir 判断相对路径是否位于分层目录（internal/ 或 cmd/）之下。
func underLayerDir(rel string) bool {
	r := filepath.ToSlash(rel)
	for _, dir := range []string{"internal", "cmd"} {
		if r == dir || strings.HasPrefix(r, dir+"/") || strings.Contains(r, "/"+dir+"/") {
			return true
		}
	}
	return false
}

// findModuleRoot 从 file 向上查找最近的 go.mod，返回其所在目录；
// 查找范围不超过 root（项目根）。未找到返回空串。
func findModuleRoot(file, root string) string {
	dir := filepath.Dir(file)
	for {
		if fileExists(filepath.Join(dir, "go.mod")) {
			return dir
		}
		if dir == root || dir == filepath.Dir(dir) {
			break
		}
		dir = filepath.Dir(dir)
	}
	return ""
}

// fileExists 报告路径是否存在且非目录。
func fileExists(p string) bool {
	info, err := os.Stat(p)
	return err == nil && !info.IsDir()
}
