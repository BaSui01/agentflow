// Deprecated: 此文件中的缓存实现已迁移至 llm/cache/ 子包。
//
// 请使用以下导入路径：
//
//	import "github.com/BaSui01/agentflow/llm/cache"
//
// 该子包提供更完整的实现，包括：
//   - cache.MultiLevelCache — 多级缓存（本地 LRU + Redis），支持策略模式的缓存键生成
//   - cache.LRUCache — 本地 LRU 缓存，支持 Delete/Clear/Stats
//   - cache.CacheConfig — 缓存配置，支持 KeyStrategyType 和 CacheableCheck
//   - cache.CacheEntry — 缓存条目，支持 PromptVersion/ModelVersion
//   - cache.ErrCacheMiss — 缓存未命中错误
//   - cache.ToolResultCache — 工具调用结果缓存
//   - cache.KeyStrategy — 缓存键生成策略接口（hash/hierarchical）
//
// 由于 llm/cache/ 子包 import 了 llm 包（用于 ChatRequest 等类型），
// 此文件不能通过类型别名转发（会导致循环依赖）。
// 所有新代码应直接使用 llm/cache/ 子包。
package llm
