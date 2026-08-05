// agents.go AGENTS.md 渲染：固定头 + 撰写硬性规定（RenderWritingRules）+ 固定尾。
//
// AGENTS.md 由 `litkit init` / `litkit lint init` / MCP `lint_init` 生成，
// AI agent 在本工作目录工作时自动加载，是「事前指导」的唯一载体。

package lint

import (
	"fmt"
	"strings"
)

// agentsHead AGENTS.md 固定头部（配置/命令/检索策略）。
const agentsHead = "# litkit — AI agent 使用说明\n" +
	"\n" +
	"> 本文件由 `litkit init` 生成，AI 在本工作目录工作时自动加载。\n" +
	"\n" +
	"## 配置\n" +
	"\n" +
	"- 运行 litkit 前须设置 `LITKIT_WORK_DIR=<本目录>`（未设置时 init/search/lib 拒绝执行，FR-LIB-03）\n" +
	"- 配置文件：`.env`（本目录）；所有默认值可在其中调整\n" +
	"- 文献库：`litkit.db`（本目录）；删除工作目录即删除文献库（无 TTL）\n" +
	"- 撰写规范：`.litkit/`（rules.md 规则 / checklist.md 人工清单 / specs/manuscript-spec.yaml 阈值配置）\n" +
	"\n" +
	"## 核心命令\n" +
	"\n" +
	"- `litkit search \"<英文关键词>\"` 跨源检索（自动入库，输出精简视图）\n" +
	"  - `-s arxiv,pubmed` 源过滤；`-n 5` 每源条数\n" +
	"  - `--mode tiab|full` 检索等级（默认 tiab）\n" +
	"  - `--years N` / `--since YEAR` 时间范围\n" +
	"  - `--full` 完整元数据与错误\n" +
	"- `litkit sources` 查看可用源\n" +
	"- `litkit lib list / search / rm <citeKey> / stats / path` 文献库管理\n" +
	"- `litkit verify manuscript/*.md` 成稿后机械化验证（三要素：问题/修复/规则编号）\n" +
	"\n" +
	"## 检索策略\n" +
	"\n" +
	"- 检索词**必须英文**（各源英文语料为主，中文命中率极低，FR-SEARCH-11）\n" +
	"- 默认 tiab=题目+摘要+关键词；结果不足时用 `--mode full` 全文检索\n" +
	"- 默认最近 3 年；可 `--years 10` 或 `--since 2015` 放宽\n" +
	"- 单源失败不阻断整体：`errors` 字段说明原因，退出码 3 = 部分源成功\n"

// agentsTail AGENTS.md 固定尾部（验证与引用）。
const agentsTail = "## 验证与引用\n" +
	"\n" +
	"- 成稿后运行 `litkit verify manuscript/*.md`；人工复核 `.litkit/checklist.md`\n" +
	"- 引用时写 `[@<citeKey>]` 占位符，不展开元数据；manuscript 流水线按 citeKey\n" +
	"  生成 GB/T 7714 / APA / IEEE 规范引用\n"

// 固定高频硬性规定（zh/en 各一套，写死模板；阈值类从 spec 取可变值）。
var (
	zhFixedRules = []string{
		"全文中文撰写；英文仅限专业术语首次括注",
		"正文禁止：列表、加粗、回引（如\"见本文2.3节\"）、AI 痕迹词（\"首次证实\"\"颠覆性\"）",
		"P 值：≥0.01 保留 2 位（P=0.03）；0.001≤P<0.01 保留 3 位（P=0.006）；P<0.001 写 \"P<0.001\"",
		"统计量保留 2 位小数（χ²=68.40）；表格 % 提取到表头统一标注",
	}
	enFixedRules = []string{
		"Write in academic English; define abbreviations at first use",
		"No lists or bold in body text; no back-references or self-promotion",
		"Report statistics with consistent decimals; percentages rounded to 1 decimal",
	}
)

// RenderWritingRules 渲染 AGENTS.md 的「撰写硬性规定」段（事前指导）。
//
// 固定高频规则 + spec 可变阈值（字数/引用数/章节/标题）→ 精简祈使句。
// 不是 yaml 的翻译，而是 AI 写稿时可执行的硬性规定。
func RenderWritingRules(spec *ManuscriptSpec) string {
	rules := zhFixedRules
	if spec.Lang == LangEN {
		rules = enFixedRules
	}

	var b strings.Builder
	b.WriteString("## 撰写硬性规定（事前指导，源自 .litkit/specs/manuscript-spec.yaml）\n\n")
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
	return b.String()
}

// RenderAgentsMD 渲染完整 AGENTS.md（固定头 + 撰写硬性规定 + 固定尾）。
func RenderAgentsMD(spec *ManuscriptSpec) string {
	return agentsHead + "\n" + RenderWritingRules(spec) + "\n" + agentsTail
}
