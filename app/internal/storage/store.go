// Package storage 基于 SQLite 的文献库持久化（叶子层）。
//
// 库文件跟随工作目录（FR-LIB-03：WORK_DIR/litkit.db），生命周期由工作目录
// 决定：删除工作目录即删除库，因此不引入 TTL/缓存清理（FR-CACHE 并入 FR-LIB）。
//
// 分层归属：叶子层，可被 internal/core 依赖；不 import 任何上层包。
package storage

import (
	"database/sql"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "modernc.org/sqlite" // 纯 Go SQLite 驱动

	"litkit/internal/model"
	"litkit/internal/util/textutil"
)

//go:embed schema/*.sql
var schemaFS embed.FS

// DefaultDBName 库文件名（FR-LIB-03：WORK_DIR/litkit.db）。
const DefaultDBName = "litkit.db"

// 存储常量（mnd：避免魔法值）。
const (
	defaultListLimit   = 100 // ListPapers 默认条数
	defaultSearchLimit = 50  // SearchLocal 默认条数
	dbDirPerm          = 0o750
	busyTimeoutMS      = 5000 // 多进程 CLI 并发写时等待锁的上限
)

// Stats 文献库统计（lib stats）。
type Stats struct {
	Total        int            `json:"total"`
	WithAbstract int            `json:"withAbstract"`
	WithDOI      int            `json:"withDOI"`
	BySource     map[string]int `json:"bySource"`
	DBPath       string         `json:"dbPath"`
}

// Store SQLite 文献库存储。
type Store struct {
	db   *sql.DB
	path string
}

// Open 打开（或创建）dbPath 处的 SQLite 库并执行 schema 迁移。
//
// 目录不存在时自动创建；多次 Open 同一路径幂等（CREATE TABLE IF NOT EXISTS）。
func Open(dbPath string) (*Store, error) {
	if strings.TrimSpace(dbPath) == "" {
		return nil, errors.New("storage: db path 为空")
	}
	dir := filepath.Dir(dbPath)
	if err := os.MkdirAll(dir, dbDirPerm); err != nil {
		return nil, fmt.Errorf("storage open: mkdir %s: %w", dir, err)
	}
	db, err := sql.Open("sqlite", filepath.ToSlash(dbPath))
	if err != nil {
		return nil, fmt.Errorf("storage open: %w", err)
	}
	// PRAGMA 是 per-connection 的，连接池新连接不继承；限制单连接保证
	// PRAGMA 始终生效，同时串行化全部操作，消除 check-then-act 竞态。
	// SQLite 写入本就要串行，读并发由 WAL 保证。
	db.SetMaxOpenConns(1)
	if err := db.Ping(); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("storage open ping: %w", err)
	}
	// WAL：读写并发；busy_timeout：缓解多进程 CLI 争锁
	if _, err := db.Exec("PRAGMA journal_mode=WAL"); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("storage pragma wal: %w", err)
	}
	if _, err := db.Exec(fmt.Sprintf("PRAGMA busy_timeout=%d", busyTimeoutMS)); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("storage pragma busy_timeout: %w", err)
	}

	s := &Store{db: db, path: dbPath}
	if err := s.migrate(); err != nil {
		_ = db.Close()
		return nil, err
	}
	return s, nil
}

// Close 关闭底层数据库连接。
func (s *Store) Close() error { return s.db.Close() }

// Path 返回库文件绝对路径。
func (s *Store) Path() string { return s.path }

// migrate 执行 schema/schema.sql（单文件管理全部 DDL）。
// CREATE TABLE IF NOT EXISTS，幂等。
func (s *Store) migrate() error {
	entries, err := schemaFS.ReadDir("schema")
	if err != nil {
		return fmt.Errorf("storage migrate: read schema dir: %w", err)
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".sql") {
			continue
		}
		data, rerr := schemaFS.ReadFile("schema/" + e.Name())
		if rerr != nil {
			return fmt.Errorf("storage migrate: read %s: %w", e.Name(), rerr)
		}
		if _, xerr := s.db.Exec(string(data)); xerr != nil {
			return fmt.Errorf("storage migrate: exec %s: %w", e.Name(), xerr)
		}
	}
	return s.ensureColumns()
}

