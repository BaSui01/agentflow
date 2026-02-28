// Package agentflow provides a top-level convenience entry point for creating
// agents with minimal boilerplate.
//
// Usage:
//
//	import "github.com/BaSui01/agentflow"
//
//	a, err := agentflow.New(agentflow.WithOpenAI("gpt-4o-mini"))
//	a, err := agentflow.New(agentflow.WithAnthropic("claude-sonnet-4-20250514"))
//	a, err := agentflow.New(agentflow.WithProvider(myProvider), agentflow.WithModel("custom"))
//
// This is a thin wrapper around [quick.New]; both produce identical results.
// Use this package when you prefer the shorter import path.
package agentflow

import (
	"github.com/BaSui01/agentflow/agent"
	"github.com/BaSui01/agentflow/quick"
)

// Option configures the agent created by [New].
type Option = quick.Option

// New creates a [agent.BaseAgent] with minimal configuration.
// At minimum, a provider must be specified via [WithOpenAI], [WithAnthropic],
// [WithDeepSeek], or [WithProvider].
func New(opts ...Option) (*agent.BaseAgent, error) {
	return quick.New(opts...)
}

// Re-export provider shortcuts so callers never need to import quick/.

// WithProvider sets a pre-built LLM provider.
var WithProvider = quick.WithProvider

// WithOpenAI creates an OpenAI provider. API key from OPENAI_API_KEY env.
var WithOpenAI = quick.WithOpenAI

// WithAnthropic creates an Anthropic Claude provider. API key from ANTHROPIC_API_KEY env.
var WithAnthropic = quick.WithAnthropic

// WithDeepSeek creates a DeepSeek provider. API key from DEEPSEEK_API_KEY env.
var WithDeepSeek = quick.WithDeepSeek

// WithModel overrides the model name.
var WithModel = quick.WithModel

// WithName sets the agent name.
var WithName = quick.WithName

// WithSystemPrompt sets the system prompt.
var WithSystemPrompt = quick.WithSystemPrompt

// WithLogger sets a custom zap logger.
var WithLogger = quick.WithLogger

// WithAPIKey overrides the API key for provider shortcuts.
var WithAPIKey = quick.WithAPIKey
