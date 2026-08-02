# 功能设计：撰写约束验证（lint / harness）

---
last_updated: 2026-08-02
status: active
owner: litkit-core
---

对应 PRD：FR-LINT

## 目标

为 AI 撰写论文提供「事前指导 + 事后验证」双层的撰写 harness：
`litkit init` 生成宿主工作目录的 `.litkit/`（规则/人工清单/阈值配置），
AGENTS.md 携带由 manuscript-spec.yaml 渲染的「撰写硬性规定」段（事前指导，AI 自动加载）；
成稿后 `litkit verify` 按规则集机械化验证（事后兜底），输出"问题/修复/规则编号"三要素。

## 范围

### 包含
- `litkit init`：生成 `.litkit/`（rules.md / checklist.md / specs/manuscript-spec.yaml / verifier_models.json），
  并从 manuscript-spec.yaml 渲染 AGENTS.md「撰写硬性规定」段（FR-LINT-01）
  - `--type review|empirical` 论文类型 preset（阈值切换，规则代码零差异）
  - `--lang zh|en` 撰写语言
  - `--refresh` 按现有 yaml 重新渲染 AGENTS.md（不覆盖用户手改的 yaml）
- 通用规则体系 R0-R9：每条规则含定义、违规示例、验证方法（A/S/M）、langs 标注（FR-LINT-04）
  - zh 专属：全半角标点、中文引号、的地得、句式冗余、AI 痕迹（FR-LINT-02）
  - en 专属：冠词、时态、学术措辞（FR-LINT-03）
  - 通用：编号层级、P 值/统计量/百分率、引用占位符、字数阈值
- `verify`：--lang/--verbose/--rule/--mode（chapter/draft/final），三要素报错（FR-LINT-05）
- checklist.md 人工审查清单（M 类规则）（FR-LINT-06）
- manuscript-spec.yaml 阈值配置：字数/引用数/章节/标题层级/引用样式（FR-LINT-07）
- 引用验证（LLM 评分，增量缓存）（FR-LINT-08，三期）
- 验证命令在终端运行（lint init 返回 next_steps）（FR-LINT-09）

### 不包含
- 中文文献库检索
- 期刊差异预设包（稿约不同，改 manuscript-spec.yaml 即可，YAGNI）

## 分层设计

### internal/lint
`spec.go`：ManuscriptSpec 类型 + yaml 解析/校验/默认值。
`harness.go`：.litkit 目录生成（go:embed 模板）+ RenderWritingRules（yaml → AGENTS.md 事前指导段）。
服务层；后续 verify 规则函数注册表也落于此（FR-LINT-05）。

### templates（go:embed 编译进二进制）
`internal/lint/templates/`：rules.md（R0-R9 通用规则，langs 标注）、checklist.md、
manuscript-spec.yaml、verifier_models.json。

### 入口层
`litkit init`（--type/--lang/--refresh/--force）+ 后续 `litkit verify`（FR-IFACE-03 两处注册）。

## 关键规则/约束

- **`.litkit/` 独立于 litkit 自身开发约束 `.harness/`**：宿主论文项目用 `.litkit/`，统一命名空间
- **规则代码单套，按 langs 过滤**：不设 zh/en 两套系统；每条规则声明 langs，`--lang zh|en|zh,en` 启用
- **论文类型 = preset 阈值切换**：rules.md 零差异，manuscript-spec.yaml 换阈值与章节清单
- **事前指导 ≠ yaml 翻译**：AGENTS.md 撰写段是精简祈使句（15-20 行），非 yaml 数据复制；
  AI 写稿时遵守，事后 verify 兜底
- **yaml → AGENTS.md 手动同步**：改 yaml 后 `litkit init --refresh` 重新渲染（不隐式覆盖用户手改）
- 验证方法三分类：A 自动 / S 半自动 / M 人工（FR-LINT-04）；M 类走 checklist，不自动判 fail
- verify 报错含三要素：问题 / 修复 / 规则编号（FR-LINT-05）
- lint init 产物零外部文件依赖（go:embed）

## 测试要求

- [x] manuscript-spec.yaml 解析：默认值 / 自定义 / 非法配置报错（FR-LINT-07）
- [x] RenderWritingRules：zh/en 各自渲染、阈值注入、章节按类型
- [x] init 集成：.litkit 四件套、--type review 生效、--refresh 重渲染且不改 yaml、无 force 不覆盖
- [ ] zh/en 各自违规样例全部被检出（FR-LINT-02/03 验收，verify 实现时）
- [ ] 三要素完整性测试：rule_id / problem / suggestion 全有（verify 实现时）
- [ ] mode 递增范围测试：chapter/draft/final 启用规则递增（verify 实现时）
- [ ] CLI 与 MCP 一致性测试（FR-IFACE-03，M5）
