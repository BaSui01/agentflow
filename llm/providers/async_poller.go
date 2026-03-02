package providers

import (
	"context"
	"time"
)

// PollResult 表示单次轮询检查的结果。
type PollResult[T any] struct {
	Done   bool  // 任务是否完成（成功或失败）
	Result *T    // 成功时的结果
	Err    error // 失败时的错误（设置 Done=true + Err 表示任务失败）
}

// PollFunc 是轮询回调函数，每次 tick 调用一次。
// 返回 PollResult 指示任务状态。
type PollFunc[T any] func(ctx context.Context) PollResult[T]

// PollConfig 配置轮询行为。
type PollConfig struct {
	Interval    time.Duration // 轮询间隔，默认 5s
	MaxAttempts int           // 最大尝试次数，0 表示无限（依赖 ctx 超时）
}

// Poll 执行通用异步轮询，替代各模块重复的 ticker+select 循环。
func Poll[T any](ctx context.Context, cfg PollConfig, check PollFunc[T]) (*T, error) {
	interval := cfg.Interval
	if interval == 0 {
		interval = 5 * time.Second
	}

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	attempts := 0
	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-ticker.C:
			result := check(ctx)
			attempts++
			if result.Done {
				if result.Err != nil {
					return nil, result.Err
				}
				return result.Result, nil
			}
			if cfg.MaxAttempts > 0 && attempts >= cfg.MaxAttempts {
				return nil, context.DeadlineExceeded
			}
		}
	}
}
