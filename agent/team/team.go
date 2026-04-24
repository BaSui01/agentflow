package team

import (
	"context"
	"fmt"
	"time"

	"github.com/BaSui01/agentflow/agent/persistence"
	agent "github.com/BaSui01/agentflow/agent/runtime"
	"github.com/BaSui01/agentflow/agent/team/internal/engines/hierarchical"
	"github.com/BaSui01/agentflow/agent/team/internal/engines/multiagent"

	"go.uber.org/zap"
)

// TeamMode defines the collaboration mode for a team.
type TeamMode string

const (
	ModeSupervisor TeamMode = "supervisor"  // Supervisor 路由分配
	ModeRoundRobin TeamMode = "round_robin" // 轮询发言
	ModeSelector   TeamMode = "selector"    // LLM 选择下一个 agent
	ModeSwarm      TeamMode = "swarm"       // 自主协作 + handoff
)

// TeamConfig holds configuration for an AgentTeam.
type TeamConfig struct {
	Mode            TeamMode
	MaxRounds       int
	Timeout         time.Duration
	EnablePlanner   bool
	SelectorPrompt  string
	TerminationFunc func(history []TurnRecord) bool
}

// TurnRecord records a single turn in the team conversation.
type TurnRecord struct {
	AgentID string
	Content string
	Round   int
}

// AgentTeam is the official multi-agent facade for AgentFlow.
type AgentTeam struct {
	id       string
	name     string
	members  []agent.TeamMember
	mode     TeamMode
	strategy teamModeStrategy
	config   TeamConfig
	logger   *zap.Logger
}

type Team interface {
	ID() string
	Members() []agent.TeamMember
	Execute(ctx context.Context, task string, opts ...agent.TeamOption) (*agent.TeamResult, error)
}

type ModeRegistry = multiagent.ModeRegistry
type ModeStrategy = multiagent.ModeStrategy
type SharedState = multiagent.SharedState
type InMemorySharedState = multiagent.InMemorySharedState
type CollaborationPattern = multiagent.CollaborationPattern
type MultiAgentConfig = multiagent.MultiAgentConfig
type MultiAgentSystem = multiagent.MultiAgentSystem
type Message = multiagent.Message
type MessageType = multiagent.MessageType
type MessageHub = multiagent.MessageHub
type AggregationStrategy = multiagent.AggregationStrategy
type WorkerResult = multiagent.WorkerResult
type AggregatedResult = multiagent.AggregatedResult
type Aggregator = multiagent.Aggregator
type FailurePolicy = multiagent.FailurePolicy
type WorkerPoolConfig = multiagent.WorkerPoolConfig
type WorkerTask = multiagent.WorkerTask
type WorkerPool = multiagent.WorkerPool
type QueryDecomposer = multiagent.QueryDecomposer
type RetrievalWorker = multiagent.RetrievalWorker
type RetrievalResultAggregator = multiagent.RetrievalResultAggregator
type RetrievalSupervisor = multiagent.RetrievalSupervisor
type DedupResultAggregator = multiagent.DedupResultAggregator
type ScopedStores = multiagent.ScopedStores
type TaskSplitter = multiagent.TaskSplitter
type SupervisorConfig = multiagent.SupervisorConfig
type Supervisor = multiagent.Supervisor
type StaticSplitter = multiagent.StaticSplitter
type RoleType = multiagent.RoleType
type RoleStatus = multiagent.RoleStatus
type RoleCapability = multiagent.RoleCapability
type RoleDefinition = multiagent.RoleDefinition
type RetryPolicy = multiagent.RetryPolicy
type RoleInstance = multiagent.RoleInstance
type RoleTransition = multiagent.RoleTransition
type RoleRegistry = multiagent.RoleRegistry
type PipelineConfig = multiagent.PipelineConfig
type RolePipeline = multiagent.RolePipeline
type RoleExecuteFunc = multiagent.RoleExecuteFunc
type HierarchicalAgent = hierarchical.HierarchicalAgent
type HierarchicalConfig = hierarchical.HierarchicalConfig
type TaskCoordinator = hierarchical.TaskCoordinator
type Task = hierarchical.Task
type TaskStatus = hierarchical.TaskStatus
type WorkerStatus = hierarchical.WorkerStatus
type AssignmentStrategy = hierarchical.AssignmentStrategy
type RoundRobinStrategy = hierarchical.RoundRobinStrategy
type LeastLoadedStrategy = hierarchical.LeastLoadedStrategy
type RandomStrategy = hierarchical.RandomStrategy
type ModeExecutor interface {
	Execute(ctx context.Context, modeName string, agents []agent.Agent, input *agent.Input) (*agent.Output, error)
}

