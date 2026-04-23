# ChannelRoutedProvider External Integration Template

This guide is for projects that already own their own `channel / key / model mapping` infrastructure.

The goal is not to rebuild a handler runtime outside agentflow. The goal is to implement only the business-specific adapters, then reuse:

- `llm/runtime/router.BuildChannelRoutedProvider(...)`
- `llm/runtime/compose.Build(...)`
- `llm/gateway`

## 1. When this chain is the right fit

Use it when:

- your project already has its own `channel / key / model mapping` storage and routing rules
- you want `Handler/Service` to keep talking only to `Gateway`
- you do not want AES, quota, cooldown, or usage write-back logic hardcoded into agentflow core

Do not use it when:

- you still want the built-in DB-backed `provider + api_key pool` path
- you do not yet own a channel model and `MultiProviderRouter` is still sufficient

Single-chain rule:

- legacy built-in path: `Handler/Service -> Gateway -> RoutedChatProvider -> MultiProviderRouter -> provider API`
- recommended channel-based path: `Handler/Service -> Gateway -> ChannelRoutedProvider -> resolvers/selectors -> provider factory -> provider API`
- one request should go through one routed-provider chain only

## 2. The minimum adapters an external project needs

Required:

- `ModelMappingResolver`
  - returns mapping candidates for a public model
- `ChannelSelector`
  - chooses the final `channel + key` using your own priority / weight / region / exclusion rules
- `SecretResolver`
  - resolves decrypted secret material or key credentials
- `ProviderConfigSource`
  - converts the selected route into `provider / baseURL / remote model`

Recommended:

- `UsageRecorder`
  - writes back usage, failure reason, latency, provider/baseURL/remoteModel
- `CooldownController`
  - enforces key cooldown or channel cascade cooldown
- `QuotaPolicy`
  - enforces daily limit / concurrency limit / runtime availability checks

Reusable defaults:

- `ModelResolver`
  - if no public-model normalization is needed, use `router.PassthroughModelResolver{}`
- `UsageRecorder`
  - if phase 1 does not persist usage yet, start with `router.NoopUsageRecorder{}`
- `CooldownController`
  - if phase 1 does not enforce cooldown yet, start with `router.NoopCooldownController{}`
- `QuotaPolicy`
  - if phase 1 does not enforce quota yet, start with `router.NoopQuotaPolicy{}`

## 3. Adapter-only minimal wiring

The example below shows the intended split:

- the external project implements only business-specific adapters
- resilience/cache/policy/tool-provider assembly still comes from shared agentflow runtime composition

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

That split means:

- your business logic only owns route-planning adapters
- handler-facing resilience/cache/policy/tool-provider wiring is reused instead of reimplemented
- the `Gateway` call surface above it stays unchanged

## 4. If you do not want to build every adapter from scratch

The repository already ships a generic starting point:

- `llm/runtime/router/extensions/channelstore`

It does not assume fixed table names or a fixed ORM. You can:

- start with `channelstore.StaticStore` for a proof of concept
- replace `ModelMappingSource / ChannelSource / KeySource / SecretSource` with your own DB or service-backed sources later

Minimal example:

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

This is a good fit for:

- proof-of-concept wiring
- integration tests
- getting the text chain working before you swap in your real business adapters

## 5. Recommended config-switch pattern

At the current stage, agentflow core already provides:

- `BuildChannelRoutedProvider(...)`
- `llm/runtime/compose.Build(...)`
- `channelstore.NewMainProviderBuilder(...)`
- the built-in config field `llm.main_provider_mode`

That means an external project can now reuse agentflow's built-in server-side switch:

```yaml
llm:
  main_provider_mode: channel_routed # legacy | channel_routed
```

Then construct the channel builder in its own composition root:

```go
import (
    "context"
    "github.com/BaSui01/agentflow/config"
    "github.com/BaSui01/agentflow/llm"
    "github.com/BaSui01/agentflow/llm/runtime/router/extensions/channelstore"
    "go.uber.org/zap"
    "gorm.io/gorm"
)

func buildMainProviderBuilder(store channelstore.Store) func(context.Context, *config.Config, *gorm.DB, *zap.Logger) (llm.Provider, error) {
    return channelstore.NewMainProviderBuilder(channelstore.MainProviderBuilderOptions{
        Name:  "acme-channel-router",
        Store: store,
    })
}
```

If you do not want to reuse agentflow's built-in `config.Config`, you can still keep an app-owned config and map it into:

- `cfg.LLM.MainProviderMode`
- your own startup/composition registry

This keeps the boundary clean because:

- the external project can plug directly into the built-in server startup chain instead of maintaining an extra local engine wrapper
- channel semantics still live in the externally registered builder and adapters, not inside agentflow core
- switching between `legacy` and `channel_routed` still means choosing one routed-provider chain, not stacking two

The point of this pattern is:

- the switch only changes the routed-provider chain behind `Gateway`
- `Gateway`, handlers, services, tool-provider wiring, cache, and budget wiring stay stable
- rollback is just a config flip back to legacy

## 6. Why the current phase is still text-only

`ChannelRoutedProvider` currently implements `llm.Provider`, so phase 1 only covers:

- `Completion`
- `Stream`

The reason image/video are not wired here yet is not “missing capability”. It is an explicit boundary decision:

- text chat lives on `llm.Provider`
- image/video already live on `llm/gateway + llm/capabilities/* + llm/providers/vendor.Profile`

If image/video were forced into `ChannelRoutedProvider` right now:

- text routed-provider concerns and multimodal capability dispatch would be coupled too early
- `llm.Provider` would keep growing into a larger umbrella interface
- the future `gateway + capabilities` boundary would become less clear

So the correct current-stage message is:

- get text `Completion / Stream` working first
- close the loop for remote model remap / dynamic baseURL / key selection / usage recording / retry exclusion on the text chain
- later reuse the same route-planning abstractions for image/video, but plug them into `gateway + capabilities + vendor.Profile`, not into a larger `llm.Provider`
