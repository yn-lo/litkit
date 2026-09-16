# litkit — AI agent 使用说明

> litkit 工具使用地图，由 `litkit init` 生成。
> 撰写硬性规定见 `.litkit/<type-lang>/manuscript-spec.yaml`（AI 必读）。

## 核心命令

- `litkit search "<英文关键词>"` — 跨源检索并自动入库
  - `-s arxiv,pubmed` 源过滤；`-n 5` 每源条数；`--mode tiab|full` 检索等级
- `litkit lib list / search / rm / stats / path` — 文献库管理
- `litkit verify manuscript/*.md --type <type> --lang <lang>` — 验证文稿格式
  - `--check citation,word_counts` 仅运行指定检查类别
  - `--skip-check boast_words` 跳过指定检查类别
  - 可用类别：language, structure, statistics, punctuation, style, citation, heading, boast_words, word_counts, todo

## 论文类型

每种类型在 `.litkit/<type-lang>/` 下有独立阈值配置（`manuscript-spec.yaml`）。
查看已注册类型：`ls .litkit/`；追加：`litkit init --type review|empirical|book|proposal --lang zh|en`
类型说明：review（综述）/ empirical（四段式实证）/ book（书籍专著）/ proposal（学术标书/课题申请书，中文）

## 检索策略

- 检索词**必须英文**（各源英文语料为主）
- 默认 tiab=题目+摘要+关键词；结果不足时用 `--mode full` 全文检索
- 默认最近 3 年；可 `--years 10` 或 `--since 2015` 放宽

## 撰写注意事项

- 手稿文件（`manuscript/*.md`）**仅含正文**，不写摘要、关键词、参考文献列表
- 引用用 `[@<citeKey>]` 占位符，不展开元数据
- 引用模式看 spec 的 `citation_mode`：默认 inline（`[@citeKey]` 内联引用）；endnote（标书等）正文**禁止内联引用**，参考文献编号条目置于报告正文末尾（R10.3）
- 标书（proposal）封面字段（负责人、单位、电话等）写在首个正文章节标题之前，不参与字数与用词审查；占位符 `〔　〕` 会被 R10.4 提示
- 所有撰写硬性规定（字数、引用数、章节结构、标题层级、格式要求等）见 `.litkit/<type-lang>/manuscript-spec.yaml` 顶部注释

## 撰写工作流（AI 必读）

**核心原则：分工，不是取舍。** 文献提供事实，作者提供逻辑与综合——两件事各有规则，互不竞争：

- **论证结构层**（叙事链、逻辑连接、跨文献综合、推论、局限分析）：由研究问题决定，
  以连贯、通顺、前后呼应为首要。**这层由你写，不需要引用，也不该为引用让路**
- **事实断言层**（数据、他人结论、具体发现）：**必须可溯源**——硬约束，不是"兼顾"

两种反模式都禁止："为引而写"（叙事迁就检索结果）与"为顺而编"（给逻辑层塞编造事实）。

1. **立意**：探索性检索确认研究缺口（开放式调研交 deep-research 技能），产出大纲 + 各节论断清单。
   **骨架定不下来，文献就会替你当组织者**
2. **撰写（边写边检索）**：按骨架逐节写，**摘要全程可用**。写到需要证据处当场检索（论断级英文词）：
   - 检索到且确实支撑 → 放 `[@citeKey]`
   - 只部分支撑或结论相反 → 弱化论断；**不得改论断去迁就文献**
   - 检索不到 → 写 `[待引证]` 占位
   - 逻辑连接与推论 → 直接写，不检索、不占位
   - 顺序不可反：**先有论断再检索**，不是检索到什么就写什么
3. **回填复核**：终稿前集中处理 `[待引证]`——补文献或弱化表述
4. **验证**：`litkit verify --mode final`（`[待引证]` 必须清零，R5.2）；
   引错用 `--report citation-refs` 检出（需配置 LLM，见 SKILL.md）

逐处检索会自动入库、累积弱相关文献，用 `-n 3` 控制条数以免污染文献库。
方法论详见 **academic-style** 技能；细节见 litkit 技能 `references/manuscript-writing.md`。

## 重要规则（AI 必读）

- **严禁编造文献**：所有引用必须来自文献库（`litkit lib list` 或 `litkit search` 获得），不得凭空生成 citeKey 或捏造 DOI/作者/标题
- **严禁捏造事实**：事实断言（数据、统计结果、他人结论）必须可溯源；确无支撑的以 `[待引证]` 占位并弱化表述，不得虚构
- **严禁篡改数据**：引用文献的结论、数字、统计量不得曲解或篡改以符合论点
- **分工不可混**：逻辑连接、跨文献综合、推论、局限分析由你写，以连贯通顺为首要，不需要引用；
  事实断言必须溯源。既不"为引而写"（叙事迁就文献），也不"为顺而编"（给逻辑层塞编造事实）
- **写后必验**：文稿完成后必须运行 `litkit verify manuscript/*.md --type <type> --lang <lang>`进行核查
