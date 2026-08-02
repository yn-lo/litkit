# 平台能力矩阵 — litkit（国内可达性视角）

---
last_updated: 2026-08-02
status: active
owner: litkit-core
---

> 检索：支持 / 可选；摘要：可用 / 无 / 部分
> 检索源摘要门槛：无摘要能力的源不纳入检索（FR-SRC-19）。
> 与 `litkit sources` 输出保持一致；M2 验收要求与实测一致。

## 一期默认启用（国内可达、稳定、大源，均含摘要）

| 平台 | 检索 | 摘要 | 说明 |
|---|:---:|:---:|---|
| arXiv | 支持 | 可用 | 偶发慢；Atom Feed；限速 1 req/3s |
| PubMed | 支持 | 可用 | NCBI EUtils XML；无 key 3 req/s |
| bioRxiv / medRxiv | 支持 | 可用 | REST JSON |
| Semantic Scholar | 支持 | 可用 | 403 自动降级；建议可选 key 提档 |
| OpenAlex | 支持 | 可用 | 覆盖 CORE / DOAJ / OpenAIRE 等聚合数据 |

## 反查源（非检索，供引用回填）

| 平台 | 用途 |
|---|---|
| CrossRef | DOI / title 元数据反查（FR-REF-02）；无摘要，不参与检索 |

## 二期（可选激活）

| 平台 | 检索 | 摘要 | 说明 |
|---|:---:|:---:|---|
| Zenodo | 支持 | 可用 | 机构仓储 |
| IEEE / ACM | key | 部分 | 需订阅 key；检索时剔除无摘要论文 |

## 移除（不实现）

| 平台 | 说明 |
|---|---|
| CORE / DOAJ / OpenAIRE | 数据被 OpenAlex 完整索引，适配收益低 |
| Unpaywall | OA 反查非检索源，与摘要工作流不匹配 |
| Europe PMC / PMC | 与 PubMed 数据重合 |
| HAL | 小众机构仓储，与 Zenodo 重合 |
| BASE | 需机构 IP，协议老旧 |
| SSRN / CiteSeerX | 端点不稳定 |
| dblp | 无摘要，直接不实现 |
| Google Scholar | 国内不可达，需代理 |
| Sci-Hub | 法律风险；摘要工作流不依赖下载 |

## 规则

- 新源接入必须更新本矩阵（检索/摘要/说明三列）
- 矩阵声明与实现不一致 = 文档腐化，违反 NFR-MAINT-04
