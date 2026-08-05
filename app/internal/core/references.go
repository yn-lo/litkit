// references.go 引用渲染：内置格式化器（GB/T 7714—2025 / APA 7th / IEEE）+ BibTeX/RIS 生成。
//
// 纯函数、零网络（FR-REF-03/04/05/06）：输入 model.Paper，输出格式化字符串。
// 三种核心样式原生实现；CSL/citeproc 仅用于原生未覆盖样式（FR-REF-07，二期）。

package core

import (
	"fmt"
	"strconv"
	"strings"

	"litkit/internal/model"
)

// Style 引用样式。
type Style string

const (
	// StyleGB7714 中文模式：GB/T 7714—2025（支持中英文文献混排著录，FR-REF-03）。
	StyleGB7714 Style = "gb7714-2025"
	// StyleAPA 英文模式：APA 7th（编号列表输出，FR-REF-04）。
	StyleAPA Style = "apa"
	// StyleIEEE 英文模式：IEEE（编号列表输出，FR-REF-04）。
	StyleIEEE Style = "ieee"
)

// docTypeMark 文献类型标志（GB/T 7714—2025 新文献类型，C3）。
// 未知类型回落通用 [Z]（其他）。键为 model.Paper.DocType 的值。
var docTypeMark = map[string]string{
	model.DocTypeArticle:        "[J]",
	model.DocTypeJournalArticle: "[J]",
	model.DocTypePreprint:       "[PP]",
	model.DocTypeDataset:        "[DS]",
	model.DocTypeBook:           "[M]",
	model.DocTypeMonograph:      "[M]",
	model.DocTypeConference:     "[C]",
	model.DocTypeProceedings:    "[C]",
	model.DocTypeReport:         "[R]",
	model.DocTypeThesis:         "[D]",
	model.DocTypePatent:         "[P]",
	model.DocTypeSoftware:       "[CP]",
	model.DocTypeWebpage:        "[EB/OL]",
}

// docTypeMarkGB 返回文献类型标志；空/未知回落 [Z]（其他）。
func docTypeMarkGB(p model.Paper) string {
	key := strings.ToLower(strings.TrimSpace(p.DocType))
	if m, ok := docTypeMark[key]; ok {
		return m
	}
	return "[Z]"
}

// FormatReference 按样式渲染单条参考文献（含编号前缀）。
//
//	GB/T 7714：  [1] 作者. 题名[J]. 刊名, 年, 卷(期): 页码. DOI:xxx.
//	APA：        Author, A. B., & Author, C. (2020). Title. Journal, 5(2), 12-34. https://doi.org/xxx
//	IEEE：       [1] A. Author, "Title," Journal, vol. 5, no. 2, pp. 12-34, 2020.
//
// 编号 number 仅用于 GB/T 7714 与 IEEE；APA 不输出编号（调用方自行决定列表格式）。
func FormatReference(p model.Paper, style Style, number int) (string, error) {
	switch style {
	case StyleGB7714:
		return formatGB7714(p, number), nil
	case StyleAPA:
		return formatAPA(p), nil
	case StyleIEEE:
		return formatIEEE(p, number), nil
	default:
		return "", fmt.Errorf("references: 未知样式 %q（支持 gb7714-2025|apa|ieee）", style)
	}
}

// formatGB7714 渲染 GB/T 7714—2025 条目。
//
// 中文文献作者用顿号分隔 + "等"；英文文献作者姓全大写在前、名缩写，
// 3 名以上省略为 "et al."（GB/T 7714 混排约定）。无作者时以题名起始。
// 按文献类型差异化格式：期刊[J]用刊名/卷期/页码；书籍[M]用出版地:出版社；
// 学位论文[D]用学校；网页[EB/OL]用URL+访问日期。
func formatGB7714(p model.Paper, number int) string {
	cjk := isPaperCJK(p)
	sep := gbSep(cjk)
	var b strings.Builder
	if number > 0 {
		b.WriteString("[" + strconv.Itoa(number) + "] ")
	}
	if a := authorsGB(p.Authors); a != "" {
		b.WriteString(a)
		b.WriteString(sep)
	}
	b.WriteString(cleanTitle(p.Title))
	b.WriteString(docTypeMarkGB(p))
	dt := strings.ToLower(strings.TrimSpace(p.DocType))
	switch dt {
	case model.DocTypeBook, model.DocTypeMonograph:
		fmtGBBook(&b, p, cjk, sep)
	case model.DocTypeThesis:
		fmtGBThesis(&b, p, cjk, sep)
	case model.DocTypeWebpage:
		fmtGBWebpage(&b, p, cjk, sep)
	default:
		fmtGBJournal(&b, p, cjk, sep)
	}
	return b.String()
}

