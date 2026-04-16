# Provider Tool Payload Mapping

## Goal

Keep tool/function-call payload mapping consistent across OpenAI, Anthropic, and Gemini native providers without forcing their wire formats to become identical.

The shared rule is:

- official SDKs own transport and native payload delivery
- AgentFlow owns normalized tool semantics
- provider-local code should only keep the minimum protocol-specific envelope logic

## Shared Mapping Layers

### 1. Request-side tool schema normalization

Shared helpers in `llm/providers/base/tool_mapping.go` handle:

- tool type normalization
- custom vs function tool schema branching
- search tool placeholder detection
- normalized tool choice parsing

Provider-local code should call these helpers before building upstream SDK params.

### 2. Streaming tool-call delta accumulation

Shared helpers in `llm/providers/base/tool_streaming.go` handle:

- registration of tool-call metadata
- incremental payload accumulation
- final function/custom tool-call materialization

Current usage:

- OpenAI Responses streaming uses the shared `ToolCallDeltaAccumulator`
- Anthropic and Gemini reuse the lower-level append/build helpers where their stream protocols differ

### 3. Tool output writeback normalization

Shared helpers in `llm/providers/base/tool_streaming.go` handle:

- extracting a normalized writeback envelope from `types.Message{Role: tool}`
- rendering provider-specific upstream tool output items

Current builders:

- `BuildOpenAIResponsesToolOutputItem(...)`
- `BuildAnthropicToolResultBlock(...)`
- `BuildGeminiFunctionResponse(...)`

## Provider Responsibilities

### OpenAI

Provider-local code may still keep:

- Responses-specific item wrappers
- `call_` -> `fc_` ID conversion
- SSE event-type handling

But tool-call accumulation and tool output writeback should prefer shared helpers.

### Anthropic

Provider-local code may still keep:

- `tool_use` / `tool_result` block envelopes
- `content_block_*` stream parsing
- Claude-specific web-search server tool blocks

But normalized tool call construction and tool result writeback should prefer shared helpers.

### Gemini

Provider-local code may still keep:

- `FunctionCall` / `FunctionResponse` envelope objects
- thought-signature / grounding metadata handling
- Gemini-specific tool config fields

But normalized tool call construction and tool output response shaping should prefer shared helpers.

## Forbidden Patterns

Do not add new provider-local copies of the following unless the shared helper cannot represent a required upstream field:

- tool type normalization
- tool choice normalization for common modes (`auto/any/required/none/validated`)
- tool output extraction from `types.Message`
- generic JSON delta concatenation for streamed tool arguments
- generic function/custom tool-call construction into `types.ToolCall`

## Allowed Provider-local Differences

These differences are expected and should remain local:

- OpenAI Responses function/custom call item names and ID rules
- Anthropic `tool_use` / `tool_result` block layout
- Gemini `FunctionDeclaration`, `FunctionCall`, `FunctionResponse`, and server-side tool invocation flags

## Maintenance Rule

When changing tool payload mapping in one native provider:

1. Check whether the logic is already represented in `llm/providers/base/tool_mapping.go` or `tool_streaming.go`.
2. If not, add the smallest reusable helper there first.
3. Keep provider-local code focused on envelope translation, not duplicated normalization logic.
4. Add or update regression tests in provider tests and shared base tests.

