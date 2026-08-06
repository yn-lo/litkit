-- schema.sql 本地文献库 Schema（FR-LIB）。
-- 模型以 .sql 文件统一管理：internal/storage 在 Open 时幂等执行（CREATE TABLE IF NOT EXISTS）。

-- papers 论文总表（FR-LIB-01/03）。
-- 每篇论文一条记录；dedup_key 唯一（DOI > title+authors > paper_id 三级，FR-SEARCH-02 语义入库）。
-- cite_key 为 3 字母引用标识（FR-LIB-06），AI 引用的唯一入口。
CREATE TABLE IF NOT EXISTS papers (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    dedup_key  TEXT UNIQUE NOT NULL,
    cite_key   TEXT UNIQUE NOT NULL,
    doi        TEXT NOT NULL DEFAULT '',
    paper_id   TEXT NOT NULL DEFAULT '',
    title      TEXT NOT NULL,
    authors    TEXT NOT NULL DEFAULT '[]',  -- JSON 数组 [{"given","family"}]
    abstract   TEXT NOT NULL DEFAULT '',
    year       INTEGER NOT NULL DEFAULT 0,
    venue      TEXT NOT NULL DEFAULT '',
    source     TEXT NOT NULL DEFAULT '',
    doc_type   TEXT NOT NULL DEFAULT '',
    url        TEXT NOT NULL DEFAULT '',
    pdf_url    TEXT NOT NULL DEFAULT '',   -- 全文可用的 PDF 直链（FR-FETCH-02）
    pmid       TEXT NOT NULL DEFAULT '',
    arxiv_id   TEXT NOT NULL DEFAULT '',
    volume     TEXT NOT NULL DEFAULT '',
    number     TEXT NOT NULL DEFAULT '',
    pages      TEXT NOT NULL DEFAULT '',
    fulltext   TEXT NOT NULL DEFAULT '',   -- 抽取的全文缓存（FR-FETCH-04，按 dedup_key 缓存）
    citations  INTEGER NOT NULL DEFAULT 0,
    fetched_at TEXT NOT NULL,
    publisher    TEXT NOT NULL DEFAULT '',  -- 出版社（书籍 [M] 用）
    city         TEXT NOT NULL DEFAULT '',  -- 出版地（书籍 [M] 用）
    institution  TEXT NOT NULL DEFAULT '',  -- 学位授予机构（学位论文 [D] 用）
    access_date  TEXT NOT NULL DEFAULT ''   -- 网页访问日期（网页用）
);

CREATE INDEX IF NOT EXISTS idx_papers_doi     ON papers(doi);
CREATE INDEX IF NOT EXISTS idx_papers_source  ON papers(source);
CREATE INDEX IF NOT EXISTS idx_papers_title   ON papers(title);

-- paper_refs 引用标记表（FR-LIB-07）。
-- 记录"手稿中哪句话引用哪篇文献"——由 verify 流程全量扫描手稿维护。
-- (cite_key, sentence_hash, manuscript) 唯一：同句同引同手稿幂等。
-- 该表是 citation_scores 的事实基础，两者解耦不共享主键。
CREATE TABLE IF NOT EXISTS paper_refs (
    id            INTEGER PRIMARY KEY AUTOINCREMENT,
    cite_key      TEXT NOT NULL REFERENCES papers(cite_key) ON DELETE CASCADE,
    sentence_hash TEXT NOT NULL,           -- sha256 前缀指纹
    manuscript    TEXT NOT NULL,            -- 手稿文件名（相对 WORK_DIR）
    sentence      TEXT NOT NULL DEFAULT '', -- 引用句原文
    created_at    TEXT NOT NULL,
    UNIQUE(cite_key, sentence_hash, manuscript)
);

CREATE INDEX IF NOT EXISTS idx_refs_manuscript  ON paper_refs(manuscript);
CREATE INDEX IF NOT EXISTS idx_refs_cite_key    ON paper_refs(cite_key);

-- citation_scores LLM 引用相关性评分缓存（FR-LINT-08）。
-- 主键 (cite_key, sentence_hash, model_id, prompt_version) 自带失效语义：
--   - 句子改了 → sentence_hash 变 → 自动不命中
--   - prompt 升级 → prompt_version 变 → 自动不命中
--   - 换模型 → model_id 变 → 自动不命中
-- 无 TTL——库生命周期由工作目录决定（FR-LIB-03），评分结果随库文件走。
-- 不带 manuscript 字段：同句同引同模型同 prompt 跨手稿共享评分（LLM 评分是纯函数）。
CREATE TABLE IF NOT EXISTS citation_scores (
    id             INTEGER PRIMARY KEY AUTOINCREMENT,
    cite_key       TEXT NOT NULL REFERENCES papers(cite_key) ON DELETE CASCADE,
    sentence_hash  TEXT NOT NULL,
    model_id       TEXT NOT NULL,
    prompt_version TEXT NOT NULL,
    score          REAL NOT NULL CHECK(score >= 0.0 AND score <= 1.0),
    rationale      TEXT NOT NULL DEFAULT '',
    scored_at      TEXT NOT NULL,
    UNIQUE(cite_key, sentence_hash, model_id, prompt_version)
);

CREATE INDEX IF NOT EXISTS idx_scores_lookup ON citation_scores(cite_key, sentence_hash);
CREATE INDEX IF NOT EXISTS idx_scores_model  ON citation_scores(model_id);
