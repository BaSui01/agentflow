package providers_test

import (
	"testing"

	"github.com/BaSui01/agentflow/llm"
	"github.com/BaSui01/agentflow/llm/providers/vendor"
	"go.uber.org/zap"
)

func newCompatTestProvider(t *testing.T, providerCode string, cfg vendor.ChatProviderConfig, logger *zap.Logger) llm.Provider {
	t.Helper()
	provider, err := vendor.NewChatProviderFromConfig(providerCode, cfg, logger)
	if err != nil {
		t.Fatalf("create compat test provider %s: %v", providerCode, err)
	}
	return provider
}
