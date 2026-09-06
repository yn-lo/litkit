# 功能设计：统计分析包

---
last_updated: 2026-09-04
status: draft
owner: litkit-core
---

对应 PRD：新增 FR-STAT 系列（写入 PRD 后再解锁 gate 同步校验）

## 目标

提供一组"**会做前置假设校验、能讲清违规、供 AI 转述给人类**"的统计检验，覆盖**护理科研论文全场景**所可能用到的分析。核心契约是给 AI 的**结构化结果**（必出）；人类可读的报告与图表为**可选导出**，仅最终定稿时触发，避免多轮探讨冗余。

## 算源格局（已逐库核实 pkg.go.dev 函数清单，2026-09-04）

| 库 | 角色 | 提供内容 | 成熟度 |
|---|---|---|---|
| **gonum.org/v1/gonum/stat** (+distuv) | 描述统计底座 | 均值/方差/Skew/分位数、Pearson/Kendall、双样本 `KolmogorovSmirnov`、`ChiSquare` 统计量；`distuv` 分布 `CDF/Survival` 求 p。**无假设检验函数** | 高，Go 科学计算事实标准 |
| **aclements/go-moremath/stats** | 推断检验主体 | `TwoSampleTTest`/`TwoSampleWelchTTest`/`PairedTTest`/`OneSampleTTest`、`MannWhitneyUTest`（内置结校正、精确/近似自动切换）；另有 CI 工具。**没有 Wilcoxon 符号秩/ANOVA/Kruskal-Wallis/Spearman** | 中高（golang.org/x/perf 血统） |
| **montanaflynn/stats** | 补 Spearman | `Spearman`、ZTest、描述统计全套；零依赖 | 高 |
| **HazelnutParadise/insyra/stats** | 补多组与非参数族 | `WilcoxonTest`(符号秩)、`OneWayANOVA`/`TwoWayANOVA`/`RepeatedMeasuresANOVA`、`KruskalWallis`、`Friedman`、`ChiSquareTest`、`BartlettTest`、`FTest` | ⚠️ 年轻（v0.3.x，2026-05 才合入非参数族），数值正确性必须经 R oracle 对拍后才启用 |
| **glycerine/fisherexact** | 补 Fisher | Fisher 精确检验（2×2）+ 带 Yates 校正卡方 | 小而专，C 代码移植 |

### 纯 Go 覆盖（五库合计）

- 两组连续：独立 t（等/异方差）、配对 t、单样本 t、Mann-Whitney U、Wilcoxon 符号秩
- 多组连续：单因素/双因素/重复测量 ANOVA、Kruskal-Wallis、Friedman
- 分类：卡方（拟合优度/独立性）、Fisher 精确（2×2）
- 相关：Pearson、Spearman、Kendall τ
- 描述统计：Mean/Median/SD/IQR/分位数、均值/分位数置信区间
- 正态性：Lilliefors（gonum 双样本 KS 改造，单样本校正）

### 纯 Go 硬缺口（仅 R 可解，决定分层分发）

| 缺口 | R 方案 | 备注 |
|---|---|---|
| **SEM/CFA**（路径模型、拟合指数 CFI/RMSEA） | `lavaan` | 量表构建/中介效应刚需，Go 生态整类缺失 |
| **生存分析**（Cox 回归、Kaplan-Meier、竞争风险） | `survival` | R 推荐包，默认安装自带；护理随访研究刚需 |
| **Shapiro-Wilk** | `shapiro.test` | 纯 Go 只能 Lilliefors 代，论文若点名 Shapiro-Wilk 需 R |
| **现代贝叶斯**（MCMC、层次模型） | `brms`/Stan | 远期 |

另有部分缺口半解：效应量（insyra 结果结构体含 EffectSizes 字段，Cohen's d/Cramer's V/η² 种类待核实）；rstatix 全家桶依赖过重，R 引擎阶段一并评估。

## 分层分发策略

**体积账**：纯 Go 单 exe 约 15-25 MB；R 完整运行时装机 450-500 MB（下载 85-90 MB）；加 lavaan +20-40 MB；rstatix +100-150 MB。

