# litkit

国内学术写作场景的论文工具包。面向 AI agent 与命令行用户：跨源检索文献（摘要工作流）、生成规范引用（GB/T 7714—2025 / APA / IEEE）、排版手稿、AI 撰写合规门禁。

CLI 为第一接口（输出默认 JSON，可被 AI shell 调用），MCP Server 为可选第二接口（stdio，供 Claude Desktop / Trae 等客户端发现调用）。CLI 与 MCP 共享同一核心实现，同输入同输出。

## 功能

| 能力 | 说明 |
|---|---|
| 跨源检索 | arxiv / bioRxiv / OpenAlex / PubMed / Semantic Scholar，摘要工作流（不下载 PDF） |
| 元数据反查 | 按 DOI / PMID / arXiv ID / 标题反查论文元数据，入库本地文献库（SQLite） |
| 全文获取 | 按 citeKey 或 DOI 取回全文：Unpaywall OA 优先 → Sci-Hub 兜底，PDF 落盘 + 全文缓存入库 |
| 规范引用 | 导出 BibTeX / RIS / 文本，样式支持 GB/T 7714—2025 / APA / IEEE |
| 手稿排版 | 解析 `[@citeKey]` 引用占位符，生成引用列表与编号；`--preview` 输出自描述标记便于人工核查 |
| 撰写合规门禁 | 19 条规则（结构 / 数据 / 标点 / 引用 / 字数 / 行文），`lint init` 生成项目 harness |
| MCP 接口 | `litkit mcp` 启动 stdio Server，全部工具与 CLI 命令一一对应 |

## 安装

从 [Releases](https://github.com/yn-lo/litkit/releases) 下载对应平台的二进制（Windows / Linux / macOS × amd64 / arm64），解压后加入 `PATH` 即可。

源码构建（需 Go 1.26+）：

```bash
cd app && go build -o litkit ./cmd/litkit
```

## 快速开始

```powershell
# 设置工作目录（litkit 在此初始化文献库与配置）
$env:LITKIT_WORK_DIR = "$HOME\litkit-workspace"   # Linux/macOS: export LITKIT_WORK_DIR=...

# 1. 初始化项目（--type 必填：review 综述 | empirical 四段式实证）
litkit init --type empirical --lang zh

# 2. 跨源检索（关键词用英文，结果默认 JSON）
litkit search "retrieval augmented generation" -n 3

# 3. 元数据反查并入库
litkit metadata doi 10.5555/3295222.3295349

# 4. 取回论文全文（需已入库；Unpaywall OA → Sci-Hub 兜底）
litkit fetch <citeKey>

# 5. 排版手稿（[@citeKey] 占位符自动解析为 [1][2]）
litkit manuscript draft.md --lang zh

# 5b. 预览模式（内联标记自描述，不生成引用列表，便于人工核查）
litkit manuscript draft.md --preview

# 6. 撰写合规门禁
litkit lint init --type empirical --lang zh
litkit verify chapter1.md --mode draft
```

## 命令一览

| 命令 | 说明 |
|---|---|
| `litkit init` | 初始化项目（`.env` + `litkit.db` + `AGENTS.md` + 论文类型目录） |
| `litkit search <query>` | 跨源检索（`-s` 源过滤 / `-n` 每源条数 / `--mode tiab\|full` / `--years N`） |
| `litkit metadata <id_type> <id>` | 按 doi / pmid / arxiv / title 反查元数据 |
| `litkit fetch <citeKey\|doi>` | 取回全文，PDF 落盘 + 全文缓存入库 |
| `litkit sources` | 列出可用检索源 |
| `litkit manuscript <draft.md>` | 手稿排版（`--preview` / `--docx` / `-o`） |
| `litkit export <papers.json>` | 批量导出引用（`-f bibtex\|ris\|text`） |
| `litkit lib list\|search\|rm\|stats\|path` | 本地文献库管理 |
| `litkit lint init` | 生成撰写约束（`.litkit/` + `AGENTS.md`） |
| `litkit verify <file.md>` | 机械化验证（`--mode chapter\|draft\|final`） |
| `litkit mcp` | 启动 MCP stdio Server |

完整命令与输出契约见 [`.harness/specs/reference/api.md`](.harness/specs/reference/api.md)。

## AI 调用

- **CLI**：所有命令输出 JSON（`--full` 逃生口输出完整元数据）；`litkit --help` 自描述。
- **MCP**：`litkit mcp` 启动 stdio Server，向 MCP 客户端注册 `search_papers` / `get_paper_metadata` / `fetch_paper` / `process_manuscript` / `export_references` / `lint_init` / `verify_manuscript` / `lib_*` / `search_<source>` 等工具。

## 文档导航

| 文档 | 位置 |
|---|---|
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

# 发布（打 tag 触发 GitHub Actions 自动构建并上传 Release）
git tag v0.1.0 && git push origin v0.1.0
```

## License

[Apache-2.0](LICENSE)。Copyright © 2026 [ynlo](https://www.ynlo.top/)。
