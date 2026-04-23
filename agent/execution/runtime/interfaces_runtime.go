package runtime

import (
	"context"
	"encoding/json"
	"fmt"
	memorycore "github.com/BaSui01/agentflow/agent/capabilities/memory"
	promptcap "github.com/BaSui01/agentflow/agent/capabilities/prompt"
	"github.com/BaSui01/agentflow/agent/capabilities/reasoning"
	skills "github.com/BaSui01/agentflow/agent/capabilities/tools"
	agentcore "github.com/BaSui01/agentflow/agent/core"
	agentcontext "github.com/BaSui01/agentflow/agent/execution/context"
	executionloop "github.com/BaSui01/agentflow/agent/execution/loop"
	agentfeatures "github.com/BaSui01/agentflow/agent/integration"
	agentevents "github.com/BaSui01/agentflow/agent/observability/events"
	agentpersistence "github.com/BaSui01/agentflow/agent/persistence"
	agentcheckpoint "github.com/BaSui01/agentflow/agent/persistence/checkpoint"
	checkpointcore "github.com/BaSui01/agentflow/agent/persistence/checkpoint/core"
	llmtools "github.com/BaSui01/agentflow/llm/capabilities/tools"
	llm "github.com/BaSui01/agentflow/llm/core"
	"github.com/BaSui01/agentflow/types"
	"go.uber.org/zap"
	"sort"
	"strings"
	"sync"
	"time"
)

// =============================================================================
// Workflow-Local Interfaces for Optional Agent Features
// =============================================================================
// These interfaces break circular dependencies between agent/ and its sub-packages
// (agent/skills, agent/protocol/mcp, agent/lsp, agent/memory, agent/observability).
//
// Each interface declares ONLY the methods that agent/ actually calls via type
// assertions in integration.go. The concrete implementations in sub-packages
// implicitly satisfy these interfaces (Go duck typing).
//
// See quality-guidelines.md section 15 for the pattern rationale.
// =============================================================================

// ReflectionRunner executes a task with iterative self-reflection.
// Implemented by: *ReflectionExecutor (agent/reflection.go)
type ReflectionRunner interface {
	ExecuteWithReflection(ctx context.Context, input *Input) (*Output, error)
}

// DynamicToolSelectorRunner dynamically selects tools relevant to a given task.
// Implemented by: *DynamicToolSelector (agent/tool_selector.go)
type DynamicToolSelectorRunner interface {
	SelectTools(ctx context.Context, task string, availableTools []types.ToolSchema) ([]types.ToolSchema, error)
}

// PromptEnhancerRunner enhances user prompts with additional context.
// Implemented by: *PromptEnhancer (agent/prompt_enhancer.go)
type PromptEnhancerRunner interface {
	EnhanceUserPrompt(prompt, context string) (string, error)
}

// SkillDiscoverer discovers skills relevant to a task.
// Implemented by: *skills.DefaultSkillManager (agent/skills/)
type SkillDiscoverer interface {
	DiscoverSkills(ctx context.Context, task string) ([]*types.DiscoveredSkill, error)
}

// MCPServerRunner represents an MCP server instance.
// Implemented by: *mcp.MCPServer (agent/protocol/mcp/)
// Currently used only for nil-check (feature status); no methods called directly.
type MCPServerRunner interface{}

// LSPClientRunner represents an LSP client instance.
// Implemented by: *lsp.LSPClient (agent/lsp/)
// Used in Teardown for Shutdown call.
type LSPClientRunner interface {
	Shutdown(ctx context.Context) error
}

// LSPLifecycleOwner represents an optional lifecycle owner for LSP (e.g. *ManagedLSP).
// Used in Teardown for Close call.
type LSPLifecycleOwner interface {
	Close() error
}

// EnhancedMemoryRunner provides advanced memory capabilities.
// Implemented by: *memory.EnhancedMemorySystem (agent/memory/)
type EnhancedMemoryRunner interface {
	LoadWorking(ctx context.Context, agentID string) ([]types.MemoryEntry, error)
	LoadShortTerm(ctx context.Context, agentID string, limit int) ([]types.MemoryEntry, error)
	SaveShortTerm(ctx context.Context, agentID, content string, metadata map[string]any) error
	RecordEpisode(ctx context.Context, event *types.EpisodicEvent) error
}

// ObservabilityRunner provides metrics, tracing, and logging.
// Implemented by: *observability.ObservabilitySystem (agent/observability/)
type ObservabilityRunner interface {
	StartTrace(traceID, agentID string)
	EndTrace(traceID, status string, err error)
	RecordTask(agentID string, success bool, duration time.Duration, tokens int, cost, quality float64)
}

// ExplainabilityRecorder is an optional observability extension for recording
// structured reasoning steps against the execution trace.
type ExplainabilityRecorder interface {
	StartExplainabilityTrace(traceID, sessionID, agentID string)
	AddExplainabilityStep(traceID, stepType, content string, metadata map[string]any)
	EndExplainabilityTrace(traceID string, success bool, output, errorMsg string)
}

// ExplainabilityTimelineRecorder is an optional extension for recording
// high-level decision timeline entries alongside low-level reasoning steps.
type ExplainabilityTimelineRecorder interface {
	AddExplainabilityTimeline(traceID, entryType, summary string, metadata map[string]any)
}

// ExplainabilitySynopsisReader is an optional extension for reading the latest
// completed synopsis for an agent/session so it can be fed back into runtime.
type ExplainabilitySynopsisReader interface {
	GetLatestExplainabilitySynopsis(sessionID, agentID, excludeTraceID string) string
}

type ExplainabilitySynopsisSnapshot = agentcontext.ExplainabilitySynopsisSnapshot

// ExplainabilitySynopsisSnapshotReader is an optional richer reader that
// returns both the short synopsis and compressed long-history summary.
type ExplainabilitySynopsisSnapshotReader interface {
	GetLatestExplainabilitySynopsisSnapshot(sessionID, agentID, excludeTraceID string) ExplainabilitySynopsisSnapshot
}

type MemoryRecallOptions = memorycore.MemoryRecallOptions

type MemoryObservationInput = memorycore.MemoryObservationInput

type MemoryRuntime = memorycore.MemoryRuntime

type UnifiedMemoryFacade = memorycore.UnifiedMemoryFacade

type PromptBundle = promptcap.PromptBundle

type SystemPrompt = promptcap.SystemPrompt

type Example = promptcap.Example

type MemoryConfig = promptcap.MemoryConfig

type PlanConfig = promptcap.PlanConfig

type ReflectionConfig = promptcap.ReflectionConfig

type PromptEnhancerConfig = promptcap.PromptEnhancerConfig

type PromptEnhancer = promptcap.PromptEnhancer

type EnhancedExecutionOptions = agentfeatures.EnhancedExecutionOptions

type RuntimeStreamEventType = agentevents.RuntimeStreamEventType

type SDKStreamEventType = agentevents.SDKStreamEventType

type SDKRunItemEventName = agentevents.SDKRunItemEventName

type RuntimeToolCall = agentevents.RuntimeToolCall

type RuntimeToolResult = agentevents.RuntimeToolResult

type RuntimeStreamEvent = agentevents.RuntimeStreamEvent

type RuntimeStreamEmitter = agentevents.RuntimeStreamEmitter

const (
	RuntimeStreamToken        = agentevents.RuntimeStreamToken
	RuntimeStreamReasoning    = agentevents.RuntimeStreamReasoning
	RuntimeStreamToolCall     = agentevents.RuntimeStreamToolCall
	RuntimeStreamToolResult   = agentevents.RuntimeStreamToolResult
	RuntimeStreamToolProgress = agentevents.RuntimeStreamToolProgress
	RuntimeStreamApproval     = agentevents.RuntimeStreamApproval
	RuntimeStreamSession      = agentevents.RuntimeStreamSession
	RuntimeStreamStatus       = agentevents.RuntimeStreamStatus
	RuntimeStreamSteering     = agentevents.RuntimeStreamSteering
	RuntimeStreamStopAndSend  = agentevents.RuntimeStreamStopAndSend
)

const (
	SDKRawResponseEvent  = agentevents.SDKRawResponseEvent
	SDKRunItemEvent      = agentevents.SDKRunItemEvent
	SDKAgentUpdatedEvent = agentevents.SDKAgentUpdatedEvent
)

const (
	SDKMessageOutputCreated = agentevents.SDKMessageOutputCreated
	SDKHandoffRequested     = agentevents.SDKHandoffRequested
	SDKToolCalled           = agentevents.SDKToolCalled
	SDKToolSearchCalled     = agentevents.SDKToolSearchCalled
	SDKToolSearchOutput     = agentevents.SDKToolSearchOutput
	SDKToolOutput           = agentevents.SDKToolOutput
	SDKReasoningCreated     = agentevents.SDKReasoningCreated
	SDKApprovalRequested    = agentevents.SDKApprovalRequested
	SDKApprovalResponse     = agentevents.SDKApprovalResponse
	SDKMCPApprovalRequested = agentevents.SDKMCPApprovalRequested
	SDKMCPApprovalResponse  = agentevents.SDKMCPApprovalResponse
	SDKMCPListTools         = agentevents.SDKMCPListTools
)

var SDKHandoffOccured = agentevents.SDKHandoffOccured

type PromptTemplateLibrary = promptcap.PromptTemplateLibrary

type PromptTemplate = promptcap.PromptTemplate

type DefensivePromptConfig = promptcap.DefensivePromptConfig

type FailureMode = promptcap.FailureMode

type OutputSchema = promptcap.OutputSchema

type GuardRail = promptcap.GuardRail

type InjectionDefenseConfig = promptcap.InjectionDefenseConfig

type DefensivePromptEnhancer = promptcap.DefensivePromptEnhancer

type CheckpointDiff = agentcheckpoint.CheckpointDiff

type Checkpoint = agentcheckpoint.Checkpoint

type CheckpointMessage = agentcheckpoint.CheckpointMessage

