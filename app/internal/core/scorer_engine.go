// scorer_engine.go 多模型扇出评分引擎（FR-LINT-08）。
//
// ScorerEngine 是三层漏斗中 Layer 2 的编排器：
//   - 缓存优先：查 citation_scores 表，全命中直接返回聚合结果
//   - 扇出并发：未命中模型用 errgroup 并行调用 LLMScorer
//   - 优雅降级：部分模型失败不影响其他模型，只看剩余模型的一致率
//   - 禁用模式：LITKIT_VERIFY_LINT_LLM=false 或全部模型无 key 时静默跳过
//
// 调用方（verify pipeline）只需检查 ScoreResult == nil 决定是否跳过报告。

package core

import (
	"context"
	"fmt"
	"math"
	"sync"
	"time"

	"golang.org/x/sync/errgroup"

	"litkit/internal/model"
	"litkit/internal/storage"
)

// ModelScore 单模型评分明细。
type ModelScore struct {
	ModelID   string  `json:"modelId"`
	Score     float64 `json:"score"`
	Rationale string  `json:"rationale"`
	Failed    bool    `json:"failed"`
	Error     string  `json:"error,omitempty"`
}

// ScoreResult 一次引用的多模型交叉评分结果。
type ScoreResult struct {
	CiteKey       string       `json:"citeKey"`
	SentenceHash  string       `json:"sentenceHash"`
	MeanScore     float64      `json:"meanScore"` // 成功模型平均分
	Consensus     float64      `json:"consensus"` // 一致率 [0, 1]，越高越一致
	PerModel      []ModelScore `json:"perModel"`
	Cached        bool         `json:"cached"` // 是否全部来自缓存
	PromptVersion string       `json:"promptVersion"`
}

// ScorerEngine 多模型扇出评分引擎。
type ScorerEngine struct {
	disabled      bool // 禁用模式（LITKIT_VERIFY_LINT_LLM=false 或无可启用的模型）
	store         *storage.Store
	scorers       []Scorer // 启用的模型评分器
	config        ScoringConfig
	promptVersion string
}

// cachedInfo 评分缓存条目，在 Score 方法中用于合并缓存与新鲜评分。
type cachedInfo struct {
	score     float64
	rationale string
}

// NewScorerEngine 创建扇出评分引擎。
//
// 参数：
//   - store：存储层（缓存读写）
//   - cfg：verifier_models.json 解析后的配置
//   - apiKey：LITKIT_LLM_API_KEY（所有模型共用）
//   - baseURL：LITKIT_LLM_BASE_URL（可选，自托管 endpoint）
//   - timeout：LLM 单次评分超时
//   - enabled：LITKIT_VERIFY_LINT_LLM 开关
//
// 如果 !enabled 或没有可启用的模型（启用但无 key），返回禁用引擎。
func NewScorerEngine(store *storage.Store, cfg *VerifierModels, apiKey, baseURL string, timeout time.Duration, enabled bool) *ScorerEngine {
	if !enabled || cfg == nil {
		return &ScorerEngine{disabled: true}
	}

	var scorers []Scorer
	for _, m := range cfg.Models {
		if !m.Enabled {
			continue
		}

		key := m.APIKey
		if key == "" {
			key = apiKey
		}
		if key == "" {
			continue // 启用但无 key，跳过该模型
		}

		u := m.BaseURL
		if u == "" {
			u = baseURL
		}

		scorers = append(scorers, NewLLMScorer(m.ID, key, u, cfg.PromptVersion, timeout))
	}

	if len(scorers) == 0 {
		return &ScorerEngine{disabled: true}
	}

	return &ScorerEngine{
		store:         store,
		scorers:       scorers,
		config:        cfg.Scoring,
		promptVersion: cfg.PromptVersion,
	}
}