- **默认层（所有用户）**：纯 Go 单二进制，覆盖上述"纯 Go 覆盖"全部检验，安装包保持 ~20 MB
- **R 增强层（可选）**：`litkit stats --engine r` 显式触发；运行时探测本机 R，无则明确报错并回退纯 Go（缺的检验如实报"需 R 引擎"）
- **增强包分发**：安装器可选勾选或事后单独下载"R 增强包"（约 100-200 MB，含精简 R 运行时 + lavaan），不捆绑进默认安装
- **权威性双保险**：常规检验的正确性由 CI 中的 **R oracle 对拍**背书（开发期用 R 算标准答案断言纯 Go 结果一致）；SEM/生存分析等 R 专属检验由 R 直接执行
- **禁止** cgo 嵌入 R（rgo 路线）：破坏交叉编译、部署需带 R 共享库；统一走子进程 + JSON 协议
- R 引擎具体实现（子进程协议、R 脚本打包）为二期，本期只做纯 Go

## 范围

### 包含（本期，纯 Go 算源）
- **两组连续变量**：独立 t（含 Welch）、配对 t、单样本 t、Mann-Whitney U、Wilcoxon signed-rank
- **多组连续变量**：单因素 ANOVA、Kruskal-Wallis H（双因素/重复测量视二期）
- **分类变量**：卡方独立性（gonum `ChiSquare` + `distuv` 求 p 或 insyra 直接出）、Fisher 精确 2×2
- **相关性**：Pearson（gonum）、Spearman（montanaflynn）、Kendall τ（gonum）
- **分布检验**：Lilliefors 正态性（KS 校正）、双样本 KS
- **描述统计**：Mean/Median/SD/IQR/分位数，供论文表格基线数据
- 统一入口 `Analyze` 返回结构化结果：选定检验、统计量、df、p 值、效应量（可得项）、每项假设校验 PASS/FAIL + 白话补救
- **默认输出**结构化 JSON（必出）；报告/图表未来版本实现（本期不实现）

### 不包含（本期）
- R 引擎执行路径（SEM/CFA/生存分析/Shapiro-Wilk/贝叶斯）——二期；分层分发协议本期预留 `--engine` 旗标位
- 图表渲染（SVG/PDF）与 Markdown 报告落盘——未来版本；本期只出结构化结果
- 效应量全套（Cohen's d/Cramer's V/η²）——随 insyra 核实后按需补
- 概率分布全套封装，仅暴露检验所需最小面

## 分层设计

新增**叶子层** `internal/stats`（纯计算，零网络、零 I/O，不 import 上层）。被 `internal/core`（服务层）编排调用；CLI `litkit stats` 子命令为入口。

### internal/stats（叶子层）
- `analyze.go`：`Analyze` 统一入口（两组比较决策链，含假设校验自动降级）
- `tests.go`：封装各库假设检验（t/配对/Mann-Whitney ← go-moremath；Wilcoxon/ANOVA/Kruskal ← insyra）
- `chi2.go`：分类变量卡方 + Fisher 精确
- `corr.go`：Pearson/Spearman/Kendall（gonum/montanaflynn）
- `normality.go`：正态性判定（Lilliefors/KS）→ 驱动 t 系 vs 非参数系
- `desc.go`：描述统计（均值/中位数/SD/分位数）
- 依赖：gonum + go-moremath + montanaflynn + insyra + fisherexact，全部隔离在本包内，core 不直接感知上游

### 入口层
`litkit stats --data x.csv`（默认出结构化 JSON；`--report` 等图表/报告旗标留待未来版本；`--engine r` 旗标位预留，二期实现）

### 仓库归属：独立统计仓库，直连接入 litkit

统计模块**独立建仓**（占位名 `gostats`，起仓前查重），由 litkit 作为**首个消费者**直接接入；同时作为独立组件，供其他 Go 工具（进程内 import）与他人/AI（CLI exe，JSON 协议）复用。

- **两个产物**：
  - `stats/` Go module：纯计算包（本设计文档的算源五库全部隔离在层内，`Result` 契约在这一层定义）
  - `cmd/gostats/` CLI exe：`JSON stdin/stdout`，供非 Go 工具与 AI agent 调用
