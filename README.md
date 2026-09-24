# litkit

面向护理与临床医学科研写作场景的论文工具包：跨源检索、规范引用、手稿排版与 AI 撰写合规门禁。

[English](README.en.md) | **中文**

## 概览

litkit 是一个 Go 编写的论文工具包（Go 1.26 / cobra / SQLite），**面向护理与临床医学科研工作**——覆盖论文从选题检索、文献管理到数据统计与撰写的全流程，医学生同样适用；聚焦临床/人群研究写作，不面向生化等基础实验学科。CLI 为唯一接口，面向 AI agent 与命令行用户：

- **跨源检索**：arxiv / PubMed / bioRxiv / medRxiv / Semantic Scholar / OpenAlex / Crossref / DOAJ 并发检索 + 去重；摘要工作流（不下载 PDF、不抽取全文）；`crossref`/`doaj` 支持中文语料检索。
- **文献入库**：按 DOI / PMID / arXiv / 标题反查元数据；`lib add --doi` 反查即入库；本地 SQLite 文献库管理。
- **全文获取**：按 citeKey / DOI 取回——Unpaywall OA 优先 → Sci-Hub 兜底；PDF 落盘 + 全文缓存（再次取回零网络）。
- **规范引用**：导出 BibTeX / RIS / 文本，支持 GB/T 7714—2025 / APA / IEEE。
- **手稿排版**：解析 `[@citeKey]` 占位符为引用编号；`--preview` 输出自描述核查版，`--docx` 转 Word。
- **撰写合规门禁**：机械化规则验证（语言 / 结构 / 统计 / 标点 / 引用 / 行文 / 字数），`verify` 三档模式；`fix` 自动修正可修项。

**AI-first**：默认返回 AI 写作所需最小字段集（citeKey / title / firstAuthor / year / abstract），完整元数据按需取回；CLI 输出 JSON 可被 AI shell 直接调用。

## 功能

| 能力         | 说明                                                                                                              |
| ------------ | ----------------------------------------------------------------------------------------------------------------- |
| 跨源检索     | 8 源并发 + 去重；`-s` 源过滤 / `-n` 每源条数 / `--mode tiab\|full` / `--years N` / `--since YEAR` / `-y YEAR` / `--exclude` |
| 源清单       | `sources` 列出已注册检索源及其限速配置                                                                             |
| 元数据反查   | `metadata doi\|pmid\|arxiv\|title <id>` 反查元数据（查询）；`lib add --doi <DOI>` 反查并入库                       |
| 全文获取     | `fetch <citeKey\|doi>`：Unpaywall OA → Sci-Hub 兜底；PDF 落盘 + 全文缓存（再次取回零网络）                          |
| 规范引用     | `export <papers.json> -f bibtex\|ris\|text`；样式 GB/T 7714—2025 / APA / IEEE                                       |
| 手稿排版     | `manuscript <draft.md>`：`[@citeKey]` → `[1][2]`；`--preview` / `--docx` / `-o`                                    |
| 撰写合规门禁 | `lint init` 生成约束设施；`verify --mode chapter\|draft\|final`（lang × type × mode 三维过滤）；`--report citation-refs` 引用相关性 LLM 评分；`fix` 自动修正可修项 |
| 规则查询     | `rules` 列出全部规则（ID / 类别 / 适用语言与论文类型 / 验证方法 A·S·M）                                            |
| 文献库管理   | `lib add\|get\|list\|search\|rm\|stats\|path`                                                                      |

## 检索源

不依赖单一检索源，按角色组合公开开放源：

- **元数据骨干**：OpenAlex、Semantic Scholar、Crossref（反查、补全；亦作中文检索源）
- **中文语料**：Crossref、DOAJ（中文关键词可命中中文标题/中文摘要）
- **学科源**：arxiv（预印本）、PubMed（生物医学）、bioRxiv / medRxiv（生命科学预印本）
- **全文通道**：Unpaywall（OA 解析，需邮箱）→ Sci-Hub（兜底，合规风险自知）

全部源均提供摘要（FR-SEARCH-03）。已知上游限制：Semantic Scholar 匿名限速（429）；Sci-Hub 镜像不稳定、可用性随时变化，是否启用由用户自行评估。

## 环境变量

全部配置经 `.env` 读取（`LITKIT_ENV_FILE` 可指定路径；发现顺序 `LITKIT_WORK_DIR/.env` > 当前目录向上溯源，进程环境变量优先），**无任何必需密钥**：

