# litkit

面向国内学术写作场景的论文工具包：跨源检索、规范引用、手稿排版与 AI 撰写合规门禁。

[English](README.en.md) | **中文**

## 概览

litkit 是一个 Go 编写的论文工具包（Go 1.26 / cobra / SQLite），CLI 为唯一接口，面向 AI agent 与命令行用户：

- **跨源检索**：arxiv / PubMed / bioRxiv / medRxiv / Semantic Scholar / OpenAlex 并发检索 + 去重；摘要工作流（不下载 PDF、不抽取全文）。
- **文献入库**：按 DOI / PMID / arXiv / 标题反查元数据；`lib add --doi` 反查即入库；本地 SQLite 文献库管理。
- **全文获取**：按 citeKey / DOI 取回——Unpaywall OA 优先 → Sci-Hub 兜底；PDF 落盘 + 全文缓存（再次取回零网络）。
- **规范引用**：导出 BibTeX / RIS / 文本，支持 GB/T 7714—2025 / APA / IEEE。
- **手稿排版**：解析 `[@citeKey]` 占位符为引用编号；`--preview` 输出自描述核查版，`--docx` 转 Word。
- **撰写合规门禁**：机械化规则验证（语言 / 结构 / 统计 / 标点 / 引用 / 行文 / 字数），`verify` 三档模式。

**AI-first**：默认返回 AI 写作所需最小字段集（citeKey / title / firstAuthor / year / abstract），完整元数据按需取回；CLI 输出 JSON 可被 AI shell 直接调用。

## 项目原则

- **AI-first 降噪**：接口设计以降低上下文噪声为第一约束。
- **CLI 唯一接口**：全部功能经 CLI 命令完成。
- **摘要工作流**：检索源必须提供摘要，无摘要论文默认过滤；不下载 PDF、不抽取全文。
- **免费优先**：全部源为公开开放接口，无强制 API key；密钥一律走 `.env`（gitignored），禁止硬编码。
- **接口同步**：新增 CLI 功能同步接口文档 api.md。

## 功能

| 能力 | 说明 |
| --- | --- |
| 跨源检索 | 6 源并发 + 去重；`-s` 源过滤 / `-n` 每源条数 / `--mode tiab\|full` / `--years N` |
| 元数据反查 | `metadata doi\|pmid\|arxiv\|title <id>` 反查元数据（查询）；`lib add --doi <DOI>` 反查并入库 |
| 全文获取 | Unpaywall OA → Sci-Hub 兜底；PDF 落盘 + 全文缓存（再次取回零网络） |
| 规范引用 | `export -f bibtex\|ris\|text`；样式 GB/T 7714—2025 / APA / IEEE |
| 手稿排版 | `[@citeKey]` → `[1][2]`；`--preview` / `--docx` / `-o` |
| 撰写合规门禁 | `lint init` 生成 harness；`verify --mode draft\|chapter\|final`；`--report citation-refs` 引用相关性 LLM 评分 |
| 文献库管理 | `lib add\|search\|list\|rm\|stats\|path` |

## 检索源

不依赖单一检索源，按角色组合公开开放源：

- **元数据骨干**：OpenAlex、Semantic Scholar、Crossref（反查、补全）
- **学科源**：arxiv（预印本）、PubMed（生物医学）、bioRxiv / medRxiv（生命科学预印本）
- **全文通道**：Unpaywall（OA 解析，需邮箱）→ Sci-Hub（兜底，合规风险自知）

全部源均提供摘要（FR-SEARCH-03）。已知上游限制：Semantic Scholar 匿名限速（429）；Sci-Hub 镜像不稳定、可用性随时变化，是否启用由用户自行评估。

## 配置

全部配置经 `.env` 读取（`LITKIT_ENV_FILE` 可指定路径），**无任何必需密钥**：

