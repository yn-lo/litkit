package lint

import (
	"strings"
	"time"

	"litkit/internal/storage"
)

// exitPass 全部通过时的退出提示字面量。
const exitPass = "pass"

// Options 一次多文件验证的聚合配置。
type Options struct {
	Lang           string     // "zh" / "en"
	Mode           Mode       // chapter / draft / final
	PaperType      string     // review / empirical（空=不过滤类型）
	Only           []string   // --rule 仅运行指定规则（空=全部）
	Skip           []string   // --skip 跳过指定规则
	OnlyCategories []Category // --check 仅运行指定检查类别（空=全部）
	SkipCategories []Category // --skip-check 跳过指定检查类别
}

// Report 多文件验证汇总。
type Report struct {
	Files           []FileReport             `json:"files"`
	Passed          bool                     `json:"passed"`
	ExitHint        string                   `json:"exitHint"` // "pass" / "fix_and_rerun" / "manual_review"
	ManualChecklist []string                 `json:"manualChecklist,omitempty"`
	CitationRefs    *CitationRelevanceReport `json:"citationRefs,omitempty"` // 引用评分报告（可选）
	Recency         *RecencySummary          `json:"recency,omitempty"`      // 引用时效分档统计（R5.8）
}

// RecencySummary 被引文献时效分档统计（R5.8）。
//
// 均为"距今超过 N 年"计数，枚举全部成功解析出年份的被引文献（按 citeKey 去重）。
// Within5 + Over5 = Total；Over5 ⊇ Over10。
type RecencySummary struct {
	Total       int     `json:"total"`       // 已解析出年份的被引文献总数
	Within5     int     `json:"within5"`     // 距今 ≤ warn_age_years（默认 5）篇数
	Over5       int     `json:"over5"`       // 距今 > warn_age_years（默认 5）篇数
	Over10      int     `json:"over10"`      // 距今 > max_age_years（默认 10）篇数
	RecentRatio float64 `json:"recentRatio"` // Within5 / Total（近 5 年文献占比）
}

// CitationRelevanceReport 引用相关性评分汇总。
type CitationRelevanceReport struct {
	Enabled bool                    `json:"enabled"` // 是否启用 LLM 评分
	Models  []string                `json:"models,omitempty"`
	Results []CitationRelevanceItem `json:"results,omitempty"`
}

// CitationRelevanceItem 单条引用的相关性评分结果。
type CitationRelevanceItem struct {
	File         string  `json:"file"`
	Line         int     `json:"line"`
	CiteKey      string  `json:"citeKey"`
	Sentence     string  `json:"sentence"`
	MeanScore    float64 `json:"meanScore"`
	Consensus    float64 `json:"consensus"`
	Cached       bool    `json:"cached"`
	LowScore     bool    `json:"lowScore"`     // meanScore < 0.3}
	LowConsensus bool    `json:"lowConsensus"` // consensus < 0.5
}

