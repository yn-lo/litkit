package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"litkit/internal/config"
	"litkit/internal/core"
	"litkit/internal/lint"
	"litkit/internal/model"
	"litkit/internal/storage"
)

// TestWriteManuscriptArtifacts_PandocMissing 覆盖 FR-REF-11：Pandoc 缺失时
// 仅跳过 docx，md/bib/ris 仍正常生成且不报错。
func TestWriteManuscriptArtifacts_PandocMissing(t *testing.T) {
	old := lookPath
	lookPath = func(string) (string, error) { return "", errors.New("not found") }
	t.Cleanup(func() { lookPath = old })

	res := &core.ManuscriptResult{Text: "body", Papers: []model.Paper{{Title: "T"}}}
	files, err := writeManuscriptArtifacts(t.TempDir(), "draft", "20260101_000000", res, "参考文献", true)
	if err != nil {
		t.Fatalf("Pandoc 缺失不应报错: %v", err)
	}
	if _, ok := files["formatted.docx"]; ok {
		t.Fatal("Pandoc 缺失时不应生成 docx")
	}
	for _, name := range []string{core.ManuscriptFormatted, core.ManuscriptBib, core.ManuscriptRIS} {
		if _, ok := files[name]; !ok {
			t.Errorf("Pandoc 缺失时仍应生成 %s", name)
		}
	}
}

func round2ScoringFixture(t *testing.T) (*config.Config, *storage.Store, string, string) {
	t.Helper()
	dir := t.TempDir()
	st, err := storage.Open(storage.DBPath(dir))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	key, _, err := st.UpsertPaper(model.Paper{Title: "Study", DOI: "10.1/probe", Abstract: "Evidence"})
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"message": map[string]any{"content": `{"score":0.6,"rationale":"fixture"}`}}}})
	}))
	t.Cleanup(srv.Close)
	vm := core.VerifierModels{PromptVersion: "v1", ConsensusThreshold: 0.9, Models: []core.ModelConfig{{ID: "round2", Enabled: true, APIKey: "test-placeholder", BaseURL: srv.URL}}, Scoring: core.ScoringConfig{MinModels: 1, AgreementRatio: 0.67, LowScoreThreshold: 0.8, MediumScoreThreshold: 0.9}}
	b, err := json.Marshal(vm)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".litkit", "verifier_models.json"), b, 0o600); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "doc.md")
	return &config.Config{WorkDir: dir, LLMTimeoutMS: 1000}, st, path, key
}

// TestUnverifiedNumbers Layer 0 数字集合规则：只列出摘要中找不到的数字。
func TestUnverifiedNumbers(t *testing.T) {
	got := unverifiedNumbers(
		"The trial enrolled 500 patients in 2020.",
		"We enrolled 500 patients with hypertension.")
	if len(got) != 1 || got[0] != "2020" {
		t.Fatalf("应仅剩摘要中不存在的 2020，got %v", got)
	}
	if extra := unverifiedNumbers("We enrolled 500 patients.", "Enrolled 500 patients."); len(extra) != 0 {
		t.Fatalf("数字一致时不应报告，got %v", extra)
	}
}

func TestRelevanceConfiguredConsensusThreshold(t *testing.T) {
	vm := &core.VerifierModels{Scoring: core.DefaultScoringConfig(), ConsensusThreshold: 0.9}
	result := &core.ScoreResult{MeanScore: 0.6, Consensus: 0.8, PerModel: []core.ModelScore{{ModelID: "mock"}}}
	if got := buildRelevanceItem("doc.md", 1, "AAA", "Claim", result, vm); !got.LowConsensus || got.LowScore {
		t.Fatalf("configured consensus threshold ignored: %+v", got)
	}
}

