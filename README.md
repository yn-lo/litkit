# litkit

国内学术写作场景的论文工具包。面向 AI agent 与命令行用户：跨源检索文献（摘要工作流）、生成规范引用（GB/T 7714—2025 / APA / IEEE）、排版手稿、AI 撰写合规门禁。

CLI 为第一接口（输出默认 JSON，可被 AI shell 调用），MCP Server 为可选第二接口（stdio，供 Claude Desktop / Trae 等客户端发现调用）。CLI 与 MCP 共享同一核心实现，同输入同输出。

## 功能

| 能力 | 说明 |
|---|---|
| 跨源检索 | arxiv / bioRxiv / OpenAlex / PubMed / Semantic Scholar，摘要工作流（不下载 PDF） |
| 元数据反查 | 按 DOI / PMID / arXiv ID / 标题反查论文元数据，入库本地文献库（SQLite） |
| 规范引用 | 导出 BibTeX / RIS / 文本，样式支持 GB/T 7714—2025 / APA / IEEE |
| 手稿排版 | 解析 `[@citeKey]` 引用占位符，生成引用列表与编号，可选 Pandoc 转 docx |
| 撰写合规门禁 | 19 条规则（结构 / 数据 / 标点 / 引用 / 字数 / 行文），`lint init` 生成项目 harness |
| MCP 接口 | `litkit mcp` 启动 stdio Server，全部工具与 CLI 命令一一对应 |

## 快速开始

```bash
# 安装（发布二进制，见 Releases；或源码构建）
go build -o litkit ./cmd/litkit    # 在 app/ 目录

# 1. 初始化工作目录（生成 .env / AGENTS.md，初始化文献库）
export LITKIT_WORK_DIR="$HOME/litkit-workspace"   # Windows: $env:LITKIT_WORK_DIR="..."
litkit init

# 2. 跨源检索（关键词用英文；结果默认 JSON）
litkit search "retrieval augmented generation" -n 3

# 3. 元数据反查并入库
litkit metadata doi 10.5555/3295222.3295349

# 4. 排版手稿（[@citeKey] 占位符自动解析为 [1][2]）
litkit manuscript draft.md --lang zh

# 5. 撰写合规门禁
litkit lint init . && litkit verify chapter1.md
```

完整命令与输出契约见 [`.harness/specs/reference/api.md`](.harness/specs/reference/api.md)。

## AI 调用

- **CLI**：所有命令输出 JSON（`--full` 逃生口输出完整元数据）；`litkit --help` 自描述。
- **MCP**：`litkit mcp` 启动 stdio Server，向 MCP 客户端注册 `search_papers` / `get_paper_metadata` / `process_manuscript` / `export_references` / `lint_init` / `verify_manuscript` / `lib_*` / `search_<source>` 等工具。

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
# 全量门禁（7 项：gofmt → build → lint → vet → test → vulncheck → arch-check + sync）
powershell -File .harness/constraints/gate.ps1    # Windows
bash .harness/constraints/gate.sh                 # Linux/macOS

# 接口一致性（CLI / MCP / api.md 三处同步，FR-IFACE-03）
cd .harness/constraints/sync && go run .

# 触网测试（手动）
cd app && go test -tags integration ./tests/integration/

# 发布（快照构建）
cd app && goreleaser build --snapshot
```

## License

[Apache-2.0](LICENSE)。Copyright © 2026 litkit 贡献者。
