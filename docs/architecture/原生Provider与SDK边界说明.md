# Native Provider SDK Boundary

## Goal

Keep OpenAI, Anthropic, and Gemini native providers on official SDK-backed transport paths while preserving AgentFlow's stable public contracts:

- `llm.Provider`
- `llm.MultiModalProvider`
- `llm.EmbeddingProvider`
- `llm/providers/vendor.NewChatProviderFromConfig(...)`

The SDK owns client construction, request sending, retries, and low-level request/response transport details.
AgentFlow owns cross-provider normalization, public request/response contracts, and integration with Agent / Workflow / API layers.

## Stable Boundary

Official SDK usage is required inside:

- `llm/providers/openai`
- `llm/providers/anthropic`
- `llm/providers/gemini`
- shared native client factories under `llm/internal/*official` or provider-specific internal SDK helpers

SDK types must not leak upward into:

- `agent/`
- `workflow/`
- `api/`
- `cmd/`
- shared public `types/`

## Allowed Local Adapters

These are still allowed inside provider implementations:

- request-field mapping from AgentFlow contracts to SDK params
- response normalization from SDK/native payloads into `llm.*` response structs
- event normalization for streaming/tool/handoff semantics
- small endpoint/path helpers used only for diagnostics or unsupported SDK gaps
- compatibility shims for provider-specific edge cases such as fallback retries or `[DONE]` handling

## Forbidden Patterns

Do not add new parallel native HTTP transport paths for OpenAI / Anthropic / Gemini when the official SDK already supports the operation.

Examples of forbidden regressions:

- new `http.NewRequest(...)` + `client.Do(...)` native chat path in `llm/providers/openai`
- new manual `/v1/messages` request sender in `llm/providers/anthropic`
- new hand-written Gemini `generateContent` / `embedContent` transport path where `genai` already supports it

If the official SDK does not support an operation yet, keep the manual fallback narrowly scoped to that operation and document the reason in code.

## Current Status

Transport baseline after the current migration wave:

- OpenAI native chat/models/responses/multimodal primary paths use the official OpenAI Go SDK
- Anthropic native chat/models/messages primary paths use the official Anthropic Go SDK
- Gemini native chat/streaming/embedding/image/video primary paths use `google.golang.org/genai`
- `llm/providers/openaicompat` remains intentionally HTTP-based for non-native OpenAI-compatible vendors

OpenAI-specific rule of thumb:

- Responses completion, typed streaming, and native input-token counting should default to `client.Responses.*`
- request construction may adapt AgentFlow contracts into SDK params, but new work must not add a second manual `/v1/responses` transport path beside the SDK-backed one
- if a provider-local fallback is temporarily required for an SDK gap, keep it scoped to that one operation and document the gap inline

## Migration Rule For New Work

When adding or changing a native OpenAI / Anthropic / Gemini capability:

1. Check whether the official SDK already exposes the operation.
2. If yes, add the capability through the provider's SDK-backed client path.
3. If not, isolate the manual HTTP fallback to the smallest possible provider-local helper.
4. Add or update regression tests proving the SDK-backed path remains the default path.

## Tool / Agent Features

Tool use, handoff semantics, and AgentFlow stream events stay above the SDK boundary.

That means:

- official SDKs may carry raw tool/function-call payloads
- AgentFlow still maps those payloads into `types.ToolCall`, `types.ToolResult`, `RuntimeStreamEvent`, and handoff semantics
- migration should replace transport first, not remove AgentFlow's cross-provider semantic layer

Tool payload mapping details are documented in [Provider工具负载映射说明.md](./Provider工具负载映射说明.md).

## Token Counting

Native chat providers should also expose SDK-backed token counting through `llm.TokenCountProvider`.

Gateway chat budget admission depends on that native token counting path and must not fall back to tokenizer estimation.

See [Provider原生Token计数说明.md](./Provider原生Token计数说明.md).
