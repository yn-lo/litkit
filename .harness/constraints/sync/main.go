// sync 检查接口一致性门禁（CI docs job 调用，P0/P2）：
//
//  1. registration sync：CLI 子命令、MCP 工具、api.md §1/§2 三处清单一致
//     （FR-IFACE-03：新增功能必须 CLI 与 MCP 两处注册且同步文档）。
//  2. link check：CLAUDE.md 与 .harness/ 下 Markdown 相对链接指向存在
//     （NFR-MAINT-04）。
//
// 运行：cd .harness/constraints/sync && go run . [repo_root]
// 无外部依赖，仅标准库。
package main

import (
	"fmt"
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
	// server.go 中 gomcp.Tool{...} 的 Name 字面量（排除 Implementation）。
	toolNameRe = regexp.MustCompile(`(?s)&gomcp\.Tool\{.*?Name:\s+"([^"]+)"`)
	// 链接目标。
	linkRe = regexp.MustCompile(`\[[^\]]*\]\(([^)#]+)(?:#[^)]*)?\)`)
	// api.md §1 代码块中的命令行。
	litCmdRe = regexp.MustCompile(`(?m)^litkit (\S+)`)
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

// checkRegistration 校验 CLI 命令/api.md §1、MCP 工具/api.md §2 三处清单一致。
func checkRegistration(root string) error {
	var errs []string

	cliCmds, err := cliCommands(root)
	if err != nil {
		return err
	}
	mcpTools := mcpTools(root)
	apiCmds := apiSectionCommands(root)
	apiTools := apiSectionTools(root)

	// CLI ↔ api.md §1
	if d := diffSets(cliCmds, apiCmds); len(d) > 0 {
		errs = append(errs, fmt.Sprintf("CLI 命令与 api.md §1 不一致: %v", d))
	}
	// MCP ↔ api.md §2
	if d := diffSets(mcpTools, apiTools); len(d) > 0 {
		errs = append(errs, fmt.Sprintf("MCP 工具与 api.md §2 不一致: %v", d))
	}
	if len(errs) > 0 {
		return fmt.Errorf("%s", strings.Join(errs, "; "))
	}
	return nil
}

// cliCommands 从 cmd/litkit 源码提取 cobra 一级命令名（Use 首词）。
// 主命令集以 main.go 的 AddCommand 注册列表为准（10 个顶层命令），
// 每个命令的 Use 在其构造函数定义处（跨文件）提取，避免子命令
// （如 lib 下的 list/stats、lint 下的 init）混入顶层清单。
func cliCommands(root string) ([]string, error) {
	mainSrc, err := os.ReadFile(filepath.Join(root, "app", "cmd", "litkit", "main.go"))
	if err != nil {
		return nil, err
	}
	dir := filepath.Join(root, "app", "cmd", "litkit")
	files, err := filepath.Glob(filepath.Join(dir, "*.go"))
	if err != nil {
		return nil, err
	}
	all := ""
	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") {
			continue
		}
		b, err := os.ReadFile(f)
		if err != nil {
			return nil, err
		}
		all += string(b) + "\n"
	}
	names := map[string]bool{}
	for _, m := range newCmdRe.FindAllStringSubmatch(string(mainSrc), -1) {
		if m[1] == "Root" {
			continue
		}
		// 定向匹配：func newXxxCmd(...) 定义体内的 Use 首词。
		def := regexp.MustCompile(`(?s)func new` + m[1] + `Cmd\(.*?Use:\s+"([^"]+)"`).FindStringSubmatch(all)
		if len(def) > 1 {
			names[strings.Fields(def[1])[0]] = true
		} else {
			names[strings.ToLower(m[1])] = true
		}
	}
	return sortedKeys(names), nil
}

// mcpTools 从 internal/mcp/server.go 提取工具名（search_ 前缀规范化为占位符）。
func mcpTools(root string) []string {
	b, err := os.ReadFile(filepath.Join(root, "app", "internal", "mcp", "server.go"))
	if err != nil {
		return nil
	}
	names := map[string]bool{}
	for _, m := range toolNameRe.FindAllStringSubmatch(string(b), -1) {
		n := m[1]
		if n == "search_" {
			n = "search_<source>"
		}
		names[n] = true
	}
	return sortedKeys(names)
}

// apiSectionCommands 提取 api.md §1 的 CLI 一级命令。
func apiSectionCommands(root string) []string {
	b, err := os.ReadFile(filepath.Join(root, ".harness", "specs", "reference", "api.md"))
	if err != nil {
		return nil
	}
	section := sectionBetween(string(b), "## 1.", "## 2.")
	names := map[string]bool{}
	for _, m := range litCmdRe.FindAllStringSubmatch(section, -1) {
		names[m[1]] = true
	}
	return sortedKeys(names)
}

// apiSectionTools 提取 api.md §2 的 MCP 工具名。
// 表格第一列可能含多个反引号工具名（`lib_list` / `lib_search`），逐行按第一列提取。
func apiSectionTools(root string) []string {
	b, err := os.ReadFile(filepath.Join(root, ".harness", "specs", "reference", "api.md"))
	if err != nil {
		return nil
	}
	colRe := regexp.MustCompile(`^\|\s*(.*?)\s*\|`)
	btRe := regexp.MustCompile("`([^`|]+)`")
	section := sectionBetween(string(b), "## 2.", "## 3.")
	names := map[string]bool{}
	for _, line := range strings.Split(section, "\n") {
		col := colRe.FindStringSubmatch(line)
		if len(col) < 2 {
			continue
		}
		for _, m := range btRe.FindAllStringSubmatch(col[1], -1) {
			names[m[1]] = true
		}
	}
	return sortedKeys(names)
}

// ---- 2. link check ----

// checkLinks 扫描 CLAUDE.md 与 .harness/ 下 Markdown，验证相对链接目标存在。
func checkLinks(root string) error {
	var errs []string
	targets := []string{filepath.Join(root, "CLAUDE.md")}
	filepath.WalkDir(filepath.Join(root, ".harness"), func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(path, ".md") {
			return nil
		}
		targets = append(targets, path)
		return nil
	})
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
