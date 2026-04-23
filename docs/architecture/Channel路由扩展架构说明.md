# Channel Routing Extension Architecture

## 1. 目标

本设计不是把 `agentflow` 改造成某个业务项目专用的 API Key Pool。

目标是把“channel-based routing”上移为 `llm` 层可复用能力：

- core 只定义通用路由抽象与 provider 入口
- channel / key / mapping / AES / quota / cooldown 的具体存储和策略放到外部 extension 或 adapter
- 现有 `MultiProviderRouter` 保留，作为框架内置的 DB-backed provider router
- 新增 `ChannelRoutedProvider`，作为更泛化的 routed provider 入口

## 2. 推荐主链路

对于“业务侧已经有自己的 channels / keys / mappings 基础设施”的场景，推荐收敛到一条主链路：

`Handler/Service -> Gateway -> ChannelRoutedProvider -> resolvers/selectors -> provider factory -> provider API`

职责拆分：

- `Handler/Service`
  - 只负责业务入站、参数准备、调用 usecase
  - 不直接操作 channel store、provider factory、secret 解密
- `Gateway`
  - 作为统一 LLM 入口承接 chat/completion/stream
  - 负责统一请求语义、观测字段和 `ProviderDecision`
- `ChannelRoutedProvider`
  - 把一次 chat 请求转换为 channel route 规划与执行
  - 统一串联 resolvers、selectors、usage/cooldown/quota 回写
- `resolvers/selectors`
  - 负责 model 归一化、mapping 解析、channel/key 选择、secret 解析、runtime policy 判定
  - 由外部 extension / adapter 注入实现，不在 core 硬编码表结构
- `provider factory`
  - 根据最终解析出的 `provider/baseURL/model/secret` 创建底层 provider
- `provider API`
  - 实际调用 OpenAI、Anthropic、Gemini 或任意 OpenAI-compatible upstream

边界约束：

- 对 channel-based routing，推荐的运行时主入口是 `Gateway -> ChannelRoutedProvider`，而不是 `Handler -> MultiProviderRouter`。
- `ChannelRoutedProvider` 不内置 `channels/channel_keys/channel_model_mappings` 表名，也不要求固定 ORM。
- `MultiProviderRouter` 与 `ChannelRoutedProvider` 可以在仓库级别同时存在，但不应在同一条请求链里串联成双重路由。
- 它们的关系不是“`ChannelRoutedProvider` 包一层 `MultiProviderRouter`”，而是 `Gateway` 后两个不同语义的 routed provider 入口：内置 DB-backed provider routing vs. 外部注入的 channel-based routing。

## 3. 当前边界

### 3.1 保留为 framework core

- `llm.Provider`
  - chat/completion/stream 的稳定抽象
- `llm/resilience`
  - retry / circuit breaker / idempotency 与具体路由语义无关
- `llm/gateway`
  - 统一 chat/image/video/audio/... 入口与 `ProviderDecision`
- `llm/runtime/router`
  - routed provider、route policy、provider factory、health monitor 等运行时路由基建
- `agent/runtime`、`agent`、`workflow`
  - 继续只依赖 `llm.Provider` 或 `llm/core.Gateway`
  - 不感知 channel、key、mapping 表结构

### 3.2 放到 platform extension / adapter

- channels / channel_keys / channel_model_mappings 的存储模型
- AES 解密
- DB 表名、ORM、查询索引、region 过滤实现
- mapping 级 priority / weight 覆盖
- cooldown 级联策略
- daily limit / concurrency limit / runtime availability filter
- 调用结果回写到业务侧 usage / quota / audit 表

## 4. 新增抽象

第一阶段在 `llm/runtime/router` 增加以下接口：

- `ChannelSelector`
  - 基于解析后的 model/mapping 候选，选择最终 `channel + key`
- `ModelResolver`
  - 将 public model 归一化为 route planning 使用的 model
- `ModelMappingResolver`
  - 解析 model 对应的 channel mapping 候选
- `SecretResolver`
  - 为最终选中的 route 返回密钥材料
- `UsageRecorder`
  - 记录单次调用的结果、usage、失败信息
- `CooldownController`
  - 在调用前做 cooldown 准入，在调用后写回 cooldown 结果
- `QuotaPolicy`
  - 在调用前做 quota 准入，在调用后写回 quota 使用量
- `ProviderConfigSource`
  - 将最终 route 解析成 provider runtime config，例如 `provider/baseURL/model/extra`
- `ChannelRouteRetryPolicy`
  - 控制失败 key/channel 排除重试

辅助结构：

- `ChannelRouteRequest`
  - 统一路由输入，包含 `Capability`、`Mode`、`RequestedModel`、`ProviderHint`、`RoutePolicy`、`Region`
