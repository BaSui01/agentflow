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
	modelCfg := DefaultAgentContextConfig(cfg.LLM.Model)
	if cfg.Context == nil {
		return modelCfg
	}
	out := modelCfg
	out.Enabled = cfg.Context.Enabled
	if cfg.Context.MaxContextTokens > 0 {
		out.MaxContextTokens = cfg.Context.MaxContextTokens
	}
	if cfg.Context.ReserveForOutput > 0 {
		out.ReserveForOutput = cfg.Context.ReserveForOutput
	}
	if cfg.Context.SoftLimit > 0 {
		out.SoftLimit = cfg.Context.SoftLimit
	}
	if cfg.Context.WarnLimit > 0 {
		out.WarnLimit = cfg.Context.WarnLimit
	}
	if cfg.Context.HardLimit > 0 {
		out.HardLimit = cfg.Context.HardLimit
	}
	if cfg.Context.TargetUsage > 0 {
		out.TargetUsage = cfg.Context.TargetUsage
	}
	if cfg.Context.KeepLastN > 0 {
		out.KeepLastN = cfg.Context.KeepLastN
	}
	out.KeepSystem = cfg.Context.KeepSystem || modelCfg.KeepSystem
	out.EnableMetrics = cfg.Context.EnableMetrics || modelCfg.EnableMetrics
	out.EnableSummarize = cfg.Context.EnableSummarize || modelCfg.EnableSummarize
	if cfg.Context.MemoryBudgetRatio > 0 {
		out.MemoryBudgetRatio = cfg.Context.MemoryBudgetRatio
	}
	if cfg.Context.RetrievalBudgetRatio > 0 {
		out.RetrievalBudgetRatio = cfg.Context.RetrievalBudgetRatio
	}
	if cfg.Context.ToolStateBudgetRatio > 0 {
		out.ToolStateBudgetRatio = cfg.Context.ToolStateBudgetRatio
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
