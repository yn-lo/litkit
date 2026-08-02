# 错误处理 — litkit

---
last_updated: 2026-08-02
status: active
owner: litkit-core
---

## 规则

- 返回 `error`，用 `%w` 包装上下文（如 `fmt.Errorf("search arxiv: %w", err)`）
- **禁止吞掉 error**：`_ = err` 只允许在有明确理由处
- **禁止用 panic 做流程控制**：panic 仅用于不可恢复的程序错误；网络/解析/业务错误一律返回 error
- 单源失败**不中断整体**：归入 `SearchResult.Errors`（`SourceError{Source, Error}`）
- 所有网络调用接受 `context.Context`，支持超时与取消；上游 429/503 由重试层处理
- 可选字段用零值（空串 / 0）表示"不可用"，JSON 输出中空串等价于 null 语义

## 背景与意图

- **Go 无异常机制**：调用方显式处理错误是唯一路径，吞错会导致静默失败（尤其多源检索时一个源挂了要能看到）
- **单源失败隔离**：国内网络波动下整体可用（NFR-REL-02）——这是设计决策，不是妥协
- **零值约定**：`Paper.Abstract` 空串表示"源无摘要"，与 PRD A6 假设一致，避免 `nil` 穿透 JSON 序列化

## 错误码参考

→ 见 [`../reference/error-codes.md`](../reference/error-codes.md)