const (
	PatternDebate    = multiagent.PatternDebate
	PatternConsensus = multiagent.PatternConsensus
	PatternPipeline  = multiagent.PatternPipeline
	PatternBroadcast = multiagent.PatternBroadcast
	PatternNetwork   = multiagent.PatternNetwork

	MessageTypeProposal  = multiagent.MessageTypeProposal
	MessageTypeResponse  = multiagent.MessageTypeResponse
	MessageTypeVote      = multiagent.MessageTypeVote
	MessageTypeConsensus = multiagent.MessageTypeConsensus
	MessageTypeBroadcast = multiagent.MessageTypeBroadcast

	StrategyMergeAll      = multiagent.StrategyMergeAll
	StrategyBestOfN       = multiagent.StrategyBestOfN
	StrategyVoteMajority  = multiagent.StrategyVoteMajority
	StrategyWeightedMerge = multiagent.StrategyWeightedMerge

	PolicyFailFast      = multiagent.PolicyFailFast
	PolicyPartialResult = multiagent.PolicyPartialResult
	PolicyRetryFailed   = multiagent.PolicyRetryFailed

	RoleCollector   = multiagent.RoleCollector
	RoleFilter      = multiagent.RoleFilter
	RoleGenerator   = multiagent.RoleGenerator
	RoleDesigner    = multiagent.RoleDesigner
	RoleImplementer = multiagent.RoleImplementer
	RoleValidator   = multiagent.RoleValidator
	RoleWriter      = multiagent.RoleWriter
	RoleCoordinator = multiagent.RoleCoordinator
	RoleCustom      = multiagent.RoleCustom

	RoleStatusIdle    = multiagent.RoleStatusIdle
	RoleStatusActive  = multiagent.RoleStatusActive
	RoleStatusBlocked = multiagent.RoleStatusBlocked
	RoleStatusDone    = multiagent.RoleStatusDone
	RoleStatusFailed  = multiagent.RoleStatusFailed

	TaskStatusPending   = hierarchical.TaskStatusPending
	TaskStatusAssigned  = hierarchical.TaskStatusAssigned
	TaskStatusRunning   = hierarchical.TaskStatusRunning
	TaskStatusCompleted = hierarchical.TaskStatusCompleted
	TaskStatusFailed    = hierarchical.TaskStatusFailed
	TaskStatusCancelled = hierarchical.TaskStatusCancelled
	WorkerStatusIdle    = hierarchical.WorkerStatusIdle
	WorkerStatusBusy    = hierarchical.WorkerStatusBusy

	ModeReasoning     = multiagent.ModeReasoning
	ModeCollaboration = multiagent.ModeCollaboration
	ModeHierarchical  = multiagent.ModeHierarchical
	ModeCrew          = multiagent.ModeCrew
	ModeDeliberation  = multiagent.ModeDeliberation
	ModeFederation    = multiagent.ModeFederation
	ModeParallel      = multiagent.ModeParallel
	ModeLoop          = multiagent.ModeLoop
)

func NewModeRegistry() *ModeRegistry { return multiagent.NewModeRegistry() }

func DefaultMultiAgentConfig() MultiAgentConfig { return multiagent.DefaultMultiAgentConfig() }

func NewMultiAgentSystem(agents []agent.Agent, config MultiAgentConfig, logger *zap.Logger) *MultiAgentSystem {
	return multiagent.NewMultiAgentSystem(agents, config, logger)
}

func NewMessageHub(logger *zap.Logger) *MessageHub { return multiagent.NewMessageHub(logger) }

func NewMessageHubWithStore(logger *zap.Logger, store persistence.MessageStore) *MessageHub {
	return multiagent.NewMessageHubWithStore(logger, store)
}

func NewAggregator(strategy AggregationStrategy) *Aggregator {
	return multiagent.NewAggregator(strategy)
}

func DefaultWorkerPoolConfig() WorkerPoolConfig { return multiagent.DefaultWorkerPoolConfig() }

func NewWorkerPool(cfg WorkerPoolConfig, logger *zap.Logger) *WorkerPool {
	return multiagent.NewWorkerPool(cfg, logger)
}

func NewRetrievalSupervisor(decomposer QueryDecomposer, workers []RetrievalWorker, aggregator RetrievalResultAggregator, logger *zap.Logger) *RetrievalSupervisor {
	return multiagent.NewRetrievalSupervisor(decomposer, workers, aggregator, logger)
}

