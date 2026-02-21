// 包 discovery 提供多智能体场景下的能力发现与匹配机制。
//
// 该包围绕"谁能做什么"这一核心问题，提供以下能力：
//   - 能力登记：维护 Agent 能力、负载与元信息
//   - 能力匹配：按任务描述、能力标签与策略选择候选 Agent
//   - 能力编排：将多个 Agent 的能力组合成可执行方案
//   - 服务发现：支持本地与网络环境下的 Agent 发现
//
// # 核心组件
//
//   - Registry：负责 Agent 能力注册、更新、删除与健康状态维护
//   - Matcher：按匹配策略筛选最优候选 Agent
//   - Composer：将能力需求映射到可执行的多 Agent 组合
//   - Protocol：提供发现协议抽象（如本地、HTTP、多播等）
//   - Service：对外统一暴露发现、匹配与组合能力
//
// # 核心接口
//
//   - [Registry]：能力注册表接口，管理 Agent 与 Capability 的 CRUD、
//     负载更新、执行记录与事件订阅
//   - [Matcher]：能力匹配接口，提供 Match / MatchOne / Score 方法
//   - [Composer]：能力编排接口，提供 Compose / ResolveDependencies / DetectConflicts
//   - [Protocol]：发现协议接口，提供 Start / Stop / Announce / Discover / Subscribe
//   - [AgentCapabilityProvider]：Agent 能力提供者接口，用于集成层自动注册
//
// # 核心类型
//
//   - [DiscoveryService]：统一门面，整合 Registry、Matcher、Composer、Protocol
//   - [AgentInfo]：Agent 注册信息，包含 A2A Card、状态、能力列表与负载
//   - [CapabilityInfo]：能力详情，包含评分、负载、执行统计与健康状态
//   - [MatchRequest] / [MatchResult]：匹配请求与结果
//   - [CompositionRequest] / [CompositionResult]：编排请求与结果
//   - [DiscoveryEvent] / [DiscoveryEventHandler]：事件模型与处理器
//   - [DiscoveryFilter]：Agent 发现过滤条件
//   - [Conflict] / [ConflictType]：能力冲突描述与类型
//
// # 默认实现
//
//   - [CapabilityRegistry]：基于内存的 Registry 实现，支持健康检查与事件通知
//   - [CapabilityMatcher]：默认 Matcher 实现，支持语义匹配与多策略评分
//   - [CapabilityComposer]：默认 Composer 实现，支持依赖解析与冲突检测
//   - [DiscoveryProtocol]：默认 Protocol 实现，支持本地、HTTP 与多播发现
//   - [HealthChecker]：周期性健康检查器，自动降级不健康 Agent
//
// # 集成层
//
// [AgentDiscoveryIntegration] 提供 Agent 与发现系统之间的桥接：
//   - 自动注册与注销 Agent 能力
//   - 周期性负载上报
//   - 执行结果记录与能力评分回写
//   - 任务匹配与能力编排的便捷方法
//
// # 使用流程
//
// 推荐按以下顺序接入：
//   1. 初始化 Discovery Service
//   2. 注册 Agent 能力（从 Agent Card 或自定义信息）
//   3. 根据任务与能力约束执行匹配
//   4. 对复杂任务执行能力组合
//
// # 匹配策略
//
// 匹配器支持多种策略，例如：
//   - BestMatch：综合评分最高优先
//   - LeastLoaded：优先选择负载更低的 Agent
//   - HighestScore：优先能力匹配得分最高者
//   - RoundRobin：轮询分配候选 Agent
//   - Random：随机选择匹配结果
//
// # 健康与事件
//
// Registry 可配置周期性健康检查，自动将不健康 Agent 降级或剔除。
// 同时 Service 提供事件订阅能力，可监听注册、下线、健康变化等关键事件。
//
// # 与 agent 包协同
//
// discovery 可与 `agent` 执行框架联动，实现：
//   - 执行前的动态能力路由
//   - 执行后的能力评分回写
//   - 负载与可用性驱动的持续调度优化
package discovery
