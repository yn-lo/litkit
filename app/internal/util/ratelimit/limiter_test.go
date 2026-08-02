package ratelimit

import (
	"context"
	"testing"
	"time"
)

func TestLimiter_AllowImmediately(t *testing.T) {
	// 大速率下首令牌立即可用
	l := New(100, 10) // 100 RPS, burst 10
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	if !l.Allow(ctx) {
		t.Fatal("首令牌应立即可用")
	}
}

func TestLimiter_WaitBlocksThenAllows(t *testing.T) {
	// 速率 1 RPS, burst 1：第二个令牌需等待 ~1s
	// 用 50ms 模拟以加速测试
	l := New(20, 1) // 20 RPS = 每 50ms 一个令牌
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	start := time.Now()
	if !l.Allow(ctx) {
		t.Fatal("首令牌应立即可用")
	}
	if !l.Allow(ctx) {
		t.Fatal("第二令牌应在 50ms 内可用")
	}
	elapsed := time.Since(start)
	if elapsed < 30*time.Millisecond {
		t.Fatalf("第二令牌应阻塞约 50ms，实际 %v", elapsed)
	}
}

func TestLimiter_ContextCancelInterrupts(t *testing.T) {
	// 速率极低，令牌需长时间等待；ctx 取消应中断
	l := New(1, 1) // 1 RPS, burst 1（首令牌立即可用）
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Millisecond)
	defer cancel()

	if !l.Allow(ctx) {
		t.Fatal("首令牌应可用")
	}
	// 此时令牌桶空，第二次 Allow 需等 1s
	if l.Allow(ctx) {
		t.Fatal("ctx 已取消/超时，Allow 应返回 false")
	}
}

func TestLimiter_NilDoesNotBlock(t *testing.T) {
	// nil 限速器视作无限制（便于可选注入）
	var l *Limiter
	ctx := context.Background()
	if !l.Allow(ctx) {
		t.Fatal("nil Limiter 应允许通过")
	}
}
