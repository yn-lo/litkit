// references.go 引用渲染入口：样式解析 + CSL 渲染 + BibTeX/RIS 交换格式。
//
// 所有正式引用样式由 CSL 文件驱动（内置 go:embed，或用户 .csl）；
// preview/草稿视图是独立的 Go 实现，见 manuscript.go。

package core

import (
	"strconv"
	"strings"

	"litkit/internal/csl"
	"litkit/internal/model"
)

// ResolveStyle 由 lang/style 解析引用样式；stylesDir 为用户样式目录（可为空）。
func ResolveStyle(lang, ref, stylesDir string) (*csl.Style, error) {
	return csl.Resolve(lang, ref, stylesDir)
}

// RenderReferenceList 渲染文末参考文献列表（CSL 引擎）。
func RenderReferenceList(papers []model.Paper, style *csl.Style) (string, error) {
	entries, err := csl.RenderBibliography(papers, style)
	if err != nil {
		return "", err
	}
	return strings.Join(entries, "\n\n"), nil
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
		return sanitizeBib(k)
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
		"\r", "", "\n", "", "\t", "",
	)
	return repl.Replace(s)
}

// risField 把字段折叠为单行：RIS 是逐行记录格式，字段内的 CR/LF 会伪造新字段甚至提前终止条目。
func risField(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

// escapeBibText 转义 BibTeX 字段文本（保留大小写，防 TeX 特殊字符误解析）。
func escapeBibText(s string) string {
	repl := strings.NewReplacer(
		"\\", `\textbackslash{}`, "&", `\&`, "%", `\%`, "#", `\#`,
		"_", `\_`, "$", `\$`, "~", `\textasciitilde{}`, "^", `\textasciicircum{}`,
	)
	s = repl.Replace(strings.Join(strings.Fields(s), " "))
	// BibTeX 连反斜杠转义的花括号也计入结构；保留成对的大小写保护组，
	// 仅把未配对括号改为不含裸括号的 TeX 命令。
	paired := make([]bool, len(s))
	var opens []int
	for i := range len(s) {
		switch s[i] {
		case '{':
			opens = append(opens, i)
		case '}':
			if len(opens) > 0 {
				j := opens[len(opens)-1]
				opens = opens[:len(opens)-1]
				paired[i], paired[j] = true, true
			}
		}
	}
	var b strings.Builder
	for i := range len(s) {
		switch {
		case s[i] == '{' && !paired[i]:
			b.WriteString(`\textbraceleft{}`)
		case s[i] == '}' && !paired[i]:
			b.WriteString(`\textbraceright{}`)
		default:
			b.WriteByte(s[i])
		}
	}
	return b.String()
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
	writeBibField(&b, "author", escapeBibText(bibAuthors(p.Authors)))
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
			b.WriteString("AU  - " + risField(name) + "\n")
		}
	}
	if t := risField(p.Title); t != "" {
		b.WriteString("TI  - " + t + "\n")
	}
	if ab := risField(p.Abstract); ab != "" {
		b.WriteString("AB  - " + ab + "\n")
	}
	if v := risField(p.Venue); v != "" {
		b.WriteString("JO  - " + v + "\n")
	}
	if p.Year > 0 {
		b.WriteString("PY  - " + strconv.Itoa(p.Year) + "\n")
	}
	if v := risField(p.Volume); v != "" {
		b.WriteString("VL  - " + v + "\n")
	}
	if v := risField(p.Number); v != "" {
		b.WriteString("IS  - " + v + "\n")
	}
	if sp, ep := risPages(p.Pages); sp != "" {
		b.WriteString("SP  - " + risField(sp) + "\n")
		if ep != "" {
			b.WriteString("EP  - " + risField(ep) + "\n")
		}
	}
	if d := risField(p.DOI); d != "" {
		b.WriteString("DO  - " + d + "\n")
	}
	if u := risField(p.URL); u != "" {
		b.WriteString("UR  - " + u + "\n")
	}
	if v := risField(p.Publisher); v != "" {
		b.WriteString("PB  - " + v + "\n")
	}
	if v := risField(p.City); v != "" {
		b.WriteString("CY  - " + v + "\n")
	}
	if v := risField(p.Institution); v != "" {
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
