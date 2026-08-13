# 接口规范 — litkit

---
last_updated: 2026-08-02
status: active
owner: litkit-core
---

> 本文件定义 CLI 命令与环境变量。
> 数据模型（Paper/SearchResult/PaperSource）见 [`data-model.md`](data-model.md)。
> 错误码与 VerifyIssue 结构见 [`error-codes.md`](error-codes.md)。

## 1. CLI

### 1.1 全局约定

- 输出：默认 JSON（可读模式下人类可读）
- 退出码：`0` 成功 / `1` 运行错误 / `2` 参数错误 / `3` 部分源失败但部分成功
- 所有子命令支持 `--help`；帮助信息自描述（供 AI 调用）

### 1.2 子命令

```
litkit search <query> [-s sources] [-n N] [--mode tiab|full] [--years N|--since YEAR] [-y year] [--keep-no-abstract] [--exclude TERM,...] [--full]
```

| 参数 | 说明 | 默认 |
|---|---|---|
| `-s, --sources` | 逗号分隔源列表（`litkit sources` 查看） | 全部默认源 |
| `-n, --max-results` | 每源最大条数 | 5 |
| `--mode` | 检索等级：`tiab`（题目+摘要+关键词）\| `full`（全文） | `tiab`（`LITKIT_DEFAULT_SEARCH_MODE`） |
| `--years` | 最近 N 年；`0`=不限 | 3（`LITKIT_DEFAULT_RECENT_YEARS`） |
| `--since` | 显式起始年份（含），优先于 `--years` | 无 |
| `-y, --year` | 精确年份过滤（源支持时） | 无 |
| `--keep-no-abstract` | 保留无摘要论文（默认过滤，FR-SEARCH-03） | 关 |
| `--exclude` | 逗号分隔排除词：标题或摘要命中任一排除词即剔除（本地召回后筛查，先排除后入库） | 无 |
| `--full` | 输出完整元数据 + 完整错误（默认精简视图，FR-IFACE-02） | 关 |

> 远程检索仅 keyword 模式（FR-SEARCH-07）；semantic 模式仅限本地文献库 `lib search`（二期）。
> `errors` 默认精简为失败类型（`timeout` / `rate limited` / `HTTP <码>`），完整错误需 `--full`（FR-IFACE-02）。
> **检索词语言**：必须使用英文（各源英文语料为主，中文命中率极低，FR-SEARCH-11）。
> **检索等级**：默认 `tiab`（题目+摘要+关键词），全文检索 `--mode full`（FR-SEARCH-12）；**时间范围**默认最近 3 年（FR-SEARCH-13）。

输出：`{ total, sourceResults, errors, papers[] }`

```
litkit init [--force] [--type review|empirical|book] [--lang zh|en] [--journal NAME] [--refresh]
```

| 参数 | 说明 | 默认 |
|---|---|---|
| `--force` | 覆盖已存在的 `.env` / `AGENTS.md` | 关 |
| `--type` | 论文类型：`review`（综述）\| `empirical`（四段式实证）\| `book`（书籍，中文编校细则） | empirical |
| `--lang` | 撰写语言 | zh |
| `--journal` | 目标期刊名称（写入 spec，影响引用格式默认值与 checklist） | 空 |
| `--refresh` | 按现有 manuscript-spec.yaml 重新生成 AGENTS.md 撰写段 | 关 |

> 初始化当前工作目录：生成 `.env`（默认配置）与 `AGENTS.md`（AI agent 使用说明，
> 含检索策略：英文词 / `--mode full` / 时间范围放宽），并初始化 `litkit.db`。
> 已存在文件默认不覆盖。换新工作目录 → `litkit init`。
> `--type`/`--lang`/`--journal` 仅在首次生成 yaml 时生效；`--refresh` 按现有 yaml 重新渲染 AGENTS.md。
> 交互式向导：旗标未显式传值且 stdin 是终端时，进入问答。

```
litkit metadata <id_type> <identifier>
```

| 参数 | 说明 |
|---|---|
| `id_type` | `doi` \| `pmid` \| `arxiv` \| `title` |
| `identifier` | 对应标识符 / 标题 |

输出：`Paper | null`