type CheckpointToolCall = agentcheckpoint.CheckpointToolCall

type ExecutionContext = agentcheckpoint.ExecutionContext

type CheckpointVersion = agentcheckpoint.CheckpointVersion

type CheckpointStore = agentcheckpoint.Store

type CheckpointManager struct {
	inner  *agentcheckpoint.Manager
	store  CheckpointStore
	logger *zap.Logger

	autoSaveEnabled  bool
	autoSaveInterval time.Duration
	autoSaveCancel   context.CancelFunc
	autoSaveDone     chan struct{}
	autoSaveMu       sync.Mutex
}

func NewPromptBundleFromIdentity(version, identity string) PromptBundle {
	return promptcap.NewPromptBundleFromIdentity(version, identity)
}

func DefaultPromptEnhancerConfig() *PromptEnhancerConfig {
	return promptcap.DefaultPromptEnhancerConfig()
}

func NewPromptEnhancer(config PromptEnhancerConfig) *PromptEnhancer {
	return promptcap.NewPromptEnhancer(config)
}

type PromptOptimizer struct{}

func NewPromptOptimizer() *PromptOptimizer {
	return &PromptOptimizer{}
}

func NewPromptTemplateLibrary() *PromptTemplateLibrary {
	return promptcap.NewPromptTemplateLibrary()
}

func DefaultDefensivePromptConfig() DefensivePromptConfig {
	return promptcap.DefaultDefensivePromptConfig()
}

func DefaultEnhancedExecutionOptions() EnhancedExecutionOptions {
	return agentfeatures.DefaultEnhancedExecutionOptions()
}

func WithRuntimeStreamEmitter(ctx context.Context, emit RuntimeStreamEmitter) context.Context {
	return agentevents.WithRuntimeStreamEmitter(ctx, emit)
}

func runtimeStreamEmitterFromContext(ctx context.Context) (RuntimeStreamEmitter, bool) {
	return agentevents.RuntimeStreamEmitterFromContext(ctx)
}

func emitRuntimeStatus(emit RuntimeStreamEmitter, status string, event RuntimeStreamEvent) {
	agentevents.EmitRuntimeStatus(emit, status, event)
}

// CompletionJudge decides whether the loop can stop or must continue.
type CompletionJudge interface {
	Judge(ctx context.Context, state *LoopState, output *Output, err error) (*CompletionDecision, error)
}

type LoopValidationStatus = agentcore.LoopValidationStatus

const (
	LoopValidationStatusPassed  LoopValidationStatus = agentcore.LoopValidationStatusPassed
	LoopValidationStatusPending LoopValidationStatus = agentcore.LoopValidationStatusPending
	LoopValidationStatusFailed  LoopValidationStatus = agentcore.LoopValidationStatusFailed
)

type LoopValidationIssue struct {
	Validator string               `json:"validator,omitempty"`
	Code      string               `json:"code,omitempty"`
	Category  string               `json:"category,omitempty"`
	Status    LoopValidationStatus `json:"status,omitempty"`
	Message   string               `json:"message,omitempty"`
}

type LoopValidationResult struct {
	Status             LoopValidationStatus  `json:"status,omitempty"`
	Passed             bool                  `json:"passed"`
	Pending            bool                  `json:"pending,omitempty"`
	Reason             string                `json:"reason,omitempty"`
	Summary            string                `json:"summary,omitempty"`
	Issues             []LoopValidationIssue `json:"issues,omitempty"`
	AcceptanceCriteria []string              `json:"acceptance_criteria,omitempty"`
	UnresolvedItems    []string              `json:"unresolved_items,omitempty"`
	RemainingRisks     []string              `json:"remaining_risks,omitempty"`
	Metadata           map[string]any        `json:"metadata,omitempty"`
}

type LoopValidator interface {
	Validate(ctx context.Context, input *Input, state *LoopState, output *Output, err error) (*LoopValidationResult, error)
}

type LoopValidatorChain struct {
	validators []LoopValidator
}

func NewLoopValidatorChain(validators ...LoopValidator) *LoopValidatorChain {
	filtered := make([]LoopValidator, 0, len(validators))
	for _, validator := range validators {
		if validator != nil {
			filtered = append(filtered, validator)
		}
	}
	return &LoopValidatorChain{validators: filtered}
}

func (c *LoopValidatorChain) Validate(ctx context.Context, input *Input, state *LoopState, output *Output, err error) (*LoopValidationResult, error) {
	aggregate := newLoopValidationResult(LoopValidationStatusPassed, "validation passed")
	aggregate.AcceptanceCriteria = acceptanceCriteriaForValidation(input, state)
	for _, validator := range c.validators {
		result, validateErr := validator.Validate(ctx, input, state, output, err)
		if validateErr != nil {
			return nil, validateErr
		}
		mergeLoopValidationResult(aggregate, result)
	}
	finalizeLoopValidationResult(aggregate)
	return aggregate, nil
}

// DefaultCompletionJudge implements the unified stop semantics.
type DefaultCompletionJudge struct{}

func NewDefaultCompletionJudge() *DefaultCompletionJudge { return &DefaultCompletionJudge{} }

func (j *DefaultCompletionJudge) Judge(ctx context.Context, state *LoopState, output *Output, err error) (*CompletionDecision, error) {
	decision, judgeErr := executionloop.JudgeDefault(ctx, loopExecutionStateFromRoot(state), loopExecutionOutputFromRoot(output), err)
	if judgeErr != nil {
		return nil, judgeErr
	}
	return rootCompletionDecisionFromLoop(decision), nil
}

type DefaultLoopValidator struct {
	chain *LoopValidatorChain
}

func NewDefaultLoopValidator() *DefaultLoopValidator {
	return &DefaultLoopValidator{
		chain: NewLoopValidatorChain(
			GenericLoopValidator{},
			ToolVerificationLoopValidator{},
			NewCodeTaskLoopValidator(),
		),
	}
}

func (v *DefaultLoopValidator) Validate(ctx context.Context, input *Input, state *LoopState, output *Output, err error) (*LoopValidationResult, error) {
	if v == nil || v.chain == nil {
		return newLoopValidationResult(LoopValidationStatusPassed, "validation passed"), nil
	}
	return v.chain.Validate(ctx, input, state, output, err)
}

type GenericLoopValidator struct{}

func (GenericLoopValidator) Validate(_ context.Context, input *Input, state *LoopState, output *Output, err error) (*LoopValidationResult, error) {
	return rootLoopValidationResultFromLoop(executionloop.ValidateGeneric(loopExecutionInputFromRoot(input), loopExecutionStateFromRoot(state), loopExecutionOutputFromRoot(output), err)), nil
}

type ToolVerificationLoopValidator struct{}

func (ToolVerificationLoopValidator) Validate(_ context.Context, input *Input, state *LoopState, output *Output, err error) (*LoopValidationResult, error) {
	return rootLoopValidationResultFromLoop(executionloop.ValidateToolVerification(loopExecutionInputFromRoot(input), loopExecutionStateFromRoot(state), loopExecutionOutputFromRoot(output), err)), nil
}

type CodeTaskLoopValidator struct {
	codeValidator *CodeValidator
}

func NewCodeTaskLoopValidator() CodeTaskLoopValidator {
	return CodeTaskLoopValidator{codeValidator: NewCodeValidator()}
}

func (v CodeTaskLoopValidator) Validate(_ context.Context, input *Input, state *LoopState, output *Output, err error) (*LoopValidationResult, error) {
	return rootLoopValidationResultFromLoop(executionloop.ValidateCodeTask(loopCodeWarningProviderAdapter{validator: v.codeValidator}, loopExecutionInputFromRoot(input), loopExecutionStateFromRoot(state), loopExecutionOutputFromRoot(output), err)), nil
}

func newLoopValidationResult(status LoopValidationStatus, reason string) *LoopValidationResult {
	return rootLoopValidationResultFromLoop(executionloop.NewValidationResult(loopValidationStatusToSubpkg(status), reason))
}

func mergeLoopValidationResult(target *LoopValidationResult, incoming *LoopValidationResult) {
	if target == nil || incoming == nil {
		return
	}
	merged := rootLoopValidationResultToSubpkg(target)
	executionloop.MergeValidationResult(merged, rootLoopValidationResultToSubpkg(incoming))
	*target = *rootLoopValidationResultFromLoop(merged)
}

func finalizeLoopValidationResult(result *LoopValidationResult) {
	if result == nil {
		return
	}
	normalized := rootLoopValidationResultToSubpkg(result)
	executionloop.FinalizeValidationResult(normalized)
	*result = *rootLoopValidationResultFromLoop(normalized)
}

func acceptanceCriteriaForValidation(input *Input, state *LoopState) []string {
	return executionloop.AcceptanceCriteriaForValidation(loopExecutionInputFromRoot(input), loopExecutionStateFromRoot(state))
}

func toolVerificationRequired(input *Input, state *LoopState, output *Output) bool {
	return executionloop.ToolVerificationRequired(loopExecutionInputFromRoot(input), loopExecutionStateFromRoot(state), loopExecutionOutputFromRoot(output))
}

func codeTaskRequired(input *Input, state *LoopState, output *Output) bool {
	return executionloop.CodeTaskRequired(loopExecutionInputFromRoot(input), loopExecutionStateFromRoot(state), loopExecutionOutputFromRoot(output))
}

func classifyStopReason(msg string) StopReason {
	return StopReason(executionloop.ClassifyStopReason(msg))
}

func appendUniqueString(values []string, value string) []string {
	return agentcore.AppendUniqueString(values, value)
}

func fallbackString(values ...string) string {
	return agentcore.FallbackString(values...)
}

type loopCodeWarningProviderAdapter struct {
	validator *CodeValidator
}

func (a loopCodeWarningProviderAdapter) Validate(lang executionloop.CodeValidationLanguage, code string) []string {
	if a.validator == nil {
		return nil
	}
	return a.validator.Validate(CodeValidationLanguage(lang), code)
}

