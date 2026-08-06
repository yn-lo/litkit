package core

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestNewLLMScorer_Defaults(t *testing.T) {
	s := NewLLMScorer("gpt-4o", "sk-test", "", "", 0)
	if s.ModelID() != "gpt-4o" {
		t.Fatalf("ModelID 应为 gpt-4o，got %q", s.ModelID())
	}
	if s.PromptVersion() != "v1" {
		t.Fatalf("PromptVersion 默认应为 v1，got %q", s.PromptVersion())
	}
	if s.baseURL != "https://api.openai.com/v1" {
		t.Fatalf("baseURL 默认应为 OpenAI，got %q", s.baseURL)
	}
}

func TestLLMScorer_Score_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer sk-test" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		resp := map[string]any{
			"choices": []any{
				map[string]any{
					"message": map[string]any{
						"content": `{"score": 0.85, "rationale": "摘要与引用句一致"}`,
					},
				},
			},
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	s := NewLLMScorer("gpt-4o", "sk-test", srv.URL, "v1", 0)
	score, rationale, err := s.Score(context.Background(), "引用句", "摘要内容")
	if err != nil {
		t.Fatalf("Score: %v", err)
	}
	if score != 0.85 {
		t.Fatalf("score 应为 0.85，got %f", score)
	}
	if !strings.Contains(rationale, "摘要") {
		t.Fatalf("rationale 应含「摘要」，got %q", rationale)
	}
}

func TestLLMScorer_Score_HTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte("rate limit"))
	}))
	defer srv.Close()

	s := NewLLMScorer("gpt-4o", "sk-test", srv.URL, "v1", 0)
	_, _, err := s.Score(context.Background(), "s", "a")
	if err == nil {
		t.Fatal("429 应返回错误")
	}
}

func TestDefaultScoringConfig(t *testing.T) {
	c := DefaultScoringConfig()
	if c.MinModels != 2 {
		t.Fatalf("MinModels 默认应为 2，got %d", c.MinModels)
	}
	if c.AgreementRatio != 0.67 {
		t.Fatalf("AgreementRatio 默认应为 0.67，got %f", c.AgreementRatio)
	}
}

func TestVerifierModels_JSONRoundTrip(t *testing.T) {
	vm := VerifierModels{
		PromptVersion:      "v1",
		ConsensusThreshold: 0.7,
		Models: []ModelConfig{
			{ID: "gpt-4o", Provider: "openai", Enabled: true, Weight: 1.0},
		},
		Scoring: DefaultScoringConfig(),
	}
	data, err := json.Marshal(vm)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	var got VerifierModels
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if got.PromptVersion != "v1" {
		t.Fatalf("PromptVersion round-trip 失败")
	}
	if len(got.Models) != 1 || got.Models[0].ID != "gpt-4o" {
		t.Fatalf("Models round-trip 失败")
	}
}