| 变量                                                 | 说明                                                    |
| ---------------------------------------------------- | ------------------------------------------------------- |
| `LITKIT_WORK_DIR`                                  | 工作目录（初始化文献库与配置）                          |
| `LITKIT_ENV_FILE`                                  | 可选，显式指定 `.env` 路径                              |
| `LITKIT_LANG`                                      | 默认写作语言（zh / en，默认 zh）                        |
| `LITKIT_HTTP_TIMEOUT_MS` / `LITKIT_HTTP_RETRIES` | 可选，单请求超时（默认 15000ms）/ 429、5xx 重试次数（默认 2） |
| `LITKIT_PROXY_URL`                                 | 可选，显式代理（http/https/socks5）；设置后全部外呼（检索/元数据/全文/LLM 评分）走代理 |
| `LITKIT_SEMANTIC_SCHOLAR_API_KEY`                  | 可选，提升 Semantic Scholar 限速                        |
| `LITKIT_DEFAULT_MAX_RESULTS` / `LITKIT_DEFAULT_RECENT_YEARS` / `LITKIT_DEFAULT_SEARCH_MODE` / `LITKIT_SEARCH_TIMEOUT_MS` | 可选，检索默认值：每源条数（5）/ 最近年数（3）/ 检索等级（tiab）/ 检索超时 |
| `LITKIT_UNPAYWALL_EMAIL`                           | 可选，Unpaywall 合规邮箱（不设则跳过 OA 通道）          |
| `LITKIT_SCI_HUB_URL` / `LITKIT_FETCH_DOWNLOAD_DIR` | 可选，Sci-Hub 镜像（默认 sci-hub.se）/ PDF 落盘目录（默认 `WORK_DIR/downloads`） |
| `LITKIT_LLM_API_KEY` / `LITKIT_LLM_BASE_URL` / `LITKIT_LLM_TIMEOUT_MS` | 可选，LLM 引用评分凭据、endpoint 与单次超时（默认 30000ms）。全局回落；推荐在 `.litkit/verifier_models.json` 按模型 `api_key`/`base_url` 配置（JSON 优先），也支持 `LITKIT_LLM_API_KEY_<模型ID大写>` 按模型命名约定回落 |

完整清单与逐项注释见 [`app/.env.example`](app/.env.example)。

## 安装

从 [Releases](https://github.com/yn-lo/litkit/releases) 下载对应平台二进制（Windows / Linux / macOS × amd64 / arm64），解压后加入 `PATH`。

源码构建（需 Go 1.26+）：

```bash
cd app && go build -o litkit ./cmd/litkit
```

## 快速开始

```powershell
$env:LITKIT_WORK_DIR = "$HOME\litkit-workspace"   # Linux/macOS: export LITKIT_WORK_DIR=...

litkit init --type empirical --lang zh                 # 1. 初始化（review 综述 | empirical 实证 | book 专著 | proposal 标书）
litkit search "retrieval augmented generation" -n 3    # 2. 跨源检索（输出 JSON）
litkit lib add --doi 10.5555/3295222.3295349           # 3. DOI 反查并入库
litkit fetch <citeKey>                                 # 4. 取回全文（Unpaywall OA → Sci-Hub）
litkit manuscript draft.md --lang zh                   # 5. 手稿排版（[@citeKey] → [1][2]）
litkit manuscript draft.md --preview                   # 5b. 预览版，便于人工核查
litkit lint init --type empirical --lang zh            # 6. 生成撰写约束（已有工作目录时补齐）
litkit verify chapter1.md --mode draft                 # 6b. 撰写合规门禁
litkit fix chapter1.md                                 # 6c. 自动修正可修项（原地覆盖）
litkit verify chapter1.md --report citation-refs       # 6d. 引用相关性 LLM 评分（需配置模型凭据）
```

## 接口与文档

- **CLI**：所有命令输出 JSON（`--full` 输出完整元数据）；`litkit --help` 自描述。
- 完整接口契约（CLI / 数据模型）见 [`.harness/specs/reference/api.md`](.harness/specs/reference/api.md)。

| 文档                       | 位置                                                                              |
| -------------------------- | --------------------------------------------------------------------------------- |
| 需求基线（PRD）            | [`.harness/specs/requirements/PRD.md`](.harness/specs/requirements/PRD.md)       |
| 架构与数据流               | [`.harness/specs/architecture/`](.harness/specs/architecture/)                   |
| 接口规范（CLI / 数据模型） | [`.harness/specs/reference/api.md`](.harness/specs/reference/api.md)             |
| 开发计划                   | [`.harness/specs/plans/roadmap.md`](.harness/specs/plans/roadmap.md)             |
| 开发约定 / 门禁            | [`.harness/specs/conventions/process.md`](.harness/specs/conventions/process.md) |

## 未来规划

> 详细里程碑见 [roadmap.md](.harness/specs/plans/roadmap.md)。已发布 M1–M6；M7（本地库中文检索与二期源）、M8（LLM 引用相关性评分）进行中——M8 核心已落地（Scorer / ScorerEngine 多模型扇出 / 引用句抽取 / `citation_scores` 缓存 / `verify --report citation-refs`），余多模型选型 POC 与人工标注集阈值校准。延伸方向：

- **图表生成**：直接产出 SVG（森林图 / PRISMA 流程图 / 纳入文献统计），数据来自本地库。
- **系统综述 `litkit review`**：PRISMA 工作流（检索→去重→筛选→纳入清单→森林图数据）。
- **统计分析**：封装 R/Rscript 执行检验（可选依赖），数据交接、回读结果进稿件。
- **AI 质性研究分析**：LLM 辅助质性编码与主题分析（独立命令族）。

## 开发

```bash
# 全量门禁（gofmt → build → lint → vet → test → vulncheck → arch-check → sync）
powershell -File .harness/constraints/gate.ps1    # Windows
bash .harness/constraints/gate.sh                 # Linux/macOS
```

## License

[Apache-2.0](LICENSE). Copyright © 2026 [YnLo](https://www.ynlo.top/).