func loopExecutionInputFromRoot(input *Input) *executionloop.Input {
	if input == nil {
		return nil
	}
	var contextMap map[string]any
	if input.Context != nil {
		contextMap = mapsClone(input.Context)
	}
	return &executionloop.Input{
		Content: input.Content,
		Context: contextMap,
	}
}

func loopExecutionOutputFromRoot(output *Output) *executionloop.Output {
	if output == nil {
		return nil
	}
	return &executionloop.Output{
		Content:  output.Content,
		Metadata: cloneMetadata(output.Metadata),
	}
}

func loopExecutionStateFromRoot(state *LoopState) *executionloop.State {
	if state == nil {
		return nil
	}
	return &executionloop.State{
		Goal:               state.Goal,
		CurrentStage:       string(state.CurrentStage),
		Iteration:          state.Iteration,
		MaxIterations:      state.MaxIterations,
		Decision:           string(state.Decision),
		StopReason:         string(state.StopReason),
		PlanSteps:          cloneStringSlice(state.Plan),
		NeedHuman:          state.NeedHuman,
		Confidence:         state.Confidence,
		ValidationStatus:   loopValidationStatusToSubpkg(state.ValidationStatus),
		ValidationSummary:  state.ValidationSummary,
		AcceptanceCriteria: cloneStringSlice(state.AcceptanceCriteria),
		UnresolvedItems:    cloneStringSlice(state.UnresolvedItems),
		RemainingRisks:     cloneStringSlice(state.RemainingRisks),
		SelectedMode:       state.SelectedReasoningMode,
	}
}

func loopValidationStatusToSubpkg(status LoopValidationStatus) executionloop.ValidationStatus {
	return executionloop.ValidationStatus(status)
}

func rootLoopValidationStatusFromSubpkg(status executionloop.ValidationStatus) LoopValidationStatus {
	return LoopValidationStatus(status)
}

func rootCompletionDecisionFromLoop(decision *executionloop.CompletionDecision) *CompletionDecision {
	if decision == nil {
		return nil
	}
	return &CompletionDecision{
		Solved:         decision.Solved,
		NeedReplan:     decision.NeedReplan,
		NeedReflection: decision.NeedReflection,
		NeedHuman:      decision.NeedHuman,
		Decision:       LoopDecision(decision.Decision),
		StopReason:     StopReason(decision.StopReason),
		Confidence:     decision.Confidence,
		Reason:         decision.Reason,
	}
}

func rootLoopValidationResultFromLoop(result *executionloop.ValidationResult) *LoopValidationResult {
	if result == nil {
		return nil
	}
	issues := make([]LoopValidationIssue, 0, len(result.Issues))
	for _, issue := range result.Issues {
		issues = append(issues, LoopValidationIssue{
			Validator: issue.Validator,
			Code:      issue.Code,
			Category:  issue.Category,
			Status:    rootLoopValidationStatusFromSubpkg(issue.Status),
			Message:   issue.Message,
		})
	}
	metadata := map[string]any{}
	for key, value := range result.Metadata {
		switch typed := value.(type) {
		case []executionloop.ValidationIssue:
			mapped := make([]LoopValidationIssue, 0, len(typed))
			for _, issue := range typed {
				mapped = append(mapped, LoopValidationIssue{
					Validator: issue.Validator,
					Code:      issue.Code,
					Category:  issue.Category,
					Status:    rootLoopValidationStatusFromSubpkg(issue.Status),
					Message:   issue.Message,
				})
			}
			metadata[key] = mapped
		default:
			metadata[key] = value
		}
	}
	return &LoopValidationResult{
		Status:             rootLoopValidationStatusFromSubpkg(result.Status),
		Passed:             result.Passed,
		Pending:            result.Pending,
		Reason:             result.Reason,
		Summary:            result.Summary,
		Issues:             issues,
		AcceptanceCriteria: cloneStringSlice(result.AcceptanceCriteria),
		UnresolvedItems:    cloneStringSlice(result.UnresolvedItems),
		RemainingRisks:     cloneStringSlice(result.RemainingRisks),
		Metadata:           metadata,
	}
}

func rootLoopValidationResultToSubpkg(result *LoopValidationResult) *executionloop.ValidationResult {
	if result == nil {
		return nil
	}
	issues := make([]executionloop.ValidationIssue, 0, len(result.Issues))
	for _, issue := range result.Issues {
		issues = append(issues, executionloop.ValidationIssue{
			Validator: issue.Validator,
			Code:      issue.Code,
			Category:  issue.Category,
			Status:    loopValidationStatusToSubpkg(issue.Status),
			Message:   issue.Message,
		})
	}
	metadata := map[string]any{}
	for key, value := range result.Metadata {
		switch typed := value.(type) {
		case []LoopValidationIssue:
			mapped := make([]executionloop.ValidationIssue, 0, len(typed))
			for _, issue := range typed {
				mapped = append(mapped, executionloop.ValidationIssue{
					Validator: issue.Validator,
					Code:      issue.Code,
					Category:  issue.Category,
					Status:    loopValidationStatusToSubpkg(issue.Status),
					Message:   issue.Message,
				})
			}
			metadata[key] = mapped
		default:
			metadata[key] = value
		}
	}
	return &executionloop.ValidationResult{
		Status:             loopValidationStatusToSubpkg(result.Status),
		Passed:             result.Passed,
		Pending:            result.Pending,
		Reason:             result.Reason,
		Summary:            result.Summary,
		Issues:             issues,
		AcceptanceCriteria: cloneStringSlice(result.AcceptanceCriteria),
		UnresolvedItems:    cloneStringSlice(result.UnresolvedItems),
		RemainingRisks:     cloneStringSlice(result.RemainingRisks),
		Metadata:           metadata,
	}
}

func mapsClone(src map[string]any) map[string]any {
	if len(src) == 0 {
		return nil
	}
	dst := make(map[string]any, len(src))
	for key, value := range src {
		dst[key] = value
	}
	return dst
}

type LoopValidationFuncAdapter LoopValidationFunc

func (f LoopValidationFuncAdapter) Validate(ctx context.Context, input *Input, state *LoopState, output *Output, err error) (*LoopValidationResult, error) {
	return f(ctx, input, state, output, err)
}

func NewDefensivePromptEnhancer(config DefensivePromptConfig) *DefensivePromptEnhancer {
	return promptcap.NewDefensivePromptEnhancer(config)
}

func NewCheckpointManager(store CheckpointStore, logger *zap.Logger) *CheckpointManager {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &CheckpointManager{
		inner:  agentcheckpoint.NewManager(store, logger),
		store:  store,
		logger: logger.With(zap.String("component", "checkpoint_manager")),
	}
}

func NewCheckpointManagerFromNativeStore(store agentcheckpoint.Store, logger *zap.Logger) *CheckpointManager {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &CheckpointManager{
		inner:  agentcheckpoint.NewManager(store, logger),
		logger: logger.With(zap.String("component", "checkpoint_manager")),
	}
}

func GenerateCheckpointID() string {
	return agentcheckpoint.GenerateID()
}

func generateCheckpointID() string {
	return GenerateCheckpointID()
}

func (m *CheckpointManager) SaveCheckpoint(ctx context.Context, checkpoint *Checkpoint) error {
	return m.ensureInner().SaveCheckpoint(ctx, checkpoint)
}

func (m *CheckpointManager) LoadCheckpoint(ctx context.Context, checkpointID string) (*Checkpoint, error) {
	return m.ensureInner().LoadCheckpoint(ctx, checkpointID)
}

func (m *CheckpointManager) LoadLatestCheckpoint(ctx context.Context, threadID string) (*Checkpoint, error) {
	return m.ensureInner().LoadLatestCheckpoint(ctx, threadID)
}

func (m *CheckpointManager) ResumeFromCheckpoint(ctx context.Context, agent Agent, checkpointID string) error {
	_, err := m.LoadCheckpointForAgent(ctx, agent, checkpointID)
	return err
}

func (m *CheckpointManager) LoadCheckpointForAgent(ctx context.Context, agent Agent, checkpointID string) (*Checkpoint, error) {
	checkpoint, err := m.LoadCheckpoint(ctx, checkpointID)
	if err != nil {
		return nil, err
	}
	if err := m.restoreAgentFromCheckpoint(ctx, agent, checkpoint); err != nil {
		return nil, err
	}
	return checkpoint, nil
}

func (m *CheckpointManager) LoadLatestCheckpointForAgent(ctx context.Context, agent Agent, threadID string) (*Checkpoint, error) {
	checkpoint, err := m.LoadLatestCheckpoint(ctx, threadID)
	if err != nil {
		return nil, err
	}
	if err := m.restoreAgentFromCheckpoint(ctx, agent, checkpoint); err != nil {
		return nil, err
	}
	return checkpoint, nil
}

func (m *CheckpointManager) restoreAgentFromCheckpoint(ctx context.Context, agent Agent, checkpoint *Checkpoint) error {
	if checkpoint == nil {
		return fmt.Errorf("checkpoint is nil")
	}
	m.loggerOrNop().Info("resuming from checkpoint",
		zap.String("checkpoint_id", checkpoint.ID),
		zap.String("agent_id", checkpoint.AgentID),
		zap.String("state", string(checkpoint.State)),
	)

	if agent.ID() != checkpoint.AgentID {
		return fmt.Errorf("agent ID mismatch: expected %s, got %s", checkpoint.AgentID, agent.ID())
	}

	type transitioner interface {
		Transition(ctx context.Context, newState State) error
	}
	if t, ok := agent.(transitioner); ok {
		if err := t.Transition(ctx, State(checkpoint.State)); err != nil {
			return fmt.Errorf("failed to restore state: %w", err)
		}
	}

	m.loggerOrNop().Info("checkpoint restored successfully")
	return nil
}

