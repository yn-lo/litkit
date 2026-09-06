---
name: manuscript-format
description: 学术论文叙事格式（"科研八股文"）总技能，中英文通用（同一类型的八股叙事中英同构）：按文稿类型细分叙事格式见 references/（实证/综述/书稿）。起草/规划/修改论文时，动笔前先读本技能并按类型读对应 reference；硬性阈值以 litkit verify 为准。
---

# 稿件叙事格式（Manuscript Format）

**动手写/改正文前先读**。本技能管"每章写什么、按什么顺序讲故事"——科研八股文的叙事骨架。硬性阈值（标题编号/字数/引用/P 值）交给 `litkit verify` 与 `manuscript-spec.yaml`；本章节构成与叙事顺序即此处规范。

**中英文同构**：同一文稿类型的八股叙事中英文一致；语言仅影响章节命名与阈值，命名对照见各 reference，阈值随 `.litkit/<type-lang>/` spec 区分。

职责分工：

| 层 | 归属 | 内容 |
|---|---|---|
| 叙事格式（本章构成、各章任务、写作顺序） | **本技能**（含 references/） | 八股骨架、段落使命 |
| 硬性阈值（标题编号/字数/引用/P 值） | `manuscript-spec.yaml` + `litkit verify` 规则 | R1.x / R2.x / R8.x |
| 语言风格（论证定位、承重、修辞、AI 痕迹） | **academic-style** 技能 | 判断性写作质量 |

## When to Use

- 起草正文、规划章节结构：按文稿类型读 references/ 对应文档搭骨架
- 修订成稿：核对章节齐全、各章完成叙事任务、结果-讨论是否对齐

## When NOT to Use

- 检索/引用/合规验证 → **litkit** 技能（阈值由 `litkit verify` 自动判定）
- 语言风格、段落承重、AI 痕迹判断 → **academic-style** 技能
- 具体阈值数字（字数区间、引用篇数）→ `manuscript-spec.yaml`（唯一事实源）

## 类型路由（references/ 按文稿类型，不按语言）

| 文稿类型 | 叙事文档 | 适用场景（章节命名对照） |
|---|---|---|
| 实证 empirical | [references/empirical.md](references/empirical.md) | 原始研究：zh「引言→资料与方法→结果→讨论→结论」/ en「Introduction→Methods→Results→Discussion→Conclusion」 |
| 综述 review | [references/review.md](references/review.md) | 叙事性综述：zh「引言→文献检索方法→主题分析→讨论与展望→结论」/ en「Introduction→Literature Search→Thematic Synthesis→Discussion & Outlook→Conclusion」 |
| 书稿 book | [references/book.md](references/book.md) | 书籍/专著：中文编号体系，章节为自洽论证单元 |

## 通用叙事骨架（所有类型共享）

全文只讲"一件事"（一条主线），整体呈**沙漏形**：宽（引言）→ 窄（方法+结果）→ 宽（讨论）。

- **引言**：漏斗式——领域背景 → 已有共识 → 核心缺口 → 本研究目的。交代"是什么、为什么做、做什么"
- **方法**：证明可复现。先一般资料（基线），再措施/步骤，再观察指标，最后统计
- **结果**：只陈述"发现了什么"，不解释"为什么"。顺序与方法指标一一对应
- **讨论**：解释"意味着什么"。开头回扣目的 → 逐点对齐结果（一段对应一个结果）→ 机制与意义 → 局限展望
- **结果-讨论对齐铁律**：**结果写了几个要点，讨论就按相同次序安排几段**

---

## 摘要（Abstract，结构式）

按「目的 / 方法 / 结果 / 结论」四段，全文故事浓缩，无引用，字数区间见 spec.yaml。

## 标题（Title）

三要素：研究对象 + 处理/干预因素 + 观察指标，可点明研究设计；信息型裸题优先，不用缩略语与商品名，字数限制见 spec.yaml。

> 每章的段落使命、小节顺序、示例标题按类型读 references/ 对应文档。