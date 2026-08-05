// manuscript.go 手稿占位符解析流水线（FR-REF-09）。
//
// 正文中 [@token] 形式的占位符（Pandoc 风格）：
//   - 裸 token 视为 citeKey，查本地库（先精确匹配，未命中再大写重试一次）
//   - 带前缀 doi:/pmid:/arxiv:/title: 的 token 经 MetadataFetcher 现场反查，成功后 UpsertPaper 入库取 citeKey
//   - 解析失败（库未命中、Fetch 未命中或错误）记入 Unresolved，不静默丢弃
//
// 同一论文（ComputeID 去重）无论被引用几次只占一个编号，按首次出现顺序编号。

package core

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

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
	Text        string            // 占位符替换后的正文
	CitationMap map[string]string // token（原文写法）→ 正文引用标记，如 "[1]" 或 "(Vaswani, 2017)"
	Papers      []model.Paper     // 按首次出现顺序、去重后的论文（第 i 篇引用编号 = i+1）
	Unresolved  []string          // 未解析 token（原文出现顺序、去重）
}

// placeholderRe 匹配 [@token] 占位符（token 内不含 [ 与 ]；空 token [@] 天然不匹配）。
var placeholderRe = regexp.MustCompile(`\[@([^\[\]]+)\]`)

// idTypePrefixes 支持的反查前缀（小写，与 MetadataFetcher.Fetch 的 id_type 对应）。
var idTypePrefixes = []string{"doi:", "pmid:", "arxiv:", "title:"}

