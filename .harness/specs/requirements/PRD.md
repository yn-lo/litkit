# litkit 需求文档（PRD）

---
last_updated: 2026-08-02
status: active
owner: litkit-core
---

| 字段 | 值 |
|---|---|
| 版本 | v1.0 |
| 日期 | 2026-08-02 |
| 状态 | 基线（面向实现） |
| 定位 | 唯一权威需求基线，描述系统"做什么"与验收标准 |

---

## 1. 项目概述

litkit 是一个面向**国内学术写作场景**的论文工具包：检索文献（摘要级工作流）、生成规范引用、排版手稿、并强制约束 AI 撰写内容的学术规范性。以 **CLI** 为第一接口、**MCP Server** 为可选第二接口对外服务。

核心价值主张：

- **中文优先**：内置中文（GB/T 7714）与英文（APA/IEEE）两套写作模式，支持中英文文献混排著录
- **摘要工作流**：基于检索返回的元数据与摘要工作，不下载 PDF、不抽取全文
- **统一检索**：单次调用跨多个国内可达学术平台检索，结果标准化、去重合并
- **双模式检索**：本地文献库支持 keyword（FTS5+中文分词）与语义（跨语言，本地 embedding）双模式；远程检索保持 keyword 原生排序
- **AI 友好**：CLI 输出 JSON 可被 AI shell 调用；MCP 可被客户端发现调用
- **撰写合规门禁**：将论文撰写规范机械化为可执行验证，AI 输出违规即验证失败
- **免费优先**：核心源全部基于公开 API，无订阅依赖

## 2. 背景与目标

### 2.1 背景

研究者使用 AI 辅助撰写论文时面临三个割裂问题：

1. **检索割裂**：文献分散在多个平台，多数需订阅或国内不可达，缺乏统一入口
2. **引用割裂**：AI 生成的参考文献格式错误率高，中文文献的 GB/T 7714 著录尤其混乱
3. **合规割裂**：AI 撰写内容缺乏"审稿人视角"的机械校验（格式、结构、引用、AI 痕迹）

### 2.2 目标

| 编号 | 目标 | 度量 |
|---|---|---|
| G1 | 统一检索 | 单次调用并发查询 ≥5 个国内可达学术源，输出含摘要，结果去重 |
| G2 | 中文优先 | zh/en 双写作模式；GB/T 7714 支持中英文文献混排著录 |
| G3 | AI 友好 | CLI 输出 JSON；MCP 工具可被主流客户端发现 |
| G4 | 撰写合规门禁 | 规则集按 zh/en 分设，验证失败输出三要素 |
| G5 | 可扩展 | 新增学术源 ≤1 个适配文件 + 2 处注册 |
| G6 | 可维护 | 静态类型检查、单元测试、覆盖率门禁机械化 |

### 2.3 非目标

- 中文文献数据库检索（知网 / 万方 / 维普无开放 API）
- 远程全库语义检索（不预计算全库向量；远程检索保持 keyword 原生排序）
- 文献管理数据库（Zotero/Mendeley 替代品）
- PDF 阅读器 UI / 论文浏览器 / 引文图谱可视化
- 商业数据库订阅访问（IEEE/ACM 仅提供可选适配骨架）

## 3. 用户画像与典型场景

| 用户 | 主要场景 | 关键诉求 |
|---|---|---|
| 中文研究者 | 撰写中文论文，引用中英文文献 | GB/T 7714 著录、Word 输出、中文规范校验 |
| 英文写作研究者 | 撰写英文论文 | APA/IEEE 引用、英文规范校验 |
| AI 辅助写作用户 | 让 Claude/Trae 自动检索并写入论文 | 工具稳定、输出结构化、可验证 |
| 工具开发者 | 集成到自有流水线 | CLI 可脚本化、JSON 输出、接口稳定 |

**场景 A — 中文论文撰写门禁**：宿主项目初始化 lint 基础设施 → Agent 撰写章节 → 运行验证命令（zh 模式）→ 违规项按"问题/修复/规则编号"自动修复 → 产出符合 GB/T 7714 与中文排版规范的手稿。

**场景 B — 引用回填**：手稿中 `[@doi:...]` / `[@pmid:...]` 占位符自动取回元数据与摘要，生成中英文混排参考文献列表。

