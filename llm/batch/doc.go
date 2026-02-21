// 版权所有 2024 AgentFlow Authors. 版权所有。
// 此源代码的使用由 MIT 许可规范,该许可可以是
// 在LICENSE文件中找到。

/*
包 batch 提供 LLM 请求的批处理与调度能力，通过将多个独立请求
合并为批次统一发送，降低网络开销并提升吞吐量。

# 概述

在高并发场景下，逐条发送 LLM 请求会产生大量网络往返开销。
本包通过 BatchProcessor 将短时间内到达的请求自动聚合为批次，
由后台 Worker 池统一处理，从而显著提升整体吞吐效率。

# 核心接口

  - BatchHandler：批量请求处理回调，接收一组 Request 并返回对应 Response。
  - BatchProcessor：核心批处理器，管理请求队列、Worker 池与批次调度。
  - BatchConfig：配置批大小上限、等待时间、队列容量与 Worker 数量。

# 主要能力

  - 自动聚合：按 MaxBatchSize 或 MaxWaitTime 触发批次提交。
  - 异步提交：Submit 返回 channel，调用方可非阻塞等待结果。
  - 同步提交：SubmitSync 提供阻塞式调用便捷方法。
  - 多 Worker 并行：支持配置多个 Worker 并发消费队列。
  - 运行统计：通过 Stats 获取提交数、完成数、失败数与批次效率。

# 使用方式

	cfg := batch.DefaultBatchConfig()
	bp := batch.NewBatchProcessor(cfg, func(ctx context.Context, reqs []*batch.Request) []*batch.Response {
	    // 调用下游 LLM 批量接口
	    return responses
	})
	defer bp.Close()

	resp, err := bp.SubmitSync(ctx, &batch.Request{ID: "1", Model: "gpt-4"})
*/
package batch
