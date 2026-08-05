// fetch.go 全文获取：Unpaywall OA 优先 → Sci-Hub 兜底 → 本地抽取缓存（FR-FETCH）。
//
// 流程（FR-FETCH-01/04）：
//  1. 按 cite_key / DOI 定位库内论文；库中已有全文缓存直接返回（Via=cache，零网络）。
//  2. Unpaywall 按 DOI 解析最佳 OA PDF URL（需 LITKIT_UNPAYWALL_EMAIL，FR-FETCH-02）。
//  3. Unpaywall 未命中则 Sci-Hub 兜底（默认开启，URL 可配；失败静默降级，FR-FETCH-03）。
//  4. 下载 PDF → 落盘 <downloadDir>/<citeKey>.pdf。
//  5. 纯 Go 抽取全文（ledongthuc/pdf，FR-FETCH-05）；扫描版 PDF 返回空文本并提示。
//  6. 全文写入库（papers.fulltext，FR-FETCH-04），后续 Fetch 命中缓存。
//
// Sci-Hub 失败静默：任何一步失败仅跳过该源，不中断整体流程；仅当两个源都未命中时返回错误。

package core

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/ledongthuc/pdf"

	"litkit/internal/model"
	"litkit/internal/storage"
	"litkit/internal/util/httpclient"
)

// FetchResult 全文获取结果（FR-FETCH-01）。
type FetchResult struct {
	CiteKey  string `json:"citeKey"`
	PdfPath  string `json:"pdfPath,omitempty"` // PDF 落盘路径（缓存命中且无本地 PDF 时为空）
	Fulltext string `json:"fulltext"`
	Via      string `json:"via"` // cache | unpaywall | scihub
}

// FulltextFetcher 全文获取器（FR-FETCH）。依赖注入 httpclient.Client 与存储，便于测试。
type FulltextFetcher struct {
	store          *storage.Store
	client         *httpclient.Client
	unpaywallBase  string // 测试可覆盖，默认 https://api.unpaywall.org/v2
	unpaywallEmail string // 无 email 时跳过 Unpaywall（FR-FETCH-02）
	sciHubBase     string // 兜底镜像，默认来自 config（https://sci-hub.se）
	downloadDir    string // PDF 落盘目录，默认 <WorkDir>/downloads
}

const (
	defaultUnpaywallBase = "https://api.unpaywall.org/v2"
	defaultSciHubBase    = "https://sci-hub.se"

	// 目录与文件权限（仅所有者读写/执行）。
	dirPerm  = 0o700
	filePerm = 0o600
)

// fetchUserAgent 全文获取 User-Agent（Unpaywall 要求 UA 含联系邮箱）。
const fetchUserAgent = "litkit/0.1 (https://github.com/litkit/litkit)"

// browserUA Sci-Hub 等对默认 UA 有反爬，用浏览器 UA 降低失败率（旧实现同策略）。
const browserUA = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/124.0 Safari/537.36"

// NewFulltextFetcher 创建全文获取器。
func NewFulltextFetcher(store *storage.Store, client *httpclient.Client, unpaywallEmail, sciHubURL, downloadDir string) *FulltextFetcher {
	if sciHubURL == "" {
		sciHubURL = defaultSciHubBase
	}
	return &FulltextFetcher{
		store:          store,
		client:         client,
		unpaywallBase:  defaultUnpaywallBase,
		unpaywallEmail: strings.TrimSpace(unpaywallEmail),
		sciHubBase:     strings.TrimRight(sciHubURL, "/"),
		downloadDir:    downloadDir,
	}
}

