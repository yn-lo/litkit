# 文献检索

## 检索命令

```bash
litkit search "<英文关键词>" [选项]
litkit search "<中文关键词>" -s crossref,doaj     # 中文文献
```

## 选项

| 选项 | 默认值 | 说明 |
|---|---|---|
| `-s, --sources` | 默认参与检索的源 | 源过滤（逗号分隔；默认源受 `.env` 白名单/排除控制） |
| `-n, --max-results` | `5` | 每源检索条数 |
| `--mode` | `tiab` | 检索等级：tiab（题目+摘要+关键词）/ full（全文） |
| `--years` | `3` | 最近 N 年 |
| `--since` | - | 起始年份（优先于 --years） |
| `-y, --year` | - | 精确单年过滤 |
| `--keep-no-abstract` | 关 | 保留无摘要论文（默认过滤） |
| `--exclude` | - | 排除词（标题/摘要命中即剔除，先排除后入库） |
| `--no-store` | 关 | 探索模式：只检索不入库（citeKey 为空） |
| `--full` | 关 | 输出完整元数据与完整错误 |

## 输出字段

默认精简视图：`citeKey` / `title` / `firstAuthor` / `year` / `venue` / `docType` / `citations` / `abstract`。
`venue` + `docType` 用于区分顶刊论文与预印本；`citations` 是证据分量参考。
`filteredNoAbstract` 表示因无摘要被剔除的条数——`total=0` 且该字段 >0 时不是检索失败。

## 默认源配置

`.env` 可收窄未显式 `-s` 时的检索范围（`-s` 一旦指定则完全覆盖）：

```dotenv
LITKIT_SEARCH_SOURCES=pubmed,openalex,semantic_scholar   # 白名单，留空=默认参与检索的源
LITKIT_SEARCH_EXCLUDE_SOURCES=crossref,doaj              # 排除源
```

未知源名会告警并忽略；过滤后无可用源则报错。`litkit sources` 的 `enabled` 反映该配置。
`biorxiv` / `medrxiv` 无关键词检索端点（只能拉最近批次本地过滤），默认不参与检索，需 `-s` 显式指定。

## 检索策略

- **英文源用英文关键词**（arxiv/pubmed/openalex/semantic_scholar 均为英文语料）
- **中文文献**：`-s crossref,doaj` + 中文关键词（命中中文学报/中文 OA 期刊的标题与摘要）
- 多词检索按词项 AND 组合（不必写成短语）；已含引号/布尔语法时原样下发
- 默认 tiab 模式；结果不足时用 `--mode full`（pubmed/arxiv/openalex 支持区分等级）
- 默认最近 3 年；可 `--years 10` 或 `--since 2015` 放宽
- 检索词拿不准时先加 `--no-store` 试跑（不入库、无 citeKey），确认相关后再正式检索
- 正式检索自动入库，返回 citeKey 句柄

## 文献库管理

```bash
litkit lib get Kxq Abc        # 按 citeKey 取完整元数据（含摘要）——修订文稿时用这个
litkit lib list               # 列出文献（精简视图）
litkit lib search "<关键词>"   # 按标题/作者搜索
litkit lib add --doi 10.1000/xyz         # 标识符反查入库（也支持 --pmid / --arxiv）
litkit lib add -               # 手动录入：从 stdin 读 JSON（必填 title + abstract）
litkit lib stats              # 库统计
litkit lib path               # 库文件路径
litkit lib rm Kxq             # 删除文献（连带清理已下载 PDF）
```

## 全文获取（按需）

```bash
litkit fetch Kxq              # 或 litkit fetch 10.1000/xyz
```

默认基于摘要写作。仅当需要核对具体实验细节/原文数据时取全文：
Unpaywall OA → Sci-Hub 兜底 → PDF 落盘 `WORK_DIR/downloads/<citeKey>.pdf` → 全文入库缓存。
需 `LITKIT_UNPAYWALL_EMAIL`；库中已有缓存时零网络返回。

## 注意事项

- 无摘要的论文默认过滤（`--keep-no-abstract` 可保留）；入库元数据必须含摘要
- 检索本身不下载 PDF、不抽取全文（全文由 `litkit fetch` 单独按需触发）
