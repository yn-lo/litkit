// agents.go AGENTS.md 渲染。
//
// 根 AGENTS.md（项目级通用信息）由 InitProjectInfra 生成。
// 类型 AGENTS.md（撰写硬性规定）由 InitPaperType 生成，采用方案 C：
// 自动区（从 yaml 渲染，--refresh 覆盖）+ 人类追加区（自由编辑，不检查）。

package lint

import (
	"fmt"
	"strings"
)

// RootAgentsMD 根 AGENTS.md 内容（项目级通用信息，精炼地图文件）。
// 由 `init` 首次创建，writeIfAbsent 不覆盖用户修改。
// 引导 AI 去 .litkit/<type>/ 查看详细撰写规定。
func RootAgentsMD() string {
	var b strings.Builder
	b.WriteString("# litkit — AI agent 使用说明\n\n")
	b.WriteString("> 本文件是项目地图，由 `litkit init` 生成，**不会覆盖用户手动修改**。\n")
	b.WriteString("> 详细撰写规定请查看 `.litkit/<type-lang>/AGENTS.md`。\n\n")

	b.WriteString("## 配置\n\n")
	b.WriteString("- 工作目录：`LITKIT_WORK_DIR=<本目录>`\n")
	b.WriteString("- 配置文件：`.env`（本目录）\n")
	b.WriteString("- 文献库：`litkit.db`（本目录）\n\n")

	b.WriteString("## 核心命令\n\n")
	b.WriteString("- `litkit search \"<英文关键词>\"` 跨源检索（自动入库）\n")
	b.WriteString("  - `-s arxiv,pubmed` 源过滤；`-n 5` 每源条数\n")
	b.WriteString("  - `--mode tiab|full` 检索等级；`--years N` 时间范围\n")
	b.WriteString("  - `--full` 完整元数据与错误\n")
	b.WriteString("- `litkit sources` 查看可用源\n")
	b.WriteString("- `litkit lib list / search / rm <citeKey> / stats / path` 文献库管理\n")
	b.WriteString("- `litkit verify manuscript/*.md --type <type> --lang <lang>` 验证文稿\n\n")

	b.WriteString("## 论文类型与撰写规定\n\n")
	b.WriteString("每种论文类型有独立的撰写规定，位于 `.litkit/<type-lang>/` 目录下：\n")
	b.WriteString("- `AGENTS.md` — 撰写硬性规定（AI 每次写作前必读）\n")
	b.WriteString("- `manuscript-spec.yaml` — 阈值配置（字数/引用数/章节等）\n\n")
	b.WriteString("查看已注册类型：`ls .litkit/`\n")
	b.WriteString("追加类型：`litkit init --type review|empirical --lang zh|en`\n\n")

	b.WriteString("## 检索策略\n\n")
	b.WriteString("- 检索词**必须英文**（各源英文语料为主，中文命中率极低）\n")
	b.WriteString("- 默认 tiab=题目+摘要+关键词；结果不足时用 `--mode full` 全文检索\n")
	b.WriteString("- 默认最近 3 年；可 `--years 10` 或 `--since 2015` 放宽\n")
	b.WriteString("- 单源失败不阻断整体：`errors` 字段说明原因，退出码 3 = 部分源成功\n\n")

	b.WriteString("## 验证与引用\n\n")
	b.WriteString("- 成稿后运行 `litkit verify` 检查\n")
	b.WriteString("- 引用用 `[@<citeKey>]` 占位符，不展开元数据\n\n")

	b.WriteString("## 撰写注意事项\n\n")
	b.WriteString("- 手稿文件（`manuscript/*.md`）**仅含正文**，不写摘要、关键词、参考文献列表\n")
	b.WriteString("- 摘要和参考文献由 litkit 流水线自动生成，写入手稿会干扰 lint 实体检查\n")
	return b.String()
}