// Fetch 取回论文全文：ref 为 cite_key（3 字母）或 DOI。
//
// 返回 (nil, error) 表示库中无此论文 / 无 DOI / 两个源均未命中 / 下载或抽取失败。
// 抽取失败不删除已落盘 PDF（FR-FETCH-05）。
func (f *FulltextFetcher) Fetch(ctx context.Context, ref string) (*FetchResult, error) {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return nil, errors.New("fetch: ref 为空")
	}
	p, err := f.lookup(ref)
	if err != nil {
		return nil, err
	}
	if p == nil {
		return nil, fmt.Errorf("fetch: 库中未找到论文 %q（请先 search/metadata 入库）", ref)
	}

	// 全文缓存命中：直接返回，零网络（FR-FETCH-04）
	if ft, err := f.store.GetFulltext(p.CiteKey); err == nil && strings.TrimSpace(ft) != "" {
		return &FetchResult{CiteKey: p.CiteKey, Fulltext: ft, Via: "cache"}, nil
	}

	if p.DOI == "" {
		return nil, fmt.Errorf("fetch: 论文 %s 无 DOI，Unpaywall/Sci-Hub 均需 DOI", p.CiteKey)
	}

	// 1) Unpaywall OA 优先（FR-FETCH-02）
	pdfURL := f.resolveUnpaywall(ctx, p.DOI)
	via := "unpaywall"
	if pdfURL == "" {
		// 2) Sci-Hub 兜底（FR-FETCH-03）：失败静默，不单独报错
		pdfURL = f.resolveSciHub(ctx, p.DOI)
		via = "scihub"
	}
	if pdfURL == "" {
		return nil, fmt.Errorf("fetch: 未找到可用 PDF（Unpaywall 无 OA 且 Sci-Hub 未命中）")
	}

	pdfPath, err := f.downloadPDF(ctx, pdfURL, p.CiteKey)
	if err != nil {
		return nil, fmt.Errorf("fetch: 下载 PDF 失败: %w", err)
	}

	text, err := f.extractText(pdfPath)
	if err != nil {
		// 抽取失败不删除已落盘 PDF（FR-FETCH-05）
		return nil, fmt.Errorf("fetch: 抽取全文失败（PDF 已保存至 %s）: %w", pdfPath, err)
	}
	// 抽取成功（可能为空文本——扫描版 PDF 无文本层），全文缓存入库
	_ = f.store.SetFulltext(p.CiteKey, text)
	_ = f.store.SetPDFURL(p.CiteKey, pdfURL)

	return &FetchResult{CiteKey: p.CiteKey, PdfPath: pdfPath, Fulltext: text, Via: via}, nil
}

// lookup 定位论文：3 字母纯字母视为 cite_key，否则按 DOI 查询。
func (f *FulltextFetcher) lookup(ref string) (*model.Paper, error) {
	if len(ref) == 3 && isAlphaOnly(ref) {
		if p, err := f.store.GetByCiteKey(ref); err != nil {
			return nil, fmt.Errorf("fetch: 按 cite_key 查询: %w", err)
		} else if p != nil {
			return p, nil
		}
	}
	p, err := f.store.GetByDOI(ref)
	if err != nil {
		return nil, fmt.Errorf("fetch: 按 DOI 查询: %w", err)
	}
	return p, nil
}

func isAlphaOnly(s string) bool {
	for _, r := range s {
		if (r < 'a' || r > 'z') && (r < 'A' || r > 'Z') {
			return false
		}
	}
	return true
}

// ---- Unpaywall（FR-FETCH-02）----

// unpaywallResponse Unpaywall API v2 响应（仅解析所需字段）。
type unpaywallResponse struct {
	IsOA           bool                `json:"is_oa"`
	BestOALocation *unpaywallLocation  `json:"best_oa_location"`
	OALocations    []unpaywallLocation `json:"oa_locations"`
}

type unpaywallLocation struct {
	URL       string `json:"url"`
	URLForPDF string `json:"url_for_pdf"`
}

// resolveUnpaywall 按 DOI 解析最佳 OA PDF URL；无 email / 非 OA / 请求失败返回空串。
func (f *FulltextFetcher) resolveUnpaywall(ctx context.Context, doi string) string {
	if f.unpaywallEmail == "" || f.unpaywallBase == "" {
		return ""
	}
	u := f.unpaywallBase + "/" + url.PathEscape(doi) + "?email=" + url.QueryEscape(f.unpaywallEmail)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return ""
	}
	req.Header.Set("User-Agent", fetchUserAgent)
	resp, err := f.client.Do(ctx, req)
	if err != nil {
		return ""
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return ""
	}
	data, err := httpclient.ReadAll(resp)
	if err != nil {
		return ""
	}
	var r unpaywallResponse
	if err := json.Unmarshal(data, &r); err != nil {
		return ""
	}
	if loc := r.BestOALocation; loc != nil {
		if u := pickPDFURL(loc.URLForPDF, loc.URL); u != "" {
			return u
		}
	}
	for _, loc := range r.OALocations {
		if u := pickPDFURL(loc.URLForPDF, loc.URL); u != "" {
			return u
		}
	}
	return ""
}

// pickPDFURL 优先取 url_for_pdf，回退 url。
func pickPDFURL(pdfURL, pageURL string) string {
	if u := strings.TrimSpace(pdfURL); u != "" {
		return u
	}
	return strings.TrimSpace(pageURL)
}

// ---- Sci-Hub 兜底（FR-FETCH-03）----

