# ChannelRoutedProvider 外部接入模板

这份文档面向“业务项目已经有自己的 channels / keys / mappings 基础设施”的场景。

目标不是让外部项目重写一套 handler runtime，而是只实现业务相关 adapters，然后直接复用 agentflow 的：

- `llm/runtime/router.BuildChannelRoutedProvider(...)`
- `llm/runtime/compose.Build(...)`
- `llm/gateway`

## 1. 什么时候适合用这条链

适合：

- 你的项目已经有自己的 `channel / key / model mapping` 存储和路由规则
- 你希望 `Handler/Service` 继续只面向 `Gateway`
- 你不想把 AES、quota、cooldown、usage 回写逻辑硬塞进 agentflow core

不适合：

- 你只是想继续使用框架内置的 DB-backed `provider + api_key pool`
- 你还没有自己的渠道模型，暂时只需要 `MultiProviderRouter`

单链路原则：

- legacy 内置链路：`Handler/Service -> Gateway -> RoutedChatProvider -> MultiProviderRouter -> provider API`
- channel-based 推荐链路：`Handler/Service -> Gateway -> ChannelRoutedProvider -> resolvers/selectors -> provider factory -> provider API`
- 一次请求只选一条 routed-provider 链，不要把两条链串起来

## 2. 外部项目最少要实现哪些 adapters

最小必需：

- `ModelMappingResolver`
  - 给定 public model，返回可选 mapping 候选
- `ChannelSelector`
  - 根据 priority / weight / region / 失败排除等规则，选出最终 `channel + key`
- `SecretResolver`
  - 负责密钥解密或密钥材料获取
- `ProviderConfigSource`
  - 把最终 route 转成 `provider / baseURL / remote model`

建议实现：

- `UsageRecorder`
  - 回写 usage、失败原因、latency、provider/baseURL/remoteModel
- `CooldownController`
  - 做 key cooldown 或 channel cascade cooldown
- `QuotaPolicy`
  - 做 daily limit / concurrency limit / runtime availability 检查

可直接复用默认实现：

- `ModelResolver`
  - 如果 public model 不需要预归一化，直接用 `router.PassthroughModelResolver{}`
- `UsageRecorder`
  - 如果第一阶段先不落 usage 存储，可先用 `router.NoopUsageRecorder{}`
- `CooldownController`
  - 如果第一阶段先不做 cooldown，可先用 `router.NoopCooldownController{}`
- `QuotaPolicy`
  - 如果第一阶段先不做 quota，可先用 `router.NoopQuotaPolicy{}`

## 3. Adapter-Only 最小接入代码

下面的示例强调两件事：

- 外部项目只实现业务相关 adapters
- resilience/cache/policy/tool-provider 继续复用 agentflow 公共装配

