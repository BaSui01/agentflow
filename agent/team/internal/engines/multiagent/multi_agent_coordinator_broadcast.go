package multiagent

import (
	"context"
	"fmt"
	"strings"
	"sync"

	agent "github.com/BaSui01/agentflow/agent/runtime"
	"go.uber.org/zap"
)

type BroadcastCoordinator struct {
	config MultiAgentConfig
	hub    *MessageHub
	logger *zap.Logger
}

func NewBroadcastCoordinator(config MultiAgentConfig, hub *MessageHub, logger *zap.Logger) *BroadcastCoordinator {
	return &BroadcastCoordinator{
		config: config,
		hub:    hub,
		logger: logger.With(zap.String("coordinator", "broadcast")),
	}
}

func (c *BroadcastCoordinator) Coordinate(ctx context.Context, agents map[string]agent.Agent, input *agent.Input) (*agent.Output, error) {
	c.logger.Info("broadcast coordination started", zap.Int("agents", len(agents)))

	orderedIDs := SortedAgentIDs(agents)
	if len(orderedIDs) == 0 {
		return nil, fmt.Errorf("broadcast requires at least one agent")
	}

	// Phase 1 — Fan-out: execute all agents in parallel.
	type agentResult struct {
		id     string
		output *agent.Output
		err    error
	}

	results := make([]agentResult, len(orderedIDs))
	var wg sync.WaitGroup

	for i, id := range orderedIDs {
		wg.Add(1)
		go func(idx int, agentID string) {
			defer wg.Done()
			defer func() {
				if r := recover(); r != nil {
					results[idx] = agentResult{
						id:  agentID,
						err: fmt.Errorf("agent panicked: %v", r),
					}
					c.logger.Error("parallel agent execution panicked",
						zap.String("agent_id", agentID),
						zap.Any("recover", r),
						zap.Stack("stack"),
					)
				}
			}()

			output, err := agents[agentID].Execute(ctx, input)
			results[idx] = agentResult{id: agentID, output: output, err: err}
		}(i, id)
	}

	wg.Wait()

	// Collect successful outputs in deterministic order.
	succeeded := make([]agentResult, 0, len(results))
	for _, r := range results {
		if r.err != nil {
			c.logger.Warn("agent execution failed",
				zap.String("agent_id", r.id),
				zap.Error(r.err),
			)
			continue
		}
		succeeded = append(succeeded, r)
	}

	if len(succeeded) == 0 {
		return nil, fmt.Errorf("all agents failed during broadcast execution")
	}

	// Phase 2 — Fan-in: synthesize all outputs into a coherent result.
	// If only one agent succeeded, return its output directly.
	if len(succeeded) == 1 {
		return succeeded[0].output, nil
	}

	synthesizerID := orderedIDs[0]
	var allOutputs strings.Builder
	for _, r := range succeeded {
		allOutputs.WriteString(fmt.Sprintf("\n--- Agent [%s] ---\n%s\n", r.id, r.output.Content))
	}

	synthesisPrompt := fmt.Sprintf(
		"Multiple agents have independently responded to the same question.\n\n"+
			"## Original Question\n%s\n\n"+
			"## Individual Agent Responses\n%s\n\n"+
			"## Instructions\n"+
			"1. Review all agent responses for accuracy and completeness.\n"+
			"2. Identify common themes, agreements, and unique insights from each agent.\n"+
			"3. Synthesize a single, comprehensive answer that:\n"+
			"   - Combines the best insights from all responses.\n"+
			"   - Resolves any contradictions by favoring the most well-reasoned position.\n"+
			"   - Provides a complete, well-structured answer.\n",
		input.Content,
		allOutputs.String(),
	)

	synthesisInput := &agent.Input{
		TraceID: input.TraceID,
		Content: synthesisPrompt,
		Context: map[string]any{
			"phase":          "synthesis",
			"synthesizer_id": synthesizerID,
			"num_responses":  len(succeeded),
		},
	}

	finalOutput, err := agents[synthesizerID].Execute(ctx, synthesisInput)
	if err != nil {
		// Fallback: return the first successful output.
		c.logger.Warn("broadcast synthesis failed, returning first output",
			zap.Error(err),
		)
		return succeeded[0].output, nil
	}

	// Aggregate total usage.
	totalTokens, totalCost := 0, 0.0
	for _, r := range succeeded {
		totalTokens += r.output.TokensUsed
		totalCost += r.output.Cost
	}
	finalOutput.TokensUsed += totalTokens
	finalOutput.Cost += totalCost
	finalOutput.Metadata = MergeMetadata(finalOutput.Metadata, map[string]any{
		"broadcast_agents": len(succeeded),
		"failed_agents":    len(results) - len(succeeded),
		"synthesizer_id":   synthesizerID,
	})

	c.logger.Info("broadcast coordination completed",
		zap.Int("succeeded", len(succeeded)),
		zap.Int("failed", len(results)-len(succeeded)),
	)

	return finalOutput, nil
}

// NetworkCoordinator 网络协调器