**场景 C — CLI 批处理**：`litkit search "图神经网络" -s crossref,arxiv -n 50 > refs.json`，导入 Zotero。

**场景 D — 手稿闭环**：Markdown 草稿 → 解析占位符 → 生成 formatted.md + refs.bib + refs.ris +（可选）docx。

## 4. 功能需求

> 编号规则：`FR-<域>-<序号>`。优先级：P0 必须 / P1 重要 / P2 可选 / P3 三期。
> 验收标准以"可运行命令 + 可断言输出"表述，与实现语言无关。

### 4.1 FR-SRC 文献平台连接

**一期默认启用（国内可达、稳定、大源）**：

| ID | 需求 | 优先级 | 验收标准 |
|---|---|---|---|
| FR-SRC-01 | 统一源抽象：`PaperSource` 等价接口（search / search_with_cache） | P0 | 新源只需实现 search |
| FR-SRC-02 | arXiv（Atom Feed，含摘要） | P0 | `litkit search "x" -s arxiv -n 5` 返回 ≥1 篇含 abstract |
| FR-SRC-03 | PubMed（NCBI EUtils XML，含摘要） | P0 | 同上 |
| FR-SRC-04 | bioRxiv / medRxiv（REST JSON，含摘要） | P0 | 两源均返回结果 |
| FR-SRC-05 | Semantic Scholar（Graph API，含摘要） | P0 | 无 key 可用；403 自动降级匿名 |
| FR-SRC-06 | OpenAlex（元数据主干，含摘要） | P0 | 按 DOI 与按关键词均可查 |

> **检索源摘要门槛**：无摘要能力（`HasAbstract()`=false）的源直接不实现（如 dblp）；部分提供摘要的源可保留（如 IEEE/ACM），检索时剔除无摘要论文（FR-SEARCH-03）。CrossRef 无摘要，仅作元数据反查源（FR-REF-02），不参与检索。

**二期（可选激活）**：

| ID | 需求 | 优先级 | 验收标准 |
|---|---|---|---|
| FR-SRC-07 | Zenodo（机构仓储，含摘要） | P1 | 返回结果 |
| FR-SRC-08 | IEEE / ACM（需 API key，部分摘要） | P2 | 无 key 不注册；有 key 注册 search；检索时剔除无摘要论文 |

**明确移除（不实现）**：

| ID | 需求 | 说明 |
|---|---|---|
| FR-SRC-09 | CORE / DOAJ / OpenAIRE | 数据被 OpenAlex 完整索引，适配收益低 |
| FR-SRC-10 | Unpaywall | OA 反查非检索源，与摘要工作流不匹配 |
| FR-SRC-11 | Europe PMC / PMC | 与 PubMed 数据重合 |
| FR-SRC-12 | HAL | 小众机构仓储，与 Zenodo 重合 |
| FR-SRC-13 | BASE（OAI-PMH） | 需机构 IP，协议老旧 |
| FR-SRC-14 | SSRN / CiteSeerX | 端点不稳定 |
| FR-SRC-15 | Google Scholar | 国内不可达，需代理；不作为内置源 |
| FR-SRC-16 | Sci-Hub | 法律风险；不作检索源。仅作全文下载兜底（FR-FETCH-03），默认开启、失败静默降级 |
| FR-SRC-17 | dblp | 无摘要，直接不实现 |

| ID | 需求 | 优先级 | 验收标准 |
|---|---|---|---|
| FR-SRC-18 | 新增源不修改核心层 | P0 | 新源仅放适配目录 + 注册，依赖方向由架构检查强制 |
| FR-SRC-19 | 检索源必须提供摘要 | P0 | `HasAbstract()=false` 的源不实现；部分摘要源检索时剔除无摘要论文 |

### 4.2 FR-SEARCH 统一检索（摘要工作流）

