// sync 检查接口一致性门禁（CI docs job 调用，P0/P2）：
//
//  1. registration sync：CLI 子命令与 api.md §1 清单一致。
//  2. link check：AGENTS.md 与 .harness/ 下 Markdown 相对链接指向存在
//     （NFR-MAINT-04）。
//
// 运行：cd .harness/constraints/sync && go run . [repo_root]
// 无外部依赖，仅标准库。
package main

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// ---- 提取规则 ----

var (
	// main.go 中 AddCommand 注册的构造函数名（保持出现顺序）。
	newCmdRe = regexp.MustCompile(`new(\w+)Cmd\(`)
	// 链接目标。
	linkRe = regexp.MustCompile(`\[[^\]]*\]\(([^)#]+)(?:#[^)]*)?\)`)
	// litCmdLineRe 匹配 api.md §1 代码块中以 litkit 开头的整行命令。
	litCmdLineRe = regexp.MustCompile(`(?m)^litkit (.+)$`)
	// identRe 判断子命令是否为裸标识符（排除 <arg>、[flag] 等）。
	identRe = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_-]*$`)
	// envKeyRe 匹配 config.go 中读取的环境变量键（含动态后缀前缀）。
	envKeyRe = regexp.MustCompile(`(?:os\.Getenv|getenvDefault|getenvInt)\("(LITKIT_[A-Z0-9_]+)"`)
	// apiEnvRowRe 匹配 api.md §2 环境变量表首列。
	apiEnvRowRe = regexp.MustCompile(`(?m)^\| (LITKIT_[A-Z0-9_]+)`)
)

// main 入口。
func main() {
	root := repoRoot()
	fail := 0
	if err := checkRegistration(root); err != nil {
		fmt.Fprintln(os.Stderr, "FAIL registration:", err)
		fail++
	}
	if err := checkLinks(root); err != nil {
		fmt.Fprintln(os.Stderr, "FAIL links:", err)
		fail++
	}
	if err := checkEnvSync(root); err != nil {
		fmt.Fprintln(os.Stderr, "FAIL env:", err)
		fail++
	}
	if fail > 0 {
		fmt.Fprintf(os.Stderr, "sync: %d 项检查未通过\n", fail)
		os.Exit(1)
	}
	fmt.Println("sync: registration + links 全部通过")
}

// repoRoot 定位仓库根：参数优先，否则 CWD 向上 3 级。
func repoRoot() string {
	if len(os.Args) > 1 {
		return filepath.Clean(os.Args[1])
	}
	return filepath.Clean(filepath.Join(mustwd(), "..", "..", ".."))
}

// ---- 1. registration sync ----

// checkRegistration 校验 CLI 命令树（一级 + 二级）与 api.md §1 一致。
func checkRegistration(root string) error {
	cliCmds, err := cliCommandPaths(root)
	if err != nil {
		return err
	}
	apiCmds := apiCommandPaths(root)
	if d := diffSets(cliCmds, apiCmds); len(d) > 0 {
		return fmt.Errorf("CLI 命令树与 api.md §1 不一致: %s", d)
	}
	return nil
}

// cliCommandPaths 从 cmd/litkit 源码提取命令路径（"lib"、"lib add" 等）。
// 一级命令以 main.go 的 AddCommand 注册列表为准；二级命令由各构造函数体内的
// AddCommand 调用解析（go/ast），避免子命令增删被漏检。
func cliCommandPaths(root string) ([]string, error) {
	mainSrc, err := os.ReadFile(filepath.Join(root, "app", "cmd", "litkit", "main.go"))
	if err != nil {
		return nil, err
	}
	dir := filepath.Join(root, "app", "cmd", "litkit")
	files, err := filepath.Glob(filepath.Join(dir, "*.go"))
	if err != nil {
		return nil, err
	}
	sources := map[string]string{}
	all := ""
	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") {
			continue
		}
		b, err := os.ReadFile(f)
		if err != nil {
			return nil, err
		}
		sources[filepath.Base(f)] = string(b)
		all += string(b) + "\n"
	}
	uses := constructorUses(all)
	paths := map[string]bool{}
	for _, m := range newCmdRe.FindAllStringSubmatch(string(mainSrc), -1) {
		ctor := m[1]
		if ctor == "Root" {
			continue
		}
		name := useName(uses, ctor)
		paths[name] = true
		for _, child := range constructorChildren(sources, "new"+ctor+"Cmd") {
			paths[name+" "+useName(uses, child)] = true
		}
	}
	return sortedKeys(paths), nil
}

// constructorUses 收集构造函数体内 Use 首词（跨文件）。
func constructorUses(all string) map[string]string {
	uses := map[string]string{}
	for _, m := range regexp.MustCompile(`(?s)func new(\w+)Cmd\(.*?Use:\s+"([^"]+)"`).FindAllStringSubmatch(all, -1) {
		uses[m[1]] = strings.Fields(m[2])[0]
	}
	return uses
}

