# 错误码参考 — litkit

---
last_updated: 2026-08-02
status: active
owner: litkit-core
---

## CLI 退出码

| 退出码 | 含义 |
|---|---|
| 0 | 成功 |
| 1 | 运行错误 |
| 2 | 参数错误 |
| 3 | 部分源失败但部分成功 |

## 单源失败记录（SourceError）

单源失败不中断整体，归入 `SearchResult.Errors`：

```json
{ "source": "pubmed", "error": "timeout" }
```

## 验证问题（VerifyIssue 三要素）

```json
{
  "ruleId": "R3-2",
  "problem": "中英文之间缺少空格",
  "suggestion": "在中文与英文/数字之间插入一个空格",
  "location": { "line": 12, "column": 8 },
  "severity": "error"
}
```

## 规则

- 新增业务错误必须在本文件登记
- 新增错误码必须在测试中覆盖对应错误分支
- 内部错误统一 `%w` 包装上下文（见 [`../conventions/error-handling.md`](../conventions/error-handling.md)）
