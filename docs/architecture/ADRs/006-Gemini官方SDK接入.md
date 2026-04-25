# ADR 006: Adopt Google GenAI Official SDK for Gemini Runtime

## Status

Accepted

## Context

ADR 005 kept the baseline on self-managed HTTP adapters and required any SDK adoption to:

- stay inside the provider boundary
- keep `llm.Provider` and `vendor` factory contracts stable
- remove the replaced implementation instead of running dual paths

The Gemini surface in AgentFlow had already expanded beyond plain chat into:

- chat completion + streaming
- embeddings
- image generation
- video analysis / generation
- cached content, safety, grounding, and structured output parameters

That left AgentFlow owning a growing amount of low-level Google protocol code and endpoint drift.

## Decision

AgentFlow adopts `google.golang.org/genai` as the runtime transport for Gemini / Google provider implementations, while preserving the existing AgentFlow public contracts:

- `llm.Provider`
- `llm.MultiModalProvider`
- `llm.EmbeddingProvider`
- `llm/providers/vendor.NewChatProviderFromConfig(...)`
- vendor profile composition

The SDK boundary remains inside `llm/`. SDK types must not leak upward into `agent/`, `workflow/`, `api/`, or `cmd/`.

## Scope

This decision applies to Gemini-related runtime calls in:

- `llm/providers/gemini`
- `llm/capabilities/embedding/gemini.go`
- `llm/capabilities/image/gemini.go`
- `llm/capabilities/video/gemini.go`

Shared SDK client construction is centralized in `llm/internal/googlegenai`.

The Gemini / Vertex endpoint paths covered by this ADR are provider-outbound calls, including:

- `POST /v1beta/models/{model}:generateContent`
- `POST /v1beta/models/{model}:streamGenerateContent`
- `POST /v1/projects/{project}/locations/{location}/publishers/google/models/{model}:generateContent`

They are not project-level inbound HTTP routes and must not be re-exposed from `api/routes` or `api/handlers`.

## Consequences

### Positive

- removes Gemini-specific hand-written request plumbing from runtime paths
- aligns AgentFlow with Google-maintained request / response mapping
- keeps existing AgentFlow factory and provider abstraction stable
- reduces protocol drift risk for Gemini, Imagen, and Veo endpoints

### Negative

- introduces a direct dependency on Google’s SDK in the `llm` layer
- some AgentFlow-only request fields still need local type adaptation at the provider boundary

## Guardrails

- do not add parallel Gemini construction paths outside the existing vendor / factory entrypoints
- do not leak `genai` types outside `llm/`
- do not introduce project-level `/v1beta/models/*`, `/v1/projects/*`, or `/v1/google/*` HTTP proxy routes for Gemini / Vertex protocol paths
- when the SDK cannot represent an AgentFlow field directly, adapt at the provider boundary instead of bypassing the SDK with fresh hand-written HTTP calls
