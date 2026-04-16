package context

import (
	"context"
	"fmt"
	"strings"

	"github.com/BaSui01/agentflow/types"
	"go.uber.org/zap"
)

type Assembler struct {
	config     AgentContextConfig
	tokenizer  types.Tokenizer
	summarizer messageSummarizer
	logger     *zap.Logger
}

func newAssembler(cfg AgentContextConfig, logger *zap.Logger) *Assembler {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &Assembler{
		config:    cfg,
		tokenizer: types.NewEstimateTokenizer(),
		logger:    logger,
	}
}

func (a *Assembler) Assemble(ctx context.Context, req *AssembleRequest) (*AssembleResult, error) {
	if req == nil {
		return &AssembleResult{}, nil
	}

	segments := a.buildSegments(req)
	planBudget := a.config.MaxContextTokens - a.config.ReserveForOutput
	if planBudget < 0 {
		planBudget = 0
	}
	before := a.estimateSegmentTokens(segments)
	kept, dropped, summarized, reason, err := a.fitSegments(ctx, segments, req.Query, planBudget)
	if err != nil {
		return nil, err
	}
	messages := renderSegments(kept)
	after := a.tokenizer.CountMessagesTokens(messages)

	breakdown := make(map[SegmentType]int)
	for _, seg := range kept {
		breakdown[seg.Type] += seg.TokenCost
	}

	return &AssembleResult{
		Messages:           messages,
		SegmentsKept:       kept,
		SegmentsDropped:    dropped,
		SegmentsSummarized: summarized,
		TokensBefore:       before,
		TokensAfter:        after,
		Plan: ContextPlan{
			Budget:            planBudget,
			Used:              after,
			Strategy:          string(a.config.Strategy),
			CompressionReason: reason,
			Breakdown:         breakdown,
		},
	}, nil
}

func (a *Assembler) buildSegments(req *AssembleRequest) []ContextSegment {
	segments := make([]ContextSegment, 0, 8+len(req.Conversation)+len(req.MemoryContext)+len(req.Retrieval)+len(req.ToolState))
	if prompt := strings.TrimSpace(req.SystemPrompt); prompt != "" {
		if extra := additionalContextText(req.AdditionalContext); extra != "" {
			prompt += "\n\n<additional_context>\n" + extra + "\n</additional_context>"
		}
		segments = append(segments, a.newSegment("system", SegmentSystem, types.RoleSystem, prompt, 100, true, nil))
	}

	for i, item := range req.MemoryContext {
		content := strings.TrimSpace(item)
		if content == "" {
			continue
		}
		segments = append(segments, a.newSegment(fmt.Sprintf("memory-%d", i), SegmentMemory, types.RoleSystem, content, 60, false, nil))
	}
	keepFrom := len(req.Conversation) - a.config.KeepLastN
	for i, msg := range req.Conversation {
		sticky := a.config.KeepLastN > 0 && i >= keepFrom
		segments = append(segments, a.newSegment(fmt.Sprintf("conversation-%d", i), SegmentConversation, msg.Role, msg.Content, 40, sticky, nil))
	}
	for i, item := range req.Retrieval {
		content := strings.TrimSpace(item.Content)
		if content == "" {
			continue
		}
		metadata := map[string]any{"title": item.Title, "source": item.Source, "score": item.Score}
		segments = append(segments, a.newSegment(fmt.Sprintf("retrieval-%d", i), SegmentRetrieval, types.RoleSystem, content, 30, false, metadata))
	}
	for i, item := range req.ToolState {
		content := strings.TrimSpace(item.Summary)
		if content == "" {
			continue
		}
		metadata := map[string]any{"tool_name": item.ToolName, "artifact_id": item.ArtifactID}
		segments = append(segments, a.newSegment(fmt.Sprintf("tool-%d", i), SegmentToolState, types.RoleSystem, content, 35, false, metadata))
	}
	if input := strings.TrimSpace(req.UserInput); input != "" {
		segments = append(segments, a.newSegment("input", SegmentInput, types.RoleUser, input, 90, true, nil))
	}
	return segments
}

func (a *Assembler) newSegment(id string, segmentType SegmentType, role types.Role, content string, priority int, sticky bool, metadata map[string]any) ContextSegment {
	msg := types.Message{Role: role, Content: content}
	return ContextSegment{
		ID:        id,
		Type:      segmentType,
		Role:      role,
		Content:   content,
		Source:    string(segmentType),
		Priority:  priority,
		Sticky:    sticky,
		TokenCost: a.tokenizer.CountMessageTokens(msg),
		Metadata:  metadata,
	}
}

