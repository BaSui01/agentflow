package usecase

import agentteam "github.com/BaSui01/agentflow/agent/team"

func normalizedExecutionMode(req AgentExecuteRequest) string {
	return agentteam.NormalizeExecutionMode(req.Mode, len(req.AgentIDs) > 0)
}

func SupportedExecutionModes() []string {
	return agentteam.SupportedExecutionModes()
}

func IsSupportedExecutionMode(mode string) bool {
	return agentteam.IsSupportedExecutionMode(mode)
}
