# Function Calling Regression Matrix

This document defines the current regression scope for provider tool/function
calling. It is an index and acceptance matrix, not a claim that every row is
fully covered today.

## Scope

Function calling changes must keep these layers aligned:

```text
API / SDK request
  -> types.ToolSchema / types.ToolChoice / types.ToolCall
    -> agent/adapters.DefaultChatRequestAdapter
      -> llm/gateway
        -> llm/providers/base tool mapping helpers
          -> provider-native request / stream / tool-result writeback
```

The provider boundary owns wire-format envelopes. AgentFlow owns normalized
tool semantics.

## Required Provider Matrix

| Provider path | Request mapping | Streaming accumulation | Tool result writeback | Notes |
|---|---|---|---|---|
| OpenAI Responses | Required | Required | Required | Includes Responses item wrappers and function/custom tool calls. |
| OpenAI Chat / compatible | Required | Required | Required | Covers OpenAI-compatible providers using chat-completions semantics. |
| Anthropic Claude Messages | Required | Required | Required | Covers `tool_use` and `tool_result` content blocks. |
| Google Gemini | Required | Required | Required | Covers `FunctionDeclaration`, `FunctionCall`, and `FunctionResponse`. |
| XML fallback | Required | Required | Required | Used when a provider lacks native function calling. |

## Required Behavior Matrix

| Behavior | Required check |
|---|---|
| `tools` request field | Tool schema reaches provider payload or XML fallback prompt. |
| `tool_choice=auto` | Provider receives auto/default tool policy. |
| `tool_choice=none` | Provider does not call tools. |
| `tool_choice=required/any` | Provider is forced into tool-use-capable mode when supported. |
| Specific tool choice | Only the selected tool is requested when the provider supports it. |
| `parallel_tool_calls` | Parallel tool call flag survives supported provider boundaries. |
| Streaming tool deltas | Incremental arguments are accumulated into complete `types.ToolCall` values. |
| Tool result writeback | `types.Message{Role: tool}` becomes the provider-native tool-result envelope. |
| Unknown tool | Runtime returns a controlled error, not a silent final answer. |
| Malformed arguments | Runtime surfaces validation or execution error with trace context. |
| Provider error | Error mapping preserves provider context without leaking SDK internals above provider layer. |

## Current Coverage Status

As of 2026-04-25, the shared provider contract has executable coverage for:

- `tool_choice` normalization across auto, none, required/any, specific tool, and allowed-tool modes.
- Streaming tool-call delta accumulation for interleaved function calls and custom tool calls.
- Tool result writeback helper behavior for OpenAI Responses, Anthropic, and Gemini payload builders.
- Parallel tool-call behavior through request flags and multiple/interleaved normalized tool calls.
- Controlled provider/tool error handling for provider errors, malformed arguments, and unknown tools.

OpenAI Responses provider coverage is complete for this matrix row:

- Request mapping covers `tools`, `tool_choice=required`, `parallel_tool_calls`, and web search placeholder payloads.
- Streaming accumulation covers function tool-call argument deltas and custom tool-call input deltas.
- Tool result writeback covers Responses `function_call_output` and `custom_tool_call_output` input items.

OpenAI Chat Completions / compatible provider coverage is complete for this matrix row:

- Request mapping covers `types.ToolSchema`, `tool_choice`, `parallel_tool_calls`, and chat-completions message payloads.
- Streaming accumulation covers OpenAI-compatible tool-call deltas into normalized tool calls.
- Tool result writeback covers assistant tool calls and `RoleTool` results in chat-completions messages.

Anthropic Claude provider coverage is complete for this matrix row:

- Request mapping covers `types.ToolSchema`, web search payloads, `tool_choice`, and disable-parallel behavior.
- Streaming accumulation covers Claude `input_json_delta` into complete normalized tool calls.
- Tool result writeback covers Claude `tool_result` blocks including content, tool-use ID, and error flag.

Google Gemini provider coverage is complete for this matrix row:

- Request mapping covers `FunctionDeclaration`, Google Search payloads, `ToolConfig`, and allowed function names.
- Streaming accumulation covers streamed `FunctionCall` parts into normalized tool calls.
- Tool result writeback covers Gemini `FunctionResponse` parts for JSON and plain-text tool results.

XML fallback provider coverage is complete for this matrix row:

- Request mapping covers gateway `ToolCallModeXML` switching and middleware system-prompt tool injection.
- Streaming accumulation covers `<tool_calls>` chunks into normalized tool calls.
- Tool result writeback covers preserving `RoleTool` messages across XML rewrite for the next turn.

Livecheck coverage exists through `scripts/livecheck` test `B-tool-loop`, which
builds an Agent with a real tool manager, requests one `add` tool call, executes
the tool, and observes runtime `tool_call` / `tool_result` events.

## Current Validation Commands

Use the narrowest command that covers the touched layer:

```bash
go test ./types -run "TestAgentConfigExecutionOptions|TestModelCatalog" -count=1
go test ./agent/adapters -run TestDefaultChatRequestAdapter -count=1
go test ./llm/providers/base -run "Tool|Function" -count=1
go test ./llm/providers -run "Tool|Function|Stream" -count=1
go test ./api/handlers -run "Tool|Function|Stream" -count=1
go test ./agent/runtime -run "Tool|Function|ToolResult" -count=1
```

When a provider-specific package has a narrower test, prefer that package first,
then run the shared provider and API checks.

## Maintenance Rules

- Start from [Provider工具负载映射说明.md](./Provider工具负载映射说明.md) before changing provider tool payloads.
- Reuse `llm/providers/base/tool_mapping.go` and `llm/providers/base/tool_streaming.go` before adding provider-local normalization.
- Keep provider SDK structs below `llm/providers/*`.
- Keep handler code protocol-only; handler code must not implement provider-specific tool payload rules.
- Update this matrix when a new provider path, tool choice mode, or streaming event shape becomes official.