- `ChannelModelMapping`
  - channel 维度的 model mapping 候选
- `ChannelSelection`
  - 最终选中的 `channel + key`
- `ChannelSecret`
  - 解析后的密钥材料
- `ChannelProviderConfig`
  - 注入 provider factory 的运行时配置
- `ChannelUsageRecord`
  - 一次调用的完整记录

对外装配入口：

- `BuildChannelRoutedProvider(ChannelRoutedProviderConfig)`
  - 外部项目注入 adapters 后，一次性构建单条 channel-routed chat 链
- `llm/runtime/router/extensions/channelstore`
  - 仓库内置的通用 extension 起点
  - 提供 source 契约、`StoreModelMappingResolver`、`PriorityWeightedSelector`、`StoreSecretResolver`、`StoreProviderConfigSource`
  - 提供 `StaticStore` 作为无固定数据库依赖的测试/示例实现

最小装配示意：

```go
chatProvider, err := router.BuildChannelRoutedProvider(router.ChannelRoutedProviderConfig{
    ModelResolver:        myModelResolver,
    ModelMappingResolver: myMappingResolver,
    ChannelSelector:      myChannelSelector,
    SecretResolver:       mySecretResolver,
    UsageRecorder:        myUsageRecorder,
    CooldownController:   myCooldownController,
    QuotaPolicy:          myQuotaPolicy,
    ProviderConfigSource: myProviderConfigSource,
    RetryPolicy: router.ChannelRouteRetryPolicy{
        MaxAttempts:          2,
        ExcludeFailedChannel: true,
    },
})
if err != nil {
    return err
}

gateway := llmgateway.New(llmgateway.Config{
    ChatProvider: chatProvider,
    Logger:       logger,
})
```

如果业务项目暂时不想从零实现全部 adapter，可以直接复用 `llm/runtime/router/extensions/channelstore`：

```go
store := channelstore.NewStaticStore(channelstore.StaticStoreConfig{
    Mappings: mappings,
    Keys:     keys,
    Secrets:  secrets,
})

chatProvider, err := router.BuildChannelRoutedProvider(router.ChannelRoutedProviderConfig{
    ModelMappingResolver: channelstore.StoreModelMappingResolver{Source: store},
    ChannelSelector:      &channelstore.PriorityWeightedSelector{Source: store},
    SecretResolver:       channelstore.StoreSecretResolver{Source: store},
    ProviderConfigSource: channelstore.StoreProviderConfigSource{Channels: store},
})
```

这个装配口径强调两点：

- 业务侧只负责实现 adapters，不需要在 handler/service 中手工串接 route planning 细节。
- `Gateway` 仍然是对上层暴露的统一 LLM 入口，`ChannelRoutedProvider` 只负责 `Gateway` 后面的 routed provider 链。
- 外部项目如果已经自己构造好了 main provider，可以通过 `llm/runtime/compose.Build(...)` 复用同一套 handler-facing runtime 装配，而不是复制 resilience/cache/policy/tool-provider wiring。
- 仓库自身的组合根继续通过 `internal/app/bootstrap.BuildLLMHandlerRuntimeFromProvider(...)` 复用这层公共装配。
- 仓库内置了 `llm.main_provider_mode` 启动切换位，公共注册入口是 `llm/runtime/compose.RegisterMainProviderBuilder(...)`。
- `llm/runtime/router/extensions/channelstore.NewMainProviderBuilder(...)` 可以把外部 channel store 直接适配到这条启动链。
- `llm/runtime/router/extensions/runtimepolicy` 提供 `UsageRecorder`、`CooldownController`、`QuotaPolicy` 的参考实现，适合逐步补齐 usage 回写、cooldown、daily limit、concurrency limit。
- 内置 config hot reload 会沿同一条公共装配 seam 重建文本 runtime，因此 `chat/agent/workflow` 的文本主链可以在不重启进程的情况下切换到新的 main provider；多模态 runtime 仍建议按重启口径处理。
- 当前 hot reload 只会原地更新启动时已经绑定到 `ServeMux` 的 handler；如果某些文本端点在启动时因为没有 main provider 而未注册，后续 reload 出来的 runtime 仍需要重启进程才能暴露这些新路由。
- workflow runtime 在 reload 时会复用同一个 `hitl.InterruptManager`，因此已有 approval/input interrupt 不会因为 parser/runtime 重建而丢失。
- reload 成功完成一次 runtime swap 后才会清理旧 resolver cache；如果新的 text runtime 构建失败，回滚会按恢复配置再走同一条 reload seam，只有当恢复后的 runtime 已经重新接管时才会清理过期 resolver cache。
- 外部项目的最小 adapter-only 接入模板、组合根配置切换示例见 `docs/architecture/Channel路由外部接入模板-中文版.md`。