| ID | 需求 | 优先级 | 验收标准 |
|---|---|---|---|
| FR-SEARCH-01 | `search` 跨源并发检索 | P0 | 多源并发，单源失败不影响其他，错误归入 errors 字段 |
| FR-SEARCH-02 | 结果按 DOI→title→id 三级去重 | P0 | 同文跨源合并为一条 |
| FR-SEARCH-03 | 检索结果必须含摘要 | P0 | 无摘要论文默认过滤；`--keep-no-abstract` 可显式保留 |
| FR-SEARCH-04 | 支持 sources / max_results_per_source / year 参数 | P0 | year 参数语义按源能力文档化 |
| FR-SEARCH-05 | 每源独立 `search_<source>` 工具 | P1 | 默认源均有独立定向工具 |
| FR-SEARCH-06 | 输出标准化字段集 | P0 | 字段集固定；列表字段可序列化往返无损 |
| FR-SEARCH-07 | `search` 统一接口（keyword 模式） | P0 | 远程检索仅 keyword；semantic 模式仅限本地文献库（FR-LIB-05） |
| FR-SEARCH-08 | 远程语义重排（keyword top-K → 本地重排） | 不包含（二期可评估） | 远程检索保持 keyword 原生排序；语义能力集中在本地文献库，避免 top-K 召回局限 |
| FR-SEARCH-10 | 结果默认年份倒序 | P0 | 最新在前；year=0 排末尾。各源原始顺序语义不一，跨源混合后必须显式排序 |
| FR-SEARCH-11 | 检索词语言约束 | P0 | 检索词必须为英文（各源英文语料为主，中文命中率极低）；CLI help 与 api.md 显式声明 |
| FR-SEARCH-12 | 检索等级 | P0 | 默认 tiab=题目+摘要+关键词（源支持时）；`--mode full` 全文作高级选项；由 AGENTS.md 告知 AI 结果不足时的升级路径 |
| FR-SEARCH-13 | 默认时间范围 | P0 | 默认最近 3 年（`LITKIT_DEFAULT_RECENT_YEARS`）；`--years N` 放宽或 `--since YEAR` 显式起始年；0=不限 |
| FR-SEARCH-09 | embedding 基础设施（provider 抽象 + 本地向量库） | P1 | 仅服务本地库语义检索（FR-LIB-05）；默认本地模型零 key 零网络；配置 API key 自动切换；向量存 SQLite 同库；本地库规模（万级）检索 < 1s |

### 4.3 FR-FETCH 全文获取（可选能力，非摘要工作流默认路径）

> 摘要工作流仍为默认（C2）；全文获取为**按需可选能力**，由 `litkit fetch` / `fetch_paper` 显式触发，不改变检索入库行为。

| ID | 需求 | 优先级 | 验收标准 |
|---|---|---|---|
| FR-FETCH-01 | 按 cite_key 或 DOI 取回论文全文 | P0 | `litkit fetch <cite_key\|doi>` 下载 PDF → 抽取文本 → 全文缓存入库；返回 { citeKey, pdfPath, fulltext } |
| FR-FETCH-02 | OA 优先：Unpaywall 按 DOI 解析 OA PDF | P0 | 需 `LITKIT_UNPAYWALL_EMAIL`；解析到最佳 OA PDF URL 并下载；无 email 或无 OA 时跳过并记录 |
| FR-FETCH-03 | Sci-Hub 兜底（默认开启，失败静默） | P1 | Unpaywall 失败后尝试 Sci-Hub（`LITKIT_SCI_HUB_URL` 可配，默认 sci-hub.se）；不存在/下载失败仅记录，不报错中断；Scan 版 PDF 无文本层，抽取返回空并提示 |
| FR-FETCH-04 | PDF 落盘 + 全文缓存双层存储 | P0 | PDF 存 `WORK_DIR/downloads/<citeKey>.pdf`；抽取文本存 `papers.fulltext` 列（按 dedup_key 缓存，避免重复抽取）；库中已缓存全文时直接返回不重复下载 |
| FR-FETCH-05 | PDF 文本抽取（纯 Go） | P0 | 用 pdfcpu 抽取；支持扫描版检测（无文本层返回空）；抽取失败不删除已落盘 PDF |

### 4.4 FR-REF 引用与手稿处理

