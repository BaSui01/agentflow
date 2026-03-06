package agent

import (
	"context"
	"fmt"
	"strings"

	"github.com/BaSui01/agentflow/llm"
	"github.com/BaSui01/agentflow/types"

	"go.uber.org/zap"
)

// preparedRequest holds the fully-built ChatRequest together with provider
// references needed by the execution paths (streaming, ReAct, plain completion).
type preparedRequest struct {
	req          *llm.ChatRequest
	chatProvider llm.Provider
	toolProvider llm.Provider // for ReAct loop (may equal chatProvider)
	hasTools     bool
	maxReActIter int
}

// prepareChatRequest builds a ChatRequest from messages, applying context
// engineering, model selection, RunConfig overrides, route hints, and tool
// filtering. Both ChatCompletion and StreamCompletion delegate here so that
// the logic is maintained in a single place.
func (b *BaseAgent) prepareChatRequest(ctx context.Context, messages []types.Message) (*preparedRequest, error) {
	if b.provider == nil {
		return nil, ErrProviderNotSet
	}
	if messages == nil || len(messages) == 0 {
		return nil, NewError(types.ErrInputValidation, "messages cannot be nil or empty")
	}

	chatProv := b.gatewayProvider()

	// 1. Context engineering: optimise message history
	if b.contextEngineEnabled && b.contextManager != nil && len(messages) > 1 {
		query := lastUserQuery(messages)
		optimized, err := b.contextManager.PrepareMessages(ctx, messages, query)
		if err != nil {
			b.logger.Warn("context optimization failed, using original messages", zap.Error(err))
		} else {
			tokensBefore := b.contextManager.EstimateTokens(messages)
			tokensAfter := b.contextManager.EstimateTokens(optimized)
			if tokensBefore != tokensAfter {
				b.logger.Debug("context optimized",
					zap.Int("tokens_before", tokensBefore),
					zap.Int("tokens_after", tokensAfter))
			}
			messages = optimized
		}
	}

	// 2. Model selection (context override takes precedence over config)
	model := b.config.LLM.Model
	if v, ok := types.LLMModel(ctx); ok && strings.TrimSpace(v) != "" {
		model = strings.TrimSpace(v)
	}

	// 3. Build base request
	req := &llm.ChatRequest{
		Model:       model,
		Messages:    messages,
		MaxTokens:   b.config.LLM.MaxTokens,
		Temperature: b.config.LLM.Temperature,
	}

	// 4. Apply RunConfig overrides
	rc := GetRunConfig(ctx)
	if rc != nil {
		rc.ApplyToRequest(req, b.config)
	}
	applyContextRouteHints(req, ctx)

	// 5. Tool whitelist filtering
	if b.toolManager != nil && len(b.config.Runtime.Tools) > 0 {
		req.Tools = filterToolSchemasByWhitelist(
			b.toolManager.GetAllowedTools(b.config.Core.ID),
			b.config.Runtime.Tools,
		)
	}

	// 6. Validate tool provider capability
	toolProv := chatProv
	if b.toolProvider != nil {
		toolProv = b.gatewayToolProvider()
	}
	if len(req.Tools) > 0 {
		if toolProv != nil && !toolProv.SupportsNativeFunctionCalling() {
			return nil, ErrToolProviderNotSupported.WithCause(fmt.Errorf("provider %q", toolProv.Name()))
		}
	}

	// 7. Effective ReAct iterations
	effectiveIter := rc.EffectiveMaxReActIterations(b.maxReActIterations())

	return &preparedRequest{
		req:          req,
		chatProvider: chatProv,
		toolProvider: toolProv,
		hasTools:     len(req.Tools) > 0 && b.toolManager != nil,
		maxReActIter: effectiveIter,
	}, nil
}

// lastUserQuery extracts the content of the last user message.
func lastUserQuery(messages []types.Message) string {
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role == llm.RoleUser {
			return messages[i].Content
		}
	}
	return ""
}

// effectiveToolModel returns the tool-specific model if configured, otherwise
// falls back to the main model.
func effectiveToolModel(mainModel string, configuredToolModel string) string {
	if v := strings.TrimSpace(configuredToolModel); v != "" {
		return v
	}
	return mainModel
}
