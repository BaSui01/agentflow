package usecase

import "github.com/BaSui01/agentflow/llm/capabilities/image"

import "github.com/BaSui01/agentflow/llm/capabilities/video"

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