| ID | 需求 | 优先级 | 验收标准 |
|---|---|---|---|
| FR-REF-01 | 解析占位符：裸 `[@citeKey]`（查本地库）或 `[@doi:]` / `[@pmid:]` / `[@arxiv:]` / `[@title:]` | P0 | 返回位置与全文匹配；未解析归入 unresolved 不静默丢失 |
| FR-REF-02 | 按标识符取元数据（DOI→CrossRef，PMID→NCBI，arXiv→Atom，title→CrossRef） | P0 | 返回完整字段（含摘要，源提供时） |
| FR-REF-03 | 中文模式引用样式：GB/T 7714—2025 | P0 | 支持中英文文献混排著录；支持预印本[PP]、数据集[DS]等新文献类型；未知样式报错 |
| FR-REF-04 | 英文模式引用样式：APA 7th / IEEE | P0 | 编号列表输出 |
| FR-REF-05 | 生成 BibTeX | P0 | `@article{...}` 含 author/title/year/journal/volume/number/pages/doi/url |
| FR-REF-06 | 生成 RIS | P0 | 兼容 Zotero/Mendeley/EndNote |
| FR-REF-07 | 内置 CSL 样式经 citeproc 引擎渲染 | P1 | 内置 5 个 .csl；核心样式（GB/T 7714 / APA / IEEE）优先走原生格式化器，CSL 仅用于原生未覆盖样式；`--docx` 按 style 选用 |
| FR-REF-08 | `manuscript` 完整流水线 | P0 | Markdown → formatted.md + refs.bib + refs.ris + references.txt +（可选）docx |
| FR-REF-09 | 未解析占位符归入 unresolved 列表 | P1 | 不静默丢失 |
| FR-REF-10 | `export_references` 批量导出 | P1 | bibtex/ris/text 三格式 |
| FR-REF-11 | docx 转换依赖 Pandoc，缺失时优雅降级 | P1 | 跳过 docx，其余产物正常 |

### 4.4 FR-LINT 论文撰写约束验证

| ID | 需求 | 优先级 | 验收标准 |
|---|---|---|---|
| FR-LINT-01 | `litkit init` 初始化宿主项目撰写约束 | P0 | 生成 `.litkit/`（rules.md / checklist.md / specs/manuscript-spec.yaml / verifier_models.json，go:embed 编译进二进制）并渲染 AGENTS.md「撰写硬性规定」段；支持 `--type review\|empirical`（preset 阈值切换）、`--lang zh\|en`、`--journal NAME`（目标期刊，写入 spec）、`--refresh`（按现有 yaml 重渲染）、`--force`；交互式向导（stdin 是终端时） |
| FR-LINT-02 | zh 专属规则 | P0 | 覆盖：全半角标点、中文引号、句式冗余（"进行""通过…使"）、"的/地/得"、"了/着/过"、AI 痕迹；规则标注 langs: zh |
| FR-LINT-03 | en 专属规则 | P0 | 覆盖：语法一致性、时态、冠词/单复数、学术措辞、AI 痕迹；规则标注 langs: en |
| FR-LINT-04 | 规则体系结构 | P0 | 每条规则含：定义、违规示例、验证方法（A 自动 / S 半自动 / M 人工）、langs 标注、types 标注（空=全部类型；如 empirical 仅实证论文触发）；**规则代码单套按 langs x types 过滤，不设多套系统** |
| FR-LINT-05 | `verify` 自动验证命令 | P0 | 支持 `--lang zh\|en`、`--type review\|empirical`（空=从 spec 自动取）、`--rule`、`--mode`（chapter/draft/final）；三维过滤（lang x type x mode）；报错含三要素 |
| FR-LINT-06 | 人工审查清单 checklist.md | P1 | 覆盖 M 类规则 |
| FR-LINT-07 | 可变标准配置 manuscript-spec.yaml | P1 | 字数、引用数、章节、标题层级、引用样式阈值可配置；改后 `litkit init --refresh` 同步 AGENTS.md |
| FR-LINT-08 | 引用相关性 LLM 评分 | P3（三期） | LLM 对文稿中引用文献的句子与该文献内容的相关性评分；多模型交叉打分 + 增量缓存避免重复验证。本期仅做接口预留与配置占位，不实现 |
| FR-LINT-09 | `lint init` 引导终端运行 verify | P1 | lint init 返回的 next_steps 指引终端命令；MCP `verify_manuscript` 同步可用 |
| FR-LINT-10 | 事前指导（撰写硬性规定） | P0 | AGENTS.md 携带由 manuscript-spec.yaml 渲染的精简祈使句段落（非 yaml 数据复制），AI 写稿时自动遵守，事后 verify 兜底 |

