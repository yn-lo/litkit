package httpclient

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
)

// newTestProxy 启动一个仅支持普通 HTTP 转发的测试代理，返回代理服务器与命中标记。
func newTestProxy(t *testing.T) (*httptest.Server, *atomic.Bool) {
	t.Helper()
	var hit atomic.Bool
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hit.Store(true)
		// 普通 HTTP 经代理转发时请求行为绝对 URI（r.URL.Host 非空）
		if r.URL.Host == "" {
			http.Error(w, "expected absolute-uri request", http.StatusBadRequest)
			return
		}
		resp, err := http.DefaultTransport.RoundTrip(r)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}
		defer func() { _ = resp.Body.Close() }()
		for k, vv := range resp.Header {
			for _, v := range vv {
				w.Header().Add(k, v)
			}
		}
		w.WriteHeader(resp.StatusCode)
		_, _ = io.Copy(w, resp.Body)
	}))
	t.Cleanup(proxy.Close)
	return proxy, &hit
}

func TestNew_withProxyRequestGoesThroughProxy(t *testing.T) {
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("ok"))
	}))
	defer target.Close()
	proxy, hit := newTestProxy(t)
	proxyURL, err := ParseProxyURL(proxy.URL)
	if err != nil {
		t.Fatalf("ParseProxyURL: %v", err)
	}

	c := New(Options{TimeoutMS: 2000, Proxy: proxyURL})
	req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, target.URL+"/x", nil)
	resp, err := c.Do(context.Background(), req)
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	body, err := ReadAll(resp)
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	if string(body) != "ok" {
		t.Errorf("应取回目标响应 ok，got %q", body)
	}
	if !hit.Load() {
		t.Errorf("请求应经代理转发")
	}
}

func TestNew_withoutProxyBypassesProxy(t *testing.T) {
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("direct"))
	}))
	defer target.Close()
	_, hit := newTestProxy(t)

	c := New(Options{TimeoutMS: 2000})
	req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, target.URL, nil)
	resp, err := c.Do(context.Background(), req)
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	if _, err = ReadAll(resp); err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	if hit.Load() {
		t.Errorf("未配置代理时请求不应经过代理")
	}
}

func TestParseProxyURL(t *testing.T) {
	cases := []struct {
		raw    string
		want   string // 期望解析结果；空串表示期望 nil
		hasErr bool
	}{
		{raw: "", want: ""},
		{raw: "http://127.0.0.1:7890", want: "http://127.0.0.1:7890"},
		{raw: "https://user:pass@proxy.example.com:8443", want: "https://user:pass@proxy.example.com:8443"},
		{raw: "socks5://127.0.0.1:1080", want: "socks5://127.0.0.1:1080"},
		{raw: "socks5h://127.0.0.1:1080", want: "socks5h://127.0.0.1:1080"},
		{raw: "ftp://proxy.example.com", hasErr: true}, // 不支持的协议
		{raw: "proxy.example.com:7890", hasErr: true},  // 缺协议（scheme 为空）
		{raw: "://bad", hasErr: true},                  // 非法 URL
	}
	for _, tc := range cases {
		u, err := ParseProxyURL(tc.raw)
		if tc.hasErr {
			if err == nil {
				t.Errorf("ParseProxyURL(%q) 应报错", tc.raw)
			}
			continue
		}
		if err != nil {
			t.Errorf("ParseProxyURL(%q): %v", tc.raw, err)
			continue
		}
		if tc.want == "" {
			if u != nil {
				t.Errorf("ParseProxyURL(%q) 应返回 nil，got %v", tc.raw, u)
			}
			continue
		}
		if u == nil || u.String() != tc.want {
			t.Errorf("ParseProxyURL(%q) = %v, want %s", tc.raw, u, tc.want)
		}
	}
}
