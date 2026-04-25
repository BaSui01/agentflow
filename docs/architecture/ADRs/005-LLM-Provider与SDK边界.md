# ADR 005: LLM Provider SDK Boundary

## Status

Superseded in part by ADR 006 and updated by subsequent provider transport changes.

## Context

AgentFlow is a multi-provider framework, not an application tied to a single LLM vendor. The current `llm` layer already provides:

- a unified `llm.Provider` contract for text chat, streaming, health checks, and model discovery
- native provider implementations for OpenAI, Anthropic, and Gemini
- a shared `openaicompat` base for OpenAI-compatible providers
- a config-driven chat provider entry via `llm/providers/vendor.NewChatProviderFromConfig(...)`
- vendor-aligned capability aggregation via `llm/providers/vendor.Profile`

At this stage, the main architectural risk is not "missing SDKs". The risk is fragmenting provider construction and capability assembly across multiple parallel entrypoints.

## Decision

### 1. Baseline Strategy

AgentFlow keeps the `llm` provider baseline on:

- the unified `llm.Provider` abstraction
- config-driven construction through `llm/providers/vendor`
- provider-internal transport boundaries that do not leak SDK types upward

Current runtime baseline:

- `llm/providers/openai` uses the official OpenAI Go SDK as the client construction and request-sending base for OpenAI-native paths
- `llm/providers/anthropic` uses the official Anthropic Go SDK as the client construction and request-sending base for Claude-native paths
- `llm/providers/gemini` uses `google.golang.org/genai` per ADR 006
- `llm/providers/openaicompat` remains a self-managed HTTP base for OpenAI-compatible vendors

Official SDKs are therefore allowed inside the provider boundary, but they do not replace the unified `llm.Provider` contract or the vendor factory entry.

OpenAI-specific execution rule:

- `llm/providers/openai` treats the official OpenAI Go SDK as the default transport for native Responses, streaming, token counting, models, and multimodal endpoints
- provider-local adapters may still normalize AgentFlow semantics (tools, reasoning, stream events), but must not reintroduce a parallel manual HTTP primary path for native OpenAI operations already covered by the SDK

### 2. Standard Chat Construction Entry

For config-driven chat provider construction, the standard entry is:

- `llm/providers/vendor.NewChatProviderFromConfig(...)`
- `llm/runtime/router.VendorChatProviderFactory`

Runtime composition, routing, and startup wiring must reuse that entry instead of introducing parallel chat-provider factories.

### 3. Standard Capability Aggregation Direction

Capability assembly should continue converging on vendor-aligned `Profile` composition:

- OpenAI profile aggregates Chat / Embedding / Image / TTS / STT
- Gemini profile aggregates Chat / Embedding / Image / Video
- Anthropic profile remains the chat-first aggregate until additional native capabilities are needed

The goal is to keep provider credentials, defaults, and capability wiring aligned per vendor instead of scattering them across unrelated factories.

### 4. Official SDK Admission Criteria

An official SDK may be adopted only if all of the following are true:

1. It materially reduces protocol-adaptation code.
2. It covers the required AgentFlow surface for that provider, especially streaming, tools, structured output, base URL overrides, multi-key support, timeouts, and error mapping.
3. SDK-specific types do not leak outside the provider implementation boundary into `agent`, `workflow`, `api`, `cmd`, or shared runtime composition.
4. The migration removes the old implementation; it must not leave dual implementations in place.

### 5. Stable Boundary

The SDK-backed transport boundary must stay inside:

- `llm/providers/openai`
- `llm/providers/anthropic`
- `llm/providers/gemini`

The following contracts must remain stable:

- `llm.Provider`
- `llm/runtime/router`
- `llm/providers/vendor`
- startup and handler composition paths

The shared OpenAI-compatible base remains intentionally outside the official-SDK path.

### 6. Protocol Adapter Boundary

Compatibility HTTP adapters exposed by the server remain:

- `POST /v1/chat/completions`
- `POST /v1/responses`
- `POST /v1/messages`

These inbound protocol adapters must keep routing through the same startup and execution chain:

- `api/routes -> api/handlers -> internal/usecase -> llm/gateway -> routed provider`

Google Gemini Developer API and Vertex AI endpoint paths such as:

- `POST /v1beta/models/{model}:generateContent`
- `POST /v1beta/models/{model}:streamGenerateContent`
- `POST /v1/projects/{project}/locations/{location}/publishers/google/models/{model}:generateContent`

remain provider-outbound protocol paths owned by `llm/providers/gemini` and `llm/providers/vendor`, not new project-level inbound HTTP routes.

## Consequences

### Positive

- preserves one stable provider abstraction across all vendors
- keeps routing and startup composition vendor-agnostic
- reduces protocol drift for OpenAI / Anthropic / Gemini native providers
- keeps vendor-specific capability aggregation easier to evolve and test

### Negative

- AgentFlow still owns response mapping and cross-provider normalization above the SDK boundary
- upstream SDK changes can now affect provider-internal transport behavior and test baselines

## Guardrails

- composition code must keep using `VendorChatProviderFactory` / `vendor.NewChatProviderFromConfig(...)`
- public multi-provider docs must demonstrate the vendor factory path instead of legacy ad-hoc registration
- handler/readme/api docs must keep the distinction between server inbound compatibility routes and provider outbound Gemini / Vertex endpoint paths
- any future SDK proposal must be accompanied by an ADR update or a new ADR