### 4.5 FR-LIB 本地文献库

| ID | 需求 | 优先级 | 验收标准 |
|---|---|---|---|
| FR-LIB-01 | SQLite 存储论文元数据与摘要 | P0 | 入库元数据必须含摘要；检索结果自动 upsert 入库 |
| FR-LIB-02 | 增删查接口（CLI + MCP） | P1 | 可按 DOI/title/关键词查询；`lib list` / `lib rm` |
| FR-LIB-03 | 库文件位置跟随工作目录 | P0 | WORK_DIR/litkit.db；删除工作目录即删除库（无 TTL）。**未设置 LITKIT_WORK_DIR 时拒绝执行（errNoWorkDir），不退化为 CWD，避免污染任意目录**。**测试固化目录：`e:\Codes\litkit\workspace`** |
| FR-LIB-04 | 本地 keyword 检索（FTS5 + 中文分词） | P1 | M1 为 LIKE 检索（标题/作者/摘要）；FTS5+分词二期 |
| FR-LIB-05 | 本地语义检索（跨语言） | P1 | 中文 query 可命中英文文献（导入时生成 embedding）；嵌入信息可重建 |
| FR-LIB-06 | 引用标识 cite_key | P0 | 3 字母 a-zA-Z 唯一；入库自动分配；AI 引用与引用标记的唯一入口 |
| FR-LIB-07 | 引用标记 paper_refs | P1 | 记录"哪句话引用哪篇文献"（cite_key + 句子指纹 + 手稿）；同句重复引用幂等 |

### 4.6 FR-CACHE 缓存（已并入 FR-LIB）

搜索结果不再使用独立 JSON 缓存与 TTL：检索结果直接 upsert 进本地文献库（SQLite），
库生命周期由工作目录决定（删除工作目录即删除库）。原 FR-CACHE 需求去向：

| 原需求 | 去向 |
|---|---|
| FR-CACHE-01 搜索结果自动缓存 | FR-LIB-01（检索结果自动入库，重复检索按 dedup_key 去重更新） |
| FR-CACHE-02 缓存键确定性 | dedup_key（DOI > title+authors > paper_id） |
| FR-CACHE-03 默认 TTL 24h 可覆盖 | 删除（无 TTL） |
| FR-CACHE-04 缓存目录 WORK_DIR/.litkit_cache | FR-LIB-03（WORK_DIR/litkit.db） |
| FR-CACHE-05 `cache list` / `cache clear` | `litkit lib list` / `litkit lib rm <cite_key>` |

### 4.7 FR-IFACE 接口

| ID | 需求 | 优先级 | 验收标准 |
|---|---|---|---|
| FR-IFACE-01 | CLI 为第一接口 | P0 | 全部功能可经 CLI 完成；输出 JSON |
| FR-IFACE-02 | MCP Server 为可选第二接口 | P0 | stdio 传输，注册全部工具；客户端可发现调用 |
| FR-IFACE-03 | CLI 与 MCP 共享同一核心实现 | P0 | 新增功能两处注册（机械化检查防止遗漏） |
| FR-IFACE-04 | 输出精简（AI-first） | P0 | search/lib 默认返回 PaperSummary（citeKey/title/firstAuthor/year/abstract）；`--full` 返回完整元数据。降低 AI agent 上下文噪声 |

### 4.8 FR-CONFIG 配置

| ID | 需求 | 优先级 | 验收标准 |
|---|---|---|---|
| FR-CONFIG-01 | 所有密钥经统一配置入口读取，禁止硬编码 | P0 | 静态扫描强制 |
| FR-CONFIG-02 | .env 自动发现（ENV_FILE > WORK_DIR > CWD > 项目根） | P0 | 按序查找 |
| FR-CONFIG-03 | 提供 .env.example 模板 | P0 | 列出全部可选 key |
| FR-CONFIG-04 | 写作语言模式默认 zh | P0 | 可用 --lang 覆盖 |

## 5. 非功能需求

### 5.1 NFR-PERF 性能

