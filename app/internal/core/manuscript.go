// manuscript.go 手稿占位符解析与渲染流水线（FR-REF-08/09/11）。
//
// 正文中 [@token] 形式的占位符（Pandoc 风格）：
//   - 裸 token 视为 citeKey，查本地库（先精确匹配，未命中再大写重试一次）
//   - 带前缀 doi:/pmid:/arxiv:/title: 的 token 经 MetadataFetcher 现场反查，成功后 UpsertPaper 入库取 citeKey
//   - 解析失败（库未命中、Fetch 未命中）记入 Unresolved，不静默丢弃
//   - 运行错误（网络、解析、入库失败）向上返回，不伪装成 Unresolved
//
// 正式样式由 CSL 引擎渲染；preview/草稿视图是独立的 Go 实现（不依赖引擎）。
// 同一论文（ComputeID 去重）无论被引用几次只占一个编号，按首次出现顺序编号。

package core

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"litkit/internal/csl"
	"litkit/internal/model"
)

// PaperStore 手稿流水线所需的最小存储接口（便于测试注入 fake）。
type PaperStore interface {
	GetByCiteKey(citeKey string) (*model.Paper, error)
	UpsertPaper(p model.Paper) (string, bool, error)
}

// Fetcher 手稿流水线所需的反查接口（*MetadataFetcher 实现之，便于测试注入 fake）。
type Fetcher interface {
	Fetch(ctx context.Context, idType, identifier string) (*model.Paper, error)
}

// ManuscriptResult 手稿处理产物。
type ManuscriptResult struct {
	Text         string            // 占位符替换后的正文
	CitationMap  map[string]string // token（原文写法）→ 正文引用标记，如 "[1]" 或 "(Vaswani, 2017)"
	Papers       []model.Paper     // 按首次出现顺序、去重后的论文（第 i 篇引用编号 = i+1）
	Unresolved   []string          // 未解析 token（原文出现顺序、去重）
	Bibliography []string          // 文末参考文献条目（已按样式渲染）
}

// ResolvedItem 一个占位符的解析结果。
type ResolvedItem struct {
	Start, End int
	Token      string // 占位符内的 token
	Raw        string // 原始 [@token] 文本
	PaperIndex int    // Papers 下标；-1 表示未解析
}

// ResolvedManuscript 解析完成、尚未渲染的手稿。
type ResolvedManuscript struct {
	Source     string // 展开引用组后的正文
	Items      []ResolvedItem
	Papers     []model.Paper
	Unresolved []string
}

// placeholderRe 匹配 [@token] 占位符（token 内不含 [ 与 ]；空 token [@] 天然不匹配）。
var placeholderRe = regexp.MustCompile(`\[@([^\[\]]+)\]`)

// bodyCiteRe 匹配正文中的裸 [@citeKey] 占位符（与 placeholderRe 配合：前者用于分组展开，
// 后者用于实际解析）。3-5 字母以支持 cite_key 空间耗尽升 4/5。
var bodyCiteRe = regexp.MustCompile(`\[@([a-zA-Z]{3,5}(?:\s*,\s*[a-zA-Z]{3,5})*)\]`)

// idTypePrefixes 支持的反查前缀（小写，与 MetadataFetcher.Fetch 的 id_type 对应）。
var idTypePrefixes = []string{"doi:", "pmid:", "arxiv:", "title:"}

// ResolveManuscript 解析手稿中的占位符并建立引用表，不产生任何渲染结果。
//
// 语义未命中（库中无此条目）只记入 Unresolved 不中断流水线；运行错误向上返回。
// 先解析一次，正式渲染与 preview 视图共享同一结果，避免 [@doi:] 重复联网反查。
func ResolveManuscript(ctx context.Context, store PaperStore, fetcher Fetcher, src string) (*ResolvedManuscript, error) {
	src = expandCiteGroups(src)
	matches := placeholderRe.FindAllStringSubmatchIndex(src, -1)
	res := &ResolvedManuscript{
		Source:     src,
		Items:      make([]ResolvedItem, len(matches)),
		Unresolved: make([]string, 0),
	}
	seen := make(map[string]int)    // paper.ID → Papers 下标
	unseen := make(map[string]bool) // 已记入 Unresolved 的 token（去重）

	for i, m := range matches {
		token := src[m[2]:m[3]]
		p, err := resolvePaper(ctx, store, fetcher, token)
		if err != nil {
			return nil, err
		}
		if p == nil {
			if !unseen[token] {
				unseen[token] = true
				res.Unresolved = append(res.Unresolved, token)
			}
			res.Items[i] = ResolvedItem{Start: m[0], End: m[1], Token: token, Raw: src[m[0]:m[1]], PaperIndex: -1}
			continue
		}
		if p.ID == "" {
			p.ID = p.ComputeID()
		}
		idx, ok := seen[p.ID]
		if !ok {
			idx = len(res.Papers)
			seen[p.ID] = idx
			res.Papers = append(res.Papers, *p)
		}
		res.Items[i] = ResolvedItem{Start: m[0], End: m[1], Token: token, Raw: src[m[0]:m[1]], PaperIndex: idx}
	}
	return res, nil
}

