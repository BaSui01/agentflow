// Package video provides unified video processing provider interfaces.
package video

import (
	"context"
	"time"
)

// VideoFormat represents supported video formats.
type VideoFormat string

const (
	VideoFormatMP4  VideoFormat = "mp4"
	VideoFormatWebM VideoFormat = "webm"
	VideoFormatMOV  VideoFormat = "mov"
	VideoFormatAVI  VideoFormat = "avi"
	VideoFormatMKV  VideoFormat = "mkv"
)

// AnalyzeRequest represents a video analysis request.
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

// AnalyzeResponse represents the response from video analysis.
type AnalyzeResponse struct {
	Provider  string          `json:"provider"`
	Model     string          `json:"model"`
	Content   string          `json:"content"`
	Frames    []FrameAnalysis `json:"frames,omitempty"`
	Duration  float64         `json:"duration,omitempty"`
	Usage     VideoUsage      `json:"usage,omitempty"`
	CreatedAt time.Time       `json:"created_at"`
}

// FrameAnalysis represents analysis of a single frame.
type FrameAnalysis struct {
	Timestamp   float64           `json:"timestamp"`
	Description string            `json:"description,omitempty"`
	Objects     []DetectedObject  `json:"objects,omitempty"`
	Text        string            `json:"text,omitempty"` // OCR text
	Metadata    map[string]string `json:"metadata,omitempty"`
}

// DetectedObject represents an object detected in a frame.
type DetectedObject struct {
	Label       string       `json:"label"`
	Confidence  float64      `json:"confidence"`
	BoundingBox *BoundingBox `json:"bounding_box,omitempty"`
}

// BoundingBox represents object location in frame.
type BoundingBox struct {
	X      float64 `json:"x"`
	Y      float64 `json:"y"`
	Width  float64 `json:"width"`
	Height float64 `json:"height"`
}

// GenerateRequest represents a video generation request.
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

// GenerateResponse represents the response from video generation.
type GenerateResponse struct {
	Provider  string      `json:"provider"`
	Model     string      `json:"model"`
	Videos    []VideoData `json:"videos"`
	Usage     VideoUsage  `json:"usage,omitempty"`
	CreatedAt time.Time   `json:"created_at"`
}

// VideoData represents a generated video.
type VideoData struct {
	URL           string  `json:"url,omitempty"`
	B64JSON       string  `json:"b64_json,omitempty"`
	Duration      float64 `json:"duration,omitempty"`
	Width         int     `json:"width,omitempty"`
	Height        int     `json:"height,omitempty"`
	RevisedPrompt string  `json:"revised_prompt,omitempty"`
}

// VideoUsage represents usage statistics.
type VideoUsage struct {
	VideosGenerated int     `json:"videos_generated"`
	DurationSeconds float64 `json:"duration_seconds"`
	Cost            float64 `json:"cost,omitempty"`
}

// Provider defines the video processing provider interface.
type Provider interface {
	// Analyze processes and understands video content.
	Analyze(ctx context.Context, req *AnalyzeRequest) (*AnalyzeResponse, error)

	// Generate creates videos from text/image prompts.
	Generate(ctx context.Context, req *GenerateRequest) (*GenerateResponse, error)

	// Name returns the provider name.
	Name() string

	// SupportedFormats returns supported video formats for analysis.
	SupportedFormats() []VideoFormat

	// SupportsGeneration returns whether the provider supports video generation.
	SupportsGeneration() bool
}
