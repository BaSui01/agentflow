package core

import (
	"context"
	"testing"

	"github.com/BaSui01/agentflow/types"
	"github.com/stretchr/testify/assert"
	"go.uber.org/zap"
)

type enhancedMemoryRunnerStub struct {
	saveCalled   bool
	recordCalled bool
}

func (s *enhancedMemoryRunnerStub) LoadWorking(context.Context, string) ([]types.MemoryEntry, error) {
	return nil, nil
}

func (s *enhancedMemoryRunnerStub) LoadShortTerm(context.Context, string, int) ([]types.MemoryEntry, error) {
	return nil, nil
}

func (s *enhancedMemoryRunnerStub) SaveShortTerm(context.Context, string, string, map[string]any) error {
	s.saveCalled = true
	return nil
}

func (s *enhancedMemoryRunnerStub) RecordEpisode(context.Context, *types.EpisodicEvent) error {
	s.recordCalled = true
	return nil
}

func TestExtensionRegistry_SaveToEnhancedMemory_SkipsWritebackWhenPolicyDisablesExternalContext(t *testing.T) {
	runner := &enhancedMemoryRunnerStub{}
	reg := NewExtensionRegistry[Input, Output](zap.NewNop())
	reg.EnableEnhancedMemory(runner)

	ctx := types.WithMemoryExternalContextPolicy(context.Background(), "disable_recall,disable_write")
	reg.SaveToEnhancedMemory(ctx, "agent-1", EnhancedMemoryRecord{Content: "result"})

	assert.False(t, runner.saveCalled)
	assert.False(t, runner.recordCalled)
}

func TestExtensionRegistry_SaveToEnhancedMemory_WritesWithoutExternalContextPolicy(t *testing.T) {
	runner := &enhancedMemoryRunnerStub{}
	reg := NewExtensionRegistry[Input, Output](zap.NewNop())
	reg.EnableEnhancedMemory(runner)

	reg.SaveToEnhancedMemory(context.Background(), "agent-1", EnhancedMemoryRecord{Content: "result"})

	assert.True(t, runner.saveCalled)
	assert.True(t, runner.recordCalled)
}