| 变量 | 说明 |
| --- | --- |
| `LITKIT_WORK_DIR` | 工作目录（初始化文献库与配置） |
| `LITKIT_LANG` | 默认语言（zh / en） |
| `LITKIT_SEMANTIC_SCHOLAR_API_KEY` | 可选，提升 Semantic Scholar 限速 |
| `LITKIT_UNPAYWALL_EMAIL` | 可选，Unpaywall 合规邮箱（不设则跳过 OA 通道） |
| `LITKIT_SCI_HUB_URL` | 可选，Sci-Hub 镜像地址（默认 sci-hub.se） |
| `LITKIT_HTTP_TIMEOUT_MS` / `LITKIT_HTTP_RETRIES` | 可选，网络超时与重试 |
| `LITKIT_LLM_API_KEY` | 可选，LLM 引用评分 key |
| `LITKIT_LLM_BASE_URL` | 可选，LLM 自托管 endpoint |
| `LITKIT_VERIFY_LINT_LLM` | 可选，启用 LLM 引用评分（默认 false，避免意外远程调用） |

## 安装

从 [Releases](https://github.com/yn-lo/litkit/releases) 下载对应平台二进制（Windows / Linux / macOS × amd64 / arm64），解压后加入 `PATH`。

源码构建（需 Go 1.26+）：

```bash
cd app && go build -o litkit ./cmd/litkit
```

## 快速开始

```powershell
$env:LITKIT_WORK_DIR = "$HOME\litkit-workspace"   # Linux/macOS: export LITKIT_WORK_DIR=...

litkit init --type empirical --lang zh                 # 1. 初始化（review 综述 | empirical 实证）
litkit search "retrieval augmented generation" -n 3    # 2. 跨源检索（输出 JSON）
litkit lib add --doi 10.5555/3295222.3295349           # 3. DOI 反查并入库
litkit fetch <citeKey>                                 # 4. 取回全文（Unpaywall OA → Sci-Hub）
litkit manuscript draft.md --lang zh                   # 5. 手稿排版（[@citeKey] → [1][2]）
litkit manuscript draft.md --preview                   # 5b. 预览版，便于人工核查
litkit lint init --type empirical --lang zh            # 6. 生成撰写约束
litkit verify chapter1.md --mode draft                 # 6b. 撰写合规门禁
```

## 接口与文档

- **CLI**：所有命令输出 JSON（`--full` 输出完整元数据）；`litkit --help` 自描述。
- 完整接口契约（CLI / 数据模型）见 [`.harness/specs/reference/api.md`](.harness/specs/reference/api.md)。

| 文档 | 位置 |
| --- | --- |
| 需求基线（PRD） | [`.harness/specs/requirements/PRD.md`](.harness/specs/requirements/PRD.md) |
| 架构与数据流 | [`.harness/specs/architecture/`](.harness/specs/architecture/) |
| 接口规范（CLI / 数据模型） | [`.harness/specs/reference/api.md`](.harness/specs/reference/api.md) |
| 开发计划 | [`.harness/specs/plans/roadmap.md`](.harness/specs/plans/roadmap.md) |
| 开发约定 / 门禁 | [`.harness/specs/conventions/process.md`](.harness/specs/conventions/process.md) |

## 开发

```bash
# 全量门禁（gofmt → build → lint → vet → test → vulncheck → arch-check → sync）
powershell -File .harness/constraints/gate.ps1    # Windows
bash .harness/constraints/gate.sh                 # Linux/macOS

# 发布：打 tag 触发 GitHub Actions 构建并上传 Release
git tag v0.1.0 && git push origin v0.1.0
```

## 贡献

需求、设计与接口基线见 [`.harness/specs/`](.harness/specs/)；关键需求/节点采用 TDD 开发。提交前请跑通全量门禁（见[开发](#开发)）。所有贡献默认遵循 [Apache-2.0](LICENSE)。

## License

[Apache-2.0](LICENSE)。Copyright © 2026 [YnLo](https://www.ynlo.top/)。