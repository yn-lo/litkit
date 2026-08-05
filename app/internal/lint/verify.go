package lint

import (
	"strings"

	"litkit/internal/storage"
)

// exitPass 全部通过时的退出提示字面量。
const exitPass = "pass"

// Options 一次多文件验证的聚合配置。
type Options struct {
	Lang      string   // "zh" / "en"
	Mode      Mode     // chapter / draft / final
	PaperType string   // review / empirical（空=不过滤类型）
	Only      []string // --rule 仅运行指定规则（空=全部）
	Skip      []string // --skip 跳过指定规则
}

// Report 多文件验证汇总。
type Report struct {
	Files           []FileReport `json:"files"`
	Passed          bool         `json:"passed"`
	ExitHint        string       `json:"exitHint"` // "pass" / "fix_and_rerun" / "manual_review"
	ManualChecklist []string     `json:"manualChecklist,omitempty"`
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

// RunFilesWithStore 对文件执行规则验证 + citeKey 存在性校验（引用防伪）。
//
// 在纯规则 RunFiles 之上叠加 R5.6 查库校验；store 为 nil 时退化为纯规则验证。
// 追加违规后按 A/S 方法重算 exitHint（R5.6 属 A 类：命中即 fix_and_rerun）。
func RunFilesWithStore(paths []string, spec *ManuscriptSpec, opts Options, store *storage.Store) (Report, error) {
	report, err := RunFiles(paths, spec, opts)
	if err != nil {
		return report, err
	}
	if store == nil {
		return report, nil
	}
	method := map[string]Method{}
	for _, r := range AllRules() {
		method[r.ID] = r.Method
	}
	hasA, hasS := report.ExitHint == "fix_and_rerun", report.ExitHint == "manual_review"
	for i := range report.Files {
		src, perr := ParseSource(report.Files[i].Path)
		if perr != nil {
			continue
		}
		for _, v := range CheckCiteKeys(src, store) {
			report.Files[i].Violations = append(report.Files[i].Violations, v)
			// R5.6 属 A 类（命中即需修复）；其余按规则注册表方法判定
			if v.RuleID == ruleCiteExists {
				hasA = true
				continue
			}
			switch method[v.RuleID] {
			case MethodA:
				hasA = true
			case MethodS:
				hasS = true
			}
		}
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
	return report, nil
}
