package state

import "sync/atomic"

// stats 只保留生成侧计数；传输侧统计由 agentlink 提供。
type Stats struct {
	Generated atomic.Int64 // 已生成事件
	Enqueued  atomic.Int64 // 已成功入队
}
