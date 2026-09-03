// integrity.go 引用完整性附加校验（需查库 / 联网，独立于规则注册表）。
//
// 与 R5.6 引用存在性同一模式：纯函数 lint.Run 无 IO，查库/联网校验由入口层注入：
//   - R5.7 撤稿状态：联网查 Crossref update-to，判定被引文献是否已撤稿
//   - R5.8 引用时效：查库取文献年份，跨度过大提示（默认 10 年）
//   - R5.9 自引比例：查库取作者，超过上限提示（默认 15%）
//
// 网络失败静默跳过（不阻断验证）；无 DOI / 无文献年份 / 未配置自引作者时对应项跳过。

package lint

import (
	"context"
	"fmt"
	"strings"

	"litkit/internal/model"
	"litkit/internal/storage"
)

// RetractionStatus 文献撤稿状态（R5.7）。
type RetractionStatus struct {
	Retracted     bool   `json:"retracted"`
	RetractionDOI string `json:"retractionDoi,omitempty"` // 撤稿声明 DOI
	Source        string `json:"source,omitempty"`        // publisher | retraction-watch
	Label         string `json:"label,omitempty"`         // 声明类型（Retraction）
}

// RetractionResolver 解析文献 DOI 的撤稿状态（网络 IO，由入口层注入实现）。
type RetractionResolver interface {
	Resolve(ctx context.Context, doi string) (RetractionStatus, error)
}

// defaultMaxAgeYears 引用时效判定默认跨度（R5.8）。
const defaultMaxAgeYears = 10

// defaultSelfCiteRatio 自引比例默认上限（R5.9）。
const defaultSelfCiteRatio = 0.15

// checkRetractions 对全文（多文件）聚合校验正文引用的 DOI 是否已被撤稿（R5.7）。
//
// store 或 resolver 为 nil 时跳过。网络失败 / 无 DOI 静默跳过，不阻断验证。
// 撤稿属 A 类：命中即需修复（替换文献）。
// 与 R5.8/R5.9 同构：按 citeKey 全局去重，同一篇撤稿文献只在首次出现处报一条
// （避免多文件书稿中同一被撤稿文献被多处引用时重复报）。
func checkRetractions(srcs []*Source, store *storage.Store, resolver RetractionResolver) []healthViolation {
	if store == nil || resolver == nil {
		return nil
	}
	var out []healthViolation
	checked := map[string]bool{}
	for fi, src := range srcs {
		for i, ln := range src.Body {
			lineNo := src.bodyIdx[i]
			for _, m := range citeRe.FindAllString(ln, -1) {
				inner := strings.TrimSuffix(strings.TrimPrefix(m, "[@"), "]")
				for _, part := range strings.Split(inner, ",") {
					key := strings.TrimSpace(part)
					if key == "" || checked[key] {
						continue
					}
					checked[key] = true
					p, err := store.GetByCiteKey(key)
					if err != nil || p == nil || p.DOI == "" {
						continue // 缺失由 R5.6 覆盖；无 DOI 无法查撤稿
					}
					st, rerr := resolver.Resolve(context.Background(), p.DOI)
					if rerr != nil {
						continue // 网络失败不阻断验证，静默跳过
					}
					if st.Retracted {
						out = append(out, healthViolation{fi, Violation{
							RuleID:     ruleRetracted,
							Line:       lineNo,
							Problem:    "引用的文献 " + key + " 已被撤稿",
							Suggestion: "替换为未撤稿的文献（撤稿声明 DOI: " + st.RetractionDOI + "）",
						}})
					}
				}
			}
		}
	}
	return out
}

// citeOrigin 某 citeKey 首次出现位置（文件索引 + 行号）。
type citeOrigin struct {
	file int
	line int
}

// healthViolation 一条跨文件引用健康校验违规（记录归属文件）。
type healthViolation struct {
	file int
	v    Violation
}

// checkCitationHealth 对全文（多文件）聚合执行引用健康校验：
//   - R5.8 时效：被引文献年份早于 baselineYear - maxAge 违规
//   - R5.9 自引：spec 配置了作者时，自引比例超上限违规
//
// 返回跨文件违规列表；各违规归属首次引用所在文件。
func checkCitationHealth(srcs []*Source, store *storage.Store, spec *ManuscriptSpec, baselineYear int) []healthViolation {
	if store == nil || spec == nil {
		return nil
	}
	origin := map[string]citeOrigin{}
	for fi, src := range srcs {
		for i, ln := range src.Body {
			for _, m := range citeRe.FindAllString(ln, -1) {
				inner := strings.TrimSuffix(strings.TrimPrefix(m, "[@"), "]")
				for _, part := range strings.Split(inner, ",") {
					k := strings.TrimSpace(part)
					if k == "" {
						continue
					}
					if _, ok := origin[k]; !ok {
						origin[k] = citeOrigin{file: fi, line: src.bodyIdx[i]}
					}
				}
			}
		}
	}
	if len(origin) == 0 {
		return nil
	}

	var out []healthViolation
	cutoff := baselineYear - spec.Citation.MaxAge()
	selfHits := 0
	for k, o := range origin {
		p, err := store.GetByCiteKey(k)
		if err != nil || p == nil {
			continue
		}
		if p.Year > 0 && p.Year < cutoff {
			out = append(out, healthViolation{o.file, Violation{
				RuleID:     ruleCurrency,
				Line:       o.line,
				Problem:    fmt.Sprintf("引用的文献 %s（%d 年）距今已超过 %d 年", k, p.Year, spec.Citation.MaxAge()),
				Suggestion: "核查是否有更新的权威文献可替换",
			}})
		}
		if spec.Citation.HasSelfCite() && isSelfCited(p, spec.Citation.SelfCitationAuthors) {
			selfHits++
		}
	}
	if spec.Citation.HasSelfCite() && len(origin) > 0 {
		ratio := float64(selfHits) / float64(len(origin))
		if ratio > spec.Citation.SelfCiteRatio() {
			out = append(out, healthViolation{0, Violation{
				RuleID:     ruleSelfCite,
				Line:       1,
				Problem:    fmt.Sprintf("自引比例 %.0f%%（%d/%d 篇）超过上限 %.0f%%", ratio*percent, selfHits, len(origin), spec.Citation.SelfCiteRatio()*percent),
				Suggestion: "减少自身文献引用，补充独立来源",
			}})
		}
	}
	return out
}

// isSelfCited 判断论文是否含指定作者（作者本人署名匹配任意 Family）。
func isSelfCited(p *model.Paper, ownAuthors []string) bool {
	for _, a := range p.Authors {
		for _, own := range ownAuthors {
			if strings.TrimSpace(a.Family) != "" && strings.EqualFold(a.Family, own) {
				return true
			}
		}
	}
	return false
}
