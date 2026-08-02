# 命名规范 — litkit

---
last_updated: 2026-08-02
status: active
owner: litkit-core
---

## 规则

| 标识符类型 | 约定 | 示例 |
|---|---|---|
| 导出标识符（类型/函数/字段） | PascalCase | `PaperSource`、`SearchResult` |
| 非导出标识符 | camelCase | `cacheKey`、`dedupeKey` |
| 包名 | 单词小写，简短 | `core`、`sources`、`model` |
| 常量 | 按 Go 惯例（驼峰，不强制全大写） | `defaultTTL`、`MaxResults` |
| 接口名 | 动词或名词 + `er` | `PaperSource` |
| 单文件多类型 | 每类型独立 `_test.go` | `paper_test.go` |

## 背景与意图

- **驼峰而非 snake_case**：Go 标准库与生态惯例，工具链（gofmt/goimports）默认支持
- **包名简短**：调用处 `sources.Arxiv` 而非 `sources_pkg.Arxiv`，减少冗余
- **命名跟随领域语言**：`Paper` / `SearchResult` / `SourceError` 与 PRD 术语一一对应，AI 可直接对照 FR 编写

## 具体规则由约束层执行

→ 见 `.harness/constraints/style/.golangci.yml`
