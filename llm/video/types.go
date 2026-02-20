package video

import (
	"context"
	"time"
)

// VideoFormat 表示支持的视频格式.
type VideoFormat string

const (
	VideoFormatMP4  VideoFormat = "mp4"
	VideoFormatWebM VideoFormat = "webm"
	VideoFormatMOV  VideoFormat = "mov"
	VideoFormatAVI  VideoFormat = "avi"
	VideoFormatMKV  VideoFormat = "mkv"
)

// AnalyzeRequest 表示视频分析请求.
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

// AnalyzeResponse 表示视频分析的响应.
type AnalyzeResponse struct {
	Provider  string          `json:"provider"`
	Model     string          `json:"model"`
	Content   string          `json:"content"`
	Frames    []FrameAnalysis `json:"frames,omitempty"`
	Duration  float64         `json:"duration,omitempty"`
	Usage     VideoUsage      `json:"usage,omitempty"`
	CreatedAt time.Time       `json:"created_at"`
}

// FrameAnalysis 表示单帧的分析结果.
type FrameAnalysis struct {
	Timestamp   float64           `json:"timestamp"`
	Description string            `json:"description,omitempty"`
	Objects     []DetectedObject  `json:"objects,omitempty"`
	Text        string            `json:"text,omitempty"` // OCR text
	Metadata    map[string]string `json:"metadata,omitempty"`
}

// DetectedObject 表示在帧中检测到的对象.
type DetectedObject struct {
	Label       string       `json:"label"`
	Confidence  float64      `json:"confidence"`
	BoundingBox *BoundingBox `json:"bounding_box,omitempty"`
}

// BoundingBox 表示帧中的对象位置.
type BoundingBox struct {
	X      float64 `json:"x"`
	Y      float64 `json:"y"`
	Width  float64 `json:"width"`
	Height float64 `json:"height"`
}

// GenerateRequest 表示视频生成请求.
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

// GenerateResponse 表示视频生成响应.
type GenerateResponse struct {
	Provider  string      `json:"provider"`
	Model     string      `json:"model"`
	Videos    []VideoData `json:"videos"`
	Usage     VideoUsage  `json:"usage,omitempty"`
	CreatedAt time.Time   `json:"created_at"`
}

// VideoData 表示一个已生成的视频.
type VideoData struct {
	URL           string  `json:"url,omitempty"`
	B64JSON       string  `json:"b64_json,omitempty"`
	Duration      float64 `json:"duration,omitempty"`
	Width         int     `json:"width,omitempty"`
	Height        int     `json:"height,omitempty"`
	RevisedPrompt string  `json:"revised_prompt,omitempty"`
}

// VideoUsage 表示使用统计.
type VideoUsage struct {
	VideosGenerated int     `json:"videos_generated"`
	DurationSeconds float64 `json:"duration_seconds"`
	Cost            float64 `json:"cost,omitempty"`
}

// Provider 定义视频处理提供者接口.
type Provider interface {
	// Analyze 处理和理解视频内容.
	Analyze(ctx context.Context, req *AnalyzeRequest) (*AnalyzeResponse, error)

	// Generate 从文本/图像提示生成视频.
	Generate(ctx context.Context, req *GenerateRequest) (*GenerateResponse, error)

	// Name 返回提供者名称.
	Name() string

	// SupportedFormats 返回支持用于分析的视频格式.
	SupportedFormats() []VideoFormat

	// SupportsGeneration 返回提供者是否支持视频生成.
	SupportsGeneration() bool
}
