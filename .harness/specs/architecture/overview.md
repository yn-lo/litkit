# 架构概览 — litkit

---
last_updated: 2026-08-02
status: active
owner: litkit-core
---

## 技术栈

| 层 | 选型 |
|---|---|
| 语言 | Go ≥ 1.26（静态类型、单一二进制、原生并发） |
| CLI | spf13/cobra |
| MCP | modelcontextprotocol/go-sdk（stdio 传输） |
| HTTP / XML | net/http、encoding/xml（标准库） |
| HTML（可选源） | goquery |
| 引用渲染 | 内置格式化器（GB/T 7714—2025 / APA / IEEE）+ Pandoc CSL（可选） |
| 存储 | modernc.org/sqlite（纯 Go，无 CGO） |
| 语义检索（仅本地文献库） | embedding provider 抽象（本地纯 Go 推理 / 可选国内 API）+ SQLite 同库向量存储 |
| .env | joho/godotenv |
| 并发/限速 | goroutine + errgroup + golang.org/x/time/rate |
| 模板嵌入 | go:embed（lint 模板 + CSL 文件编译进二进制） |
| docx | Pandoc（外部二进制，可选） |
| 构建/发布 | goreleaser（跨平台单一二进制） |

## 模块划分

四层单向依赖架构，详见 [`boundaries.md`](boundaries.md)：

```
cmd/litkit · internal/mcp       入口层（参数解析与注册，不含业务逻辑）
internal/core                   服务层（检索去重 / 本地库双模式检索 / 元数据反查 / 引用渲染 / 手稿流水线 / lint / 缓存 / 文献库）
internal/sources                适配层（每个学术源一个适配器，实现 PaperSource 接口）
internal/model · config · storage · util · embedding   叶子层（数据模型 / 配置 / SQLite / embedding / 工具）
```

## 部署拓扑

- **单一二进制分发**：模板与 CSL 样式经 `go:embed` 编译进二进制，用户无需安装运行时或依赖
- **三个入口共享同一核心**：`litkit` CLI（第一接口）、MCP Server（可选第二接口）、未来可扩展 HTTP/Web（入口层加适配器即可）
- **数据落盘**：`WORK_DIR/litkit.db`（本地文献库）、`WORK_DIR/.litkit_cache`（搜索缓存）、`WORK_DIR` 下输出文件

## 关键约束

- 依赖方向单向，禁止反向（架构检查强制）
- 数据模型纯净：`internal/model` 不依赖任何上层
- 新源仅放适配目录 + 注册，不改核心层

## 详见

- 分层意图：[`boundaries.md`](boundaries.md)
- 数据流：[`data-flow.md`](data-flow.md)
- 需求基线：[`../requirements/PRD.md`](../requirements/PRD.md)
