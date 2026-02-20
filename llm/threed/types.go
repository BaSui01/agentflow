// 3d包提供了AI 三维模型生成能力.
package threed

import (
	"context"
	"time"
)

// ThreeDProvider为3D模型生成定义了接口.
type ThreeDProvider interface {
	Name() string
	Generate(ctx context.Context, req *GenerateRequest) (*GenerateResponse, error)
}

// 生成请求代表一个 三维生成请求 。
type GenerateRequest struct {
	Prompt      string   `json:"prompt,omitempty"`       // Text description
	Image       string   `json:"image,omitempty"`        // Base64 image for image-to-3D
	ImageURL    string   `json:"image_url,omitempty"`    // Image URL
	Images      []string `json:"images,omitempty"`       // Multi-view images
	Model       string   `json:"model,omitempty"`        // Model to use
	Format      string   `json:"format,omitempty"`       // Output format (glb, fbx, obj, usdz)
	Style       string   `json:"style,omitempty"`        // Style preset
	Quality     string   `json:"quality,omitempty"`      // Quality level (draft, standard, high)
	TextureSize int      `json:"texture_size,omitempty"` // Texture resolution
}

// 生成响应(Generate Response)代表3D生成响应.
type GenerateResponse struct {
	Provider  string      `json:"provider"`
	Model     string      `json:"model"`
	Models    []ModelData `json:"models"`
	Usage     ThreeDUsage `json:"usage"`
	CreatedAt time.Time   `json:"created_at"`
}

// ModelData代表生成的3D模型.
type ModelData struct {
	ID           string `json:"id,omitempty"`
	URL          string `json:"url,omitempty"`           // Download URL
	B64Data      string `json:"b64_data,omitempty"`      // Base64 encoded model
	Format       string `json:"format"`                  // File format
	TextureURL   string `json:"texture_url,omitempty"`   // Texture download URL
	ThumbnailURL string `json:"thumbnail_url,omitempty"` // Preview image
}

// 3DUsage包含使用信息.
type ThreeDUsage struct {
	ModelsGenerated int     `json:"models_generated"`
	Credits         float64 `json:"credits,omitempty"`
}
