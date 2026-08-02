# 功能设计：检索与文献平台连接

---
last_updated: 2026-08-02
status: active
owner: litkit-core
---

对应 PRD：FR-SRC、FR-SEARCH、FR-LIB（结果入库）

## 目标

单次调用并发跨多个国内可达学术平台检索，输出精简元数据 + 摘要（FR-IFACE-04），结果去重合并、年份倒序，自动入库本地文献库。新源可插拔。

## 范围

### 包含
- `PaperSource` 接口 + 源注册表（CLI 与 MCP 共用）
- 默认 5 个一期源（均含摘要）：arXiv、PubMed、OpenAlex、Semantic Scholar、bioRxiv/medRxiv
- 跨源并发检索、单源失败隔离、DOI→title→id 三级去重
- 每源令牌桶限速（≥10 次/分钟）+ 429/503 指数退避重试
- 检索结果去重后 upsert 进本地文献库（FR-LIB-01），回填 cite_key（FR-LIB-06）；无独立缓存层
- 结果默认年份倒序（FR-SEARCH-10）：最新在前，year=0 排末尾
- 默认输出 PaperSummary（citeKey/title/firstAuthor/year/abstract，FR-IFACE-04）；`--full` 输出完整 Paper

### 不包含
- 二期源（Zenodo、IEEE/ACM）本期不实现
- 移除源（CORE、DOAJ、OpenAIRE、Unpaywall、Europe PMC/PMC、HAL、BASE、SSRN/CiteSeerX、dblp）不实现
- 无摘要源直接不实现（FR-SRC-19）；CrossRef 检索仅作元数据反查（见 feature-references）
- 远程语义重排（`search --mode semantic`）：语义能力仅限本地文献库（FR-LIB-05），一期不实现
- Google Scholar / Sci-Hub（明确移除）
- PDF 下载与全文抽取（摘要工作流）

## 分层设计

### internal/model（叶子层）
`Paper` 数据载体（含 Abstract/DOI/PMID/ArXivID/DocType/CiteKey），`SearchResult`、`SourceError`。

### internal/sources（适配层）
- `source.go`：`PaperSource` 接口 + 降级/限速公共逻辑（源基类）
- `registry.go`：源注册表，CLI 与 MCP 共用（FR-SRC-18、FR-IFACE-03）
- 每源一文件：`arxiv.go`、`pubmed.go`、`crossref.go`、`openalex.go` 等

### internal/core（服务层）
`search.go`：并发检索编排、去重、结果入库（FR-LIB-01）。

### internal/storage（叶子层）
SQLite 文献库：upsert/去重/引用标记；schema 以 `.sql` 文件管理（见 feature-cache.md）。

### 入口层
`litkit search` 子命令 + `search_papers` / `search_<source>` MCP 工具。

## 关键规则/约束

- 所有源必须实现 `PaperSource` 接口，禁止绕过注册表（C7）
- 检索源必须提供摘要（`HasAbstract()`=true），无摘要源不纳入检索（FR-SRC-19）
- 单源失败归入 errors，不中断整体（FR-SEARCH-01）
- 去重优先级：DOI（非空）> Title（归一化）> ID（source:externalId）
- 每源限速器取合规限速保守值（arXiv ≥3s/次）
- 检索结果必须含摘要：无摘要论文默认过滤（`--keep-no-abstract` 显式保留）（FR-SEARCH-03）
- 新增源不修改核心层（FR-SRC-18）

## 测试要求

- [ ] 每源单元测试：httptest mock，覆盖解析正确性与错误分支
- [ ] 每源触网测试（integration tag）：可达性 + 解析正确性
- [ ] 去重测试：同文跨源合并为一条
- [ ] 单源失败隔离测试：一个源挂掉不影响其他
- [ ] 无摘要过滤测试：无摘要论文默认不出现；`--keep-no-abstract` 时保留（FR-SEARCH-03）
- [ ] 入库测试：检索结果 upsert 进本地库并回填 cite_key；无摘要不入库（FR-LIB-01/06）
- [ ] 排序测试：结果按年份倒序；year=0 排末尾（FR-SEARCH-10）
- [ ] PaperSummary 测试：默认输出 5 字段；firstAuthor 为 "Family Given"；`--full` 输出完整字段（FR-IFACE-04）
- [ ] 检索等级测试：arXiv `ti:`+`abs:`、PubMed `[Title/Abstract]`+`[Keyword]`；`--mode full` 回退全文（FR-SEARCH-12）
- [ ] 时间范围测试：`--since`/`--years` 换算；arXiv/bioRxiv 本地过滤、PubMed/OpenAlex 服务端过滤（FR-SEARCH-13）
- [ ] 限速测试：并发扇出不超每源上限（NFR-PERF-04）
