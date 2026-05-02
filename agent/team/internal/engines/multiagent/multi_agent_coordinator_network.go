package multiagent

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	agent "github.com/BaSui01/agentflow/agent/runtime"
	"go.uber.org/zap"
)

type NetworkCoordinator struct {
	config MultiAgentConfig
	hub    *MessageHub
	logger *zap.Logger
}

func NewNetworkCoordinator(config MultiAgentConfig, hub *MessageHub, logger *zap.Logger) *NetworkCoordinator {
	return &NetworkCoordinator{
		config: config,
		hub:    hub,
		logger: logger.With(zap.String("coordinator", "network")),
	}
}

func (c *NetworkCoordinator) Coordinate(ctx context.Context, agents map[string]agent.Agent, input *agent.Input) (*agent.Output, error) {
	c.logger.Info("network coordination started",
		zap.Int("agents", len(agents)),
		zap.Int("max_rounds", c.config.MaxRounds),
	)

	orderedIDs := SortedAgentIDs(agents)
	if len(orderedIDs) == 0 {
		return nil, fmt.Errorf("network requires at least one agent")
	}

	positions, err := c.collectNetworkPositions(ctx, agents, orderedIDs, input)
	if err != nil {
		return nil, err
	}
	rounds := c.config.MaxRounds
	if rounds <= 0 {
		rounds = 1
	}
	if err := c.runNetworkRounds(ctx, agents, orderedIDs, input, positions, rounds); err != nil {
		return nil, err
	}
	return c.aggregateNetworkResult(ctx, agents, orderedIDs, input, positions, rounds)
}

func (c *NetworkCoordinator) collectNetworkPositions(ctx context.Context, agents map[string]agent.Agent, orderedIDs []string, input *agent.Input) (map[string]*agent.Output, error) {
	positions := make(map[string]*agent.Output, len(orderedIDs))
	for _, id := range orderedIDs {
		if err := ctx.Err(); err != nil {
			return nil, fmt.Errorf("network canceled during initial phase: %w", err)
		}
		output, err := agents[id].Execute(ctx, input)
		if err != nil {
			c.logger.Warn("agent initial response failed",
				zap.String("agent_id", id),
				zap.Error(err),
			)
			continue
		}
		positions[id] = output
	}
	if len(positions) == 0 {
		return nil, fmt.Errorf("all agents failed during initial response phase")
	}
	return positions, nil
}

func (c *NetworkCoordinator) runNetworkRounds(ctx context.Context, agents map[string]agent.Agent, orderedIDs []string, input *agent.Input, positions map[string]*agent.Output, rounds int) error {
	for round := 1; round <= rounds; round++ {
		if err := ctx.Err(); err != nil {
			return fmt.Errorf("network canceled at round %d: %w", round, err)
		}
		c.logger.Debug("network communication round", zap.Int("round", round))
		c.broadcastNetworkRound(ctx, orderedIDs, positions, round)
		c.refineNetworkRound(ctx, agents, orderedIDs, input, positions, round, rounds)
	}
	return nil
}

func (c *NetworkCoordinator) broadcastNetworkRound(ctx context.Context, orderedIDs []string, positions map[string]*agent.Output, round int) {
	for _, id := range orderedIDs {
		if pos, ok := positions[id]; ok {
			sendErr := c.hub.SendWithContext(ctx, &Message{
				FromID:  id,
				Type:    MessageTypeProposal,
				Content: pos.Content,
				Metadata: map[string]any{
					"round": round,
				},
				Timestamp: time.Now(),
			})
			if sendErr != nil {
				c.logger.Warn("failed to broadcast peer message",
					zap.String("agent_id", id),
					zap.Int("round", round),
					zap.Error(sendErr),
				)
			}
		}
	}
}

func (c *NetworkCoordinator) refineNetworkRound(ctx context.Context, agents map[string]agent.Agent, orderedIDs []string, input *agent.Input, positions map[string]*agent.Output, round, rounds int) {
	for _, id := range orderedIDs {
		if _, ok := positions[id]; !ok {
			continue
		}
		networkInput, ok := c.buildNetworkRoundInput(input, positions, orderedIDs, id, round, rounds)
		if !ok {
			continue
		}
		output, err := agents[id].Execute(ctx, networkInput)
		if err != nil {
			c.logger.Warn("agent network round failed",
				zap.String("agent_id", id),
				zap.Int("round", round),
				zap.Error(err),
			)
			continue
		}
		positions[id] = output
	}
}

func (c *NetworkCoordinator) buildNetworkRoundInput(input *agent.Input, positions map[string]*agent.Output, orderedIDs []string, id string, round, rounds int) (*agent.Input, bool) {
	peerInsights := buildPeerPositions(orderedIDs, positions, id, "\n--- Peer [%s] (round %d) ---\n%s\n", round)
	if peerInsights == "" {
		return nil, false
	}
	networkPrompt := fmt.Sprintf(
		"You are agent [%s] in a peer-to-peer network (Round %d/%d).\n"+
			"Each agent has specialized knowledge and you've received messages from your peers.\n\n"+
			"## Original Question\n%s\n\n"+
			"## Your Current Position\n%s\n\n"+
			"## Peer Messages Received\n%s\n\n"+
			"## Instructions\n"+
			"1. Consider each peer's perspective carefully.\n"+
			"2. Identify new information or arguments that strengthen or challenge your position.\n"+
			"3. Update your position by incorporating valuable peer insights.\n"+
			"4. Highlight any specific points where you changed your mind and why.\n"+
			"5. Maintain your expertise while being open to valid corrections.\n",
		id, round, rounds,
		input.Content,
		positions[id].Content,
		peerInsights,
	)
	return &agent.Input{
		TraceID: input.TraceID,
		Content: networkPrompt,
		Context: map[string]any{
			"network_round": round,
			"agent_id":      id,
			"peer_count":    len(orderedIDs) - 1,
		},
	}, true
}

