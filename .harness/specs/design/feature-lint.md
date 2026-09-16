# 功能设计：撰写约束验证（lint / harness）

---
last_updated: 2026-09-16
status: active
owner: litkit-core
---

对应 PRD：FR-LINT

## 目标

为 AI 撰写论文提供「事前指导 + 事后验证」双层的撰写 harness：
`litkit init` 生成宿主工作目录的 `.litkit/`（使用地图 + 阈值配置）与 `.agents/skills/`（技能包），
**撰写硬性规定即 `manuscript-spec.yaml` 顶部注释**（单一事实源，AI 必读）；
成稿后 `litkit verify` 按规则集机械化验证（事后兜底），输出"问题/修复/规则编号"三要素。

## 范围

### 包含
- `litkit init`：生成 `.litkit/`（AGENTS.md 使用地图 / verifier_models.json / litkit.db）
  与 `.agents/skills/`（litkit、academic-style、manuscript-format），并按 `--type`/`--lang`
  生成 `.litkit/<type-lang>/manuscript-spec.yaml`（FR-LINT-01）
  - `--type review|empirical|book|proposal` 论文类型 preset（阈值切换，规则代码零差异）
  - `--lang zh|en` 撰写语言
  - `--journal` 目标期刊（写入 spec 的 `journal` 字段）
- 通用规则体系 R0-R10：每条规则含定义、违规示例、验证方法（A/S/M）、langs 标注（FR-LINT-04）
  - zh 专属：全半角标点、中文引号、的地得、句式冗余、AI 痕迹（FR-LINT-02）
  - en 专属：冠词、时态、学术措辞（FR-LINT-03）
  - 通用：编号层级、P 值/统计量/百分率、引用占位符、字数阈值
- `verify`：--lang/--mode（chapter/draft/final）/--rule/--skip，三要素报错（FR-LINT-05）
- M 类人工审查清单（FR-LINT-06）：由 `verify` 输出的 `manualChecklist` 字段承载，不落独立文件
- manuscript-spec.yaml 阈值配置：字数/引用数/章节/标题层级/引用样式（FR-LINT-07）
- 引用验证（LLM 评分，增量缓存）（FR-LINT-08，已实现一期 core）
  - Scorer 接口与 ScorerEngine（多模型扇出 + 增量缓存 + 优雅降级）
  - ExtractCiteSentences（引用句抽取，锚点前后找句末标点，跳过代码块/参考文献）
  - CheckNumericConsistency（Layer 0 数字集合规则，免费确定性检查）
  - citation_scores 表（SQLite 缓存，主键自然失效）
  - `litkit verify --report citation-refs` 输出
- 验证命令在终端运行（lint init 返回 next_steps）（FR-LINT-09）

### 不包含
- 中文文献库检索
- 期刊差异预设包（稿约不同，改 manuscript-spec.yaml 即可，YAGNI）
- **缺引用检测**（"某句该有引用却无引用"）：`ExtractCiteSentences` 是锚点驱动——以已存在的
  `[@citeKey]` 为锚点抽句，只能评"已有引用 ↔ 文献"的匹配度，无法判断某句是否**应当**有引用。
  当前靠三条间接兜底：R5.2（final 模式 `[待引证]` 清零）、R5.3（引用数区间下限）、
  `manualChecklist` 人工核对。真要做需 S 类启发式（如"研究表明…"类标识）或 LLM 判句；
  需求不明确前不实现（YAGNI）
- 独立 `rules.md` / `checklist.md` 模板、AGENTS.md 渲染撰写段（`RenderWritingRules`）与 `--refresh`：
  原设计已废弃——规则由 `litkit rules` 查询，人工项由 `verify` 输出，撰写规定以 spec yaml 注释为
  单一事实源，不存在需重新渲染同步的下游产物

## 分层设计

### internal/lint
`spec.go`：ManuscriptSpec 类型 + yaml 解析/校验/默认值。
`harness.go`：`.litkit/` 目录生成 + `.agents/skills/` 分发（go:embed 模板）。
`verify.go`：规则函数注册表 + lint.Run() 纯函数入口（无 IO，CLI 是薄壳）。
服务层；verify 规则函数注册表已落地（FR-LINT-05）。