// ProcessManuscript 解析手稿中 [@token] 占位符并生成引用表。
//
// style 决定正文引用标记：GB7714/IEEE 用 "[n]"；APA 用 "(Author, Year)"。
// per-token 解析失败只记入 Unresolved 不中断流水线；仅参数级错误（未知 style）返回 error。
func ProcessManuscript(ctx context.Context, store PaperStore, fetcher Fetcher, src string, style Style) (*ManuscriptResult, error) {
	switch style {
	case StyleGB7714, StyleAPA, StyleIEEE, StylePreview:
	default:
		return nil, fmt.Errorf("manuscript: 未知样式 %q（支持 gb7714-2025|apa|ieee|preview）", style)
	}

	res := &ManuscriptResult{CitationMap: make(map[string]string)}
	seen := make(map[string]int)    // paper.ID → Papers 下标
	unseen := make(map[string]bool) // 已记入 Unresolved 的 token（去重）

	matches := placeholderRe.FindAllStringSubmatchIndex(src, -1)

	// item 记录一个占位符匹配及其解析结果。
	type item struct {
		start, end int // 匹配在 src 中的位置
		number     int // 1-based 引用编号；0 表示未解析
		token      string
		raw        string // 原始 [@token] 文本
	}
	items := make([]item, len(matches))
	for i, m := range matches {
		token := src[m[2]:m[3]]
		p, err := resolvePaper(ctx, store, fetcher, token)
		if err != nil || p == nil {
			if !unseen[token] {
				unseen[token] = true
				res.Unresolved = append(res.Unresolved, token)
			}
			items[i] = item{start: m[0], end: m[1], number: 0, token: token, raw: src[m[0]:m[1]]}
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
		items[i] = item{start: m[0], end: m[1], number: idx + 1, token: token, raw: src[m[0]:m[1]]}
	}

	// 构建输出正文：编号样式折叠相邻引用，其他样式逐个替换。
	collapsible := style == StyleGB7714 || style == StyleIEEE
	var b strings.Builder
	pos := 0
	for i := 0; i < len(items); {
		// 不可折叠或未解析：逐个替换
		if !collapsible || items[i].number == 0 {
			b.WriteString(src[pos:items[i].start])
			mark := items[i].raw
			if items[i].number > 0 {
				mark = citationMark(res.Papers[items[i].number-1], style, items[i].number)
				res.CitationMap[items[i].token] = mark
			}
			b.WriteString(mark)
			pos = items[i].end
			i++
			continue
		}
		// 收集相邻已解析占位符（间隙仅空白）
		group := []item{items[i]}
		j := i + 1
		for j < len(items) && strings.TrimSpace(src[items[j-1].end:items[j].start]) == "" && items[j].number > 0 {
			group = append(group, items[j])
			j++
		}
		b.WriteString(src[pos:group[0].start])
		// 编号去重 + 升序
		numSet := make(map[int]bool, len(group))
		nums := make([]int, 0, len(group))
		for _, g := range group {
			if !numSet[g.number] {
				numSet[g.number] = true
				nums = append(nums, g.number)
			}
		}
		sort.Ints(nums)
		mark := "[" + collapseNumbers(nums) + "]"
		b.WriteString(mark)
		for _, g := range group {
			res.CitationMap[g.token] = mark
		}
		pos = group[len(group)-1].end
		i = j
	}
	b.WriteString(src[pos:])
	res.Text = b.String()
	return res, nil
}

// collapseNumbers 将升序去重编号列表折叠为范围字符串。
// [1,2,3] → "1-3", [1,3,4,5] → "1,3-5", [1,3] → "1,3", [1] → "1"。
func collapseNumbers(nums []int) string {
	if len(nums) == 0 {
		return ""
	}
	if len(nums) == 1 {
		return strconv.Itoa(nums[0])
	}
	var parts []string
	start, end := nums[0], nums[0]
	for i := 1; i < len(nums); i++ {
		if nums[i] == end+1 {
			end = nums[i]
		} else {
			parts = append(parts, rangeStr(start, end))
			start, end = nums[i], nums[i]
		}
	}
	parts = append(parts, rangeStr(start, end))
	return strings.Join(parts, ",")
}

func rangeStr(start, end int) string {
	if start == end {
		return strconv.Itoa(start)
	}
	return strconv.Itoa(start) + "-" + strconv.Itoa(end)
}

// resolvePaper 解析单个 token 为论文；失败返回 (nil, nil)。
//
// 裸 token → 视为 citeKey 查库（先精确，未命中再 strings.ToUpper 一次）；
// 带前缀 → MetadataFetcher.Fetch 现场反查，成功后 UpsertPaper 入库并以返回的 citeKey 补回。
func resolvePaper(ctx context.Context, store PaperStore, fetcher Fetcher, token string) (*model.Paper, error) {
	if idType, ident, ok := splitPrefixed(token); ok {
		if ident == "" || fetcher == nil {
			return nil, nil
		}
		p, err := fetcher.Fetch(ctx, idType, ident)
		if err != nil || p == nil {
			return nil, nil
		}
		key, _, err := store.UpsertPaper(*p)
		if err != nil {
			return nil, nil
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

// citationMark 生成正文引用标记：GB7714/IEEE 为 "[n]"，APA 为 "(Author, Year)"，preview 为自描述标记。
func citationMark(p model.Paper, style Style, number int) string {
	switch style {
	case StylePreview:
		return previewMark(p)
	case StyleAPA:
		return apaInline(p)
	default:
		return "[" + strconv.Itoa(number) + "]"
	}
}

// previewMark 预览模式内联标记：有 DOI 用 [@doi:{DOI} — {标题}]，否则 [@标题]。
// 自描述便于人工核查；preview 模式不生成引用列表（WriteManuscriptOutputs）。
func previewMark(p model.Paper) string {
	title := cleanTitle(p.Title)
	if p.DOI != "" {
		return "[@doi:" + p.DOI + " — " + title + "]"
	}
	return "[@" + title + "]"
}

// apaInline 生成 APA 正文标记：
// 1 作者 → (Family, Year)；2 作者 → (A & B, Year)；≥3 → (A et al., Year)；
// year<=0 → n.d.；无作者 → (Title 首词, Year) 的简化形式。
func apaInline(p model.Paper) string {
	name := ""
	if len(p.Authors) > 0 {
		name = authorLastName(p.Authors[0])
		switch {
		case len(p.Authors) == 2:
			name += " & " + authorLastName(p.Authors[1])
		case len(p.Authors) > 2:
			name += " et al."
		}
	} else if words := strings.Fields(p.Title); len(words) > 0 {
		name = words[0]
	}
	year := "n.d."
	if p.Year > 0 {
		year = strconv.Itoa(p.Year)
	}
	return "(" + name + ", " + year + ")"
}

// authorLastName 取作者姓（Family），缺省回落 Given。
func authorLastName(a model.Author) string {
	if f := strings.TrimSpace(a.Family); f != "" {
		return f
	}
	return strings.TrimSpace(a.Given)
}

// 手稿产物逻辑名（map key，FR-REF-11）。实际落盘文件名为 {base}_{时间戳}.{ext}。
const (
	ManuscriptFormatted = "formatted.md" // 正文 + 文末参考文献列表
	ManuscriptBib       = "refs.bib"
	ManuscriptRIS       = "refs.ris"
	manuscriptFilePerm  = 0o600
)

// ManuscriptStamp 生成产物文件名时间戳（精确到秒，所有产物共用同一值）。
func ManuscriptStamp(t time.Time) string {
	return t.Format("20060102_150405")
}

// RenderReferenceList 渲染编号引用列表（formatted.md 文末 / MCP referenceList 共用）。
func RenderReferenceList(papers []model.Paper, style Style) (string, error) {
	var b strings.Builder
	for i, p := range papers {
		if i > 0 {
			b.WriteString("\n\n")
		}
		line, err := FormatReference(p, style, i+1)
		if err != nil {
			return "", err
		}
		b.WriteString(line)
	}
	return b.String(), nil
}

// formattedContent 构建最终文稿：正文 + 文末参考文献列表（FR-REF-09/11）。
// 无论何种样式（含 preview）均追加列表；无引用论文时仅返回正文。
func formattedContent(res *ManuscriptResult, style Style) (string, error) {
	if len(res.Papers) == 0 {
		return res.Text, nil
	}
	refs, err := RenderReferenceList(res.Papers, style)
	if err != nil {
		return "", err
	}
	return res.Text + "\n\n# 参考文献\n\n" + refs, nil
}

// WriteManuscriptOutputs 落盘 {base}_{ts}.md（正文+文末引用列表）/ {base}_{ts}.bib / {base}_{ts}.ris。
//
// 返回 逻辑名 → 绝对路径 映射（ManuscriptFormatted/ManuscriptBib/ManuscriptRIS）；
// 不含 docx（由调用方按需经 PandocToDocx 转换）。CLI 与 MCP 共用（FR-IFACE-03）。
func WriteManuscriptOutputs(outDir, base, ts string, res *ManuscriptResult, style Style) (map[string]string, error) {
	files := make(map[string]string)
	text, err := formattedContent(res, style)
	if err != nil {
		return nil, err
	}
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

// PandocToDocx 调用 pandoc 将 markdown 转为 docx（FR-REF-11）。
// 可执行文件固定为 pandoc，参数为本地文件路径；调用方负责 LookPath 预检与降级提示。
func PandocToDocx(src, dst string) error {
	// #nosec G204 -- 可执行文件固定为 pandoc，参数为本地文件路径，无 shell 注入面。
	out, err := exec.Command("pandoc", src, "-o", dst).CombinedOutput()
	if err != nil {
		if len(out) > 0 {
			return fmt.Errorf("%v: %s", err, strings.TrimSpace(string(out)))
		}
		return err
	}
	return nil
}
