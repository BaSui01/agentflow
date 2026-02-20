// 包视频提供统一的视频处理提供者接口.
package video

import (
	"context"
	"time"
)

// VideoFormat 代表支持的视频格式.
type VideoFormat string

const (
	VideoFormatMP4  VideoFormat = "mp4"
	VideoFormatWebM VideoFormat = "webm"
	VideoFormatMOV  VideoFormat = "mov"
	VideoFormatAVI  VideoFormat = "avi"
	VideoFormatMKV  VideoFormat = "mkv"
)

// 分析请求是一个视频分析请求。
type AnalyzeRequest struct {
	VideoURL    string            `json:"video_url,omitempty"`
	VideoData   string            `json:"video_data,omitempty"` // Base64 encoded
	VideoFormat VideoFormat       `json:"video_format,omitempty"`
	Prompt      string            `json:"prompt"`
	Model       string            `json:"model,omitempty"`
	MaxFrames   int               `json:"max_frames,omitempty"` // Max frames to analyze
	Interval    float64           `json:"interval,omitempty"`   // Frame interval in seconds
	StartTime   float64           `json:"start_time,omitempty"` // Start time in seconds
	EndTime     float64           `json:"end_time,omitempty"`   // End time in seconds
	Metadata    map[string]string `json:"metadata,omitempty"`
}

// Analysis Response代表视频分析的反应.
type AnalyzeResponse struct {
	Provider  string          `json:"provider"`
	Model     string          `json:"model"`
	Content   string          `json:"content"`
	Frames    []FrameAnalysis `json:"frames,omitempty"`
	Duration  float64         `json:"duration,omitempty"`
	Usage     VideoUsage      `json:"usage,omitempty"`
	CreatedAt time.Time       `json:"created_at"`
}

// 框架分析是单一框架的分析。
type FrameAnalysis struct {
	Timestamp   float64           `json:"timestamp"`
	Description string            `json:"description,omitempty"`
	Objects     []DetectedObject  `json:"objects,omitempty"`
	Text        string            `json:"text,omitempty"` // OCR text
	Metadata    map[string]string `json:"metadata,omitempty"`
}

// 被检测对象代表在帧中检测到的对象.
type DetectedObject struct {
	Label       string       `json:"label"`
	Confidence  float64      `json:"confidence"`
	BoundingBox *BoundingBox `json:"bounding_box,omitempty"`
}

// BboundingBox代表框中的对象位置.
type BoundingBox struct {
	X      float64 `json:"x"`
	Y      float64 `json:"y"`
	Width  float64 `json:"width"`
	Height float64 `json:"height"`
}

// 生成请求代表视频生成请求.
type GenerateRequest struct {
	Prompt         string            `json:"prompt"`
	NegativePrompt string            `json:"negative_prompt,omitempty"`
	Model          string            `json:"model,omitempty"`
	Duration       float64           `json:"duration,omitempty"`     // Duration in seconds
	AspectRatio    string            `json:"aspect_ratio,omitempty"` // 16:9, 9:16, 1:1
	Resolution     string            `json:"resolution,omitempty"`   // 720p, 1080p
	FPS            int               `json:"fps,omitempty"`
	Seed           int64             `json:"seed,omitempty"`
	Image          string            `json:"image,omitempty"`           // Image-to-video base64
	ImageURL       string            `json:"image_url,omitempty"`       // Image-to-video URL
	ResponseFormat string            `json:"response_format,omitempty"` // url, b64_json
	Metadata       map[string]string `json:"metadata,omitempty"`
}

// GenerateResponse代表了视频生成的响应.
type GenerateResponse struct {
	Provider  string      `json:"provider"`
	Model     string      `json:"model"`
	Videos    []VideoData `json:"videos"`
	Usage     VideoUsage  `json:"usage,omitempty"`
	CreatedAt time.Time   `json:"created_at"`
}

// VideoData代表一个已生成的视频.
type VideoData struct {
	URL           string  `json:"url,omitempty"`
	B64JSON       string  `json:"b64_json,omitempty"`
	Duration      float64 `json:"duration,omitempty"`
	Width         int     `json:"width,omitempty"`
	Height        int     `json:"height,omitempty"`
	RevisedPrompt string  `json:"revised_prompt,omitempty"`
}

// VideoUsage代表使用统计.
type VideoUsage struct {
	VideosGenerated int     `json:"videos_generated"`
	DurationSeconds float64 `json:"duration_seconds"`
	Cost            float64 `json:"cost,omitempty"`
}

// 提供方定义了视频处理提供者接口.
type Provider interface {
	// 分析过程和理解视频内容.
	Analyze(ctx context.Context, req *AnalyzeRequest) (*AnalyzeResponse, error)

	// 从文本/图像提示生成视频 。
	Generate(ctx context.Context, req *GenerateRequest) (*GenerateResponse, error)

	// 名称返回提供者名称 。
	Name() string

	// 支持Formats返回支持用于分析的视频格式.
	SupportedFormats() []VideoFormat

	// 支持 Generation 返回提供者是否支持视频生成 。
	SupportsGeneration() bool
}
