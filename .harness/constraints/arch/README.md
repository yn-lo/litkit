# 架构约束检查 — litkit

分层依赖方向检查程序（独立 Go 模块，不进主项目构建）。

## 检查规则

依赖方向单向：**入口层 → 服务层 → 适配层 → 叶子层**，禁止反向。

| 层 | 目录 | 允许依赖 |
|---|---|---|
| 入口层 | `internal/mcp` · `cmd/` | 服务层 / 适配层 / 叶子层 |
| 服务层 | `internal/core` | 适配层 / 叶子层 |
| 适配层 | `internal/sources` | 叶子层 |
| 叶子层 | `internal/model` · `config` · `storage` · `util` · `embedding` | 仅叶子层 |

额外规则：

- `internal/model` 不 import 任何上层（数据模型纯净，PRD C6）
- 新增目录必须在本检查程序与 `boundaries.md` 同步登记

## 运行

```bash
# 在 arch 目录内运行（独立 go.mod，避免与主模块嵌套冲突）
cd .harness/constraints/arch
go run .
```

> 注：主项目 go.mod 存在后，此目录因含独立 go.mod 会被 `go run ./...` 排除，属预期行为。CI 中在独立 job 运行。

## 输出原则

- 全部通过：**零输出**，退出码 0
- 存在违规：只输出错误行 `文件:位置 — 说明 + 文档引用`，退出码 1

## 集成

- 本地预检：`cd .harness/constraints/arch && go run .`
- CI：见 `.harness/constraints/ci/ci.yml` 的 `arch-check` 步骤
