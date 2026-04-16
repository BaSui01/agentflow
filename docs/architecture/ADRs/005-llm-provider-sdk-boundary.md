# ADR 005: LLM Provider SDK Boundary

## Status

Superseded by ADR 006

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

- self-managed HTTP adapters
- the unified `llm.Provider` abstraction
- config-driven construction through `llm/providers/vendor`

Official vendor SDKs for OpenAI, Gemini, and Anthropic are **not** adopted as foundational dependencies for the core provider architecture.

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

An official SDK may be evaluated only as a **single-provider pilot**, and only if all of the following are true:

1. It materially reduces protocol-adaptation code.
2. It covers the required AgentFlow surface for that provider, especially streaming, tools, structured output, base URL overrides, multi-key support, timeouts, and error mapping.
3. SDK-specific types do not leak outside the provider implementation boundary into `agent`, `workflow`, `api`, `cmd`, or shared runtime composition.
4. The migration removes the old implementation; it must not leave dual implementations in place.

### 5. Pilot Scope Default

If a future pilot is approved, the default first candidate is OpenAI only.

The pilot boundary must stay inside:

- `llm/providers/openai`

The following contracts must remain stable during the pilot:

- `llm.Provider`
- `llm/runtime/router`
- `llm/providers/vendor`
- startup and handler composition paths

Anthropic, Gemini, and the shared OpenAI-compatible base are out of scope for that first pilot.

## Consequences

### Positive

- preserves one stable provider abstraction across all vendors
- avoids three different SDK dependency models entering the core at once
- keeps routing and startup composition vendor-agnostic
- makes vendor-specific capability aggregation easier to evolve and test

### Negative

- AgentFlow continues owning low-level HTTP protocol adaptation
- new upstream API changes still require manual adapter maintenance

## Guardrails

- composition code must keep using `VendorChatProviderFactory` / `vendor.NewChatProviderFromConfig(...)`
- public multi-provider docs must demonstrate the vendor factory path instead of legacy ad-hoc registration
- any future SDK proposal must be accompanied by an ADR update or a new ADR