// resolveSciHub 按 DOI 请求 Sci-Hub 页面并解析出 PDF 直链；失败返回空串（静默）。
func (f *FulltextFetcher) resolveSciHub(ctx context.Context, doi string) string {
	if f.sciHubBase == "" {
		return ""
	}
	u := f.sciHubBase + "/" + url.PathEscape(doi)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return ""
	}
	req.Header.Set("User-Agent", browserUA)
	req.Header.Set("Accept", "text/html,application/xhtml+xml")
	req.Header.Set("Referer", f.sciHubBase)
	resp, err := f.client.Do(ctx, req)
	if err != nil {
		return ""
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return ""
	}
	body, err := httpclient.ReadAll(resp)
	if err != nil {
		return ""
	}
	if strings.Contains(strings.ToLower(string(body)), "article not found") {
		return ""
	}
	return extractPDFURLFromHTML(string(body), f.sciHubBase)
}

// pdfURLEmbedRe 匹配 <embed type="application/pdf" src="...">。
var pdfURLEmbedRe = regexp.MustCompile(`(?is)<embed[^>]+src=["']([^"']+)["']`)

// pdfURLIframeRe 匹配 <iframe src="...">。
var pdfURLIframeRe = regexp.MustCompile(`(?is)<iframe[^>]+src=["']([^"']+)["']`)

// pdfURLButtonRe 匹配按钮 onclick 中的 location.href='...'。
var pdfURLButtonRe = regexp.MustCompile(`(?i)location\.href=['"]([^'"]+)['"]`)

// pdfURLLinkRe 匹配含 pdf 的 <a href="...">。
var pdfURLLinkRe = regexp.MustCompile(`(?is)<a[^>]+href=["']([^"']*pdf[^"']*)["']`)

// extractPDFURLFromHTML 从 Sci-Hub 页面提取 PDF 直链（embed → iframe → button → a）。
// base 用于补全协议相对（//）与站内相对（/）URL；未找到返回空串。
func extractPDFURLFromHTML(html, base string) string {
	base = strings.TrimRight(base, "/")
	rems := []*regexp.Regexp{pdfURLEmbedRe, pdfURLIframeRe, pdfURLButtonRe, pdfURLLinkRe}
	for _, re := range rems {
		m := re.FindStringSubmatch(html)
		if len(m) < 2 {
			continue
		}
		if u := normalizeSciHubURL(strings.TrimSpace(m[1]), base); u != "" {
			return u
		}
	}
	return ""
}

// normalizeSciHubURL 补全 //、/、相对路径为绝对 URL；非 http(s) 协议丢弃。
func normalizeSciHubURL(u, base string) string {
	u = strings.TrimSpace(u)
	switch {
	case strings.HasPrefix(u, "//"):
		return "https:" + u
	case strings.HasPrefix(u, "/"):
		return base + u
	case strings.HasPrefix(u, "http://"), strings.HasPrefix(u, "https://"):
		return u
	}
	return ""
}

// ---- 下载与抽取 ----

// downloadPDF 下载 PDF 并落盘为 <downloadDir>/<citeKey>.pdf；返回落盘路径。
func (f *FulltextFetcher) downloadPDF(ctx context.Context, pdfURL, citeKey string) (string, error) {
	if err := os.MkdirAll(f.downloadDir, dirPerm); err != nil {
		return "", fmt.Errorf("创建下载目录: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, pdfURL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", browserUA)
	req.Header.Set("Accept", "application/pdf,*/*")
	resp, err := f.client.Do(ctx, req)
	if err != nil {
		return "", err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	data, err := httpclient.ReadAll(resp)
	if err != nil {
		return "", err
	}
	if !looksLikePDF(data) {
		return "", errors.New("响应内容不是 PDF")
	}
	path := filepath.Join(f.downloadDir, citeKey+".pdf")
	if err := os.WriteFile(path, data, filePerm); err != nil {
		return "", err
	}
	return path, nil
}

// looksLikePDFCheckSize 检查 PDF 魔数时读取的头部最大字节数。
const looksLikePDFCheckSize = 1024

// looksLikePDF 检查响应体头部是否含 %PDF 魔数（比 Content-Type 更可靠）。
func looksLikePDF(data []byte) bool {
	head := data
	if len(head) > looksLikePDFCheckSize {
		head = head[:looksLikePDFCheckSize]
	}
	return bytes.Contains(head, []byte("%PDF"))
}

// extractText 用 ledongthuc/pdf 抽取 PDF 全文（FR-FETCH-05）。
// 扫描版 PDF（无文本层）返回空文本，不视为错误。
func (f *FulltextFetcher) extractText(pdfPath string) (string, error) {
	fh, r, err := pdf.Open(pdfPath)
	if err != nil {
		return "", err
	}
	defer func() { _ = fh.Close() }()
	b, err := r.GetPlainText()
	if err != nil {
		return "", err
	}
	data, err := io.ReadAll(b)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(data)), nil
}