- **接入 litkit**：litkit `go.mod` 用 `replace github.com/<owner>/gostats => ../gostats` 本地联动开发（v0.x 阶段），稳定后锁定 tag 摘 replace
- gostats CLI exe 与未来的 R 引擎采用**同一协议模式**（子进程 + JSON），litkit 内部形成对称路由：`--engine go`（进程内 import gostats）/ `--engine r`（子进程）
- **oracle 基建（R 对拍脚本、标准答案、对拍工具）归属 stats 仓**，litkit 侧不重复建
- litkit 只依赖 gostats 的**公共 API** 与外部契约，不感知其上游五库（gonum/insyra 等）

## 关键规则/约束

- 统计正确性属"不可省"硬约束：p 值/自由度一律由上游库计算，禁止手写近似公式
- 分层单向：`internal/stats` 不得 import `internal/core` 或入口层
- 默认零冗余：普通调用只输出结构化 JSON；报告/图表按旗标显式触发（本期未实现）
- 结构化结果字段最小化：检验/统计量/df/p/效应量/假设校验项，够 AI 判断即可
- 假设校验失败自动降级：正态不成立 → 自动落非参数并列明降级原因到结果
- 引用多库时必须标注各函数归属，避免误判某检验的算源
- insyra 启用前提：对应检验先通过 R oracle 对拍测试；未过对拍的 insyra 函数不暴露
- 禁止 cgo 嵌 R；R 引擎（二期）只走子进程 + JSON 协议

## 测试要求

- [ ] TDD：`Analyze` 决策链——正态数据走 t / 非正态自动落 Mann-Whitney（教科书已知数据对拍）
- [ ] t/配对 t/Mann-Whitney 各一个已知数据用例（go-moremath 直出），断言统计量/p 与教科书或 SPSS 一致
- [ ] Wilcoxon 符号秩/ANOVA/Kruskal-Wallis 已知数据用例（insyra），**须先与 R 对拍建立标准答案**再断言
- [ ] 卡方 2×2：断言 χ²、df、p 与手工计算一致；Fisher 精确小样本用例
- [ ] 相关：Pearson/Spearman/Kendall 已知数列断言 r/ρ
- [ ] 正态性：正态样本 PASS、偏态/离群样本 FAIL 且降级路径正确
- [ ] `Result` JSON 序列化：默认字段最小集，无报告冗余
- [ ] gate 门禁：go.mod 新增五依赖过 lint/vulncheck/arch

## 对外对接契约（未来接入方必读）

litkit 是 stats 仓的**首个消费者**，未来其他 Go 工具 / 非 Go 工具 / AI agent 均通过同一契约接入。以下为对接时必须遵守的提醒：

**Result 数据契约**
- `Result` 字段**只增不改名、不删**；新增字段必须向后兼容——这是所有消费者的唯一稳定契约
- 字段最小化：检验/统计量/df/p/效应量/假设校验项，够下游判断即可；报告/图表不进默认结果

**CLI 协议（cmd/gostats）**
- 统一 `JSON stdin/stdout`；进程退出码语义化（0=成功，非 0=错误类型），AI 与编程工具共用此协议
- 必带 `--version`：输出 gostats 自身版本 + 上游五库版本，便于对接方记录/复现环境
- 输入校验返回明确错误类型（样本量不足、分组不一致），而非静默 NaN

**引擎路由**
- `--engine go`（默认，进程内）/ `--engine r`（子进程，二期）；无 R 时明确报错并回退纯 Go
- 缺 R 专属检验（SEM/生存分析等）如实报"需 R 引擎"，不伪造结果

**对接边界**
- litkit 只依赖 gostats 公共 API 与外部契约，不感知上游五库
- oracle 基建（R 对拍脚本 / 标准答案 / 对拍工具）归 stats 仓，下游不重复建；正确性证明件由 stats 仓维护

## 备注

- PRD 需新增 FR-STAT 系列后再过同步校验；本期为 draft，先定设计
- go-moremath/insyra 均 v0.x，封装层隔离其上游变更，避免波及 core
- 二手资料宣传与实际 API 有落差（例：多来源声称 gonum 有完整假设检验），一律以 pkg.go.dev 实测函数清单为准
