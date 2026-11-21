package main

import (
	"errors"
	"slices"
	"testing"

	"litkit/internal/config"
	"litkit/internal/model"
)

func TestResolveSearchSources(t *testing.T) {
	registered := []string{"arxiv", "openalex", "pubmed"}
	cases := []struct {
		name       string
		explicit   string
		cfg        *config.Config
		want       []string
		wantWarnNo int
	}{
		{
			name:     "显式 -s 完全覆盖 env",
			explicit: "pubmed",
			cfg: &config.Config{
				SearchSources:        []string{"arxiv"},
				SearchExcludeSources: []string{"pubmed"},
			},
			want: []string{"pubmed"},
		},
		{
			name:     "显式 -s trim/去空/去重",
			explicit: " pubmed ,,pubmed,openalex",
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
		got, warns := resolveSearchSources(tc.explicit, tc.cfg, registered, registered)
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

func TestExitCodeOf(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want int
	}{
		{"成功", nil, 0},
		{"参数错误", &paramError{msg: "bad"}, 2},
		{"部分成功", &exitError{code: exitCodePartialFailure}, 3},
		{"验证未通过", &exitError{code: 1}, 1},
		{"运行错误", errors.New("boom"), 1},
	}
	for _, tc := range cases {
		if got := exitCodeOf(tc.err); got != tc.want {
			t.Errorf("%s: exitCodeOf = %d, want %d", tc.name, got, tc.want)
		}
	}
}

func TestValidatePaging(t *testing.T) {
	if err := validatePaging(0, 0); err != nil {
		t.Fatalf("合法分页参数不应报错: %v", err)
	}
	for _, tc := range []struct{ limit, offset int }{{-1, 0}, {0, -1}} {
		err := validatePaging(tc.limit, tc.offset)
		var pe *paramError
		if !errors.As(err, &pe) {
			t.Errorf("limit=%d offset=%d 应返回 paramError，got %v", tc.limit, tc.offset, err)
		}
	}
}

// TestSourceErrors_ExcludesStorage：本地入库错误不算检索源失败，不影响退出码判定。
func TestSourceErrors_ExcludesStorage(t *testing.T) {
	in := []model.SourceError{
		{Source: "pubmed", Error: "HTTP 500"},
		{Source: "storage", Error: "2 篇入库失败"},
	}
	got := sourceErrors(in)
	if len(got) != 1 || got[0].Source != "pubmed" {
		t.Fatalf("应仅保留检索源错误，got %+v", got)
	}
}

func TestSearchOutcomeError(t *testing.T) {
	// 部分源失败且有结果 → 退出码 3
	partial := model.NewSearchResult()
	partial.Total = 1
	partial.SourceResults["openalex"] = nil
	partial.Errors = []model.SourceError{{Source: "pubmed", Error: "HTTP 500"}}
	var ee *exitError
	if err := searchOutcomeError(partial); !errors.As(err, &ee) || ee.code != exitCodePartialFailure {
		t.Errorf("部分成功应映射退出码 3，got %v", err)
	}
	// 源成功但零结果 + 另一源失败 → 仍属部分成功（退出码 3）
	zeroResult := model.NewSearchResult()
	zeroResult.SourceResults["openalex"] = nil
	zeroResult.Errors = []model.SourceError{{Source: "pubmed", Error: "HTTP 500"}}
	if err := searchOutcomeError(zeroResult); !errors.As(err, &ee) || ee.code != exitCodePartialFailure {
		t.Errorf("有源成功（零结果）应映射退出码 3，got %v", err)
	}
	// 仅 storage 错误 → 不算源失败
	storeOnly := model.NewSearchResult()
	storeOnly.Total = 1
	storeOnly.Errors = []model.SourceError{{Source: "storage", Error: "入库失败"}}
	if err := searchOutcomeError(storeOnly); err != nil {
		t.Errorf("storage 错误不应导致非零退出码，got %v", err)
	}
	// 全部源失败 → 普通错误（退出码 1）
	all := model.NewSearchResult()
	all.Errors = []model.SourceError{{Source: "pubmed", Error: "HTTP 500"}}
	if err := searchOutcomeError(all); err == nil {
		t.Error("全部源失败应返回错误")
	}
}
