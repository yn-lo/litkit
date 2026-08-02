# 功能设计：引用与手稿处理

---
last_updated: 2026-08-02
status: active
owner: litkit-core
---

对应 PRD：FR-REF

## 目标

解析手稿中的引用占位符，按标识符取回元数据（含摘要），生成 GB/T 7714—2025（中英文混排）/ APA / IEEE / BibTeX / RIS，产出格式化手稿与参考文献。

## 范围

### 包含
- 占位符解析：`[@doi:]` / `[@pmid:]` / `[@arxiv:]` / `[@title:]`（FR-REF-01）
- 按标识符取元数据：DOI→CrossRef/OpenAlex，PMID→PubMed，arXiv→Atom，title→CrossRef（FR-REF-02）
- 引用渲染：zh→GB/T 7714—2025（含预印本[PP]/数据集[DS]类型）、en→APA 7th/IEEE（FR-REF-03/04）
- BibTeX / RIS 生成（FR-REF-05/06）
- `manuscript` 完整流水线：formatted.md + refs.bib + refs.ris + references.txt +（可选）docx（FR-REF-08）
- 未解析占位符归入 unresolved（FR-REF-09）
- `export_references` 批量导出（FR-REF-10）
- Pandoc docx 可选，缺失优雅降级（FR-REF-11）

### 不包含
- PDF 下载与全文抽取（摘要工作流）
- 中文文献库（知网/万方）检索

## 分层设计

### internal/model
`Paper` 承载元数据（引用渲染输入）。

### internal/core
- `metadata.go`：标识符 → Paper 反查
- `references.go`：占位符解析、内置格式化器（GB/T 7714—2025/APA/IEEE）、BibTeX/RIS 渲染
- `manuscript.go`：手稿流水线（Pandoc 可选）

### 入口层
`litkit metadata` / `litkit manuscript` / `litkit export` + `get_paper_metadata` / `process_manuscript` / `export_references` MCP 工具。

## 关键规则/约束

- 中文模式支持中英文文献混排著录（FR-REF-03）
- 引用标准 GB/T 7714—2025（2026-07-01 实施），支持新文献类型（C3）
- 未解析占位符不静默丢失（FR-REF-09）
- 3 种核心样式内置格式化器原生实现；5 个 .csl 经 Pandoc（可选）渲染（FR-REF-07）
- Pandoc 缺失仅 docx 不可用，其余产物正常（FR-REF-11）

## 测试要求

- [ ] 占位符解析测试：位置与全文匹配（FR-REF-01）
- [ ] 内置格式化器测试：GB/T 7714 中英文混排样例；APA/IEEE 编号列表
- [ ] BibTeX/RIS 生成测试：字段完整性（author/title/year/journal/volume/number/pages/doi/url）
- [ ] unresolved 测试：未解析占位符出现在列表
- [ ] Pandoc 缺失降级测试：跳过 docx，其余产物正常
- [ ] CLI 与 MCP 一致性测试（FR-IFACE-03）
