package lint

// Options 验证选项。
type Options struct {
	Lang string   // "zh" / "en"
	Mode Mode     // chapter / draft / final
	Only []string // --rule 仅运行指定规则（空=全部）
	Skip []string // --skip 跳过指定规则
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

// Run 对单个 Source 执行验证（纯函数，无 IO）。
func Run(src *Source, spec *ManuscriptSpec, opts Options) FileReport {
	rep := FileReport{Path: src.Path, Violations: []Violation{}}
	for _, rule := range AllRules() {
		if !langOK(rule, opts.Lang) {
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
		"R2.4 数据一致性：核对正文数据与表格/图片是否一致",
		"R4.3 术语缩写：核对缩写首次出现是否给出全称",
	}
	return report, nil
}
