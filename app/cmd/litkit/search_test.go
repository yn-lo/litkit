package main

import (
	"testing"

	"litkit/internal/model"
)

func TestHasNetworkError(t *testing.T) {
	cases := []struct {
		name string
		errs []model.SourceError
		want bool
	}{
		{"空错误", nil, false},
		{"非网络错误", []model.SourceError{{Source: "pubmed", Error: "HTTP 500"}}, false},
		{"TLS 超时", []model.SourceError{{Source: "pubmed", Error: `Get "https://x": net/http: TLS handshake timeout`}}, true},
		{"连接拒绝", []model.SourceError{{Source: "arxiv", Error: `dial tcp: connection refused`}}, true},
		{"大小写不敏感", []model.SourceError{{Source: "s2", Error: "Connection Reset by peer"}}, true},
	}
	for _, tc := range cases {
		if got := hasNetworkError(tc.errs); got != tc.want {
			t.Errorf("%s: hasNetworkError = %v, want %v", tc.name, got, tc.want)
		}
	}
}
