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

## 说明

- 密钥只经 `internal/config` 读取，配置走 `.env`（gitignored）
- 新增依赖时人工确认来源可信并定期更新依赖版本（不引入自动漏洞扫描门禁）