// Score 对一句话与其引用文献的摘要做多模型交叉评分。
//
// 返回 (nil, nil) 表示禁用模式或全部模型失败——调用方静默跳过即可。
func (e *ScorerEngine) Score(ctx context.Context, citeKey, sentenceHash, sentence, abstract string) (*ScoreResult, error) {
	if e.disabled {
		return nil, nil
	}

	// 1. 查缓存——按模型逐个查，标记已缓存的
	cached := make(map[string]cachedInfo, len(e.scorers))
	var uncached []Scorer // 未命中的模型

	for _, s := range e.scorers {
		cs, err := e.store.GetCitationScore(citeKey, sentenceHash, s.ModelID(), s.PromptVersion())
		if err != nil || cs == nil {
			uncached = append(uncached, s)
			continue
		}
		cached[s.ModelID()] = cachedInfo{score: cs.Score, rationale: cs.Rationale}
	}

	// 全部缓存命中 → 直接聚合返回
	if len(uncached) == 0 {
		result := e.aggregate(citeKey, sentenceHash, cached, nil)
		result.Cached = true
		return result, nil
	}

	// 2. 扇出未命中模型
	var muResult sync.Mutex
	perModel := make([]ModelScore, 0, len(uncached))
	eg, ctx := errgroup.WithContext(ctx)

	for _, s := range uncached {
		s := s // capture
		eg.Go(func() error {
			score, rationale, err := s.Score(ctx, sentence, abstract)
			ms := ModelScore{
				ModelID: s.ModelID(),
			}
			if err != nil {
				ms.Failed = true
				ms.Error = err.Error()
			} else {
				ms.Score = score
				ms.Rationale = rationale
				// 落缓存
				_ = e.store.SaveCitationScore(model.CitationScore{
					CiteKey:       citeKey,
					SentenceHash:  sentenceHash,
					ModelID:       s.ModelID(),
					PromptVersion: s.PromptVersion(),
					Score:         score,
					Rationale:     rationale,
				})
			}
			muResult.Lock()
			perModel = append(perModel, ms)
			muResult.Unlock()
			return nil // 不返回错误——单个模型失败不阻断整体
		})
	}

	_ = eg.Wait() // 忽略扇出错误——各模型失败已记入 perModel

	// 3. 合并缓存 + 新评分
	all := make(map[string]cachedInfo, len(e.scorers))
	for id, c := range cached {
		all[id] = c
	}
	for _, ms := range perModel {
		if !ms.Failed {
			all[ms.ModelID] = cachedInfo{score: ms.Score, rationale: ms.Rationale}
		}
	}
	result := e.aggregate(citeKey, sentenceHash, all, perModel)
	return result, nil
}

// aggregate 从成功模型评分聚合出最终结果。
func (e *ScorerEngine) aggregate(citeKey, sentenceHash string, scored map[string]cachedInfo, newScores []ModelScore) *ScoreResult {
	if len(scored) == 0 {
		return nil
	}

	var total float64
	var count int
	for _, c := range scored {
		total += c.score
		count++
	}
	mean := total / float64(count)

	// 一致率 = 1 - min(1.0, maxDeviation / 0.5)
	var maxDev float64
	for _, c := range scored {
		dev := math.Abs(c.score - mean)
		if dev > maxDev {
			maxDev = dev
		}
	}
	consensus := 1.0 - math.Min(1.0, maxDev/0.5) //nolint: mnd

	// 合并全部评分明细（缓存 + 新评分）
	// 用 map 去重，缓存评分优先用新评分（如果新评分有同模型）
	seen := map[string]bool{}
	var all []ModelScore
	for _, ms := range newScores {
		all = append(all, ms)
		seen[ms.ModelID] = true
	}
	for id, c := range scored {
		if !seen[id] {
			all = append(all, ModelScore{
				ModelID:   id,
				Score:     c.score,
				Rationale: c.rationale,
			})
		}
	}

	return &ScoreResult{
		CiteKey:       citeKey,
		SentenceHash:  sentenceHash,
		MeanScore:     math.Round(mean*100) / 100,      //nolint: mnd
		Consensus:     math.Round(consensus*100) / 100, //nolint: mnd
		PerModel:      all,
		PromptVersion: e.promptVersion,
	}
}

// IsDisabled 报告引擎是否处于禁用模式。
func (e *ScorerEngine) IsDisabled() bool {
	return e.disabled
}

// EnabledModels 返回已启用的模型 ID 列表（用于日志/调试）。
func (e *ScorerEngine) EnabledModels() []string {
	if e.disabled {
		return nil
	}
	ids := make([]string, len(e.scorers))
	for i, s := range e.scorers {
		ids[i] = s.ModelID()
	}
	return ids
}

// Verify that *ScorerEngine implements a useful interface
var _ = (*ScorerEngine)(nil).IsDisabled

// Format 为 ScoreResult 实现 fmt.Stringer，便于调试/日志输出。
func (r *ScoreResult) String() string {
	return fmt.Sprintf("Score{mean=%.2f, consensus=%.2f, models=%d, cached=%v}",
		r.MeanScore, r.Consensus, len(r.PerModel), r.Cached)
}