| ID | 需求 | 度量 |
|---|---|---|
| NFR-PERF-01 | 多源搜索并发执行 | 总耗时 ≈ 最慢源 |
| NFR-PERF-02 | 检索结果入库 | 单次检索去重入库 < 100ms 附加耗时 |
| NFR-PERF-03 | 单源超时不阻塞整体 | 单源失败归入 errors |
| NFR-PERF-04 | 每源稳定检索速率 | 单源 ≥ 10 次/分钟且不限速（不触发上游 429 / IP 封禁） |

### 5.2 NFR-REL 可靠性

| ID | 需求 | 度量 |
|---|---|---|
| NFR-REL-01 | 上游 429/403 自动退避 | 指数退避 + 随机抖动重试；Semantic Scholar 等实现重试降级 |
| NFR-REL-02 | 上游不可达优雅返回空 | 不抛异常中断 |
| NFR-REL-03 | 缓存损坏不影响运行 | 读取/加载均容错 |
| NFR-REL-04 | 每源全局限速器 | 令牌桶按源合规间隔限速；并发被收敛，突发被平滑，不直接打满上游 |

### 5.3 NFR-SEC 安全

| ID | 需求 | 度量 |
|---|---|---|
| NFR-SEC-01 | 禁止硬编码 API key | 静态扫描 + CI 强制 |
| NFR-SEC-02 | .env 不入库 | .gitignore 覆盖 |
| NFR-SEC-03 | 依赖漏洞扫描 | CI 运行 |

### 5.4 NFR-MAINT 可维护

| ID | 需求 | 度量 |
|---|---|---|
| NFR-MAINT-01 | 静态类型检查全程开启 | Go 原生类型系统（编译期强制） |
| NFR-MAINT-02 | 单元测试离线 hermetic | CI 只跑离线套件；触网测试单独目录手动触发 |
| NFR-MAINT-03 | 覆盖率门禁 | ≥ 60% |
| NFR-MAINT-04 | 文档不腐化 | 引用不断链；文档声明与实现一致 |

### 5.5 NFR-COMP 兼容

| ID | 需求 | 度量 |
|---|---|---|
| NFR-COMP-01 | 跨平台单一二进制 | Windows / macOS / Linux；无运行时依赖（goreleaser 交叉编译） |
| NFR-COMP-02 | MCP 客户端兼容 | Claude Desktop / Trae / 任意 MCP 客户端 |
| NFR-COMP-03 | CLI 可被 AI shell 调用 | 帮助信息自描述，退出码语义化 |

## 6. 技术栈（Go）

| 层 | 选型 | 说明 |
|---|---|---|
| 语言 | Go 1.26（2026-02 发布，当前最新稳定版） | 静态类型、编译期检查、单一二进制；1.27（2026-08 发布）后可评估升级 |
| CLI | spf13/cobra v1.9+ | 子命令、自动帮助生成、shell 补全 |
| MCP | modelcontextprotocol/go-sdk v1.7+ | 官方 Go SDK（2026-07-27），stdio 传输，支持 2026-07-28 MCP 规范 |
| HTTP | net/http（标准库） | 零依赖；超时/重试自封装 |
| XML/Atom | encoding/xml（标准库） | arXiv Atom / PubMed EUtils |
| HTML | goquery（可选源） | 需 HTML 抓取的源（预留，二期） |
| 引用渲染 | 内置格式化器（GB/T 7714—2025 / APA / IEEE）+ Pandoc CSL | 3 种核心样式原生实现；Zotero 已发布 GB/T 7714—2025 CSL 样式（numeric/author-date/note）；5 个 .csl 经 Pandoc |
| 存储 | modernc.org/sqlite v1.54+ | 纯 Go（无 CGO），SQLite 3.53，FTS5 内建，交叉编译友好 |
| 语义检索（仅本地文献库） | embedding provider 抽象：本地纯 Go 推理（goformer / go-semantica 候选）+ 可选国内 API（阿里百炼 text-embedding-v4 / 硅基流动 BGE-M3） | 默认本地零 key 零网络；API 提升质量；向量存 SQLite 同库，暴力余弦（本地规模） |
| .env | joho/godotenv | .env 文件解析 |
| 并发/限速 | goroutine + errgroup + golang.org/x/time/rate | 多源并发检索，单源失败隔离；每源令牌桶限速 |
| 模板嵌入 | go:embed | lint 模板 + CSL 文件编译进二进制 |
| docx | Pandoc（外部二进制，可选） | 缺失时仅 docx 不可用 |
| 构建/发布 | goreleaser v2.17+ | 跨平台单一二进制（Windows/macOS/Linux） |

