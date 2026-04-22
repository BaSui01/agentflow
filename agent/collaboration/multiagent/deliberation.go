package multiagent

import (
	"context"
	"fmt"
	"strings"

	"github.com/BaSui01/agentflow/agent"
	"go.uber.org/zap"
)

const defaultDeliberationMaxRounds = 3

type deliberationModeStrategy struct {
	logger *zap.Logger
}

func newDeliberationModeStrategy(logger *zap.Logger) *deliberationModeStrategy {
	return &deliberationModeStrategy{
		logger: logger.With(zap.String("mode", ModeDeliberation)),
	}
}

func (m *deliberationModeStrategy) Name() string { return ModeDeliberation }

func (m *deliberationModeStrategy) Execute(ctx context.Context, agents []agent.Agent, input *agent.Input) (*agent.Output, error) {
	if len(agents) < 2 {
		return nil, fmt.Errorf("deliberation mode requires at least two agents")
	}

	agentMap := make(map[string]agent.Agent)
	for _, a := range agents {
		agentMap[a.ID()] = a
	}
	orderedIDs := SortedAgentIDs(agentMap)

	maxRounds := defaultDeliberationMaxRounds
	if input != nil && input.Context != nil {
		if v, ok := input.Context["max_rounds"].(int); ok && v > 0 {
			maxRounds = v
		}
	}

	var sharedState SharedState
	if input != nil && input.Context != nil {
		if ss, ok := input.Context["shared_state"].(SharedState); ok {
			sharedState = ss
		}
	}

	outputs := make(map[string]*agent.Output)
	for _, id := range orderedIDs {
		if err := ctx.Err(); err != nil {
			return nil, fmt.Errorf("deliberation cancelled during initial phase: %w", err)
		}
		out, err := agentMap[id].Execute(ctx, input)
		if err != nil {
			m.logger.Warn("agent initial execution failed",
				zap.String("agent_id", id),
				zap.Error(err),
			)
			continue
		}
		outputs[id] = out
		if sharedState != nil {
			_ = sharedState.Set(ctx, "agent:"+id, out)
		}
	}
	if len(outputs) == 0 {
		return nil, fmt.Errorf("all agents failed during initial phase")
	}

	prevContents := make(map[string]string)
	for id, o := range outputs {
		prevContents[id] = o.Content
	}

	for round := 2; round <= maxRounds; round++ {
		if err := ctx.Err(); err != nil {
			return nil, fmt.Errorf("deliberation cancelled at round %d: %w", round, err)
		}

		for _, id := range orderedIDs {
			if _, ok := outputs[id]; !ok {
				continue
			}

			var othersBuilder strings.Builder
			for _, peerID := range orderedIDs {
				if peerID == id {
					continue
				}
				if o, ok := outputs[peerID]; ok {
					othersBuilder.WriteString(fmt.Sprintf("\n--- Agent [%s] ---\n%s\n", peerID, o.Content))
				}
			}

			prompt := input.Content
			if othersBuilder.Len() > 0 {
				prompt = fmt.Sprintf(
					"You are in a deliberation process (Round %d/%d). Reflect on your previous output and others' outputs.\n\n"+
						"## Original Question\n%s\n\n"+
						"## Your Previous Output\n%s\n\n"+
						"## Other Agents' Outputs\n%s\n\n"+
						"## Instructions\n"+
						"1. Identify errors or gaps in your own previous output.\n"+
						"2. Incorporate valid insights from other agents.\n"+
						"3. Produce a refined, corrected output.\n",
					round, maxRounds,
					input.Content,
					outputs[id].Content,
					othersBuilder.String(),
				)
			}

			delibInput := &agent.Input{
				TraceID: input.TraceID,
				Content: prompt,
				Context: map[string]any{
					"deliberation_round": round,
					"max_rounds":         maxRounds,
					"agent_id":           id,
				},
			}

			out, err := agentMap[id].Execute(ctx, delibInput)
			if err != nil {
				m.logger.Warn("agent deliberation round failed",
					zap.String("agent_id", id),
					zap.Int("round", round),
					zap.Error(err),
				)
				continue
			}
			outputs[id] = out
			if sharedState != nil {
				_ = sharedState.Set(ctx, "round:"+fmt.Sprintf("%d", round)+":"+id, out)
			}
		}

		converged := true
		for id, o := range outputs {
			if prev, ok := prevContents[id]; ok && prev != o.Content {
				converged = false
				break
			}
		}
		for id, o := range outputs {
			prevContents[id] = o.Content
		}
		if converged {
			m.logger.Debug("deliberation converged early", zap.Int("round", round))
			break
		}
	}

	synthesizerID := orderedIDs[0]
	var allOutputs strings.Builder
	for _, id := range orderedIDs {
		if o, ok := outputs[id]; ok {
			allOutputs.WriteString(fmt.Sprintf("\n--- Agent [%s] ---\n%s\n", id, o.Content))
		}
	}

	synthesisPrompt := fmt.Sprintf(
		"You are the synthesizer in a deliberation process.\n\n"+
			"## Original Question\n%s\n\n"+
			"## All Agents' Final Outputs\n%s\n\n"+
			"## Instructions\n"+
			"1. Integrate the best elements from all outputs.\n"+
			"2. Resolve contradictions by favoring the most well-reasoned position.\n"+
			"3. Produce a single, comprehensive answer.\n",
		input.Content,
		allOutputs.String(),
	)

	synthesisInput := &agent.Input{
		TraceID: input.TraceID,
		Content: synthesisPrompt,
		Context: map[string]any{
			"phase":             "synthesis",
			"synthesizer_id":    synthesizerID,
			"participating_ids": orderedIDs,
		},
	}

	finalOutput, err := agentMap[synthesizerID].Execute(ctx, synthesisInput)
	if err != nil {
		m.logger.Warn("synthesizer failed, falling back to first output",
			zap.String("synthesizer_id", synthesizerID),
			zap.Error(err),
		)
		for _, id := range orderedIDs {
			if o, ok := outputs[id]; ok {
				return o, nil
			}
		}
		return nil, fmt.Errorf("deliberation completed but no valid outputs remain")
	}

	if sharedState != nil {
		_ = sharedState.Set(ctx, "result:deliberation", finalOutput)
	}

	totalTokens, totalCost := AggregateUsage(outputs)
	finalOutput.TokensUsed += totalTokens
	finalOutput.Cost += totalCost
	finalOutput.Metadata = MergeMetadata(finalOutput.Metadata, map[string]any{
		"mode":              ModeDeliberation,
		"participating_ids": orderedIDs,
		"synthesizer_id":    synthesizerID,
	})

	return finalOutput, nil
}
