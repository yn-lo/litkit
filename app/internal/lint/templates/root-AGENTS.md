# litkit — AI agent 使用说明

> litkit 工具使用地图，由 `litkit init` 生成。
> 撰写硬性规定见 `.litkit/<type-lang>/manuscript-spec.yaml`（AI 必读）。

## 核心命令

- `litkit search "<英文关键词>"` — 跨源检索并自动入库（默认精简视图：citeKey/title/firstAuthor/year/venue/docType/abstract）
  - `-s arxiv,pubmed` 源过滤；`-n 5` 每源条数；`--mode tiab|full` 检索等级
  - `--no-store` 探索模式：只返回不入库（无 citeKey），检索词试跑用
  - `--exclude TERM,...` 排除词；`--years 10` / `--since 2015` 放宽时间范围
  - 默认源由 `.env` 的 `LITKIT_SEARCH_SOURCES`（白名单）/ `LITKIT_SEARCH_EXCLUDE_SOURCES`（排除）收窄，`-s` 显式覆盖
  - 多词检索按词项 AND 组合（不必写成短语）；已写引号/布尔语法的检索原样下发
- `litkit sources` — 查看已注册源及各源 `enabled`（是否参与默认检索）
- `litkit metadata <doi|pmid|arxiv|title> <标识符>` — 按标识符取回单篇元数据（未命中输出 `null`）
- `litkit fetch <citeKey|doi>` — 按需取全文（Unpaywall OA → Sci-Hub 兜底，PDF 落盘 + 全文入库）
  - 仅在需要核对具体实验细节/原文数据时使用；默认摘要工作流无需全文
- `litkit lib get <citeKey> [citeKey ...]` — **按 citeKey 取完整元数据（含摘要）**，修订文稿时用这个，勿用 `lib list` 全量翻找
- `litkit lib add <metadata.json | --> [--doi|--pmid|--arxiv <标识符>]` — 入库检索源覆盖不到的文献（中文库文献、图书、网页）
  - 手动录入必填 `title` + `abstract`；`-` 表示从 stdin 读 JSON（免临时文件）
- `litkit lib list / search / rm / stats / path` — 文献库管理
- `litkit export [papers.json | -] [--keys K1,K2 | --all] [-f bibtex|ris|text]` — 导出引用
- `litkit verify manuscript/*.md --type <type> --lang <lang>` — 验证文稿格式
  - `--check citation,word_counts` 仅运行指定检查类别；`--skip-check boast_words` 跳过
  - 可用类别：language, structure, statistics, punctuation, style, citation, heading, boast_words, word_counts, todo
  - 每次运行自动覆盖写 `outputs/verify-report.md`（人类可读：字数/违规明细/下一步）；`--no-md` 关闭，`--md <path>` 改路径
- `litkit fix manuscript/*.md` — **自动修正确定性格式违规**（全半角/直引号/P 值/标题标点/引用位置等）
  - verify 报告里 `fixable: true` 的违规先跑 fix，再处理剩余；不要逐行手改标点
- `litkit rules` — 列出全部规则 ID（`--rule`/`--skip` 与 spec `skip_rules` 用）

## 论文类型

每种类型在 `.litkit/<type-lang>/` 下有独立阈值配置（`manuscript-spec.yaml`）。
查看已注册类型：`ls .litkit/`；追加：`litkit init --type review|empirical|qualitative|book|proposal --lang zh|en`
类型说明：review（综述）/ empirical（四段式实证）/ qualitative（质性研究，主题呈现）/ book（书籍专著）/ proposal（学术标书/课题申请书，中文）

## 检索策略

- **英文源用英文关键词**（arxiv/pubmed/openalex/semantic_scholar/biorxiv 均为英文语料）
- **中文文献**用 `-s crossref,doaj` 配中文关键词（可命中中文学报/中文 OA 期刊的标题与摘要）
- 默认 tiab=题目+摘要+关键词；结果不足时用 `--mode full` 全文检索（pubmed/arxiv/openalex 支持区分）
- 默认最近 3 年；可 `--years 10` 或 `--since 2015` 放宽
- `biorxiv`/`medrxiv` 无关键词检索端点（只能拉最近批次本地过滤），默认不参与检索，需 `-s` 显式指定

