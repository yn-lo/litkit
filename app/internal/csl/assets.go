// assets.go 内嵌 CSL 引擎与样式资源。
//
// 所有资源随二进制分发，用户无需安装运行时。许可证与来源见 assets/NOTICE.md：
//   - citeproc-js：CPAL 1.0 或 AGPL 3.0（双许可）
//   - CSL 样式与 locale：CC BY-SA 3.0

package csl

import _ "embed"

//go:embed assets/citeproc.min.js
var citeprocJS string

//go:embed assets/locales-zh-CN.xml
var localeZhCN string

//go:embed assets/locales-en-US.xml
var localeEnUS string

//go:embed assets/gb7714-2025.csl
var styleGB7714 string

//go:embed assets/apa.csl
var styleAPA string

//go:embed assets/ieee.csl
var styleIEEE string