## 5. 新入口

### 5.1 `ChannelRoutedProvider`

`ChannelRoutedProvider` 实现 `llm.Provider`，第一阶段只承接：

- `Completion`
- `Stream`

推荐执行链路：

1. `Handler/Service` 把请求交给 `Gateway`
2. `Gateway` 把 chat/completion/stream 转交给 `ChannelRoutedProvider`
3. `ChannelRoutedProvider` 从 `ChatRequest` 组装 `ChannelRouteRequest`
4. `ModelResolver`
5. `ModelMappingResolver`
6. `ChannelSelector`
7. `CooldownController.Allow`
8. `QuotaPolicy.Allow`
9. `SecretResolver`
10. `ProviderConfigSource`
11. `ChatProviderFactory`
12. 调用底层 provider API
13. `UsageRecorder`
14. `CooldownController.RecordResult`
15. `QuotaPolicy.RecordUsage`

如果开启 `ChannelRouteRetryPolicy`：

16. 在失败后排除已失败 `key`
17. 必要时同时排除失败 `channel`
18. 重新走同一条 route planning 链，而不是切回旧 router

这条链路的设计目标是把“路由语义”集中在 `ChannelRoutedProvider` 之后，而不是让 handler、service、bootstrap 或业务侧 usecase 直接下探到底层渠道实现。

### 5.2 与现有 `MultiProviderRouter` 的关系

- `MultiProviderRouter`
  - 继续保留
  - 仍然是框架内置的 provider/model/api_key DB router
  - 适合框架默认的 provider catalog + API key pool 场景
- `RoutedChatProvider`
  - 继续保留
  - 仍然是 `MultiProviderRouter` 的 `llm.Provider` 包装
- `ChannelRoutedProvider`
  - 新增
  - 面向 channel-based routing extension
  - 是新的推荐主入口，用于外部项目注入自定义 channels / keys / mappings 体系

推荐实现方式：

- 新项目直接使用 `BuildChannelRoutedProvider(...)`
- 老项目继续保留 `MultiProviderRouter`
- 不要在同一条请求链中把 `MultiProviderRouter` 再包进 `ChannelRoutedProvider`

单链路对照：

| 链路 | 定位 | 说明 |
|------|------|------|
| `Gateway -> RoutedChatProvider -> MultiProviderRouter` | legacy 默认文本链路 | 继续服务框架内置的 DB-backed provider routing |
| `Gateway -> ChannelRoutedProvider` | 推荐的新 channel-based 文本链路 | 面向外部注入的 channels / keys / mappings 体系 |
| `Gateway -> ChannelRoutedProvider -> MultiProviderRouter` | 不推荐 | 会把两套路由语义串成双重路由，不是目标架构 |

推荐口径：

- 新做 channel-based routing 集成时，优先选 `Gateway -> ChannelRoutedProvider`。
- 已经基于 `MultiProviderRouter` 跑通的老部署可继续保持，不要求一次性切换。
- phased migration 的目标是逐步把“业务侧 route 语义”迁移到 `ChannelRoutedProvider` 后面的扩展接口里，而不是把 `MultiProviderRouter` 再包一层冒充 channel router。
- 一次请求只应经过一条 routed provider 主链；不要把 `MultiProviderRouter` 当作 `ChannelRoutedProvider` 的子步骤。

## 6. 为什么第一阶段不直接接 image/video

当前仓库中更干净的多模态扩展面是：

- `llm/gateway`
- `llm/capabilities/*`
- `llm/providers/vendor.Profile`

而不是扩张 `llm.Provider` 本身。

因此第一阶段只让 `ChannelRoutedProvider` 解决 chat/completion/stream，先把：

- remote model remap
- dynamic baseURL injection
- secret resolution
- usage recording

这条主链跑通。

后续 image/video 应该复用同一套 route planning 抽象，但接到 `gateway + capabilities + vendor.Profile`，而不是把 `llm.Provider` 继续做成大而全接口。

换句话说，第一阶段不接 image/video 不是能力缺失，而是刻意保持边界清晰：文本 routed provider 先稳定在 `llm.Provider` 这条链，多模态再沿 capability 入口扩展。

## 7. 分阶段迁移

### 7.1 迁移总原则

- 主链路先收敛，再替换底层实现。
- 优先保持 `Handler/Service -> Gateway` 不变，把迁移焦点放在 `Gateway` 之后。
- 不要求业务项目一次性替换全部渠道实现；先接 `ChannelRoutedProvider`，再逐步迁移 resolver/selector/recorder。
- `MultiProviderRouter` 只作为 legacy path 保留，不作为 channel-based routing 的长期演进目标。

### 7.2 Phase 1：低风险桥接层

