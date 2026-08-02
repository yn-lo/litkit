// Package model 定义 litkit 的核心数据载体（叶子层）。
//
// 本包是项目最底层依赖，不 import 任何上层包（C5 数据模型纯净，C6）。
// 任何对 Paper/SearchResult 的字段重构都会波及全项目，因此字段稳定是首要约束。
//
// 数据模型规格见 .harness/specs/reference/data-model.md。
package model
