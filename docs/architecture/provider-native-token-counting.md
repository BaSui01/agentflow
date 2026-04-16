# Provider Native Token Counting

## Rule

Gateway chat budget precheck now depends on native provider token counting only.

The required path is:

`Handler/Service -> Gateway.preflightPolicy -> llm.TokenCountProvider`

There is no tokenizer fallback in gateway budget precheck anymore.

## What This Means

- If a chat provider implements `llm.TokenCountProvider`, gateway uses the provider's native SDK-backed token counting path.
- If a chat provider does not implement `llm.TokenCountProvider`, chat budget precheck fails before the request is sent.
- Non-chat capabilities do not perform gateway-side tokenizer estimation anymore.

## Why

- Native token counting is closer to upstream billing and context-window behavior.
- Local tokenizer estimation caused drift and hidden mismatches across providers.
- The gateway should not silently switch from precise provider-native counting to heuristic estimation.

## Allowed Patterns

- OpenAI native provider: use official Responses input token counting.
- Anthropic native provider: use official Messages count-tokens endpoint.
- Gemini native provider: use official `genai.Models.CountTokens`.

## Forbidden Patterns

Do not reintroduce any of the following in gateway preflight:

- `TokenizerResolver`
- `llm/tokenizer.GetTokenizerOrEstimator(...)` as a budget fallback
- local message/tool token estimation for chat budget admission

Tokenizer utilities may still exist elsewhere for standalone analysis or tooling, but they are no longer part of gateway budget admission.

## Migration Guidance

When adding a new native chat provider or changing an existing one:

1. Implement `llm.TokenCountProvider`.
2. Use the provider's official SDK/native counting API if available.
3. If no native counting API exists, either:
   - block gateway budget precheck for that provider, or
   - add a documented provider-local native counting implementation.

Do not hide missing native token counting behind a heuristic fallback.