// FormatManuscript 用 CSL 样式渲染正文引用标记与文末列表。
func FormatManuscript(res *ResolvedManuscript, style *csl.Style) (*ManuscriptResult, error) {
	renderer, err := csl.NewRenderer(style)
	if err != nil {
		return nil, err
	}
	if err := renderer.Add(res.Papers); err != nil {
		return nil, err
	}
	out := &ManuscriptResult{
		CitationMap: make(map[string]string),
		Papers:      res.Papers,
		Unresolved:  res.Unresolved,
	}
	var b strings.Builder
	pos := 0
	for i := 0; i < len(res.Items); {
		it := res.Items[i]
		if it.PaperIndex < 0 {
			b.WriteString(res.Source[pos:it.Start])
			b.WriteString(it.Raw)
			pos = it.End
			i++
			continue
		}
		// 收集相邻已解析占位符（间隙仅空白），作为一个引用簇交给 CSL。
		group := []ResolvedItem{it}
		j := i + 1
		for j < len(res.Items) &&
			res.Items[j].PaperIndex >= 0 &&
			strings.TrimSpace(res.Source[res.Items[j-1].End:res.Items[j].Start]) == "" {
			group = append(group, res.Items[j])
			j++
		}
		b.WriteString(res.Source[pos:group[0].Start])
		ids := make([]string, 0, len(group))
		for _, g := range group {
			ids = append(ids, csl.ItemID(res.Papers[g.PaperIndex]))
		}
		mark, err := renderer.Cite(ids...)
		if err != nil {
			return nil, err
		}
		b.WriteString(mark)
		for _, g := range group {
			out.CitationMap[g.Token] = mark
		}
		pos = group[len(group)-1].End
		i = j
	}
	b.WriteString(res.Source[pos:])
	out.Text = b.String()
	out.Bibliography, err = renderer.Bibliography()
	if err != nil {
		return nil, err
	}
	return out, nil
}

// FormatPreview 渲染 preview/草稿视图：内联自描述标记 + 核对清单，不经过 CSL 引擎。
func FormatPreview(res *ResolvedManuscript) *ManuscriptResult {
	out := &ManuscriptResult{
		CitationMap: make(map[string]string),
		Papers:      res.Papers,
		Unresolved:  res.Unresolved,
	}
	var b strings.Builder
	pos := 0
	for _, it := range res.Items {
		b.WriteString(res.Source[pos:it.Start])
		if it.PaperIndex >= 0 {
			mark := previewMark(res.Papers[it.PaperIndex])
			out.CitationMap[it.Token] = mark
			b.WriteString(mark)
		} else {
			b.WriteString(it.Raw)
		}
		pos = it.End
	}
	b.WriteString(res.Source[pos:])
	out.Text = b.String()
	out.Bibliography = previewBibliography(res.Papers)
	return out
}

// expandCiteGroups 把 [@a,b] 拆成 [@a] [@b]；带前缀 token（identifier 内可能含逗号）保持原样。
func expandCiteGroups(src string) string {
	return bodyCiteRe.ReplaceAllStringFunc(src, func(raw string) string {
		body := raw[2 : len(raw)-1]
		if _, _, ok := splitPrefixed(body); ok {
			return raw
		}
		parts := strings.Split(body, ",")
		if len(parts) == 1 {
			return raw
		}
		var b strings.Builder
		for i, part := range parts {
			if i > 0 {
				b.WriteByte(' ')
			}
			b.WriteString("[@" + strings.TrimSpace(part) + "]")
		}
		return b.String()
	})
}

