// Package httpclient 提供带超时与 429/503 指数退避重试的 HTTP 客户端（叶子层）。
//
// 用于满足 NFR-PERF-03（单源超时不阻塞整体）与 NFR-REL-01（429/503 指数退避重试）。
// 限速由调用方经 ratelimit.Limiter 注入；本客户端只负责超时与重试。
package httpclient

import (
	"context"
	"errors"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// maxBackoffShift 指数退避位移上限（2^6=64 倍基数，约 6.4s @ base=100ms）。
const maxBackoffShift = 6

// Options HTTP 客户端选项。
type Options struct {
	TimeoutMS     int      // 单请求超时（毫秒），0 表示无超时
	MaxRetries    int      // 429/503 重试次数（不含首次请求）
	BackoffBaseMS int      // 指数退避基数（毫秒），默认 100
	Proxy         *url.URL // 显式代理；nil=默认 Transport（尊重标准 HTTPS_PROXY 环境变量）
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
	hc := &http.Client{Timeout: timeout}
	if tr := ProxyTransport(opts.Proxy); tr != nil {
		hc.Transport = tr
	}
	return &Client{
		http:        hc,
		maxRetries:  opts.MaxRetries,
		backoffBase: backoffBase,
	}
}

// ParseProxyURL 解析并校验代理 URL，供入口层将 LITKIT_PROXY_URL 一次性转为 *url.URL。
// raw 为空返回 (nil, nil)；支持 http/https/socks5/socks5h（socks5h=域名解析也走代理）。
func ParseProxyURL(raw string) (*url.URL, error) {
	if raw == "" {
		return nil, nil
	}
	u, err := url.Parse(raw)
	if err != nil {
		return nil, fmt.Errorf("proxy url %q: %w", raw, err)
	}
	switch u.Scheme {
	case "http", "https", "socks5", "socks5h":
		return u, nil
	default:
		return nil, fmt.Errorf("proxy url %q: 不支持的协议 %q（应为 http/https/socks5）", raw, u.Scheme)
	}
}

// ProxyTransport 返回显式代理 Transport（克隆 DefaultTransport 保留 HTTP/2 与默认配置）。
// u 为 nil 返回 nil：调用方保持 nil Transport 即走默认代理规则（标准环境变量）。
func ProxyTransport(u *url.URL) http.RoundTripper {
	if u == nil {
		return nil
	}
	base, ok := http.DefaultTransport.(*http.Transport)
	if !ok {
		// 理论不可达（DefaultTransport 即 *Transport）；退化为默认 Transport 规则
		return nil
	}
	tr := base.Clone()
	tr.Proxy = http.ProxyURL(u)
	return tr
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
			return nil, fmt.Errorf("http do: %w", redactURLError(err))
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

// sensitiveQueryKeys URL 查询串中的敏感参数名（错误信息需脱敏）。
var sensitiveQueryKeys = map[string]bool{
	"api_key": true, "apikey": true, "api-key": true,
	"key": true, "token": true, "access_token": true,
}

// redactURLError 移除网络错误中 URL 的凭据与敏感查询参数。
// net/http 的 *url.Error 会携带完整 URL；若查询串含 api_key，错误信息会把密钥
// 带进 search 的 JSON 输出与日志，故在向上返回前统一脱敏。
func redactURLError(err error) error {
	var uerr *url.Error
	if !errors.As(err, &uerr) {
		return err
	}
	redacted := uerr.URL
	if u, perr := url.Parse(uerr.URL); perr == nil {
		u.User = nil
		q := u.Query()
		for k := range q {
			if sensitiveQueryKeys[strings.ToLower(k)] {
				q.Set(k, "REDACTED")
			}
		}
		u.RawQuery = q.Encode()
		redacted = u.String()
	}
	return &url.Error{Op: uerr.Op, URL: redacted, Err: uerr.Err}
}

// sleepForRetry 等待退避时间。返回 false 表示 ctx 已取消。
func (c *Client) sleepForRetry(ctx context.Context, resp *http.Response, attempt int) bool {
	timer := time.NewTimer(c.retryWait(resp, attempt))
	defer timer.Stop()
	select {
	case <-timer.C:
		return true
	case <-ctx.Done():
		return false
	}
}

// maxRetryAfter Retry-After 等待上限：防止异常响应让命令挂起数小时。
const maxRetryAfter = 30 * time.Second

// retryWait 计算本次重试等待时长（优先 Retry-After，受 maxRetryAfter 约束）。
func (c *Client) retryWait(resp *http.Response, attempt int) time.Duration {
	wait := c.backoffDuration(attempt)
	if ra := resp.Header.Get("Retry-After"); ra != "" {
		if secs, perr := strconv.Atoi(ra); perr == nil && secs > 0 {
			wait = time.Duration(secs) * time.Second
			if wait > maxRetryAfter {
				wait = maxRetryAfter
			}
		}
	}
	return wait
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
	return ReadAllLimit(resp, 0)
}

// ReadAllLimit 读取并关闭响应体，最多 limit 字节（0=不限）；超限返回错误而非截断。
// 用于网络边界防御超大/恶意响应耗尽内存。
func ReadAllLimit(resp *http.Response, limit int64) ([]byte, error) {
	defer func() { _ = resp.Body.Close() }()
	if limit <= 0 {
		return io.ReadAll(resp.Body)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > limit {
		return nil, fmt.Errorf("响应体超过上限 %d 字节", limit)
	}
	return data, nil
}
