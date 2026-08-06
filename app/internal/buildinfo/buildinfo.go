// Package buildinfo 携带构建期注入的版本信息。
//
// 发布构建经 goreleaser 的 -X ldflags 覆盖（见 app/.goreleaser.yml）；
// 本地开发（go build / go run）保持 "dev"。
package buildinfo

// Version 版本号。
var Version = "dev"
