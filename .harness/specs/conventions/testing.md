# 测试策略 — litkit

---
last_updated: 2026-08-02
status: active
owner: litkit-core
---

## 测试分层

| 类别 | 位置 | 要求 |
|---|---|---|
| 单元测试 | 各包 `*_test.go` | 离线 hermetic；mock 所有网络 IO（httptest）；CI 只跑此层 |
| 触网测试 | `app/tests/integration/`（build tag `integration`，待二期补充） | 真实 API；`go test -tags integration ./tests/integration/`；不进 CI 默认流程 |
| 一致性测试 | 双接口 | CLI 与 MCP 同输入同输出断言（FR-IFACE-03） |

## 规则

- **表驱动测试（table-driven）为默认风格**
- 覆盖率仅计 unit 测试，门禁 ≥ 60%（NFR-MAINT-03）
- 新增源应附触网测试（探测可达性 + 解析正确性），放 `app/tests/integration/`（当前待二期补充）
- 任何在 `init`/`TestMain` 阶段触网的测试不得放入 unit
- 覆盖率门禁用 floor 而非 ceiling，随测试基建成熟度单调上调

## 背景与意图

- **离线 hermetic 是 CI 的前提**：CI 环境无稳定外网，触网测试会让 CI 随机红
- **一致性测试守护 C8**：CLI 与 MCP 共享核心，行为分叉即破坏接口一致性约束

## 具体门禁由约束层执行

→ 见 `.harness/constraints/ci/ci.yml` 与 `.harness/constraints/style/.golangci.yml`
