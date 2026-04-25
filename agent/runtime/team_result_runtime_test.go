package runtime

import (
	"testing"
	"time"

	"github.com/BaSui01/agentflow/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTeamResultRunEventMapsTeamMetadataAndUsage(t *testing.T) {
	result := &TeamResult{
		Content:    "team done",
		TokensUsed: 11,
		Cost:       0.03,
		Duration:   2 * time.Second,
		Metadata: map[string]any{
			"team_id":   "team-1",
			"team_mode": "selector",
		},
	}

	event := result.RunEvent()

	assert.Equal(t, types.RunEventStatus, event.Type)
	assert.Equal(t, types.RunScopeTeam, event.Scope)
	assert.Equal(t, "team-1", event.TeamID)
	assert.Equal(t, "selector", event.Metadata["team_mode"])
	require.NotNil(t, event.Usage)
	assert.Equal(t, 11, event.Usage.TotalTokens)
	require.IsType(t, map[string]any{}, event.Data)
	data := event.Data.(map[string]any)
	assert.Equal(t, "completed", data["status"])
	assert.Equal(t, "team done", data["content"])
	assert.Equal(t, 0.03, data["cost"])
	assert.False(t, event.Timestamp.IsZero())
}