```go
package app

import (
    "context"
    "time"

    llmgateway "github.com/BaSui01/agentflow/llm/gateway"
    llmcompose "github.com/BaSui01/agentflow/llm/runtime/compose"
    llmrouter "github.com/BaSui01/agentflow/llm/runtime/router"
    "go.uber.org/zap"
)

func buildGateway(cfg AppConfig, logger *zap.Logger) (*llmgateway.Service, error) {
    store := mychannel.NewStore(cfg.ChannelStore)
    crypto := mychannel.NewCrypto(cfg.Crypto)

    mainProvider, err := llmrouter.BuildChannelRoutedProvider(llmrouter.ChannelRoutedProviderConfig{
        Name:                 "acme-channel-router",
        ModelResolver:        llmrouter.PassthroughModelResolver{},
        ModelMappingResolver: mychannel.ModelMappingResolver{Store: store},
        ChannelSelector:      &mychannel.PriorityWeightedSelector{Store: store},
        SecretResolver:       mychannel.SecretResolver{Store: store, Crypto: crypto},
        ProviderConfigSource: mychannel.ProviderConfigSource{Store: store},
        UsageRecorder:        mychannel.UsageRecorder{Store: store},
        CooldownController:   mychannel.CooldownController{Store: store},
        QuotaPolicy:          mychannel.QuotaPolicy{Store: store},
        RetryPolicy: llmrouter.ChannelRouteRetryPolicy{
            MaxAttempts:          2,
            ExcludeFailedChannel: true,
        },
        Callbacks: llmrouter.ChannelRouteCallbacks{
            OnKeySelected: func(ctx context.Context, selection *llmrouter.ChannelSelection) {
                logger.Debug("channel key selected",
                    zap.String("channel_id", selection.ChannelID),
                    zap.String("key_id", selection.KeyID))
            },
            OnModelRemapped: func(ctx context.Context, event *llmrouter.ModelRemapEvent) {
                logger.Debug("channel model remapped",
                    zap.String("requested_model", event.RequestedModel),
                    zap.String("remote_model", event.RemoteModel))
            },
        },
        Logger: logger,
    })
    if err != nil {
        return nil, err
    }

    runtime, err := llmcompose.Build(llmcompose.Config{
        Timeout:    cfg.LLM.Timeout,
        MaxRetries: cfg.LLM.MaxRetries,
        Budget: llmcompose.BudgetConfig{
            Enabled:            cfg.Budget.Enabled,
            MaxTokensPerMinute: cfg.Budget.MaxTokensPerMinute,
            MaxTokensPerDay:    cfg.Budget.MaxTokensPerDay,
            MaxCostPerDay:      cfg.Budget.MaxCostPerDay,
            AlertThreshold:     cfg.Budget.AlertThreshold,
        },
        Cache: llmcompose.CacheConfig{
            Enabled:      cfg.Cache.Enabled,
            LocalMaxSize: cfg.Cache.LocalMaxSize,
            LocalTTL:     cfg.Cache.LocalTTL,
            EnableRedis:  cfg.Cache.EnableRedis,
            RedisTTL:     cfg.Cache.RedisTTL,
            KeyStrategy:  cfg.Cache.KeyStrategy,
        },
        Tool: llmcompose.ToolProviderConfig{
            Provider:        cfg.LLM.ToolProvider,
            DefaultProvider: cfg.LLM.DefaultProvider,
            APIKey:          cfg.LLM.ToolAPIKey,
            DefaultAPIKey:   cfg.LLM.APIKey,
            BaseURL:         cfg.LLM.ToolBaseURL,
            DefaultBaseURL:  cfg.LLM.BaseURL,
            Timeout:         cfg.LLM.ToolTimeout,
            MaxRetries:      cfg.LLM.ToolMaxRetries,
        },
    }, mainProvider, logger)
    if err != nil {
        return nil, err
    }

    return llmgateway.New(llmgateway.Config{
        ChatProvider:  runtime.Provider,
        PolicyManager: runtime.PolicyManager,
        Ledger:        runtime.Ledger,
        Logger:        logger,
    }), nil
}

type AppConfig struct {
    LLM struct {
        RoutedProvider string
        DefaultProvider string
        ToolProvider    string
        APIKey          string
        ToolAPIKey      string
        BaseURL         string
        ToolBaseURL     string
        Timeout         time.Duration
        ToolTimeout     time.Duration
        MaxRetries      int
        ToolMaxRetries  int
    }
    Cache struct {
        Enabled      bool
        LocalMaxSize int
        LocalTTL     time.Duration
        EnableRedis  bool
        RedisTTL     time.Duration
        KeyStrategy  string
    }
    Budget struct {
        Enabled            bool
        MaxTokensPerMinute int
        MaxTokensPerDay    int
        MaxCostPerDay      float64
        AlertThreshold     float64
    }
    ChannelStore any
    Crypto       any
}
```

这条装配意味着：

- 你的业务逻辑只需要关心 route planning adapters
- handler-facing runtime 的 resilience/cache/policy/tool-provider 不用再重复造一套
- `Gateway` 之上的 Handler/Service 调用面保持不变

## 4. 如果不想从零实现全部 adapters

仓库内已经提供了一个通用起点：

- `llm/runtime/router/extensions/channelstore`

它不要求固定数据库表名，也不要求固定 ORM。你可以：

