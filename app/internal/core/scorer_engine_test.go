package core

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"litkit/internal/model"
	"litkit/internal/storage"
)

// mockScorer 实现 Scorer 接口，返回预设分数。
type mockScorer struct {
	modelID       string
	promptVersion string
	score         float64
	rationale     string
	fail          bool
}

func (m *mockScorer) Score(_ context.Context, _, _ string) (float64, string, error) {
	if m.fail {
		return 0, "", assertAnError("mock failure")
	}
	return m.score, m.rationale, nil
}
func (m *mockScorer) ModelID() string       { return m.modelID }
func (m *mockScorer) PromptVersion() string { return m.promptVersion }

func assertAnError(s string) error {
	return &mockError{s}
}

type mockError struct{ s string }

func (e *mockError) Error() string { return e.s }

// newTestStore 创建临时存储供测试用。
func newTestStore(t *testing.T) *storage.Store {
	t.Helper()
	s, err := storage.Open(filepath.Join(t.TempDir(), storage.DefaultDBName))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

// insertPaperWithAbstract 插入测试论文（与 fetch_test.go 的 insertPaper 区分）。
func insertPaperWithAbstract(t *testing.T, s *storage.Store, citeKey, title, abstract string) string {
	t.Helper()
	p := model.Paper{
		CiteKey:  citeKey,
		Title:    title,
		Abstract: abstract,
		Source:   "fake",
		Year:     2024,
		DocType:  model.DocTypeArticle,
	}
	p.ID = p.ComputeID()
	k, _, err := s.UpsertPaper(p)
	if err != nil {
		t.Fatalf("UpsertPaper: %v", err)
	}
	return k
}

// ---- 禁用模式 ----

func TestScorerEngine_Disabled_ReturnsNil(t *testing.T) {
	store := newTestStore(t)
	engine := NewScorerEngine(store, nil, "", "", 0, false)
	if !engine.IsDisabled() {
		t.Fatal("禁用模式引擎应返回 IsDisabled=true")
	}
	result, err := engine.Score(context.Background(), "Kxq", "hash", "句子", "摘要")
	if err != nil {
		t.Fatalf("Score: %v", err)
	}
	if result != nil {
		t.Fatal("禁用模式应返回 nil")
	}
}

func TestScorerEngine_NoEnabledModels_ReturnsNil(t *testing.T) {
	store := newTestStore(t)
	cfg := &VerifierModels{
		PromptVersion: "v1",
		Models:        []ModelConfig{{ID: "gpt-4o", Enabled: false}},
		Scoring:       DefaultScoringConfig(),
	}
	engine := NewScorerEngine(store, cfg, "", "", 0, true)
	if !engine.IsDisabled() {
		t.Fatal("无启用模型应返回 IsDisabled=true")
	}
	result, err := engine.Score(context.Background(), "Kxq", "hash", "句子", "摘要")
	if err != nil {
		t.Fatalf("Score: %v", err)
	}
	if result != nil {
		t.Fatal("无启用模型应返回 nil")
	}
}

func TestScorerEngine_EnabledNoKey_ReturnsNil(t *testing.T) {
	store := newTestStore(t)
	cfg := &VerifierModels{
		PromptVersion: "v1",
		Models:        []ModelConfig{{ID: "gpt-4o", Enabled: true}},
		Scoring:       DefaultScoringConfig(),
	}
	engine := NewScorerEngine(store, cfg, "", "", 0, true)
	if !engine.IsDisabled() {
		t.Fatal("启用但无 key 应返回 IsDisabled=true")
	}
}

// ---- 缓存命中 ----

func TestScorerEngine_CacheHit_ReturnsImmediately(t *testing.T) {
	store := newTestStore(t)
	k := insertPaperWithAbstract(t, store, "Kxq", "论文", "摘要")

	// 预存缓存
	_ = store.SaveCitationScore(model.CitationScore{
		CiteKey: k, SentenceHash: "hash1", ModelID: "mock-a",
		PromptVersion: "v1", Score: 0.85, Rationale: "一致",
	})
	_ = store.SaveCitationScore(model.CitationScore{
		CiteKey: k, SentenceHash: "hash1", ModelID: "mock-b",
		PromptVersion: "v1", Score: 0.90, Rationale: "一致",
	})

	// 用 mock scorer 构造引擎（即使 scorer 返回错误，缓存命中也不应调用）
	scorers := []Scorer{
		&mockScorer{modelID: "mock-a", promptVersion: "v1", fail: true},
		&mockScorer{modelID: "mock-b", promptVersion: "v1", fail: true},
	}
	engine := &ScorerEngine{
		store: store, scorers: scorers,
		config: DefaultScoringConfig(), promptVersion: "v1",
	}

	result, err := engine.Score(context.Background(), k, "hash1", "句子", "摘要")
	if err != nil {
		t.Fatalf("Score: %v", err)
	}
	if result == nil {
		t.Fatal("缓存命中应返回结果")
	}
	if !result.Cached {
		t.Fatal("应标记为 Cached=true")
	}
	if result.MeanScore != 0.88 {
		t.Fatalf("MeanScore 应为 0.88，got %f", result.MeanScore)
	}
	if len(result.PerModel) != 2 {
		t.Fatalf("应含 2 个模型评分，got %d", len(result.PerModel))
	}
}

// ---- 缓存未命中（扇出） ----

func TestScorerEngine_CacheMiss_FansOut(t *testing.T) {
	store := newTestStore(t)
	k := insertPaperWithAbstract(t, store, "Kxq", "论文", "摘要")

	scorers := []Scorer{
		&mockScorer{modelID: "mock-a", promptVersion: "v1", score: 0.85, rationale: "好"},
		&mockScorer{modelID: "mock-b", promptVersion: "v1", score: 0.75, rationale: "不错"},
	}
	engine := &ScorerEngine{
		store: store, scorers: scorers,
		config: DefaultScoringConfig(), promptVersion: "v1",
	}

	result, err := engine.Score(context.Background(), k, "hash2", "句子", "摘要")
	if err != nil {
		t.Fatalf("Score: %v", err)
	}
	if result == nil {
		t.Fatal("缓存未命中应返回结果")
	}
	if result.Cached {
		t.Fatal("不应标记为 Cached=true")
	}
	if result.MeanScore != 0.80 {
		t.Fatalf("MeanScore 应为 0.80，got %f", result.MeanScore)
	}

	// 验证缓存已写入
	cs, _ := store.GetCitationScore(k, "hash2", "mock-a", "v1")
	if cs == nil || cs.Score != 0.85 {
		t.Fatal("mock-a 评分应已缓存")
	}
	cs, _ = store.GetCitationScore(k, "hash2", "mock-b", "v1")
	if cs == nil || cs.Score != 0.75 {
		t.Fatal("mock-b 评分应已缓存")
	}
}

// ---- 部分缓存命中 ----

func TestScorerEngine_PartialCache_PartialFanOut(t *testing.T) {
	store := newTestStore(t)
	k := insertPaperWithAbstract(t, store, "Kxq", "论文", "摘要")

	// mock-a 已缓存
	_ = store.SaveCitationScore(model.CitationScore{
		CiteKey: k, SentenceHash: "hash3", ModelID: "mock-a",
		PromptVersion: "v1", Score: 0.90, Rationale: "已知",
	})

	scorers := []Scorer{
		&mockScorer{modelID: "mock-a", promptVersion: "v1", score: 0.99, rationale: "新评分"},
		&mockScorer{modelID: "mock-b", promptVersion: "v1", score: 0.80, rationale: "不错"},
	}
	engine := &ScorerEngine{
		store: store, scorers: scorers,
		config: DefaultScoringConfig(), promptVersion: "v1",
	}

	result, err := engine.Score(context.Background(), k, "hash3", "句子", "摘要")
	if err != nil {
		t.Fatalf("Score: %v", err)
	}
	if result == nil {
		t.Fatal("应返回结果")
	}
	if result.Cached {
		t.Fatal("部分缓存不应标记为 Cached=true")
	}
	// mock-a 分值来自缓存 (0.90)，mock-b 来自新评分 (0.80)
	// mean = (0.90+0.80)/2 = 0.85
	if result.MeanScore != 0.85 {
		t.Fatalf("MeanScore 应为 0.85，got %f", result.MeanScore)
	}
}

// ---- 部分模型失败 ----

func TestScorerEngine_PartialFailure_DegradesGracefully(t *testing.T) {
	store := newTestStore(t)
	k := insertPaperWithAbstract(t, store, "Kxq", "论文", "摘要")

	scorers := []Scorer{
		&mockScorer{modelID: "mock-a", promptVersion: "v1", score: 0.85, rationale: "好"},
		&mockScorer{modelID: "mock-b", promptVersion: "v1", fail: true},
		&mockScorer{modelID: "mock-c", promptVersion: "v1", score: 0.75, rationale: "不错"},
	}
	engine := &ScorerEngine{
		store: store, scorers: scorers,
		config: DefaultScoringConfig(), promptVersion: "v1",
	}

	result, err := engine.Score(context.Background(), k, "hash4", "句子", "摘要")
	if err != nil {
		t.Fatalf("Score: %v", err)
	}
	if result == nil {
		t.Fatal("部分模型失败应降级返回")
	}
	// 只有 mock-a(0.85) 和 mock-c(0.75) 成功
	if result.MeanScore != 0.80 {
		t.Fatalf("MeanScore 应为 0.80，got %f", result.MeanScore)
	}
	// 检查失败模型标记
	var foundFail bool
	for _, m := range result.PerModel {
		if m.ModelID == "mock-b" && m.Failed {
			foundFail = true
			break
		}
	}
	if !foundFail {
		t.Fatal("mock-b 应标记为 Failed")
	}
}

// ---- 全部模型失败 ----

func TestScorerEngine_AllFail_ReturnsNil(t *testing.T) {
	store := newTestStore(t)
	k := insertPaperWithAbstract(t, store, "Kxq", "论文", "摘要")

	scorers := []Scorer{
		&mockScorer{modelID: "mock-a", promptVersion: "v1", fail: true},
		&mockScorer{modelID: "mock-b", promptVersion: "v1", fail: true},
	}
	engine := &ScorerEngine{
		store: store, scorers: scorers,
		config: DefaultScoringConfig(), promptVersion: "v1",
	}

	result, err := engine.Score(context.Background(), k, "hash5", "句子", "摘要")
	if err != nil {
		t.Fatalf("Score: %v", err)
	}
	if result != nil {
		t.Fatal("全部模型失败应返回 nil")
	}
}

// ---- 一致率 ----

func TestScorerEngine_Consensus_High(t *testing.T) {
	store := newTestStore(t)
	k := insertPaperWithAbstract(t, store, "Kxq", "论文", "摘要")

	scorers := []Scorer{
		&mockScorer{modelID: "mock-a", promptVersion: "v1", score: 0.85, rationale: "a"},
		&mockScorer{modelID: "mock-b", promptVersion: "v1", score: 0.87, rationale: "b"},
		&mockScorer{modelID: "mock-c", promptVersion: "v1", score: 0.83, rationale: "c"},
	}
	engine := &ScorerEngine{
		store: store, scorers: scorers,
		config: DefaultScoringConfig(), promptVersion: "v1",
	}

	result, err := engine.Score(context.Background(), k, "hash6", "句子", "摘要")
	if err != nil {
		t.Fatalf("Score: %v", err)
	}
	if result.Consensus < 0.9 {
		t.Fatalf("相近分数应有一致率 >= 0.9，got %f", result.Consensus)
	}
}

func TestScorerEngine_Consensus_Low(t *testing.T) {
	store := newTestStore(t)
	k := insertPaperWithAbstract(t, store, "Kxq", "论文", "摘要")

	scorers := []Scorer{
		&mockScorer{modelID: "mock-a", promptVersion: "v1", score: 0.95, rationale: "a"},
		&mockScorer{modelID: "mock-b", promptVersion: "v1", score: 0.20, rationale: "b"},
	}
	engine := &ScorerEngine{
		store: store, scorers: scorers,
		config: DefaultScoringConfig(), promptVersion: "v1",
	}

	result, err := engine.Score(context.Background(), k, "hash7", "句子", "摘要")
	if err != nil {
		t.Fatalf("Score: %v", err)
	}
	if result.Consensus > 0.5 {
		t.Fatalf("差异大的分数应有一致率 < 0.5，got %f", result.Consensus)
	}
}

// ---- 辅助方法 ----

func TestScorerEngine_EnabledModels(t *testing.T) {
	store := newTestStore(t)
	cfg := &VerifierModels{
		PromptVersion: "v1",
		Models: []ModelConfig{
			{ID: "model-a", Enabled: true, APIKey: "key-a"},
			{ID: "model-b", Enabled: true, APIKey: "key-b"},
			{ID: "model-c", Enabled: false},
		},
		Scoring: DefaultScoringConfig(),
	}
	engine := NewScorerEngine(store, cfg, "", "", 0, true)
	ids := engine.EnabledModels()
	if len(ids) != 2 {
		t.Fatalf("应返回 2 个启用模型，got %d: %v", len(ids), ids)
	}
	if ids[0] != "model-a" || ids[1] != "model-b" {
		t.Fatalf("模型 ID 不匹配，got %v", ids)
	}
}

// ---- 并发安全 ----

func TestScorerEngine_ConcurrentSafe(t *testing.T) {
	store := newTestStore(t)
	k := insertPaperWithAbstract(t, store, "Kxq", "论文", "摘要")

	scorers := []Scorer{
		&mockScorer{modelID: "mock-a", promptVersion: "v1", score: 0.85, rationale: "a"},
		&mockScorer{modelID: "mock-b", promptVersion: "v1", score: 0.80, rationale: "b"},
	}
	engine := &ScorerEngine{
		store: store, scorers: scorers,
		config: DefaultScoringConfig(), promptVersion: "v1",
	}

	// 并发调用 10 次
	done := make(chan struct{}, 10)
	for i := 0; i < 10; i++ {
		go func() {
			_, _ = engine.Score(context.Background(), k, "hash-concurrent", "句子", "摘要")
			done <- struct{}{}
		}()
	}
	for i := 0; i < 10; i++ {
		<-done
	}
	// 不 panic 即通过
}

// ---- 超时传递 ----

func TestScorerEngine_TimeoutRespected(t *testing.T) {
	store := newTestStore(t)
	k := insertPaperWithAbstract(t, store, "Kxq", "论文", "摘要")

	// 创建一个超时极短的引擎
	cfg := &VerifierModels{
		PromptVersion: "v1",
		Models:        []ModelConfig{{ID: "gpt-4o", Enabled: true, APIKey: "sk-test", BaseURL: "http://localhost:19999"}},
		Scoring:       DefaultScoringConfig(),
	}
	engine := NewScorerEngine(store, cfg, "", "", 1*time.Millisecond, true)
	if engine.IsDisabled() {
		t.Fatal("有 key 模型不应禁用")
	}

	result, err := engine.Score(context.Background(), k, "hash-timeout", "句子", "摘要")
	if err != nil {
		t.Fatalf("Score: %v", err)
	}
	// 连接超时导致模型失败，但应降级返回 nil（只有一个模型）
	if result != nil {
		t.Fatal("超时导致全部模型失败应返回 nil")
	}
}
