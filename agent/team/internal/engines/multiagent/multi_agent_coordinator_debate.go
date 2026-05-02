package multiagent

import (
	"context"
	"fmt"

	agent "github.com/BaSui01/agentflow/agent/runtime"
	"go.uber.org/zap"
)

// DebateCoordinator 辩论协调器
type DebateCoordinator struct {
	config MultiAgentConfig
	hub    *MessageHub
	logger *zap.Logger
}

func NewDebateCoordinator(config MultiAgentConfig, hub *MessageHub, logger *zap.Logger) *DebateCoordinator {
	return &DebateCoordinator{
		config: config,
		hub:    hub,
		logger: logger.With(zap.String("coordinator", "debate")),
	}
}

func (c *DebateCoordinator) Coordinate(ctx context.Context, agents map[string]agent.Agent, input *agent.Input) (*agent.Output, error) {
	c.logger.Info("debate coordination started",
		zap.Int("agents", len(agents)),
		zap.Int("max_rounds", c.config.MaxRounds),
	)

	orderedIDs := SortedAgentIDs(agents)
	if len(orderedIDs) == 0 {
		return nil, fmt.Errorf("debate requires at least one agent")
	}

	proposals, proposalsErr := c.collectDebateProposals(ctx, agents, orderedIDs, input)
	if proposalsErr != nil {
		return nil, proposalsErr
	}

	if runErr := c.runDebateRounds(ctx, agents, orderedIDs, input, proposals); runErr != nil {
		return nil, runErr
	}

	finalOutput, err := c.synthesizeDebateResult(ctx, agents, orderedIDs, input, proposals)
	if err != nil {
		return nil, err
	}

	c.logger.Info("debate coordination completed",
		zap.Int("rounds", c.config.MaxRounds),
		zap.Int("proposals", len(proposals)),
	)

	return finalOutput, nil
}

func (c *DebateCoordinator) collectDebateProposals(ctx context.Context, agents map[string]agent.Agent, orderedIDs []string, input *agent.Input) (map[string]*agent.Output, error) {
	proposals := make(map[string]*agent.Output, len(orderedIDs))
	for _, id := range orderedIDs {
		if err := ctx.Err(); err != nil {
			return nil, fmt.Errorf("debate canceled during initial proposals: %w", err)
		}
		output, err := agents[id].Execute(ctx, input)
		if err != nil {
			c.logger.Warn("agent initial proposal failed",
				zap.String("agent_id", id),
				zap.Error(err),
			)
			continue
		}
		proposals[id] = output
	}
	if len(proposals) == 0 {
		return nil, fmt.Errorf("all agents failed during initial proposal phase")
	}
	return proposals, nil
}

func (c *DebateCoordinator) runDebateRounds(ctx context.Context, agents map[string]agent.Agent, orderedIDs []string, input *agent.Input, proposals map[string]*agent.Output) error {
	for round := 1; round <= c.config.MaxRounds; round++ {
		if err := ctx.Err(); err != nil {
			return fmt.Errorf("debate canceled at round %d: %w", round, err)
		}
		c.logger.Debug("debate round started", zap.Int("round", round))
		for _, id := range orderedIDs {
			debateInput, ok := c.buildDebateRoundInput(input, proposals, orderedIDs, id, round)
			if !ok {
				continue
			}
			output, err := agents[id].Execute(ctx, debateInput)
			if err != nil {
				c.logger.Warn("agent debate round failed",
					zap.String("agent_id", id),
					zap.Int("round", round),
					zap.Error(err),
				)
				continue
			}
			proposals[id] = output
		}
	}
	return nil
}

func (c *DebateCoordinator) buildDebateRoundInput(input *agent.Input, proposals map[string]*agent.Output, orderedIDs []string, id string, round int) (*agent.Input, bool) {
	otherPositions := buildPeerPositions(orderedIDs, proposals, id, "\n--- Agent [%s] ---\n%s\n")
	if otherPositions == "" {
		return nil, false
	}
	debatePrompt := fmt.Sprintf(
		"You are participating in a structured multi-agent debate (Round %d/%d).\n\n"+
			"## Original Question\n%s\n\n"+
			"## Your Previous Position\n%s\n\n"+
			"## Other Agents' Positions\n%s\n\n"+
			"## Instructions\n"+
			"1. Identify the strongest and weakest points in each position above.\n"+
			"2. Acknowledge valid arguments from other agents.\n"+
			"3. Refute incorrect or incomplete reasoning with evidence.\n"+
			"4. Synthesize an improved, well-structured response that integrates the best insights.\n"+
			"5. Clearly state your refined position.\n",
		round, c.config.MaxRounds,
		input.Content,
		proposals[id].Content,
		otherPositions,
	)
	return &agent.Input{
		TraceID: input.TraceID,
		Content: debatePrompt,
		Context: map[string]any{
			"debate_round": round,
			"max_rounds":   c.config.MaxRounds,
			"agent_id":     id,
		},
	}, true
}

func (c *DebateCoordinator) synthesizeDebateResult(ctx context.Context, agents map[string]agent.Agent, orderedIDs []string, input *agent.Input, proposals map[string]*agent.Output) (*agent.Output, error) {
	judgeID := orderedIDs[0]
	allPositions := buildCollectedPositions(orderedIDs, proposals, "\n--- Agent [%s] (final position) ---\n%s\n")
	synthesisPrompt := fmt.Sprintf(
		"You are the final judge in a multi-agent debate.\n\n"+
			"## Original Question\n%s\n\n"+
			"## All Agents' Final Positions\n%s\n\n"+
			"## Instructions\n"+
			"1. Evaluate each agent's final position for accuracy, completeness, and reasoning quality.\n"+
			"2. Identify points of agreement across agents (consensus areas).\n"+
			"3. Resolve remaining disagreements by selecting the best-supported arguments.\n"+
			"4. Produce a single, authoritative, well-structured answer that synthesizes the strongest elements.\n"+
			"5. Do NOT simply pick one agent's answer — integrate and improve upon all of them.\n",
		input.Content,
		allPositions,
	)
	finalOutput, err := agents[judgeID].Execute(ctx, &agent.Input{
		TraceID: input.TraceID,
		Content: synthesisPrompt,
		Context: map[string]any{
			"phase":      "synthesis",
			"judge_id":   judgeID,
			"num_agents": len(proposals),
		},
	})
	if err != nil {
		c.logger.Warn("judge synthesis failed, falling back to best proposal",
			zap.String("judge_id", judgeID),
			zap.Error(err),
		)
		for _, id := range orderedIDs {
			if p, ok := proposals[id]; ok {
				return p, nil
			}
		}
		return nil, fmt.Errorf("debate completed but no valid proposals remain")
	}
	totalTokens, totalCost := AggregateUsage(proposals)
	finalOutput.TokensUsed += totalTokens
	finalOutput.Cost += totalCost
	finalOutput.Metadata = MergeMetadata(finalOutput.Metadata, map[string]any{
		"debate_rounds":     c.config.MaxRounds,
		"participating_ids": orderedIDs,
		"judge_id":          judgeID,
	})

	return finalOutput, nil
}

