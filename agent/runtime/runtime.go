package runtime

import legacy "github.com/BaSui01/agentflow/agent/execution/runtime"

type Agent = legacy.Agent
type BaseAgent = legacy.BaseAgent
type AgentType = legacy.AgentType
type Input = legacy.Input
type Output = legacy.Output
type PlanResult = legacy.PlanResult
type Feedback = legacy.Feedback
type State = legacy.State
type RunConfig = legacy.RunConfig
type ToolManager = legacy.ToolManager
type BuildOptions = legacy.BuildOptions
type Builder = legacy.Builder
type AgentRegistry = legacy.AgentRegistry

type RuntimeStreamEmitter = legacy.RuntimeStreamEmitter
type RuntimeStreamEvent = legacy.RuntimeStreamEvent
type RuntimeToolCall = legacy.RuntimeToolCall
type RuntimeToolResult = legacy.RuntimeToolResult
type RuntimeHandoffTarget = legacy.RuntimeHandoffTarget

type TeamMember = legacy.TeamMember
type TeamResult = legacy.TeamResult
type TeamOptions = legacy.TeamOptions
type TeamOption = legacy.TeamOption
type Team = legacy.Team

const (
	TypeGeneric    = legacy.TypeGeneric
	TypeAssistant  = legacy.TypeAssistant
	TypeAnalyzer   = legacy.TypeAnalyzer
	TypeTranslator = legacy.TypeTranslator
	TypeSummarizer = legacy.TypeSummarizer
	TypeReviewer   = legacy.TypeReviewer

	StateInit      = legacy.StateInit
	StateReady     = legacy.StateReady
	StateRunning   = legacy.StateRunning
	StatePaused    = legacy.StatePaused
	StateCompleted = legacy.StateCompleted
	StateFailed    = legacy.StateFailed
)

var (
	NewBuilder                = legacy.NewBuilder
	NewAgentRegistry          = legacy.NewAgentRegistry
	DefaultBuildOptions       = legacy.DefaultBuildOptions
	WithRunConfig             = legacy.WithRunConfig
	WithMaxRounds             = legacy.WithMaxRounds
	WithTeamTimeout           = legacy.WithTeamTimeout
	WithTeamContext           = legacy.WithTeamContext
	WithRuntimeStreamEmitter  = legacy.WithRuntimeStreamEmitter
	WithRuntimeHandoffTargets = legacy.WithRuntimeHandoffTargets
	StringPtr                 = legacy.StringPtr
)
