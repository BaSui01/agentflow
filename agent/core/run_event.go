package core

import (
	"time"

	"github.com/BaSui01/agentflow/types"
)

// RunEvent maps an agent output to the shared run event contract.
func (o *Output) RunEvent(scope types.RunScope) types.RunEvent {
	event := types.RunEvent{
		Type:      types.RunEventStatus,
		Scope:     scope,
		Timestamp: time.Now(),
	}
	if o == nil {
		return event
	}
	event.TraceID = o.TraceID
	event.CheckpointID = o.CheckpointID
	event.Usage = outputUsage(o)
	event.Metadata = outputRunEventMetadata(o)
	event.Data = outputRunEventData(o)
	return event
}

func outputUsage(o *Output) *types.ChatUsage {
	if o == nil || o.TokensUsed <= 0 {
		return nil
	}
	return &types.ChatUsage{TotalTokens: o.TokensUsed}
}

func outputRunEventMetadata(o *Output) map[string]string {
	if o == nil {
		return nil
	}
	metadata := map[string]string{}
	if o.FinishReason != "" {
		metadata["finish_reason"] = o.FinishReason
	}
	if o.CurrentStage != "" {
		metadata["current_stage"] = o.CurrentStage
	}
	if o.SelectedReasoningMode != "" {
		metadata["selected_reasoning_mode"] = o.SelectedReasoningMode
	}
	if o.StopReason != "" {
		metadata["stop_reason"] = o.StopReason
	}
	if len(metadata) == 0 {
		return nil
	}
	return metadata
}

func outputRunEventData(o *Output) map[string]any {
	if o == nil {
		return nil
	}
	data := map[string]any{
		"status":   "completed",
		"content":  o.Content,
		"duration": o.Duration,
	}
	if o.ReasoningContent != nil {
		data["reasoning_content"] = *o.ReasoningContent
	}
	if o.Cost > 0 {
		data["cost"] = o.Cost
	}
	if o.IterationCount > 0 {
		data["iteration_count"] = o.IterationCount
	}
	if o.Resumable {
		data["resumable"] = o.Resumable
	}
	if len(o.Metadata) > 0 {
		data["metadata"] = o.Metadata
	}
	return data
}
