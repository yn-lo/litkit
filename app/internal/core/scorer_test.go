package core

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNewLLMScorer_Defaults(t *testing.T) {
	s := NewLLMScorer(ModelConfig{ID: "gpt-4o", APIKey: "sk-test"}, "", "", 0, nil)
	if s.ModelID() != "gpt-4o" {
		t.Fatalf("ModelID 应为 gpt-4o，got %q", s.ModelID())
	}
	if s.PromptVersion() != "v1" {
		t.Fatalf("PromptVersion 默认应为 v1，got %q", s.PromptVersion())
	}
	if s.baseURL != "https://api.openai.com/v1" {
		t.Fatalf("baseURL 默认应为 OpenAI，got %q", s.baseURL)
	}
	if s.temperature != DefaultScoreTemperature {
		t.Fatalf("temperature 默认应为 %v，got %v", DefaultScoreTemperature, s.temperature)
	}
	if s.maxTokens != DefaultScoreMaxTokens {
		t.Fatalf("maxTokens 默认应为 %d，got %d", DefaultScoreMaxTokens, s.maxTokens)
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

	s := NewLLMScorer(ModelConfig{ID: "gpt-4o", APIKey: "sk-test"}, srv.URL, "v1", 0, nil)
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

// TestLLMScorer_Score_TolerantContent 模型回复被 markdown 围栏或说明文字包裹时仍应解析出分数。
// 实测 gemini/agnes 系模型习惯返回 ```json 围栏，严格 Unmarshal 会整条丢弃评分。
func TestLLMScorer_Score_TolerantContent(t *testing.T) {
	cases := []struct {
		name    string
		content string
		want    float64
	}{
		{"裸 JSON", `{"score": 0.85, "rationale": "ok"}`, 0.85},
		{"json 围栏", "```json\n{\"score\": 0.7, \"rationale\": \"ok\"}\n```", 0.7},
		{"无语言围栏", "```\n{\"score\": 0.6, \"rationale\": \"ok\"}\n```", 0.6},
		{"围栏前有说明", "好的，结果如下：\n```json\n{\"score\": 0.4, \"rationale\": \"ok\"}\n```", 0.4},
		{"围栏后有说明", "```json\n{\"score\": 0.2, \"rationale\": \"ok\"}\n```\n如需调整请告知。", 0.2},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				resp := map[string]any{
					"choices": []any{map[string]any{"message": map[string]any{"content": tc.content}}},
				}
				_ = json.NewEncoder(w).Encode(resp)
			}))
			defer srv.Close()

			s := NewLLMScorer(ModelConfig{ID: "m", APIKey: "sk-test"}, srv.URL, "v1", 0, nil)
			score, _, err := s.Score(context.Background(), "引用句", "摘要")
			if err != nil {
				t.Fatalf("Score: %v", err)
			}
			if score != tc.want {
				t.Fatalf("score 应为 %v，got %v", tc.want, score)
			}
		})
	}
}

// TestLLMScorer_Score_NonJSONStillFails 非 JSON 回复仍应报错（容忍逻辑不得把错误吞掉）。
func TestLLMScorer_Score_NonJSONStillFails(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := map[string]any{
			"choices": []any{map[string]any{"message": map[string]any{"content": "我不知道该怎么打分。"}}},
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	s := NewLLMScorer(ModelConfig{ID: "m", APIKey: "sk-test"}, srv.URL, "v1", 0, nil)
	if _, _, err := s.Score(context.Background(), "引用句", "摘要"); err == nil {
		t.Fatal("非 JSON 回复应报错")
	}
}

func TestLLMScorer_Score_HTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte("rate limit"))
	}))
	defer srv.Close()

	s := NewLLMScorer(ModelConfig{ID: "gpt-4o", APIKey: "sk-test"}, srv.URL, "v1", 0, nil)
	_, _, err := s.Score(context.Background(), "s", "a")
	if err == nil {
		t.Fatal("429 应返回错误")
	}
}

func TestLLMScorer_RequestParams(t *testing.T) {
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		if err := json.Unmarshal(raw, &gotBody); err != nil {
			t.Errorf("请求体应为合法 JSON: %v", err)
		}
		resp := map[string]any{
			"choices": []any{map[string]any{"message": map[string]any{"content": `{"score": 0.5, "rationale": "ok"}`}}},
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	temp := 0.3
	tok := 200
	s := NewLLMScorer(ModelConfig{
		ID:          "qwen-plus",
		APIKey:      "sk-test",
		Temperature: &temp,
		MaxTokens:   &tok,
		ExtraBody:   map[string]any{"enable_thinking": false, "top_p": 0.9},
	}, srv.URL, "v1", 0, nil)
	if _, _, err := s.Score(context.Background(), "s", "a"); err != nil {
		t.Fatalf("Score: %v", err)
	}

	if gotBody["model"] != "qwen-plus" {
		t.Fatalf("model 应为 qwen-plus，got %v", gotBody["model"])
	}
	if gotBody["temperature"] != 0.3 {
		t.Fatalf("temperature 应为 0.3，got %v", gotBody["temperature"])
	}
	if gotBody["max_tokens"] != float64(200) {
		t.Fatalf("max_tokens 应为 200，got %v", gotBody["max_tokens"])
	}
	if gotBody["enable_thinking"] != false {
		t.Fatalf("extra_body.enable_thinking 应透传为 false，got %v", gotBody["enable_thinking"])
	}
	if gotBody["top_p"] != 0.9 {
		t.Fatalf("extra_body.top_p 应透传，got %v", gotBody["top_p"])
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

// writeTempConfig 写临时 verifier_models.json 并加载。
func loadTempConfig(t *testing.T, content string) (*VerifierModels, error) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "verifier_models.json")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("写临时配置: %v", err)
	}
	return LoadVerifierModels(path)
}

