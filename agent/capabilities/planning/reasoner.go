package planning

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/BaSui01/agentflow/llm"
	llmcore "github.com/BaSui01/agentflow/llm/core"
	"github.com/BaSui01/agentflow/types"
	"go.uber.org/zap"
)

// LLMReasoner implements Reasoner using an llmcore.Gateway.
type LLMReasoner struct {
	gateway llmcore.Gateway
	model   string
	logger  *zap.Logger
}

// NewLLMReasoner creates a new LLM-backed Reasoner.
func NewLLMReasoner(gateway llmcore.Gateway, model string, logger *zap.Logger) *LLMReasoner {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &LLMReasoner{
		gateway: gateway,
		model:   model,
		logger:  logger.With(zap.String("component", "llm_reasoner")),
	}
}

// Think sends a reasoning prompt to the LLM and returns the response content
// along with a confidence score extracted from the response.
func (r *LLMReasoner) Think(ctx context.Context, prompt string) (content string, confidence float64, err error) {
	req := newReasonerChatRequest(r.model, []types.Message{
		{
			Role:    llm.RoleSystem,
			Content: reasoningSystemPrompt,
		},
		{
			Role:    llm.RoleUser,
			Content: prompt,
		},
	}, 0.3)

	resp, err := r.invokeChat(ctx, req)
	if err != nil {
		return "", 0, fmt.Errorf("llm invoke failed: %w", err)
	}

	if len(resp.Choices) == 0 {
		return "", 0, fmt.Errorf("llm returned no choices")
	}

	content = resp.Choices[0].Message.Content
	confidence = parseConfidence(content)

	r.logger.Debug("reasoning complete",
		zap.Int("content_len", len(content)),
		zap.Float64("confidence", confidence),
	)

	return content, confidence, nil
}

func (r *LLMReasoner) invokeChat(ctx context.Context, req *llm.ChatRequest) (*llm.ChatResponse, error) {
	if r.gateway == nil {
		return nil, fmt.Errorf("gateway is not configured")
	}
	resp, err := r.gateway.Invoke(ctx, &llmcore.UnifiedRequest{
		Capability: llmcore.CapabilityChat,
		ModelHint:  req.Model,
		TraceID:    req.TraceID,
		Payload:    req,
	})
	if err != nil {
		return nil, err
	}
	chatResp, ok := resp.Output.(*llm.ChatResponse)
	if !ok || chatResp == nil {
		return nil, fmt.Errorf("invalid chat response from gateway")
	}
	return chatResp, nil
}

// parseConfidence extracts a confidence value from the LLM response.
// It looks for a line like "CONFIDENCE: 0.85" in the response text.
// Returns 0.5 as default if no confidence is found.
func parseConfidence(content string) float64 {
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		lower := strings.ToLower(line)
		if strings.HasPrefix(lower, "confidence:") {
			valStr := strings.TrimSpace(line[len("confidence:"):])
			if val, err := strconv.ParseFloat(valStr, 64); err == nil {
				if val < 0 {
					return 0
				}
				if val > 1 {
					return 1
				}
				return val
			}
		}
	}
	return 0.5
}

const reasoningSystemPrompt = `You are a reasoning engine for an AI agent. Analyze the given prompt carefully and provide your reasoning.

At the end of your response, you MUST include a confidence score on its own line in this exact format:
CONFIDENCE: <value>

Where <value> is a decimal number between 0 and 1 representing how confident you are in your analysis.

Example:
Based on the available tools and task description, the best approach is to use the search tool first.
CONFIDENCE: 0.85`
