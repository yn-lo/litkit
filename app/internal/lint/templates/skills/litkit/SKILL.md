---
name: litkit
description: 使用 litkit 工具包完成学术写作全流程：跨源检索文献、规范引用、撰写手稿、合规验证。触发词（中/英）：检索/查文献/参考文献/引用/papers/literature/search/reference/citation/引用管理/manuscript/文稿/学术写作/论文/verify/验证/查重/撤稿/文献时效。当用户需要检索论文、管理文献库、撰写学术论文、验证文稿格式或检查引用合规时使用此技能。
---
# litkit

学术写作工具包：检索 → 入库 → 引用 → 撰写 → 验证。

## When to Use

- 用户需要检索学术文献（search、查文献、找参考文献）
- 用户需要管理文献库（查看、搜索、删除）
- 用户需要撰写学术论文并引用文献
- 用户需要验证文稿是否符合规范（verify、检查引用/撤稿/字数）
- 用户询问字数、引用格式、章节结构、自引、引用时效等写作规范

## When NOT to Use

若用户意图不在 litkit 能力范围内，先路由到对应技能，不要自行调用 litkit CLI：

| 用户想要的                    | 去向                                                                                  |
| ----------------------------- | ------------------------------------------------------------------------------------- |
| 撰写完整论文/多轮 AI 写作助理 | **academic-paper** 技能（litkit 仅提供检索/引用/合规工具，不替代写作 agent）    |
| 同行评审、审稿意见            | **academic-paper-reviewer** 技能                                                |
| 开放式主题调研、问题探索      | **deep-research** 技能（litkit 的 `search` 只做具体文献检索，不做探索式归纳） |
| 端到端论文流水线编排          | **academic-pipeline** 技能                                                      |
| dedupe/查重引擎原理           | **dedup-engine**（litkit 不提供查重）                                           |

只有当用户落地在使用 litkit CLI 写作/检索/验证，或明确要求调用 litkit 时才执行本技能。

## Instructions

### 文献检索

详见 `references/literature-search.md`。

```bash
litkit search "<英文关键词>" -s arxiv,pubmed -n 5
```

- 检索词**必须英文**
- 默认 tiab（题目+摘要+关键词）；不足时 `--mode full`
- 默认最近 3 年；可 `--years 10` 或 `--since 2015`

### 文献库管理

```bash
litkit lib list              # 列出全部文献
litkit lib search "<关键词>"  # 按标题/作者搜索
litkit lib stats             # 库统计
litkit lib rm <citeKey>      # 删除
```

### 论文撰写

详见 `references/manuscript-writing.md`。

- 手稿文件（`manuscript/*.md`）**仅含正文**，不写摘要、关键词、参考文献列表
- 引用用 `[@<citeKey>]` 占位符，不展开元数据
- 撰写硬性规定见 `.litkit/<type-lang>/manuscript-spec.yaml`
- **撰写工作流**（立意 → 撰写/边写边检索 → 回填复核 → 验证）见 `references/manuscript-writing.md`；
  核心原则：文献提供事实、作者提供逻辑与综合；**事实断言必须可溯源**，缺支撑时以 `[待引证]` 占位
  （逻辑连接与推论不需要引用，也不标占位）
- **正文写作方法**（结构、写作质量自省、摘要规范、修辞）见 **academic-style** 技能
- **叙事格式（八股骨架）**：按论文类型规划章节结构，写作前读 **manuscript-format** 技能（references/ 下按类型取文档）

### 验证文稿

```bash
# 全量验证
litkit verify manuscript/*.md --type <type> --lang <lang>

# 按类别筛选
litkit verify manuscript/*.md --type <type> --lang <lang> --check citation
litkit verify manuscript/*.md --type <type> --lang <lang> --check citation,word_counts
litkit verify manuscript/*.md --type <type> --lang <lang> --skip-check boast_words
```

**检查类别**：`language` `structure` `heading` `statistics` `punctuation` `style` `citation` `boast_words` `word_counts` `todo`

**验证模式**（递增）：`chapter`（结构）→ `draft`（+数据/标点/引用）→ `final`（+字数/行文）

**引用标记规则**：