// useName 返回构造函数的命令名；缺失 Use 时回退为小写构造函数名。
func useName(uses map[string]string, ctor string) string {
	if n, ok := uses[ctor]; ok {
		return n
	}
	return strings.ToLower(ctor)
}

// constructorChildren 解析指定构造函数体内的 AddCommand(newXxxCmd(...)) 调用。
func constructorChildren(sources map[string]string, funcName string) []string {
	var out []string
	for _, src := range sources {
		fset := token.NewFileSet()
		f, err := parser.ParseFile(fset, "", src, 0)
		if err != nil {
			continue
		}
		for _, d := range f.Decls {
			fd, ok := d.(*ast.FuncDecl)
			if !ok || fd.Name.Name != funcName || fd.Body == nil {
				continue
			}
			ast.Inspect(fd.Body, func(n ast.Node) bool {
				call, ok := n.(*ast.CallExpr)
				if !ok {
					return true
				}
				sel, ok := call.Fun.(*ast.SelectorExpr)
				if !ok || sel.Sel.Name != "AddCommand" {
					return true
				}
				for _, a := range call.Args {
					ac, ok := a.(*ast.CallExpr)
					if !ok {
						continue
					}
					id, ok := ac.Fun.(*ast.Ident)
					if !ok || !strings.HasPrefix(id.Name, "new") || !strings.HasSuffix(id.Name, "Cmd") {
						continue
					}
					out = append(out, strings.TrimSuffix(strings.TrimPrefix(id.Name, "new"), "Cmd"))
				}
				return true
			})
		}
	}
	return out
}

// apiCommandPaths 提取 api.md §1 的命令路径（含二级子命令）。
func apiCommandPaths(root string) []string {
	b, err := os.ReadFile(filepath.Join(root, ".harness", "specs", "reference", "api.md"))
	if err != nil {
		return nil
	}
	section := sectionBetween(string(b), "## 1.", "## 2.")
	names := map[string]bool{}
	for _, m := range litCmdLineRe.FindAllStringSubmatch(section, -1) {
		line := strings.TrimSpace(m[1])
		if i := strings.Index(line, " #"); i >= 0 {
			line = strings.TrimSpace(line[:i])
		}
		fields := strings.Fields(line)
		if len(fields) == 0 {
			continue
		}
		parent := fields[0]
		names[parent] = true
		hasSub := len(fields) >= 2 && identRe.MatchString(fields[1])
		if hasSub {
			names[parent+" "+fields[1]] = true
			// 兼容 "litkit lib stats | path" 这类合并写法；
			// 含方括号的行（如 [--mode chapter|draft|final]）的 | 是枚举分隔，不是子命令。
			if !strings.Contains(line, "[") {
				for _, seg := range strings.Split(line, "|")[1:] {
					if f := strings.Fields(seg); len(f) == 1 && identRe.MatchString(f[0]) {
						names[parent+" "+f[0]] = true
					}
				}
			}
		}
	}
	return sortedKeys(names)
}

// ---- 2. env sync ----