func NewDedupResultAggregator() *DedupResultAggregator { return multiagent.NewDedupResultAggregator() }

func NewScopedStores(inner *agent.PersistenceStores, agentID string, logger *zap.Logger) *ScopedStores {
	return multiagent.NewScopedStores(inner, agentID, logger)
}

func DefaultSupervisorConfig() SupervisorConfig { return multiagent.DefaultSupervisorConfig() }

func NewSupervisor(splitter TaskSplitter, cfg SupervisorConfig, logger *zap.Logger) *Supervisor {
	return multiagent.NewSupervisor(splitter, cfg, logger)
}

func NewRoleRegistry(logger *zap.Logger) *RoleRegistry { return multiagent.NewRoleRegistry(logger) }

func DefaultPipelineConfig() PipelineConfig { return multiagent.DefaultPipelineConfig() }

func NewRolePipeline(config PipelineConfig, registry *RoleRegistry, executeFn RoleExecuteFunc, logger *zap.Logger) *RolePipeline {
	return multiagent.NewRolePipeline(config, registry, executeFn, logger)
}

func RegisterResearchRoles(registry *RoleRegistry) error {
	return multiagent.RegisterResearchRoles(registry)
}

func DefaultHierarchicalConfig() HierarchicalConfig { return hierarchical.DefaultHierarchicalConfig() }

func NewHierarchicalAgent(baseAgent *agent.BaseAgent, supervisor agent.Agent, workers []agent.Agent, config HierarchicalConfig, logger *zap.Logger) *HierarchicalAgent {
	return hierarchical.NewHierarchicalAgent(baseAgent, supervisor, workers, config, logger)
}

func NewTaskCoordinator(workers []agent.Agent, config HierarchicalConfig, logger *zap.Logger) *TaskCoordinator {
	return hierarchical.NewTaskCoordinator(workers, config, logger)
}

func RegisterDefaultModes(reg *ModeRegistry, logger *zap.Logger) error {
	return multiagent.RegisterDefaultModes(reg, logger)
}

func GlobalModeRegistry() *ModeRegistry { return multiagent.GlobalModeRegistry() }

func GlobalModeExecutor() ModeExecutor { return multiagent.GlobalModeRegistry() }

func NewInMemorySharedState() *InMemorySharedState { return multiagent.NewInMemorySharedState() }

func NewInMemorySharedStateWithLogger(logger *zap.Logger) *InMemorySharedState {
	return multiagent.NewInMemorySharedStateWithLogger(logger)
}

// teamModeStrategy is the internal interface for mode-specific execution logic.
type teamModeStrategy interface {
	Execute(ctx context.Context, members []agent.TeamMember, task string, config TeamConfig, opts agent.TeamOptions) (*agent.Output, error)
}

// ID returns the team's unique identifier.
func (t *AgentTeam) ID() string { return t.id }

// Members returns the team's members.
func (t *AgentTeam) Members() []agent.TeamMember { return t.members }

// Execute runs the team on the given task using the configured mode strategy.
func (t *AgentTeam) Execute(ctx context.Context, task string, opts ...agent.TeamOption) (*agent.TeamResult, error) {
	o := &agent.TeamOptions{
		MaxRounds: t.config.MaxRounds,
		Timeout:   t.config.Timeout,
	}
	for _, fn := range opts {
		fn(o)
	}

	if o.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, o.Timeout)
		defer cancel()
	}

	t.logger.Info("team executing",
		zap.String("team_id", t.id),
		zap.String("mode", string(t.mode)),
		zap.Int("members", len(t.members)),
		zap.String("task_preview", truncateStr(task, 80)),
	)

	start := time.Now()
	out, err := t.strategy.Execute(ctx, t.members, task, t.config, *o)
	if err != nil {
		return nil, fmt.Errorf("team %s execution failed: %w", t.id, err)
	}

	result := &agent.TeamResult{
		Content:    out.Content,
		TokensUsed: out.TokensUsed,
		Cost:       out.Cost,
		Duration:   time.Since(start),
		Metadata:   out.Metadata,
	}
	if result.Metadata == nil {
		result.Metadata = make(map[string]any)
	}
	result.Metadata["team_id"] = t.id
	result.Metadata["team_mode"] = string(t.mode)

	t.logger.Info("team execution completed",
		zap.String("team_id", t.id),
		zap.Duration("duration", result.Duration),
	)
	return result, nil
}

func truncateStr(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
