package team

import (
	"context"

	multiagent "github.com/BaSui01/agentflow/agent/collaboration/multiagent"
	legacy "github.com/BaSui01/agentflow/agent/collaboration/team"
	runtime "github.com/BaSui01/agentflow/agent/runtime"
	"go.uber.org/zap"
)

type TeamMode = legacy.TeamMode
type TeamConfig = legacy.TeamConfig
type TurnRecord = legacy.TurnRecord
type AgentTeam = legacy.AgentTeam
type TeamBuilder = legacy.TeamBuilder
type Crew = legacy.Crew
type CrewConfig = legacy.CrewConfig
type ProcessType = legacy.ProcessType
type Role = legacy.Role
type CrewTask = legacy.CrewTask
type TaskResult = legacy.TaskResult
type Proposal = legacy.Proposal
type ProposalType = legacy.ProposalType
type NegotiationResult = legacy.NegotiationResult
type TeamMember = runtime.TeamMember
type TeamOptions = runtime.TeamOptions
type TeamOption = runtime.TeamOption
type TeamResult = runtime.TeamResult

type Team interface {
	ID() string
	Members() []TeamMember
	Execute(ctx context.Context, task string, opts ...TeamOption) (*TeamResult, error)
}

type ModeStrategy = multiagent.ModeStrategy
type ModeRegistry = multiagent.ModeRegistry
type SharedState = multiagent.SharedState
type InMemorySharedState = multiagent.InMemorySharedState

type CollaborationPattern = multiagent.CollaborationPattern
type MultiAgentConfig = multiagent.MultiAgentConfig
type MultiAgentSystem = multiagent.MultiAgentSystem

type ModeExecutor interface {
	Execute(ctx context.Context, modeName string, agents []runtime.Agent, input *runtime.Input) (*runtime.Output, error)
}

const (
	ModeSupervisor TeamMode = legacy.ModeSupervisor
	ModeRoundRobin TeamMode = legacy.ModeRoundRobin
	ModeSelector   TeamMode = legacy.ModeSelector
	ModeSwarm      TeamMode = legacy.ModeSwarm

	ModeReasoning     = multiagent.ModeReasoning
	ModeCollaboration = multiagent.ModeCollaboration
	ModeHierarchical  = multiagent.ModeHierarchical
	ModeCrew          = multiagent.ModeCrew
	ModeDeliberation  = multiagent.ModeDeliberation
	ModeFederation    = multiagent.ModeFederation
	ModeParallel      = multiagent.ModeParallel
	ModeLoop          = multiagent.ModeLoop

	PatternDebate    = multiagent.PatternDebate
	PatternConsensus = multiagent.PatternConsensus
	PatternPipeline  = multiagent.PatternPipeline
	PatternBroadcast = multiagent.PatternBroadcast
	PatternNetwork   = multiagent.PatternNetwork

	ProcessSequential   = legacy.ProcessSequential
	ProcessHierarchical = legacy.ProcessHierarchical
	ProcessConsensus    = legacy.ProcessConsensus

)

func NewTeamBuilder(name string) *TeamBuilder { return legacy.NewTeamBuilder(name) }

func NewCrew(config CrewConfig, logger *zap.Logger) *Crew { return legacy.NewCrew(config, logger) }

func NewModeRegistry() *ModeRegistry { return multiagent.NewModeRegistry() }

func NewInMemorySharedState() *InMemorySharedState { return multiagent.NewInMemorySharedState() }

func NewInMemorySharedStateWithLogger(logger *zap.Logger) *InMemorySharedState {
	return multiagent.NewInMemorySharedStateWithLogger(logger)
}

func RegisterDefaultModes(reg *ModeRegistry, logger *zap.Logger) error {
	return multiagent.RegisterDefaultModes(reg, logger)
}

func DefaultModeExecutor(logger *zap.Logger) (ModeExecutor, error) {
	reg := multiagent.NewModeRegistry()
	if err := multiagent.RegisterDefaultModes(reg, logger); err != nil {
		return nil, err
	}
	return reg, nil
}

func GlobalModeExecutor() ModeExecutor { return multiagent.GlobalModeRegistry() }

func GlobalModeRegistry() *ModeRegistry { return multiagent.GlobalModeRegistry() }