func (m *CheckpointManager) EnableAutoSave(ctx context.Context, agent Agent, threadID string, interval time.Duration) error {
	m.autoSaveMu.Lock()
	defer m.autoSaveMu.Unlock()

	if m.autoSaveEnabled {
		return fmt.Errorf("auto-save already enabled")
	}
	if interval <= 0 {
		return fmt.Errorf("invalid interval: must be positive")
	}

	m.autoSaveInterval = interval
	m.autoSaveEnabled = true

	autoSaveCtx, cancel := context.WithCancel(ctx)
	m.autoSaveCancel = cancel
	m.autoSaveDone = make(chan struct{})

	go m.autoSaveLoop(autoSaveCtx, m.autoSaveDone, agent, threadID)

	m.loggerOrNop().Info("auto-save enabled",
		zap.Duration("interval", interval),
		zap.String("thread_id", threadID),
	)
	return nil
}

func (m *CheckpointManager) DisableAutoSave() {
	m.autoSaveMu.Lock()
	if !m.autoSaveEnabled {
		m.autoSaveMu.Unlock()
		return
	}

	cancel := m.autoSaveCancel
	done := m.autoSaveDone
	m.autoSaveCancel = nil
	m.autoSaveDone = nil
	m.autoSaveEnabled = false
	m.autoSaveMu.Unlock()

	if cancel != nil {
		cancel()
	}
	if done != nil {
		<-done
	}

	m.loggerOrNop().Info("auto-save disabled")
}

func (m *CheckpointManager) autoSaveLoop(ctx context.Context, done chan struct{}, agent Agent, threadID string) {
	ticker := time.NewTicker(m.autoSaveInterval)
	defer func() {
		ticker.Stop()
		close(done)
	}()

	for {
		select {
		case <-ctx.Done():
			m.loggerOrNop().Debug("auto-save loop stopped")
			return
		case <-ticker.C:
			if err := m.CreateCheckpoint(ctx, agent, threadID); err != nil {
				m.loggerOrNop().Error("auto-save failed", zap.Error(err))
			} else {
				m.loggerOrNop().Debug("auto-save checkpoint created", zap.String("thread_id", threadID))
			}
		}
	}
}

func (m *CheckpointManager) CreateCheckpoint(ctx context.Context, agent Agent, threadID string) error {
	_, err := m.ensureInner().CreateCheckpoint(ctx, threadID, agent.ID(), string(agent.State()))
	return err
}

func (m *CheckpointManager) RollbackToVersion(ctx context.Context, agent Agent, threadID string, version int) error {
	m.loggerOrNop().Info("rolling back to version",
		zap.String("thread_id", threadID),
		zap.Int("version", version),
	)

	checkpoint, err := m.ensureInner().LoadVersion(ctx, threadID, version)
	if err != nil {
		return err
	}
	if err := m.restoreAgentFromCheckpoint(ctx, agent, checkpoint); err != nil {
		return err
	}
	if err := m.ensureInner().RollbackToVersion(ctx, threadID, version); err != nil {
		return err
	}

	m.loggerOrNop().Info("rollback completed",
		zap.String("thread_id", threadID),
		zap.Int("version", version),
	)
	return nil
}

func (m *CheckpointManager) CompareVersions(ctx context.Context, threadID string, version1, version2 int) (*CheckpointDiff, error) {
	return m.ensureInner().CompareVersions(ctx, threadID, version1, version2)
}

func (m *CheckpointManager) ListVersions(ctx context.Context, threadID string) ([]CheckpointVersion, error) {
	return m.ensureInner().ListVersions(ctx, threadID)
}

func (m *CheckpointManager) compareMessages(msgs1, msgs2 []CheckpointMessage) string {
	return checkpointcore.CompareMessageCounts(len(msgs1), len(msgs2))
}

func (m *CheckpointManager) compareMetadata(meta1, meta2 map[string]any) string {
	return checkpointcore.CompareMetadata(meta1, meta2)
}

func (m *CheckpointManager) ensureInner() *agentcheckpoint.Manager {
	if m.inner == nil {
		m.inner = agentcheckpoint.NewManager(m.store, m.loggerOrNop())
	}
	return m.inner
}

func (m *CheckpointManager) loggerOrNop() *zap.Logger {
	if m != nil && m.logger != nil {
		return m.logger
	}
	return zap.NewNop()
}

func (o *PromptOptimizer) OptimizePrompt(prompt string) string {
	optimized := prompt
	if len(prompt) < 20 {
		optimized = o.makeMoreSpecific(optimized)
	}
	if !o.hasTaskDescription(optimized) {
		optimized = o.addTaskDescription(optimized)
	}
	if !o.hasConstraints(optimized) {
		optimized = o.addBasicConstraints(optimized)
	}
	return optimized
}

func (o *PromptOptimizer) makeMoreSpecific(prompt string) string {
	return promptcap.MakeMoreSpecific(prompt)
}

func (o *PromptOptimizer) hasTaskDescription(prompt string) bool {
	return promptcap.HasTaskDescription(prompt)
}

func (o *PromptOptimizer) addTaskDescription(prompt string) string {
	return promptcap.AddTaskDescription(prompt)
}

func (o *PromptOptimizer) hasConstraints(prompt string) bool {
	return promptcap.HasConstraints(prompt)
}

func (o *PromptOptimizer) addBasicConstraints(prompt string) string {
	return fmt.Sprintf("%s\n\n要求：\n- 回答要准确、完整\n- 使用清晰的语言\n- 提供必要的解释", prompt)
}

func formatBulletSection(title string, items []string) string {
	return promptcap.FormatBulletSection(title, items)
}

func replaceTemplateVars(text string, vars map[string]string) string {
	return promptcap.ReplaceTemplateVars(text, vars)
}

func NewUnifiedMemoryFacade(base MemoryManager, enhanced EnhancedMemoryRunner, logger *zap.Logger) *UnifiedMemoryFacade {
	return memorycore.NewUnifiedMemoryFacade(base, enhanced, logger)
}

func NewDefaultMemoryRuntime(facadeProvider func() *UnifiedMemoryFacade, baseProvider func() MemoryManager, logger *zap.Logger) *memorycore.DefaultMemoryRuntime {
	return memorycore.NewDefaultMemoryRuntime(facadeProvider, baseProvider, logger)
}

type ChatProvider = types.ChatProvider
type ChatRequest = types.ChatRequest
type ChatResponse = types.ChatResponse
type StreamChunk = types.StreamChunk

type ProviderAdapter struct {
	Provider llm.Provider
}

func NewProviderAdapter(p llm.Provider) *ProviderAdapter {
	return &ProviderAdapter{Provider: p}
}

var _ types.ChatProvider = (llm.Provider)(nil)

type ToolResult = types.ToolResult

type ToolExecutorAdapter struct {
	Executor llmtools.ToolExecutor
}

func (a *ToolExecutorAdapter) Execute(ctx context.Context, calls []types.ToolCall) []types.ToolResult {
	if a.Executor == nil {
		return nil
	}
	results := a.Executor.Execute(ctx, calls)
	out := make([]types.ToolResult, len(results))
	for i, r := range results {
		out[i] = types.ToolResult{
			ToolCallID: r.ToolCallID,
			Name:       r.Name,
			Result:     r.Result,
			Error:      r.Error,
			Duration:   r.Duration,
			FromCache:  r.FromCache,
		}
	}
	return out
}

func (a *ToolExecutorAdapter) ExecuteOne(ctx context.Context, call types.ToolCall) types.ToolResult {
	if a.Executor == nil {
		return types.ToolResult{ToolCallID: call.ID, Name: call.Name, Error: "executor not configured"}
	}
	r := a.Executor.ExecuteOne(ctx, call)
	return types.ToolResult{
		ToolCallID: r.ToolCallID,
		Name:       r.Name,
		Result:     r.Result,
		Error:      r.Error,
		Duration:   r.Duration,
		FromCache:  r.FromCache,
	}
}

// =============================================================================
// MongoDB Persistence Store Interfaces (required)
// =============================================================================
// These interfaces and DTOs now live in agent/persistence/stores.go; root keeps
// aliases so existing imports do not break.

type PromptStoreProvider = agentpersistence.PromptStoreProvider
type PromptDocument = agentpersistence.PromptDocument
type ConversationStoreProvider = agentpersistence.ConversationStoreProvider
type ConversationDoc = agentpersistence.ConversationDoc
type ConversationMessage = agentpersistence.ConversationMessage
type ConversationUpdate = agentpersistence.ConversationUpdate
type RunStoreProvider = agentpersistence.RunStoreProvider
type RunDoc = agentpersistence.RunDoc
type RunOutputDoc = agentpersistence.RunOutputDoc

// =============================================================================
// Orchestration Interfaces (used by agent/orchestration bridge)
// =============================================================================

// OrchestratorRunner executes a multi-agent orchestration task.
// Implemented by: *orchestration.OrchestratorAdapter (agent/orchestration/)
type OrchestratorRunner interface {
	Execute(ctx context.Context, task *OrchestrationTaskInput) (*OrchestrationTaskOutput, error)
}

// OrchestrationTaskInput is the input for an orchestration task.
type OrchestrationTaskInput struct {
	ID          string         `json:"id"`
	Description string         `json:"description"`
	Input       string         `json:"input"`
	Agents      []string       `json:"agents,omitempty"`
	Metadata    map[string]any `json:"metadata,omitempty"`
}

// OrchestrationTaskOutput is the output from an orchestration task.
type OrchestrationTaskOutput struct {
	Pattern   string         `json:"pattern"`
	Output    string         `json:"output"`
	AgentUsed []string       `json:"agent_used,omitempty"`
	Duration  time.Duration  `json:"duration"`
	Metadata  map[string]any `json:"metadata,omitempty"`
}

// =============================================================================
// Tool Manager Interface (merged from tool_manager.go)
// =============================================================================