### templates（go:embed 编译进二进制）
`internal/lint/templates/`：root-AGENTS.md（使用地图）、`<type-lang>/manuscript-spec.yaml`
（各类型阈值 + 撰写硬性规定注释）、verifier_models.json、
skills/（litkit + references、academic-style、manuscript-format + references）。

### 入口层
`litkit init`（--type/--lang/--journal/--force）+ `litkit verify`（--lang/--mode/--rule/--skip）。

## 关键规则/约束

- **`.litkit/` 独立于 litkit 自身开发约束 `.harness/`**：宿主论文项目用 `.litkit/`，统一命名空间
- **规则代码单套，按 langs 过滤**：不设 zh/en 两套系统；每条规则声明 langs，`--lang zh|en|zh,en` 启用
- **论文类型 = preset 阈值切换**：manuscript-spec.yaml 换阈值与章节清单，规则代码零差异
- **事前指导 = spec yaml 顶部注释**：精简祈使句（非 yaml 数据复制），AGENTS.md 与技能文档只指向它；
  AI 写稿时遵守，事后 verify 兜底
- **单一事实源，无同步步骤**：改 yaml 阈值立即生效（不隐式覆盖用户手改），不存在需重新渲染的下游文件
- 验证方法三分类：A 自动 / S 半自动 / M 人工（FR-LINT-04）；M 类由 verify 的 `manualChecklist` 输出，不自动判 fail
- verify 报错含三要素：问题 / 修复 / 规则编号（FR-LINT-05）
- lint init 产物零外部文件依赖（go:embed）

## verify 实现要点（已落地）

- **42 条规则**：28 A 类（自动判定）+ 14 S 类（半自动）；S 类命中仅提示人工确认，不计入 fail 判定
- **模式递增**：chapter（结构）→ draft（+数据/标点/引用）→ final（+字数/行文），高模式包含低模式全部规则
- **证据不足的显式出口**：R5.2 `[待引证]` 仅 final 模式判违规，chapter/draft 视为"待人工补文献"的
  合法中间态（写作中不得因占位判 fail，否则模型会删论断或改写空话规避）；
  R5.4 `[TODO]`/`[TBD]` 属作者备忘，draft 起即违规。二者原共用 R5.2，语义不同故拆分
- **Markdown 分段**：排除代码块/参考文献/表格后检查 Body
- **纯函数设计**：lint.Run() 无 IO，接收文本与配置返回 Report；CLI 是薄壳（读文件 → Run → 输出 JSON）
- **exitHint 三值**：pass / fix_and_rerun / manual_review；退出码 0=通过或仅需人工复核，1=有 A 类违规
- **引用评分（FR-LINT-08）**：`--report citation-refs` 在规则验证后额外执行，输出包含 enabled/models/results 的 JSON 块
  - 三层漏斗：Layer 0 数字集合规则（免费确定性）→ Layer 1 embedding 语义预筛（依赖 M7）→ Layer 2 LLM 多模型评分
  - 禁用模式：LITKIT_VERIFY_LINT_LLM=false 或无可启用模型时静默跳过，不报错
  - 缓存优先：citation_scores 表全命中则直接返回聚合结果，不调 API
  - 优雅降级：部分模型失败（401/超时）不影响其他模型评分
- **能力边界（缺引用检测）**：`ExtractCiteSentences` 锚点驱动，只对**已含 `[@citeKey]` 的句子**
  生成评分项。因此 citation-refs 检不出"该有引用而无引用"，且"无引用句"不会出现在评分结果里
  （不要把它当作漏报 bug）。间接兜底见「不包含」

## 测试要求

- [x] manuscript-spec.yaml 解析：默认值 / 自定义 / 非法配置报错（FR-LINT-07）
- [x] init 集成：`.litkit/` 基础设施（AGENTS.md / verifier_models.json / litkit.db）+ `.agents/skills/` +
  `<type-lang>/manuscript-spec.yaml`、--type review 生效、无 force 不覆盖
- [x] zh/en 各自违规样例全部被检出（FR-LINT-02/03 验收）
- [x] 三要素完整性测试：rule_id / problem / suggestion 全有
- [x] mode 递增范围测试：chapter/draft/final 启用规则递增
- [x] R5.2/R5.4 模式语义：`[待引证]` 在 chapter/draft 不报、final 报；`[TODO]`/`[TBD]` 在 draft 报
