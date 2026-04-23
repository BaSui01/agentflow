package checkpoint

import (
	"context"
	"encoding/json"
	"time"

	agentcore "github.com/BaSui01/agentflow/agent/core"
	checkpointcore "github.com/BaSui01/agentflow/agent/persistence/checkpoint/core"
)

// CheckpointDiff 代表两个检查点版本之间的差异。
type CheckpointDiff struct {
	ThreadID     string        `json:"thread_id"`
	Version1     int           `json:"version1"`
	Version2     int           `json:"version2"`
	StateChanged bool          `json:"state_changed"`
	OldState     agentcore.State `json:"old_state"`
	NewState     agentcore.State `json:"new_state"`
	MessagesDiff string        `json:"messages_diff"`
	MetadataDiff string        `json:"metadata_diff"`
	TimeDiff     time.Duration `json:"time_diff"`
}

// Checkpoint Agent 执行检查点（基于 LangGraph 2026 标准）。
type Checkpoint struct {
	ID                  string              `json:"id"`
	ThreadID            string              `json:"thread_id"`
	AgentID             string              `json:"agent_id"`
	LoopStateID         string              `json:"loop_state_id,omitempty"`
	RunID               string              `json:"run_id,omitempty"`
	Goal                string              `json:"goal,omitempty"`
	AcceptanceCriteria  []string            `json:"acceptance_criteria,omitempty"`
	UnresolvedItems     []string            `json:"unresolved_items,omitempty"`
	RemainingRisks      []string            `json:"remaining_risks,omitempty"`
	CurrentPlanID       string              `json:"current_plan_id,omitempty"`
	PlanVersion         int                 `json:"plan_version,omitempty"`
	CurrentStepID       string              `json:"current_step_id,omitempty"`
	ValidationStatus    agentcore.LoopValidationStatus `json:"validation_status,omitempty"`
	ValidationSummary   string              `json:"validation_summary,omitempty"`
	ObservationsSummary string              `json:"observations_summary,omitempty"`
	LastOutputSummary   string              `json:"last_output_summary,omitempty"`
	LastError           string              `json:"last_error,omitempty"`
	Version             int                 `json:"version"`
	State               agentcore.State     `json:"state"`
	Messages            []CheckpointMessage `json:"messages"`
	Metadata            map[string]any      `json:"metadata"`
	CreatedAt           time.Time           `json:"created_at"`
	ParentID            string              `json:"parent_id,omitempty"`

	ExecutionContext *ExecutionContext `json:"execution_context,omitempty"`
}

type CheckpointMessage struct {
	Role      string               `json:"role"`
	Content   string               `json:"content"`
	ToolCalls []CheckpointToolCall `json:"tool_calls,omitempty"`
	Metadata  map[string]any       `json:"metadata,omitempty"`
}

type CheckpointToolCall struct {
	ID        string          `json:"id"`
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments"`
	Result    json.RawMessage `json:"result,omitempty"`
	Error     string          `json:"error,omitempty"`
}

type ExecutionContext struct {
	WorkflowID          string         `json:"workflow_id,omitempty"`
	CurrentNode         string         `json:"current_node,omitempty"`
	NodeResults         map[string]any `json:"node_results,omitempty"`
	Variables           map[string]any `json:"variables,omitempty"`
	LoopStateID         string         `json:"loop_state_id,omitempty"`
	RunID               string         `json:"run_id,omitempty"`
	AgentID             string         `json:"agent_id,omitempty"`
	Goal                string         `json:"goal,omitempty"`
	AcceptanceCriteria  []string       `json:"acceptance_criteria,omitempty"`
	UnresolvedItems     []string       `json:"unresolved_items,omitempty"`
	RemainingRisks      []string       `json:"remaining_risks,omitempty"`
	CurrentPlanID       string         `json:"current_plan_id,omitempty"`
	PlanVersion         int            `json:"plan_version,omitempty"`
	CurrentStepID       string         `json:"current_step_id,omitempty"`
	ValidationStatus    agentcore.LoopValidationStatus `json:"validation_status,omitempty"`
	ValidationSummary   string         `json:"validation_summary,omitempty"`
	ObservationsSummary string         `json:"observations_summary,omitempty"`
	LastOutputSummary   string         `json:"last_output_summary,omitempty"`
	LastError           string         `json:"last_error,omitempty"`
}

