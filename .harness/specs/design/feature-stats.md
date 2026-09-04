# 功能设计：统计分析包

---
last_updated: 2026-09-04
status: draft
owner: litkit-core
---

对应 PRD：新增 FR-STAT 系列（写入 PRD 后再解锁 gate 同步校验）

## 目标

提供一组"**会做前置假设校验、能讲清违规、供 AI 转述给人类**"的统计检验，覆盖**护理科研论文全场景**所可能用到的分析。核心契约是给 AI 的**结构化结果**（必出）；人类可读的报告与图表为**可选导出**，仅最终定稿时触发，避免多轮探讨冗余。

## 现状澄清（重要）

两库分工，缺一不可：

- **gonum/stat + gonum/stat/distuv** 提供：描述统计（Mean/Variance/Skew/Quantile/Histogram/…）、双样本 `KolmogorovSmirnov`（正态性检验基座）、`ChiSquare` 统计量、概率分布（`CircularIn...`/`ChiSquare`/`StudentsT` 等）及 `CDF/Survival`（求 p 值）。**不提供假设检验函数。**
- **aclements/go-moremath/stats** 提供真正的高质量**假设检验**：单/双样本 t、配对 t、Mann-Whitney U、Wilcoxon 符号秩、Kruskal-Wallis、Spearman 等级相关、双样本 KS；并内置结(ties)校正、小样本精确分布 vs 大样本正态近似自动切换。

正因如此，一期**需要引入 `go-moremath/stats`** 这个依赖（gonum/stat 的假设检验是空白）。

## 范围

### 包含（护理科研常用，均以 go-moremath / gonum 为算源）
- **两组连续变量**：独立 t 检验（含 Welch）、配对 t、Mann-Whitney U、Wilcoxon signed-rank
- **多组连续变量**：单因素 ANOVA（Welch）、Kruskal-Wallis H
- **分类变量**：卡方独立性检验（`stat.ChiSquare` + `distuv.ChiSquare` 求 p）、Fisher 精确
- **相关性**：Pearson、Spearman（等级）、Kendall τ
- **分布检验**：Shapiro-Wilk / 单样本正态性（Lilliefors，基于 KS 校正）、双样本 KS
- **描述统计**：Mean/Median/SD/IQR/分位数，供论文表格基线数据
- 未在以上但两库中存在的计算函数按需补齐封装（分布求 p、效应量、置信区间）
- 统一入口 `Analyze` 返回结构化结果：选定检验、统计量、df、p 值、效应量、每项假设校验 PASS/FAIL + 白话补救
- **默认输出**结构化 JSON（必出）；报告/图表未来版本实现（本期不实现）

### 不包含（本期）
- 图表渲染（SVG/PDF）与 Markdown 报告落盘——未来版本；本期只出结构化结果
- 回归建模、生存分析（护理论文少用，且不在两库假设检验范畴）
- 多因素/重复测量高级设计（二期视需要）
- 概率分布全套封装，仅暴露检验所需最小面

## 分层设计

新增**叶子层** `internal/stats`（纯计算，零网络、零 I/O，不 import 上层）。被 `internal/core`（服务层）编排调用；CLI `litkit stats` 子命令为入口。

### internal/stats（叶子层）
- `analyze.go`：`Analyze` 统一入口（两组比较决策链，含假设校验自动降级）
- `tests.go`：封装 go-moremath 假设检验（t/配对/Mann-Whitney/Wilcoxon/Kruskal-Wallis/ANOVA/Fisher/corr）
- `chi2.go`：分类变量卡方（gonum `stat.ChiSquare` + `distuv.ChiSquare` 求 p）
- `normality.go`：正态性判定（Lilliefors/KS）→ 驱动 t 系 vs 非参数系
- `desc.go`：描述统计（均值/中位数/SD/分位数）
- 依赖：gonum + go-moremath/stats，维持叶子层纯净

### 入口层
`litkit stats --data x.csv`（默认出结构化 JSON；`--report` 等图表/报告旗标留待未来版本）

## 关键规则/约束

- 统计正确性属"不可省"硬约束：p 值/自由度一律由 go-moremath / gonum 计算，禁止手写近似公式
- 分层单向：`internal/stats` 不得 import `internal/core` 或入口层
- 默认零冗余：普通调用只输出结构化 JSON；报告/图表按旗标显式触发（本期未实现）
- 结构化结果字段最小化：检验/统计量/df/p/效应量/假设校验项，够 AI 判断即可
- 假设校验失败自动降级：正态不成立 → 自动落非参数并列明降级原因到结果
- 引用两库时必须标注各函数归属，避免误以为某检验在 gonum 而实际在其姊妹库

## 测试要求

- [ ] TDD：`Analyze` 决策链——正态数据走 t / 非正态自动落 Mann-Whitney（教科书已知数据对拍）
- [ ] t/配对 t/Mann-Whitney/Wilcoxon 各一个已知数据用例，断言统计量/p 与教科书或 SPSS 一致
- [ ] 多组：ANOVA(Welch)/Kruskal-Wallis 各一已知数据用例
- [ ] 卡方 2×2：断言 χ²、df、p 与手工计算一致；Fisher 精确小样本用例
- [ ] 相关：Pearson/Spearman/Kendall 已知数列断言 r/ρ
- [ ] 正态性：正态样本 PASS、偏态/离群样本 FAIL 且降级路径正确
- [ ] `Result` JSON 序列化：默认字段最小集，无报告冗余
- [ ] gate 门禁：go.mod 新增 gonum + go-moremath/stats 依赖过 lint/vulncheck/arch

## 备注

- PRD 需新增 FR-STAT 系列后再过同步校验；本期为 draft，先定设计
- go-moremath API 标注为"不稳定"（v0.x），封装层隔离其上游变更，避免波及 core