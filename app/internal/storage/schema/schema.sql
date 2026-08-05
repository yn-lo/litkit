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
    pmid       TEXT NOT NULL DEFAULT '',
    arxiv_id   TEXT NOT NULL DEFAULT '',
    volume     TEXT NOT NULL DEFAULT '',
    number     TEXT NOT NULL DEFAULT '',
    pages      TEXT NOT NULL DEFAULT '',
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
