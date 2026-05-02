package multiagent

import (
	"context"
	"fmt"

	agent "github.com/BaSui01/agentflow/agent/runtime"
	"go.uber.org/zap"
)

type PipelineCoordinator struct {
	config MultiAgentConfig
	hub    *MessageHub
	logger *zap.Logger
}

func NewPipelineCoordinator(config MultiAgentConfig, hub *MessageHub, logger *zap.Logger) *PipelineCoordinator {
	return &PipelineCoordinator{
		config: config,
		hub:    hub,
		logger: logger.With(zap.String("coordinator", "pipeline")),
	}
}

func (c *PipelineCoordinator) Coordinate(ctx context.Context, agents map[string]agent.Agent, input *agent.Input) (*agent.Output, error) {
	c.logger.Info("pipeline coordination started", zap.Int("stages", len(agents)))

	// Deterministic stage ordering by agent ID.
	orderedIDs := SortedAgentIDs(agents)
	if len(orderedIDs) == 0 {
		return nil, fmt.Errorf("pipeline requires at least one agent")
	}

	currentInput := input
	var currentOutput *agent.Output
	totalStages := len(orderedIDs)

	for i, id := range orderedIDs {
		stage := i + 1

		if err := ctx.Err(); err != nil {
			return nil, fmt.Errorf("pipeline canceled at stage %d/%d: %w", stage, totalStages, err)
		}

		c.logger.Debug("pipeline stage executing",
			zap.Int("stage", stage),
			zap.Int("total_stages", totalStages),
			zap.String("agent_id", id),
		)

		// For stages after the first, wrap the previous output with pipeline context.
		if i > 0 && currentOutput != nil {
			pipelinePrompt := fmt.Sprintf(
				"You are stage %d of %d in a processing pipeline.\n\n"+
					"## Original Request\n%s\n\n"+
					"## Output From Previous Stage (Stage %d)\n%s\n\n"+
					"## Instructions\n"+
					"Process the previous stage's output according to your expertise.\n"+
					"Build upon and refine the work done so far.\n"+
					"Maintain consistency with the original request's intent.\n",
				stage, totalStages,
				input.Content,
				stage-1,
				currentOutput.Content,
			)

			currentInput = &agent.Input{
				TraceID: input.TraceID,
				Content: pipelinePrompt,
				Context: mergeContextMaps(input.Context, map[string]any{
					"pipeline_stage": stage,
					"total_stages":   totalStages,
					"previous_agent": orderedIDs[i-1],
				}),
			}
		}

		output, err := agents[id].Execute(ctx, currentInput)
		if err != nil {
			return nil, fmt.Errorf("pipeline stage %d/%d (agent %s) failed: %w", stage, totalStages, id, err)
		}
		currentOutput = output
	}

	if currentOutput == nil {
		return nil, fmt.Errorf("pipeline produced no output")
	}

	currentOutput.Metadata = MergeMetadata(currentOutput.Metadata, map[string]any{
		"pipeline_stages": totalStages,
		"stage_order":     orderedIDs,
	})

	c.logger.Info("pipeline coordination completed", zap.Int("stages", totalStages))

	return currentOutput, nil
}

// BroadcastCoordinator 广播协调器
