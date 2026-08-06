// Package core 实现服务层业务编排（跨源检索、结果入库、元数据反查、引用渲染、手稿流水线、约束验证）。
//
// 分层归属：服务层，可依赖适配层（internal/sources）与叶子层（model/config/util/storage/embedding）。
// 禁止 import 入口层（cmd/litkit）。
package core