// checkEnvSync 校验 config.go 读取的环境变量键与 api.md §2 表格一致。
func checkEnvSync(root string) error {
	cfg, err := os.ReadFile(filepath.Join(root, "app", "internal", "config", "config.go"))
	if err != nil {
		return err
	}
	api, err := os.ReadFile(filepath.Join(root, ".harness", "specs", "reference", "api.md"))
	if err != nil {
		return err
	}
	section := sectionBetween(string(api), "## 2.", "## 3.")
	codeKeys := map[string]bool{}
	for _, m := range envKeyRe.FindAllStringSubmatch(string(cfg), -1) {
		codeKeys[m[1]] = true
	}
	docKeys := map[string]bool{}
	for _, m := range apiEnvRowRe.FindAllStringSubmatch(section, -1) {
		docKeys[m[1]] = true
	}
	if d := diffSets(sortedKeys(codeKeys), sortedKeys(docKeys)); len(d) > 0 {
		return fmt.Errorf("config.go 与 api.md §2 环境变量不一致: %s", d)
	}
	return nil
}

// ---- 3. link check ----

// checkLinks 扫描 AGENTS.md 与 .harness/ 下 Markdown，验证相对链接目标存在。
func checkLinks(root string) error {
	var errs []string
	targets := []string{filepath.Join(root, "AGENTS.md")}
	walkErr := filepath.WalkDir(filepath.Join(root, ".harness"), func(path string, d os.DirEntry, err error) error {
		if err != nil {
			errs = append(errs, err.Error())
			return nil
		}
		if d.IsDir() || !strings.HasSuffix(path, ".md") {
			return nil
		}
		targets = append(targets, path)
		return nil
	})
	if walkErr != nil {
		errs = append(errs, walkErr.Error())
	}
	for _, md := range targets {
		b, err := os.ReadFile(md)
		if err != nil {
			errs = append(errs, err.Error())
			continue
		}
		for _, seg := range outsideCodeBlocks(string(b)) {
			for _, m := range linkRe.FindAllStringSubmatch(seg, -1) {
				dest := m[1]
				if isExternal(dest) {
					continue
				}
				resolved := filepath.Clean(filepath.Join(filepath.Dir(md), dest))
				if _, err := os.Stat(resolved); err != nil {
					errs = append(errs, fmt.Sprintf("%s: 链接目标不存在: %s", rel(root, md), dest))
				}
			}
		}
	}
	if len(errs) > 0 {
		return fmt.Errorf("%s", strings.Join(errs, "; "))
	}
	return nil
}

// ---- 工具函数 ----

// sectionBetween 返回 start 与 end 之间的文本（行首匹配）。
func sectionBetween(s, start, end string) string {
	i := strings.Index(s, start)
	if i < 0 {
		return ""
	}
	j := strings.Index(s[i:], end)
	if j < 0 {
		return s[i:]
	}
	return s[i : i+j]
}

// outsideCodeBlocks 按 ``` 切分，返回代码块外的片段。
func outsideCodeBlocks(s string) []string {
	var out []string
	for i, seg := range strings.Split(s, "```") {
		if i%2 == 0 {
			out = append(out, seg)
		}
	}
	return out
}

// isExternal 判断链接目标是否为外链/锚点/空。
func isExternal(dest string) bool {
	d := strings.TrimSpace(dest)
	return d == "" || strings.HasPrefix(d, "#") ||
		strings.HasPrefix(d, "http://") || strings.HasPrefix(d, "https://") ||
		strings.HasPrefix(d, "mailto:")
}

// diffSets 返回集合差异（多出/缺少）的人类可读描述；一致时为空。
func diffSets(got, want []string) string {
	g, w := set(got), set(want)
	var d []string
	for k := range g {
		if !w[k] {
			d = append(d, "多出 "+k)
		}
	}
	for k := range w {
		if !g[k] {
			d = append(d, "缺少 "+k)
		}
	}
	sort.Strings(d)
	return strings.Join(d, ", ")
}

func set(xs []string) map[string]bool {
	m := make(map[string]bool, len(xs))
	for _, x := range xs {
		m[x] = true
	}
	return m
}

func sortedKeys(m map[string]bool) []string {
	ks := make([]string, 0, len(m))
	for k := range m {
		ks = append(ks, k)
	}
	sort.Strings(ks)
	return ks
}

func rel(root, p string) string {
	r, err := filepath.Rel(root, p)
	if err != nil {
		return p
	}
	return r
}

func mustwd() string {
	wd, err := os.Getwd()
	if err != nil {
		panic(err)
	}
	return wd
}
