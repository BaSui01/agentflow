package multiagent

import (
	"context"
	"fmt"

	agent "github.com/BaSui01/agentflow/agent/runtime"
	"go.uber.org/zap"
)

// ConsensusCoordinator 共识协调器
type ConsensusCoordinator struct {
	config MultiAgentConfig
	hub    *MessageHub
	logger *zap.Logger
}

func NewConsensusCoordinator(config MultiAgentConfig, hub *MessageHub, logger *zap.Logger) *ConsensusCoordinator {
	return &ConsensusCoordinator{
		config: config,
		hub:    hub,
		logger: logger.With(zap.String("coordinator", "consensus")),
	}
}

func (c *ConsensusCoordinator) Coordinate(ctx context.Context, agents map[string]agent.Agent, input *agent.Input) (*agent.Output, error) {
	c.logger.Info("consensus coordination started",
		zap.Int("agents", len(agents)),
		zap.Float64("threshold", c.config.ConsensusThreshold),
	)

	orderedIDs := SortedAgentIDs(agents)
	if len(orderedIDs) == 0 {
		return nil, fmt.Errorf("consensus requires at least one agent")
	}

	proposals, err := c.collectConsensusProposals(ctx, agents, orderedIDs, input)
	if err != nil {
		return nil, err
	}

	if result, done, err := c.runConsensusRounds(ctx, agents, orderedIDs, input, proposals); err != nil {
		return nil, err
	} else if done {
		return result, nil
	}

	c.logger.Info("consensus threshold not met, synthesizing merged answer")
	return c.synthesizeConsensusResult(ctx, agents, orderedIDs, input, proposals)
}

func (c *ConsensusCoordinator) collectConsensusProposals(ctx context.Context, agents map[string]agent.Agent, orderedIDs []string, input *agent.Input) (map[string]*agent.Output, error) {
	proposals := make(map[string]*agent.Output, len(orderedIDs))
	for _, id := range orderedIDs {
		if err := ctx.Err(); err != nil {
			return nil, fmt.Errorf("consensus canceled during proposals: %w", err)
		}
		output, err := agents[id].Execute(ctx, input)
		if err != nil {
			c.logger.Warn("agent proposal failed",
				zap.String("agent_id", id),
				zap.Error(err),
			)
			continue
		}
		proposals[id] = output
	}
	if len(proposals) == 0 {
		return nil, fmt.Errorf("all agents failed during proposal phase")
	}
	return proposals, nil
}

func (c *ConsensusCoordinator) runConsensusRounds(ctx context.Context, agents map[string]agent.Agent, orderedIDs []string, input *agent.Input, proposals map[string]*agent.Output) (*agent.Output, bool, error) {
	for round := 1; round <= c.config.MaxRounds; round++ {
		if err := ctx.Err(); err != nil {
			return nil, false, fmt.Errorf("consensus canceled at round %d: %w", round, err)
		}
		c.logger.Debug("consensus round started", zap.Int("round", round))
		votes := c.collectConsensusVotes(ctx, agents, orderedIDs, input, proposals, round)
		if result, ok := c.consensusWinner(proposals, votes, round); ok {
			return result, true, nil
		}
	}
	return nil, false, nil
}

func (c *ConsensusCoordinator) collectConsensusVotes(ctx context.Context, agents map[string]agent.Agent, orderedIDs []string, input *agent.Input, proposals map[string]*agent.Output, round int) map[string][]string {
	votes := make(map[string][]string, len(proposals))
	for _, voterID := range orderedIDs {
		if _, ok := proposals[voterID]; !ok {
			continue
		}
		voteInput := c.buildConsensusVoteInput(input, proposals, orderedIDs, voterID, round)
		voteOutput, err := agents[voterID].Execute(ctx, voteInput)
		if err != nil {
			c.logger.Warn("agent vote failed",
				zap.String("agent_id", voterID),
				zap.Int("round", round),
				zap.Error(err),
			)
			continue
		}
		votedID := parseVote(voteOutput.Content, orderedIDs)
		if votedID == "" {
			votedID = voterID
		}
		votes[votedID] = append(votes[votedID], voterID)
		proposals[voterID] = voteOutput
	}
	return votes
}