- 先用 `channelstore.StaticStore` 跑通概念验证
- 再把 `ModelMappingSource / ChannelSource / KeySource / SecretSource` 换成自己的数据库或服务调用

最小示意：

```go
store := channelstore.NewStaticStore(channelstore.StaticStoreConfig{
    Channels: channels,
    Keys:     keys,
    Mappings: mappings,
    Secrets:  secrets,
})

provider, err := llmrouter.BuildChannelRoutedProvider(llmrouter.ChannelRoutedProviderConfig{
    ModelResolver:        llmrouter.PassthroughModelResolver{},
    ModelMappingResolver: channelstore.StoreModelMappingResolver{Source: store},
    ChannelSelector:      &channelstore.PriorityWeightedSelector{Source: store},
    SecretResolver:       channelstore.StoreSecretResolver{Source: store},
    ProviderConfigSource: channelstore.StoreProviderConfigSource{Channels: store},
})
```

这适合：

- 概念验证
- 集成测试
- 先把文本链路跑通，再逐步替换成真实业务 adapter

## 5. 推荐的配置切换方式

当前阶段，agentflow 的公共核心已经提供了：

- `BuildChannelRoutedProvider(...)`
- `llm/runtime/compose.Build(...)`
- `llm/runtime/compose.RegisterMainProviderBuilder(...)`
- 内置配置字段 `llm.main_provider_mode`

也就是说，外部项目现在可以直接复用 agentflow 的内置 server 切换位：

```yaml
llm:
  main_provider_mode: channel_routed # legacy | channel_routed
```

然后在自己的组合根里注册 channel builder：

```go
import (
    "github.com/BaSui01/agentflow/config"
    llmcompose "github.com/BaSui01/agentflow/llm/runtime/compose"
    "github.com/BaSui01/agentflow/llm/runtime/router/extensions/channelstore"
)

func registerMainProviderBuilders(store channelstore.Store) error {
    return llmcompose.RegisterMainProviderBuilder(
        config.LLMMainProviderModeChannelRouted,
        channelstore.NewMainProviderBuilder(channelstore.MainProviderBuilderOptions{
            Name:  "acme-channel-router",
            Store: store,
        }),
    )
}
```

如果你不想复用 agentflow 的内置 `config.Config`，也可以继续使用自己的应用级配置，然后在应用代码里映射到：

- `cfg.LLM.MainProviderMode`
- `llmcompose.RegisterMainProviderBuilder(...)`

这样做的好处是：

- 外部项目可以直接接入内置 server 启动链，而不是继续包一层本地 engine
- channel 具体语义仍由外部项目注册的 builder 和 adapters 决定，不会把业务数据库模型反向绑进 core
- `legacy` 与 `channel_routed` 仍然是单链路切换，不会把两条 routed-provider 链串起来

这个写法的意义是：

- 开关只决定 `Gateway` 后面接哪条 routed-provider 链
- `Gateway`、Handler、Service、tool-provider、cache、budget 等上层装配保持稳定
- 回滚时只把开关切回 legacy，不需要整条请求链重写

## 6. 为什么当前阶段仍只覆盖文本链路

当前 `ChannelRoutedProvider` 实现的是 `llm.Provider`，第一阶段只覆盖：

- `Completion`
- `Stream`

不直接接 image/video 的原因不是“能力还没做”，而是故意保持边界清晰：

- 文本 chat 走 `llm.Provider`
- image/video 当前走 `llm/gateway + llm/capabilities/* + llm/providers/vendor.Profile`

如果现在强行把 image/video 也塞进 `ChannelRoutedProvider`：

- 会把文本 routed provider 和多模态 capability dispatch 提前耦合
- 会让 `llm.Provider` 继续膨胀
- 会模糊后续 `gateway + capabilities` 的演进边界

因此当前阶段的正确口径是：

- 先把文本 `Completion / Stream` 的 channel-based routing 跑通
- 让 remote model remap / dynamic baseURL / key selection / usage recording / retry exclusion 先在文本链里闭环
- 后续再把同一套 route planning 抽象复用到 image/video，但承接点仍然是 `gateway + capabilities + vendor.Profile`
