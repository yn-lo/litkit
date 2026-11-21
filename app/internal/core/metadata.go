// metadata.go 是元数据反查的服务层入口（FR-REF-02）。
//
// 平台端点、请求参数、响应解析与撤稿判定均下沉到适配层 internal/sources；
// 本文件只保留服务层稳定的对外类型，避免入口层依赖适配层实现。
// 说明：本文件注释不构成 package doc（package doc 见各包 doc/主文件）。

package core

import (
	"litkit/internal/sources"
	"litkit/internal/util/httpclient"
)

// MetadataFetcher 按标识符反查论文元数据。
//
// 内嵌适配层实现，方法（Fetch / CheckRetraction）直接提升；
// 依赖注入 httpclient.Client，便于测试 mock。
type MetadataFetcher struct {
	*sources.MetadataFetcher
}

// NewMetadataFetcher 创建反查器。
func NewMetadataFetcher(client *httpclient.Client) *MetadataFetcher {
	return &MetadataFetcher{MetadataFetcher: sources.NewMetadataFetcher(client)}
}

// RetractionInfo 撤稿判定结果（对适配层类型的服务层别名）。
type RetractionInfo = sources.RetractionInfo
