// Package csl 基于 citeproc-js 渲染 CSL 引用样式。
//
// 正式引用样式（GB/T 7714—2025 / APA / IEEE / 用户 .csl）统一由 CSL 引擎渲染；
// 内置样式与 locale 经 go:embed 随二进制分发，用户样式放在 WORK_DIR/styles。
package csl
