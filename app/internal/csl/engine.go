// engine.go 封装 citeproc-js 的 Go 会话。
//
// 与上游 citeproc-js-go 封装的区别：会话状态（citationsPre）挂在
// Engine 实例上，而非包级全局变量，因此同一进程内可安全创建多个独立会话；
// 同时把输出格式固定为 text，避免 HTML 标签泄漏到 Markdown 产物。

package csl

import (
	"fmt"
	"html"
	"regexp"
	"strconv"
	"strings"
	"sync"

	"github.com/dop251/goja"
)

// engineJS 在 goja 中初始化 CSL 引擎；locale 按请求语言返回内置资源。
const engineJS = `
let citations = {};
function addCitation(id, citation) { citations[id] = citation; }
citeprocSys = {
  retrieveLocale: function (lang) {
    lang = String(lang || "").toLowerCase();
    return lang.indexOf("zh") === 0 ? localeZh : localeEn;
  },
  retrieveItem: function (id) { return citations[id]; }
};
let engine = new CSL.Engine(citeprocSys, cslFile);
engine.setOutputFormat("text");
function pcc(citationData, pre) { return engine.processCitationCluster(citationData, pre, []); }
`

// 编译一次、复用到每个会话，避免每次都解析 500KB 的 citeproc-js。
// 延迟到首次渲染：search / lib 等不涉及引用的命令不必为 JS 解析付出启动成本。
var (
	citeprocOnce sync.Once
	citeprocProg *goja.Program
	citeprocErr  error
	engineOnce   sync.Once
	engineProg   *goja.Program
	engineErr    error
)

func loadCiteprocProgram() (*goja.Program, error) {
	citeprocOnce.Do(func() {
		citeprocProg, citeprocErr = goja.Compile("citeproc.min.js", citeprocJS, false)
	})
	return citeprocProg, citeprocErr
}

func loadEngineProgram() (*goja.Program, error) {
	engineOnce.Do(func() {
		engineProg, engineErr = goja.Compile("engine.js", engineJS, false)
	})
	return engineProg, engineErr
}

// Engine 是一个独立的 CSL 渲染会话。
type Engine struct {
	vm     *goja.Runtime
	pre    []interface{}
	nextID int
}

// NewEngine 用给定 CSL 样式创建会话。
func NewEngine(style []byte) (*Engine, error) {
	if strings.TrimSpace(string(style)) == "" {
		return nil, fmt.Errorf("csl: 样式内容为空")
	}
	vm := goja.New()
	vm.SetFieldNameMapper(goja.TagFieldNameMapper("json", true))
	for name, val := range map[string]string{
		"cslFile":  string(style),
		"localeZh": localeZhCN,
		"localeEn": localeEnUS,
	} {
		if err := vm.Set(name, val); err != nil {
			return nil, fmt.Errorf("csl: 注入 %s 失败: %w", name, err)
		}
	}
	citeprocProgram, err := loadCiteprocProgram()
	if err != nil {
		return nil, fmt.Errorf("csl: 编译 citeproc-js 失败: %w", err)
	}
	engineProgram, err := loadEngineProgram()
	if err != nil {
		return nil, fmt.Errorf("csl: 编译引擎脚本失败: %w", err)
	}
	if _, err := vm.RunProgram(citeprocProgram); err != nil {
		return nil, fmt.Errorf("csl: 加载 citeproc-js 失败: %w", err)
	}
	if _, err := vm.RunProgram(engineProgram); err != nil {
		return nil, fmt.Errorf("csl: 初始化样式失败（CSL 文件无效？）: %w", err)
	}
	return &Engine{vm: vm}, nil
}

// Add 注册条目，供后续 Cite / Bibliography 使用。
func (e *Engine) Add(items ...Item) error {
	fn, ok := goja.AssertFunction(e.vm.Get("addCitation"))
	if !ok {
		return fmt.Errorf("csl: addCitation 不可用")
	}
	for _, it := range items {
		if it.ID == "" {
			return fmt.Errorf("csl: 条目缺少 id")
		}
		if _, err := fn(goja.Undefined(), e.vm.ToValue(it.ID), e.vm.ToValue(it.JSON)); err != nil {
			return fmt.Errorf("csl: 注册条目 %s 失败: %w", it.ID, err)
		}
	}
	return nil
}

