// Package speech 提供统一的 TTS 和 STT 提供者接口.
package speech

import (
	"context"
	"io"
	"time"
)

// ============================================================
// 文本转语音 (TTS)
// ============================================================

// TTSRequest 表示文本转语音请求.
type TTSRequest struct {
	Text           string            `json:"text"`
	Model          string            `json:"model,omitempty"`
	Voice          string            `json:"voice,omitempty"`
	Speed          float64           `json:"speed,omitempty"`           // 0.25-4.0
	ResponseFormat string            `json:"response_format,omitempty"` // mp3, opus, aac, flac, wav, pcm
	Language       string            `json:"language,omitempty"`
	Metadata       map[string]string `json:"metadata,omitempty"`
}

// TTSResponse 表示 TTS 请求的响应.
type TTSResponse struct {
	Provider  string        `json:"provider"`
	Model     string        `json:"model"`
	Audio     io.ReadCloser `json:"-"`                    // Audio stream
	AudioData []byte        `json:"audio_data,omitempty"` // Audio bytes (if buffered)
	Format    string        `json:"format"`
	Duration  time.Duration `json:"duration,omitempty"`
	CharCount int           `json:"char_count,omitempty"`
	CreatedAt time.Time     `json:"created_at"`
}

// TTSProvider 定义 TTS 提供者接口.
type TTSProvider interface {
	// Synthesize 将文本转换为语音.
	Synthesize(ctx context.Context, req *TTSRequest) (*TTSResponse, error)

	// SynthesizeToFile 将文本转换为语音并保存为文件.
	SynthesizeToFile(ctx context.Context, req *TTSRequest, filepath string) error

	// ListVoices 返回可用声音.
	ListVoices(ctx context.Context) ([]Voice, error)

	// Name 返回提供者名称.
	Name() string
}

// Voice 表示一个可用的声音.
type Voice struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Language    string   `json:"language"`
	Gender      string   `json:"gender,omitempty"` // male, female, neutral
	Description string   `json:"description,omitempty"`
	PreviewURL  string   `json:"preview_url,omitempty"`
	Labels      []string `json:"labels,omitempty"`
}

// ============================================================
// 语音转文本 (STT)
// ============================================================

// STTRequest 表示语音转文本请求.
type STTRequest struct {
	Audio                  io.Reader         `json:"-"`
	AudioURL               string            `json:"audio_url,omitempty"`
	Model                  string            `json:"model,omitempty"`
	Language               string            `json:"language,omitempty"`        // ISO-639-1 code
	Prompt                 string            `json:"prompt,omitempty"`          // Context hint
	ResponseFormat         string            `json:"response_format,omitempty"` // json, text, srt, vtt, verbose_json
	Temperature            float64           `json:"temperature,omitempty"`
	TimestampGranularities []string          `json:"timestamp_granularities,omitempty"` // word, segment
	Diarization            bool              `json:"diarization,omitempty"`             // Speaker identification
	Metadata               map[string]string `json:"metadata,omitempty"`
}

// STTResponse 表示 STT 请求的响应.
type STTResponse struct {
	Provider   string        `json:"provider"`
	Model      string        `json:"model"`
	Text       string        `json:"text"`
	Language   string        `json:"language,omitempty"`
	Duration   time.Duration `json:"duration,omitempty"`
	Segments   []Segment     `json:"segments,omitempty"`
	Words      []Word        `json:"words,omitempty"`
	Confidence float64       `json:"confidence,omitempty"`
	CreatedAt  time.Time     `json:"created_at"`
}

// Segment 表示转录片段.
type Segment struct {
	ID         int           `json:"id"`
	Start      time.Duration `json:"start"`
	End        time.Duration `json:"end"`
	Text       string        `json:"text"`
	Speaker    string        `json:"speaker,omitempty"`
	Confidence float64       `json:"confidence,omitempty"`
}

// Word 表示带时间戳的转录词.
type Word struct {
	Word       string        `json:"word"`
	Start      time.Duration `json:"start"`
	End        time.Duration `json:"end"`
	Confidence float64       `json:"confidence,omitempty"`
	Speaker    string        `json:"speaker,omitempty"`
}

// STTProvider 定义 STT 提供者接口.
type STTProvider interface {
	// Transcribe 将语音转换为文本.
	Transcribe(ctx context.Context, req *STTRequest) (*STTResponse, error)

	// TranscribeFile 转录音频文件.
	TranscribeFile(ctx context.Context, filepath string, opts *STTRequest) (*STTResponse, error)

	// Name 返回提供者名称.
	Name() string

	// SupportedFormats 返回支持的音频格式.
	SupportedFormats() []string
}
