package sources

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"litkit/internal/util/httpclient"
	"litkit/internal/util/ratelimit"
)

func TestRound2_RateLimitDeadlineMustReturnError(t *testing.T) {
	l := ratelimit.New(1, 1)
	if !l.Allow(context.Background()) {
		t.Fatal("initial token")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	b := NewBaseSource("probe", httpclient.New(httpclient.Options{}), l)
	req, _ := http.NewRequest(http.MethodGet, "http://127.0.0.1:1", nil)
	resp, err := b.Do(ctx, req)
	if resp == nil && err == nil {
		t.Fatalf("nil response and nil error; ctx.Err()=%v", ctx.Err())
	}
}

func TestRound2_RateLimitSearchMustNotPanic(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`<feed xmlns="http://www.w3.org/2005/Atom"></feed>`))
	}))
	defer srv.Close()
	l := ratelimit.New(1, 1)
	l.Allow(context.Background())
	src := NewArxivSource(httpclient.New(httpclient.Options{}), l)
	src.BaseURL = srv.URL
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("source panicked instead of returning deadline error: %v", r)
		}
	}()
	_, err := src.Search(ctx, "test", SearchOptions{})
	if err == nil {
		t.Fatal("should return rate limit deadline error")
	}
}