func TestVerifyInvalidSpecDoesNotUseDefaults(t *testing.T) {
	dir := t.TempDir()
	if _, err := lint.InitPaperType(dir, "review", "zh", false); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(lint.SpecPath(dir, "review", "zh"), []byte("paper_type: [invalid"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := loadVerifySpec(dir, "review", "zh"); err == nil {
		t.Fatal("invalid spec was silently replaced by defaults")
	}
}

func TestRound2_DeletedCitationsClearRefs(t *testing.T) {
	cfg, st, path, key := round2ScoringFixture(t)
	if err := os.WriteFile(path, []byte("结论[@"+key+"]。"), 0o600); err != nil {
		t.Fatal(err)
	}
	first := runCitationRelevance(context.Background(), []string{path}, st, cfg, lint.DefaultSpec())
	if len(first.Results) != 1 {
		t.Fatalf("fixture score: %+v", first)
	}
	if err := os.WriteFile(path, []byte("结论。"), 0o600); err != nil {
		t.Fatal(err)
	}
	_ = runCitationRelevance(context.Background(), []string{path}, st, cfg, lint.DefaultSpec())
	refs, err := st.GetRefsByManuscript("doc.md")
	if err != nil {
		t.Fatal(err)
	}
	if len(refs) != 0 {
		t.Fatalf("deleted references still in database: %+v", refs)
	}
}

func TestRound2_UnknownRefDoesNotDropValidLaterRef(t *testing.T) {
	cfg, st, path, key := round2ScoringFixture(t)
	missing := "AAA"
	if missing == key {
		missing = "BBB"
	}
	if err := os.WriteFile(path, []byte("未知[@"+missing+"]。已知[@"+key+"]。"), 0o600); err != nil {
		t.Fatal(err)
	}
	got := runCitationRelevance(context.Background(), []string{path}, st, cfg, lint.DefaultSpec())
	if len(got.Results) != 1 {
		t.Fatalf("fixture score: %+v", got)
	}
	refs, err := st.GetRefsByManuscript("doc.md")
	if err != nil {
		t.Fatal(err)
	}
	if len(refs) != 1 || refs[0].CiteKey != key {
		t.Fatalf("valid reference dropped after missing key: %+v", refs)
	}
}

func TestRound2_CustomScoreThreshold(t *testing.T) {
	cfg, st, path, key := round2ScoringFixture(t)
	if err := os.WriteFile(path, []byte("结论[@"+key+"]。"), 0o600); err != nil {
		t.Fatal(err)
	}
	got := runCitationRelevance(context.Background(), []string{path}, st, cfg, lint.DefaultSpec())
	if len(got.Results) != 1 {
		t.Fatalf("fixture score: %+v", got)
	}
	if !got.Results[0].LowScore {
		t.Fatalf("configured low_score_threshold=0.8 ignored for score 0.6: %+v", got.Results[0])
	}
}

func TestRound2_ExplicitSpecTypeAndLang(t *testing.T) {
	for _, tc := range []struct{ typ, lang string }{{"book", "zh"}, {"empirical", "en"}, {"review", "en"}} {
		spec, err := loadVerifySpec(t.TempDir(), tc.typ, tc.lang)
		if err != nil {
			t.Fatal(err)
		}
		if spec.PaperType != tc.typ || spec.Lang != tc.lang {
			t.Fatalf("explicit %s/%s became %s/%s", tc.typ, tc.lang, spec.PaperType, spec.Lang)
		}
	}
	if _, err := loadVerifySpec(t.TempDir(), "book", "en"); err == nil {
		t.Fatal("missing book/en preset must return an error, not empirical/zh or panic")
	}
}

func TestRound2_SelectedLanguageNotIgnored(t *testing.T) {
	dir := t.TempDir()
	if _, err := lint.InitPaperType(dir, "review", "zh", false); err != nil {
		t.Fatal(err)
	}
	spec, err := loadVerifySpec(dir, "", "en")
	if err != nil {
		t.Fatal(err)
	}
	if spec.Lang != "en" {
		t.Fatalf("requested en but auto-selected %s/%s", spec.PaperType, spec.Lang)
	}
}
