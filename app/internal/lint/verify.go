package lint

// Options 验证选项。
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
		report.ExitHint = "pass"
	}
	report.Passed = report.ExitHint == "pass"
	// M 类规则（无法自动判定）固定输出人工核对提示。
	report.ManualChecklist = []string{
		"数据一致性：核对正文数据与表格/图片是否一致",
		"术语缩写：核对缩写首次出现是否给出全称",
	}
	return report, nil
}
