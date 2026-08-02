# 功能设计：缓存与本地文献库

---
last_updated: 2026-08-02
status: active
owner: litkit-core
---

对应 PRD：FR-CACHE、FR-LIB

## 目标

搜索缓存避免重复网络 IO；本地 SQLite 文献库存储元数据与摘要（含入库摘要约束）。

## 范围

### 包含
- 搜索结果自动缓存（FR-CACHE-01）：命中且未过 TTL 直接返回
- 缓存键确定性（FR-CACHE-02）：同参数命中同条目（MD5 hash(query|source|params)）
- TTL 默认 24h 可覆盖（FR-CACHE-03）
- 缓存目录默认 `WORK_DIR/.litkit_cache`（FR-CACHE-04）
- `cache list` / `cache clear`（FR-CACHE-05）
- SQLite 文献库：入库元数据必须含摘要（FR-LIB-01、C9）；增删查接口（FR-LIB-02）；库文件 `WORK_DIR/litkit.db`（FR-LIB-03）
- 本地 keyword 检索（FR-LIB-04）：SQLite FTS5 + trigram 中文分词，中文文献按关键词/标题/作者命中
- 本地语义检索（FR-LIB-05）：跨语言——中文 query 可命中英文文献；文献导入时生成 embedding 落库，可重建

### 不包含
- 文献管理数据库（Zotero/Mendeley 替代品）

## 分层设计

### internal/core
`cache.go`：TTL 缓存读写、list/clear；`library.go`：文献库增删查。

### internal/storage（叶子层）
SQLite 封装（modernc.org/sqlite，纯 Go 无 CGO）：元数据 + 摘要 + 向量表（paper_id, vector BLOB）。

### internal/embedding（叶子层）
Provider 抽象：`Embed(texts []string) ([]Vector, error)`；local（纯 Go 推理）/ api（阿里百炼 / 硅基流动）；默认 local 零 key 零网络；仅服务本地库语义检索（FR-LIB-05）。

### 入口层
`litkit cache` / `litkit library` + `cache_list` / `cache_clear` MCP 工具。

## 关键规则/约束

- 缓存损坏不影响运行（NFR-REL-03）：读取/加载容错
- 入库元数据必须含摘要（C9）：摘要缺失拒绝入库或标记
- 缓存目录随项目迁移（FR-CACHE-04）
- 远程检索仅 keyword（FR-SEARCH-07）；语义能力集中在本地文献库
- embedding 默认本地零 key 零网络；配置 API key 自动切换（C10）

## 测试要求

- [ ] 缓存命中零网络 IO（<100ms）（NFR-PERF-02）
- [ ] 缓存键确定性：同参数命中同条目
- [ ] TTL 过期重新检索
- [ ] 缓存损坏容错：坏文件不 panic
- [ ] 文献库：入库含摘要约束、增删查、库文件位置跟随 WORK_DIR
- [ ] 本地 keyword：中文文献按关键词/标题/作者命中（FR-LIB-04）
- [ ] 本地语义：中文 query 命中英文文献；embedding 导入时生成、可重建（FR-LIB-05）