// ensureColumns 为旧库补充新增列（ALTER TABLE ADD COLUMN 幂等：先查 PRAGMA table_info）。
func (s *Store) ensureColumns() error {
	const table = "papers"
	rows, err := s.db.Query("PRAGMA table_info(" + table + ")")
	if err != nil {
		return fmt.Errorf("storage ensure columns: %w", err)
	}
	have := map[string]bool{}
	for rows.Next() {
		var cid int
		var name, typ string
		var notNull, pk int
		var dflt any
		if err := rows.Scan(&cid, &name, &typ, &notNull, &dflt, &pk); err != nil {
			_ = rows.Close()
			return fmt.Errorf("storage ensure columns scan: %w", err)
		}
		have[name] = true
	}
	if err := rows.Close(); err != nil {
		return err
	}
	adds := []struct{ name, ddl string }{
		{"volume", "ALTER TABLE papers ADD COLUMN volume TEXT NOT NULL DEFAULT ''"},
		{"number", "ALTER TABLE papers ADD COLUMN number TEXT NOT NULL DEFAULT ''"},
		{"pages", "ALTER TABLE papers ADD COLUMN pages TEXT NOT NULL DEFAULT ''"},
		{"publisher", "ALTER TABLE papers ADD COLUMN publisher TEXT NOT NULL DEFAULT ''"},
		{"city", "ALTER TABLE papers ADD COLUMN city TEXT NOT NULL DEFAULT ''"},
		{"institution", "ALTER TABLE papers ADD COLUMN institution TEXT NOT NULL DEFAULT ''"},
		{"access_date", "ALTER TABLE papers ADD COLUMN access_date TEXT NOT NULL DEFAULT ''"},
		{"pdf_url", "ALTER TABLE papers ADD COLUMN pdf_url TEXT NOT NULL DEFAULT ''"},
		{"fulltext", "ALTER TABLE papers ADD COLUMN fulltext TEXT NOT NULL DEFAULT ''"},
	}
	for _, a := range adds {
		if !have[a.name] {
			if _, err := s.db.Exec(a.ddl); err != nil {
				return fmt.Errorf("storage ensure columns %s: %w", a.name, err)
			}
		}
	}
	return nil
}

// dedupKey 生成入库去重键：DOI > 归一化标题 > paper_id 三级，命中同一键视为同一篇论文。
// 与检索层 dedupPapers 保持一致（DOI > title > ID），避免两层去重行为分叉。
func dedupKey(p model.Paper) string {
	if doi := strings.ToLower(strings.TrimSpace(p.DOI)); doi != "" {
		return "doi:" + doi
	}
	if title := textutil.NormalizeTitle(p.Title); title != "" {
		return "title:" + title
	}
	return "id:" + strings.ToLower(strings.TrimSpace(p.ID))
}

