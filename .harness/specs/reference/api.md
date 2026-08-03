# 接口规范 — litkit

---
last_updated: 2026-08-02
status: active
owner: litkit-core
---

> 本文件定义 CLI 命令、MCP 工具与环境变量。CLI 与 MCP 共享同一核心实现（同输入同输出）。
> 数据模型（Paper/SearchResult/PaperSource）见 [`data-model.md`](data-model.md)。
> 错误码与 VerifyIssue 结构见 [`error-codes.md`](error-codes.md)。

## 1. CLI

### 1.1 全局约定

- 输出：默认 JSON（可读模式下人类可读）
- 退出码：`0` 成功 / `1` 运行错误 / `2` 参数错误 / `3` 部分源失败但部分成功
- 所有子命令支持 `--help`；帮助信息自描述（供 AI 调用）

### 1.2 子命令

```
litkit search <query> [-s sources] [-n N] [--mode tiab|full] [--years N|--since YEAR] [-y year] [--keep-no-abstract] [--full]
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
| `--full` | 输出完整元数据 + 完整错误（默认精简视图，FR-IFACE-04） | 关 |

> 远程检索仅 keyword 模式（FR-SEARCH-07）；semantic 模式仅限本地文献库 `lib search`（二期）。
> `errors` 默认精简为失败类型（`timeout` / `rate limited` / `HTTP <码>`），完整错误需 `--full`（FR-IFACE-04）。
> **检索词语言**：必须使用英文（各源英文语料为主，中文命中率极低，FR-SEARCH-11）。
> **检索等级**：默认 `tiab`（题目+摘要+关键词），全文检索 `--mode full`（FR-SEARCH-12）；**时间范围**默认最近 3 年（FR-SEARCH-13）。

输出：`{ total, sourceResults, errors, papers[] }`

```
litkit init [--force]
```

| 参数 | 说明 |
|---|---|
| `--force` | 覆盖已存在的 `.env` / `AGENTS.md` |

> 初始化当前工作目录：生成 `.env`（默认配置）与 `AGENTS.md`（AI agent 使用说明，
> 含检索策略：英文词 / `--mode full` / 时间范围放宽），并初始化 `litkit.db`。
> 已存在文件默认不覆盖。换新工作目录 → `litkit init`。

```
litkit metadata <id_type> <identifier>
```

| 参数 | 说明 |
|---|---|
| `id_type` | `doi` \| `pmid` \| `arxiv` \| `title` |
| `identifier` | 对应标识符 / 标题 |

输出：`Paper | null`

```
litkit sources
```

输出：`{ sources: [{ name, searchable, hasAbstract, enabled }] }`

```
litkit manuscript <draft.md> [--lang zh|en] [-s style] [--docx] [-o output_dir]
```

| 参数 | 说明 | 默认 |
|---|---|---|
| `--lang` | 写作语言模式 | zh |
| `-s, --style` | zh: gb7714-2025；en: apa / ieee | 按 lang |
| `--docx` | 生成 Word（需 Pandoc） | 关 |
| `-o, --output-dir` | 输出目录 | WORK_DIR |

产物：`formatted.md` + `refs.bib` + `refs.ris` + `references.txt` +（可选）`formatted.docx`

```
litkit export <papers.json> [-f bibtex|ris|text] [-s style]
litkit lib list [--source S] [--limit N] [--offset N]
litkit lib search <keyword> [--limit N]
litkit lib rm <cite_key>          # 别名：forget
litkit lib stats | path
litkit lint init [project_dir] [--force] [--lang zh|en]
```

```
litkit verify <file.md> [file2.md ...] [--lang zh|en] [--mode chapter|draft|final] [--rule R2.1,R7.1] [--skip R4.2]
```

| 参数 | 说明 | 默认 |
|---|---|---|
| `<file.md> ...` | 待验证 Markdown 文件（支持多文件） | 必填 |
| `--lang` | 写作语言模式 | zh |
| `--mode` | 验证深度：`chapter`（结构）\| `draft`（+数据/标点/引用）\| `final`（+字数/行文） | draft |
| `--rule` | 仅运行指定规则（逗号分隔，如 `R2.1,R7.1`） | 全部 |
| `--skip` | 跳过指定规则（逗号分隔） | 无 |

> 需要 `LITKIT_WORK_DIR`（读取 `.litkit/specs/manuscript-spec.yaml` 阈值配置）。
> 19 条规则（16 A 类 + 3 S 类）；M 类（R2.4/R4.3）仅输出人工核对提示，不判 fail。
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

## 2. MCP 工具

传输：stdio。所有工具共享核心实现，行为与 CLI 等价。

| 工具 | 入参 | 出参 |
|---|---|---|
| `search_papers` | query: string, sources?: []string, maxResultsPerSource?: int, year?: int, keepNoAbstract?: bool | `SearchResult` |
| `get_paper_metadata` | idType: doi\|pmid\|arxiv\|title, identifier: string | `Paper \| null` |
| `process_manuscript` | text: string, lang?: zh\|en, style?: string, generateDocx?: bool, outputDir?: string | `{ processedText, referenceList, citationMap, unresolved, files }` |
| `export_references` | papers: []Paper, format: bibtex\|ris\|text, style?: string | `{ success, content }` |
| `lint_init` | projectDir?: string, force?: bool, lang?: zh\|en | `{ status, files[], nextSteps }` |
| `verify_manuscript` | files: []string, lang?: zh\|en, mode?: chapter\|draft\|final, rule?: string, skip?: string | `{ files[], passed, exitHint, manualChecklist }` |
| `search_<source>` | query: string, maxResults?: int | 单源 `SearchResult` |
| `lib_list` / `lib_search` / `lib_rm` | source?/keyword?/limit? / citeKey | 库内论文 / 命中 / 删除结果 |
| `lib_stats` / `lib_path` | — | 统计 / 库路径 |

## 3. 环境变量

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
| LITKIT_HTTP_TIMEOUT_MS | 可选 | 单请求超时（默认 15000） |
| LITKIT_HTTP_RETRIES | 可选 | 429/5xx 重试次数（默认 2） |

### .env 发现顺序

`LITKIT_ENV_FILE` 指定 > `WORK_DIR/.env` > 当前目录 `.env` > 项目根 `.env`

## 4. 数据交换格式

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
