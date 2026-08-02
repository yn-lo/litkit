# 功能设计：检索与文献平台连接

---
last_updated: 2026-08-02
status: active
owner: litkit-core
---

对应 PRD：FR-SRC、FR-SEARCH、FR-CACHE

## 目标

单次调用并发跨多个国内可达学术平台检索，输出标准化元数据 + 摘要，结果去重合并，缓存复用。新源可插拔。

## 范围

### 包含
- `PaperSource` 接口 + 源注册表（CLI 与 MCP 共用）
- 默认 5 个一期源（均含摘要）：arXiv、PubMed、OpenAlex、Semantic Scholar、bioRxiv/medRxiv
- 跨源并发检索、单源失败隔离、DOI→title→id 三级去重
- 每源令牌桶限速（≥10 次/分钟）+ 429/503 指数退避重试
- 搜索缓存（MD5 键 + 24h TTL + cache list/clear）

### 不包含
- 二期源（Zenodo、IEEE/ACM）本期不实现
- 移除源（CORE、DOAJ、OpenAIRE、Unpaywall、Europe PMC/PMC、HAL、BASE、SSRN/CiteSeerX、dblp）不实现
- 无摘要源直接不实现（FR-SRC-19）；CrossRef 检索仅作元数据反查（见 feature-references）
- 远程语义重排（`search --mode semantic`）：语义能力仅限本地文献库（FR-LIB-05），一期不实现
- Google Scholar / Sci-Hub（明确移除）
- PDF 下载与全文抽取（摘要工作流）

## 分层设计

### internal/model（叶子层）
`Paper` 数据载体（含 Abstract/DOI/PMID/ArXivID/DocType），`SearchResult`、`SourceError`、`CacheEntry`。

### internal/sources（适配层）
- `source.go`：`PaperSource` 接口 + 缓存/降级/限速公共逻辑（源基类）
- `registry.go`：源注册表，CLI 与 MCP 共用（FR-SRC-18、FR-IFACE-03）
- 每源一文件：`arxiv.go`、`pubmed.go`、`crossref.go`、`openalex.go` 等

### internal/core（服务层）
`search.go`：并发检索编排、去重、缓存读写；`cache.go`：TTL 缓存管理。

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
- [ ] 缓存测试：命中零网络 IO；TTL 过期重新检索
- [ ] 限速测试：并发扇出不超每源上限（NFR-PERF-04）
