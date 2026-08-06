// scorer.go 引用相关性评分抽象（FR-LINT-08）。
//
// Scorer 接口是"多模型交叉打分"的核心抽象：
//   - 每个模型实现 Scorer 接口，由 LLMScorer 包裹具体 API 调用
//   - 评分结果经存储层增量缓存，自动由主键（sentence_hash+model_id+prompt_version）失效
//   - 漏斗中 Layer 2 使用：仅对 Layer 0/1 标记的高风险项调用

package core

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

// Scorer 引用相关性评分接口（FR-LINT-08）。
//
// Score 对一句话与其引用文献的摘要做相关性评分。
// 返回 0.0 ~ 1.0 分数 + 简短理由（中文）。
// 调用方负责缓存（store.GetCitationScore / SaveCitationScore）。
type Scorer interface {
	Score(ctx context.Context, sentence, abstract string) (score float64, rationale string, err error)
	ModelID() string
	PromptVersion() string
}

// ModelConfig 对应 verifier_models.json 中 models[] 项的运行时形态。
// API key 不在 JSON 中，由 config.ModelID 查 LITKIT_LLM_API_KEY。
type ModelConfig struct {
	ID       string  `json:"id"`
	Provider string  `json:"provider"`
	Enabled  bool    `json:"enabled"`
	Weight   float64 `json:"weight"`
	APIKey   string  // 运行时注入，不走 JSON
	BaseURL  string  // 运行时注入，LITKIT_LLM_BASE_URL
}

// ScoringConfig 对应 verifier_models.json 的 scoring 段。
type ScoringConfig struct {
	MinModels            int     `json:"min_models"`
	AgreementRatio       float64 `json:"agreement_ratio"`
	LowScoreThreshold    float64 `json:"low_score_threshold"`
	MediumScoreThreshold float64 `json:"medium_score_threshold"`
}

// VerifierModels 对应 verifier_models.json 顶层结构。
type VerifierModels struct {
	PromptVersion      string        `json:"prompt_version"`
	ConsensusThreshold float64       `json:"consensus_threshold"`
	Models             []ModelConfig `json:"models"`
	Scoring            ScoringConfig `json:"scoring"`
}

// LoadVerifierModels 从指定路径加载 verifier_models.json。
// path 为空时返回 nil（禁用模式）。
func LoadVerifierModels(path string) (*VerifierModels, error) {
	if path == "" {
		return nil, nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("verifier_models: %w", err)
	}
	var vm VerifierModels
	if err := json.Unmarshal(data, &vm); err != nil {
		return nil, fmt.Errorf("verifier_models: %w", err)
	}
	if vm.Scoring.MinModels == 0 {
		vm.Scoring = DefaultScoringConfig()
	}
	return &vm, nil
}

// DefaultScoringConfig 默认评分阈值（对应 verifier_models.json 模板值）。
func DefaultScoringConfig() ScoringConfig {
	return ScoringConfig{
		MinModels:            2,
		AgreementRatio:       0.67, //nolint: mnd
		LowScoreThreshold:    0.3,  //nolint: mnd
		MediumScoreThreshold: 0.6,  //nolint: mnd
	}
}

// ScorePrompt 评分提示词模板。
// 返回"引用句"与"文献摘要"的吻合度分数（0.0~1.0）及理由。
const ScorePrompt = `你是一个学术引用质量审查助手。请判断以下引用句与其所引文献摘要的相关性。

## 引用句
{{sentence}}

## 文献摘要
{{abstract}}

## 评分规则
- 1.0: 完全吻合——引用句内容直接来自该摘要，核心结论/数据/方法一致
- 0.7~0.9: 高度相关——引用句描述的工作与摘要一致，但细节不完全匹配
- 0.4~0.6: 部分相关——引用句涉及的主题在摘要中出现，但非核心内容
- 0.1~0.3: 弱相关——引用句与摘要的主题领域相关，但具体内容不符
- 0.0: 不相关——引用句描述的内容在摘要中完全不存在，或包含摘要中不存在的具体数字/结论

## 重点检查
1. 引用句中出现的具体数字（百分比、样本量、年份、p值等）是否在摘要中可找到
2. 引用句的核心结论是否与摘要一致
3. 引用句是否将摘要未提及的发现归因于该文献

请以 JSON 格式返回：
{"score": 0.85, "rationale": "理由（中文，20字以内）"}`

// LLMResponse OpenAI 兼容 API 的 chat completion 响应格式。
type LLMResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
}

// LLMScorer 基于 OpenAI 兼容 API 的引用评分器。
type LLMScorer struct {
	modelID       string
	promptVersion string
	apiKey        string
	baseURL       string
	httpClient    *http.Client
}

// NewLLMScorer 创建 LLM 评分器。
//
// baseURL 为空时默认使用 OpenAI 官方 endpoint。
// timeout 为 0 时使用默认 30s 超时。
func NewLLMScorer(modelID, apiKey, baseURL, promptVersion string, timeout time.Duration) *LLMScorer {
	if baseURL == "" {
		baseURL = "https://api.openai.com/v1"
	}
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	if promptVersion == "" {
		promptVersion = "v1"
	}
	return &LLMScorer{
		modelID:       modelID,
		promptVersion: promptVersion,
		apiKey:        apiKey,
		baseURL:       strings.TrimRight(baseURL, "/"),
		httpClient:    &http.Client{Timeout: timeout},
	}
}

// ModelID 返回该评分器的模型标识。
func (s *LLMScorer) ModelID() string { return s.modelID }

// PromptVersion 返回该评分器使用的提示词版本。
func (s *LLMScorer) PromptVersion() string { return s.promptVersion }

// Score 调用 LLM API 对引用句与摘要做相关性评分。
func (s *LLMScorer) Score(ctx context.Context, sentence, abstract string) (float64, string, error) {
	prompt := strings.ReplaceAll(ScorePrompt, "{{sentence}}", sentence)
	prompt = strings.ReplaceAll(prompt, "{{abstract}}", abstract)

	body := fmt.Sprintf(`{
		"model": %q,
		"messages": [{"role": "user", "content": %q}],
		"temperature": 0.1,
		"max_tokens": 150
	}`, s.modelID, prompt)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.baseURL+"/chat/completions", strings.NewReader(body))
	if err != nil {
		return 0, "", fmt.Errorf("scorer: 构造请求: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+s.apiKey)

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return 0, "", fmt.Errorf("scorer: API 调用: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return 0, "", fmt.Errorf("scorer: HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(respBody)))
	}

	var llmResp LLMResponse
	if err := json.NewDecoder(resp.Body).Decode(&llmResp); err != nil {
		return 0, "", fmt.Errorf("scorer: 解析响应: %w", err)
	}
	if len(llmResp.Choices) == 0 {
		return 0, "", fmt.Errorf("scorer: 空 choices")
	}

	content := strings.TrimSpace(llmResp.Choices[0].Message.Content)
	// 尝试解析 JSON 响应
	var scoreResult struct {
		Score     float64 `json:"score"`
		Rationale string  `json:"rationale"`
	}
	if err := json.Unmarshal([]byte(content), &scoreResult); err != nil {
		return 0, "", fmt.Errorf("scorer: 解析评分 JSON: %w (content=%q)", err, content)
	}

	if scoreResult.Score < 0 || scoreResult.Score > 1 {
		return 0, "", fmt.Errorf("scorer: 分数越界 %f", scoreResult.Score)
	}
	return scoreResult.Score, scoreResult.Rationale, nil
}
