# litkit — AI agent 使用说明

> litkit工具使用地图，由 `litkit init` 生成。
> 撰写硬性规定见 `.litkit/<type-lang>/manuscript-spec.yaml`（AI 必读）。

## 核心命令

- `litkit search "<英文关键词>"` — 跨源检索并自动入库
  - `-s arxiv,pubmed` 源过滤；`-n 5` 每源条数；`--mode tiab|full` 检索等级
- `litkit lib list / search / rm / stats / path` — 文献库管理
- `litkit verify manuscript/*.md --type <type> --lang <lang>` — 验证文稿格式

## 论文类型

每种类型在 `.litkit/<type-lang>/` 下有独立阈值配置（`manuscript-spec.yaml`）。
查看已注册类型：`ls .litkit/`；追加：`litkit init --type review|empirical --lang zh|en`

## 检索策略

- 检索词**必须英文**（各源英文语料为主）
- 默认 tiab=题目+摘要+关键词；结果不足时用 `--mode full` 全文检索
- 默认最近 3 年；可 `--years 10` 或 `--since 2015` 放宽

## 撰写注意事项

- 手稿文件（`manuscript/*.md`）**仅含正文**，不写摘要、关键词、参考文献列表
- 引用用 `[@<citeKey>]` 占位符，不展开元数据
- 所有撰写硬性规定（字数、引用数、章节结构、标题层级、格式要求等）见 `.litkit/<type-lang>/manuscript-spec.yaml` 顶部注释

## 重要规则（AI 必读）

- **严禁编造文献**：所有引用必须来自文献库（`litkit lib list` 或 `litkit search` 获得），不得凭空生成 citeKey 或捏造 DOI/作者/标题
- **严禁捏造事实**：所有数据、统计结果、结论必须有文献支撑，不得虚构研究结果
- **严禁篡改数据**：引用文献的结论、数字、统计量不得曲解或篡改以符合论点
- **引用即查**：每次引用前必须用 `litkit lib search` 确认该论文确实存在于库中，且元数据与上下文一致