// RenderWritingRules 渲染类型 AGENTS.md 的「撰写硬性规定」段（自动区）。
//
// 固定高频规则 + spec 可变阈值（字数/引用数/章节/标题）→ 精简祈使句。
// 自动区由 <!-- lint:auto-start --> 和 <!-- lint:auto-end --> 包裹。
func RenderWritingRules(spec *ManuscriptSpec) string {
	zhFixedRules := []string{
		"全文中文撰写；英文仅限专业术语首次括注",
		"正文禁止：列表、加粗、回引（如\"见本文2.3节\"）、AI 痕迹词（\"首次证实\"\"颠覆性\"）",
		"P 值：≥0.01 保留 2 位（P=0.03）；0.001≤P<0.01 保留 3 位（P=0.006）；P<0.001 写 \"P<0.001\"",
		"统计量保留 2 位小数（χ²=68.40）；表格 % 提取到表头统一标注",
	}
	enFixedRules := []string{
		"Write in academic English; define abbreviations at first use",
		"No lists or bold in body text; no back-references or self-promotion",
		"Report statistics with consistent decimals; percentages rounded to 1 decimal",
	}

	rules := zhFixedRules
	if spec.Lang == LangEN {
		rules = enFixedRules
	}

	var b strings.Builder
	b.WriteString("<!-- lint:auto-start -->\n")
	b.WriteString("> **自动生成，请勿手动编辑**\n")
	b.WriteString("> 修改阈值请编辑 `.litkit/<type-lang>/manuscript-spec.yaml`，然后运行\n")
	b.WriteString("> `litkit init --refresh --type <type> --lang <lang>` 重新生成本段。\n\n")
	b.WriteString("## 撰写硬性规定（事前指导，源自 manuscript-spec.yaml）\n\n")
	// 论文类型与目标期刊
	typeLabel := "四段式实证"
	if spec.PaperType == PaperTypeReview {
		typeLabel = "综述"
	}
	fmt.Fprintf(&b, "- 论文类型：%s（%s）\n", typeLabel, spec.PaperType)
	if spec.Journal != "" {
		fmt.Fprintf(&b, "- 目标期刊：%s\n", spec.Journal)
	}
	for _, r := range rules {
		b.WriteString("- " + r + "\n")
	}
	fmt.Fprintf(&b, "- 章节结构（%s）：%s\n",
		spec.PaperType, strings.Join(spec.SectionList(), " → "))
	fmt.Fprintf(&b, "- 标题层级 ≤%d 级，标题 ≤%d 字，末尾无标点\n",
		spec.Heading.MaxLevel, spec.Heading.MaxLength)
	fmt.Fprintf(&b, "- 引用：全文 %d-%d 篇，用 [@citeKey] 占位符（%s），不展开元数据\n",
		spec.Citation.Count[0], spec.Citation.Count[1], spec.StyleLabel())
	fmt.Fprintf(&b, "- 字数：全文 %d-%d；摘要 %d-%d；段落 %d-%d\n",
		spec.WordCount.Total[0], spec.WordCount.Total[1],
		spec.WordCount.Abstract[0], spec.WordCount.Abstract[1],
		spec.WordCount.Paragraph[0], spec.WordCount.Paragraph[1])
	b.WriteString("<!-- lint:auto-end -->\n\n")
	return b.String()
}

// TypeAgentsMD 渲染类型论文的完整 AGENTS.md（自动区 + 追加区）。
func TypeAgentsMD(spec *ManuscriptSpec) string {
	return RenderWritingRules(spec) +
		"## 附加撰写要求（人类自由编辑，verify 不检查此段）\n\n" +
		"- \n"
}

// RenderAgentsMD 保留向后兼容：渲染完整 AGENTS.md（根级 + 撰写规定）。
// 新代码应使用 RootAgentsMD + TypeAgentsMD。
func RenderAgentsMD(spec *ManuscriptSpec) string {
	return RootAgentsMD() + "\n" + RenderWritingRules(spec) + "\n"
}