```
litkit fetch <cite_key|doi>
```

| 参数 | 说明 |
|---|---|
| `<cite_key\|doi>` | 3 字母引用标识或 DOI；论文须已入库 |

输出：`{ citeKey, pdfPath?, fulltext, via }`，via = `cache` \| `unpaywall` \| `scihub`。
流程：Unpaywall 按 DOI 解析 OA PDF（需 `LITKIT_UNPAYWALL_EMAIL`）→ 未命中则 Sci-Hub
兜底（默认开启，`LITKIT_SCI_HUB_URL` 可配，失败静默）→ PDF 落盘
`<WORK_DIR>/downloads/<citeKey>.pdf` → 抽取全文缓存入库（`papers.fulltext`）。
库中已有全文缓存时直接返回（零网络）。扫描版 PDF 无文本层，抽取返回空文本。

```
litkit sources
```

输出：`{ sources: [{ name, searchable, hasAbstract, enabled }] }`

```
litkit manuscript <draft.md> [--lang zh|en] [-s style] [--preview] [--docx] [-o output_dir]
```

| 参数 | 说明 | 默认 |
|---|---|---|
| `--lang` | 写作语言模式 | zh |
| `-s, --style` | zh: gb7714-2025；en: apa / ieee | 按 lang |
| `--preview` | 预览模式：内联标记自描述（`[@doi:{DOI} — 标题]`；无 DOI 用 `[@标题]`），文末仍追加编号引用列表 | 关 |
| `--docx` | 生成 Word（需 Pandoc） | 关 |
| `-o, --output-dir` | 输出目录（默认 WORK_DIR/outputs） | WORK_DIR/outputs |

产物：`{base}_{ts}.md`（正文 + 文末参考文献列表）+ `{base}_{ts}.bib` + `{base}_{ts}.ris` +（可选）`{base}_{ts}.docx`。`ts` 为时间戳（精确到秒，统一格式 `20060102_150405`），`base` 为输入文件名去除扩展名。所有产物共用一个时间戳。

```
litkit export <papers.json> [-f bibtex|ris|text] [-s style]
litkit lib list [--source S] [--limit N] [--offset N]
litkit lib search <keyword> [--limit N]
litkit lib rm <cite_key>          # 别名：forget
litkit lib stats | path
litkit lint init [project_dir] [--force] [--lang zh|en] [--type review|empirical|book] [--journal NAME]
litkit rules
```

```
litkit rules
```

输出：`{ rules: [{ id, name, category, langs, types, method, from }] }`

| 字段 | 含义 |
|---|---|
| `id` | 规则 ID（如 `R3.4`，用于 `--skip` 与 spec `skip_rules`） |
| `category` | 检查类别（`--check`/`--skip-check` 用） |
| `types` | 适用论文类型（空=`[]` 表示全部类型） |
| `from` | 启用模式：`chapter` \| `draft` \| `final`（递增） |

> 纯查询命令，不依赖 `LITKIT_WORK_DIR`。

```
litkit verify <file.md> [file2.md ...] [--lang zh|en] [--mode chapter|draft|final] [--type review|empirical|book] [--rule R2.1,R7.1] [--skip R4.2]
```

| 参数 | 说明 | 默认 |
|---|---|---|
| `<file.md> ...` | 待验证 Markdown 文件（支持多文件） | 必填 |
| `--lang` | 写作语言模式 | zh |
| `--mode` | 验证深度：`chapter`（结构）\| `draft`（+数据/标点/引用）\| `final`（+字数/行文） | draft |
| `--type` | 论文类型：`review` \| `empirical` \| `book`（空=从 spec 自动取） | 空 |
| `--rule` | 仅运行指定规则（逗号分隔，如 `R2.1,R7.1`） | 全部 |
| `--skip` | 跳过指定规则（逗号分隔） | 无 |

> 需要 `LITKIT_WORK_DIR`（读取 `.litkit/specs/manuscript-spec.yaml` 阈值配置）。
> 21 条规则（18 A 类 + 3 S 类）；M 类（R2.4/R4.3）仅输出人工核对提示，不判 fail。
> spec 的 `skip_rules` 字段可永久跳过指定规则（等效每次 `--skip`）。
> 模式递增：chapter → draft → final，高模式包含低模式全部规则。
> Markdown 分段：排除代码块/参考文献/表格后检查 Body。

