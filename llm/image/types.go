// Package image provides unified image generation provider interfaces.
package image

import (
	"context"
	"io"
	"time"
)

// GenerateRequest represents an image generation request.
type GenerateRequest struct {
	Prompt         string            `json:"prompt"`
	NegativePrompt string            `json:"negative_prompt,omitempty"`
	Model          string            `json:"model,omitempty"`
	N              int               `json:"n,omitempty"`               // Number of images
	Size           string            `json:"size,omitempty"`            // 1024x1024, 1792x1024, etc.
	Quality        string            `json:"quality,omitempty"`         // standard, hd
	Style          string            `json:"style,omitempty"`           // vivid, natural
	ResponseFormat string            `json:"response_format,omitempty"` // url, b64_json
	Seed           int64             `json:"seed,omitempty"`
	Steps          int               `json:"steps,omitempty"`     // For SD/Flux
	CFGScale       float64           `json:"cfg_scale,omitempty"` // Guidance scale
	Metadata       map[string]string `json:"metadata,omitempty"`
}

// GenerateResponse represents the response from image generation.
type GenerateResponse struct {
	Provider  string      `json:"provider"`
	Model     string      `json:"model"`
	Images    []ImageData `json:"images"`
	Usage     ImageUsage  `json:"usage,omitempty"`
	CreatedAt time.Time   `json:"created_at"`
}

// ImageData represents a generated image.
type ImageData struct {
	URL           string `json:"url,omitempty"`
	B64JSON       string `json:"b64_json,omitempty"`
	RevisedPrompt string `json:"revised_prompt,omitempty"`
	Seed          int64  `json:"seed,omitempty"`
}

// ImageUsage represents usage statistics.
type ImageUsage struct {
	ImagesGenerated int     `json:"images_generated"`
	Cost            float64 `json:"cost,omitempty"`
}

// EditRequest represents an image editing request.
type EditRequest struct {
	Image          io.Reader         `json:"-"`
	Mask           io.Reader         `json:"-"`
	Prompt         string            `json:"prompt"`
	Model          string            `json:"model,omitempty"`
	N              int               `json:"n,omitempty"`
	Size           string            `json:"size,omitempty"`
	ResponseFormat string            `json:"response_format,omitempty"`
	Metadata       map[string]string `json:"metadata,omitempty"`
}

// VariationRequest represents an image variation request.
type VariationRequest struct {
	Image          io.Reader         `json:"-"`
	Model          string            `json:"model,omitempty"`
	N              int               `json:"n,omitempty"`
	Size           string            `json:"size,omitempty"`
	ResponseFormat string            `json:"response_format,omitempty"`
	Metadata       map[string]string `json:"metadata,omitempty"`
}

// Provider defines the image generation provider interface.
type Provider interface {
	// Generate creates images from text prompts.
	Generate(ctx context.Context, req *GenerateRequest) (*GenerateResponse, error)

	// Edit modifies an existing image based on a prompt.
	Edit(ctx context.Context, req *EditRequest) (*GenerateResponse, error)

	// CreateVariation creates variations of an existing image.
	CreateVariation(ctx context.Context, req *VariationRequest) (*GenerateResponse, error)

	// Name returns the provider name.
	Name() string

	// SupportedSizes returns supported image sizes.
	SupportedSizes() []string
}
