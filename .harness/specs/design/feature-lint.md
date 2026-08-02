# 功能设计：撰写约束验证（lint）

---
last_updated: 2026-08-02
status: active
owner: litkit-core
---

对应 PRD：FR-LINT

## 目标

将论文撰写规范机械化为可执行验证：`lint init` 初始化宿主项目约束基础设施，`verify` 按 zh/en 规则集验证文稿，输出"问题/修复/规则编号"三要素。

## 范围

### 包含
- `lint init`：生成 CLAUDE.md + `.harness/`（rules.md / verify / checks/ / checklist.md / specs / verifier_models.json）；已存在时备份并提示（FR-LINT-01）
- zh 规则集：全半角标点、中文引号、句式冗余（"进行""通过…使"）、"的/地/得"、"了/着/过"、编号层级、GB/T 7714—2025 引用规范、AI 痕迹（FR-LINT-02）
- en 规则集：语法一致性、时态、冠词/单复数、学术措辞、APA/IEEE 引用、AI 痕迹（FR-LINT-03）
- 规则体系：每条规则含定义、违规示例、验证方法（A 自动/S 半自动/M 人工）（FR-LINT-04）
- `verify`：--lang/--verbose/--rule/--mode（chapter/draft/final），三要素报错（FR-LINT-05）
- checklist.md 人工审查清单（FR-LINT-06）
- manuscript-spec.yaml 阈值配置（FR-LINT-07）
- 引用验证（LLM 评分，增量缓存）（FR-LINT-08）
- 验证命令在终端运行（lint init 返回 next_steps）（FR-LINT-09）

### 不包含
- 中文文献库检索

## 分层设计

### internal/core
`lint.go`：约束验证编排（zh/en 规则集选择、mode 递增、A/S/M 分类执行）。

### templates（go:embed 编译进二进制）
`lint init` 产出的宿主模板：rules.md、verify 脚本、checks/、checklist.md、specs、verifier_models.json。

### 入口层
`litkit lint init` / `litkit verify` + `lint_init` / `verify_manuscript` MCP 工具。

## 关键规则/约束

- 规则集按 zh/en 分设（C1 中文优先，zh 为默认模式）
- 验证方法三分类：A 自动 / S 半自动 / M 人工（FR-LINT-04）
- M 类规则走 checklist 人工复核，不自动判 fail
- verify 报错含三要素：问题 / 修复 / 规则编号（FR-LINT-05）
- lint init 产物零外部文件依赖（go:embed）

## 测试要求

- [ ] zh/en 各自违规样例全部被检出（FR-LINT-02/03 验收）
- [ ] 三要素完整性测试：rule_id / problem / suggestion 全有
- [ ] mode 递增范围测试：chapter/draft/final 启用规则递增
- [ ] lint init 幂等测试：已存在时备份并提示
- [ ] CLI 与 MCP 一致性测试（FR-IFACE-03）
