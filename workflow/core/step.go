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
	StepTypeCode             StepType = "code"
	StepTypeAgent            StepType = "agent"
	StepTypeHybridRetrieve   StepType = "hybrid_retrieve"
	StepTypeMultiHopRetrieve StepType = "multi_hop_retrieve"
	StepTypeRerank           StepType = "rerank"
	StepTypeOrchestration    StepType = "orchestration"
	StepTypeChain            StepType = "chain"
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
	Data     map[string]any    // 上游步骤输出 / 用户输入
	Metadata map[string]string // trace_id/run_id/node_id 等
}

// StepOutput 步骤输出。
type StepOutput struct {
	Data    map[string]any    // 输出数据
	Usage   *types.TokenUsage // 可选，LLM 步骤填充
	Latency time.Duration
}
