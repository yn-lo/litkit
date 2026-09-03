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

| 用户想要的 | 去向 |
| --- | --- |
| 撰写完整论文/多轮 AI 写作助理 | **academic-paper** 技能（litkit 仅提供检索/引用/合规工具，不替代写作 agent） |
| 同行评审、审稿意见 | **academic-paper-reviewer** 技能 |
| 开放式主题调研、问题探索 | **deep-research** 技能（litkit 的 `search` 只做具体文献检索，不做探索式归纳） |
| 端到端论文流水线编排 | **academic-pipeline** 技能 |
| dedupe/查重引擎原理 | **dedup-engine**（litkit 不提供查重） |

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

**引用健康检查**（final 模式，需本地库 `--type/--lang` 生成 `.litkit`）：
- R5.6 引用防伪：`[@citeKey]` 必须存在于本地库（A 类，退出码 1）
- R5.7 撤稿：联网查 Crossref，被引文献已撤稿即报（A 类）
- R5.8 引用时效：文献年份距今超 `max_age_years`（默认 10）提示（S 类）
- R5.9 自引比例：配置 `self_citation_authors` 后可启用，比例超 `self_citation_max_ratio`（默认 0.15）提示（S 类）

**退出码**：0=通过或仅需人工复核；1=有 A 类违规需修复。

### 论文类型管理

```bash
ls .litkit/                                    # 查看已注册类型
litkit init --type review|empirical|book --lang zh|en  # 追加类型
```

## Critical Rules

- **严禁编造文献**：所有引用必须来自文献库，不得凭空生成 citeKey 或捏造 DOI/作者/标题
- **严禁捏造事实**：所有数据、统计结果、结论必须有文献支撑
- **严禁篡改数据**：引用文献的结论、数字、统计量不得曲解或篡改
- **写后必验**：文稿完成后必须运行 `litkit verify` 核查
