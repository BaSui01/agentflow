package usecase

import (
	"encoding/json"
	"time"

	"github.com/BaSui01/agentflow/types"
)

// ChatRequest is the usecase-layer chat input DTO.
type ChatRequest struct {
	TraceID                          string
	TenantID                         string
	UserID                           string
	Model                            string
	Provider                         string
	RoutePolicy                      string
	EndpointMode                     string
	Messages                         []Message
	MaxTokens                        int
	Temperature                      float32
	TopP                             float32
	FrequencyPenalty                 *float32
	PresencePenalty                  *float32
	RepetitionPenalty                *float32
	N                                *int
	LogProbs                         *bool
	TopLogProbs                      *int
	Stop                             []string
	Tools                            []ToolSchema
	ToolChoice                       any
	ResponseFormat                   *ResponseFormat
	StreamOptions                    *StreamOptions
	ParallelToolCalls                *bool
	ServiceTier                      *string
	User                             string
	MaxCompletionTokens              *int
	ReasoningEffort                  string
	ReasoningSummary                 string
	ReasoningDisplay                 string
	InferenceSpeed                   string
	Store                            *bool
	Modalities                       []string
	WebSearchOptions                 *WebSearchOptions
	PromptCacheKey                   string
	PromptCacheRetention             string
	CacheControl                     *CacheControl
	CachedContent                    string
	IncludeServerSideToolInvocations *bool
	PreviousResponseID               string
	ConversationID                   string
	Include                          []string
	Truncation                       string
	Timeout                          string
	Metadata                         map[string]string
	Tags                             []string
}

type ResponseFormat struct {
	Type       string
	JSONSchema *ResponseFormatJSONSchema
}

type ResponseFormatJSONSchema struct {
	Name        string
	Description string
	Schema      map[string]any
	Strict      *bool
}

type StreamOptions struct {
	IncludeUsage      bool
	ChunkIncludeUsage bool
}

type WebSearchOptions struct {
	SearchContextSize string
	UserLocation      *WebSearchLocation
	AllowedDomains    []string
	BlockedDomains    []string
	MaxUses           int
}

type WebSearchLocation struct {
	Type     string
	Country  string
	Region   string
	City     string
	Timezone string
}

type CacheControl struct {
	Type string
	TTL  string
}

// ChatResponse is the usecase-layer chat output DTO.
type ChatResponse struct {
	ID        string
	Provider  string
	Model     string
	Choices   []ChatChoice
	Usage     ChatUsage
	CreatedAt time.Time
}

type ChatChoice struct {
	Index        int
	FinishReason string
	Message      Message
}

type ChatUsage struct {
	PromptTokens            int
	CompletionTokens        int
	TotalTokens             int
	PromptTokensDetails     *PromptTokensDetails
	CompletionTokensDetails *CompletionTokensDetails
}

type PromptTokensDetails struct {
	CachedTokens        int
	CacheCreationTokens int
	AudioTokens         int
}

type CompletionTokensDetails struct {
	ReasoningTokens          int
	AudioTokens              int
	AcceptedPredictionTokens int
	RejectedPredictionTokens int
}

type Message struct {
	Role               string
	Content            string
	ReasoningContent   *string
	ReasoningSummaries []types.ReasoningSummary
	OpaqueReasoning    []types.OpaqueReasoning
	ThinkingBlocks     []types.ThinkingBlock
	Refusal            *string
	Name               string
	ToolCalls          []types.ToolCall
	ToolCallID         string
	IsToolError        bool
	Images             []ImageContent
	Videos             []types.VideoContent
	Annotations        []types.Annotation
	Metadata           any
	Timestamp          time.Time
}

type ImageContent struct {
	Type string
	URL  string
	Data string
}

type ToolSchema struct {
	Type        string
	Name        string
	Description string
	Parameters  json.RawMessage
	Format      *ToolFormat
	Strict      *bool
	Version     string
}

type ToolFormat struct {
	Type       string
	Syntax     string
	Definition string
}
