package runtime

import (
	"context"
	"fmt"
	"strings"

	"github.com/BaSui01/agentflow/types"
)

func (b *BaseAgent) prepareResumeInput(ctx context.Context, input *Input) (*Input, error) {
	if input == nil || b.checkpointManager == nil {
		return input, nil
	}
	checkpointID, resumeLatest := resumeDirective(input)
	if checkpointID == "" && !resumeLatest {
		return input, nil
	}

	var (
		checkpoint *Checkpoint
		err        error
	)
	if checkpointID != "" {
		checkpoint, err = b.checkpointManager.LoadCheckpoint(ctx, checkpointID)
	} else {
		threadID := resumeThreadID(input, b.ID())
		checkpoint, err = b.checkpointManager.LoadLatestCheckpoint(ctx, threadID)
	}
	if err != nil {
		return nil, err
	}
	if checkpoint != nil && checkpoint.AgentID != "" && checkpoint.AgentID != b.ID() {
		return nil, NewError(types.ErrInputValidation,
			fmt.Sprintf("checkpoint agent ID mismatch: checkpoint belongs to %s, current agent is %s", checkpoint.AgentID, b.ID()))
	}
	return mergeInputWithCheckpoint(input, checkpoint), nil
}

func resumeDirective(input *Input) (string, bool) {
	if input == nil || len(input.Context) == 0 {
		return "", false
	}
	if checkpointID, ok := input.Context["checkpoint_id"].(string); ok && strings.TrimSpace(checkpointID) != "" {
		return strings.TrimSpace(checkpointID), true
	}
	if enabled, ok := input.Context["resume_latest"].(bool); ok && enabled {
		return "", true
	}
	if enabled, ok := input.Context["resume"].(bool); ok && enabled {
		return "", true
	}
	return "", false
}

func resumeThreadID(input *Input, fallbackAgentID string) string {
	if input == nil {
		return fallbackAgentID
	}
	if threadID := strings.TrimSpace(input.ChannelID); threadID != "" {
		return threadID
	}
	if traceID := strings.TrimSpace(input.TraceID); traceID != "" {
		return traceID
	}
	return fallbackAgentID
}

func mergeInputWithCheckpoint(input *Input, checkpoint *Checkpoint) *Input {
	merged := shallowCopyInput(input)
	if merged.Context == nil {
		merged.Context = make(map[string]any)
	}
	if checkpoint == nil {
		return merged
	}

	if strings.TrimSpace(merged.ChannelID) == "" {
		merged.ChannelID = checkpoint.ThreadID
	}
	merged.Context["checkpoint_id"] = checkpoint.ID
	merged.Context["resume_from_checkpoint"] = true
	merged.Context["resumable"] = true

	for key, value := range checkpoint.Metadata {
		merged.Context[key] = value
	}
	if checkpoint.ExecutionContext != nil {
		if strings.TrimSpace(checkpoint.ExecutionContext.CurrentNode) != "" {
			merged.Context["current_stage"] = checkpoint.ExecutionContext.CurrentNode
		}
		for key, value := range checkpoint.ExecutionContext.Variables {
			merged.Context[key] = value
		}
	}
	if strings.TrimSpace(merged.Content) == "" {
		if goal, ok := merged.Context["goal"].(string); ok && strings.TrimSpace(goal) != "" {
			merged.Content = goal
		}
	}
	return merged
}
