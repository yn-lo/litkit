// Package httpclient 提供带超时与 429/503 指数退避重试的 HTTP 客户端（叶子层）。
//
// 用于满足 NFR-PERF-03（单源超时不阻塞整体）与 NFR-REL-01（429/503 指数退避重试）。
// 限速由调用方经 ratelimit.Limiter 注入；本客户端只负责超时与重试。
package httpclient

import (
	"context"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"strconv"
	"time"
)

// maxBackoffShift 指数退避位移上限（2^6=64 倍基数，约 6.4s @ base=100ms）。
const maxBackoffShift = 6

// Options HTTP 客户端选项。
type Options struct {
	TimeoutMS     int // 单请求超时（毫秒），0 表示无超时
	MaxRetries    int // 429/503 重试次数（不含首次请求）
	BackoffBaseMS int // 指数退避基数（毫秒），默认 100
}

// Client HTTP 客户端，封装 net/http.Client + 重试逻辑。
type Client struct {
	http        *http.Client
	maxRetries  int
	backoffBase int
}

// New 创建客户端。
func New(opts Options) *Client {
	timeout := time.Duration(opts.TimeoutMS) * time.Millisecond
	if opts.TimeoutMS == 0 {
		timeout = 0
	}
	backoffBase := opts.BackoffBaseMS
	if backoffBase == 0 {
		backoffBase = 100
	}
	return &Client{
		http:        &http.Client{Timeout: timeout},
		maxRetries:  opts.MaxRetries,
		backoffBase: backoffBase,
	}
}

// Do 执行 HTTP 请求，对 429/503 进行指数退避重试。
//
// 调用方负责关闭 resp.Body（若 err == nil）。
// ctx 取消会中断重试等待。
func (c *Client) Do(ctx context.Context, req *http.Request) (*http.Response, error) {
	var resp *http.Response
	var err error
	for attempt := 0; attempt <= c.maxRetries; attempt++ {
		if err = ctx.Err(); err != nil {
			return nil, fmt.Errorf("http do: %w", err)
		}
		resp, err = c.doOnce(ctx, req)
		if err != nil {
			return nil, fmt.Errorf("http do: %w", err)
		}
		if !shouldRetry(resp.StatusCode) || attempt == c.maxRetries {
			return resp, nil
		}
		// 准备重试：关闭当前响应体，等待退避
		_ = resp.Body.Close()
		if !c.sleepForRetry(ctx, resp, attempt) {
			return nil, fmt.Errorf("http do: context cancelled during retry backoff: %w", ctx.Err())
		}
	}
	return resp, nil
}

// doOnce 执行单次请求，复用 req.Body（每次重试重置）。
func (c *Client) doOnce(ctx context.Context, req *http.Request) (*http.Response, error) {
	// 关联 ctx 以支持超时与取消
	req = req.Clone(ctx)
	return c.http.Do(req)
}

// sleepForRetry 等待退避时间。返回 false 表示 ctx 已取消。
func (c *Client) sleepForRetry(ctx context.Context, resp *http.Response, attempt int) bool {
	wait := c.backoffDuration(attempt)
	// 优先尊重 Retry-After 头
	if ra := resp.Header.Get("Retry-After"); ra != "" {
		if secs, perr := strconv.Atoi(ra); perr == nil && secs > 0 {
			wait = time.Duration(secs) * time.Second
		}
	}
	timer := time.NewTimer(wait)
	defer timer.Stop()
	select {
	case <-timer.C:
		return true
	case <-ctx.Done():
		return false
	}
}

func (c *Client) backoffDuration(attempt int) time.Duration {
	// 指数退避：base * 2^attempt + 随机抖动（0..base）
	base := time.Duration(c.backoffBase) * time.Millisecond
	shift := uint(attempt)
	if shift > maxBackoffShift {
		shift = maxBackoffShift // 上限 ~6.4s @ base=100ms
	}
	backoff := base << shift
	jitter := time.Duration(rand.Intn(c.backoffBase)) * time.Millisecond
	return backoff + jitter
}

func shouldRetry(status int) bool {
	return status == http.StatusTooManyRequests || status == http.StatusServiceUnavailable
}

// ReadAll 读取并关闭响应体，返回字节切片。
// 失败时仍确保 Body 关闭，便于调用方错误分支处理。
func ReadAll(resp *http.Response) ([]byte, error) {
	defer func() { _ = resp.Body.Close() }()
	return io.ReadAll(resp.Body)
}