// FileReport 单文件验证结果。
type FileReport struct {
	Path       string      `json:"path"`
	Violations []Violation `json:"violations"`
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

func categoryIn(list []Category, c Category) bool {
	for _, x := range list {
		if x == c {
			return true
		}
	}
	return false
}

func langOK(rule Rule, lang string) bool {
	for _, l := range rule.Langs {
		if l == lang {
			return true
		}
	}
	return false
}

// typeOK 规则是否适用于指定论文类型。
// Types 为空时适用全部类型；paperType 为空时不过滤（向后兼容）。
func typeOK(rule Rule, paperType string) bool {
	if len(rule.Types) == 0 || paperType == "" {
		return true
	}
	for _, t := range rule.Types {
		if t == paperType {
			return true
		}
	}
	return false
}

// Run 对单个 Source 执行验证（纯函数，无 IO）。
func Run(src *Source, spec *ManuscriptSpec, opts Options) FileReport {
	rep := FileReport{Path: src.Path, Violations: []Violation{}}
	for _, rule := range AllRules() {
		if !langOK(rule, opts.Lang) {
			continue
		}
		if !typeOK(rule, opts.PaperType) {
			continue
		}
		if modeRank(rule.From) > modeRank(opts.Mode) {
			continue
		}
		if len(opts.Only) > 0 && !contains(opts.Only, rule.ID) {
			continue
		}
		if contains(opts.Skip, rule.ID) {
			continue
		}
		if len(opts.OnlyCategories) > 0 && !categoryIn(opts.OnlyCategories, rule.Category) {
			continue
		}
		if categoryIn(opts.SkipCategories, rule.Category) {
			continue
		}
		rep.Violations = append(rep.Violations, rule.Check(src, spec)...)
	}
	return rep
}

// RunFiles 对多个文件路径执行验证。
func RunFiles(paths []string, spec *ManuscriptSpec, opts Options) (Report, error) {
	// PaperType 未显式指定时从 spec 取（三维过滤：lang × type × mode）
	if opts.PaperType == "" && spec != nil {
		opts.PaperType = spec.PaperType
	}
	report := Report{Files: []FileReport{}}
	method := map[string]Method{}
	for _, r := range AllRules() {
		method[r.ID] = r.Method
	}
	hasA, hasS := false, false
	for _, p := range paths {
		src, err := ParseSource(p)
		if err != nil {
			return report, err
		}
		fr := Run(src, spec, opts)
		for _, v := range fr.Violations {
			switch method[v.RuleID] {
			case MethodA:
				hasA = true
			case MethodS:
				hasS = true
			}
		}
		report.Files = append(report.Files, fr)
	}
	switch {
	case hasA:
		report.ExitHint = "fix_and_rerun"
	case hasS:
		report.ExitHint = "manual_review"
	default:
		report.ExitHint = exitPass
	}
	report.Passed = report.ExitHint == exitPass
	// M 类规则（无法自动判定）固定输出人工核对提示。
	report.ManualChecklist = []string{
		"数据一致性：核对正文数据与表格/图片是否一致",
		"术语缩写：核对缩写首次出现是否给出全称",
	}
	return report, nil
}

// CheckCiteKeys 校验文件正文中 [@citeKey] 是否存在于本地文献库（引用防伪，R5.6）。
//
// 纯函数 lint.Run 无 IO；此校验需查库，故独立于规则注册表，由入口层在 RunFiles 后调用。
// store 为 nil 时跳过（无库场景）。返回缺失 / 查询失败的违规。
func CheckCiteKeys(src *Source, store *storage.Store) []Violation {
	if store == nil {
		return nil
	}
	var out []Violation
	checked := map[string]bool{} // 去重：同一 citeKey 只报一次
	for i, ln := range src.Body {
		lineNo := src.bodyIdx[i]
		for _, m := range citeRe.FindAllStringSubmatch(ln, -1) {
			inner := strings.TrimSuffix(strings.TrimPrefix(m[0], "[@"), "]")
			for _, part := range strings.Split(inner, ",") {
				key := strings.TrimSpace(part)
				if key == "" || checked[key] {
					continue
				}
				checked[key] = true
				p, err := store.GetByCiteKey(key)
				if err != nil {
					out = append(out, Violation{
						RuleID:     ruleCiteExists,
						Line:       lineNo,
						Problem:    "引用的文献 " + key + " 查询失败",
						Suggestion: "检查本地文献库是否可用",
					})
					continue
				}
				if p == nil {
					out = append(out, Violation{
						RuleID:     ruleCiteExists,
						Line:       lineNo,
						Problem:    "引用的文献 " + key + " 不在本地文献库中",
						Suggestion: "用 litkit search 检索该文献并入库，或改用库中真实存在的 citeKey",
					})
				}
			}
		}
	}
	return out
}

// RunFilesWithStore 对文件执行规则验证 + 引用完整性附加校验。
//
// 在纯规则 RunFiles 之上叠加 R5.6 查库 + R5.7 撤稿（联网）+ R5.8 时效 + R5.9 自引；
// store 为 nil 时退化为纯规则验证，resolver 为 nil 时跳过撤稿。
// 追加违规后按 A/S 方法重算 exitHint（R5.6/R5.7 属 A 类，R5.8/R5.9 属 S 类）。
func RunFilesWithStore(paths []string, spec *ManuscriptSpec, opts Options, store *storage.Store, resolver RetractionResolver) (Report, error) {
	report, err := RunFiles(paths, spec, opts)
	if err != nil {
		return report, err
	}
	method := map[string]Method{}
	for _, r := range AllRules() {
		method[r.ID] = r.Method
	}
	// R5.6/R5.7/R5.8/R5.9（跨文件聚合，须拿到全部源码再统一查库）
	var srcs []*Source
	if store != nil {
		srcs = make([]*Source, 0, len(paths))
		for i := range report.Files {
			src, perr := ParseSource(report.Files[i].Path)
			if perr != nil {
				return report, perr
			}
			srcs = append(srcs, src)
			report.Files[i].Violations = append(report.Files[i].Violations, CheckCiteKeys(src, store)...)
		}
		// 撤稿/时效/自引均跨文件聚合：同一 citeKey 只在首次出现处报一条
		if resolver != nil {
			for _, h := range checkRetractions(srcs, store, resolver) {
				report.Files[h.file].Violations = append(report.Files[h.file].Violations, h.v)
			}
		}
		health, recency := checkCitationHealth(srcs, store, spec, time.Now().Year())
		for _, h := range health {
			report.Files[h.file].Violations = append(report.Files[h.file].Violations, h.v)
		}
		report.Recency = recency
	}
	hasA, hasS := recomputeExitHint(method, report,
		[]string{ruleCiteExists, ruleRetracted}, []string{ruleCurrency, ruleSelfCite})
	switch {
	case hasA:
		report.ExitHint = "fix_and_rerun"
	case hasS:
		report.ExitHint = "manual_review"
	default:
		report.ExitHint = exitPass
	}
	report.Passed = report.ExitHint == exitPass
	return report, nil
}

// recomputeExitHint 遍历报告违规，按方法分类重算 A/S 命中。
// notInRegistryA / notInRegistryS 为不在注册表中、需强制归类的规则 ID 白名单（如查库/联网类引用校验）。
// 其余按规则注册表里的 Method 判定；未注册且不在白名单的违规不参与 A/S 归类。
func recomputeExitHint(method map[string]Method, report Report, notInRegistryA, notInRegistryS []string) (hasA, hasS bool) {
	isIn := func(id string, ids []string) bool {
		for _, r := range ids {
			if r == id {
				return true
			}
		}
		return false
	}
	for i := range report.Files {
		for _, v := range report.Files[i].Violations {
			switch {
			case isIn(v.RuleID, notInRegistryA):
				hasA = true
			case isIn(v.RuleID, notInRegistryS):
				hasS = true
			case method[v.RuleID] == MethodA:
				hasA = true
			case method[v.RuleID] == MethodS:
				hasS = true
			}
		}
	}
	return
}