func (c *NetworkCoordinator) aggregateNetworkResult(ctx context.Context, agents map[string]agent.Agent, orderedIDs []string, input *agent.Input, positions map[string]*agent.Output, rounds int) (*agent.Output, error) {
	aggregatorID := orderedIDs[0]
	allPositions := buildCollectedPositions(orderedIDs, positions, "\n--- Agent [%s] (final) ---\n%s\n")
	aggregatePrompt := fmt.Sprintf(
		"After %d rounds of peer-to-peer discussion, all agents have refined their positions.\n\n"+
			"## Original Question\n%s\n\n"+
			"## All Agents' Final Positions\n%s\n\n"+
			"## Instructions\n"+
			"1. Agents have already exchanged and incorporated each other's feedback.\n"+
			"2. Identify the converged consensus points.\n"+
			"3. For remaining differences, select the most well-supported position.\n"+
			"4. Produce a final, unified, comprehensive answer.\n",
		rounds,
		input.Content,
		allPositions,
	)
	finalOutput, err := agents[aggregatorID].Execute(ctx, &agent.Input{
		TraceID: input.TraceID,
		Content: aggregatePrompt,
		Context: map[string]any{
			"phase":         "aggregation",
			"aggregator_id": aggregatorID,
			"num_agents":    len(positions),
			"total_rounds":  rounds,
		},
	})
	if err != nil {
		for _, id := range orderedIDs {
			if p, ok := positions[id]; ok {
				return p, nil
			}
		}
		return nil, fmt.Errorf("network coordination failed with no available positions")
	}

	totalTokens, totalCost := AggregateUsage(positions)
	finalOutput.TokensUsed += totalTokens
	finalOutput.Cost += totalCost
	finalOutput.Metadata = MergeMetadata(finalOutput.Metadata, map[string]any{
		"network_rounds":    rounds,
		"participating_ids": orderedIDs,
		"aggregator_id":     aggregatorID,
	})

	c.logger.Info("network coordination completed",
		zap.Int("rounds", rounds),
		zap.Int("agents", len(positions)),
	)
	return finalOutput, nil
}

func buildPeerPositions(ids []string, outputs map[string]*agent.Output, selfID, format string, args ...any) string {
	var b strings.Builder
	for _, peerID := range ids {
		if peerID == selfID {
			continue
		}
		if output, ok := outputs[peerID]; ok {
			formatArgs := append([]any{peerID}, args...)
			formatArgs = append(formatArgs, output.Content)
			b.WriteString(fmt.Sprintf(format, formatArgs...))
		}
	}
	return b.String()
}

func buildCollectedPositions(ids []string, outputs map[string]*agent.Output, format string) string {
	var b strings.Builder
	for _, id := range ids {
		if output, ok := outputs[id]; ok {
			b.WriteString(fmt.Sprintf(format, id, output.Content))
		}
	}
	return b.String()
}

// ---------------------------------------------------------------------------
// Shared helpers for coordinators
// ---------------------------------------------------------------------------

// SortedAgentIDs returns deterministic, lexicographically sorted agent IDs.
func SortedAgentIDs(agents map[string]agent.Agent) []string {
	ids := make([]string, 0, len(agents))
	for id := range agents {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

// AggregateUsage sums TokensUsed and Cost from a map of outputs.
func AggregateUsage(outputs map[string]*agent.Output) (totalTokens int, totalCost float64) {
	for _, o := range outputs {
		if o != nil {
			totalTokens += o.TokensUsed
			totalCost += o.Cost
		}
	}
	return
}

// MergeMetadata non-destructively merges extra key-value pairs into an
// existing metadata map. If base is nil a new map is allocated.
func MergeMetadata(base, extra map[string]any) map[string]any {
	if base == nil {
		base = make(map[string]any, len(extra))
	}
	for k, v := range extra {
		base[k] = v
	}
	return base
}

// mergeContextMaps merges two context maps, with override taking precedence.
func mergeContextMaps(base, override map[string]any) map[string]any {
	if len(base) == 0 && len(override) == 0 {
		return nil
	}
	merged := make(map[string]any, len(base)+len(override))
	for k, v := range base {
		merged[k] = v
	}
	for k, v := range override {
		merged[k] = v
	}
	return merged
}

// parseVote extracts a voted agent ID from an output string.
// It looks for the pattern "VOTE: [agent_id]" and validates against known IDs.
func parseVote(content string, validIDs []string) string {
	// Build a set for O(1) lookup.
	valid := make(map[string]struct{}, len(validIDs))
	for _, id := range validIDs {
		valid[id] = struct{}{}
	}

	// Search for "VOTE:" prefix (case-insensitive).
	lower := strings.ToLower(content)
	idx := strings.Index(lower, "vote:")
	if idx < 0 {
		return ""
	}
	rest := strings.TrimSpace(content[idx+5:]) // skip "vote:"

	// Extract the token after "VOTE:" (may be wrapped in brackets).
	rest = strings.TrimLeft(rest, "[ \t")
	end := strings.IndexAny(rest, "] \t\n\r,")
	if end < 0 {
		end = len(rest)
	}
	candidate := strings.TrimSpace(rest[:end])

	if _, ok := valid[candidate]; ok {
		return candidate
	}
	return ""
}
