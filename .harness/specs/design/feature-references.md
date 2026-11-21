# 功能设计：引用与手稿处理

---
last_updated: 2026-10-09
status: active
owner: litkit-core
---

对应 PRD：FR-REF

## 目标

解析手稿中的引用占位符，按标识符取回元数据（含摘要），**由 CSL 引擎统一渲染** GB/T 7714—2025（中英文混排）/ APA 7th / IEEE / 用户自定义样式，生成格式化手稿与参考文献；BibTeX / RIS 作为交换格式单独输出。preview/草稿视图是独立的 Go 实现，不经过 CSL 引擎。

## 范围

### 包含
- 占位符解析：裸 `[@citeKey]`（Pandoc 风格，查本地库；FR-REF-01 方案 B）与带前缀 `[@doi:]` / `[@pmid:]` / `[@arxiv:]` / `[@title:]`（现场反查并入库）
- 裸句柄组合 `[@AAA,BBB]` 与独立相邻引用一致；评分保留原始组合句，共享句子指纹
- 按标识符取元数据：DOI→CrossRef，PMID→PubMed，arXiv→Atom，title→CrossRef（FR-REF-02）
- 引用渲染：CSL 引擎（citeproc-js，goja 内嵌）统一渲染；内置 `gb7714-2025` / `apa` / `ieee`，支持 `.csl` 路径与 `WORK_DIR/styles/*.csl`（FR-REF-03/04/07）
- 相邻引用作为一个引用簇交给 CSL（GB/T 合并为 `[1-3]`，APA 排序为 `(A, 2020; B, 2021)`）
- BibTeX / RIS 生成（FR-REF-05/06）
- `manuscript` 完整流水线：formatted.md + refs.bib + refs.ris +（可选）docx（FR-REF-08）
- 未解析占位符归入 unresolved（FR-REF-09）
- `litkit export` 批量导出（FR-REF-10）
- `litkit styles list|show|export`：查看/导出内置 CSL，用户样式目录发现
- Pandoc docx 可选，缺失优雅降级（FR-REF-11）
- preview/草稿视图：内联自描述标记 + 核对清单，纯 Go 实现

### 不包含
- PDF 下载与全文抽取（摘要工作流）
- 中文文献库（知网/万方）检索

## 分层设计

### internal/model
`Paper` 承载元数据（引用渲染输入），`Language` 供 CSL 双语样式判断语言。

### internal/csl（适配层）
- `engine.go`：citeproc-js 会话（goja），会话状态独立，输出固定为 text
- `map.go`：`Paper → CSL-JSON`（type/author/issued/container-title/volume/issue/page/DOI/URL/publisher/language）
- `style.go`：样式注册表（内置 go:embed + 用户目录 + 路径）、`list/show/export`
- `assets/`：citeproc-js、3 份 CSL、zh-CN/en-US locale、NOTICE

### internal/core
- `metadata.go`：标识符 → Paper 反查
- `references.go`：样式解析 + CSL 渲染编排 + BibTeX/RIS
- `manuscript.go`：占位符解析（`ResolveManuscript`）、CSL 渲染（`FormatManuscript`）、草稿视图（`FormatPreview`）

### 入口层
`litkit metadata` / `litkit manuscript` / `litkit export` / `litkit styles` 命令。

## 关键规则/约束

- 正式引用样式只有一条渲染路径：CSL；不再维护原生格式化器
- 中文模式支持中英文文献混排著录（FR-REF-03）
- 引用标准 GB/T 7714—2025（2026-07-01 实施），支持新文献类型（C3）
- 未解析占位符不静默丢失（FR-REF-09）
- 内置名保留：用户目录同名文件不覆盖内置样式，只在 `styles list` 中标记冲突
- preview/草稿视图不依赖 CSL 引擎（调试工具不依赖被调试组件）
- Pandoc 缺失仅 docx 不可用，其余产物正常（FR-REF-11）
- 内置 APA 为紧凑 CSL：官方 apa.csl 在 goja 下初始化约 12s，紧凑实现约 50ms；需要官方完整实现时用 `--style` 加载

## 测试要求

- [x] 占位符解析测试：位置与全文匹配（FR-REF-01）
- [x] CSL 渲染测试：GB/T 中英文混排、APA、IEEE 样例
- [x] 引用簇测试：数值样式合并、APA 排序、未解析断组
- [x] 样式注册表测试：默认、内置、路径、用户目录、冲突、导出
- [x] Paper→CSL-JSON 映射测试：type/language/date/page/thesis publisher
- [x] BibTeX/RIS 生成测试：字段完整性（author/title/year/journal/volume/number/pages/doi/url）
- [x] preview/草稿视图测试：自描述标记与核对清单
- [x] unresolved 测试：未解析占位符出现在列表
- [x] Pandoc 缺失降级测试：跳过 docx，其余产物正常
