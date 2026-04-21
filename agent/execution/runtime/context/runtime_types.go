package context

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/BaSui01/agentflow/types"
)

type SegmentType string

const (
	SegmentSystem       SegmentType = "system"
	SegmentEphemeral    SegmentType = "ephemeral"
	SegmentSkill        SegmentType = "skill"
	SegmentMemory       SegmentType = "memory"
	SegmentConversation SegmentType = "conversation"
	SegmentRetrieval    SegmentType = "retrieval"
	SegmentToolState    SegmentType = "tool_state"
	SegmentInput        SegmentType = "input"
	SegmentSummary      SegmentType = "summary"
)

type ContextSegment struct {
	ID        string
	Type      SegmentType
	Role      types.Role
	Content   string
	Source    string
	Priority  int
	Sticky    bool
	TokenCost int
	Metadata  map[string]any
}

type RetrievalItem struct {
	Title   string
	Content string
	Source  string
	Score   float64
}

type ToolState struct {
	ToolName   string
	Summary    string
	ArtifactID string
}

type PromptLayer struct {
	ID       string
	Type     SegmentType
	Role     types.Role
	Content  string
	Priority int
	Sticky   bool
	Metadata map[string]any
}

type AssembleRequest struct {
	SystemPrompt      string
	AdditionalContext map[string]any
	EphemeralLayers   []PromptLayer
	SkillContext      []string
	MemoryContext     []string
	Conversation      []types.Message
	Retrieval         []RetrievalItem
	ToolState         []ToolState
	UserInput         string
	Query             string
}

type ContextPlan struct {
	Budget            int                 `json:"budget"`
	Used              int                 `json:"used"`
	Strategy          string              `json:"strategy"`
	CompressionReason string              `json:"compression_reason,omitempty"`
	Breakdown         map[SegmentType]int `json:"breakdown,omitempty"`
	AppliedLayers     []PromptLayerMeta   `json:"applied_layers,omitempty"`
}

type PromptLayerMeta struct {
	ID        string         `json:"id"`
	Type      SegmentType    `json:"type"`
	Priority  int            `json:"priority,omitempty"`
	Sticky    bool           `json:"sticky,omitempty"`
	TokenCost int            `json:"token_cost,omitempty"`
	Metadata  map[string]any `json:"metadata,omitempty"`
}

type AssembleResult struct {
	Messages           []types.Message  `json:"messages"`
	SegmentsKept       []ContextSegment `json:"segments_kept,omitempty"`
	SegmentsDropped    []ContextSegment `json:"segments_dropped,omitempty"`
	SegmentsSummarized []ContextSegment `json:"segments_summarized,omitempty"`
	TokensBefore       int              `json:"tokens_before"`
	TokensAfter        int              `json:"tokens_after"`
	Plan               ContextPlan      `json:"plan"`
}

type messageSummarizer interface {
	Summarize(context.Context, []types.Message) (string, error)
}

type summaryFuncAdapter struct {
	fn func(context.Context, []types.Message) (string, error)
}

func (a summaryFuncAdapter) Summarize(ctx context.Context, messages []types.Message) (string, error) {
	return a.fn(ctx, messages)
}

func ConfigFromAgentConfig(cfg types.AgentConfig) AgentContextConfig {
	options := cfg.ExecutionOptions()
	modelCfg := DefaultAgentContextConfig(options.Model.Model)
	if options.Control.Context == nil {
		return modelCfg
	}
	out := modelCfg
	out.Enabled = options.Control.Context.Enabled
	if options.Control.Context.MaxContextTokens > 0 {
		out.MaxContextTokens = options.Control.Context.MaxContextTokens
	}
	if options.Control.Context.ReserveForOutput > 0 {
		out.ReserveForOutput = options.Control.Context.ReserveForOutput
	}
	if options.Control.Context.SoftLimit > 0 {
		out.SoftLimit = options.Control.Context.SoftLimit
	}
	if options.Control.Context.WarnLimit > 0 {
		out.WarnLimit = options.Control.Context.WarnLimit
	}
	if options.Control.Context.HardLimit > 0 {
		out.HardLimit = options.Control.Context.HardLimit
	}
	if options.Control.Context.TargetUsage > 0 {
		out.TargetUsage = options.Control.Context.TargetUsage
	}
	if options.Control.Context.KeepLastN > 0 {
		out.KeepLastN = options.Control.Context.KeepLastN
	}
	out.KeepSystem = options.Control.Context.KeepSystem || modelCfg.KeepSystem
	out.EnableMetrics = options.Control.Context.EnableMetrics || modelCfg.EnableMetrics
	out.EnableSummarize = options.Control.Context.EnableSummarize || modelCfg.EnableSummarize
	if options.Control.Context.MemoryBudgetRatio > 0 {
		out.MemoryBudgetRatio = options.Control.Context.MemoryBudgetRatio
	}
	if options.Control.Context.RetrievalBudgetRatio > 0 {
		out.RetrievalBudgetRatio = options.Control.Context.RetrievalBudgetRatio
	}
	if options.Control.Context.ToolStateBudgetRatio > 0 {
		out.ToolStateBudgetRatio = options.Control.Context.ToolStateBudgetRatio
	}
	return out
}

func AdditionalContextText(values map[string]any) string {
	if len(values) == 0 {
		return ""
	}
	data, err := json.Marshal(values)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}
