# 综述叙事格式（Review）

综述（叙事性综述）八股叙事，中英文同构。章节顺序：引言 → 文献检索方法 → 主题分析 → 讨论与展望 → 结论（zh）/ Introduction → Literature Search Methods → Thematic Synthesis → Discussion & Outlook → Conclusion（en），以 `.litkit/review-<lang>/manuscript-spec.yaml` 的 `sections` 为唯一顺序源。

## 1 引言（Introduction）

漏斗式：主题背景与意义 → 研究现状 → 综述必要性/范围 → 综述目的。

## 2 文献检索方法（Literature Search Methods）

可复现检索：数据库、检索时限、检索词（中英文）、纳入与排除标准、最终纳入文献数（可配筛选流程图，PRISMA 式）。说明检索策略可与重现。

## 3 主题分析（Thematic Synthesis）：综述主体

按主题/亚主题分层组织，以**主题为主线**而非按文献罗列：

- 先交代整体图景（分类框架 / 主线）
- 再逐主题整理已有证据与共识，主题间注意逻辑递进与对照
- 每主题以证据说话，**忌流水账**（避免"××研究发现……××研究发现……"逐条堆砌）

## 4 讨论与展望（Discussion & Outlook）

跨主题综合：提炼共识与分歧 → 指出现有研究局限 → 提出未来研究启示与临床/政策意义。

## 5 结论（Conclusion）

2–3 句概括性结论，呼应综述目的。

## 示例骨架

```markdown
# 老年疼痛恐惧护理干预研究进展

# 1 引言        （漏斗：主题意义→现状→综述必要性→目的）

# 2 文献检索方法  （数据库/时限/检索词/纳入排除标准/纳入篇数）

# 3 主题分析
## 3.1 评估工具
## 3.2 干预策略
## 3.3 效果与不足

# 4 讨论与展望    （共识→分歧→局限→未来方向）

# 5 结论
```

> 语言风格、承重、AI 痕迹判断 → academic-style；阈值/编号 → spec.yaml + litkit verify。