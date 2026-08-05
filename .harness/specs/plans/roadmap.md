# 开发计划 — litkit

---
last_updated: 2026-08-02
status: active
owner: litkit-core
---

> 里程碑按依赖顺序排列。每个里程碑有明确范围、产出与验收标准（对应 PRD 的 FR 编号）。

## M1 项目骨架

**范围**：工程初始化、类型与工具链、CLI 框架、配置、数据模型。

| 任务 | 对应 FR |
|---|---|
| Go ≥ 1.26 工程初始化（go.mod / golangci-lint / go test / goreleaser） | NFR-MAINT-01 |
| CLI 框架（cobra 子命令骨架，`--help` 自描述） | FR-IFACE-01 |
| config 模块（.env 发现、密钥读取、LITKIT_ 前缀） | FR-CONFIG-01/02/03 |
| models：Paper / Author / SearchResult | C5 数据模型纯净 |
| utils：网络（超时/重试）、日志、文本工具 | NFR-PERF-03 |

**产出**：可运行 `litkit --help`；CI 门禁（gofmt/golangci-lint/go vet/go test/coverage）就绪。

**验收**：`litkit --help` 输出全部子命令；`litkit sources` 返回空源表（待 M2 注册）。

## M2 检索与本地文献库

**范围**：PaperSource 抽象、首批源适配、跨源检索与去重、结果入库（SQLite 文献库）。

