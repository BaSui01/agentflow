package avatar

import (
	"context"
	"time"

	"github.com/BaSui01/agentflow/types"
)

// Provider 定义数字人/Avatar 能力接口。
type Provider interface {
	Name() string
	Generate(ctx context.Context, req *GenerateRequest) (*GenerateResponse, error)
}

// GenerateRequest 表示 Avatar 生成请求。
type GenerateRequest struct {
	Prompt           string                  `json:"prompt,omitempty"`
	Script           string                  `json:"script,omitempty"`
	Model            string                  `json:"model,omitempty"`
	AvatarID         string                  `json:"avatar_id,omitempty"`
	Voice            string                  `json:"voice,omitempty"`
	Duration         float64                 `json:"duration,omitempty"`
	Resolution       string                  `json:"resolution,omitempty"`
	AudioURL         string                  `json:"audio_url,omitempty"`
	ImageURL         string                  `json:"image_url,omitempty"`
	DriveVideoURL    string                  `json:"drive_video_url,omitempty"`
	DriveMode        types.AvatarDriveMode   `json:"drive_mode,omitempty"`
	NarrationProfile *types.NarrationProfile `json:"narration_profile,omitempty"`
	Metadata         map[string]string       `json:"metadata,omitempty"`
}

// GenerateResponse 表示 Avatar 生成响应。
type GenerateResponse struct {
	Provider  string       `json:"provider"`
	Model     string       `json:"model"`
	Assets    []AvatarData `json:"assets"`
	Usage     AvatarUsage  `json:"usage,omitempty"`
	CreatedAt time.Time    `json:"created_at,omitempty"`
}

// AvatarData 表示单个生成结果。
type AvatarData struct {
	ID           string `json:"id,omitempty"`
	VideoURL     string `json:"video_url,omitempty"`
	AudioURL     string `json:"audio_url,omitempty"`
	ThumbnailURL string `json:"thumbnail_url,omitempty"`
	Transcript   string `json:"transcript,omitempty"`
	B64Video     string `json:"b64_video,omitempty"`
}

// AvatarUsage 表示用量统计。
type AvatarUsage struct {
	AvatarsGenerated int     `json:"avatars_generated"`
	DurationSeconds  float64 `json:"duration_seconds,omitempty"`
	Credits          float64 `json:"credits,omitempty"`
}
