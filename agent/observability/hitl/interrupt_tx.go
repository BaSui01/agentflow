package hitl

import "context"

// TxInterruptStore 是 InterruptStore 的可选扩展能力（capability interface）：
// 持久化实现（如 PostgreSQL/MySQL）可以选择实现它来支持事务，让上层把多个 store 操作
// 包在同一个事务里，避免并发场景下"内存状态已变更但持久化失败"造成的不一致。
//
// 设计要点（issue #18）：
//   - 不强制 InterruptStore 都实现事务（InMemoryInterruptStore 不需要事务）
//   - 上层使用 RunInTransaction 而非直接断言，自动 fallback 到无事务路径
//   - 实现方需要保证传给 fn 的 InterruptStore 是 tx-bound（fn 内的 ops 必须在事务内执行）
//   - fn 返回错误时，实现方必须 rollback 事务并把错误透传给调用方
//   - fn 返回 nil 时，实现方提交事务；提交失败时返回 commit 错误
type TxInterruptStore interface {
	InterruptStore
	WithTransaction(ctx context.Context, fn func(tx InterruptStore) error) error
}

// RunInTransaction 在 store 支持事务时把 fn 包在事务内执行；否则直接调用 fn(store)。
// 上层代码使用这个 helper 而非直接做类型断言，让"是否事务"对调用代码透明。
//
// 使用示例：
//
//	err := RunInTransaction(ctx, m.store, func(s InterruptStore) error {
//	    return s.Update(ctx, interrupt)
//	})
func RunInTransaction(ctx context.Context, store InterruptStore, fn func(InterruptStore) error) error {
	if tx, ok := store.(TxInterruptStore); ok {
		return tx.WithTransaction(ctx, fn)
	}
	return fn(store)
}
