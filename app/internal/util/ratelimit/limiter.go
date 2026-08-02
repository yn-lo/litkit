// Package ratelimit 提供每源令牌桶限速器（叶子层）。
//
// 用于满足 NFR-PERF-04（单源 ≥10 次/分钟且不触发上游 429）与 NFR-REL-04（每源全局限速器）。
// 基于 golang.org/x/time/rate 实现，按源合规间隔取保守值。
package ratelimit

import (
	"context"

	"golang.org/x/time/rate"
)

// Limiter 每源令牌桶限速器。
//
// 封装 x/time/rate.Limiter，统一 nil 处理：nil 视作无限制，便于可选注入。
type Limiter struct {
	limiter *rate.Limiter
}

// New 创建限速器。
//   - rps：每秒允许的请求数
//   - burst：突发桶容量
//
// 例：arXiv 官方 1 req/3s ≈ 0.33 RPS，burst 1。
func New(rps float64, burst int) *Limiter {
	return &Limiter{limiter: rate.NewLimiter(rate.Limit(rps), burst)}
}

// Allow 等待直到获取令牌或 ctx 取消。
// 返回 true 表示获取成功，false 表示 ctx 取消。
// nil Limiter 视作无限制，直接返回 true。
func (l *Limiter) Allow(ctx context.Context) bool {
	if l == nil || l.limiter == nil {
		return true
	}
	return l.limiter.Wait(ctx) == nil
}