// Legacy engine aliases keep existing examples and live checks on the official
// agent/team import path while the implementations move under agent/team/internal.
type AggregatedResult = multiagent.AggregatedResult
type AggregationStrategy = multiagent.AggregationStrategy
type Aggregator = multiagent.Aggregator
type BroadcastCoordinator = multiagent.BroadcastCoordinator
type ConsensusCoordinator = multiagent.ConsensusCoordinator
type Coordinator = multiagent.Coordinator
type DebateCoordinator = multiagent.DebateCoordinator
type DedupResultAggregator = multiagent.DedupResultAggregator
type FailurePolicy = multiagent.FailurePolicy
type Message = multiagent.Message
type MessageHub = multiagent.MessageHub
type MessageType = multiagent.MessageType
type NetworkCoordinator = multiagent.NetworkCoordinator
type PipelineConfig = multiagent.PipelineConfig
type PipelineCoordinator = multiagent.PipelineCoordinator
type QueryDecomposer = multiagent.QueryDecomposer
type RetrievalResultAggregator = multiagent.RetrievalResultAggregator
type RetrievalSupervisor = multiagent.RetrievalSupervisor
type RetrievalWorker = multiagent.RetrievalWorker
type RetryPolicy = multiagent.RetryPolicy
type RoleCapability = multiagent.RoleCapability
type RoleDefinition = multiagent.RoleDefinition
type RoleExecuteFunc = multiagent.RoleExecuteFunc
type RoleInstance = multiagent.RoleInstance
type RolePipeline = multiagent.RolePipeline
type RoleRegistry = multiagent.RoleRegistry
type RoleStatus = multiagent.RoleStatus
type RoleTransition = multiagent.RoleTransition
type RoleType = multiagent.RoleType
type ScopedStores = multiagent.ScopedStores
type StaticSplitter = multiagent.StaticSplitter
type Supervisor = multiagent.Supervisor
type SupervisorConfig = multiagent.SupervisorConfig
type TaskSplitter = multiagent.TaskSplitter
type WorkerPool = multiagent.WorkerPool
type WorkerPoolConfig = multiagent.WorkerPoolConfig
type WorkerResult = multiagent.WorkerResult
type WorkerTask = multiagent.WorkerTask

const (
	MessageTypeBroadcast = multiagent.MessageTypeBroadcast
	MessageTypeConsensus = multiagent.MessageTypeConsensus
	MessageTypeProposal = multiagent.MessageTypeProposal
	MessageTypeResponse = multiagent.MessageTypeResponse
	MessageTypeVote = multiagent.MessageTypeVote
	ModeTeamRoundRobin = multiagent.ModeTeamRoundRobin
	ModeTeamSelector = multiagent.ModeTeamSelector
	ModeTeamSupervisor = multiagent.ModeTeamSupervisor
	ModeTeamSwarm = multiagent.ModeTeamSwarm
	PolicyFailFast = multiagent.PolicyFailFast
	PolicyPartialResult = multiagent.PolicyPartialResult
	PolicyRetryFailed = multiagent.PolicyRetryFailed
	RoleCollector = multiagent.RoleCollector
	RoleCoordinator = multiagent.RoleCoordinator
	RoleCustom = multiagent.RoleCustom
	RoleDesigner = multiagent.RoleDesigner
	RoleFilter = multiagent.RoleFilter
	RoleGenerator = multiagent.RoleGenerator
	RoleImplementer = multiagent.RoleImplementer
	RoleStatusActive = multiagent.RoleStatusActive
	RoleStatusBlocked = multiagent.RoleStatusBlocked
	RoleStatusDone = multiagent.RoleStatusDone
	RoleStatusFailed = multiagent.RoleStatusFailed
	RoleStatusIdle = multiagent.RoleStatusIdle
	RoleValidator = multiagent.RoleValidator
	RoleWriter = multiagent.RoleWriter
	StrategyBestOfN = multiagent.StrategyBestOfN
	StrategyMergeAll = multiagent.StrategyMergeAll
	StrategyVoteMajority = multiagent.StrategyVoteMajority
	StrategyWeightedMerge = multiagent.StrategyWeightedMerge
)

var (
	AggregateUsage = multiagent.AggregateUsage
	DefaultMultiAgentConfig = multiagent.DefaultMultiAgentConfig
	DefaultPipelineConfig = multiagent.DefaultPipelineConfig
	DefaultSupervisorConfig = multiagent.DefaultSupervisorConfig
	DefaultWorkerPoolConfig = multiagent.DefaultWorkerPoolConfig
	MergeMetadata = multiagent.MergeMetadata
	NewAggregator = multiagent.NewAggregator
	NewBroadcastCoordinator = multiagent.NewBroadcastCoordinator
	NewConsensusCoordinator = multiagent.NewConsensusCoordinator
	NewDebateCoordinator = multiagent.NewDebateCoordinator
	NewDedupResultAggregator = multiagent.NewDedupResultAggregator
	NewMessageHub = multiagent.NewMessageHub
	NewMessageHubWithStore = multiagent.NewMessageHubWithStore
	NewNetworkCoordinator = multiagent.NewNetworkCoordinator
	NewPipelineCoordinator = multiagent.NewPipelineCoordinator
	NewResearchCollectorRole = multiagent.NewResearchCollectorRole
	NewResearchFilterRole = multiagent.NewResearchFilterRole
	NewResearchGeneratorRole = multiagent.NewResearchGeneratorRole
	NewResearchValidatorRole = multiagent.NewResearchValidatorRole
	NewResearchWriterRole = multiagent.NewResearchWriterRole
	NewRetrievalSupervisor = multiagent.NewRetrievalSupervisor
	NewRolePipeline = multiagent.NewRolePipeline
	NewRoleRegistry = multiagent.NewRoleRegistry
	NewScopedStores = multiagent.NewScopedStores
	NewSupervisor = multiagent.NewSupervisor
	NewWorkerPool = multiagent.NewWorkerPool
	RegisterResearchRoles = multiagent.RegisterResearchRoles
	RegisterTeamModes = multiagent.RegisterTeamModes
	SortedAgentIDs = multiagent.SortedAgentIDs
)

func NewMultiAgentSystem(agents []runtime.Agent, config MultiAgentConfig, logger *zap.Logger) *MultiAgentSystem {
	return multiagent.NewMultiAgentSystem(agents, config, logger)
}