func TestLoadVerifierModels_StrictUnknownField(t *testing.T) {
	_, err := loadTempConfig(t, `{"prompt_version":"v1","models":[],"scoring":{"min_models":2},"enabld":true}`)
	if err == nil {
		t.Fatal("未知字段应报错（严格解码）")
	}
	if !strings.Contains(err.Error(), "enabld") {
		t.Fatalf("错误信息应含未知字段名，got %v", err)
	}
}

func TestLoadVerifierModels_TemplateValid(t *testing.T) {
	// 与 templates/verifier_models.json 同构的最小合法配置
	vm, err := loadTempConfig(t, `{
		"_comment": "doc",
		"prompt_version": "v1",
		"consensus_threshold": 0.7,
		"models": [
			{"id": "gpt-4o", "provider": "openai", "enabled": true, "weight": 1.0,
			 "api_key": "sk-json", "base_url": "https://api.example.com/v1",
			 "temperature": 0.1, "max_tokens": 150},
			{"id": "qwen-plus", "provider": "alibaba", "enabled": true, "weight": 1.0,
			 "extra_body": {"enable_thinking": false}}
		],
		"scoring": {"min_models": 2, "agreement_ratio": 0.67, "low_score_threshold": 0.3, "medium_score_threshold": 0.6}
	}`)
	if err != nil {
		t.Fatalf("合法配置不应报错: %v", err)
	}
	if len(vm.Models) != 2 {
		t.Fatalf("应解析 2 个模型，got %d", len(vm.Models))
	}
	if vm.Models[0].APIKey != "sk-json" {
		t.Fatalf("api_key 应从 JSON 解析，got %q", vm.Models[0].APIKey)
	}
	if vm.Models[0].BaseURL != "https://api.example.com/v1" {
		t.Fatalf("base_url 应从 JSON 解析，got %q", vm.Models[0].BaseURL)
	}
	if vm.Models[1].ExtraBody["enable_thinking"] != false {
		t.Fatalf("extra_body 应正确解析")
	}
}

func TestVerifierModels_Validate(t *testing.T) {
	newValid := func() VerifierModels {
		return VerifierModels{
			PromptVersion:      "v1",
			ConsensusThreshold: 0.7,
			Models:             []ModelConfig{{ID: "gpt-4o"}},
			Scoring:            DefaultScoringConfig(),
		}
	}
	base := newValid()
	if err := base.Validate(); err != nil {
		t.Fatalf("合法配置不应报错: %v", err)
	}

	cases := []struct {
		name    string
		mutate  func(*VerifierModels)
		wantSub string
	}{
		{"空 prompt_version", func(v *VerifierModels) { v.PromptVersion = "" }, "prompt_version"},
		{"重复模型 id", func(v *VerifierModels) {
			v.Models = append(v.Models, ModelConfig{ID: "gpt-4o"})
		}, "重复"},
		{"temperature 越界", func(v *VerifierModels) {
			temp := 2.5
			v.Models[0].Temperature = &temp
		}, "temperature"},
		{"max_tokens 非正", func(v *VerifierModels) {
			tok := 0
			v.Models[0].MaxTokens = &tok
		}, "max_tokens"},
		{"extra_body 保留键", func(v *VerifierModels) {
			v.Models[0].ExtraBody = map[string]any{"temperature": 0.9}
		}, "保留键"},
		{"min_models 为 0", func(v *VerifierModels) { v.Scoring.MinModels = 0 }, "min_models"},
		{"agreement_ratio 越界", func(v *VerifierModels) { v.Scoring.AgreementRatio = 1.5 }, "agreement_ratio"},
		{"low >= medium", func(v *VerifierModels) {
			v.Scoring.LowScoreThreshold, v.Scoring.MediumScoreThreshold = 0.7, 0.6
		}, "阈值非法"},
		{"consensus_threshold 越界", func(v *VerifierModels) { v.ConsensusThreshold = -0.1 }, "consensus_threshold"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			v := newValid()
			tc.mutate(&v)
			err := v.Validate()
			if err == nil {
				t.Fatalf("%s 应报错", tc.name)
			}
			if !strings.Contains(err.Error(), tc.wantSub) {
				t.Fatalf("错误信息应含 %q，got %v", tc.wantSub, err)
			}
		})
	}
}