> 版本核实日期：2026-08-02（Go 1.26 / go-sdk v1.7.0 / modernc.org/sqlite v1.54 / cobra v1.9 / goreleaser v2.17 均为当时最新稳定版）。

> Go 单一二进制分发：模板、CSL 样式文件经 go:embed 编译进二进制，用户无需安装运行时或依赖。

## 7. 接口需求

### 7.1 CLI

```
litkit search       <query> [-s sources] [-n N] [-y year] [--keep-no-abstract]
litkit metadata     <id_type> <identifier>          # doi | pmid | arxiv | title
litkit fetch        <cite_key|doi> [--out downloads]  # 全文获取（Unpaywall OA → Sci-Hub 兜底）
litkit sources
litkit manuscript   <draft.md> [--lang zh|en] [-s style] [--docx] [-o output_dir]
litkit export       <papers.json> [-f bibtex|ris|text] [-s style]
litkit cache        list | clear
litkit library      <subcommand>
litkit lint init    [project_dir] [--force] [--type review|empirical] [--lang zh|en] [--journal NAME]
litkit verify       <manuscript> [--lang zh|en] [--mode chapter|draft|final] [--type review|empirical]
```

### 7.2 MCP 工具（关键）

| 工具 | 入参 | 出参 |
|---|---|---|
| `search_papers` | query, sources, max_results_per_source, year | total, source_results, errors, papers[] |
| `get_paper_metadata` | id_type(doi/pmid/arxiv/title), identifier | Paper \| null |
| `fetch_paper` | cite_key 或 id_type+identifier | { citeKey, pdfPath, fulltext, via } |
| `process_manuscript` | text, lang, style, generate_docx, output_dir | processed_text, reference_list, citation_map, unresolved, files{} |
| `export_references` | papers[], format, style | success, content |
| `lint_init` | project_dir, force, lang, paperType, journal | 初始化状态 + next_steps |
| `verify_manuscript` | files[], lang, mode, paperType, rule, skip | issues[]（问题/修复/规则编号） |
| `search_<source>` | 各源特定 | 同 search_papers 结构 |
| `lib_list` / `lib_search` / `lib_rm` | source/keyword/limit / cite_key | 库内论文 / 命中 / 删除结果 |
| `lib_stats` / `lib_path` | — | 文献库统计 / 库路径 |

### 7.3 环境变量

| 变量 | 必需 | 作用 |
|---|---|---|
| LITKIT_SEMANTIC_SCHOLAR_API_KEY | 可选 | Semantic Scholar 速率提升 |
| LITKIT_UNPAYWALL_EMAIL | 可选 | 全文 OA 解析必需（Unpaywall，FR-FETCH-02） |
| LITKIT_SCI_HUB_URL | 可选 | Sci-Hub 兜底镜像（默认 https://sci-hub.se，FR-FETCH-03） |
| LITKIT_IEEE_API_KEY / ACM_API_KEY | 二期激活对应源必需 | 启用源工具 |
| LITKIT_WORK_DIR | 必填 | 统一工作目录。**未设置时 init/search/lib 拒绝执行（errNoWorkDir，FR-LIB-03）**。**测试固化目录：`e:\Codes\litkit\workspace`**（库文件、输出文件落于此） |
| LITKIT_ENV_FILE | 可选 | 显式 .env 路径 |
| LITKIT_LANG | 可选 | 默认写作语言模式（zh/en） |
| LITKIT_EMBEDDING_PROVIDER | 可选 | local（默认）/ api；服务本地库语义检索 |
| LITKIT_EMBEDDING_API_KEY | api 模式必需 | 阿里百炼 / 硅基流动 embedding key |
| LITKIT_HTTP_TIMEOUT_MS | 可选 | 单请求超时（默认 15000） |
| LITKIT_HTTP_RETRIES | 可选 | 429/5xx 重试次数（默认 2） |

