# litkit

面向国内学术写作场景的论文工具包：跨源检索、规范引用、手稿排版与 AI 撰写合规门禁。

[English](README.en.md) | **中文**

## 目录

- [概览](#概览)
- [项目原则](#项目原则)
- [功能](#功能)
- [检索源策略](#检索源策略)
- [源能力矩阵](#源能力矩阵)
- [配置](#配置)
- [安装](#安装)
- [快速开始](#快速开始)
- [AI 集成](#ai-集成)
- [文档导航](#文档导航)
- [开发](#开发)
- [贡献](#贡献)
- [License](#license)

## 概览

litkit 是一个 Go 编写的论文工具包（Go 1.26 / cobra / MCP SDK / SQLite），CLI 为第一接口，MCP Server 为可选第二接口。面向 AI agent 与命令行用户，覆盖论文写作完整链路：

- **跨源检索**：arxiv / PubMed / bioRxiv / medRxiv / Semantic Scholar / OpenAlex 并发检索 + 去重，摘要工作流（不下载 PDF、不抽取全文）。
- **元数据反查**：按 DOI / PMID / arXiv ID / 标题反查，入库 SQLite 本地文献库。
- **全文获取**：按 citeKey / DOI 取回——Unpaywall OA 优先 → Sci-Hub 兜底，PDF 落盘 + 全文缓存入库。
- **规范引用**：导出 BibTeX / RIS / 文本，支持 GB/T 7714—2025 / APA / IEEE。
- **手稿排版**：解析 `[@citeKey]` 占位符为引用编号；`--preview` 输出自描述标记便于人工核查。
- **撰写合规门禁**：19 条规则机械化验证（结构 / 数据 / 标点 / 引用 / 字数 / 行文）。

**AI-first**：默认返回 AI 写作所需最小字段集（citeKey / title / firstAuthor / year / abstract），完整元数据按需取回；`--full` 供人类调试。CLI 输出 JSON 可被 AI shell 直接调用；`litkit mcp` 启动 stdio MCP Server，与 CLI 共享同一核心实现，同输入同输出。

## 项目原则

- **AI-first 降噪**：接口设计以降低上下文噪声为第一约束。
- **CLI 第一，MCP 第二**：CLI 为主接口，MCP 为可选第二接口，共享同一核心与源注册表。
- **摘要工作流**：检索源必须提供摘要，无摘要论文默认过滤（FR-SEARCH-03）；不下载 PDF、不抽取全文。
- **免费优先**：全部源为公开开放接口，无强制 API key；密钥一律走 `.env`（gitignored），禁止硬编码。
- **接口同步**：新增功能 CLI / MCP 两处注册，并同步接口文档（FR-IFACE-03）。

## 功能

| 能力 | 说明 |
| --- | --- |
| 跨源检索 | 6 源并发 + 去重；`-s` 源过滤 / `-n` 每源条数 / `--mode tiab\|full` / `--years N` |
| 元数据反查 | `metadata doi\|pmid\|arxiv\|title <id>` 反查并入库 |
| 全文获取 | Unpaywall OA → Sci-Hub 兜底；PDF 落盘 + 全文缓存（再次取回零网络） |
| 规范引用 | `export -f bibtex\|ris\|text`；样式 GB/T 7714—2025 / APA / IEEE |
| 手稿排版 | `[@citeKey]` → `[1][2]`；`--preview` / `--docx` / `-o` |
| 撰写合规门禁 | `lint init` 生成项目 harness；`verify --mode draft\|chapter\|final` |
| 文献库管理 | `lib search\|list\|rm\|stats\|path` |
| MCP 接口 | `litkit mcp` 启动 stdio Server，工具与 CLI 一一对应 |

## 检索源策略

不依赖单一检索源，按角色组合公开开放源：

- **元数据骨干**：OpenAlex、Semantic Scholar（反查、补全元数据）
- **学科源**：arxiv（预印本）、PubMed（生物医学）、bioRxiv / medRxiv（生命科学预印本）
- **全文通道**：Unpaywall（OA 解析，需邮箱）→ Sci-Hub（兜底，合规风险自知）

长期路线：保持现有公开源稳定，按需扩展（Crossref / dblp / Europe PMC / PMC OA 等）。

## 源能力矩阵

| 源 | 检索 | 摘要 | 备注 |
| --- | --- | --- | --- |
| arxiv | ✅ | ✅ | 预印本；官方限速 1 req/3s |
| PubMed | ✅ | ✅ | 生物医学 |
| bioRxiv / medRxiv | ✅ | ✅ | 生命科学预印本 |
| Semantic Scholar | ✅ | ✅ | 可选 API key 提升限速（匿名 429） |
| OpenAlex | ✅ | ✅ | 开放元数据骨干 |

> 全文获取与检索源无关：按 DOI 走 Unpaywall OA → Sci-Hub 兜底。已知上游限制：Semantic Scholar 匿名限速（429）；Sci-Hub 镜像不稳定、可用性随时变化，是否启用由用户自行评估。

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
litkit metadata doi 10.5555/3295222.3295349            # 3. 元数据反查入库
litkit fetch <citeKey>                                 # 4. 取回全文（Unpaywall OA → Sci-Hub）
litkit manuscript draft.md --lang zh                   # 5. 手稿排版（[@citeKey] → [1][2]）
litkit manuscript draft.md --preview                   # 5b. 预览模式，便于人工核查
litkit lint init --type empirical --lang zh            # 6. 生成撰写约束
litkit verify chapter1.md --mode draft                 # 6b. 撰写合规门禁
```

## AI 集成

- **CLI**：所有命令输出 JSON（`--full` 输出完整元数据）；`litkit --help` 自描述。
- **MCP**：`litkit mcp` 启动 stdio Server，注册 `search_papers` / `get_paper_metadata` / `fetch_paper` / `process_manuscript` / `export_references` / `lint_init` / `verify_manuscript` / `lib_*` / `search_<source>` 等工具，供 Claude Desktop / Trae 等客户端调用。

完整接口契约（CLI / MCP / 数据模型）见 [`.harness/specs/reference/api.md`](.harness/specs/reference/api.md)。

## 文档导航

| 文档 | 位置 |
| --- | --- |
| 需求基线（PRD） | [`.harness/specs/requirements/PRD.md`](.harness/specs/requirements/PRD.md) |
| 架构与数据流 | [`.harness/specs/architecture/`](.harness/specs/architecture/) |
| 接口规范（CLI / MCP / 数据模型） | [`.harness/specs/reference/api.md`](.harness/specs/reference/api.md) |
| 开发计划 | [`.harness/specs/plans/roadmap.md`](.harness/specs/plans/roadmap.md) |
| 开发约定 / 门禁 | [`.harness/specs/conventions/process.md`](.harness/specs/conventions/process.md) |

## 开发

```bash
# 全量门禁（8 项：gofmt → build → lint → vet → test → vulncheck → arch-check → sync）
powershell -File .harness/constraints/gate.ps1    # Windows
bash .harness/constraints/gate.sh                 # Linux/macOS

# 接口一致性（CLI / MCP / api.md 三处同步，FR-IFACE-03）
cd .harness/constraints/sync && go run .

# 触网测试（手动）
cd app && go test -tags integration ./tests/integration/

# 发布：打 tag 触发 GitHub Actions 构建并上传 Release
git tag v0.1.0 && git push origin v0.1.0
```

## 贡献

需求、设计与接口基线见 [`.harness/specs/`](.harness/specs/)；关键需求/节点采用 TDD 开发。提交前请跑通全量门禁（见[开发](#开发)）。所有贡献默认遵循 [Apache-2.0](LICENSE)。

## License

[Apache-2.0](LICENSE)。Copyright © 2026 [YnLo](https://www.ynlo.top/)。
