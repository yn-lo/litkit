# 工具链与流程约定 — litkit

---
last_updated: 2026-08-02
status: active
owner: litkit-core
---

> 本文件覆盖命名/错误处理/测试之外的工具链、CI 门禁、文档、提交与开发环境约定。
> 命名见 [`naming.md`](naming.md)，错误处理见 [`error-handling.md`](error-handling.md)，测试见 [`testing.md`](testing.md)。

## 1. 语言与工具链

| 项 | 约定 |
|---|---|
| 语言 | Go ≥ 1.26 |
| 格式化 | gofmt + goimports，CI 强制（`gofmt -l .` 输出为空） |
| 静态检查 | golangci-lint（含 govet / staticcheck / errcheck / unused 等） |
| 测试 | go test（表驱动测试） |
| 覆盖率 | `go test -cover` ≥ 60%，CI 强制 |
| 构建 | `go build ./cmd/litkit`；发布经 goreleaser |

## 2. 分层与编码总则

- **分层依赖**：入口层 → 服务层 → 适配层 → 叶子层，单向；`internal/` 天然隔离外部引用。详见 [`../architecture/boundaries.md`](../architecture/boundaries.md)
- **数据模型纯净**：`internal/model/` 不 import 任何上层包
- **源抽象**：所有学术源必须实现 `PaperSource` 接口，禁止绕过注册表
- **注册同步**：新增功能必须在 CLI 与 MCP 两处注册，机械化检查防遗漏
- **禁止硬编码密钥**：一律经 `internal/config` 读取，写入 `.env`（gitignored）
- **错误处理**：见 [`error-handling.md`](error-handling.md)
- **上下文**：所有网络调用接受 `context.Context`，支持超时与取消
- **注释**：中文注释；不写冗余注释；复杂逻辑必须解释"为什么"；导出类型/函数必须有 godoc 注释

## 3. CI 门禁

```
gofmt / goimports   (格式检查)
golangci-lint       (静态分析)
go vet              (编译器级检查)
go test ./...       (单元测试，离线)
go test -cover      (覆盖率 ≥ 60%)
arch-check          (分层依赖 + 注册同步)
govulncheck         (依赖漏洞扫描)
```

CI 配置见 [`../../constraints/ci/ci.yml`](../../constraints/ci/ci.yml)；lint 配置见 [`../../constraints/style/.golangci.yml`](../../constraints/style/.golangci.yml)。

## 4. 文档规范

- 文档即知识地图：只指路，不重复细节
- 任何引用必须真实存在（禁止断链）；CI 提供断链检查
- 文档声明与实现必须一致（例如：PRD 声称的 CI 步骤必须在 CI 中真实存在）
- 需求文档为唯一权威基线；新增能力先改 PRD 再实现
- 所有规格层文档使用 YAML front matter（`last_updated` / `status` / `owner`），Doc Freshness CI 检查过期

## 5. 提交规范

- 提交信息：`<type>: <subject>`，type ∈ feat/fix/refactor/docs/test/chore
- 不提交：`.env`、密钥、缓存目录、本地库文件（gitignore 覆盖）
- 不可逆操作：禁止 force push 主分支

## 6. 开发环境准备

```bash
# Go 版本确认
go version  # ≥ 1.26

# 安装工具链
go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest
go install golang.org/x/vuln/cmd/govulncheck@latest

# 本地验证（等价 CI 门禁）
gofmt -l . && golangci-lint run && go vet ./... && go test ./... -cover

# 触网测试（手动）
go test -tags integration ./tests/integration/

# 构建
go build -o litkit ./cmd/litkit

# 跨平台构建（goreleaser）
goreleaser build --snapshot
```
