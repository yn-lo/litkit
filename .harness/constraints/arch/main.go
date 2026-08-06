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
//	适配层 internal/sources                   允许依赖 叶子层
//	叶子层 internal/model·config·storage·util·embedding·buildinfo 只允许依赖其他叶子层
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

// entryPrefixes / servicePrefixes / adapterPrefixes / leafPrefixes
// 目录前缀 → 层映射。新增目录时在此登记，并同步更新 boundaries.md。
var (
	entryPrefixes   = []string{}
	servicePrefixes = []string{"internal/core", "internal/lint"}
	adapterPrefixes = []string{"internal/sources"}
	leafPrefixes    = []string{"internal/model", "internal/config", "internal/storage", "internal/util", "internal/embedding", "internal/buildinfo"}
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
	for _, p := range entryPrefixes {
		if importPath == p || strings.HasPrefix(importPath, p+"/") {
			return layerEntry
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
		// 只检查 internal/ 与 cmd/ 下的源码
		if !strings.HasPrefix(rel, "internal"+string(filepath.Separator)) &&
			!strings.HasPrefix(rel, "cmd"+string(filepath.Separator)) {
			return nil
		}
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
		return // 不在任何 Go 模块内（如已被 .harness 跳过的约束层自身）
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
	} else {
		srcLayer = dirLayer(dir)
	}
	if srcLayer == layerUnknown {
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
		// 只检查对项目内部 internal/ 的依赖（分层约束只约束项目内部，模块前缀任意）
		internalIdx := strings.Index(importPath, "/internal/")
		if internalIdx < 0 {
			continue
		}
		dstPath := importPath[internalIdx+1:] // internal/xxx
		dstLayer := dirLayer(dstPath)
		if dstLayer == layerUnknown {
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