func (c *ConsensusCoordinator) buildConsensusVoteInput(input *agent.Input, proposals map[string]*agent.Output, orderedIDs []string, voterID string, round int) *agent.Input {
	allProposals := buildCollectedPositions(orderedIDs, proposals, "\n--- Proposal by Agent [%s] ---\n%s\n")
	votePrompt := fmt.Sprintf(
		"You are participating in a consensus-building process (Round %d/%d).\n\n"+
			"## Original Question\n%s\n\n"+
			"## All Current Proposals\n%s\n\n"+
			"## Instructions\n"+
			"1. Evaluate each proposal for correctness, completeness, and clarity.\n"+
			"2. State which agent's proposal you agree with most and why (use the agent ID in brackets).\n"+
			"3. Identify specific points of agreement and disagreement.\n"+
			"4. Suggest concrete improvements that could move toward consensus.\n"+
			"5. Start your response with: VOTE: [agent_id]\n",
		round, c.config.MaxRounds,
		input.Content,
		allProposals,
	)
	return &agent.Input{
		TraceID: input.TraceID,
		Content: votePrompt,
		Context: map[string]any{
			"consensus_round": round,
			"voter_id":        voterID,
		},
	}
}

func (c *ConsensusCoordinator) consensusWinner(proposals map[string]*agent.Output, votes map[string][]string, round int) (*agent.Output, bool) {
	totalVoters := 0
	for _, voters := range votes {
		totalVoters += len(voters)
	}
	if totalVoters == 0 {
		return nil, false
	}
	for candID, voters := range votes {
		ratio := float64(len(voters)) / float64(totalVoters)
		c.logger.Debug("vote tally",
			zap.String("candidate", candID),
			zap.Int("votes", len(voters)),
			zap.Float64("ratio", ratio),
		)
		if ratio >= c.config.ConsensusThreshold {
			c.logger.Info("consensus reached",
				zap.String("winner", candID),
				zap.Float64("ratio", ratio),
				zap.Int("round", round),
			)
			result := proposals[candID]
			result.Metadata = MergeMetadata(result.Metadata, map[string]any{
				"consensus_round": round,
				"consensus_ratio": ratio,
				"winner_id":       candID,
				"total_voters":    totalVoters,
			})
			return result, true
		}
	}
	return nil, false
}

func (c *ConsensusCoordinator) synthesizeConsensusResult(ctx context.Context, agents map[string]agent.Agent, orderedIDs []string, input *agent.Input, proposals map[string]*agent.Output) (*agent.Output, error) {
	synthesizerID := orderedIDs[0]
	allPositions := buildCollectedPositions(orderedIDs, proposals, "\n--- Agent [%s] ---\n%s\n")
	mergePrompt := fmt.Sprintf(
		"Multiple agents could not reach consensus on the following question after %d rounds.\n\n"+
			"## Original Question\n%s\n\n"+
			"## All Agents' Final Positions\n%s\n\n"+
			"## Instructions\n"+
			"1. Identify the areas of agreement (common ground).\n"+
			"2. For areas of disagreement, evaluate the evidence and reasoning quality of each position.\n"+
			"3. Produce a single, balanced, well-structured synthesis that:\n"+
			"   - Incorporates all valid points of agreement.\n"+
			"   - Resolves disagreements by selecting the best-supported position.\n"+
			"   - Clearly notes any remaining uncertainties.\n",
		c.config.MaxRounds,
		input.Content,
		allPositions,
	)
	finalOutput, err := agents[synthesizerID].Execute(ctx, &agent.Input{
		TraceID: input.TraceID,
		Content: mergePrompt,
		Context: map[string]any{
			"phase":          "merge",
			"synthesizer_id": synthesizerID,
		},
	})
	if err != nil {
		for _, id := range orderedIDs {
			if p, ok := proposals[id]; ok {
				return p, nil
			}
		}
		return nil, fmt.Errorf("consensus failed and no proposals available")
	}

	totalTokens, totalCost := AggregateUsage(proposals)
	finalOutput.TokensUsed += totalTokens
	finalOutput.Cost += totalCost
	finalOutput.Metadata = MergeMetadata(finalOutput.Metadata, map[string]any{
		"consensus_reached": false,
		"total_rounds":      c.config.MaxRounds,
		"synthesizer_id":    synthesizerID,
	})

	c.logger.Info("consensus coordination completed (merged)",
		zap.Int("rounds", c.config.MaxRounds),
	)
	return finalOutput, nil
}

// PipelineCoordinator 流水线协调器
