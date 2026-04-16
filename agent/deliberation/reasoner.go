package deliberation

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/BaSui01/agentflow/types"

	"github.com/BaSui01/agentflow/llm"
	"go.uber.org/zap"
)

// LLMReasoner implements Reasoner using an llm.Provider.
type LLMReasoner struct {
	provider llm.Provider
	model    string
	logger   *zap.Logger
}

// NewLLMReasoner creates a new LLM-backed Reasoner.
func NewLLMReasoner(provider llm.Provider, model string, logger *zap.Logger) *LLMReasoner {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &LLMReasoner{
		provider: provider,
		model:    model,
		logger:   logger.With(zap.String("component", "llm_reasoner")),
	}
}

// Think sends a reasoning prompt to the LLM and returns the response content
// along with a confidence score extracted from the response.
func (r *LLMReasoner) Think(ctx context.Context, prompt string) (content string, confidence float64, err error) {
	req := &llm.ChatRequest{
		Model: r.model,
		Messages: []types.Message{
			{
				Role:    llm.RoleSystem,
				Content: reasoningSystemPrompt,
			},
			{
				Role:    llm.RoleUser,
				Content: prompt,
			},
		},
		Temperature: 0.3,
	}

	resp, err := r.provider.Completion(ctx, req)
	if err != nil {
		return "", 0, fmt.Errorf("llm completion failed: %w", err)
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