// resolvePaper 解析单个 token 为论文；语义未命中返回 (nil, nil)，运行错误向上返回。
//
// 裸 token → 视为 citeKey 查库（先精确，未命中再 strings.ToUpper 一次）；
// 带前缀 → MetadataFetcher.Fetch 现场反查，成功后 UpsertPaper 入库并以返回的 citeKey 补回。
func resolvePaper(ctx context.Context, store PaperStore, fetcher Fetcher, token string) (*model.Paper, error) {
	if store == nil {
		return nil, fmt.Errorf("manuscript: 未初始化文献库，无法解析 %s", token)
	}
	if idType, ident, ok := splitPrefixed(token); ok {
		if ident == "" || fetcher == nil {
			return nil, nil
		}
		p, err := fetcher.Fetch(ctx, idType, ident)
		if err != nil {
			return nil, fmt.Errorf("manuscript: 反查 %s 失败: %w", token, err)
		}
		if p == nil {
			return nil, nil
		}
		key, _, err := store.UpsertPaper(*p)
		if err != nil {
			return nil, fmt.Errorf("manuscript: 入库 %s 失败: %w", token, err)
		}
		p.CiteKey = key
		return p, nil
	}
	p, err := store.GetByCiteKey(token)
	if err != nil || p != nil {
		return p, err
	}
	return store.GetByCiteKey(strings.ToUpper(token))
}

// splitPrefixed 识别带前缀 token，返回 (idType, identifier, 是否带前缀)。
// 前缀大小写不敏感（如 DOI: 等同 doi:）；identifier 去首尾空白。
func splitPrefixed(token string) (string, string, bool) {
	lower := strings.ToLower(token)
	for _, pfx := range idTypePrefixes {
		if strings.HasPrefix(lower, pfx) {
			return strings.TrimSuffix(pfx, ":"), strings.TrimSpace(token[len(pfx):]), true
		}
	}
	return "", "", false
}

// previewMark 预览模式内联标记：有 DOI 用 [@doi:{DOI} — {标题}]，否则 [@标题]。
func previewMark(p model.Paper) string {
	title := cleanTitle(p.Title)
	if p.DOI != "" {
		return "[@doi:" + p.DOI + " — " + title + "]"
	}
	return "[@" + title + "]"
}

// previewBibliography 草稿视图的核对清单：编号 + 题名 + 作者 + 年份 + DOI。
func previewBibliography(papers []model.Paper) []string {
	out := make([]string, 0, len(papers))
	for i, p := range papers {
		parts := []string{cleanTitle(p.Title)}
		if a := authorsAPA(p.Authors); a != "" {
			parts = append(parts, strings.TrimSuffix(a, "."))
		}
		if p.Year > 0 {
			parts = append(parts, strconv.Itoa(p.Year))
		}
		if p.DOI != "" {
			parts = append(parts, p.DOI)
		}
		out = append(out, "["+strconv.Itoa(i+1)+"] "+strings.Join(parts, ". ")+".")
	}
	return out
}

// authorsAPA 按 APA 7 列至多 20 位作者；超过时列前 19 位、省略号和末位。
func authorsAPA(as []model.Author) string {
	const maxAuthors = 20
	parts := make([]string, 0, len(as))
	for _, a := range as {
		if name := authorAPA(a); name != "" {
			parts = append(parts, name)
		}
	}
	if len(parts) > maxAuthors {
		return strings.Join(parts[:maxAuthors-1], ", ") + ", … " + parts[len(parts)-1]
	}
	if len(parts) <= 1 {
		return strings.Join(parts, "")
	}
	return strings.Join(parts[:len(parts)-1], ", ") + ", & " + parts[len(parts)-1]
}

// authorAPA 单作者：Family, G. M.
func authorAPA(a model.Author) string {
	f := strings.TrimSpace(a.Family)
	g := strings.TrimSpace(a.Given)
	switch {
	case f != "" && g != "":
		return f + ", " + initials(g)
	case f != "":
		return f
	case g != "":
		return g
	}
	return ""
}

// initials 将名字转为首字母缩写（每个词取首字母大写 + "."，空格分隔）。
func initials(name string) string {
	fields := strings.Fields(name)
	if len(fields) == 0 {
		return ""
	}
	out := make([]string, 0, len(fields))
	for _, f := range fields {
		r := []rune(f)
		if len(r) == 0 {
			continue
		}
		out = append(out, strings.ToUpper(string(r[0]))+".")
	}
	return strings.Join(out, " ")
}

// cleanTitle 清理题名：折叠空白、去首尾空格。
func cleanTitle(t string) string {
	return strings.Join(strings.Fields(t), " ")
}

// 手稿产物逻辑名（map key，FR-REF-11）。实际落盘文件名为 {base}_{时间戳}.{ext}。
const (
	ManuscriptFormatted = "formatted.md" // 正文 + 文末参考文献列表
	ManuscriptBib       = "refs.bib"
	ManuscriptRIS       = "refs.ris"
	manuscriptFilePerm  = 0o600
)