// ToolManager为Agent运行时间摘要了"工具列表+工具执行"的能力.
//
// 设计目标:
// - 直接根据pkg/剂/工具避免pkg/剂(取消进口周期)
// - 允许在应用程序层注入不同的执行(默认使用工具)。 工具管理器)
type ToolManager interface {
	GetAllowedTools(agentID string) []types.ToolSchema
	ExecuteForAgent(ctx context.Context, agentID string, calls []types.ToolCall) []llmtools.ToolResult
}

func filterToolSchemasByWhitelist(all []types.ToolSchema, whitelist []string) []types.ToolSchema {
	if len(whitelist) == 0 {
		return all
	}
	allowed := make(map[string]struct{}, len(whitelist))
	for _, name := range whitelist {
		if name == "" {
			continue
		}
		allowed[name] = struct{}{}
	}
	out := make([]types.ToolSchema, 0, len(all))
	for _, s := range all {
		if _, ok := allowed[s.Name]; ok {
			out = append(out, s)
		}
	}
	return out
}

// =============================================================================
// PersistenceStores (merged from persistence_stores.go)
// =============================================================================

type PersistenceStores = agentpersistence.PersistenceStores
type ScopedPersistenceStores = agentpersistence.ScopedPersistenceStores

func NewPersistenceStores(logger *zap.Logger) *PersistenceStores {
	return agentpersistence.NewPersistenceStores(logger)
}

func NewScopedPersistenceStores(inner *PersistenceStores, scope string) *ScopedPersistenceStores {
	return agentpersistence.NewScopedPersistenceStores(inner, scope)
}

// =============================================================================
// Agent-as-Tool Adapter
// =============================================================================

// AgentToolConfig configures how an Agent is exposed as a tool.
type AgentToolConfig struct {
	// Name overrides the default tool name (default: "agent_<agent.Name()>".
	Name string

	// Description overrides the agent's description in the tool schema.
	Description string

	// Timeout limits the agent execution time. Zero means no extra timeout.
	Timeout time.Duration
}

// AgentTool wraps an Agent instance as a callable tool, enabling lightweight
// agent-to-agent delegation via the standard tool-calling interface.
type AgentTool struct {
	agent  Agent
	config AgentToolConfig
	name   string
}

// NewAgentTool creates an AgentTool that wraps the given Agent.
// If config is nil, defaults are used.
func NewAgentTool(agent Agent, config *AgentToolConfig) *AgentTool {
	cfg := AgentToolConfig{}
	if config != nil {
		cfg = *config
	}

	name := cfg.Name
	if name == "" {
		name = "agent_" + agent.Name()
	}

	return &AgentTool{
		agent:  agent,
		config: cfg,
		name:   name,
	}
}

// agentToolArgs is the JSON schema expected in ToolCall.Arguments.
type agentToolArgs struct {
	Input     string            `json:"input"`
	Context   map[string]any    `json:"context,omitempty"`
	Variables map[string]string `json:"variables,omitempty"`
}

// Schema returns the ToolSchema describing this agent-as-tool.
func (at *AgentTool) Schema() types.ToolSchema {
	desc := at.config.Description
	if desc == "" {
		desc = fmt.Sprintf("Delegate a task to the %q agent", at.agent.Name())
	}

	params := json.RawMessage(`{
		"type": "object",
		"properties": {
			"input": {
				"type": "string",
				"description": "The task or query to send to the agent"
			},
			"context": {
				"type": "object",
				"description": "Optional context key-value pairs"
			},
			"variables": {
				"type": "object",
				"description": "Optional variable substitutions",
				"additionalProperties": {"type": "string"}
			}
		},
		"required": ["input"]
	}`)

	return types.ToolSchema{
		Name:        at.name,
		Description: desc,
		Parameters:  params,
	}
}

// Execute handles a ToolCall by delegating to the wrapped Agent.
func (at *AgentTool) Execute(ctx context.Context, call types.ToolCall) llmtools.ToolResult {
	start := time.Now()
	result := llmtools.ToolResult{
		ToolCallID: call.ID,
		Name:       at.name,
	}

	// Parse arguments
	var args agentToolArgs
	if len(call.Arguments) > 0 {
		if err := json.Unmarshal(call.Arguments, &args); err != nil {
			result.Error = fmt.Sprintf("invalid arguments: %s", err.Error())
			result.Duration = time.Since(start)
			return result
		}
	}
	if args.Input == "" {
		result.Error = "missing required field: input"
		result.Duration = time.Since(start)
		return result
	}

	// Apply timeout if configured
	execCtx := ctx
	if at.config.Timeout > 0 {
		var cancel context.CancelFunc
		execCtx, cancel = context.WithTimeout(ctx, at.config.Timeout)
		defer cancel()
	}

	// Build agent Input
	input := &Input{
		Content:   args.Input,
		Context:   args.Context,
		Variables: args.Variables,
	}

	// Execute the agent
	output, err := at.agent.Execute(execCtx, input)
	if err != nil {
		result.Error = err.Error()
		result.Duration = time.Since(start)
		return result
	}

	// Marshal the output content as the tool result
	resultJSON, err := json.Marshal(map[string]any{
		"content":       output.Content,
		"tokens_used":   output.TokensUsed,
		"duration":      output.Duration.String(),
		"finish_reason": output.FinishReason,
	})
	if err != nil {
		result.Error = fmt.Sprintf("failed to marshal output: %s", err.Error())
		result.Duration = time.Since(start)
		return result
	}

	result.Result = resultJSON
	result.Duration = time.Since(start)
	return result
}

// Name returns the tool name.
func (at *AgentTool) Name() string {
	return at.name
}

// Agent returns the underlying Agent instance.
func (at *AgentTool) Agent() Agent {
	return at.agent
}

// =============================================================================
// Team (multi-agent collaboration)
// =============================================================================

type TeamMember struct {
	Agent Agent
	Role  string
}

type TeamResult struct {
	Content    string
	TokensUsed int
	Cost       float64
	Duration   time.Duration
	Metadata   map[string]any
}

type TeamOption func(*TeamOptions)
type TeamOptions struct {
	MaxRounds int
	Timeout   time.Duration
	Context   map[string]any
}

func WithMaxRounds(n int) TeamOption {
	return func(o *TeamOptions) { o.MaxRounds = n }
}

func WithTeamTimeout(d time.Duration) TeamOption {
	return func(o *TeamOptions) { o.Timeout = d }
}

func WithTeamContext(ctx map[string]any) TeamOption {
	return func(o *TeamOptions) { o.Context = ctx }
}

type Team interface {
	ID() string
	Members() []TeamMember
	Execute(ctx context.Context, task string, opts ...TeamOption) (*TeamResult, error)
}

// Merged from reflection.go.

// 反射执行器配置
type ReflectionExecutorConfig struct {
	Enabled       bool    `json:"enabled"`
	MaxIterations int     `json:"max_iterations"` // Maximum reflection iterations
	MinQuality    float64 `json:"min_quality"`    // Minimum quality threshold (0-1)
	CriticPrompt  string  `json:"critic_prompt"`  // Critic prompt template
}

// reflectionRunnerAdapter wraps *ReflectionExecutor to satisfy ReflectionRunner.
type reflectionRunnerAdapter struct {
	executor *ReflectionExecutor
}

func (a *reflectionRunnerAdapter) ExecuteWithReflection(ctx context.Context, input *Input) (*Output, error) {
	result, err := a.executor.ExecuteWithReflection(ctx, input)
	if err != nil {
		return nil, err
	}
	return result.FinalOutput, nil
}

func (a *reflectionRunnerAdapter) ReflectStep(ctx context.Context, input *Input, output *Output, state *LoopState) (*LoopReflectionResult, error) {
	return a.executor.ReflectStep(ctx, input, output, state)
}

// AsReflectionRunner wraps a *ReflectionExecutor as a ReflectionRunner.
func AsReflectionRunner(executor *ReflectionExecutor) ReflectionRunner {
	return &reflectionRunnerAdapter{executor: executor}
}

// promptEnhancerRunnerAdapter wraps *PromptEnhancer to satisfy PromptEnhancerRunner.
type promptEnhancerRunnerAdapter struct {
	enhancer *PromptEnhancer
}

func (a *promptEnhancerRunnerAdapter) EnhanceUserPrompt(prompt, context string) (string, error) {
	return a.enhancer.EnhanceUserPrompt(prompt, context), nil
}

// AsPromptEnhancerRunner wraps a *PromptEnhancer as a PromptEnhancerRunner.
func AsPromptEnhancerRunner(enhancer *PromptEnhancer) PromptEnhancerRunner {
	return &promptEnhancerRunnerAdapter{enhancer: enhancer}
}

// 默认反射 Config 返回默认反射配置
func DefaultReflectionConfig() *ReflectionExecutorConfig {
	config := DefaultReflectionExecutorConfig()
	return &config
}

// 默认反射 ExecutorConfig 返回默认反射配置
func DefaultReflectionExecutorConfig() ReflectionExecutorConfig {
	return ReflectionExecutorConfig{
		Enabled:       true,
		MaxIterations: 3,
		MinQuality:    0.7,
		CriticPrompt: `你是一个严格的评审专家。请评估以下任务执行结果的质量。

任务：{{.Task}}

执行结果：
{{.Output}}

请从以下维度评估（0-10分）：
1. 准确性：结果是否准确回答了问题
2. 完整性：是否涵盖了所有必要信息
3. 清晰度：表达是否清晰易懂
4. 相关性：是否紧扣主题

输出格式：
评分：[总分]/10
问题：[具体问题列表]
改进建议：[具体改进建议]`,
	}
}

// Critique 评审结果
type Critique struct {
	Score       float64  `json:"score"`        // 0-1 分数
	IsGood      bool     `json:"is_good"`      // 是否达标
	Issues      []string `json:"issues"`       // 问题列表
	Suggestions []string `json:"suggestions"`  // 改进建议
	RawFeedback string   `json:"raw_feedback"` // 原始反馈
}

