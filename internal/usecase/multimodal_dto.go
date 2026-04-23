package usecase

import (
	"github.com/BaSui01/agentflow/llm/capabilities/image"
	"github.com/BaSui01/agentflow/llm/capabilities/video"
	llm "github.com/BaSui01/agentflow/llm/core"
	"github.com/BaSui01/agentflow/types"
)

type MultimodalImageRequest struct {
	Prompt            string   `json:"prompt"`
	NegativePrompt    string   `json:"negative_prompt,omitempty"`
	Model             string   `json:"model,omitempty"`
	Provider          string   `json:"provider,omitempty"`
	N                 int      `json:"n,omitempty"`
	Size              string   `json:"size,omitempty"`
	Quality           string   `json:"quality,omitempty"`
	Style             string   `json:"style,omitempty"`
	ResponseFormat    string   `json:"response_format,omitempty"`
	Advanced          bool     `json:"advanced,omitempty"`
	Stream            bool     `json:"stream,omitempty"`
	StyleTokens       []string `json:"style_tokens,omitempty"`
	QualityTokens     []string `json:"quality_tokens,omitempty"`
	ReferenceID       string   `json:"reference_id,omitempty"`
	ReferenceImageURL string   `json:"reference_image_url,omitempty"`
}

type MultimodalImageResult struct {
	Mode            string
	Provider        string
	EffectivePrompt string
	NegativePrompt  string
	Response        *image.GenerateResponse
}

type MultimodalVideoRequest struct {
	Prompt            string   `json:"prompt"`
	NegativePrompt    string   `json:"negative_prompt,omitempty"`
	Model             string   `json:"model,omitempty"`
	Provider          string   `json:"provider,omitempty"`
	Duration          float64  `json:"duration,omitempty"`
	AspectRatio       string   `json:"aspect_ratio,omitempty"`
	Resolution        string   `json:"resolution,omitempty"`
	FPS               int      `json:"fps,omitempty"`
	Seed              int64    `json:"seed,omitempty"`
	ResponseFormat    string   `json:"response_format,omitempty"`
	Advanced          bool     `json:"advanced,omitempty"`
	CallbackURL       string   `json:"callback_url,omitempty"`
	StyleTokens       []string `json:"style_tokens,omitempty"`
	Camera            string   `json:"camera,omitempty"`
	Mood              string   `json:"mood,omitempty"`
	ReferenceID       string   `json:"reference_id,omitempty"`
	ReferenceImageURL string   `json:"reference_image_url,omitempty"`
}

type MultimodalVideoResult struct {
	Mode            string
	Provider        string
	EffectivePrompt string
	Response        *video.GenerateResponse
}

type MultimodalPlanRequest struct {
	Prompt    string `json:"prompt"`
	ShotCount int    `json:"shot_count,omitempty"`
	Advanced  bool   `json:"advanced,omitempty"`
}

type MultimodalVisualPlan struct {
	Goal  string                 `json:"goal"`
	Shots []MultimodalVisualShot `json:"shots"`
}

type MultimodalVisualShot struct {
	ID          int    `json:"id"`
	Purpose     string `json:"purpose"`
	Visual      string `json:"visual"`
	Action      string `json:"action"`
	Camera      string `json:"camera"`
	DurationSec int    `json:"duration_sec"`
}

type MultimodalPlanResult struct {
	Plan *MultimodalVisualPlan
}

type MultimodalChatRequest struct {
	Model        string          `json:"model,omitempty"`
	Messages     []types.Message `json:"messages,omitempty"`
	SystemPrompt string          `json:"system_prompt,omitempty"`
	Temperature  float32         `json:"temperature,omitempty"`
	Advanced     bool            `json:"advanced,omitempty"`
	AgentMode    bool            `json:"agent_mode,omitempty"`
}

type MultimodalChatResult struct {
	Mode          string
	Response      *llm.ChatResponse
	PlannerOutput string
	FinalResponse *llm.ChatResponse
	FinalText     string
}
