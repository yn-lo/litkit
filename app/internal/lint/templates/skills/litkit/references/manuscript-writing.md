# 论文撰写

## 手稿规范

- 手稿文件（`manuscript/*.md`）**仅含正文**，不写摘要、关键词、参考文献列表
- 引用用 `[@<citeKey>]` 占位符，不展开元数据
- 撰写硬性规定见 `.litkit/<type-lang>/manuscript-spec.yaml`

## 验证命令

```bash
litkit verify manuscript/*.md --type <type> --lang <lang> [选项]
```

## 选项

| 选项 | 默认值 | 说明 |
|---|---|---|
| `--type` | 自动检测 | 论文类型：review / empirical |
| `--lang` | `zh` | 写作语言：zh / en |
| `--mode` | `draft` | 验证模式：chapter / draft / final（递增启用规则） |
| `--check` | 全部 | 仅运行指定检查类别（逗号分隔） |
| `--skip-check` | 无 | 跳过指定检查类别（逗号分隔） |
| `--rule` | 全部 | 仅运行指定规则 ID（逗号分隔，如 R2.1,R7.1） |
| `--skip` | 无 | 跳过指定规则 ID |
| `--report` | `json` | 报告格式：json / citation-refs（引用评分，需 LLM） |

## 检查类别

| 类别 | 说明 | 对应规则 |
|---|---|---|
| `language` | 语言合规 | R0.1, R0.2 |
| `structure` | 章节结构 | R1.1, R1.4-R1.8 |
| `heading` | 标题规范 | R1.2, R1.3, R7.1 |
| `statistics` | 统计格式 | R2.1 |
| `punctuation` | 标点符号 | R3.1, R3.2 |
| `style` | 行文风格 | R4.2 |
| `citation` | 引用规范 | R5.1-R5.3, R6.1 |
| `boast_words` | 自我夸大/AI痕迹 | R7.2 |
| `word_counts` | 字数统计 | R8.1-R8.3 |
| `todo` | 用户标记 | R9.1 |

## 验证模式

| 模式 | 启用规则范围 |
|---|---|
| `chapter` | 结构检查 |
| `draft` | + 数据/标点/引用 |
| `final` | + 字数/行文 |

## 退出码

- `0`：全部通过或仅 S 类需人工复核
- `1`：有 A 类违规，需修复后重跑
