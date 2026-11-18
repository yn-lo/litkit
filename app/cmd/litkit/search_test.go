package main

import (
	"slices"
	"testing"

	"litkit/internal/config"
	"litkit/internal/model"
)

func TestResolveSearchSources(t *testing.T) {
	registered := []string{"arxiv", "openalex", "pubmed"}
	cases := []struct {
		name       string
		explicit   []string
		cfg        *config.Config
		want       []string
		wantWarnNo int
	}{
		{
			name:     "显式 -s 完全覆盖 env",
			explicit: []string{"pubmed"},
			cfg: &config.Config{
				SearchSources:        []string{"arxiv"},
				SearchExcludeSources: []string{"pubmed"},
			},
			want: []string{"pubmed"},
		},
		{
			name:     "显式 -s trim/去空/去重",
			explicit: []string{" pubmed ", "", "pubmed", "openalex"},
			cfg:      &config.Config{SearchSources: []string{"arxiv"}},
			want:     []string{"pubmed", "openalex"},
		},
		{
			name: "白名单减排除",
			cfg: &config.Config{
				SearchSources:        []string{"pubmed", "openalex"},
				SearchExcludeSources: []string{"openalex"},
			},
			want: []string{"pubmed"},
		},
		{
			name: "无白名单=全部减排除",
			cfg:  &config.Config{SearchExcludeSources: []string{"arxiv"}},
			want: []string{"openalex", "pubmed"},
		},
		{
			name: "未知源告警并忽略",
			cfg: &config.Config{
				SearchSources:        []string{"pubmed", "nope"},
				SearchExcludeSources: []string{"ghost"},
			},
			want:       []string{"pubmed"},
			wantWarnNo: 2,
		},
		{
			name:       "白名单全未知返回空",
			cfg:        &config.Config{SearchSources: []string{"nope"}},
			want:       []string{},
			wantWarnNo: 1,
		},
		{
			name: "排除全部返回空",
			cfg:  &config.Config{SearchExcludeSources: []string{"arxiv", "openalex", "pubmed"}},
			want: []string{},
		},
	}
	for _, tc := range cases {
		got, warns := resolveSearchSources(tc.explicit, tc.cfg, registered)
		if !slices.Equal(got, tc.want) {
			t.Errorf("%s: sources = %v, want %v", tc.name, got, tc.want)
		}
		if len(warns) != tc.wantWarnNo {
			t.Errorf("%s: warnings = %v, want %d 条", tc.name, warns, tc.wantWarnNo)
		}
	}
}

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