// ReflectionResult Reflection 执行结果
type ReflectionResult struct {
	FinalOutput          *Output       `json:"final_output"`
	Iterations           int           `json:"iterations"`
	Critiques            []Critique    `json:"critiques"`
	TotalDuration        time.Duration `json:"total_duration"`
	ImprovedByReflection bool          `json:"improved_by_reflection"`
}

// ReflectionExecutor Reflection 执行器
type ReflectionExecutor struct {
	agent  *BaseAgent
	config ReflectionExecutorConfig
	logger *zap.Logger
}

// NewReflectionExecutor 创建 Reflection 执行器
func NewReflectionExecutor(agent *BaseAgent, config ReflectionExecutorConfig) *ReflectionExecutor {
	policyConfig := reflectionExecutorConfigFromPolicy(agent.loopControlPolicy())
	if config.MaxIterations <= 0 {
		config.MaxIterations = policyConfig.MaxIterations
	}
	if config.MinQuality <= 0 {
		config.MinQuality = policyConfig.MinQuality
	}
	if strings.TrimSpace(config.CriticPrompt) == "" {
		config.CriticPrompt = policyConfig.CriticPrompt
	}

	return &ReflectionExecutor{
		agent:  agent,
		config: config,
		logger: agent.Logger().With(zap.String("component", "reflection")),
	}
}

// ExecuteWithReflection 执行任务并进行 Reflection
func (r *ReflectionExecutor) ExecuteWithReflection(ctx context.Context, input *Input) (*ReflectionResult, error) {
	startTime := time.Now()

	if !r.config.Enabled {
		output, err := r.agent.executeCore(ctx, input)
		if err != nil {
			return nil, err
		}
		return &ReflectionResult{
			FinalOutput:          output,
			Iterations:           1,
			TotalDuration:        time.Since(startTime),
			ImprovedByReflection: false,
		}, nil
	}

	r.logger.Info("starting reflection execution", zap.String("trace_id", input.TraceID), zap.Int("max_iterations", r.config.MaxIterations))
	executor := &LoopExecutor{
		MaxIterations: r.config.MaxIterations,
		StepExecutor: func(ctx context.Context, input *Input, _ *LoopState, _ ReasoningSelection) (*Output, error) {
			return r.agent.executeCore(ctx, input)
		},
		Selector: reasoningModeSelectorFunc(func(_ context.Context, _ *Input, _ *LoopState, _ *reasoning.PatternRegistry, _ bool) ReasoningSelection {
			return ReasoningSelection{Mode: ReasoningModeReflection}
		}),
		Judge:             newReflectionCompletionJudge(r.config.MinQuality, r.critique),
		ReflectionStep:    r.ReflectStep,
		ReflectionEnabled: true,
		CheckpointManager: r.agent.checkpointManager,
		AgentID:           r.agent.ID(),
		Logger:            r.logger,
	}
	output, err := executor.Execute(ctx, input)
	if err != nil {
		return nil, err
	}
	if output.Metadata == nil {
		output.Metadata = make(map[string]any, 4)
	}
	output.Metadata["reflection_iteration_budget"] = r.config.MaxIterations
	output.Metadata["reflection_quality_threshold"] = r.config.MinQuality
	output.Metadata["reflection_budget_scope"] = internalBudgetScope

	duration := time.Since(startTime)
	critiques := outputReflectionCritiques(output)
	improved := len(critiques) > 1
	iterations := output.IterationCount
	if iterations == 0 {
		iterations = 1
	}
	r.logger.Info("reflection execution completed",
		zap.String("trace_id", input.TraceID),
		zap.Int("iterations", iterations),
		zap.Duration("total_duration", duration),
		zap.Bool("improved", improved))

	return &ReflectionResult{
		FinalOutput:          output,
		Iterations:           iterations,
		Critiques:            critiques,
		TotalDuration:        duration,
		ImprovedByReflection: improved,
	}, nil
}

func (r *ReflectionExecutor) ReflectStep(ctx context.Context, input *Input, output *Output, state *LoopState) (*LoopReflectionResult, error) {
	if !r.config.Enabled || input == nil || output == nil {
		return nil, nil
	}

	critique := outputReflectionCritique(output)
	if critique == nil {
		var err error
		critique, err = r.critique(ctx, input.Content, output.Content)
		if err != nil {
			return nil, err
		}
	}

	observation := &LoopObservation{
		Stage:     LoopStageDecideNext,
		Content:   "reflection_completed",
		Iteration: state.Iteration,
		Metadata: map[string]any{
			"reflection_critique": *critique,
			"reflection_score":    critique.Score,
			"reflection_is_good":  critique.IsGood,
		},
	}
	if critique.IsGood || state.Iteration >= state.MaxIterations {
		return &LoopReflectionResult{
			Critique:    critique,
			Observation: observation,
		}, nil
	}

	return &LoopReflectionResult{
		NextInput:   r.refineInput(input, critique),
		Critique:    critique,
		Observation: observation,
	}, nil
}

// critique 评审输出质量
func (r *ReflectionExecutor) critique(ctx context.Context, task, output string) (*Critique, error) {
	// 构建评审提示词
	prompt := r.config.CriticPrompt
	prompt = strings.ReplaceAll(prompt, "{{.Task}}", task)
	prompt = strings.ReplaceAll(prompt, "{{.Output}}", output)

	messages := []types.Message{
		{
			Role:    llm.RoleSystem,
			Content: "你是一个专业的质量评审专家，擅长发现问题并提供建设性建议。",
		},
		{
			Role:    llm.RoleUser,
			Content: prompt,
		},
	}

	// 调用 LLM 进行评审
	resp, err := r.agent.ChatCompletion(ctx, messages)
	if err != nil {
		return nil, fmt.Errorf("critique LLM call failed: %w", err)
	}

	choice, err := llm.FirstChoice(resp)
	if err != nil {
		return nil, fmt.Errorf("critique LLM returned no choices: %w", err)
	}

	feedback := choice.Message.Content

	// 解析评审结果
	critique := r.parseCritique(feedback)
	critique.RawFeedback = feedback

	return critique, nil
}

// parseCritique 解析评审反馈
func (r *ReflectionExecutor) parseCritique(feedback string) *Critique {
	critique := &Critique{
		Score:       0.5, // 默认中等分数
		Issues:      []string{},
		Suggestions: []string{},
	}

	lines := strings.Split(feedback, "\n")
	var currentSection string

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		// 提取分数
		if strings.Contains(line, "评分") || strings.Contains(line, "Score") {
			score := r.extractScore(line)
			if score > 0 {
				critique.Score = score / 10.0 // 转换为 0-1
			}
		}

		// 识别章节
		if strings.Contains(line, "问题") || strings.Contains(line, "Issues") {
			currentSection = "issues"
			continue
		}
		if strings.Contains(line, "改进建议") || strings.Contains(line, "Suggestions") {
			currentSection = "suggestions"
			continue
		}

		// 提取列表项
		if strings.HasPrefix(line, "-") || strings.HasPrefix(line, "•") ||
			(len(line) > 2 && line[0] >= '0' && line[0] <= '9' && line[1] == '.') {
			item := strings.TrimLeft(line, "-•0123456789. ")
			if item != "" {
				switch currentSection {
				case "issues":
					critique.Issues = append(critique.Issues, item)
				case "suggestions":
					critique.Suggestions = append(critique.Suggestions, item)
				}
			}
		}
	}

	// 判断是否达标
	critique.IsGood = critique.Score >= r.config.MinQuality

	return critique
}

// 从文本中提取分数
func (r *ReflectionExecutor) extractScore(text string) float64 {
	// 尝试提取“ X/ 10” 格式
	if idx := strings.Index(text, "/"); idx > 0 {
		// 提取“ /” 之前的部分
		beforeSlash := strings.TrimSpace(text[:idx])
		// 从结尾删除非数字字符
		numStr := ""
		for i := len(beforeSlash) - 1; i >= 0; i-- {
			ch := beforeSlash[i]
			if (ch >= '0' && ch <= '9') || ch == '.' {
				numStr = string(ch) + numStr
			} else if numStr != "" {
				break
			}
		}
		if numStr != "" {
			var score float64
			if _, err := fmt.Sscanf(numStr, "%f", &score); err == nil {
				return score
			}
		}
	}

	// 尝试提取纯数
	var score float64
	if _, err := fmt.Sscanf(text, "%f", &score); err == nil {
		return score
	}

	return 0
}

// refineInput 基于评审反馈改进输入
func (r *ReflectionExecutor) refineInput(original *Input, critique *Critique) *Input {
	// 构建改进提示
	refinementPrompt := fmt.Sprintf(`原始任务：
%s

之前的执行存在以下问题：
%s

改进建议：
%s

请重新执行任务，注意避免上述问题，并采纳改进建议。`,
		original.Content,
		strings.Join(critique.Issues, "\n- "),
		strings.Join(critique.Suggestions, "\n- "),
	)

	// 创建新的输入
	refined := &Input{
		TraceID:   original.TraceID,
		TenantID:  original.TenantID,
		UserID:    original.UserID,
		ChannelID: original.ChannelID,
		Content:   refinementPrompt,
		Context:   original.Context,
		Variables: original.Variables,
	}

	// 在 Context 中记录 Reflection 历史
	if refined.Context == nil {
		refined.Context = make(map[string]any)
	}
	refined.Context["reflection_feedback"] = critique

	return refined
}

func outputReflectionCritiques(output *Output) []Critique {
	if output == nil || output.Metadata == nil {
		return nil
	}
	critiques := make([]Critique, 0, 2)
	if rawCritiques, ok := output.Metadata["reflection_critiques"]; ok {
		switch stored := rawCritiques.(type) {
		case []Critique:
			critiques = append(critiques, stored...)
		case []any:
			for _, item := range stored {
				if critique, ok := coerceCritique(item); ok {
					critiques = append(critiques, critique)
				}
			}
		}
	}
	if critique := outputReflectionCritique(output); critique != nil {
		critiques = append(critiques, *critique)
	}
	critiques = dedupeCritiques(critiques)
	if len(critiques) == 0 {
		return nil
	}
	return critiques
}

