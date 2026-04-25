package runtime

import (
	"time"

	"github.com/BaSui01/agentflow/types"
)

// RunEvent maps a TeamResult to the shared run event contract.
func (r *TeamResult) RunEvent() types.RunEvent {
	event := types.RunEvent{
		Type:      types.RunEventStatus,
		Scope:     types.RunScopeTeam,
		Timestamp: time.Now(),
	}
	if r == nil {
		return event
	}
	event.TeamID = teamResultMetadataString(r.Metadata, "team_id")
	event.Usage = teamResultUsage(r)
	event.Metadata = teamResultRunEventMetadata(r)
	event.Data = teamResultRunEventData(r)
	return event
}

func teamResultUsage(r *TeamResult) *types.ChatUsage {
	if r == nil || r.TokensUsed <= 0 {
		return nil
	}
	return &types.ChatUsage{TotalTokens: r.TokensUsed}
}

func teamResultRunEventMetadata(r *TeamResult) map[string]string {
	if r == nil {
		return nil
	}
	metadata := map[string]string{}
	if mode := teamResultMetadataString(r.Metadata, "team_mode"); mode != "" {
		metadata["team_mode"] = mode
	}
	if len(metadata) == 0 {
		return nil
	}
	return metadata
}

func teamResultRunEventData(r *TeamResult) map[string]any {
	if r == nil {
		return nil
	}
	data := map[string]any{
		"status":   "completed",
		"content":  r.Content,
		"duration": r.Duration,
	}
	if r.Cost > 0 {
		data["cost"] = r.Cost
	}
	if len(r.Metadata) > 0 {
		data["metadata"] = r.Metadata
	}
	return data
}

func teamResultMetadataString(metadata map[string]any, key string) string {
	if len(metadata) == 0 {
		return ""
	}
	value, _ := metadata[key].(string)
	return value
}