func fmtGBJournal(b *strings.Builder, p model.Paper, cjk bool, sep string) {
	if p.Venue != "" {
		b.WriteString(sep)
		b.WriteString(p.Venue)
	}
	if p.Year > 0 {
		b.WriteString(", ")
		b.WriteString(strconv.Itoa(p.Year))
	}
	if vi := volIssue(p); vi != "" {
		b.WriteString(", ")
		b.WriteString(vi)
	}
	if p.Pages != "" {
		b.WriteString(": ")
		b.WriteString(normalizePages(p.Pages))
	}
	b.WriteString(gbEnd(cjk))
	if p.DOI != "" {
		b.WriteString(" DOI:")
		b.WriteString(p.DOI)
		b.WriteString(gbEnd(cjk))
	}
}

func fmtGBBook(b *strings.Builder, p model.Paper, cjk bool, sep string) {
	b.WriteString(sep)
	switch {
	case p.City != "" && p.Publisher != "":
		b.WriteString(p.City)
		b.WriteString(": ")
		b.WriteString(p.Publisher)
	case p.Publisher != "":
		b.WriteString(p.Publisher)
	case p.Venue != "":
		b.WriteString(p.Venue)
	}
	if p.Year > 0 {
		b.WriteString(", ")
		b.WriteString(strconv.Itoa(p.Year))
	}
	b.WriteString(gbEnd(cjk))
	if p.DOI != "" {
		b.WriteString(" DOI:")
		b.WriteString(p.DOI)
		b.WriteString(gbEnd(cjk))
	}
}

func fmtGBThesis(b *strings.Builder, p model.Paper, cjk bool, sep string) {
	b.WriteString(sep)
	if p.Institution != "" {
		b.WriteString(p.Institution)
	} else if p.Venue != "" {
		b.WriteString(p.Venue)
	}
	if p.Year > 0 {
		b.WriteString(", ")
		b.WriteString(strconv.Itoa(p.Year))
	}
	b.WriteString(gbEnd(cjk))
	if p.DOI != "" {
		b.WriteString(" DOI:")
		b.WriteString(p.DOI)
		b.WriteString(gbEnd(cjk))
	}
}

func fmtGBWebpage(b *strings.Builder, p model.Paper, cjk bool, sep string) {
	b.WriteString(sep)
	if p.Year > 0 {
		b.WriteString(strconv.Itoa(p.Year))
		b.WriteString(gbEnd(cjk))
	}
	if p.AccessDate != "" {
		b.WriteString("(")
		b.WriteString(p.AccessDate)
		b.WriteString(")")
	}
	if p.URL != "" {
		b.WriteString("[")
		b.WriteString(p.URL)
		b.WriteString("]")
	}
	b.WriteString(gbEnd(cjk))
	if p.DOI != "" {
		b.WriteString(" DOI:")
		b.WriteString(p.DOI)
		b.WriteString(gbEnd(cjk))
	}
}

// authorsGB 渲染作者（GB/T 7714 混排）：中文名原样顿号连接，
// 英文名 "FAMILY Given" 姓大写、名缩写；>3 名时前 3 名 + 等/et al（尾句点由调用方追加）。
func authorsGB(as []model.Author) string {
	if len(as) == 0 {
		return ""
	}
	const maxShow = 3
	parts := make([]string, 0, len(as))
	for _, a := range as {
		f := strings.TrimSpace(a.Family)
		g := strings.TrimSpace(a.Given)
		switch {
		case f != "" && g != "":
			// 判断是否含 CJK：含则保留原样（中文名），否则姓大写+名首字母缩写
			if containsCJK(f + g) {
				parts = append(parts, f+g)
			} else {
				parts = append(parts, strings.ToUpper(f)+" "+initials(g))
			}
		case f != "":
			parts = append(parts, f)
		case g != "":
			parts = append(parts, g)
		}
	}
	if len(parts) > maxShow {
		if containsCJK(parts[0]) {
			return strings.Join(parts[:maxShow], ", ") + ", 等"
		}
		return strings.Join(parts[:maxShow], ", ") + ", et al"
	}
	return strings.Join(parts, ", ")
}