func (a *Assembler) estimateSegmentTokens(segments []ContextSegment) int {
	total := 0
	for _, seg := range segments {
		total += seg.TokenCost
	}
	return total
}

func (a *Assembler) fitSegments(ctx context.Context, segments []ContextSegment, query string, budget int) ([]ContextSegment, []ContextSegment, []ContextSegment, string, error) {
	kept := append([]ContextSegment(nil), segments...)
	if a.estimateSegmentTokens(kept) <= budget {
		return kept, nil, nil, "", nil
	}

	var reason string
	var dropped []ContextSegment
	var summarized []ContextSegment

	if a.config.EnableSummarize && a.summarizer != nil {
		var summarizable []ContextSegment
		var remainder []ContextSegment
		for _, seg := range kept {
			if seg.Type == SegmentConversation {
				summarizable = append(summarizable, seg)
				continue
			}
			remainder = append(remainder, seg)
		}
		if len(summarizable) > 0 {
			keepTail := a.config.KeepLastN
			if keepTail > len(summarizable) {
				keepTail = len(summarizable)
			}
			prefix := summarizable[:len(summarizable)-keepTail]
			if len(prefix) > 0 {
				msgs := renderSegments(prefix)
				summary, err := a.summarizer.Summarize(ctx, msgs)
				if err == nil && strings.TrimSpace(summary) != "" {
					summarySeg := a.newSegment("summary", SegmentSummary, types.RoleSystem, summary, 70, false, map[string]any{"query": query})
					summarized = append(summarized, summarySeg)
					dropped = append(dropped, prefix...)
					kept = append(remainder, summarySeg)
					for _, seg := range summarizable[len(summarizable)-keepTail:] {
						kept = append(kept, seg)
					}
					reason = "summary"
				}
			}
		}
	}

	if a.estimateSegmentTokens(kept) <= budget {
		return kept, dropped, summarized, reason, nil
	}

	priorityOrder := []SegmentType{SegmentRetrieval, SegmentToolState, SegmentMemory, SegmentConversation}
	for _, segmentType := range priorityOrder {
		for i := 0; i < len(kept) && a.estimateSegmentTokens(kept) > budget; {
			if kept[i].Sticky || kept[i].Type != segmentType {
				i++
				continue
			}
			dropped = append(dropped, kept[i])
			kept = append(kept[:i], kept[i+1:]...)
			reason = "drop_" + string(segmentType)
		}
	}

	if a.estimateSegmentTokens(kept) <= budget {
		return kept, dropped, summarized, reason, nil
	}

	for i := range kept {
		if kept[i].Sticky {
			continue
		}
		maxTokens := budget / max(1, len(kept))
		if maxTokens < 64 {
			maxTokens = 64
		}
		msgs := []types.Message{{Role: kept[i].Role, Content: kept[i].Content}}
		truncated := truncateMessages(msgs, maxTokens, a.tokenizer)
		kept[i].Content = truncated[0].Content
		kept[i].TokenCost = a.tokenizer.CountMessageTokens(types.Message{Role: kept[i].Role, Content: kept[i].Content})
		reason = "truncate"
		if a.estimateSegmentTokens(kept) <= budget {
			break
		}
	}

	return kept, dropped, summarized, reason, nil
}

func renderSegments(segments []ContextSegment) []types.Message {
	messages := make([]types.Message, 0, len(segments))
	for _, seg := range segments {
		messages = append(messages, types.Message{Role: seg.Role, Content: seg.Content})
	}
	return messages
}

func truncateMessages(msgs []types.Message, maxTokens int, tokenizer types.Tokenizer) []types.Message {
	result := make([]types.Message, len(msgs))
	copy(result, msgs)
	for i := range result {
		msgTokens := tokenizer.CountMessageTokens(result[i])
		if msgTokens <= maxTokens {
			continue
		}
		ratio := float64(maxTokens) / float64(msgTokens) * 0.9
		targetLen := int(float64(len(result[i].Content)) * ratio)
		if targetLen < 64 {
			targetLen = 64
		}
		if targetLen < len(result[i].Content) {
			result[i].Content = result[i].Content[:targetLen] + "\n...[truncated]"
		}
	}
	return result
}

func max(aValue, bValue int) int {
	if aValue > bValue {
		return aValue
	}
	return bValue
}