目标：不破坏现有 public API，引入 generic channel routing 骨架。

改动：

- `llm/`
  - 新增一次调用级的 resolved provider 回传器
- `llm/runtime/router`
  - 新增 channel routing 抽象接口
  - 新增 `ChannelRoutedProvider`
  - 新增共享 `VendorChatProviderFactory`
- `llm/gateway`
  - 在 chat/completion/stream 中读取 resolved provider/baseURL/model
- 现有 `RoutedChatProvider`
  - 仅补齐 resolved provider 回传，不改原有路由语义

保留：

- `MultiProviderRouter`
- `RoutedChatProvider`
- 现有 bootstrap 装配方式

风险：

- 低
- 新入口默认不接入现有 bootstrap，不影响老用户

回滚方式：

- 不使用 `ChannelRoutedProvider`
- 现有 `RoutedChatProvider` 保持原样可运行

迁移结果：

- 仓库内开始存在两条运行时路径，但推荐主链路已经明确为 `Gateway -> ChannelRoutedProvider`。
- legacy 用户继续走 `Gateway -> RoutedChatProvider -> MultiProviderRouter`，新接入用户可以开始按接口接入 channel 体系。

### 7.3 Phase 2：adapter 落地

目标：让外部项目通过 adapter 接入 channels / keys / mappings。

改动：

- 新增 extension 包，例如 `llm/runtime/router/extensions/channelstore` 或业务侧独立模块
- 实现：
  - `ModelMappingResolver`
  - `ChannelSelector`
  - `SecretResolver`
  - `UsageRecorder`
  - `CooldownController`
  - `QuotaPolicy`
  - `ProviderConfigSource`
- 这些实现优先做成“source + resolver/selector + helper”三层组合，而不是把数据库查询、选择策略、密钥解析写进一个大对象
- 当前仓库已内置 `llm/runtime/router/extensions/channelstore` 作为 phase 2 的通用参考实现

保留：

- 旧 router 与旧 runtime

风险：

- 中
- 主要在存储模型与业务规则对齐

迁移结果：

- 外部项目可以把自己的 channel store、AES、quota、cooldown、usage 记录逻辑注入到推荐主链路中。
- 到这一阶段，`ChannelRoutedProvider` 已经能承载文本 chat/completion/stream 的主要业务语义。

### 7.4 Phase 3：bootstrap 可选接入

目标：允许组合根按配置选择旧 router 或新 channel router。当前仓库已通过内置 `llm.main_provider_mode` 和公共 builder registry 落地这一层。

改动：

- `internal/app/bootstrap`
  - 继续保留“基于任意 main provider 构造 handler runtime”的组合根适配函数
  - 默认通过公共的 `llm/runtime/compose.Build(...)` 装配 middleware/cache/policy/runtime，再由 `BuildLLMHandlerRuntimeFromProvider(...)` 作为 server bootstrap 薄包装
  - 默认仍由 legacy `MultiProviderRouter` 构造 main provider
- `cmd/agentflow`
  - 通过 `llm.main_provider_mode` 选择 routed provider 实现
- `llm/runtime/compose`
  - 通过 `RegisterMainProviderBuilder(...)` / `BuildMainProvider(...)` 暴露公共 main-provider 注册与装配口
- `llm/runtime/router/extensions/channelstore`
  - 提供 `NewMainProviderBuilder(...)`，让外部项目直接注册 channel-based startup builder

保留：

- `MultiProviderRouter` 默认路径

风险：

- 中
- 需要处理配置切换与观测面板兼容

迁移结果：

- 新部署可以直接以 `Gateway -> ChannelRoutedProvider` 作为默认装配。
- 老部署仍可保留 `MultiProviderRouter`，通过配置切换实现可回滚迁移。

### 7.5 Phase 4：多模态扩展

目标：把同一套 route planning 能力扩展到 image/video。

改动：

- `llm/gateway`
- `llm/capabilities/*`
- `llm/providers/vendor.Profile`

风险：

- 高于前几阶段
- 因为 image/video 是 capability provider，而不是单一 `llm.Provider`

迁移结果：

- channel-based routing 语义扩展到多模态，但依然经由 `Gateway + capabilities + vendor.Profile` 承接，不把 `llm.Provider` 扩成单点巨型接口。

## 8. 第一阶段验收点

- 现有 `MultiProviderRouter` / `RoutedChatProvider` 用法不变
- `ChannelRoutedProvider` 不依赖固定数据库表名
- 外部项目可以通过接口注入自己的 channel/key/mapping 系统
- chat completion 与 stream 均可走新 routed provider
- 一次调用内可贯通：
  - remote model
  - baseURL
  - key selection callback
  - usage recording
  - failed key/channel exclusion retry
