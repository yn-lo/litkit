# 数据模型参考 — litkit

---
last_updated: 2026-08-02
status: active
owner: litkit-core
---

> Paper / SearchResult / PaperSource 的 Go 类型定义与去重/入库键约定。
> 与接口契约（JSON 形态）见 [`api.md`](api.md)；分层归属见 [`../architecture/boundaries.md`](../architecture/boundaries.md)。

## 1. Paper（核心载体，最底层）

```go
package model

// Author 作者
type Author struct {
    Given  string `json:"given,omitempty"`
    Family string `json:"family,omitempty"`
}

// Paper 论文元数据（摘要工作流核心载体）
type Paper struct {
    ID        string   `json:"id"`        // 内部稳定唯一 id（hash）
    CiteKey   string   `json:"citeKey,omitempty"` // 3 字母引用标识，入库时分配（FR-LIB-06）
    Title     string   `json:"title"`
    Authors   []Author `json:"authors"`
    Abstract  string   `json:"abstract"`  // 检索源必须提供；空串 = 无摘要（检索默认过滤）
    Year      int      `json:"year"`      // 0 表示未知
    Venue     string   `json:"venue"`
    DOI       string   `json:"doi"`
    PMID      string   `json:"pmid"`
    ArXivID   string   `json:"arxivId"`
    URL       string   `json:"url"`
    Source    string   `json:"source"`    // 来源平台标识
    DocType   string   `json:"docType"`   // 文献类型：article/preprint/dataset/...（GB/T 7714—2025）
    Citations int      `json:"citations"` // 源提供时
}
```

> 约定：可选字段用零值（空串 / 0）表示"不可用"，JSON 输出中空串等价于 null 语义。

## 2. SearchResult / SourceError

```go
// SearchResult 跨源检索结果
type SearchResult struct {
    Total          int                `json:"total"`
    SourceResults map[string][]Paper `json:"sourceResults"`
    Errors         []SourceError      `json:"errors"`
    Papers         []Paper            `json:"papers"` // 去重合并后
}

// SourceError 单源失败记录
type SourceError struct {
    Source string `json:"source"`
    Error  string `json:"error"`
}
```

## 3. 去重键优先级

```
DOI（非空）> Title（归一化）> ID（source:externalId）
```

## 4. 入库键与引用标识（internal/storage）

文献库以 SQLite 存储（`WORK_DIR/litkit.db`），schema 由 `schema/schema.sql` 统一管理。

```
dedup_key（入库唯一键，FR-LIB-01）：
    DOI（小写，非空）> Title（小写）+ Authors（小写）> Paper.ID
cite_key（引用标识，FR-LIB-06）：
    3 字母 a-zA-Z 唯一；入库自动分配；重复检索保持不变；AI 引用与引用标记唯一入口
```

`paper_refs` 表记录引用标记（FR-LIB-07）：`(cite_key, sentence_hash, manuscript)` 唯一，
同句重复引用幂等；`sentence_hash` 为引用句的 sha256 前缀指纹。

## 5. PaperSource 接口

```go
// PaperSource 学术源统一抽象
type PaperSource interface {
    // Name 源标识（如 "arxiv"、"crossref"）
    Name() string
    // Search 检索（ctx 控制超时与取消）
    Search(ctx context.Context, query string, opts SearchOptions) ([]model.Paper, error)
    // HasAbstract 该源是否提供摘要
    HasAbstract() bool
}

// SearchOptions 检索参数（远程检索仅 keyword 模式）
type SearchOptions struct {
    MaxResults int
    Year       int // 0 表示不过滤
}
```

## 6. 规则

- `internal/model` 不 import 任何上层包（C6 数据模型纯净，由 [`../../constraints/arch/`](../../constraints/arch/) 强制）
- 新增字段必须同步更新 [`api.md`](api.md) 的 JSON 示例与 `litkit sources` 输出
- 文献类型 `DocType` 取值参考 GB/T 7714—2025（article / preprint / dataset / book / chapter / ...）
