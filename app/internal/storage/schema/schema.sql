-- schema.sql 本地文献库 Schema（FR-LIB）。
-- 模型以 .sql 文件统一管理：internal/storage 在 Open 时幂等执行（CREATE TABLE IF NOT EXISTS）。
-- 建表顺序：papers 先于 paper_refs，保证外键目标表存在。

-- papers 论文总表（FR-LIB-01/03）。
-- 每篇论文一条记录；dedup_key 唯一（DOI > title+authors > paper_id 三级，FR-SEARCH-02 语义入库）。
-- cite_key 为 3 字母引用标识（FR-LIB-06），AI 引用与 paper_refs 的唯一入口。
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
    citations  INTEGER NOT NULL DEFAULT 0,
    fetched_at TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_papers_doi     ON papers(doi);
CREATE INDEX IF NOT EXISTS idx_papers_source  ON papers(source);
CREATE INDEX IF NOT EXISTS idx_papers_title   ON papers(title);

-- paper_refs 引用标记表（FR-LIB-06/07）。
-- 记录"哪句话引用了哪篇文献"：cite_key → papers.cite_key；
-- (cite_key, sentence_hash, manuscript) 唯一，同句重复引用不产生重复行。
CREATE TABLE IF NOT EXISTS paper_refs (
    id            INTEGER PRIMARY KEY AUTOINCREMENT,
    cite_key      TEXT NOT NULL,
    sentence_hash TEXT NOT NULL,
    sentence_text TEXT NOT NULL,
    manuscript    TEXT NOT NULL DEFAULT '',
    created_at    TEXT NOT NULL,
    UNIQUE(cite_key, sentence_hash, manuscript),
    FOREIGN KEY (cite_key) REFERENCES papers(cite_key)
);

CREATE INDEX IF NOT EXISTS idx_refs_cite_key ON paper_refs(cite_key);
