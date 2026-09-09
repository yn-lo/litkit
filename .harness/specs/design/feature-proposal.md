# 功能设计：学术标书（课题申请书）撰写与审查支持

---
last_updated: 2026-09-09
status: draft
owner: litkit-core
---

对应 PRD：FR-PROPOSAL（待补）

## 目标

让 litkit 以与论文相同的工作流支持学术标书：AI 按标书模板撰写（事前指导），
`litkit init --type proposal` 渲染标书撰写硬性规定，`litkit verify` 事后机械校验
（章节结构 / 字数区间 / 引用模式 / 占位符）。**不新建标书子系统**，
复用 lint 的 preset 机制：标书 = 一种新的 manuscript 类型 preset（yaml 阈值切换，规则代码单套）。

## 范围

### 包含
- 新 preset：`--type proposal`（科研课题申请书，如院内基金、省自然等）
  - 内置模板 `manuscript-spec-proposal.yaml`：章节树 + 每节字数区间 + 引用模式
  - AGENTS.md「撰写硬性规定」段按标书 preset 渲染（复用 RenderWritingRules）
- manuscript-spec.yaml 新增字段 `citation_mode: inline|endnote`
  - `inline`：论文模式（正文内联引用，默认，向后兼容）
  - `endnote`：标书模式（正文不出现内联引用标记，参考文献统一置于报告正文末尾）
- verify 新增 A 类规则（R10.2/R10.3 通用；不新增章节结构规则——
  现有 R1.5 章节完整性 + R1.9 章节顺序已按 spec.Sections 覆盖，标书直接复用）：
  - R10.2 每节字数区间校验（通用，所有类型生效）：sections 条目新增可选
    `min_words`/`max_words`；未填不查该节；填了在全局字数之外加查该节。
    统计口径：中文字符按 1 字计、英文按空格分词（复用 countWords）
  - R10.3 引用模式校验（citation_mode=endnote 时，默认违规；
    走统一规则注册表，`--rule/--skip` 可按需跳过/单跑）：
    正文出现 `[1]` / `[1,2]` / `（作者，年份）` 内联引用判违规；
    文末须存在参考文献节，且条目编号连续、与文内提及一致
  - R10.4 占位符规范：`〔　〕` 类待填空项允许存在（输出人工核对提示，不计 fail），
    但 AI 禁止编造空项内容（渲染进 AGENTS.md 规定 + checklist M 类项）
- 用户自定义模板：`litkit init --spec <path.yaml>` 采纳外部标书模板 yaml
  （单位/学科差异不改代码，只改 yaml；`--refresh` 渲染机制照旧）
- 检索与引用能力照常服务标书「立项依据」的文献综述（不新增逻辑）

### 不包含
- docx/PDF 模板解析与「通用标书模板引擎」（结构 = yaml，YAGNI）
- 标书内容自动撰写 / 一键成稿（litkit 保持 AI-first 工具定位，撰写由 agent 完成）
- 预算表、人员表等表格内容校验（表格已排除在 Body 检查外，M 类走 checklist）

## 分层设计

### internal/lint
- `spec.go`：ManuscriptSpec 增加 `CitationMode` 字段与校验、默认值（inline）
- `templates/manuscript-spec-proposal.yaml`（go:embed）：proposal 章节树与阈值
- `harness.go`：RenderWritingRules 增加 proposal 分支（引用模式、占位符规定的祈使句渲染）
- `verify.go`：R10.x 规则函数注册，按 `type == proposal` 与 `citation_mode` 启用
- `init`：`--spec` 参数读取外部 yaml（覆盖内置 preset）

### 入口层
`litkit init --type proposal [--spec x.yaml]`、`litkit verify --mode chapter|draft|final`
（命令接口不变，仅 preset 扩展；api.md 同步更新）

## 关键规则/约束
- 标书类型 = preset 阈值切换，与 `--type review|empirical` 同构，不引入第二套系统
- **正文作用域收口（通用，单点改动）**：现状 ParseSource 仅排除代码块/参考文献/表格，
  封面、简表、签章页均计入 Body。改为：spec 定义 sections 时，
  首个 section 标题之前的行不进入 Body，但标题行保留（主标题仍受 R1.2 等检查，
  封面字段文字排除）——ParseSource 一处收口，
  全部既有规则（标点/加粗/AI 痕迹/引用/字数等 41 条）自动只作用于 section 范围，
  规则代码零改动；未定义 sections 的旧 yaml 保持现状（向后兼容）。
  不引入 body_start/body_end 变量，sections 即起止标记
- **作用域改动不新增章节结构规则**：结构缺失/顺序由既有 R1.5/R1.9 覆盖；
  残余冲突仅剩标题类规则（R1.3 编号预期阿拉伯数字，中文标书"（一）"风格
  可能误报）——按类型豁免或 spec 配置解决，不扩大本次改动范围
- 引用模式是 yaml 字段而非命令行开关：不同标书模板引用位置不同，随模板走
- verify 三要素（问题 / 修复 / 规则编号）与 exitHint 语义不变
- mode 递增照旧：chapter（结构+字数区间）→ draft（+引用模式/标点）→ final（+行文）
- 占位符 `〔　〕` 不判 fail（这是标书的正常形态），仅提示人工补填
- 内置 proposal 模板以常见院内课题申请书为基准（章节：摘要/立项依据/研究方案/研究基础/经费预算/附件），单位差异靠 `--spec` 覆盖

## Open Questions（搁置项）
- 单节豁免（某节正文跳过部分规则）：现有逃生口 `--rule/--skip` 为全局粒度已够用；
  真实需求出现后再设计（最可能形态：section 条目加 `skip_rules` 字段），
  需求不明确前不实现（YAGNI）

## 测试要求
- [ ] proposal yaml 解析：默认值 / citation_mode 非法值报错
- [ ] init --type proposal：四件套生成、AGENTS.md 渲染含标书规定段
- [ ] init --spec：外部 yaml 采纳、--refresh 不覆盖用户 yaml
- [ ] R1.5/R1.9 复用：标书文稿缺节 / 顺序错样例被既有规则检出
- [ ] R10.2：超字数 / 不足字数样例被检出；区间边界值通过；未配字数的节不报
- [ ] 作用域：含封面/简表/签章页的文稿，封面内容不计入任何节字数；旧 yaml（无 sections）行为回归不变
- [ ] R10.3：inline 模式下正文内联引用不报错；endnote 模式下报错且文末无参考文献节报错
- [ ] R10.4：占位符仅输出提示，不产生 A 类违规
- [ ] 回归：--type review/empirical 行为不变（citation_mode 默认 inline）
