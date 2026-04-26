package bootstrap

import (
	"testing"

	"github.com/BaSui01/agentflow/api/handlers"
	"github.com/stretchr/testify/assert"
	"go.uber.org/zap"
)

func TestHTTPHandlerSet_Count(t *testing.T) {
	logger := zap.NewNop()

	set := &HTTPHandlerSet{}
	assert.Equal(t, 0, set.Count())

	set.HealthHandler = handlers.NewHealthHandler(logger)
	assert.Equal(t, 1, set.Count())

	chatHandler, err := handlers.NewChatHandler(nil, logger)
	if err != nil {
		t.Fatal(err)
	}
	set.ChatHandler = chatHandler
	assert.Equal(t, 2, set.Count())
}

func TestLLMRuntimeSet_IsAvailable(t *testing.T) {
	set := &LLMRuntimeSet{}
	assert.False(t, set.IsAvailable())
}

func TestStorageSet_HasResolver(t *testing.T) {
	set := &StorageSet{}
	assert.False(t, set.HasResolver())
}