// ManuscriptStamp 生成产物文件名时间戳（精确到毫秒，所有产物共用同一值）。
// 毫秒级避免 AI 重试或并发调用在同一秒内静默覆盖上一批产物。
func ManuscriptStamp(t time.Time) string {
	return t.Format("20060102_150405_000")
}

// RefListHeading 文末文献节标题：英文模式用 References，其余用 参考文献。
func RefListHeading(lang string) string {
	if strings.EqualFold(strings.TrimSpace(lang), "en") {
		return "References"
	}
	return "参考文献"
}

// formattedContent 构建最终文稿：正文 + 文末参考文献列表（FR-REF-09/11）。
// 无论何种样式（含 preview）均追加列表；无引用论文时仅返回正文。
func formattedContent(res *ManuscriptResult, heading string) string {
	if len(res.Bibliography) == 0 {
		return res.Text
	}
	return res.Text + "\n\n# " + heading + "\n\n" + strings.Join(res.Bibliography, "\n\n")
}

// WriteManuscriptOutputs 落盘 {base}_{ts}.md（正文+文末引用列表）/ {base}_{ts}.bib / {base}_{ts}.ris。
//
// 返回 逻辑名 → 绝对路径 映射（ManuscriptFormatted/ManuscriptBib/ManuscriptRIS）；
// 不含 docx（由调用方按需经 PandocToDocx 转换）。
func WriteManuscriptOutputs(outDir, base, ts string, res *ManuscriptResult, heading string) (map[string]string, error) {
	files := make(map[string]string)
	text := formattedContent(res, heading)
	name := base + "_" + ts + ".md"
	if err := os.WriteFile(filepath.Join(outDir, name), []byte(text), manuscriptFilePerm); err != nil {
		return nil, fmt.Errorf("manuscript: 写入 %s 失败: %w", name, err)
	}
	files[ManuscriptFormatted] = filepath.Join(outDir, name)

	if err := os.WriteFile(filepath.Join(outDir, base+"_"+ts+".bib"), []byte(BibTeXFromPapers(res.Papers)), manuscriptFilePerm); err != nil {
		return nil, fmt.Errorf("manuscript: 写入 bib 失败: %w", err)
	}
	files[ManuscriptBib] = filepath.Join(outDir, base+"_"+ts+".bib")

	if err := os.WriteFile(filepath.Join(outDir, base+"_"+ts+".ris"), []byte(RISFromPapers(res.Papers)), manuscriptFilePerm); err != nil {
		return nil, fmt.Errorf("manuscript: 写入 ris 失败: %w", err)
	}
	files[ManuscriptRIS] = filepath.Join(outDir, base+"_"+ts+".ris")
	return files, nil
}

// WritePreviewOutput 额外写出 preview 版本 md（{base}_{ts}.preview.md）。
// 供 --preview 时与正常 formatted.md 并存输出，便于人工核查引用指向。
func WritePreviewOutput(outDir, base, ts string, res *ManuscriptResult, heading string) (string, error) {
	text := formattedContent(res, heading)
	name := base + "_" + ts + ".preview.md"
	if err := os.WriteFile(filepath.Join(outDir, name), []byte(text), manuscriptFilePerm); err != nil {
		return "", fmt.Errorf("manuscript: 写入 preview 失败: %w", err)
	}
	return filepath.Join(outDir, name), nil
}

// PandocToDocx 调用 pandoc 将 markdown 转为 docx（FR-REF-11）。
// 可执行文件固定为 pandoc，参数为本地文件路径；调用方负责 LookPath 预检与降级提示。
func PandocToDocx(src, dst string) error {
	// #nosec G204 -- 可执行文件固定为 pandoc，参数为本地文件路径，无 shell 注入面。
	cmd := exec.Command("pandoc", src, "-o", dst)
	// 子进程不继承 LITKIT_* 配置（其中可能含 API key），避免密钥外泄给 pandoc。
	cmd.Env = envWithoutLitkit(os.Environ())
	out, err := cmd.CombinedOutput()
	if err != nil {
		if len(out) > 0 {
			return fmt.Errorf("%v: %s", err, strings.TrimSpace(string(out)))
		}
		return err
	}
	return nil
}

// envWithoutLitkit 过滤掉 LITKIT_ 前缀的环境变量（子进程不应得知本工具凭据）。
// Windows 环境变量名大小写不敏感，故判断前缀时统一转大写。
func envWithoutLitkit(env []string) []string {
	out := make([]string, 0, len(env))
	for _, kv := range env {
		if strings.HasPrefix(strings.ToUpper(kv), "LITKIT_") {
			continue
		}
		out = append(out, kv)
	}
	return out
}