| 任务 | 对应 FR |
|---|---|
| internal/sources/source.go：PaperSource 接口 + 降级公共逻辑 | FR-SRC-01 |
| internal/util/ratelimit：每源令牌桶（x/time/rate）+ 429/503 指数退避重试 | NFR-PERF-04、NFR-REL-01/04 |
| internal/sources/registry.go：源注册表（CLI 与 MCP 共用） | FR-SRC-18、FR-IFACE-03 |
| 源适配：arXiv、PubMed、OpenAlex | FR-SRC-02/03/06 |
| 源适配：bioRxiv/medRxiv、Semantic Scholar | FR-SRC-04/05 |
| core/search：并发检索、单源失败隔离、三级去重、无摘要过滤 | FR-SEARCH-01/02/03/06 |
| internal/storage：SQLite 文献库（schema/*.sql、dedup upsert、cite_key、paper_refs） | FR-LIB-01/02/03/06/07 |
| core/search 入库回填 cite_key；`litkit search` / `litkit lib` / `litkit sources` | FR-LIB-01/06、FR-SEARCH-03/04 |

**产出**：`litkit search "关键词"` 返回跨源去重结果并自动入库；`litkit lib list` 可查看引用标识。

**验收**：对 5 个默认源逐一实测（触网），平台能力矩阵与实测一致（PRD 第 8 章）；每源连续 10 次检索无 429/封禁（NFR-PERF-04）。

## M3 元数据与引用（已完成）

**范围**：占位符解析、元数据反查、引用渲染、手稿流水线。

| 任务 | 对应 FR |
|---|---|
| core/metadata：doi/pmid/arxiv/title → Paper | FR-REF-01/02 |
| 引用渲染：内置格式化器（GB/T 7714—2025 / APA / IEEE，原生实现；CSL 留 P1） | FR-REF-03/04/07 |
| BibTeX / RIS 生成 | FR-REF-05/06 |
| core/manuscript：`[@citeKey]` 占位符解析、citation_map、unresolved | FR-REF-08/09 |
| Pandoc docx 可选转换与降级 | FR-REF-11 |
| `litkit metadata` / `litkit manuscript` / `litkit export` | FR-REF-10 |
| core/library：SQLite 入库（含摘要）、增删查 | FR-LIB-01/02/03 |

**产出**：`litkit manuscript draft.md --lang zh` 产出 formatted.md + refs.bib + refs.ris + references.txt。

**验收**：GB/T 7714 中英文混排样例通过；未解析占位符出现在 unresolved。

## M4 撰写约束（lint / harness）

**范围**：.litkit 模板、事前指导（AGENTS.md 撰写硬性规定）、verify 命令。

| 任务 | 对应 FR |
|---|---|
| internal/lint：ManuscriptSpec 解析/校验、.litkit 生成、RenderWritingRules（事前指导） | FR-LINT-01/07/10 |
| templates/：rules.md（R0-R9，langs 标注）、checklist.md、manuscript-spec.yaml、verifier_models.json | FR-LINT-01/06 |
| `litkit init`：--type review/empirical（preset 阈值）、--lang zh/en、--journal NAME（目标期刊）、--refresh、--force | FR-LINT-01/09 |
| zh 规则集实现（全半角/引号/句式冗余/的地得/AI 痕迹） | FR-LINT-02/04 |
| en 规则集实现（语法/时态/冠词/措辞/AI 痕迹） | FR-LINT-03/04 |
| `litkit verify`：--lang/--type/--mode/--rule/--skip，19 条规则（16A+3S），三维过滤（lang x type x mode），三值 exitHint | FR-LINT-05 |

> 已落地（M4 一期前段）：internal/lint 服务层 + 四件套模板 + init 全参数（含 AGENTS.md 撰写硬性规定）。
> 已落地（M4 一期后段）：verify 规则函数注册表（A/S/M 分类执行，规则单套按 langs x types 过滤）；
> 纯函数 lint.Run() 无 IO，CLI 薄壳；模式递增 chapter→draft→final；Markdown 分段排除代码块/参考文献/表格。

**产出**：`litkit init` 在宿主项目生成 `.litkit/` + AGENTS.md 撰写硬性规定（事前指导）；`litkit verify` 输出三要素 issues（事后兜底）。

**验收**：zh/en 模式各自违规样例全部被检出；违规项含 rule_id/problem/suggestion；改 yaml 后 `init --refresh` 同步 AGENTS.md。

## M5 MCP 接口（已完成）

**范围**：MCP Server、工具注册、与 CLI 共享核心。

| 任务 | 对应 FR |
|---|---|
| internal/mcp/：mcp-go SDK stdio Server | FR-IFACE-02 |
| 工具绑定：search_papers / get_paper_metadata / process_manuscript / export_references | FR-IFACE-02 |
| 工具绑定：lint_init / verify_manuscript / search_<source> / lib_list / lib_search / lib_rm / lib_stats / lib_path | FR-IFACE-02 |
| 一致性测试：CLI 与 MCP 同输入同输出 | FR-IFACE-03 |

> 已落地：internal/mcp 包注册全部 11 个静态工具 + 按注册表动态生成 search_<source>；
> `litkit mcp` 子命令接线（stdio）；MCP 工具直接调用 core/lint/storage 共享核心；
> 一致性测试经内存传输验证工具清单与离线工具输出（export_references / verify_manuscript / lint_init / process_manuscript 等）。

**产出**：MCP Server 可被 Claude Desktop / Trae 发现并调用全部工具。

**验收**：客户端调用 search_papers 与 CLI search 输出一致。

## M6 发布（已完成）

**范围**：打包、CI 门禁完善、文档与示例。

| 任务 | 对应 FR |
|---|---|
| 打包发布（goreleaser 跨平台单一二进制） | — |
| CI：gofmt / golangci-lint / go vet / go test / coverage / govulncheck | NFR-MAINT-02/03、NFR-SEC-03 |
| 文档核对（断链检查）、.env.example | FR-CONFIG-03 |
| 示例与快速开始完善 | — |
| 开源发布（Apache-2.0、README/CONTRIBUTING） | C12 |

> 已落地：app/.goreleaser.yml（linux/darwin/windows × amd64/arm64，版本经 ldflags 注入
> internal/buildinfo，snapshot 构建验证通过）；CI 部署 .github/workflows/ci.yml（替换
> registration/link 两个占位为 .harness/constraints/sync 检查器，docs job 补 setup-go）；
> sync 检查器（CLI/api.md §1、MCP/api.md §2 三处清单一致 + CLAUDE.md/.harness 断链检查，
> 已并入本地 gate 第 8 项）；README / LICENSE（Apache-2.0）/ CONTRIBUTING；api.md 补 `litkit mcp` 条目。

**产出**：可安装、CI 全绿、文档一致。

**验收**：`goreleaser build --snapshot` 产出 6 平台二进制且版本注入正确；sync 检查器全绿。

## M7 语义检索与二期源（二期）

**范围**：embedding 基础设施、本地库双模式检索（远程语义重排不实现）；二期源适配（dblp、Zenodo、IEEE/ACM）。

| 任务 | 对应 FR |
|---|---|
| internal/embedding/：Provider 抽象（local / api）+ 向量存储 | FR-SEARCH-09 |
| 本地模型集成 POC：goformer vs go-semantica 选型实测 | FR-SEARCH-09 |
| 本地库 FTS5 中文分词（trigram） | FR-LIB-04 |
| 本地库语义检索（导入时生成 embedding，跨语言） | FR-LIB-05 |
| API 模式接入（阿里百炼 / 硅基流动）+ 环境变量 | FR-SEARCH-09 |
| 二期源适配：Zenodo、IEEE/ACM | FR-SRC-07/08 |

**产出**：`litkit library search --mode semantic` 中文 query 命中英文文献；`litkit library search --mode keyword` 中文词法命中。

**验收**：中文 query 检索本地英文文献命中（FR-LIB-05）；本地库万级规模检索 < 1s。

## M8 LLM 引用相关性评分（三期）

**范围**：FR-LINT-08 落地——LLM 对文稿中引用文献的句子与该文献内容的相关性评分。

| 任务 | 对应 FR |
|---|---|
| 评分服务抽象：Scorer 接口（multi-model 交叉打分 + 增量缓存） | FR-LINT-08 |
| 多模型组合 POC：开源模型（本地推理）vs API 模型选型与阈值标定 | FR-LINT-08 |
| 文稿引用句抽取 + 文献摘要对齐输入构造 | FR-LINT-08 |
| 评分结果接入 verify 报告（新增 S 类规则项） | FR-LINT-04/05 |
| 配置项落地：模型组合 / 阈值 / 缓存策略 | FR-LINT-07 |

**产出**：`litkit verify --rule citation-relevance` 输出引用句-文献相关性评分与建议。

**验收**：多模型评分一致率达标；增量缓存命中率 ≥80%（避免重复打分）；阈值经人工标注集校准。

## 优先级速览

| 里程碑 | PRD 覆盖 | 依赖 | 阶段 |
|---|---|---|---|
| M1 骨架 | 接口/配置/模型 | 无 | 一期 |
| M2 检索 | FR-SRC / FR-SEARCH / FR-LIB | M1 | 一期 |
| M3 引用 | FR-REF / FR-LIB | M2 | 一期 |
| M4 lint | FR-LINT（除 08） | M1 | 一期 |
| M5 MCP | FR-IFACE | M2/M3 | 一期 |
| M6 发布 | NFR | M3/M4 | 一期 |
| M7 语义检索与二期源 | FR-SEARCH-09、FR-LIB-04/05、FR-SRC-07/08 | M2/M3 | 二期 |
| M8 LLM 引用评分 | FR-LINT-08 | M4/M7 | 三期 |
