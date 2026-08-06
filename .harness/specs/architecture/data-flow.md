# 数据流 — litkit

---
last_updated: 2026-08-02
status: active
owner: litkit-core
---

本文档描述关键数据从入口到存储的完整路径，让 AI 理解数据如何流转。

## 1. 检索数据流（search）

```
CLI → core.Search(ctx, ...)
  1. 归一化参数（query、sources、maxResults、year）
  2. 生成缓存键 hash(query|source|params)，查缓存 → 命中即返回（零网络 IO）
  3. errgroup 并发调用各源 adapter.Search(ctx, ...)
  4. 每个源返回 []Paper；单源失败 → 归入 errors，不中断
  5. 合并 → 按 DOI → title → id 三级 key 去重
  6. 写缓存（TTL 24h）
  7. 返回 SearchResult{Total, SourceResults, Errors, Papers}
```

## 2. 引用生成数据流（manuscript）

```
1. 解析占位符 [@doi:...] / [@pmid:...] / [@arxiv:...] / [@title:...]
2. 每个占位符经 core.Metadata(ctx, ...) 取元数据（含摘要）
   doi → CrossRef / OpenAlex；pmid → PubMed；arxiv → arXiv Atom；title → CrossRef
3. 生成 citationMap（占位符 → 条目）
4. 按 lang/style 渲染：zh → GB/T 7714—2025（内置格式化器，含预印本[PP]/数据集[DS]类型）
                              en → APA 7th / IEEE（内置格式化器）
5. 输出 BibTeX / RIS / references.txt
6. 可选：经 Pandoc + CSL 生成 docx（缺失则跳过）
7. 未解析占位符归入 unresolved（不静默丢失）
```

## 3. 约束验证数据流（verify）

```
1. 按 --lang 选择规则集（zh/en）
2. 按 --mode 决定启用规则范围（chapter/draft/final 递增）
3. 对文稿逐条执行规则验证（A 自动 / S 半自动）
4. 输出 []Issue：{RuleID, Problem, Suggestion, Location}（三要素）
5. M 类规则输出至 checklist 供人工复核
```

## 4. 每源限速与重试

```
1. 每个源注册一个令牌桶限速器（golang.org/x/time/rate），速率取该源合规限速的保守值
   （arXiv ≥3s/次；其余按官方限速换算为每分钟上限）
2. 检索扇出前先经限速器取令牌；未取到则等待（ctx 取消可中断）
3. 429/503 → 指数退避 + 随机抖动重试；重试耗尽归入 errors，不阻塞整体
4. 可选 key（Semantic Scholar / PubMed）自动切换到更高速率档位
5. 并发被限速器收敛：即使单次调用扇出 5 源 × 多次检索，每源实际请求率 ≤ 合规上限
```

目标：单源 ≥ 10 次/分钟稳定不限速（NFR-PERF-04），突发被平滑、不触发上游 429 或 IP 封禁。

## 5. 限速参考（国内可达性视角）

| 源 | 官方限速 | 对 10 次/分钟结论 |
|---|---|---|
| PubMed E-utilities | 无 key 3 req/s（180/分）；有 key 10 req/s | 无 key 富余 |
| CrossRef | polite pool 50 req/s | 富余（元数据反查） |
| OpenAlex | 10 req/s（10 万/天） | 富余 |
| arXiv | 官方 1 req/3s（20/分）；2026-02 起并发下频繁 429 | 10/分在限内，需间隔 + 退避 |
| Semantic Scholar | 无 key 共享池（~1000 req/s 全用户分摊 + ~100 req/5分）；有 key 保底 1 RPS | 无 key 勉强，建议可选 key |
| bioRxiv / medRxiv | 无硬性公布限速 | 富余 |

## 6. 本地文献库检索数据流（library search）

```
core.LibrarySearch(ctx, query, mode)：
  模式 keyword：SQLite FTS5（trigram 中文分词）→ 词法命中
  模式 semantic：query embedding（internal/embedding Provider：local / api）→ SQLite 同库向量余弦 → 跨语言语义命中
  embedding 在文献导入时生成并随元数据落库（paper_id, vector BLOB）；重导入触发重建
  远程检索保持 keyword（FR-SEARCH-07），语义能力集中在本地文献库（FR-LIB-05）
```