func (c *Checkpoint) LoopContextValues() map[string]any {
	if c == nil {
		return nil
	}
	c.normalizeLoopPersistenceFields()
	data := checkpointPersistenceCore(c)
	return data.LoopContextValues()
}

func (c *ExecutionContext) LoopContextValues() map[string]any {
	if c == nil {
		return nil
	}
	return executionContextPersistenceCore(c).LoopContextValues()
}

func (c *Checkpoint) normalizeLoopPersistenceFields() {
	if c == nil {
		return
	}
	data := checkpointPersistenceCore(c)
	data.Normalize()
	applyCheckpointPersistenceCore(c, data)
}

func checkpointPersistenceCore(checkpoint *Checkpoint) checkpointcore.CheckpointData {
	return checkpointcore.CheckpointData{
		LoopStateID:         checkpoint.LoopStateID,
		RunID:               checkpoint.RunID,
		AgentID:             checkpoint.AgentID,
		Goal:                checkpoint.Goal,
		AcceptanceCriteria:  cloneStringSlice(checkpoint.AcceptanceCriteria),
		UnresolvedItems:     cloneStringSlice(checkpoint.UnresolvedItems),
		RemainingRisks:      cloneStringSlice(checkpoint.RemainingRisks),
		CurrentPlanID:       checkpoint.CurrentPlanID,
		PlanVersion:         checkpoint.PlanVersion,
		CurrentStepID:       checkpoint.CurrentStepID,
		ValidationStatus:    string(checkpoint.ValidationStatus),
		ValidationSummary:   checkpoint.ValidationSummary,
		ObservationsSummary: checkpoint.ObservationsSummary,
		LastOutputSummary:   checkpoint.LastOutputSummary,
		LastError:           checkpoint.LastError,
		Metadata:            cloneMetadata(checkpoint.Metadata),
		ExecutionContext:    executionContextPersistenceCore(checkpoint.ExecutionContext),
	}
}

func executionContextPersistenceCore(ctx *ExecutionContext) *checkpointcore.ExecutionContextData {
	if ctx == nil {
		return nil
	}
	return &checkpointcore.ExecutionContextData{
		CurrentNode:         ctx.CurrentNode,
		Variables:           cloneMetadata(ctx.Variables),
		LoopStateID:         ctx.LoopStateID,
		RunID:               ctx.RunID,
		AgentID:             ctx.AgentID,
		Goal:                ctx.Goal,
		AcceptanceCriteria:  cloneStringSlice(ctx.AcceptanceCriteria),
		UnresolvedItems:     cloneStringSlice(ctx.UnresolvedItems),
		RemainingRisks:      cloneStringSlice(ctx.RemainingRisks),
		CurrentPlanID:       ctx.CurrentPlanID,
		PlanVersion:         ctx.PlanVersion,
		CurrentStepID:       ctx.CurrentStepID,
		ValidationStatus:    string(ctx.ValidationStatus),
		ValidationSummary:   ctx.ValidationSummary,
		ObservationsSummary: ctx.ObservationsSummary,
		LastOutputSummary:   ctx.LastOutputSummary,
		LastError:           ctx.LastError,
	}
}

