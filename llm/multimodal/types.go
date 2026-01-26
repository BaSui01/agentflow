// Package multimodal provides multimodal content handling for LLM providers.
package multimodal

import (
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

// ContentType represents the type of multimodal content.
type ContentType string

const (
	ContentTypeText     ContentType = "text"
	ContentTypeImage    ContentType = "image"
	ContentTypeAudio    ContentType = "audio"
	ContentTypeVideo    ContentType = "video"
	ContentTypeDocument ContentType = "document"
)

// ImageFormat represents supported image formats.
type ImageFormat string

const (
	ImageFormatPNG  ImageFormat = "png"
	ImageFormatJPEG ImageFormat = "jpeg"
	ImageFormatGIF  ImageFormat = "gif"
	ImageFormatWebP ImageFormat = "webp"
)

// AudioFormat represents supported audio formats.
type AudioFormat string

const (
	AudioFormatMP3  AudioFormat = "mp3"
	AudioFormatWAV  AudioFormat = "wav"
	AudioFormatOGG  AudioFormat = "ogg"
	AudioFormatFLAC AudioFormat = "flac"
	AudioFormatM4A  AudioFormat = "m4a"
)

// Content represents a piece of multimodal content.
type Content struct {
	Type     ContentType `json:"type"`
	Text     string      `json:"text,omitempty"`
	ImageURL string      `json:"image_url,omitempty"`
	AudioURL string      `json:"audio_url,omitempty"`
	VideoURL string      `json:"video_url,omitempty"`

	// Base64 encoded data (alternative to URL)
	Data      string `json:"data,omitempty"`
	MediaType string `json:"media_type,omitempty"` // e.g., "image/png", "audio/mp3"

	// Metadata
	FileName   string            `json:"file_name,omitempty"`
	FileSize   int64             `json:"file_size,omitempty"`
	Duration   float64           `json:"duration,omitempty"` // For audio/video in seconds
	Dimensions *ImageDimensions  `json:"dimensions,omitempty"`
	Metadata   map[string]string `json:"metadata,omitempty"`
}

// ImageDimensions represents image dimensions.
type ImageDimensions struct {
	Width  int `json:"width"`
	Height int `json:"height"`
}

// MultimodalMessage represents a message with multiple content types.
type MultimodalMessage struct {
	Role     string    `json:"role"`
	Contents []Content `json:"contents"`
}

// NewTextContent creates a text content.
func NewTextContent(text string) Content {
	return Content{
		Type: ContentTypeText,
		Text: text,
	}
}

// NewImageURLContent creates an image content from URL.
func NewImageURLContent(url string) Content {
	return Content{
		Type:     ContentTypeImage,
		ImageURL: url,
	}
}

// NewImageBase64Content creates an image content from base64 data.
func NewImageBase64Content(data string, format ImageFormat) Content {
	return Content{
		Type:      ContentTypeImage,
		Data:      data,
		MediaType: fmt.Sprintf("image/%s", format),
	}
}

// NewAudioURLContent creates an audio content from URL.
func NewAudioURLContent(url string) Content {
	return Content{
		Type:     ContentTypeAudio,
		AudioURL: url,
	}
}

// NewAudioBase64Content creates an audio content from base64 data.
func NewAudioBase64Content(data string, format AudioFormat) Content {
	return Content{
		Type:      ContentTypeAudio,
		Data:      data,
		MediaType: fmt.Sprintf("audio/%s", format),
	}
}

// LoadImageFromFile loads an image from a file path.
func LoadImageFromFile(path string) (Content, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Content{}, fmt.Errorf("failed to read image file: %w", err)
	}

	ext := strings.ToLower(filepath.Ext(path))
	var format ImageFormat
	switch ext {
	case ".png":
		format = ImageFormatPNG
	case ".jpg", ".jpeg":
		format = ImageFormatJPEG
	case ".gif":
		format = ImageFormatGIF
	case ".webp":
		format = ImageFormatWebP
	default:
		return Content{}, fmt.Errorf("unsupported image format: %s", ext)
	}

	encoded := base64.StdEncoding.EncodeToString(data)
	content := NewImageBase64Content(encoded, format)
	content.FileName = filepath.Base(path)
	content.FileSize = int64(len(data))

	return content, nil
}

