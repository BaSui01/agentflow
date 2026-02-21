// 版权所有 2024 AgentFlow Authors. 版权所有。
// 此源代码的使用由 MIT 许可规范,该许可可以是
// 在LICENSE文件中找到。

/*
包 cache 提供 LLM 请求与工具调用的多级缓存实现，通过本地 LRU
与 Redis 协同减少重复调用，降低延迟与成本。

# 概述

相同或相似的 LLM 请求在实际业务中频繁出现。本包提供两类缓存：
Prompt 级多级缓存（本地 LRU + Redis）用于缓存 ChatRequest 响应，
工具结果缓存（ToolResultCache）用于避免重复的工具执行。

# 核心接口

  - PromptCache：Prompt 缓存接口，定义 Get/Set/Delete/GenerateKey 操作。
  - KeyStrategy：缓存键生成策略接口，支持 Hash 与 Hierarchical 两种实现。
  - MultiLevelCache：多级缓存实现，本地 LRU 作为 L1、Redis 作为 L2。
  - ToolResultCache：工具执行结果缓存，支持 TTL、排除列表与按工具失效。
  - CachingToolExecutor：将 ToolExecutor 包裹为带缓存的执行器。

# 主要能力

  - 多级缓存：L1 本地 LRU（O(1) 操作）+ L2 Redis，自动回填。
  - 策略模式：Hash 策略适用于精确匹配，Hierarchical 策略支持多轮对话前缀共享。
  - 工具缓存：按工具名 + 参数 Hash 缓存结果，支持 per-tool TTL 覆盖。
  - 可缓存判断：默认跳过含 Tools 的请求，避免缓存有副作用的调用。
  - 版本失效：支持按 Prompt/Model 版本批量失效缓存。

# 使用方式

	cfg := cache.DefaultCacheConfig()
	mlc := cache.NewMultiLevelCache(redisClient, cfg, logger)
	key := mlc.GenerateKey(chatReq)
	entry, err := mlc.Get(ctx, key)
*/
package cache
