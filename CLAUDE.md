# CLAUDE.md — litkit

> 本项目的知识地图（指向"去哪找"）和重点开发规范。

## 项目概述

国内学术医学写作场景的论文工具包：跨源检索摘要、生成规范引用（GB/T 7714—2025 / APA / IEEE）、排版手稿、AI 撰写合规门禁。Go 1.26 / cobra / modernc.org/sqlite；CLI 唯一接口。

**AI-first 定位**：工具输出面向 AI agent 调用，接口设计以**降低上下文噪声**为第一约束——默认返回 AI 写作所需最小字段集（citeKey/title/firstAuthor/year/abstract），完整元数据落 SQLite 由 citeKey 句柄按需取回；`--full` 逃生口供人类调试。

## 知识导航

| 你需要… | 去这里 |
|---|---|
| 需求基线（PRD） | [`.harness/specs/requirements/PRD.md`](.harness/specs/requirements/PRD.md) |
| 架构（概览 / 分层意图 / 数据流） | [`.harness/specs/architecture/`](.harness/specs/architecture/) |
| 功能设计与范围 | [`.harness/specs/design/feature-*.md`](.harness/specs/design/) |
| 编码约定（命名 / 错误 / 测试 / 工具链） | [`.harness/specs/conventions/`](.harness/specs/conventions/) |
| 接口 / 数据模型 / 错误码 / 平台矩阵 | [`.harness/specs/reference/`](.harness/specs/reference/) |
| 开发计划与里程碑 | [`.harness/specs/plans/roadmap.md`](.harness/specs/plans/roadmap.md) |
| 架构约束 / Lint 配置 / CI 门禁 | [`.harness/constraints/`](.harness/constraints/) |

## 构建与验证

Go 源码位于 `app/` 子目录。**单一入口**跑完构建 + 全量门禁（8 项）：

```bash
# 构建全量门禁（一步跑完 8 项：gofmt → build → lint → vet → test → vulncheck → arch-check → sync）
.harness/constraints/gate.ps1        # Windows PowerShell
bash .harness/constraints/gate.sh     # Linux/macOS

# 快速预检（门禁真子集，在 app/ 执行）
cd app && gofmt -l . && go test ./...

# 触网测试（手动，不进 CI，在 app/ 执行）
cd app && go test -tags integration ./tests/integration/

# 发布（在 app/ 执行）
cd app && goreleaser build --snapshot
```

## 硬性规则

- **密钥**：禁止硬编码 API key，一律经 `internal/config` 读取，配置走 `.env`（gitignored）
- **分层**：入口层 → 服务层 → 适配层 → 叶子层，单向，架构检查强制；`internal/model` 不 import 上层包
- **源抽象**：所有学术源必须实现 `PaperSource` 接口并注册，禁止绕过注册表
- **接口同步**：CLI 与 api.md 保持同步（由 `constraints/sync` 检查器强制）
- **摘要工作流**：不下载 PDF、不抽取全文；检索源必须提供摘要（无摘要源不纳入，FR-SRC-19），检索结果无摘要论文默认过滤（FR-SEARCH-03）；入库元数据必须含摘要
- **不可逆操作**：禁止 force push 主分支
- **输出**：代码中严禁使用 emoji 提示
- **测试目录**：如果要进行真实环境的测试，默认将.exe文件放在本项目workspace目录

# Ponytail, lazy senior dev mode

Lazy = efficient, not careless. Best code is the code never written.

## Before writing code, stop at the first rung that holds

1. Need to build at all? (YAGNI)
2. Already in this codebase? Reuse it, don't re-write.
3. Stdlib does it? Use it.
4. Native platform feature covers it? Use it.
5. Installed dependency solves it? Use it.
6. Can be one line? Make it one line.
7. Only then: write the minimum that works.

**Climb only after reading the task and tracing the real flow end-to-end.** Small diff you don't understand isn't lazy, it's a second bug.

**Bug fix = root cause, not symptom.** Grep every caller; fix the shared function once. Patching only the named path leaves siblings broken.

## Rules

- No abstractions not explicitly requested
- No new dependency if avoidable
- No unasked boilerplate
- Deletion > addition. Boring > clever. Fewest files possible
- Shortest working diff wins — but only after understanding the problem
- Question complex requests: "Need X, or does Y cover it?"
- Two equal stdlib approaches → pick edge-case-correct (lazy = less code, not flimsier)
- Deliberate corner-cuts (global lock / O(n²) / naive heuristic) marked with `ponytail:` comment naming ceiling + upgrade path

## Not lazy about

Understanding the problem · trust-boundary input validation · data-loss-preventing error handling · security · accessibility · hardware calibration (clock drift, sensor offset) · anything explicitly requested.

Non-trivial logic leaves ONE runnable check behind — smallest thing that fails if logic breaks (assert demo / one tiny test; no frameworks). Trivial one-liners need no test.

## 关键需求/节点使用TDD开发