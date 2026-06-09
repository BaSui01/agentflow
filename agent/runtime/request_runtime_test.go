package runtime

import (
	"context"
	"testing"

	"github.com/BaSui01/agentflow/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

type panicPromptStore struct {
	panicValue any
}

func (s panicPromptStore) GetActive(context.Context, string, string, string) (PromptDocument, error) {
	panic(s.panicValue)
}

func TestExecuteCore_RecoversPromptStorePanic(t *testing.T) {
	provider := &captureRuntimeProvider{content: "hello"}
	gateway := wrapProviderWithGateway(provider, zap.NewNop(), nil)

	agent, err := BuildBaseAgent(types.AgentConfig{
		Core: types.CoreConfig{
			ID:   "agent-1",
			Name: "Agent 1",
			Type: string(TypeAssistant),
		},
	}, gateway, nil, nil, nil, zap.NewNop(), nil)
	require.NoError(t, err)
	require.NoError(t, agent.Init(context.Background()))

	agent.SetPromptStore(panicPromptStore{panicValue: "prompt store boom"})

	input := &Input{
		TraceID: "trace-1",
		Content: "hello",
	}

	var output *Output
	var execErr error
	assert.NotPanics(t, func() {
		output, execErr = agent.executeCore(context.Background(), input)
	})

	assert.Nil(t, output)
	require.Error(t, execErr)
	assert.Contains(t, execErr.Error(), "panic")
	assert.Equal(t, StateReady, agent.State())
	assert.Equal(t, int64(0), agent.execCount)
}