## 撰写注意事项

- 手稿文件（`manuscript/*.md`）**仅含正文**，不写摘要、关键词、参考文献列表
- 引用用 `[@<citeKey>]` 占位符，不展开元数据
- 引用模式看 spec 的 `citation_mode`：默认 inline（`[@citeKey]` 内联引用）；endnote（标书等）正文**禁止内联引用**，参考文献编号条目置于报告正文末尾（R10.3）
- 标书（proposal）封面字段（负责人、单位、电话等）写在首个正文章节标题之前，不参与字数与用词审查；占位符 `〔　〕` 会被 R10.4 提示
- 所有撰写硬性规定（字数、引用数、章节结构、标题层级、格式要求等）见 `.litkit/<type-lang>/manuscript-spec.yaml` 顶部注释

## 输出版本：默认草稿版

三种版本；**用户不明确要求其他版本时，一律输出草稿版**：

| 版本 | 何时用 | 产出 / 命令 |
|---|---|---|
| 草稿版（默认） | 所有撰写与修改 | 只写 `manuscript/*.md`，引用保持 `[@citeKey]`；**不运行 `litkit manuscript`**；verify 用 `--mode chapter|draft` |
| 校对版 | 用户明确要求核对引用/清样 | `litkit manuscript draft.md --preview` → `{base}_{时间戳}.preview.md`（自描述标记 + 文末核对清单），仅供人工核对，不投稿 |
| 投稿版 | 用户明确要求成稿/投稿/送审 | `litkit manuscript draft.md [-s gb7714-2025|apa|ieee|<.csl>] [--docx]` → `{base}_{时间戳}.md`（CSL 渲染正文 + 文末参考文献）+ `.bib` + `.ris`（+ `.docx`，需 Pandoc）；前置 `litkit verify --mode final` 全过 |

- 回复中必须说明本次生成的是哪一版。
- 产物默认落 `outputs/`（`WORK_DIR/outputs`），文件名带毫秒时间戳，不覆盖上一版。

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
   报告里 `fixable: true` 的违规先跑 `litkit fix manuscript/*.md` 自动修正，再处理剩余；
   `fixHint` 字段直接给出建议命令。
   引错用 `--report citation-refs` 检出（需配置 LLM，见 SKILL.md）

逐处检索会自动入库、累积弱相关文献，用 `-n 3` 控制条数以免污染文献库。
方法论详见 **academic-style** 技能；细节见 litkit 技能 `references/manuscript-writing.md`。

## 重要规则（AI 必读）

- **严禁编造文献**：所有引用必须来自文献库（`litkit lib list` 或 `litkit search` 获得），不得凭空生成 citeKey 或捏造 DOI/作者/标题
- **严禁捏造事实**：事实断言（数据、统计结果、他人结论）必须可溯源；确无支撑的以 `[待引证]` 占位并弱化表述，不得虚构
- **不写自辩与免责**：证据不足时弱化论断或标 `[待引证]`，不得用"需要指出的是 / 必须承认 / 本文无意 /
  当然，这并不意味着"这类辩护句补偿；局限只在讨论的局限段集中陈述一次
- **严禁篡改数据**：引用文献的结论、数字、统计量不得曲解或篡改以符合论点
- **分工不可混**：逻辑连接、跨文献综合、推论、局限分析由你写，以连贯通顺为首要，不需要引用；
  事实断言必须溯源。既不"为引而写"（叙事迁就文献），也不"为顺而编"（给逻辑层塞编造事实）
- **写后必验**：文稿完成后必须运行 `litkit verify manuscript/*.md --type <type> --lang <lang>`进行核查
