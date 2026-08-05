package core

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"litkit/internal/model"
	"litkit/internal/storage"
	"litkit/internal/util/httpclient"
)

// buildMinimalPDF 生成单页、含指定文本的最小 PDF（text extraction 测试 fixture）。
// 动态构建以保证 xref 偏移精确（rsc/pdf 系解析器依赖 xref 表）。
func buildMinimalPDF(text string) []byte {
	content := "BT /F1 24 Tf 100 700 Td (" + text + ") Tj ET"
	stream := "stream\n" + content + "\nendstream"
	objs := []string{
		"<< /Type /Catalog /Pages 2 0 R >>",
		"<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
		"<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Contents 4 0 R /Resources << /Font << /F1 5 0 R >> >> >>",
		"<< /Length " + strconv.Itoa(len(content)) + " >>\n" + stream,
		"<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>",
	}
	var b bytes.Buffer
	fmt.Fprintf(&b, "%%PDF-1.4\n")
	offsets := make([]int, len(objs))
	for i, o := range objs {
		offsets[i] = b.Len()
		fmt.Fprintf(&b, "%d 0 obj\n%s\nendobj\n", i+1, o)
	}
	xrefPos := b.Len()
	fmt.Fprintf(&b, "xref\n0 %d\n", len(objs)+1)
	fmt.Fprintf(&b, "0000000000 65535 f \n")
	for _, off := range offsets {
		fmt.Fprintf(&b, "%010d 00000 n \n", off)
	}
	fmt.Fprintf(&b, "trailer\n<< /Size %d /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF\n", len(objs)+1, xrefPos)
	return b.Bytes()
}

// fetchTestEnv 组合 httptest server + Fetcher + 库。
type fetchTestEnv struct {
	srv        *httptest.Server
	fetcher    *FulltextFetcher
	store      *storage.Store
	upCount    int // Unpaywall 请求计数
	sciCount   int // Sci-Hub 请求计数
	pdfCount   int // PDF 下载请求计数
	paperBytes []byte
}