输出：

```json
{
  "files": [
    {
      "path": "chapter1.md",
      "violations": [
        { "ruleId": "R2.1", "line": 42, "problem": "...", "suggestion": "..." }
      ]
    }
  ],
  "passed": false,
  "exitHint": "fix_and_rerun",
  "manualChecklist": ["R2.4: 核对统计量与原文一致", "R4.3: 确认引用与论述对应"]
}
```

| exitHint | 含义 |
|---|---|
| `pass` | 全部通过 |
| `fix_and_rerun` | 有 A 类违规，修复后重跑 |
| `manual_review` | 仅需人工复核（M 类提示） |

退出码：`0` = 通过或仅需人工复核；`1` = 有 A 类违规需修复。

## 2. 环境变量

| 变量 | 必需 | 作用 |
|---|---|---|
| LITKIT_SEMANTIC_SCHOLAR_API_KEY | 可选 | Semantic Scholar 速率提升 |
| LITKIT_IEEE_API_KEY | 二期激活 IEEE 必需 | 启用 IEEE 源 |
| LITKIT_ACM_API_KEY | 二期激活 ACM 必需 | 启用 ACM 源 |
| LITKIT_WORK_DIR | 必填 | 统一工作目录（库/输出默认在此）。**未设置时 init/search/lib 拒绝执行（errNoWorkDir，FR-LIB-03）**。测试固化目录：`e:\Codes\litkit\workspace` |
| LITKIT_ENV_FILE | 可选 | 显式 .env 路径 |
| LITKIT_LANG | 可选 | 默认写作语言模式（zh/en） |
| LITKIT_EMBEDDING_PROVIDER | 可选 | local（默认）/ api |
| LITKIT_EMBEDDING_API_KEY | api 模式必需 | 阿里百炼 / 硅基流动 embedding key |
| LITKIT_UNPAYWALL_EMAIL | 可选 | 全文 OA 解析必需（fetch，FR-FETCH-02） |
| LITKIT_SCI_HUB_URL | 可选 | Sci-Hub 兜底镜像（默认 https://sci-hub.se，FR-FETCH-03） |
| LITKIT_FETCH_DOWNLOAD_DIR | 可选 | 全文 PDF 落盘目录（默认 <WORK_DIR>/downloads） |
| LITKIT_HTTP_TIMEOUT_MS | 可选 | 单请求超时（默认 15000） |
| LITKIT_HTTP_RETRIES | 可选 | 429/5xx 重试次数（默认 2） |
| LITKIT_LLM_API_KEY | 引用评分必需（FR-LINT-08） | LLM 评分的 API key |
| LITKIT_LLM_BASE_URL | 可选 | LLM API base URL（自托管/代理 endpoint） |
| LITKIT_LLM_TIMEOUT_MS | 可选 | LLM 单次评分超时（默认 30000） |
| LITKIT_VERIFY_LINT_LLM | 可选 | 启用 LLM 引用评分，默认 false（避免意外远程调用） |

### .env 发现顺序

`LITKIT_ENV_FILE` 指定 > `WORK_DIR/.env` > 当前目录 `.env` > 项目根 `.env`

## 3. 数据交换格式

### Paper

```json
{
  "id": "sha256:...",
  "title": "Graph Neural Networks: A Review",
  "authors": [ { "given": "Zhou", "family": "Jie" } ],
  "abstract": "...",
  "year": 2020,
  "venue": "IEEE TPAMI",
  "doi": "10.1109/TPAMI.2020.2971442",
  "pmid": null,
  "arxivId": null,
  "url": "https://doi.org/10.1109/TPAMI.2020.2971442",
  "source": "crossref",
  "docType": "article"
}
```

### SearchResult

```json
{
  "total": 23,
  "sourceResults": { "crossref": [ ... ] },
  "errors": [ { "source": "pubmed", "error": "timeout" } ],
  "papers": [ ... ]
}
```

### VerifyIssue（三要素）

详见 [`error-codes.md`](error-codes.md)。
