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

// StreamChunk 是流式生成时的单个数据块.
// 文字 token（Text != ""）与图像数据（Image != nil）互斥出现；Done=true 表示流正常结束.
type StreamChunk struct {
	// Text 是模型的流式文字输出（思考/描述内容），在图像到达前逐步推送.
	Text string
	// Image 是生成的图像数据，仅最后一批图像 chunk 携带.
	Image *ImageData
	// Done 为 true 时表示流已正常结束（此 chunk 不携带数据）.
	Done bool
	// Err 不为 nil 时表示流异常终止.
	Err error
}

// StreamingProvider 是支持原生流式生成的可选扩展接口.
// 并非所有 Provider 都实现此接口；调用方通过类型断言检测是否支持.
// 实现方需保证：emit 按顺序调用；最后一次调用 emit 的 chunk.Done==true 或 chunk.Err!=nil.
type StreamingProvider interface {
	Provider
	// GenerateStream 启动流式生成，通过 emit 回调逐步推送 StreamChunk.
	// emit 中的 chunk.Done=true 表示流正常结束；chunk.Err!=nil 表示错误终止.
	// 实现必须在 ctx 取消后尽快退出并推送 chunk.Err=ctx.Err().
	GenerateStream(ctx context.Context, req *GenerateRequest, emit func(StreamChunk)) error
}

