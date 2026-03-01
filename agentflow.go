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
package agentflow

import (
	"fmt"
	"os"

	"github.com/BaSui01/agentflow/agent"
	"github.com/BaSui01/agentflow/llm"
	"github.com/BaSui01/agentflow/llm/factory"
	"go.uber.org/zap"
)

// Option configures the agent created by New.
type Option func(*options)

type options struct {
	name         string
	model        string
	systemPrompt string
	provider     llm.Provider
	logger       *zap.Logger

	providerName string
	apiKey       string
}

// WithProvider sets a pre-built LLM provider.
func WithProvider(p llm.Provider) Option {
	return func(o *options) { o.provider = p }
}

// WithOpenAI creates an OpenAI provider. API key from OPENAI_API_KEY env.
func WithOpenAI(model string) Option {
	return func(o *options) {
		o.providerName = "openai"
		o.model = model
		if o.apiKey == "" {
			o.apiKey = os.Getenv("OPENAI_API_KEY")
		}
	}
}

// WithAnthropic creates an Anthropic Claude provider. API key from ANTHROPIC_API_KEY env.
func WithAnthropic(model string) Option {
	return func(o *options) {
		o.providerName = "anthropic"
		o.model = model
		if o.apiKey == "" {
			o.apiKey = os.Getenv("ANTHROPIC_API_KEY")
		}
	}
}

// WithDeepSeek creates a DeepSeek provider. API key from DEEPSEEK_API_KEY env.
func WithDeepSeek(model string) Option {
	return func(o *options) {
		o.providerName = "deepseek"
		o.model = model
		if o.apiKey == "" {
			o.apiKey = os.Getenv("DEEPSEEK_API_KEY")
		}
	}
}

// WithModel overrides the model name.
func WithModel(model string) Option {
	return func(o *options) { o.model = model }
}

// WithName sets the agent name.
func WithName(name string) Option {
	return func(o *options) { o.name = name }
}

// WithSystemPrompt sets the system prompt.
func WithSystemPrompt(prompt string) Option {
	return func(o *options) { o.systemPrompt = prompt }
}

// WithLogger sets a custom zap logger.
func WithLogger(logger *zap.Logger) Option {
	return func(o *options) { o.logger = logger }
}

// WithAPIKey overrides the API key for provider shortcuts.
func WithAPIKey(key string) Option {
	return func(o *options) { o.apiKey = key }
}

// New creates a [agent.BaseAgent] with minimal configuration.
func New(opts ...Option) (*agent.BaseAgent, error) {
	o := &options{name: "agentflow-agent"}
	for _, opt := range opts {
		opt(o)
	}

	p := o.provider
	if p == nil {
		if o.providerName == "" {
			return nil, fmt.Errorf("provider is required: use WithProvider, WithOpenAI, or WithAnthropic")
		}
		if o.apiKey == "" {
			return nil, fmt.Errorf("API key is required for %s: set the environment variable or use WithAPIKey", o.providerName)
		}
		var err error
		p, err = factory.NewProviderFromConfig(o.providerName, factory.ProviderConfig{
			APIKey: o.apiKey,
			Model:  o.model,
		}, o.logger)
		if err != nil {
			return nil, fmt.Errorf("create %s provider: %w", o.providerName, err)
		}
	}

	if o.logger == nil {
		o.logger = zap.NewNop()
	}

	cfg := agent.Config{
		ID:    o.name,
		Name:  o.name,
		Type:  agent.TypeAssistant,
		Model: o.model,
	}
	if o.systemPrompt != "" {
		cfg.PromptBundle = agent.PromptBundle{
			System: agent.SystemPrompt{
				Identity: o.systemPrompt,
			},
		}
	}

	return agent.NewAgentBuilder(cfg).
		WithProvider(p).
		WithLogger(o.logger).
		Build()
}

