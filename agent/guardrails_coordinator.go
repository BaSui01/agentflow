package agent

import (
	"github.com/BaSui01/agentflow/agent/guardcore"
	"github.com/BaSui01/agentflow/agent/guardrails"
	"go.uber.org/zap"
)

// GuardrailsCoordinator is the agent facade type for guardrails coordination.
type GuardrailsCoordinator = guardcore.Coordinator

// NewGuardrailsCoordinator creates a new GuardrailsCoordinator.
func NewGuardrailsCoordinator(config *guardrails.GuardrailsConfig, logger *zap.Logger) *GuardrailsCoordinator {
	return guardcore.NewCoordinator(config, logger)
}