func applyCheckpointPersistenceCore(checkpoint *Checkpoint, data checkpointcore.CheckpointData) {
	checkpoint.LoopStateID = data.LoopStateID
	checkpoint.RunID = data.RunID
	checkpoint.AgentID = data.AgentID
	checkpoint.Goal = data.Goal
	checkpoint.AcceptanceCriteria = cloneStringSlice(data.AcceptanceCriteria)
	checkpoint.UnresolvedItems = cloneStringSlice(data.UnresolvedItems)
	checkpoint.RemainingRisks = cloneStringSlice(data.RemainingRisks)
	checkpoint.CurrentPlanID = data.CurrentPlanID
	checkpoint.PlanVersion = data.PlanVersion
	checkpoint.CurrentStepID = data.CurrentStepID
	checkpoint.ValidationStatus = agentcore.LoopValidationStatus(data.ValidationStatus)
	checkpoint.ValidationSummary = data.ValidationSummary
	checkpoint.ObservationsSummary = data.ObservationsSummary
	checkpoint.LastOutputSummary = data.LastOutputSummary
	checkpoint.LastError = data.LastError
	checkpoint.Metadata = data.Metadata
	if data.ExecutionContext == nil {
		checkpoint.ExecutionContext = nil
		return
	}
	if checkpoint.ExecutionContext == nil {
		checkpoint.ExecutionContext = &ExecutionContext{}
	}
	checkpoint.ExecutionContext.CurrentNode = data.ExecutionContext.CurrentNode
	checkpoint.ExecutionContext.Variables = data.ExecutionContext.Variables
	checkpoint.ExecutionContext.LoopStateID = data.ExecutionContext.LoopStateID
	checkpoint.ExecutionContext.RunID = data.ExecutionContext.RunID
	checkpoint.ExecutionContext.AgentID = data.ExecutionContext.AgentID
	checkpoint.ExecutionContext.Goal = data.ExecutionContext.Goal
	checkpoint.ExecutionContext.AcceptanceCriteria = cloneStringSlice(data.ExecutionContext.AcceptanceCriteria)
	checkpoint.ExecutionContext.UnresolvedItems = cloneStringSlice(data.ExecutionContext.UnresolvedItems)
	checkpoint.ExecutionContext.RemainingRisks = cloneStringSlice(data.ExecutionContext.RemainingRisks)
	checkpoint.ExecutionContext.CurrentPlanID = data.ExecutionContext.CurrentPlanID
	checkpoint.ExecutionContext.PlanVersion = data.ExecutionContext.PlanVersion
	checkpoint.ExecutionContext.CurrentStepID = data.ExecutionContext.CurrentStepID
	checkpoint.ExecutionContext.ValidationStatus = agentcore.LoopValidationStatus(data.ExecutionContext.ValidationStatus)
	checkpoint.ExecutionContext.ValidationSummary = data.ExecutionContext.ValidationSummary
	checkpoint.ExecutionContext.ObservationsSummary = data.ExecutionContext.ObservationsSummary
	checkpoint.ExecutionContext.LastOutputSummary = data.ExecutionContext.LastOutputSummary
	checkpoint.ExecutionContext.LastError = data.ExecutionContext.LastError
}

type CheckpointVersion struct {
	Version   int       `json:"version"`
	ID        string    `json:"id"`
	CreatedAt time.Time `json:"created_at"`
	State     agentcore.State `json:"state"`
	Summary   string    `json:"summary"`
}

// CheckpointStore 检查点存储接口（Agent 层）。
type CheckpointStore interface {
	Save(ctx context.Context, checkpoint *Checkpoint) error
	Load(ctx context.Context, checkpointID string) (*Checkpoint, error)
	LoadLatest(ctx context.Context, threadID string) (*Checkpoint, error)
	List(ctx context.Context, threadID string, limit int) ([]*Checkpoint, error)
	Delete(ctx context.Context, checkpointID string) error
	DeleteThread(ctx context.Context, threadID string) error
	LoadVersion(ctx context.Context, threadID string, version int) (*Checkpoint, error)
	ListVersions(ctx context.Context, threadID string) ([]CheckpointVersion, error)
	Rollback(ctx context.Context, threadID string, version int) error
}

type Store = CheckpointStore
type Snapshot = Checkpoint
type Version = CheckpointVersion
type Diff = CheckpointDiff

func GenerateID() string {
	return checkpointcore.NextCheckpointID(&checkpointIDCounter)
}

func cloneMetadata(metadata map[string]any) map[string]any {
	if len(metadata) == 0 {
		return nil
	}
	cloned := make(map[string]any, len(metadata))
	for k, v := range metadata {
		cloned[k] = v
	}
	return cloned
}

func cloneStringSlice(values []string) []string {
	return checkpointcore.CloneStringSlice(values)
}