// Cite 渲染一个引用簇（一个或多个条目），返回正文标记。
func (e *Engine) Cite(items ...Item) (string, error) {
	if len(items) == 0 {
		return "", nil
	}
	citeItems := make([]map[string]interface{}, 0, len(items))
	seen := make(map[string]bool, len(items))
	for _, it := range items {
		if seen[it.ID] {
			continue
		}
		seen[it.ID] = true
		citeItems = append(citeItems, map[string]interface{}{"id": it.ID})
	}
	e.nextID++
	noteIndex := e.nextID
	citationID := "lk" + strconv.Itoa(e.nextID)
	citation := map[string]interface{}{
		"citationItems": citeItems,
		"properties":    map[string]interface{}{"noteIndex": noteIndex},
		"citationID":    citationID,
		"sortedItems":   nil,
	}
	fn, ok := goja.AssertFunction(e.vm.Get("pcc"))
	if !ok {
		return "", fmt.Errorf("csl: pcc 不可用")
	}
	val, err := fn(goja.Undefined(), e.vm.ToValue(citation), e.vm.ToValue(e.pre))
	if err != nil {
		return "", fmt.Errorf("csl: 渲染引用失败: %w", err)
	}
	mark, err := e.parseCluster(val, citationID)
	if err != nil {
		return "", err
	}
	if strings.Contains(mark, "NO_PRINTED_FORM") {
		return "", fmt.Errorf("csl: 样式缺少可用的 citation 布局")
	}
	return mark, nil
}

// parseCluster 解析 citeproc-js processCitationCluster 的返回值，并推进会话状态。
//
// 返回值形如 [returnData, [[position, rendered, citationID], ...]]；数组可能包含
// 文档中全部已处理引用簇，必须按 citationID 匹配本次调用，不能直接取第一个。
func (e *Engine) parseCluster(val goja.Value, citationID string) (string, error) {
	arr, ok := val.Export().([]interface{})
	if !ok || len(arr) < 2 {
		return "", fmt.Errorf("csl: 意外的 citation 返回值")
	}
	cluster, ok := arr[1].([]interface{})
	if !ok {
		return "", fmt.Errorf("csl: citation 返回值缺少引用列表")
	}
	for _, raw := range cluster {
		entry, ok := raw.([]interface{})
		if !ok || len(entry) < 3 {
			continue
		}
		id, ok := entry[2].(string)
		if !ok || id != citationID {
			continue
		}
		mark, ok := entry[1].(string)
		if !ok {
			return "", fmt.Errorf("csl: citation 标记不是字符串")
		}
		e.trackBibChange(arr, citationID)
		return mark, nil
	}
	return "", fmt.Errorf("csl: citation 返回值中找不到 %s", citationID)
}

// trackBibChange 在 citeproc-js 报告 bibchange 时记录本次 citationID，供后续簇排序。
func (e *Engine) trackBibChange(arr []interface{}, citationID string) {
	meta, ok := arr[0].(map[string]interface{})
	if !ok {
		return
	}
	change, ok := meta["bibchange"].(bool)
	if !ok || !change {
		return
	}
	e.pre = append(e.pre, []interface{}{citationID, "0"})
}

// Bibliography 返回文末参考文献条目（已去除 HTML 包装）。
func (e *Engine) Bibliography() ([]string, error) {
	val, err := e.vm.RunString(`engine.makeBibliography()`)
	if err != nil {
		return nil, fmt.Errorf("csl: 生成参考文献失败: %w", err)
	}
	exp := val.Export()
	// 样式没有 <bibliography> 时，citeproc-js 返回 false。
	if exp == nil || exp == false {
		return nil, nil
	}
	arr, ok := exp.([]interface{})
	if !ok || len(arr) < 2 {
		return nil, fmt.Errorf("csl: 意外的 bibliography 返回值")
	}
	entries, ok := arr[1].([]interface{})
	if !ok {
		return nil, fmt.Errorf("csl: bibliography 条目格式异常")
	}
	out := make([]string, 0, len(entries))
	for _, en := range entries {
		raw, ok := en.(string)
		if !ok {
			continue
		}
		if text := entryText(raw); text != "" {
			out = append(out, text)
		}
	}
	return out, nil
}

var (
	// divRe 匹配 citeproc-js 输出的 <div> 包装（second-field-align 会拆成左右两段）。
	divRe = regexp.MustCompile(`(?s)<div[^>]*>(.*?)</div>`)
	// tagRe 只剥离 CSL 已知的输出标签，避免把题名中的字面 < > 当作 HTML 吞掉。
	tagRe = regexp.MustCompile(`(?i)</?(?:span|div|i|b|em|strong|sup|sub)(?:\s[^>]*)?>`)
	// htmlReplacer 将常见格式标签转成 Markdown。
	htmlReplacer = strings.NewReplacer(
		"<i>", "*", "</i>", "*",
		"<b>", "**", "</b>", "**",
		"<em>", "*", "</em>", "*",
		"<strong>", "**", "</strong>", "**",
		"<sup>", "^", "</sup>", "",
		"<sub>", "_", "</sub>", "",
	)
)

// entryText 把 citeproc-js 的 HTML 片段转换为单行 Markdown 文本。
func entryText(raw string) string {
	parts := divRe.FindAllStringSubmatch(raw, -1)
	var s string
	if len(parts) > 0 {
		chunks := make([]string, 0, len(parts))
		for _, p := range parts {
			chunks = append(chunks, p[1])
		}
		s = strings.Join(chunks, " ")
	} else {
		s = raw
	}
	s = htmlReplacer.Replace(s)
	s = tagRe.ReplaceAllString(s, "")
	s = html.UnescapeString(s)
	return strings.Join(strings.Fields(s), " ")
}
