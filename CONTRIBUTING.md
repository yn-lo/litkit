# 贡献指南 — litkit

感谢你参与 litkit 开发。本文件是贡献流程的入口；细节约定见 [`.harness/specs/conventions/`](.harness/specs/conventions/)。

## 工作流

1. **先改 PRD，再实现**：需求文档是唯一权威基线（[`PRD.md`](.harness/specs/requirements/PRD.md)）。新增能力先在 PRD 登记，再进入设计（[`feature-*.md`](.harness/specs/design/)）与实现。
2. **TDD**：关键需求/节点使用测试驱动开发（红-绿-重构）。每个非平凡逻辑留下一份可运行的回归检查。
3. **跑门禁**：提交前必须通过全量门禁（7 项），见下方。
4. **提交**：`<type>: <subject>`，type ∈ `feat/fix/refactor/docs/test/chore`。禁止 force push 主分支。

## 接口同步（硬性）

CLI 与 api.md 保持同步。**新增或修改功能时两处同步**：

| 改动 | 位置 |
|---|---|
| CLI 子命令 | `app/cmd/litkit/*.go` |
| 接口契约 | `.harness/specs/reference/api.md`（§1 命令） |

CI 的接口一致性检查（`.harness/constraints/sync`）机械化强制 CLI 命令清单与 api.md 一致，遗漏会直接红。

## 本地门禁

```bash
# 全量门禁（7 项：gofmt → build → lint → vet → test → vulncheck → arch-check + sync）
powershell -File .harness/constraints/gate.ps1    # Windows
bash .harness/constraints/gate.sh                 # Linux/macOS

# 快速预检（门禁真子集）
cd app && gofmt -l . && go test ./...
```

## 约定速览

- **分层**：入口层 → 服务层 → 适配层 → 叶子层，单向；`internal/model` 纯净，不 import 上层。
- **源抽象**：新学术源实现 `PaperSource` 接口并注册，禁止绕过注册表。
- **密钥**：禁止硬编码 API key，一律经 `internal/config` 读取（`.env`，gitignored）。
- **摘要工作流**：不下载 PDF、不抽取全文；检索源必须提供摘要，无摘要论文默认过滤。
- **测试**：表驱动；触网测试放 `app/tests/integration/`（`-tags integration`，手动跑，不进 CI）。
- **注释**：中文；导出类型/函数必须有 godoc 注释；只解释"为什么"。

## 环境

- Go ≥ 1.26；golangci-lint、govulncheck、goreleaser（发布）。
- 触网测试依赖网络与外部 API（arxiv / PubMed 等），本地手动执行。

## 报告问题

- 描述复现步骤、期望行为与实际行为、相关命令与输出。
- 若涉及接口，注明与 api.md 接口文档是否一致。
