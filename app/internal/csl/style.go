// style.go 引用样式注册表：内置 CSL（go:embed）+ WORK_DIR/styles 用户样式 + 显式路径。
//
// 内置名保留：用户目录中的同名文件不会覆盖内置样式，只在 List 中标记冲突。

package csl

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const (
	// dirPerm / filePerm 与仓库其他落盘产物对齐（0750 / 0600）。
	dirPerm  = 0o750
	filePerm = 0o600
)

// Style 一份可渲染的 CSL 样式。
type Style struct {
	Name    string
	Content []byte
	Builtin bool
}

// StyleInfo 样式清单项。
type StyleInfo struct {
	Name     string `json:"name"`
	Builtin  bool   `json:"builtin"`
	Path     string `json:"path,omitempty"`
	Conflict bool   `json:"conflict,omitempty"` // 用户文件与内置名冲突，已被忽略
}

var builtinStyles = map[string][]byte{
	"gb7714-2025": []byte(styleGB7714),
	"apa":         []byte(styleAPA),
	"ieee":        []byte(styleIEEE),
}

// builtinOrder 固定内置样式输出顺序。
var builtinOrder = []string{"gb7714-2025", "apa", "ieee"}

// DefaultName 返回语言模式对应的默认样式。
func DefaultName(lang string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(lang)) {
	case "zh":
		return "gb7714-2025", nil
	case "en":
		return "apa", nil
	default:
		return "", fmt.Errorf("csl: 未知语言模式 %q（支持 zh|en）", lang)
	}
}

// Resolve 解析样式引用：空值按 lang 取默认；.csl 路径直接加载；
// 其余按内置名 → 用户目录 <stylesDir>/<name>.csl 顺序查找。
func Resolve(lang, ref, stylesDir string) (*Style, error) {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		name, err := DefaultName(lang)
		if err != nil {
			return nil, err
		}
		return builtinStyle(name)
	}
	if isPathRef(ref) {
		return loadStyleFile(ref)
	}
	if content, ok := builtinStyles[strings.ToLower(ref)]; ok {
		name := strings.ToLower(ref)
		return &Style{Name: name, Content: content, Builtin: true}, nil
	}
	if stylesDir != "" {
		if path, ok := findUserStyleFile(stylesDir, ref); ok {
			return loadStyleFile(path)
		}
	}
	return nil, fmt.Errorf("csl: 未知样式 %q（内置：%s；也可传入 .csl 路径）",
		ref, strings.Join(builtinOrder, "|"))
}

// List 返回内置样式与用户目录样式的合并清单。
func List(stylesDir string) []StyleInfo {
	out := make([]StyleInfo, 0, len(builtinOrder))
	for _, name := range builtinOrder {
		out = append(out, StyleInfo{Name: name, Builtin: true})
	}
	if stylesDir == "" {
		return out
	}
	for _, path := range listUserStyleFiles(stylesDir) {
		name := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
		info := StyleInfo{Name: name, Path: path}
		if _, reserved := builtinStyles[strings.ToLower(name)]; reserved {
			info.Conflict = true
		}
		out = append(out, info)
	}
	return out
}

// findUserStyleFile 在用户样式目录中按 stem 查找 .csl（先精确、再大小写不敏感）。
// 用 ReadDir 而非 Glob，保证 Linux 下也能发现 .CSL 扩展名。
func findUserStyleFile(dir, name string) (string, bool) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return "", false
	}
	var folded string
	for _, e := range entries {
		if e.IsDir() || !strings.EqualFold(filepath.Ext(e.Name()), ".csl") {
			continue
		}
		stem := strings.TrimSuffix(e.Name(), filepath.Ext(e.Name()))
		if stem == name {
			return filepath.Join(dir, e.Name()), true
		}
		if folded == "" && strings.EqualFold(stem, name) {
			folded = filepath.Join(dir, e.Name())
		}
	}
	return folded, folded != ""
}

// listUserStyleFiles 列出目录内所有 .csl（扩展名大小写不敏感，结果排序稳定）。
func listUserStyleFiles(dir string) []string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	paths := make([]string, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() || !strings.EqualFold(filepath.Ext(e.Name()), ".csl") {
			continue
		}
		paths = append(paths, filepath.Join(dir, e.Name()))
	}
	sort.Strings(paths)
	return paths
}

// Show 返回指定样式的原始 CSL 内容。
func Show(name, stylesDir string) (*Style, error) {
	return Resolve("zh", name, stylesDir)
}

// Export 将内置样式写出为 <name>-custom.csl（内置名保留，避免同名文件被静默忽略），
// 返回落盘路径；目标已存在时报错，避免覆盖用户文件。
func Export(name, outDir string) (string, error) {
	name = strings.ToLower(strings.TrimSpace(name))
	content, ok := builtinStyles[name]
	if !ok {
		return "", fmt.Errorf("csl: %q 不是内置样式（内置：%s）", name, strings.Join(builtinOrder, "|"))
	}
	if outDir == "" {
		return "", fmt.Errorf("csl: 未指定导出目录")
	}
	if err := os.MkdirAll(outDir, dirPerm); err != nil {
		return "", fmt.Errorf("csl: 创建目录失败: %w", err)
	}
	path := filepath.Join(outDir, name+"-custom.csl")
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, filePerm)
	if err != nil {
		return "", fmt.Errorf("csl: 写入 %s 失败（文件已存在？）: %w", path, err)
	}
	if _, err := f.Write(content); err != nil {
		_ = f.Close()
		return "", fmt.Errorf("csl: 写入 %s 失败: %w", path, err)
	}
	if err := f.Close(); err != nil {
		return "", fmt.Errorf("csl: 写入 %s 失败: %w", path, err)
	}
	return path, nil
}

func builtinStyle(name string) (*Style, error) {
	name = strings.ToLower(strings.TrimSpace(name))
	content, ok := builtinStyles[name]
	if !ok {
		return nil, fmt.Errorf("csl: 未注册的内置样式 %q", name)
	}
	return &Style{Name: name, Content: content, Builtin: true}, nil
}

// isPathRef 判断参数是否按文件路径处理（显式 .csl 后缀或含路径分隔符）。
func isPathRef(ref string) bool {
	lower := strings.ToLower(ref)
	return strings.HasSuffix(lower, ".csl") ||
		strings.ContainsAny(ref, `/\`)
}

// loadStyleFile 读取并校验 CSL 文件。
func loadStyleFile(path string) (*Style, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("csl: 读取样式 %s 失败: %w", path, err)
	}
	if !strings.Contains(string(data), "<style") {
		return nil, fmt.Errorf("csl: %s 不是有效 CSL 文件（缺少 <style>）", path)
	}
	if err := validateXML(data); err != nil {
		return nil, fmt.Errorf("csl: %s XML 无效: %w", path, err)
	}
	name := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	return &Style{Name: name, Content: data}, nil
}

// validateXML 校验 CSL 文件是否为良构 XML（不校验 CSL 语义）。
func validateXML(data []byte) error {
	dec := xml.NewDecoder(bytes.NewReader(data))
	for {
		if _, err := dec.Token(); err != nil {
			if err == io.EOF {
				return nil
			}
			return err
		}
	}
}
