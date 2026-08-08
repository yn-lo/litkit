# 文献检索

## 检索命令

```bash
litkit search "<英文关键词>" [选项]
```

## 选项

| 选项 | 默认值 | 说明 |
|---|---|---|
| `-s, --sources` | `arxiv,pubmed` | 源过滤（逗号分隔） |
| `-n, --num` | `5` | 每源检索条数 |
| `--mode` | `tiab` | 检索等级：tiab（题目+摘要+关键词）/ full（全文） |
| `--years` | `3` | 最近 N 年 |
| `--since` | - | 起始年份（优先于 --years） |

## 检索策略

- 检索词**必须英文**（各源英文语料为主）
- 默认 tiab 模式；结果不足时用 `--mode full` 全文检索
- 默认最近 3 年；可 `--years 10` 或 `--since 2015` 放宽
- 检索结果自动入库，返回 citeKey/title/firstAuthor/year/abstract

## 文献库管理

```bash
litkit lib list              # 列出全部文献
litkit lib search "<关键词>"  # 按标题/作者搜索
litkit lib stats             # 库统计
litkit lib path              # 库文件路径
litkit lib rm <citeKey>      # 删除文献
```

## 注意事项

- 无摘要的论文默认过滤（FR-SEARCH-03）
- 入库元数据必须含摘要
- 不下载 PDF、不抽取全文
