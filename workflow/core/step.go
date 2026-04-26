package core

import (
	"context"
	"time"

	"github.com/BaSui01/agentflow/types"
)

// StepType 步骤类型枚举。
type StepType string

const (
	StepTypeLLM              StepType = "llm"
	StepTypeTool             StepType = "tool"
	StepTypeHuman            StepType = "human"
	StepTypeHumanInput       StepType = "human_input" // DSL 别名，与 StepTypeHuman 语义等价
	StepTypeCode             StepType = "code"
	StepTypeAgent            StepType = "agent"
	StepTypeHybridRetrieve   StepType = "hybrid_retrieve"
	StepTypeMultiHopRetrieve StepType = "multi_hop_retrieve"
	StepTypeRerank           StepType = "rerank"
	StepTypeOrchestration    StepType = "orchestration"
	StepTypeChain            StepType = "chain"
	StepTypePassthrough      StepType = "passthrough"
)

// StepProtocol 统一步骤协议（Command Pattern）。
// 所有步骤类型必须实现此接口。
type StepProtocol interface {
	// ID 返回步骤唯一标识。
	ID() string
	// Type 返回步骤类型。
	Type() StepType
	// Execute 执行步骤。
	Execute(ctx context.Context, input StepInput) (StepOutput, error)
	// Validate 校验步骤配置是否合法。
	Validate() error
}

// StepInput 步骤输入。
type StepInput struct {
	// Data 上游步骤输出或用户输入。
	// nil 时视为空 map，调用方应使用 len(Data) == 0 或 Data == nil 判断。
	Data map[string]any
	// Metadata trace_id、run_id、node_id 等追踪信息。
	// nil 时视为空 map，调用方应使用 len(Metadata) == 0 或 Metadata == nil 判断。
	Metadata map[string]string
}

// StepOutput 步骤输出。
type StepOutput struct {
	Data    map[string]any    // 输出数据
	Usage   *types.TokenUsage // 可选，LLM 步骤填充
	Latency time.Duration
	Agent   *AgentExecutionMetadata // 可选，Agent 步骤填充
}

// AgentExecutionMetadata carries execution metrics from an AgentStep back
// to the workflow layer for cost aggregation and observability.
type AgentExecutionMetadata struct {
	AgentID      string        `json:"agent_id,omitempty"`
	TokensUsed   int           `json:"tokens_used,omitempty"`
	Cost         float64       `json:"cost,omitempty"`
	Duration     time.Duration `json:"duration,omitempty"`
	FinishReason string        `json:"finish_reason,omitempty"`
}

type CheckpointSnapshot struct {
	StepID   string         `json:"step_id"`
	Version  int            `json:"version"`
	State    map[string]any `json:"state"`
	Checksum string         `json:"checksum,omitempty"`
}

type OrchestrationPrimitive interface {
	StepProtocol
	Checkpoint() *CheckpointSnapshot
	Resume(ctx context.Context, snapshot *CheckpointSnapshot) error
	TraceID() string
}