// formatAPA 渲染 APA 7th 条目：Author, A. B., & Author, C. (Year). Title. *Journal*, *vol*(issue), pages. https://doi.org/xxx
func formatAPA(p model.Paper) string {
	var b strings.Builder
	if a := authorsAPA(p.Authors); a != "" {
		b.WriteString(a)
		b.WriteString(" ")
	}
	b.WriteString("(")
	if p.Year > 0 {
		b.WriteString(strconv.Itoa(p.Year))
	}
	b.WriteString("). ")
	b.WriteString(cleanTitle(p.Title))
	b.WriteString(".")
	if p.Venue != "" {
		b.WriteString(" ")
		b.WriteString(italic(p.Venue))
	}
	if volIssue := volIssueAPA(p); volIssue != "" {
		b.WriteString(", ")
		b.WriteString(volIssue)
	}
	if p.Pages != "" {
		b.WriteString(", ")
		b.WriteString(normalizePages(p.Pages))
	}
	b.WriteString(".")
	if p.DOI != "" {
		b.WriteString(" https://doi.org/")
		b.WriteString(p.DOI)
	}
	return b.String()
}

// authorsAPA 渲染作者（APA：Family, G.；最多只列前 3 位，>3 名省略为前 3 名 + et al.）。
func authorsAPA(as []model.Author) string {
	if len(as) == 0 {
		return ""
	}
	const maxShow = 3
	parts := make([]string, 0, len(as))
	for _, a := range as {
		parts = append(parts, authorAPA(a))
	}
	if len(parts) > maxShow {
		return strings.Join(parts[:maxShow], ", ") + ", et al."
	}
	if len(parts) == 1 {
		return parts[0]
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

// formatIEEE 渲染 IEEE 条目：[1] A. Author, "Title," *Journal*, vol. 5, no. 2, pp. 12-34, 2020.
func formatIEEE(p model.Paper, number int) string {
	var b strings.Builder
	if number > 0 {
		b.WriteString("[" + strconv.Itoa(number) + "] ")
	}
	if a := authorsIEEE(p.Authors); a != "" {
		b.WriteString(a)
		b.WriteString(", ")
	}
	b.WriteString("\"" + cleanTitle(p.Title) + ",\"")
	if p.Venue != "" {
		b.WriteString(" ")
		b.WriteString(italic(p.Venue))
	}
	if p.Volume != "" {
		b.WriteString(", vol. ")
		b.WriteString(p.Volume)
	}
	if p.Number != "" {
		b.WriteString(", no. ")
		b.WriteString(p.Number)
	}
	if p.Pages != "" {
		b.WriteString(", pp. ")
		b.WriteString(normalizePages(p.Pages))
	}
	if p.Year > 0 {
		b.WriteString(", ")
		b.WriteString(strconv.Itoa(p.Year))
	}
	b.WriteString(".")
	if p.DOI != "" {
		b.WriteString(" doi: ")
		b.WriteString(p.DOI)
		b.WriteString(".")
	}
	return b.String()
}

// authorsIEEE 渲染作者（IEEE：G. Family；最多只列前 3 位，>3 名省略为前 3 名 + et al.）。
func authorsIEEE(as []model.Author) string {
	if len(as) == 0 {
		return ""
	}
	const maxShow = 3
	parts := make([]string, 0, len(as))
	for _, a := range as {
		parts = append(parts, authorIEEE(a))
	}
	if len(parts) > maxShow {
		return strings.Join(parts[:maxShow], ", ") + ", et al."
	}
	if len(parts) == 1 {
		return parts[0]
	}
	return strings.Join(parts[:len(parts)-1], ", ") + ", and " + parts[len(parts)-1]
}

// authorIEEE 单作者（IEEE：G. Family）。
func authorIEEE(a model.Author) string {
	f := strings.TrimSpace(a.Family)
	g := strings.TrimSpace(a.Given)
	switch {
	case f != "" && g != "":
		return initials(g) + " " + f
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

// volIssue 渲染 卷(期) 片段。
func volIssue(p model.Paper) string {
	var b strings.Builder
	if p.Volume != "" {
		b.WriteString(p.Volume)
	}
	if p.Number != "" {
		b.WriteString("(")
		b.WriteString(p.Number)
		b.WriteString(")")
	}
	return b.String()
}

// volIssueAPA 渲染 APA 卷(期)片段：卷号斜体，期号不斜体。
func volIssueAPA(p model.Paper) string {
	if p.Volume == "" && p.Number == "" {
		return ""
	}
	var b strings.Builder
	if p.Volume != "" {
		b.WriteString(italic(p.Volume))
	}
	if p.Number != "" {
		b.WriteString("(")
		b.WriteString(p.Number)
		b.WriteString(")")
	}
	return b.String()
}

// cleanTitle 清理题名：折叠空白、去首尾空格。
func cleanTitle(t string) string {
	return strings.Join(strings.Fields(t), " ")
}

// containsCJK 判断字符串是否含中日韩统一表意文字。
func containsCJK(s string) bool {
	for _, r := range s {
		if r >= 0x4E00 && r <= 0x9FFF {
			return true
		}
	}
	return false
}

// isPaperCJK 判断论文是否为中文文献（首位作者含 CJK 或标题含 CJK）。
func isPaperCJK(p model.Paper) bool {
	if len(p.Authors) > 0 {
		a := p.Authors[0]
		if containsCJK(a.Family + a.Given) {
			return true
		}
	}
	return containsCJK(p.Title)
}

// gbSep 返回 GB/T 7714 分隔符：中文用 ．（全角句点），英文用 .（半角句点）。
func gbSep(cjk bool) string {
	if cjk {
		return "．"
	}
	return ". "
}

// gbEnd 返回 GB/T 7714 句末标点：中文用 ．，英文用 .。
func gbEnd(cjk bool) string {
	if cjk {
		return "．"
	}
	return "."
}

// normalizePages 归一化页码：将 -- 统一为 -，去除首尾空白。
func normalizePages(pages string) string {
	p := strings.TrimSpace(pages)
	p = strings.ReplaceAll(p, "--", "-")
	return p
}

// italic 包裹斜体标记（markdown *语法*，纯文本场景无影响）。
func italic(s string) string {
	if s == "" {
		return ""
	}
	return "*" + s + "*"
}

// bibEntryType 文档类型 → BibTeX 条目类型。
func bibEntryType(p model.Paper) string {
	switch strings.ToLower(strings.TrimSpace(p.DocType)) {
	case model.DocTypeArticle, model.DocTypeJournalArticle:
		return model.DocTypeArticle
	case model.DocTypeBook, model.DocTypeMonograph:
		return model.DocTypeBook
	case model.DocTypeConference, model.DocTypeProceedings:
		return "inproceedings"
	case model.DocTypeReport:
		return "techreport"
	case model.DocTypeThesis:
		return "phdthesis"
	case model.DocTypePreprint, model.DocTypeDataset, model.DocTypePatent, model.DocTypeSoftware:
		return "misc"
	default:
		return "misc"
	}
}

// bibKey 生成 BibTeX 引用键：优先 CiteKey；否则 首作者姓+年；再回落 title 首词+年。
func bibKey(p model.Paper) string {
	if k := strings.TrimSpace(p.CiteKey); k != "" {
		return k
	}
	var first string
	if len(p.Authors) > 0 {
		first = strings.TrimSpace(p.Authors[0].Family)
		if first == "" {
			first = strings.TrimSpace(p.Authors[0].Given)
		}
	}
	year := ""
	if p.Year > 0 {
		year = strconv.Itoa(p.Year)
	}
	if first == "" {
		words := strings.Fields(p.Title)
		if len(words) > 0 {
			first = sanitizeBib(strings.ToLower(words[0]))
		}
	}
	return sanitizeBib(first) + year
}

// sanitizeBib 清理 BibTeX 键/字段中的非法字符。
func sanitizeBib(s string) string {
	repl := strings.NewReplacer(
		"\\", "", "{", "", "}", "", "&", "", "%", "", "#", "",
		"_", "", "$", "", "~", "", "^", "", " ", "", "'", "", "\"", "",
		",", "", ".", "", ":", "", ";", "", "/", "", "(", "", ")", "", "-", "",
	)
	return repl.Replace(s)
}

// escapeBibText 转义 BibTeX 字段文本（保留大小写，防 TeX 特殊字符误解析）。
func escapeBibText(s string) string {
	repl := strings.NewReplacer(
		"\\", `\textbackslash{}`, "&", `\&`, "%", `\%`, "#", `\#`,
		"_", `\_`, "$", `\$`, "~", `\textasciitilde{}`, "^", `\textasciicircum{}`,
	)
	return repl.Replace(strings.Join(strings.Fields(s), " "))
}

// FormatBibTeX 渲染单条 BibTeX 条目（FR-REF-05）。
// 仅输出非空字段；作者格式 "Family, Given and Family, Given"。
func FormatBibTeX(p model.Paper) string {
	var b strings.Builder
	b.WriteString("@")
	b.WriteString(bibEntryType(p))
	b.WriteString("{")
	b.WriteString(bibKey(p))
	b.WriteString(",\n")
	writeBibField(&b, "author", bibAuthors(p.Authors))
	writeBibField(&b, "title", escapeBibText(p.Title))
	writeBibField(&b, "journal", escapeBibText(p.Venue))
	if p.Year > 0 {
		writeBibField(&b, "year", strconv.Itoa(p.Year))
	}
	writeBibField(&b, "volume", escapeBibText(p.Volume))
	writeBibField(&b, "number", escapeBibText(p.Number))
	writeBibField(&b, "pages", escapeBibText(p.Pages))
	writeBibField(&b, "doi", escapeBibText(p.DOI))
	writeBibField(&b, "url", escapeBibText(p.URL))
	writeBibField(&b, "publisher", escapeBibText(p.Publisher))
	writeBibField(&b, "address", escapeBibText(p.City))
	writeBibField(&b, "school", escapeBibText(p.Institution))
	if ab := strings.TrimSpace(p.Abstract); ab != "" {
		writeBibField(&b, "abstract", escapeBibText(ab))
	}
	b.WriteString("}\n")
	return b.String()
}

// BibTeXFromPapers 批量渲染 BibTeX 条目（换行分隔）。
func BibTeXFromPapers(ps []model.Paper) string {
	var b strings.Builder
	for i, p := range ps {
		if i > 0 {
			b.WriteString("\n")
		}
		b.WriteString(FormatBibTeX(p))
	}
	return b.String()
}

// bibAuthors 作者 → "Family, Given and Family, Given"。
func bibAuthors(as []model.Author) string {
	if len(as) == 0 {
		return ""
	}
	parts := make([]string, 0, len(as))
	for _, a := range as {
		f := strings.TrimSpace(a.Family)
		g := strings.TrimSpace(a.Given)
		switch {
		case f != "" && g != "":
			parts = append(parts, f+", "+g)
		case f != "":
			parts = append(parts, f)
		case g != "":
			parts = append(parts, g)
		}
	}
	return strings.Join(parts, " and ")
}

// writeBibField 仅在值非空时写出 "key = {value},"。
func writeBibField(b *strings.Builder, key, value string) {
	if value == "" {
		return
	}
	b.WriteString("  ")
	b.WriteString(key)
	b.WriteString(" = {")
	b.WriteString(value)
	b.WriteString("},\n")
}

// risType 文档类型 → RIS TY 代码。
func risType(p model.Paper) string {
	switch strings.ToLower(strings.TrimSpace(p.DocType)) {
	case model.DocTypeArticle, model.DocTypeJournalArticle:
		return "JOUR"
	case model.DocTypePreprint:
		return "PREP"
	case model.DocTypeDataset:
		return "DATA"
	case model.DocTypeBook, model.DocTypeMonograph:
		return "BOOK"
	case model.DocTypeConference, model.DocTypeProceedings:
		return "CONF"
	case model.DocTypeReport:
		return "RPRT"
	case model.DocTypeThesis:
		return "THES"
	case model.DocTypePatent:
		return "PAT"
	case model.DocTypeSoftware:
		return "COMP"
	default:
		return "GEN"
	}
}

// risPages 拆解页码：含 "-" 拆 SP/EP，其余全落 SP。
func risPages(pages string) (sp, ep string) {
	if pages == "" {
		return "", ""
	}
	if i := strings.Index(pages, "-"); i > 0 {
		return strings.TrimSpace(pages[:i]), strings.TrimSpace(pages[i+1:])
	}
	return strings.TrimSpace(pages), ""
}

// FormatRIS 渲染单条 RIS 条目（FR-REF-06，兼容 Zotero/Mendeley/EndNote）。
func FormatRIS(p model.Paper) string {
	var b strings.Builder
	b.WriteString("TY  - " + risType(p) + "\n")
	for _, a := range p.Authors {
		// 各工具通用：优先 "Family, Given"
		f := strings.TrimSpace(a.Family)
		g := strings.TrimSpace(a.Given)
		name := f
		if g != "" {
			if name != "" {
				name += ", " + g
			} else {
				name = g
			}
		}
		if name != "" {
			b.WriteString("AU  - " + name + "\n")
		}
	}
	if t := strings.TrimSpace(p.Title); t != "" {
		b.WriteString("TI  - " + t + "\n")
	}
	if ab := strings.TrimSpace(p.Abstract); ab != "" {
		b.WriteString("AB  - " + ab + "\n")
	}
	if v := strings.TrimSpace(p.Venue); v != "" {
		b.WriteString("JO  - " + v + "\n")
	}
	if p.Year > 0 {
		b.WriteString("PY  - " + strconv.Itoa(p.Year) + "\n")
	}
	if v := strings.TrimSpace(p.Volume); v != "" {
		b.WriteString("VL  - " + v + "\n")
	}
	if v := strings.TrimSpace(p.Number); v != "" {
		b.WriteString("IS  - " + v + "\n")
	}
	if sp, ep := risPages(p.Pages); sp != "" {
		b.WriteString("SP  - " + sp + "\n")
		if ep != "" {
			b.WriteString("EP  - " + ep + "\n")
		}
	}
	if d := strings.TrimSpace(p.DOI); d != "" {
		b.WriteString("DO  - " + d + "\n")
	}
	if u := strings.TrimSpace(p.URL); u != "" {
		b.WriteString("UR  - " + u + "\n")
	}
	if v := strings.TrimSpace(p.Publisher); v != "" {
		b.WriteString("PB  - " + v + "\n")
	}
	if v := strings.TrimSpace(p.City); v != "" {
		b.WriteString("CY  - " + v + "\n")
	}
	if v := strings.TrimSpace(p.Institution); v != "" {
		b.WriteString("AD  - " + v + "\n")
	}
	b.WriteString("ER  - \n")
	return b.String()
}

// RISFromPapers 批量渲染 RIS 条目（换行分隔）。
func RISFromPapers(ps []model.Paper) string {
	var b strings.Builder
	for i, p := range ps {
		if i > 0 {
			b.WriteString("\n")
		}
		b.WriteString(FormatRIS(p))
	}
	return b.String()
}

// ResolveStyle 由 lang/style 解析引用样式（CLI/MCP 共用，FR-IFACE-03）。
// 显式 style 优先（gb7714-2025|apa|ieee）；否则按 lang：zh→gb7714-2025、en→apa；无效返回错误。
func ResolveStyle(lang, style string) (Style, error) {
	if style != "" {
		s := Style(style)
		switch s {
		case StyleGB7714, StyleAPA, StyleIEEE:
			return s, nil
		}
		return "", fmt.Errorf("references: 未知引用样式 %q（支持 gb7714-2025|apa|ieee）", style)
	}
	switch lang {
	case "zh":
		return StyleGB7714, nil
	case "en":
		return StyleAPA, nil
	default:
		return "", fmt.Errorf("references: 未知语言模式 %q（支持 zh|en）", lang)
	}
}
