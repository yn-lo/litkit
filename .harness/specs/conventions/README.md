# 约定总览 — litkit

---
last_updated: 2026-08-02
status: active
owner: litkit-core
---

本目录是 litkit 的编码约定（规格层"为什么"）。每条约定的可执行版本在约束层：

| 约定 | 规格层文档 | 约束层执行 |
|---|---|---|
| 命名规范 | [`naming.md`](naming.md) | golangci-lint（revive/nolintlint） |
| 错误处理 | [`error-handling.md`](error-handling.md) | errcheck / go vet / review |
| 测试策略 | [`testing.md`](testing.md) | go test / coverage 门禁 / build tag |
| 工具链 / CI 门禁 / 文档 / 提交 / 开发环境 | [`process.md`](process.md) | gofmt / golangci-lint / govulncheck / CI |