- R5.2 待引证占位符 `[待引证]`：chapter/draft 为**合法中间态**（记录"证据不足待补文献"），
  final 模式必须清零（A 类）。**只标事实性断言**——逻辑过渡句不需要引用，标了是噪声
- R5.4 待办标记 `[TODO]`/`[TBD]`：draft 起即违规（A 类），任何阶段都应清除
- R6.1 引用位置：`[@citeKey]` 须置于句末标点**之前**（`结论成立[@key2021]。`）

**引用健康检查**（final 模式，需本地库 `--type/--lang` 生成 `.litkit`）：

- R5.6 引用防伪：`[@citeKey]` 必须存在于本地库（A 类，退出码 1）；R5.6–R5.9 遵循 verify 的 `--rule`/`--skip` 筛选（如 `--rule R3.1` 局部验证时不查库）
- R5.7 撤稿：联网查 Crossref，被引文献已撤稿即报（A 类）
- R5.8 引用时效：距今超 `max_age_years`（默认 10）逐篇提示；距今超 `warn_age_years`（默认 5）作两档计数汇总（`recency` 字段：>5 年 N 篇 / >10 年 K 篇 / 近 5 年占比）；均 S 类；**时效仅报告，非建议替换**——经典或奠基性文献可按需保留，勿因年代久远一律找替代文献
- R5.9 自引比例：配置 `self_citation_authors` 后可启用，比例超 `self_citation_max_ratio`（默认 0.15）提示（S 类）

**引用相关性评分**（`--report citation-refs`，抓"引错"）：

```bash
litkit verify manuscript/*.md --type <type> --lang <lang> --mode final --report citation-refs
```

- 作用：逐句评"引用句 ↔ 该文献摘要"的**吻合度**（0~1），能抓结论不符、数字编造、过度归因
- 前置：凭据推荐直接写 `.litkit/verifier_models.json` 每模型 `api_key`/`base_url`（该副本勿提交 git，运行时有明文提示），
  也可走 env 回落：全局 `LITKIT_LLM_API_KEY`/`LITKIT_LLM_BASE_URL` 或按模型 `LITKIT_LLM_API_KEY_<模型ID大写>`（如 `deepseek-chat` → `LITKIT_LLM_API_KEY_DEEPSEEK_CHAT`）；优先级 JSON > 按模型 env > 全局 env；
  启用条件：`models[].enabled=true` 且有 `api_key` 即自动生效（无总开关，全禁用 = 不发远程调用）；
  `.litkit/verifier_models.json` **至少启用 2 个模型**（`enabled: true`，多模型共识 `min_models: 2`），每模型可调 `temperature`/`max_tokens`/`extra_body`（透传请求体，如 `enable_thinking`/`reasoning_effort`）
- 默认全禁用（避免意外远程调用）；未配置时静默跳过，不报错。被引文献须有摘要才参与评分
- **边界**：锚点驱动，只评**已含 `[@citeKey]` 的句子**——检不出"该引而没引"；
  输入是摘要而非全文，仅存于全文的结论会被判低分
- **不影响退出码**：结果在 `citationRefs` 字段，供人工复核，不参与 fail 判定

**退出码**：0=通过或仅需人工复核；1=有 A 类违规需修复。

### 论文类型管理

```bash
ls .litkit/                                    # 查看已注册类型
litkit init --type review|empirical|book --lang zh|en  # 追加类型
```

## Critical Rules

- **严禁编造文献**：所有引用必须来自文献库，不得凭空生成 citeKey 或捏造 DOI/作者/标题
- **严禁捏造事实**：事实断言（数据、统计结果、他人结论）必须可溯源；确无支撑的以 `[待引证]` 占位并弱化表述，不得虚构
- **严禁篡改数据**：引用文献的结论、数字、统计量不得曲解或篡改
- **分工不可混**：逻辑连接、跨文献综合、推论、局限分析由你写，以连贯通顺为首要，不需要引用；
  事实断言必须溯源。既不"为引而写"（叙事迁就文献），也不"为顺而编"（给逻辑层塞编造事实）
- **写后必验**：文稿完成后必须运行 `litkit verify` 核查