// UpsertPaper 插入或更新一篇论文（FR-LIB-01/02/06）。
//
// 返回 (citeKey, inserted, error)：首次入库生成 3 字母 cite_key 且 inserted=true；
// 同 dedup_key 再次入库更新字段且 inserted=false、cite_key 保持不变。
func (s *Store) UpsertPaper(p model.Paper) (string, bool, error) {
	key := dedupKey(p)
	// DOI 统一小写入库，保证 GetByDOI 大小写不敏感命中（与 dedupKey 归一化一致）
	p.DOI = strings.ToLower(strings.TrimSpace(p.DOI))
	now := time.Now().Format(time.RFC3339)

	var id int64
	var citeKey string
	err := s.db.QueryRow("SELECT id, cite_key FROM papers WHERE dedup_key = ?", key).Scan(&id, &citeKey)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return "", false, fmt.Errorf("storage upsert select: %w", err)
	}

	authorsJSON, jerr := json.Marshal(p.Authors)
	if jerr != nil {
		return "", false, fmt.Errorf("storage upsert: marshal authors: %w", jerr)
	}

	if errors.Is(err, sql.ErrNoRows) {
		citeKey, err = s.newCiteKey()
		if err != nil {
			return "", false, err
		}
		_, err = s.db.Exec(`INSERT INTO papers
			(dedup_key, cite_key, doi, paper_id, title, authors, abstract, year, venue,
			 source, doc_type, url, pdf_url, pmid, arxiv_id, volume, number, pages, citations, fetched_at,
			 publisher, city, institution, access_date)
			VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
			key, citeKey, p.DOI, p.ID, p.Title, string(authorsJSON), p.Abstract, p.Year,
			p.Venue, p.Source, p.DocType, p.URL, p.PdfURL, p.PMID, p.ArXivID, p.Volume, p.Number, p.Pages,
			p.Citations, now, p.Publisher, p.City, p.Institution, p.AccessDate)
		if err != nil {
			return "", false, fmt.Errorf("storage upsert insert: %w", err)
		}
		return citeKey, true, nil
	}

	// 已存在：以最新数据覆盖字段，cite_key 保持不变；
	// abstract/venue/doi/pmid/arxiv_id/url/doc_type 新值为空时保留旧值，
	// 防止部分字段的再次入库擦除已有数据（NULLIF 空串转 NULL，COALESCE 回退旧值）
	_, err = s.db.Exec(`UPDATE papers SET
		doi=COALESCE(NULLIF(?, ''), doi), paper_id=?, title=?, authors=?,
		abstract=COALESCE(NULLIF(?, ''), abstract), year=?,
		venue=COALESCE(NULLIF(?, ''), venue), source=?,
		doc_type=COALESCE(NULLIF(?, ''), doc_type), url=COALESCE(NULLIF(?, ''), url),
		pdf_url=COALESCE(NULLIF(?, ''), pdf_url),
		pmid=COALESCE(NULLIF(?, ''), pmid), arxiv_id=COALESCE(NULLIF(?, ''), arxiv_id),
		volume=COALESCE(NULLIF(?, ''), volume),
		number=COALESCE(NULLIF(?, ''), number),
		pages=COALESCE(NULLIF(?, ''), pages),
		publisher=COALESCE(NULLIF(?, ''), publisher),
		city=COALESCE(NULLIF(?, ''), city),
		institution=COALESCE(NULLIF(?, ''), institution),
		access_date=COALESCE(NULLIF(?, ''), access_date),
		citations=?, fetched_at=?
		WHERE id=?`,
		p.DOI, p.ID, p.Title, string(authorsJSON),
		p.Abstract, p.Year, p.Venue, p.Source,
		p.DocType, p.URL, p.PdfURL, p.PMID, p.ArXivID,
		p.Volume, p.Number, p.Pages,
		p.Publisher, p.City, p.Institution, p.AccessDate,
		p.Citations, now, id)
	if err != nil {
		return "", false, fmt.Errorf("storage upsert update: %w", err)
	}
	return citeKey, false, nil
}

// UpsertPapers 批量入库；返回首次新增的论文数。
func (s *Store) UpsertPapers(papers []model.Paper) (int, error) {
	newCount := 0
	for _, p := range papers {
		_, inserted, err := s.UpsertPaper(p)
		if err != nil {
			return newCount, err
		}
		if inserted {
			newCount++
		}
	}
	return newCount, nil
}

// 查询列集合（Upsert 与查询共用，字段顺序必须与 scanPaper 一致）。
const paperCols = "cite_key, doi, paper_id, title, authors, abstract, year, venue, " +
	"source, doc_type, url, pdf_url, pmid, arxiv_id, volume, number, pages, citations, fetched_at, " +
	"publisher, city, institution, access_date"

// scanPapers 将结果行扫描为 []model.Paper。
func scanPapers(rows *sql.Rows) ([]model.Paper, error) {
	var out []model.Paper
	for rows.Next() {
		var (
			p           model.Paper
			authorsJSON []byte
			fetchedAt   string
		)
		if err := rows.Scan(&p.CiteKey, &p.DOI, &p.ID, &p.Title, &authorsJSON, &p.Abstract,
			&p.Year, &p.Venue, &p.Source, &p.DocType, &p.URL, &p.PdfURL, &p.PMID, &p.ArXivID,
			&p.Volume, &p.Number, &p.Pages, &p.Citations, &fetchedAt,
			&p.Publisher, &p.City, &p.Institution, &p.AccessDate); err != nil {
			return nil, err
		}
		if len(authorsJSON) > 0 {
			_ = json.Unmarshal(authorsJSON, &p.Authors)
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// GetByCiteKey 按 3 字母引用标识取论文；未命中返回 (nil, nil)。
func (s *Store) GetByCiteKey(citeKey string) (*model.Paper, error) {
	rows, err := s.db.Query("SELECT "+paperCols+" FROM papers WHERE cite_key = ? LIMIT 1", citeKey)
	if err != nil {
		return nil, fmt.Errorf("storage get by cite key: %w", err)
	}
	defer func() { _ = rows.Close() }()
	papers, err := scanPapers(rows)
	if err != nil {
		return nil, err
	}
	if len(papers) == 0 {
		return nil, nil
	}
	return &papers[0], nil
}

// GetByDOI 按 DOI 取论文（大小写不敏感）；未命中返回 (nil, nil)。
func (s *Store) GetByDOI(doi string) (*model.Paper, error) {
	rows, err := s.db.Query("SELECT "+paperCols+" FROM papers WHERE doi = ? LIMIT 1", strings.ToLower(strings.TrimSpace(doi)))
	if err != nil {
		return nil, fmt.Errorf("storage get by doi: %w", err)
	}
	defer func() { _ = rows.Close() }()
	papers, err := scanPapers(rows)
	if err != nil {
		return nil, err
	}
	if len(papers) == 0 {
		return nil, nil
	}
	return &papers[0], nil
}

// GetFulltext 按 cite_key 取回缓存的全文文本；未缓存返回空串（FR-FETCH-04）。
func (s *Store) GetFulltext(citeKey string) (string, error) {
	var text string
	err := s.db.QueryRow("SELECT fulltext FROM papers WHERE cite_key = ? LIMIT 1", citeKey).Scan(&text)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return "", fmt.Errorf("storage get fulltext: %w", err)
	}
	return text, nil
}

// SetFulltext 按 cite_key 写入全文缓存（覆盖旧值；论文不存在时静默忽略）。
func (s *Store) SetFulltext(citeKey, text string) error {
	if _, err := s.db.Exec("UPDATE papers SET fulltext = ? WHERE cite_key = ?", text, citeKey); err != nil {
		return fmt.Errorf("storage set fulltext: %w", err)
	}
	return nil
}

// SetPDFURL 按 cite_key 写入可用的 PDF 直链（论文不存在时静默忽略）。
func (s *Store) SetPDFURL(citeKey, pdfURL string) error {
	if _, err := s.db.Exec("UPDATE papers SET pdf_url = ? WHERE cite_key = ?", pdfURL, citeKey); err != nil {
		return fmt.Errorf("storage set pdf url: %w", err)
	}
	return nil
}

// ListPapers 列出库内论文。source 非空时按源过滤；orderBy 支持 "year"（年份倒序，year=0 排末尾）和 "id"（入库倒序，默认）。
func (s *Store) ListPapers(source, orderBy string, limit, offset int) ([]model.Paper, error) {
	if limit <= 0 {
		limit = defaultListLimit
	}
	order := "id DESC"
	switch orderBy {
	case "year":
		order = "year DESC, id DESC"
	case "id", "":
		// 保持默认
	default:
		return nil, fmt.Errorf("storage list: 未知排序 %q（可选 year|id）", orderBy)
	}
	var (
		rows *sql.Rows
		err  error
	)
	if source != "" {
		rows, err = s.db.Query("SELECT "+paperCols+" FROM papers WHERE source = ? ORDER BY "+order+" LIMIT ? OFFSET ?",
			source, limit, offset)
	} else {
		rows, err = s.db.Query("SELECT "+paperCols+" FROM papers ORDER BY "+order+" LIMIT ? OFFSET ?", limit, offset)
	}
	if err != nil {
		return nil, fmt.Errorf("storage list: %w", err)
	}
	defer func() { _ = rows.Close() }()
	return scanPapers(rows)
}

// SearchLocal 本地 keyword 检索（FR-LIB-04）：标题/作者/摘要 LIKE 匹配。
func (s *Store) SearchLocal(keyword string, limit, offset int) ([]model.Paper, error) {
	if limit <= 0 {
		limit = defaultSearchLimit
	}
	pattern := "%" + keyword + "%"
	rows, err := s.db.Query(`SELECT `+paperCols+` FROM papers
		WHERE title LIKE ? OR authors LIKE ? OR abstract LIKE ?
		ORDER BY year DESC, id DESC LIMIT ? OFFSET ?`, pattern, pattern, pattern, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("storage search local: %w", err)
	}
	defer func() { _ = rows.Close() }()
	return scanPapers(rows)
}

// Forget 删除一篇论文；返回是否实际删除（FR-LIB-02 删）。
func (s *Store) Forget(citeKey string) (bool, error) {
	res, err := s.db.Exec("DELETE FROM papers WHERE cite_key = ?", citeKey)
	if err != nil {
		return false, fmt.Errorf("storage forget paper: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("storage forget rows: %w", err)
	}
	return n > 0, nil
}

// Stats 返回文献库统计（lib stats）。
func (s *Store) Stats() (Stats, error) {
	st := Stats{BySource: map[string]int{}, DBPath: s.path}
	if err := s.db.QueryRow("SELECT COUNT(*) FROM papers").Scan(&st.Total); err != nil {
		return st, fmt.Errorf("storage stats total: %w", err)
	}
	if err := s.db.QueryRow("SELECT COUNT(*) FROM papers WHERE abstract != ''").Scan(&st.WithAbstract); err != nil {
		return st, fmt.Errorf("storage stats abstract: %w", err)
	}
	if err := s.db.QueryRow("SELECT COUNT(*) FROM papers WHERE doi != ''").Scan(&st.WithDOI); err != nil {
		return st, fmt.Errorf("storage stats doi: %w", err)
	}
	rows, err := s.db.Query("SELECT source, COUNT(*) FROM papers GROUP BY source ORDER BY COUNT(*) DESC")
	if err != nil {
		return st, fmt.Errorf("storage stats by_source: %w", err)
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var src string
		var n int
		if err := rows.Scan(&src, &n); err != nil {
			return st, err
		}
		st.BySource[src] = n
	}
	return st, rows.Err()
}
