// Package storage 基于 SQLite 的文献库持久化（叶子层）。
//
// 库文件跟随工作目录（FR-LIB-03：WORK_DIR/.litkit/litkit.db），生命周期由工作目录
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

// DefaultDBName 库文件名（FR-LIB-03：WORK_DIR/.litkit/litkit.db）。
const DefaultDBName = "litkit.db"

// DBPath 返回工作目录下数据库文件的绝对路径（.litkit/litkit.db）。
func DBPath(workDir string) string {
	return filepath.Join(workDir, ".litkit", DefaultDBName)
}

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
	db, err := sql.Open("sqlite", filepath.ToSlash(dbPath)+"?_txlock=immediate")
	if err != nil {
		return nil, fmt.Errorf("storage open: %w", err)
	}
	// PRAGMA 是 per-connection 的，连接池新连接不继承；限制单连接保证
	// PRAGMA 始终生效；查后写的原子性另由 IMMEDIATE 事务保证。
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
	if _, err := db.Exec("PRAGMA foreign_keys=ON"); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("storage pragma foreign_keys: %w", err)
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

// migrate 执行 schema/schema.sql（单文件管理全部 DDL），并在同一事务内补齐旧库新增列。
// CREATE TABLE IF NOT EXISTS，幂等；任一步失败整体回滚，避免半迁移状态。
func (s *Store) migrate() error {
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("storage migrate: begin: %w", err)
	}
	defer func() { _ = tx.Rollback() }() // Commit 后 Rollback 返回 ErrTxDone，无副作用

	entries, err := schemaFS.ReadDir("schema")
	if err != nil {
		return fmt.Errorf("storage migrate: read schema dir: %w", err)
	}
	var stmts []string
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".sql") {
			continue
		}
		data, rerr := schemaFS.ReadFile("schema/" + e.Name())
		if rerr != nil {
			return fmt.Errorf("storage migrate: read %s: %w", e.Name(), rerr)
		}
		stmts = append(stmts, string(data))
	}
	if err := runMigration(tx, stmts); err != nil {
		return err
	}
	return tx.Commit()
}

// runMigration 在同一事务内执行全部 schema DDL 与旧库补列；任一步失败由调用方回滚。
func runMigration(tx *sql.Tx, stmts []string) error {
	for i, q := range stmts {
		if _, err := tx.Exec(q); err != nil {
			return fmt.Errorf("storage migrate: exec schema #%d: %w", i+1, err)
		}
	}
	return ensureColumns(tx)
}

// ensureColumns 为旧库补充新增列（ALTER TABLE ADD COLUMN 幂等：先查 PRAGMA table_info）。
func ensureColumns(tx *sql.Tx) error {
	const table = "papers"
	rows, err := tx.Query("PRAGMA table_info(" + table + ")")
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
		{"language", "ALTER TABLE papers ADD COLUMN language TEXT NOT NULL DEFAULT ''"},
		{"pdf_url", "ALTER TABLE papers ADD COLUMN pdf_url TEXT NOT NULL DEFAULT ''"},
		{"fulltext", "ALTER TABLE papers ADD COLUMN fulltext TEXT NOT NULL DEFAULT ''"},
	}
	for _, a := range adds {
		if !have[a.name] {
			if _, err := tx.Exec(a.ddl); err != nil {
				return fmt.Errorf("storage ensure columns %s: %w", a.name, err)
			}
		}
	}
	return nil
}

// dedupKey 生成入库去重键：DOI > title+authors > paper_id 三级，命中同一键视为同一篇论文。
// 无 DOI 时把第一作者纳入键，避免「Editorial」这类通用标题把不同文献合并覆盖（FR-CACHE-02）。
func dedupKey(p model.Paper) string {
	if doi := strings.ToLower(strings.TrimSpace(p.DOI)); doi != "" {
		return "doi:" + doi
	}
	if title := textutil.NormalizeTitle(p.Title); title != "" {
		if sig := authorSignature(p.Authors); sig != "" {
			return "title:" + title + "|" + sig
		}
		return "title:" + title
	}
	return "id:" + strings.ToLower(strings.TrimSpace(p.ID))
}

