// 包图像提供统一的图像生成提供者接口.
package image

import (
	"context"
	"io"
	"time"
)

// 生成请求代表图像生成请求 。
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

// 生成响应(Generate Response)代表图像生成的响应.
type GenerateResponse struct {
	Provider  string      `json:"provider"`
	Model     string      `json:"model"`
	Images    []ImageData `json:"images"`
	Usage     ImageUsage  `json:"usage,omitempty"`
	CreatedAt time.Time   `json:"created_at"`
}

// ImageData代表生成的图像.
type ImageData struct {
	URL           string `json:"url,omitempty"`
	B64JSON       string `json:"b64_json,omitempty"`
	RevisedPrompt string `json:"revised_prompt,omitempty"`
	Seed          int64  `json:"seed,omitempty"`
}

// ImageUsage代表使用统计.
type ImageUsage struct {
	ImagesGenerated int     `json:"images_generated"`
	Cost            float64 `json:"cost,omitempty"`
}

// 编辑请求代表图像编辑请求 。
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

// 变异请求代表图像变异请求.
type VariationRequest struct {
	Image          io.Reader         `json:"-"`
	Model          string            `json:"model,omitempty"`
	N              int               `json:"n,omitempty"`
	Size           string            `json:"size,omitempty"`
	ResponseFormat string            `json:"response_format,omitempty"`
	Metadata       map[string]string `json:"metadata,omitempty"`
}

// 提供方定义了图像生成提供者接口.
type Provider interface {
	// 从文本提示生成图像 。
	Generate(ctx context.Context, req *GenerateRequest) (*GenerateResponse, error)

	// 编辑根据快讯修改已存在的图像。
	Edit(ctx context.Context, req *EditRequest) (*GenerateResponse, error)

	// CreateVariation 创建现有图像的变体.
	CreateVariation(ctx context.Context, req *VariationRequest) (*GenerateResponse, error)

	// 名称返回提供者名称 。
	Name() string

	// 支持的返回大小支持的图像大小 。
	SupportedSizes() []string
}