// LoadImageFromURL loads an image from a URL.
func LoadImageFromURL(url string) (Content, error) {
	resp, err := http.Get(url)
	if err != nil {
		return Content{}, fmt.Errorf("failed to fetch image: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return Content{}, fmt.Errorf("failed to fetch image: status %d", resp.StatusCode)
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return Content{}, fmt.Errorf("failed to read image data: %w", err)
	}

	contentType := resp.Header.Get("Content-Type")
	var format ImageFormat
	switch {
	case strings.Contains(contentType, "png"):
		format = ImageFormatPNG
	case strings.Contains(contentType, "jpeg"), strings.Contains(contentType, "jpg"):
		format = ImageFormatJPEG
	case strings.Contains(contentType, "gif"):
		format = ImageFormatGIF
	case strings.Contains(contentType, "webp"):
		format = ImageFormatWebP
	default:
		// Try to detect from magic bytes
		format = detectImageFormat(data)
	}

	encoded := base64.StdEncoding.EncodeToString(data)
	content := NewImageBase64Content(encoded, format)
	content.FileSize = int64(len(data))

	return content, nil
}

// LoadAudioFromFile loads an audio file.
func LoadAudioFromFile(path string) (Content, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Content{}, fmt.Errorf("failed to read audio file: %w", err)
	}

	ext := strings.ToLower(filepath.Ext(path))
	var format AudioFormat
	switch ext {
	case ".mp3":
		format = AudioFormatMP3
	case ".wav":
		format = AudioFormatWAV
	case ".ogg":
		format = AudioFormatOGG
	case ".flac":
		format = AudioFormatFLAC
	case ".m4a":
		format = AudioFormatM4A
	default:
		return Content{}, fmt.Errorf("unsupported audio format: %s", ext)
	}

	encoded := base64.StdEncoding.EncodeToString(data)
	content := NewAudioBase64Content(encoded, format)
	content.FileName = filepath.Base(path)
	content.FileSize = int64(len(data))

	return content, nil
}

func detectImageFormat(data []byte) ImageFormat {
	if len(data) < 8 {
		return ImageFormatJPEG // default
	}

	// PNG magic bytes
	if data[0] == 0x89 && data[1] == 0x50 && data[2] == 0x4E && data[3] == 0x47 {
		return ImageFormatPNG
	}

	// JPEG magic bytes
	if data[0] == 0xFF && data[1] == 0xD8 && data[2] == 0xFF {
		return ImageFormatJPEG
	}

	// GIF magic bytes
	if data[0] == 0x47 && data[1] == 0x49 && data[2] == 0x46 {
		return ImageFormatGIF
	}

	// WebP magic bytes
	if len(data) >= 12 && data[0] == 0x52 && data[1] == 0x49 && data[2] == 0x46 && data[3] == 0x46 &&
		data[8] == 0x57 && data[9] == 0x45 && data[10] == 0x42 && data[11] == 0x50 {
		return ImageFormatWebP
	}

	return ImageFormatJPEG // default
}

// ResolutionPreset represents image resolution presets for vision models.
type ResolutionPreset string

const (
	ResolutionLow    ResolutionPreset = "low"    // 512x512 or similar
	ResolutionMedium ResolutionPreset = "medium" // 1024x1024 or similar
	ResolutionHigh   ResolutionPreset = "high"   // Original resolution
	ResolutionAuto   ResolutionPreset = "auto"   // Let the model decide
)

// VisionConfig configures vision processing.
type VisionConfig struct {
	Resolution     ResolutionPreset `json:"resolution"`
	MaxImageSize   int64            `json:"max_image_size"` // Max file size in bytes
	MaxDimension   int              `json:"max_dimension"`  // Max width/height
	AllowedFormats []ImageFormat    `json:"allowed_formats"`
}

// DefaultVisionConfig returns default vision configuration.
func DefaultVisionConfig() VisionConfig {
	return VisionConfig{
		Resolution:     ResolutionAuto,
		MaxImageSize:   20 * 1024 * 1024, // 20MB
		MaxDimension:   4096,
		AllowedFormats: []ImageFormat{ImageFormatPNG, ImageFormatJPEG, ImageFormatGIF, ImageFormatWebP},
	}
}

// AudioConfig configures audio processing.
type AudioConfig struct {
	MaxDuration    float64       `json:"max_duration"`  // Max duration in seconds
	MaxFileSize    int64         `json:"max_file_size"` // Max file size in bytes
	SampleRate     int           `json:"sample_rate"`   // Target sample rate
	AllowedFormats []AudioFormat `json:"allowed_formats"`
}

// DefaultAudioConfig returns default audio configuration.
func DefaultAudioConfig() AudioConfig {
	return AudioConfig{
		MaxDuration:    300,              // 5 minutes
		MaxFileSize:    25 * 1024 * 1024, // 25MB
		SampleRate:     16000,
		AllowedFormats: []AudioFormat{AudioFormatMP3, AudioFormatWAV, AudioFormatOGG, AudioFormatFLAC, AudioFormatM4A},
	}
}