## 8. 平台能力矩阵（国内可达性视角）

> 详见 [`.harness/specs/reference/platform-matrix.md`](../reference/platform-matrix.md)；与 `litkit sources` 输出保持一致；M2 验收要求与实测一致。

## 9. 验收标准（Definition of Done）

- 第 4 章 P0 需求全部满足，P1 需求满足 ≥80%
- 平台能力矩阵与实测结果一致
- 静态类型检查 / 单元测试 / 覆盖率门禁全部通过
- 单元测试离线 hermetic；触网测试单独目录手动触发
- CLI 与 MCP 全部工具行为一致（同输入同输出）
- 文档引用无断链，文档声明与实现一致

## 10. 约束与假设

### 10.1 约束

- **C1 中文优先**：默认写作语言模式 zh；en 为第二模式
- **C2 摘要工作流**：检索与入库仅基于元数据与摘要（不依赖 PDF）；检索结果必须含摘要（无摘要论文默认过滤，`--keep-no-abstract` 显式保留）；全文获取（FR-FETCH）为按需可选能力，由 `fetch` 显式触发，不改变检索入库默认行为
- **C3 引用标准**：中文引用遵循 GB/T 7714—2025（2026-07-01 实施），支持预印本[PP]、数据集[DS]等新文献类型
- **C4 免费优先**：核心源公开 API；付费源为可选骨架
- **C5 单向分层**：入口层 → 服务层 → 适配层 → 叶子层，禁止反向依赖
- **C6 数据模型纯净**：核心数据载体不依赖任何上层模块
- **C7 统一源抽象**：所有学术源实现统一接口
- **C8 接口一致性**：CLI 与 MCP 共享核心实现，禁止行为分叉
- **C9 入库元数据必须含摘要**
- **C10 语义检索双模式（仅本地文献库）**：本地模型默认（免费优先），可选 API 提升质量；不强制外部依赖；远程检索保持 keyword
- **C11 授权预留**：本期不实现认证/授权，仅在接口层预留扩展点
- **C12 开源发布**：核心代码开源（Apache-2.0）

### 10.2 假设

- **A1** 用户有合法网络访问权限访问公开 API
- **A2** 国内网络环境下，核心源（检索源 arXiv/PubMed/OpenAlex 等 + 反查源 CrossRef）可达；个别源偶发慢
- **A3** 上游源反爬/限速策略不保证稳定
- **A4** Pandoc 为可选外部依赖，缺失仅 docx 不可用
- **A5** 上游 API 字段变更可能导致个别源解析异常，由触网测试尽早发现
- **A6** 检索结果必须含摘要：无摘要论文默认过滤（`--keep-no-abstract` 显式保留）；abstract 可空仅用于反查/引用生成路径

## 11. 风险与开放问题

### 11.1 技术风险

| 风险 | 说明 | 缓解 |
|---|---|---|
| 检索源无摘要能力 | 无摘要源无法通过摘要门槛 | CrossRef 移出检索源（仅反查）；新源接入前核验 HasAbstract |
| 国内网络波动 | 部分源（arXiv/Semantic Scholar）偶发慢或不稳 | 超时隔离、降级、缓存 |
| 上游限速波动 | arXiv 2026-02 起并发下偶发 429（官方 20/分但实测更严）；Semantic Scholar 无 key 为全用户共享池 | 每源限速器（arXiv 间隔 ≥3s）+ 指数退避重试 + 可选 key 提升档位 |
| 上游 API 变更 | 字段变化导致解析异常 | 触网测试定期跑、解析器集中封装 |
| 本地 embedding 质量 | 小型本地模型（BGE-small 级）检索质量有限 | API 模式提质量；本地模型选型 POC 验证 |

### 11.2 开放问题

- 中文文献引用（知网等）的替代路径：仅支持著录（用户手动提供元数据），不支持检索
- 本地 embedding 模型选型（服务本地库语义检索）：goformer（BGE 系列）vs go-semantica（GGUF）vs 外部 Ollama——由 POC 实测决定
- 三期 LLM 引用评分的多模型组合策略（开源模型 vs API 模型、打分聚合方法、阈值标定）——由三期 POC 决定