func outputReflectionCritique(output *Output) *Critique {
	if output == nil || output.Metadata == nil {
		return nil
	}
	rawCritique, ok := output.Metadata["reflection_critique"]
	if !ok {
		return nil
	}
	critique, ok := coerceCritique(rawCritique)
	if !ok {
		return nil
	}
	copied := critique
	return &copied
}

type reflectionCompletionJudge struct {
	minQuality float64
	fallback   CompletionJudge
	critiqueFn func(context.Context, string, string) (*Critique, error)
}

func newReflectionCompletionJudge(minQuality float64, critiqueFn func(context.Context, string, string) (*Critique, error)) CompletionJudge {
	if minQuality <= 0 {
		minQuality = 0.7
	}
	return &reflectionCompletionJudge{
		minQuality: minQuality,
		fallback:   NewDefaultCompletionJudge(),
		critiqueFn: critiqueFn,
	}
}

func (j *reflectionCompletionJudge) Judge(ctx context.Context, state *LoopState, output *Output, err error) (*CompletionDecision, error) {
	decision, judgeErr := j.fallback.Judge(ctx, state, output, err)
	if judgeErr != nil || decision == nil || err != nil || output == nil || strings.TrimSpace(output.Content) == "" {
		return decision, judgeErr
	}

	critique, critiqueErr := j.critiqueFn(ctx, state.Goal, output.Content)
	if critiqueErr != nil {
		return nil, critiqueErr
	}
	if output.Metadata == nil {
		output.Metadata = make(map[string]any, 4)
	}
	output.Metadata["reflection_critique"] = *critique
	output.Metadata["reflection_score"] = critique.Score
	output.Metadata["reflection_is_good"] = critique.IsGood
	output.Metadata["reflection_quality_threshold"] = j.minQuality

	if critique.IsGood || critique.Score >= j.minQuality {
		decision.Decision = LoopDecisionDone
		decision.Solved = true
		decision.StopReason = StopReasonSolved
		decision.Confidence = critique.Score
		decision.Reason = "reflection quality acceptable"
		return decision, nil
	}

	if state != nil && state.Iteration >= state.MaxIterations {
		decision.Decision = LoopDecisionDone
		decision.StopReason = StopReasonBlocked
		decision.Confidence = critique.Score
		decision.Reason = "reflection iteration budget exhausted"
		if output.Metadata == nil {
			output.Metadata = make(map[string]any, 4)
		}
		output.Metadata["internal_stop_cause"] = "reflection_iteration_budget_exhausted"
		return decision, nil
	}

	return &CompletionDecision{
		Decision:       LoopDecisionReflect,
		NeedReflection: true,
		StopReason:     StopReasonBlocked,
		Confidence:     critique.Score,
		Reason:         "reflection requested another iteration",
	}, nil
}

func reflectionCritiquesFromObservations(observations []LoopObservation) []Critique {
	critiques := make([]Critique, 0, len(observations))
	for _, observation := range observations {
		if observation.Metadata == nil {
			continue
		}
		raw, ok := observation.Metadata["reflection_critique"]
		if !ok {
			continue
		}
		critique, ok := coerceCritique(raw)
		if ok {
			critiques = append(critiques, critique)
			continue
		}
		synthesized := Critique{}
		seen := false
		if score, ok := observation.Metadata["reflection_score"].(float64); ok {
			synthesized.Score = score
			seen = true
		}
		if isGood, ok := observation.Metadata["reflection_is_good"].(bool); ok {
			synthesized.IsGood = isGood
			seen = true
		}
		if seen {
			critiques = append(critiques, synthesized)
		}
	}
	return dedupeCritiques(critiques)
}

func coerceCritique(raw any) (Critique, bool) {
	switch critique := raw.(type) {
	case Critique:
		return critique, true
	case *Critique:
		if critique == nil {
			return Critique{}, false
		}
		return *critique, true
	case map[string]any:
		out := Critique{}
		if score, ok := critique["score"].(float64); ok {
			out.Score = score
		}
		if isGood, ok := critique["is_good"].(bool); ok {
			out.IsGood = isGood
		}
		if rawIssues, ok := critique["issues"].([]any); ok {
			for _, item := range rawIssues {
				if text, ok := item.(string); ok && strings.TrimSpace(text) != "" {
					out.Issues = append(out.Issues, text)
				}
			}
		} else if rawIssues, ok := critique["issues"].([]string); ok {
			out.Issues = append(out.Issues, rawIssues...)
		}
		if rawSuggestions, ok := critique["suggestions"].([]any); ok {
			for _, item := range rawSuggestions {
				if text, ok := item.(string); ok && strings.TrimSpace(text) != "" {
					out.Suggestions = append(out.Suggestions, text)
				}
			}
		} else if rawSuggestions, ok := critique["suggestions"].([]string); ok {
			out.Suggestions = append(out.Suggestions, rawSuggestions...)
		}
		if feedback, ok := critique["raw_feedback"].(string); ok {
			out.RawFeedback = feedback
		}
		return out, true
	default:
		return Critique{}, false
	}
}

func dedupeCritiques(critiques []Critique) []Critique {
	if len(critiques) == 0 {
		return nil
	}
	out := make([]Critique, 0, len(critiques))
	seen := make(map[string]struct{}, len(critiques))
	for _, critique := range critiques {
		key := critique.RawFeedback + "|" + fmt.Sprintf("%.4f|%t|%s|%s", critique.Score, critique.IsGood, strings.Join(critique.Issues, "\x00"), strings.Join(critique.Suggestions, "\x00"))
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, critique)
	}
	return out
}

// Merged from tool_selector.go.

// ToolScore 工具评分
type ToolScore = skills.DynamicToolScore

// ToolSelectionConfig 工具选择配置
type ToolSelectionConfig = skills.DynamicToolSelectionConfig

// 默认工具SecutConfig 返回默认工具选择配置
func DefaultToolSelectionConfig() *ToolSelectionConfig {
	config := defaultToolSelectionConfigValue()
	return &config
}

// 默认工具Secution ConfigValue 返回默认工具选择配置值
func defaultToolSelectionConfigValue() ToolSelectionConfig {
	return skills.DefaultDynamicToolSelectionConfig()
}

// ToolSelector 工具选择器接口
type ToolSelector interface {
	// SelectTools 基于任务选择最佳工具
	SelectTools(ctx context.Context, task string, availableTools []types.ToolSchema) ([]types.ToolSchema, error)

	// ScoreTools 对工具进行评分
	ScoreTools(ctx context.Context, task string, tools []types.ToolSchema) ([]ToolScore, error)
}

// ReasoningExposureLevel controls which non-default reasoning patterns are
// registered into the runtime. The official execution path remains react,
// with reflection as an opt-in quality enhancement outside the registry.
type ReasoningExposureLevel string

const (
	ReasoningExposureOfficial ReasoningExposureLevel = "official"
	ReasoningExposureAdvanced ReasoningExposureLevel = "advanced"
	ReasoningExposureAll      ReasoningExposureLevel = "all"
)

func normalizeReasoningExposureLevel(level ReasoningExposureLevel) ReasoningExposureLevel {
	switch level {
	case ReasoningExposureAdvanced, ReasoningExposureAll:
		return level
	default:
		return ReasoningExposureOfficial
	}
}

const (
	ReasoningModeReact          = executionloop.ReasoningModeReact
	ReasoningModeReflection     = executionloop.ReasoningModeReflection
	ReasoningModeReWOO          = executionloop.ReasoningModeReWOO
	ReasoningModePlanAndExecute = executionloop.ReasoningModePlanAndExecute
	ReasoningModeDynamicPlanner = executionloop.ReasoningModeDynamicPlanner
	ReasoningModeTreeOfThought  = executionloop.ReasoningModeTreeOfThought
)

type ReasoningSelection = executionloop.ReasoningSelection

type ReasoningModeSelector interface {
	Select(ctx context.Context, input *Input, state *LoopState, registry *reasoning.PatternRegistry, reflectionEnabled bool) ReasoningSelection
}

type reasoningModeSelectorFunc func(ctx context.Context, input *Input, state *LoopState, registry *reasoning.PatternRegistry, reflectionEnabled bool) ReasoningSelection

func (f reasoningModeSelectorFunc) Select(ctx context.Context, input *Input, state *LoopState, registry *reasoning.PatternRegistry, reflectionEnabled bool) ReasoningSelection {
	return f(ctx, input, state, registry, reflectionEnabled)
}

type DefaultReasoningModeSelector struct{}

func NewDefaultReasoningModeSelector() ReasoningModeSelector { return DefaultReasoningModeSelector{} }

func (DefaultReasoningModeSelector) Select(ctx context.Context, input *Input, state *LoopState, registry *reasoning.PatternRegistry, reflectionEnabled bool) ReasoningSelection {
	selection := executionloop.DefaultReasoningModeSelector{}.Select(ctx, loopExecutionInputFromRoot(input), loopExecutionStateFromRoot(state), registry, reflectionEnabled)
	return ReasoningSelection(selection)
}

func runtimeSelectResumedReasoningMode(state *LoopState, registry *reasoning.PatternRegistry, reflectionEnabled bool) (ReasoningSelection, bool) {
	return executionloop.SelectResumedReasoningMode(loopExecutionStateFromRoot(state), registry, reflectionEnabled)
}

func runtimeBuildReasoningSelectionWithFallback(mode string, registry *reasoning.PatternRegistry, reflectionEnabled bool) ReasoningSelection {
	return executionloop.BuildReasoningSelectionWithFallback(mode, registry, reflectionEnabled)
}

func runtimeBuildReasoningSelection(mode string, registry *reasoning.PatternRegistry) ReasoningSelection {
	return executionloop.BuildReasoningSelection(mode, registry)
}

func runtimeNormalizeReasoningMode(value string) string {
	return executionloop.NormalizeReasoningMode(value)
}

