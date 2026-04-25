package types

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRunConfigCloneDeepCopies(t *testing.T) {
	model := "gpt-5.4"
	timeout := 3 * time.Second
	maxIterations := 4
	rc := &RunConfig{
		Model:              &model,
		Stop:               []string{"END"},
		ToolWhitelist:      []string{"search"},
		Timeout:            &timeout,
		MaxReActIterations: &maxIterations,
		Metadata:           map[string]string{"trace": "t1"},
		Tags:               []string{"prod"},
	}

	clone := rc.Clone()
	require.NotNil(t, clone)
	require.NotSame(t, rc, clone)
	assert.Equal(t, rc, clone)

	*rc.Model = "mutated"
	rc.Stop[0] = "MUTATED"
	rc.ToolWhitelist[0] = "mutated"
	*rc.Timeout = time.Second
	rc.Metadata["trace"] = "mutated"
	rc.Tags[0] = "mutated"

	assert.Equal(t, "gpt-5.4", *clone.Model)
	assert.Equal(t, []string{"END"}, clone.Stop)
	assert.Equal(t, []string{"search"}, clone.ToolWhitelist)
	assert.Equal(t, 3*time.Second, *clone.Timeout)
	assert.Equal(t, map[string]string{"trace": "t1"}, clone.Metadata)
	assert.Equal(t, []string{"prod"}, clone.Tags)
}

func TestRunConfigApplyToExecutionOptionsPreservesOverrides(t *testing.T) {
	model := "gpt-5.4"
	provider := " openai "
	toolChoice := "required"
	disableTools := &RunConfig{DisableTools: true}
	timeout := 2 * time.Second
	maxIterations := 0
	rc := &RunConfig{
		Model:              &model,
		Provider:           &provider,
		ToolChoice:         &toolChoice,
		ToolWhitelist:      []string{"calc"},
		Timeout:            &timeout,
		MaxReActIterations: &maxIterations,
		Metadata:           map[string]string{"tenant": "t1"},
		Tags:               []string{"tag-1"},
	}
	options := AgentConfig{}.ExecutionOptions()
	disableTools.ApplyToExecutionOptions(&options)
	rc.ApplyToExecutionOptions(&options)

	assert.Equal(t, "gpt-5.4", options.Model.Model)
	assert.Equal(t, "openai", options.Model.Provider)
	require.NotNil(t, options.Tools.ToolChoice)
	assert.Equal(t, ToolChoiceModeRequired, options.Tools.ToolChoice.Mode)
	assert.False(t, options.Tools.DisableTools)
	assert.Equal(t, []string{"calc"}, options.Tools.ToolWhitelist)
	assert.Equal(t, 2*time.Second, options.Control.Timeout)
	assert.Equal(t, 0, options.Control.MaxReActIterations)
	assert.Equal(t, map[string]string{"tenant": "t1"}, options.Metadata)
	assert.Equal(t, []string{"tag-1"}, options.Tags)
}
