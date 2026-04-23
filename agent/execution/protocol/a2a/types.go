package a2a

import shared "github.com/BaSui01/agentflow/agent/execution/protocol/a2a/shared"

type CapabilityType = shared.CapabilityType

const (
	CapabilityTypeTask   = shared.CapabilityTypeTask
	CapabilityTypeQuery  = shared.CapabilityTypeQuery
	CapabilityTypeStream = shared.CapabilityTypeStream
)

type Capability = shared.Capability
type ToolDefinition = shared.ToolDefinition
type AgentCard = shared.AgentCard

var NewAgentCard = shared.NewAgentCard