// authorSignature 返回第一作者的归一化签名（Family+Given，忽略大小写与空白）。
func authorSignature(as []model.Author) string {
	if len(as) == 0 {
		return ""
	}
	return strings.ToLower(strings.Join(strings.Fields(as[0].Family+" "+as[0].Given), ""))
}

// authorsCompatible 判断两条记录的作者是否可视为同一文献：
// 任一方无作者时保持旧行为（允许补齐元数据），双方都有作者时必须一致。
func authorsCompatible(existingJSON []byte, incoming []model.Author) bool {
	if len(existingJSON) == 0 {
		return true
	}
	var existing []model.Author
	if err := json.Unmarshal(existingJSON, &existing); err != nil {
		return true
	}
	es, is := authorSignature(existing), authorSignature(incoming)
	return es == "" || is == "" || es == is
}

// findPaper 在写事务内按强标识、归一化标题回退匹配；冲突 DOI 不合并。
// ponytail: 精确键未命中时 O(n) 扫描，适用于 ≤1 万条本地库；更大库升级为索引化标题键。
func findPaper(tx *sql.Tx, p model.Paper) (id int64, citeKey string, err error) {
	err = tx.QueryRow("SELECT id, cite_key FROM papers WHERE dedup_key = ?", dedupKey(p)).Scan(&id, &citeKey)
	if !errors.Is(err, sql.ErrNoRows) {
		return id, citeKey, err
	}
	rows, err := tx.Query("SELECT id, cite_key, title, paper_id, authors FROM papers WHERE ? = '' OR doi = '' OR doi = ?", p.DOI, p.DOI)
	if err != nil {
		return 0, "", err
	}
	defer func() { _ = rows.Close() }()
	title := textutil.NormalizeTitle(p.Title)
	matches := 0
	for rows.Next() {
		var candidateID int64
		var candidateKey, candidateTitle, paperID string
		var candidateAuthors []byte
		if err := rows.Scan(&candidateID, &candidateKey, &candidateTitle, &paperID, &candidateAuthors); err != nil {
			return 0, "", err
		}
		if p.ID != "" && p.ID == paperID && authorsCompatible(candidateAuthors, p.Authors) {
			return candidateID, candidateKey, nil
		}
		if title != "" && title == textutil.NormalizeTitle(candidateTitle) && authorsCompatible(candidateAuthors, p.Authors) {
			id, citeKey = candidateID, candidateKey
			matches++
		}
	}
	if err := rows.Err(); err != nil {
		return 0, "", err
	}
	if matches > 1 {
		return 0, "", errors.New("storage: 标题匹配多个条目，请提供 DOI 明确身份")
	}
	if matches == 0 {
		return 0, "", sql.ErrNoRows
	}
	return id, citeKey, nil
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

	// 在首次 SELECT 前获取写锁，避免并发进程同时认定条目/引用键不存在。
	tx, err := s.db.Begin()
	if err != nil {
		return "", false, fmt.Errorf("storage upsert begin: %w", err)
	}
	defer func() { _ = tx.Rollback() }() // 已提交时为 ErrTxDone；错误路径释放写锁。
	id, citeKey, err := findPaper(tx, p)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return "", false, fmt.Errorf("storage upsert select: %w", err)
	}

	authorsJSON, jerr := json.Marshal(p.Authors)
	if jerr != nil {
		return "", false, fmt.Errorf("storage upsert: marshal authors: %w", jerr)
	}

	if errors.Is(err, sql.ErrNoRows) {
		citeKey, err = newCiteKey(tx)
		if err != nil {
			return "", false, err
		}
		_, err = tx.Exec(`INSERT INTO papers
			(dedup_key, cite_key, doi, paper_id, title, authors, abstract, year, venue,
			 source, doc_type, url, pdf_url, pmid, arxiv_id, volume, number, pages, citations, fetched_at,
			 publisher, city, institution, access_date, language)
			VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
			key, citeKey, p.DOI, p.ID, p.Title, string(authorsJSON), p.Abstract, p.Year,
			p.Venue, p.Source, p.DocType, p.URL, p.PdfURL, p.PMID, p.ArXivID, p.Volume, p.Number, p.Pages,
			p.Citations, now, p.Publisher, p.City, p.Institution, p.AccessDate, p.Language)
		if err != nil {
			return "", false, fmt.Errorf("storage upsert insert: %w", err)
		}
		if err := tx.Commit(); err != nil {
			return "", false, fmt.Errorf("storage upsert commit: %w", err)
		}
		return citeKey, true, nil
	}

	// 使用合并后的身份生成键：题名更新时不能留下旧键，稀疏更新也不能降级 DOI。
	identity := p
	var oldDOI, oldTitle, oldID string
	if err := tx.QueryRow("SELECT doi, title, paper_id FROM papers WHERE id = ?", id).Scan(&oldDOI, &oldTitle, &oldID); err != nil {
		return "", false, fmt.Errorf("storage upsert identity: %w", err)
	}
	if identity.DOI == "" {
		identity.DOI = oldDOI
	}
	if identity.Title == "" {
		identity.Title = oldTitle
	}
	if identity.ID == "" {
		identity.ID = oldID
	}
	key = dedupKey(identity)
	// 缺失值保留原数据，保留原 citeKey 和引用关系。
	_, err = tx.Exec(`UPDATE papers SET
		dedup_key=?,
		doi=COALESCE(NULLIF(?, ''), doi), paper_id=COALESCE(NULLIF(?, ''), paper_id),
		title=COALESCE(NULLIF(?, ''), title), authors=CASE WHEN ? > 0 THEN ? ELSE authors END,
		abstract=COALESCE(NULLIF(?, ''), abstract), year=COALESCE(NULLIF(?, 0), year),
		venue=COALESCE(NULLIF(?, ''), venue), source=COALESCE(NULLIF(?, ''), source),
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
		language=COALESCE(NULLIF(?, ''), language),
		citations=COALESCE(NULLIF(?, 0), citations), fetched_at=?
		WHERE id=?`,
		key, p.DOI, p.ID, p.Title, len(p.Authors), string(authorsJSON),
		p.Abstract, p.Year, p.Venue, p.Source,
		p.DocType, p.URL, p.PdfURL, p.PMID, p.ArXivID,
		p.Volume, p.Number, p.Pages,
		p.Publisher, p.City, p.Institution, p.AccessDate, p.Language,
		p.Citations, now, id)
	if err != nil {
		return "", false, fmt.Errorf("storage upsert update: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return "", false, fmt.Errorf("storage upsert commit: %w", err)
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
	"publisher, city, institution, access_date, language"

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
			&p.Publisher, &p.City, &p.Institution, &p.AccessDate, &p.Language); err != nil {
			return nil, err
		}
		if len(authorsJSON) > 0 {
			// 历史脏数据（非 JSON）：视为无作者返回，不让单行损坏阻断整个查询。
			// ponytail: 升级路径——迁移时清洗 authors 列后再移除容错。
			if err := json.Unmarshal(authorsJSON, &p.Authors); err != nil {
				p.Authors = nil
			}
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// GetByCiteKey 按 3 字母引用标识取论文；未命中返回 (nil, nil)。
// 先精确匹配，再大小写不敏感回退（cite_key 由大小写混合字母生成）。
func (s *Store) GetByCiteKey(citeKey string) (*model.Paper, error) {
	p, err := s.getByCiteKey(citeKey, false)
	if err != nil || p != nil {
		return p, err
	}
	return s.getByCiteKey(citeKey, true)
}

func (s *Store) getByCiteKey(citeKey string, nocase bool) (*model.Paper, error) {
	q := "SELECT " + paperCols + " FROM papers WHERE cite_key = ? LIMIT 1"
	if nocase {
		q = "SELECT " + paperCols + " FROM papers WHERE cite_key = ? COLLATE NOCASE LIMIT 1"
	}
	rows, err := s.db.Query(q, citeKey)
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

// GetByCiteKeys 按一批 3 字母引文标识取论文（FR-LIB：lib get）。
// 结果按 keys 输入顺序返回；miss 为未命中原键列表；空输入或全未命中返回空切片。
func (s *Store) GetByCiteKeys(keys []string) (found []model.Paper, miss []string, err error) {
	unique := make([]string, 0, len(keys)) // 去重并保留顺序
	seen := make(map[string]bool, len(keys))
	for _, k := range keys {
		if !seen[k] {
			seen[k] = true
			unique = append(unique, k)
		}
	}
	if len(unique) == 0 {
		return nil, nil, nil
	}
	placeholders := strings.Repeat("?,", len(unique))
	placeholders = placeholders[:len(placeholders)-1]
	args := make([]any, len(unique))
	for i, k := range unique {
		args[i] = k
	}
	rows, err := s.db.Query("SELECT "+paperCols+" FROM papers WHERE cite_key IN ("+placeholders+")", args...)
	if err != nil {
		return nil, nil, fmt.Errorf("storage get by cite keys: %w", err)
	}
	defer func() { _ = rows.Close() }()
	papers, err := scanPapers(rows)
	if err != nil {
		return nil, nil, err
	}
	byKey := make(map[string]model.Paper, len(papers))
	for _, p := range papers {
		byKey[p.CiteKey] = p
	}
	found = make([]model.Paper, 0, len(unique))
	for _, k := range unique {
		if p, ok := byKey[k]; ok {
			found = append(found, p)
			continue
		}
		// 大小写不敏感回退（与 GetByCiteKey 一致）
		p, perr := s.GetByCiteKey(k)
		if perr != nil {
			return nil, nil, perr
		}
		if p != nil {
			found = append(found, *p)
			continue
		}
		miss = append(miss, k)
	}
	return found, miss, nil
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

// SetFulltext 按 cite_key 写入全文缓存（覆盖旧值）。
// 论文不存在（如并发删除）时返回错误，避免调用方误以为已缓存。
func (s *Store) SetFulltext(citeKey, text string) error {
	res, err := s.db.Exec("UPDATE papers SET fulltext = ? WHERE cite_key = ?", text, citeKey)
	if err != nil {
		return fmt.Errorf("storage set fulltext: %w", err)
	}
	return requireAffected(res, "set fulltext", citeKey)
}

// SetPDFURL 按 cite_key 写入可用的 PDF 直链；论文不存在时返回错误。
func (s *Store) SetPDFURL(citeKey, pdfURL string) error {
	res, err := s.db.Exec("UPDATE papers SET pdf_url = ? WHERE cite_key = ?", pdfURL, citeKey)
	if err != nil {
		return fmt.Errorf("storage set pdf url: %w", err)
	}
	return requireAffected(res, "set pdf url", citeKey)
}

// requireAffected 校验 UPDATE 至少命中一行（论文可能已被并发删除）。
func requireAffected(res sql.Result, op, citeKey string) error {
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("storage %s rows: %w", op, err)
	}
	if n == 0 {
		return fmt.Errorf("storage %s: 论文 %q 不存在", op, citeKey)
	}
	return nil
}

// ListPapers 列出库内论文。source 非空时按源过滤；orderBy 支持 "year"（年份倒序，year=0 排末尾）和 "id"（入库倒序，默认）。
func (s *Store) ListPapers(source, orderBy string, limit, offset int) ([]model.Paper, error) {
	if limit <= 0 {
		limit = defaultListLimit
	}
	if offset < 0 {
		return nil, errors.New("storage list: offset 不能为负数")
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

// SearchLocal 本地 keyword 检索（FR-LIB-04）：标题/摘要/作者姓名 LIKE 匹配。
// LIKE 通配符会被转义（%/_ 按字面量），作者按 JSON 字段值匹配而非原始 JSON 文本。
func (s *Store) SearchLocal(keyword string, limit, offset int) ([]model.Paper, error) {
	if limit <= 0 {
		limit = defaultSearchLimit
	}
	if offset < 0 {
		return nil, errors.New("storage search local: offset 不能为负数")
	}
	pattern := "%" + escapeLike(keyword) + "%"
	// json_valid 保护：历史脏数据（空串/非 JSON）视为无作者，不阻断标题/摘要命中。
	rows, err := s.db.Query(`SELECT `+paperCols+` FROM papers
		WHERE title LIKE ? ESCAPE '\' OR abstract LIKE ? ESCAPE '\'
		   OR EXISTS (SELECT 1 FROM json_each(CASE WHEN json_valid(papers.authors) THEN papers.authors ELSE '[]' END)
		              WHERE json_extract(value, '$.family') LIKE ? ESCAPE '\'
		                 OR json_extract(value, '$.given') LIKE ? ESCAPE '\')
		ORDER BY year DESC, id DESC LIMIT ? OFFSET ?`,
		pattern, pattern, pattern, pattern, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("storage search local: %w", err)
	}
	defer func() { _ = rows.Close() }()
	return scanPapers(rows)
}

// escapeLike 转义 SQL LIKE 通配符，使关键词按字面量匹配。
func escapeLike(s string) string {
	r := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)
	return r.Replace(s)
}

// Forget 删除一篇论文；返回是否实际删除（FR-LIB-02 删）。
// 先精确匹配，未命中再大小写不敏感定位，但只删除定位到的那一行（避免误删大小写不同的另一条）。
func (s *Store) Forget(citeKey string) (bool, error) {
	p, err := s.GetByCiteKey(citeKey)
	if err != nil {
		return false, err
	}
	if p == nil {
		return false, nil
	}
	res, err := s.db.Exec("DELETE FROM papers WHERE cite_key = ?", p.CiteKey)
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
// 计数在只读事务内完成，避免并发写入时各字段互相矛盾。
func (s *Store) Stats() (Stats, error) {
	st := Stats{BySource: map[string]int{}, DBPath: s.path}
	tx, err := s.db.Begin()
	if err != nil {
		return st, fmt.Errorf("storage stats begin: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if err := tx.QueryRow(`SELECT COUNT(*),
		COALESCE(SUM(CASE WHEN TRIM(abstract) <> '' THEN 1 ELSE 0 END), 0),
		COALESCE(SUM(CASE WHEN TRIM(doi) <> '' THEN 1 ELSE 0 END), 0)
		FROM papers`).Scan(&st.Total, &st.WithAbstract, &st.WithDOI); err != nil {
		return st, fmt.Errorf("storage stats: %w", err)
	}
	rows, err := tx.Query("SELECT source, COUNT(*) FROM papers GROUP BY source ORDER BY COUNT(*) DESC")
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
	if err := rows.Err(); err != nil {
		return st, err
	}
	return st, tx.Commit()
}

// AddRef 记录一条引用标记（FR-LIB-07）。
// (cite_key, sentence_hash, manuscript) 唯一约束确保同句幂等。
func (s *Store) AddRef(ref model.PaperRef) error {
	return s.AddRefs([]model.PaperRef{ref})
}

// AddRefs 批量写入引用标记（同句幂等）。
func (s *Store) AddRefs(refs []model.PaperRef) error {
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("storage add refs begin: %w", err)
	}
	defer func() { _ = tx.Rollback() }() // 已提交时为 ErrTxDone。
	if err := writeRefs(tx, refs); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("storage add refs commit: %w", err)
	}
	return nil
}

// ReplaceRefsByManuscript 原子替换手稿引用；空集合清空，写入失败保留旧关系。
func (s *Store) ReplaceRefsByManuscript(manuscript string, refs []model.PaperRef) error {
	for _, ref := range refs {
		if ref.Manuscript != manuscript {
			return errors.New("storage replace refs: manuscript 不一致")
		}
	}
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("storage replace refs begin: %w", err)
	}
	defer func() { _ = tx.Rollback() }() // 已提交时为 ErrTxDone。
	if _, err := tx.Exec("DELETE FROM paper_refs WHERE manuscript = ?", manuscript); err != nil {
		return fmt.Errorf("storage replace refs delete: %w", err)
	}
	if err := writeRefs(tx, refs); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("storage replace refs commit: %w", err)
	}
	return nil
}

func writeRefs(tx *sql.Tx, refs []model.PaperRef) error {
	now := time.Now().Format(time.RFC3339)
	for _, ref := range refs {
		if _, err := tx.Exec(`INSERT OR IGNORE INTO paper_refs
			(cite_key, sentence_hash, manuscript, sentence, created_at)
			VALUES (?,?,?,?,?)`, ref.CiteKey, ref.SentenceHash, ref.Manuscript, ref.Sentence, now); err != nil {
			return fmt.Errorf("storage write refs %s: %w", ref.CiteKey, err)
		}
	}
	return nil
}

// RemoveRefsByManuscript 删除某手稿的全部引用标记（rebuild 安全）。
func (s *Store) RemoveRefsByManuscript(manuscript string) error {
	if _, err := s.db.Exec("DELETE FROM paper_refs WHERE manuscript = ?", manuscript); err != nil {
		return fmt.Errorf("storage remove refs by manuscript: %w", err)
	}
	return nil
}

// queryRefs 按指定 WHERE 条件查询 paper_refs 并扫描为 []PaperRef。
// where/op 为包内固定字面量（非用户输入），拼接安全。
func (s *Store) queryRefs(where, arg, op string) ([]model.PaperRef, error) {
	rows, err := s.db.Query(`SELECT cite_key, sentence_hash, manuscript, sentence
		FROM paper_refs WHERE `+where+` ORDER BY id`, arg)
	if err != nil {
		return nil, fmt.Errorf("storage %s: %w", op, err)
	}
	defer func() { _ = rows.Close() }()
	return scanRefs(rows)
}

// GetRefsByManuscript 取某手稿的全部引用标记。
func (s *Store) GetRefsByManuscript(manuscript string) ([]model.PaperRef, error) {
	return s.queryRefs("manuscript = ?", manuscript, "get refs by manuscript")
}

// GetRefsByCiteKey 取某篇论文的全部引用标记。
func (s *Store) GetRefsByCiteKey(citeKey string) ([]model.PaperRef, error) {
	return s.queryRefs("cite_key = ?", citeKey, "get refs by cite key")
}

// scanRefs 扫描 paper_refs 查询行为 []model.PaperRef。
func scanRefs(rows *sql.Rows) ([]model.PaperRef, error) {
	var out []model.PaperRef
	for rows.Next() {
		var r model.PaperRef
		if err := rows.Scan(&r.CiteKey, &r.SentenceHash, &r.Manuscript, &r.Sentence); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// GetCitationScore 查询评分缓存；未命中返回 (nil, nil)。
func (s *Store) GetCitationScore(citeKey, sentenceHash, modelID, promptVersion string) (*model.CitationScore, error) {
	var cs model.CitationScore
	err := s.db.QueryRow(
		`SELECT cite_key, sentence_hash, model_id, prompt_version, score, rationale
		 FROM citation_scores
		 WHERE cite_key = ? AND sentence_hash = ? AND model_id = ? AND prompt_version = ?`,
		citeKey, sentenceHash, modelID, promptVersion,
	).Scan(&cs.CiteKey, &cs.SentenceHash, &cs.ModelID, &cs.PromptVersion, &cs.Score, &cs.Rationale)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("storage get citation score: %w", err)
	}
	return &cs, nil
}

// SaveCitationScore 保存一条评分结果（INSERT OR REPLACE 幂等）。
func (s *Store) SaveCitationScore(cs model.CitationScore) error {
	now := time.Now().Format(time.RFC3339)
	_, err := s.db.Exec(`INSERT OR REPLACE INTO citation_scores
		(cite_key, sentence_hash, model_id, prompt_version, score, rationale, scored_at)
		VALUES (?,?,?,?,?,?,?)`,
		cs.CiteKey, cs.SentenceHash, cs.ModelID, cs.PromptVersion, cs.Score, cs.Rationale, now)
	if err != nil {
		return fmt.Errorf("storage save citation score: %w", err)
	}
	return nil
}
