# 安全约束 — litkit

---
last_updated: 2026-08-02
status: active
owner: litkit-core
---

## 规则

| 约束 | 执行工具 | 说明 |
|---|---|---|
| 禁止硬编码 API key | 静态扫描 + CI（NFR-SEC-01） | 一律经 `internal/config` 读取，写入 `.env` |
| `.env` 不入库 | .gitignore（NFR-SEC-02） | 密钥文件禁止提交 |
| 依赖漏洞扫描 | `govulncheck`（NFR-SEC-03） | CI 运行；本地 `govulncheck ./...` |

## 本地运行

```bash
go install golang.org/x/vuln/cmd/govulncheck@latest
govulncheck ./...
```

## 说明

- govulncheck 是命令型工具，无独立配置文件；扫描范围 = 当前模块依赖
- CI 中的安全扫描步骤见 `.harness/constraints/ci/ci.yml` 的 `security` job
- 新增依赖时在本地先跑一次 govulncheck 再提交
