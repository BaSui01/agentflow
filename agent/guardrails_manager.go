package agent

import (
	"github.com/BaSui01/agentflow/agent/guardcore"
	"go.uber.org/zap"
)

// GuardrailsManager is the agent facade type for guardrails management.
type GuardrailsManager = guardcore.Manager

// NewGuardrailsManager creates a new GuardrailsManager.
func NewGuardrailsManager(logger *zap.Logger) *GuardrailsManager {
	return guardcore.NewManager(logger)
}
