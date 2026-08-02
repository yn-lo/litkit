# 功能设计：本地文献库与结果持久化

---
last_updated: 2026-08-02
status: active
owner: litkit-core
---

对应 PRD：FR-LIB、FR-CACHE（已并入 FR-LIB）

## 目标

检索结果直接持久化进本地 SQLite 文献库（含摘要约束、cite_key 引用标识），
不做独立 JSON 缓存、不设 TTL：库生命周期由工作目录决定，删除工作目录即删除库。

## 范围

### 包含
- 检索结果自动 upsert 入库（FR-LIB-01）：去重后带摘要论文入库，重复检索按 dedup_key 更新
- 去重键确定性（原 FR-CACHE-02）：`dedup_key` = DOI > title+authors > paper_id
- 库文件 `WORK_DIR/litkit.db`（FR-LIB-03）：随工作目录迁移，无 TTL
- 引用标识 cite_key（FR-LIB-06）：3 字母 a-zA-Z 唯一，入库自动分配；AI 引用与引用标记唯一入口
- 引用标记 paper_refs（FR-LIB-07）：记录"哪句话引用哪篇文献"（cite_key + 句子指纹 + 手稿）；同句重复引用幂等
- 增删查接口（FR-LIB-02）：`litkit lib list | search | rm | stats | path`
- 本地 keyword 检索（FR-LIB-04）：M1 为 LIKE 检索（标题/作者/摘要）；FTS5+中文分词二期
- 本地语义检索（FR-LIB-05）：二期（跨语言 embedding）

### 不包含
- 文献管理数据库（Zotero/Mendeley 替代品）
- 独立缓存层（JSON 文件 / TTL）——已删除

## 分层设计

### internal/storage（叶子层）
`store.go`：modernc.org/sqlite（纯 Go 无 CGO）封装——upsert/去重/查询/删除/引用标记；
schema 以 `schema/schema.sql` 单文件管理，`//go:embed` 在 Open 时执行（幂等）。
`citekey.go`：3 字母 cite_key 生成（随机 + 查库去重，空间耗尽升 4/5）。

### internal/core
`search.go`：检索结果去重过滤后调用 store.UpsertPaper 入库并回填 cite_key；
入库失败不阻断检索（透明降级）。

### 入口层
`litkit lib` 子命令 + `lib_list` / `lib_search` / `lib_rm` / `lib_stats` / `lib_path` MCP 工具。

## 关键规则/约束

- 入库元数据必须含摘要（C9 / FR-LIB-01）：无摘要论文不入库
- 库生命周期由工作目录决定（FR-LIB-03）：无 TTL、无 cache clear
- dedup_key 决定同一篇论文（跨源去重语义入库，FR-SEARCH-02）
- cite_key 唯一且稳定：论文被重复检索时保持不变
- 远程检索仅 keyword（FR-SEARCH-07）；语义能力集中在本地文献库

## 测试要求

- [x] 去重 upsert：同 DOI / 同 title+authors / 同 paper_id 不重复入库，cite_key 复用
- [x] cite_key：3 字母 a-zA-Z、唯一
- [x] 无摘要论文不入库（FR-LIB-01）
- [x] 增删查：lib list / lib search / lib rm；删除级联清理引用标记
- [x] 引用标记：同句重复引用幂等；引用不存在的 cite_key 被外键拒绝
- [x] 库文件跟随 WORK_DIR（FR-LIB-03）；**测试固化目录：`e:\Codes\litkit\workspace`（litkit.db 落于此）**
- [ ] 本地 keyword：中文文献按关键词/标题/作者命中（FR-LIB-04，FTS5 二期）
- [ ] 本地语义：中文 query 命中英文文献；embedding 导入时生成、可重建（FR-LIB-05）
