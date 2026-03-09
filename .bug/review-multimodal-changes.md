# 多模态相关代码变更 Review

> 审查范围：git status 中未跟踪的多模态相关新增/修改（api/handlers/multimodal*、config、bootstrap、llm/capabilities/*、openapi 等）。

## 1. 变更概览

- **API 层**：新增 `multimodal.go` / `multimodal_service.go`，提供 capabilities、references 上传、image/video/plan/chat 等 HTTP 入口；请求校验（size/quality/style/duration/fps）、错误码与 SSE 流式图片对齐现有约定。
- **配置层**：`config/loader.go` 与 `defaults.go` 增加 `MultimodalConfig`（Image/Video 子配置、Reference 存储与默认 provider），校验要求 `reference_store_backend=redis` 且启用时需配置 `redis.addr`。
- **组合根**：`cmd/agentflow/server_handlers_runtime.go` 在 `Multimodal.Enabled` 时调用 `ValidateMultimodalReferenceBackend` → `BuildMultimodalRedisReferenceStore` → `BuildMultimodalRuntime`，再挂载 `MultimodalHandler`；`api/routes` 注册 `/api/v1/multimodal/*`。
- **能力层**：`llm/capabilities/multimodal/provider_builder.go` 统一从配置构造 image/video providers（单入口）；image/video 各 provider 实现与 factory 并存；Gateway 通过 capabilities 接入 Router。

---

## 2. 架构与分层符合性

- **依赖方向**：`api/handlers` → `agent`（PromptEnhancer/PromptOptimizer）、`llm/*`、`types`、`pkg/storage`，符合“适配层只做协议转换、调用领域能力”的约定；未出现 api 依赖 cmd 或反向依赖业务层。
- **入口链路**：多模态走 `cmd → bootstrap.BuildMultimodalRuntime → handlers.NewMultimodalHandlerFromConfig → routes.RegisterMultimodal`，符合“单入口、挂载到现有链路”的规则。
- **能力构造**：image/video 通过 `multimodal.BuildProvidersFromConfig` 单入口构造，再交给 Handler；Gateway 使用 `capabilities.NewEntry(router)` 注入，符合“领域能力通过 Builder/Factory 暴露”的约定。
- **禁止兼容代码**：未发现“为兼容旧逻辑保留双实现或兜底分支”的写法，仅保留当前单轨实现。

---

## 3. 亮点

- **Handler / Service 分离**：`multimodalService` 接口 + `defaultMultimodalService` 将“解析 provider、构建 prompt、调 Gateway”与 HTTP 解耦，便于单测与扩展。
- **引用图安全**：`ValidatePublicReferenceImageURL` 禁止内网/私有 URL，避免 SSRF；reference 存 Redis、带 TTL 与 Cleanup，策略清晰。
- **Redis 安全**：`BuildMultimodalRedisReferenceStore` 对非 loopback 强制 `rediss://`，loopback 明文时打 Warn，与项目安全要求一致。
- **配置与热重载**：Multimodal 相关字段进入 `hotreload.go` 的可热重载表，敏感字段标记与 RequiresRestart 区分合理。
- **测试**：`multimodal_test.go` 覆盖 capabilities、image/video、plan、upload、私有 URL 拒绝、stream；`server_multimodal_test.go` 覆盖 Redis 回环校验与 reference_store_backend 校验。

---

## 4. 问题与建议

### 4.1 必须修复（已修复）

- **Bootstrap 显式注入 Pipeline** ✅  
  `BuildMultimodalRuntime` 现改为先调用 `multimodal.BuildProvidersFromConfig` 得到 `providerSet`，再使用 `pipeline := &multimodal.DefaultPromptPipeline{}` 并传入 `handlers.NewMultimodalHandlerWithProviders(..., pipeline, ...)`，由组合根显式装配 Pipeline。

- **Provider 数量统计与真实注册数一致** ✅  
  已移除按 Key 手动计数逻辑，改为使用 `len(providerSet.ImageProviders)` 与 `len(providerSet.VideoProviders)` 作为 `ImageProviderCount` / `VideoProviderCount`，与运行时实际注册数量一致。

### 4.2 建议改进

- **Veo 与 Gemini 视频的覆盖关系** ✅  
  已在 `provider_builder.go` 中 `if cfg.VeoAPIKey != ""` 前增加注释：“当配置了 VeoAPIKey 时覆盖由 GoogleAPIKey 注册的 veo，优先使用独立 Veo 端点。”

- **Handler 内 defaultChatModel 常量** ✅  
  已改为由配置/组合根注入：`MultimodalHandler` 增加 `defaultChatModel` 字段，`NewMultimodalHandlerWithProviders` 增加 `defaultChatModel` 参数（空时回退为 `gpt-4o-mini`）；配置增加 `Multimodal.DefaultChatModel`，bootstrap 传入 `firstNonEmpty(cfg.Multimodal.DefaultChatModel, cfg.Agent.Model)`，与全局 Agent 默认模型一致。

- **writeProviderError 与 toHTTPStatus 的重复逻辑** ✅  
  已统一：在 `multimodal_service.go` 中新增 `httpStatusAndCodeFrom(err)` 与 `errorCodeToHTTPStatus(code)`，优先使用 `types.Error` 的 Code/HTTPStatus，非 typed 时再按文案推断；`toHTTPStatus`、`errorCodeFrom`、`writeProviderError` 均复用该映射。

- **ReferenceStore 为 nil 时的行为** ✅  
  已在 `NewMultimodalHandlerWithProviders` 的注释中说明“referenceStore 为 nil 时使用内存实现，仅建议在测试或开发环境使用”；并在 `docs/VIDEO_IMAGE_PROVIDERS.md` 新增 6.4 节说明生产必须使用 Redis、内存实现仅限测试/开发且不得用于生产。

### 4.3 可选

- **OpenAPI**：已包含 Multimodal 相关 path 与 schema，建议确认 `MultimodalImageRequest` / `MultimodalVideoRequest` 等与 handler 中 `multimodalImageRequest` / `multimodalVideoRequest` 字段命名、枚举值一致（如 size、quality、style）。
- **image factory 与 provider_builder 的职责**：当前 `llm/capabilities/image/factory.go` 的 `NewProviderFromConfig` 与 `llm/capabilities/multimodal/provider_builder.go` 中按 Key 直接构造各 image provider 并存两套入口。若后续希望“所有通过 config 的 image 构造”都走 factory，可在 provider_builder 中改为调用 image.NewProviderFromConfig（并扩展 FactoryConfig），减少重复。

---

## 5. 小结

- 多模态变更整体符合项目架构与 AGENTS.md/CLAUDE.md：分层清晰、单入口、无兼容双轨、适配层只做协议与调用。
- **已修复**：Bootstrap 显式注入 Pipeline；Provider 数量改为基于实际注册数；Veo 覆盖逻辑已加注释；defaultChatModel 从配置/Agent 注入；writeProviderError/toHTTPStatus 统一为 httpStatusAndCodeFrom；ReferenceStore nil 行为已注释与文档说明。
- 修复后已通过 `go build ./...` 及 `go test ./api/handlers/... ./internal/app/bootstrap/... ./config/...` 验证。