func newFetchTestEnv(t *testing.T, store *storage.Store) *fetchTestEnv {
	t.Helper()
	env := &fetchTestEnv{store: store}
	mux := http.NewServeMux()
	mux.HandleFunc("/unpaywall/", func(w http.ResponseWriter, r *http.Request) {
		env.upCount++
		doi := strings.TrimPrefix(r.URL.Path, "/unpaywall/")
		if strings.HasPrefix(doi, "10.404") {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		switch doi {
		case "10.1/oa":
			writeJSON(w, map[string]any{
				"is_oa": true,
				"best_oa_location": map[string]any{
					"url":         env.srv.URL + "/landing",
					"url_for_pdf": env.srv.URL + "/pdf/paper.pdf",
				},
			})
		default: // 无 OA
			writeJSON(w, map[string]any{"is_oa": false, "best_oa_location": nil, "oa_locations": []any{}})
		}
	})
	mux.HandleFunc("/scihub/", func(w http.ResponseWriter, r *http.Request) {
		env.sciCount++
		doi := strings.TrimPrefix(r.URL.Path, "/scihub/")
		if strings.HasPrefix(doi, "10.404") {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprintf(w, `<html><embed type="application/pdf" src="%s/pdf/paper.pdf"></html>`, env.srv.URL)
	})
	mux.HandleFunc("/pdf/paper.pdf", func(w http.ResponseWriter, r *http.Request) {
		env.pdfCount++
		w.Header().Set("Content-Type", "application/pdf")
		_, _ = w.Write(env.paperBytes)
	})
	env.srv = httptest.NewServer(mux)
	t.Cleanup(env.srv.Close)
	env.paperBytes = buildMinimalPDF("Hello PDF")
	env.fetcher = &FulltextFetcher{
		store:          store,
		client:         httpclient.New(httpclient.Options{TimeoutMS: 5000}),
		unpaywallBase:  env.srv.URL + "/unpaywall",
		unpaywallEmail: "test@example.com",
		sciHubBase:     env.srv.URL + "/scihub",
		downloadDir:    t.TempDir(),
	}
	return env
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

// newFetchTestStore 打开临时库（core 包测试用），测试结束自动关闭。
func newFetchTestStore(t *testing.T) *storage.Store {
	t.Helper()
	s, err := storage.Open(filepath.Join(t.TempDir(), "litkit.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

// insertPaper 入库一篇带 DOI 的论文，返回 citeKey。
func insertPaper(t *testing.T, s *storage.Store, doi string) string {
	t.Helper()
	citeKey, _, err := s.UpsertPaper(model.Paper{
		Title: "Test Paper", DOI: doi, Abstract: "abstract", Source: "fake",
	})
	if err != nil {
		t.Fatalf("UpsertPaper: %v", err)
	}
	return citeKey
}

// ---- 抽取（FR-FETCH-05）----

func TestExtractTextFromMinimalPDF(t *testing.T) {
	f := &FulltextFetcher{downloadDir: t.TempDir()}
	path := filepath.Join(t.TempDir(), "fixture.pdf")
	if err := os.WriteFile(path, buildMinimalPDF("Hello PDF"), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	text, err := f.extractText(path)
	if err != nil {
		t.Fatalf("extractText: %v", err)
	}
	if !strings.Contains(text, "Hello PDF") {
		t.Fatalf("应抽取到 Hello PDF，got %q", text)
	}
}

// ---- 缓存命中（FR-FETCH-04）----

func TestFetch_CacheHitNoNetwork(t *testing.T) {
	s := newFetchTestStore(t)
	env := newFetchTestEnv(t, s)
	citeKey := insertPaper(t, s, "10.1/oa")
	if err := s.SetFulltext(citeKey, "cached full text"); err != nil {
		t.Fatalf("SetFulltext: %v", err)
	}
	res, err := env.fetcher.Fetch(t.Context(), citeKey)
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if res.Via != "cache" || res.Fulltext != "cached full text" {
		t.Fatalf("缓存命中应直接返回，got %+v", res)
	}
	if env.upCount != 0 || env.sciCount != 0 || env.pdfCount != 0 {
		t.Fatalf("缓存命中不应发起任何网络请求，up=%d sci=%d pdf=%d", env.upCount, env.sciCount, env.pdfCount)
	}
}

// ---- Unpaywall OA（FR-FETCH-02）----

func TestFetch_UnpaywallDownloadsAndCaches(t *testing.T) {
	s := newFetchTestStore(t)
	env := newFetchTestEnv(t, s)
	citeKey := insertPaper(t, s, "10.1/oa")

	res, err := env.fetcher.Fetch(t.Context(), citeKey)
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if res.Via != "unpaywall" {
		t.Fatalf("应走 unpaywall，got %q", res.Via)
	}
	if res.PdfPath == "" {
		t.Fatal("应返回 PDF 落盘路径")
	}
	if !strings.Contains(res.Fulltext, "Hello PDF") {
		t.Fatalf("应抽取到全文，got %q", res.Fulltext)
	}
	if env.pdfCount != 1 || env.sciCount != 0 {
		t.Fatalf("OA 命中不应触达 Sci-Hub，pdf=%d sci=%d", env.pdfCount, env.sciCount)
	}
	// 全文已缓存，二次 Fetch 直接命中
	cached, err := s.GetFulltext(citeKey)
	if err != nil || cached == "" {
		t.Fatalf("全文应已入库，got %q err=%v", cached, err)
	}
	if _, err := os.Stat(res.PdfPath); err != nil {
		t.Fatalf("PDF 应已落盘：%v", err)
	}
}

// ---- Sci-Hub 兜底（FR-FETCH-03）----

func TestFetch_UnpaywallMissFallsBackToSciHub(t *testing.T) {
	s := newFetchTestStore(t)
	env := newFetchTestEnv(t, s)
	citeKey := insertPaper(t, s, "10.1/no-oa")

	res, err := env.fetcher.Fetch(t.Context(), citeKey)
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if res.Via != "scihub" {
		t.Fatalf("Unpaywall 无 OA 应走 Sci-Hub 兜底，got %q", res.Via)
	}
	if !strings.Contains(res.Fulltext, "Hello PDF") {
		t.Fatalf("应抽取到全文，got %q", res.Fulltext)
	}
	if env.upCount != 1 || env.sciCount != 1 || env.pdfCount != 1 {
		t.Fatalf("应各触发一次，up=%d sci=%d pdf=%d", env.upCount, env.sciCount, env.pdfCount)
	}
}

// ---- 无 DOI / 全部失败 ----

func TestFetch_NoDOI(t *testing.T) {
	s := newFetchTestStore(t)
	env := newFetchTestEnv(t, s)
	citeKey := insertPaper(t, s, "") // 无 DOI

	if _, err := env.fetcher.Fetch(t.Context(), citeKey); err == nil {
		t.Fatal("无 DOI 应报错（Unpaywall/Sci-Hub 均需 DOI）")
	}
}

func TestFetch_NotFoundInLibrary(t *testing.T) {
	s := newFetchTestStore(t)
	env := newFetchTestEnv(t, s)
	if _, err := env.fetcher.Fetch(t.Context(), "10.1/not-in-library"); err == nil {
		t.Fatal("库中不存在的论文应报错")
	}
}

func TestFetch_AllSourcesFail(t *testing.T) {
	s := newFetchTestStore(t)
	env := newFetchTestEnv(t, s)
	citeKey := insertPaper(t, s, "10.404/absent")

	_, err := env.fetcher.Fetch(t.Context(), citeKey)
	if err == nil {
		t.Fatal("Unpaywall 与 Sci-Hub 均未命中应报错")
	}
	if !strings.Contains(err.Error(), "PDF") {
		t.Fatalf("错误信息应提示未找到 PDF，got %v", err)
	}
}

// ---- 解析单元 ----

func TestResolveUnpaywallPrefersURLForPDF(t *testing.T) {
	s := newFetchTestStore(t)
	env := newFetchTestEnv(t, s)
	doi := "10.1/oa"
	got := env.fetcher.resolveUnpaywall(t.Context(), doi)
	if !strings.Contains(got, "/pdf/paper.pdf") {
		t.Fatalf("应优先取 url_for_pdf，got %q", got)
	}
}

func TestResolveUnpaywallNoEmail(t *testing.T) {
	s := newFetchTestStore(t)
	env := newFetchTestEnv(t, s)
	env.fetcher.unpaywallEmail = ""
	if got := env.fetcher.resolveUnpaywall(t.Context(), "10.1/oa"); got != "" {
		t.Fatalf("无 email 应跳过 Unpaywall，got %q", got)
	}
}

func TestSciHubHTMLParseModes(t *testing.T) {
	cases := []struct {
		name string
		html string
		want string
	}{
		{"embed", `<embed type="application/pdf" src="/pdf/a.pdf">`, "https://sci-hub.se/pdf/a.pdf"},
		{"embed-scheme-relative", `<embed type="application/pdf" src="//host/b.pdf">`, "https://host/b.pdf"},
		{"iframe", `<iframe src="https://cdn.example.com/c.pdf"></iframe>`, "https://cdn.example.com/c.pdf"},
		{"button", `<button onclick="location.href='https://d.example/d.pdf'">download</button>`, "https://d.example/d.pdf"},
		{"link", `<a href="https://e.example/e.pdf">pdf</a>`, "https://e.example/e.pdf"},
		{"none", `<html><body>article not found</body></html>`, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := extractPDFURLFromHTML(c.html, "https://sci-hub.se")
			if got != c.want {
				t.Fatalf("want %q, got %q", c.want, got)
			}
		})
	}
}
