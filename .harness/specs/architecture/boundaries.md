# 分层架构 — litkit

---
last_updated: 2026-08-02
status: active
owner: litkit-core
---

## 设计意图

litkit 采用四层单向依赖架构，核心目的是：

1. **入口层与业务解耦**：CLI 只做参数解析与注册，业务逻辑全部下沉 `internal/core`。未来加 Web/UI 入口时，只需新增一个入口适配器，不动核心。
2. **学术源可插拔**：所有平台适配集中在 `internal/sources`，统一实现 `PaperSource` 接口。新增一个学术源不需要修改核心层——这是 G5"可扩展"目标的架构保证。
3. **叶子层纯净**：数据模型 `internal/model` 是最底层依赖，不 import 任何上层。模型稳定 → 所有上层依赖稳定。

## 依赖方向

```
入口层（cmd/litkit）
   ↓
服务层（internal/core）
   ↓
适配层（internal/sources）
   ↓
叶子层（internal/model · config · storage · util · embedding）
```

所有箭头单向，**禁止反向依赖**。`internal/` 天然隔离外部引用，架构检查工具强制层间方向（见 `.harness/constraints/arch/`）。

## 各层职责

### 入口层（cmd/litkit）
- CLI 子命令参数解析与绑定
- 不承载任何业务逻辑，不做数据加工
- 经 `internal/sources/registry.go` 使用源注册表

### 服务层（internal/core）
- 业务流程编排：跨源并发检索、去重合并、本地库双模式检索（keyword/semantic）、元数据反查、引用渲染、手稿流水线、约束验证、缓存、文献库
- 唯一的事务与业务编排点

### 适配层（internal/sources）
- 每个学术源一个适配器，统一实现 `PaperSource` 接口
- 所有源复用同一缓存 / 降级 / 限速公共逻辑（源基类）
- 禁止反向 import 入口层或服务层

### 叶子层（internal/model · config · storage · util · embedding）
- 数据模型、配置读取（.env 发现与密钥）、SQLite 存储、embedding provider（本地模型 / 可选 API）与向量存储、网络/文本工具
- 不依赖任何上层

## 关键规则（由约束层代码强制执行）

- 分层依赖方向单向：见 `.harness/constraints/arch/`
- 新源只放适配目录 + 注册表，不修改核心层：注册同步由架构检查防遗漏
- `internal/model` 不 import 任何上层包

## 设计推理

- **为什么不用三层（入口/业务/数据）**：学术源适配的多样性与易变需求（20+ 平台，各不同的 API/限速/摘要能力）值得独立一层，否则服务层会因平台差异膨胀。
- **为什么模型在最底层**：Paper/SearchResult 被所有层引用，任何对模型的重构都会波及全项目。把它钉在最底层并禁止反向依赖，是控制变更半径的手段。