func runtimeShouldUseReflection(input *Input, state *LoopState, registry *reasoning.PatternRegistry, reflectionEnabled bool) bool {
	return executionloop.ShouldUseReflection(loopExecutionInputFromRoot(input), loopExecutionStateFromRoot(state), registry, reflectionEnabled)
}

func runtimeShouldUseReWOO(input *Input, state *LoopState, registry *reasoning.PatternRegistry) bool {
	return executionloop.ShouldUseReWOO(loopExecutionInputFromRoot(input), loopExecutionStateFromRoot(state), registry)
}

func runtimeShouldUsePlanAndExecute(input *Input, state *LoopState, registry *reasoning.PatternRegistry) bool {
	return executionloop.ShouldUsePlanAndExecute(loopExecutionInputFromRoot(input), loopExecutionStateFromRoot(state), registry)
}

func runtimeShouldUseDynamicPlanner(input *Input, state *LoopState, registry *reasoning.PatternRegistry) bool {
	return executionloop.ShouldUseDynamicPlanner(loopExecutionInputFromRoot(input), loopExecutionStateFromRoot(state), registry)
}

func runtimeShouldUseTreeOfThought(input *Input, state *LoopState, registry *reasoning.PatternRegistry) bool {
	return executionloop.ShouldUseTreeOfThought(loopExecutionInputFromRoot(input), loopExecutionStateFromRoot(state), registry)
}

func hasReasoningPattern(registry *reasoning.PatternRegistry, mode string) bool {
	return executionloop.HasReasoningPattern(registry, mode)
}

// DynamicToolSelector 动态工具选择器
type DynamicToolSelector struct {
	agent  *BaseAgent
	config ToolSelectionConfig

	// 工具统计(可以从数据库中加载)
	toolStats map[string]*ToolStats

	logger *zap.Logger
}

// ToolStats 工具统计信息
type ToolStats = skills.DynamicToolStats

// NewDynamicToolSelector 创建动态工具选择器
func NewDynamicToolSelector(agent *BaseAgent, config ToolSelectionConfig) *DynamicToolSelector {
	if config.MaxTools <= 0 {
		config.MaxTools = 5
	}
	if config.MinScore <= 0 {
		config.MinScore = 0.3
	}

	return &DynamicToolSelector{
		agent:     agent,
		config:    config,
		toolStats: make(map[string]*ToolStats),
		logger:    agent.Logger().With(zap.String("component", "tool_selector")),
	}
}

// SelectTools 选择最佳工具
func (s *DynamicToolSelector) SelectTools(ctx context.Context, task string, availableTools []types.ToolSchema) ([]types.ToolSchema, error) {
	if !s.config.Enabled || len(availableTools) == 0 {
		return availableTools, nil
	}

	s.logger.Debug("selecting tools",
		zap.String("task", task),
		zap.Int("available_tools", len(availableTools)),
	)

	// 1. 对所有工具评分
	scores, err := s.ScoreTools(ctx, task, availableTools)
	if err != nil {
		s.logger.Warn("tool scoring failed, using all tools", zap.Error(err))
		return availableTools, nil
	}

	// 2. 按分数排序
	sort.Slice(scores, func(i, j int) bool {
		return scores[i].TotalScore > scores[j].TotalScore
	})

	// 3. 可选：使用 LLM 进行二次排序
	if s.config.UseLLMRanking && len(scores) > s.config.MaxTools {
		scores, err = s.llmRanking(ctx, task, scores)
		if err != nil {
			s.logger.Warn("LLM ranking failed, using score-based ranking", zap.Error(err))
		}
	}

	// 4. 选择 Top-K 工具
	selected := []types.ToolSchema{}
	for i, score := range scores {
		if i >= s.config.MaxTools {
			break
		}
		if score.TotalScore < s.config.MinScore {
			break
		}
		selected = append(selected, score.Tool)
	}

	s.logger.Info("tools selected",
		zap.Int("selected", len(selected)),
		zap.Int("total", len(availableTools)),
	)

	return selected, nil
}

func (b *BaseAgent) toolSelectionMiddleware() ExecutionMiddleware {
	return func(ctx context.Context, input *Input, next ExecutionFunc) (*Output, error) {
		b.logger.Debug("selecting tools dynamically", zap.String("trace_id", input.TraceID))
		availableTools := b.toolManager.GetAllowedTools(b.ID())
		selected, err := b.extensions.ToolSelector().SelectTools(ctx, input.Content, availableTools)
		if err != nil {
			b.logger.Warn("tool selection failed", zap.String("trace_id", input.TraceID), zap.Error(err))
		} else {
			toolNames := make([]string, 0, len(selected))
			for _, tool := range selected {
				name := strings.TrimSpace(tool.Name)
				if name == "" {
					continue
				}
				toolNames = append(toolNames, name)
			}

			override := &RunConfig{}
			if len(toolNames) == 0 {
				override.DisableTools = true
			} else {
				override.ToolWhitelist = toolNames
			}
			ctx = WithRunConfig(ctx, MergeRunConfig(GetRunConfig(ctx), override))

			b.logger.Info("tools selected dynamically",
				zap.String("trace_id", input.TraceID),
				zap.Strings("selected_tools", toolNames),
				zap.Bool("tools_disabled", len(toolNames) == 0),
			)
		}
		return next(ctx, input)
	}
}

// ScoreTools 对工具进行评分
func (s *DynamicToolSelector) ScoreTools(ctx context.Context, task string, tools []types.ToolSchema) ([]ToolScore, error) {
	scores := make([]ToolScore, len(tools))

	for i, tool := range tools {
		score := ToolScore{
			Tool: tool,
		}

		// 1. 语义相似度（基于描述匹配）
		score.SemanticSimilarity = s.calculateSemanticSimilarity(task, tool)

		// 2. 成本评估
		score.EstimatedCost = s.estimateCost(tool)

		// 3. 延迟评估
		score.AvgLatency = s.getAvgLatency(tool.Name)

		// 4. 可靠性评估
		score.ReliabilityScore = s.getReliability(tool.Name)

		// 5. 计算综合得分
		score.TotalScore = s.calculateTotalScore(score)

		scores[i] = score
	}

	return scores, nil
}

// 计算任务和工具之间的语义相似性
func (s *DynamicToolSelector) calculateSemanticSimilarity(task string, tool types.ToolSchema) float64 {
	return skills.DynamicToolSemanticSimilarity(task, tool)
}

// 成本估计工具执行费用
func (s *DynamicToolSelector) estimateCost(tool types.ToolSchema) float64 {
	return skills.DynamicToolEstimateCost(tool)
}

// getAvgLatency 获取平均延迟
func (s *DynamicToolSelector) getAvgLatency(toolName string) time.Duration {
	return skills.DynamicToolAverageLatency(s.toolStats[toolName])
}

// getReliability 获取可靠性分数
func (s *DynamicToolSelector) getReliability(toolName string) float64 {
	return skills.DynamicToolReliability(s.toolStats[toolName])
}

// 计算总加权分数
func (s *DynamicToolSelector) calculateTotalScore(score ToolScore) float64 {
	return skills.DynamicToolTotalScore(score, s.config)
}

// llmRanking 使用 LLM 进行二级排名
func (s *DynamicToolSelector) llmRanking(ctx context.Context, task string, scores []ToolScore) ([]ToolScore, error) {
	// 构建工具列表描述
	toolList := []string{}
	for i, score := range scores {
		if i >= s.config.MaxTools*2 { // Only let LLM rank top 2*MaxTools
			break
		}
		toolList = append(toolList, fmt.Sprintf("%d. %s: %s (Score: %.2f)",
			i+1, score.Tool.Name, score.Tool.Description, score.TotalScore))
	}

	prompt := fmt.Sprintf(`任务：%s

可用工具列表：
%s

请从上述工具中选择最适合完成任务的 %d 个工具，按优先级排序。
只输出工具编号，用逗号分隔，例如：1,3,5`,
		task,
		strings.Join(toolList, "\n"),
		s.config.MaxTools,
	)

	messages := []types.Message{
		{
			Role:    llm.RoleSystem,
			Content: "你是一个工具选择专家，擅长为任务选择最合适的工具。",
		},
		{
			Role:    llm.RoleUser,
			Content: prompt,
		},
	}

	resp, err := s.agent.ChatCompletion(ctx, messages)
	if err != nil {
		return scores, err
	}

	choice, err := llm.FirstChoice(resp)
	if err != nil {
		return scores, err
	}

	// LLM 返回的解析工具索引
	selected := parseToolIndices(choice.Message.Content)

	// 重排工具
	reordered := []ToolScore{}
	for _, idx := range selected {
		if idx > 0 && idx <= len(scores) {
			reordered = append(reordered, scores[idx-1])
		}
	}

	// 添加剩余工具
	usedIndices := make(map[int]bool)
	for _, idx := range selected {
		usedIndices[idx] = true
	}
	for i := range scores {
		if !usedIndices[i+1] {
			reordered = append(reordered, scores[i])
		}
	}

	return reordered, nil
}

// AsToolSelectorRunner wraps a *DynamicToolSelector as a DynamicToolSelectorRunner.
// Since the interface now uses concrete types, this is a direct cast.
func AsToolSelectorRunner(selector *DynamicToolSelector) DynamicToolSelectorRunner {
	return selector
}

// UpdateToolStats 更新工具统计信息
func (s *DynamicToolSelector) UpdateToolStats(toolName string, success bool, latency time.Duration, cost float64) {
	skills.DynamicToolUpdateStats(s.toolStats, toolName, success, latency, cost)
}

// 取出关键字从文本中取出关键字(简化版)
func extractKeywords(text string) []string {
	return skills.DynamicToolExtractKeywords(text)
}

// 解析工具索引
// 只解析逗号分隔格式, 返回新行分隔为空
func parseToolIndices(text string) []int {
	return skills.DynamicToolParseIndices(text)
}
