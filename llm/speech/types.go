// 软件包语音提供统一的TTS和STT供应商接口.
package speech

import (
	"context"
	"io"
	"time"
)

// ============================================================
// 文字对语言( TTS)
// ============================================================

// TTS请求代表了文本对语音请求.
type TTSRequest struct {
	Text           string            `json:"text"`
	Model          string            `json:"model,omitempty"`
	Voice          string            `json:"voice,omitempty"`
	Speed          float64           `json:"speed,omitempty"`           // 0.25-4.0
	ResponseFormat string            `json:"response_format,omitempty"` // mp3, opus, aac, flac, wav, pcm
	Language       string            `json:"language,omitempty"`
	Metadata       map[string]string `json:"metadata,omitempty"`
}

// TTSResponse代表来自TTS请求的回应.
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

// TTS Provider定义了 TTS 提供者接口.
type TTSProvider interface {
	// 合成大小将文本转换为语音.
	Synthesize(ctx context.Context, req *TTSRequest) (*TTSResponse, error)

	// 将文本转换为语音并保存为文件。
	SynthesizeToFile(ctx context.Context, req *TTSRequest, filepath string) error

	// ListVoices 返回可用声音 。
	ListVoices(ctx context.Context) ([]Voice, error)

	// 名称返回提供者名称 。
	Name() string
}

// 声音代表一个可用的声音。
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
// 语音对文本( STT)
// ============================================================

// STT 请求代表语音对文本请求.
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

// STTResponse代表来自STT请求的答复.
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

// 部分代表了抄录部分.
type Segment struct {
	ID         int           `json:"id"`
	Start      time.Duration `json:"start"`
	End        time.Duration `json:"end"`
	Text       string        `json:"text"`
	Speaker    string        `json:"speaker,omitempty"`
	Confidence float64       `json:"confidence,omitempty"`
}

// 单词代表了有时间的转写词.
type Word struct {
	Word       string        `json:"word"`
	Start      time.Duration `json:"start"`
	End        time.Duration `json:"end"`
	Confidence float64       `json:"confidence,omitempty"`
	Speaker    string        `json:"speaker,omitempty"`
}

// STTProvider定义了STT提供者接口.
type STTProvider interface {
	// 将语音转换为文本 。
	Transcribe(ctx context.Context, req *STTRequest) (*STTResponse, error)

	// 转录File转录音频文件.
	TranscribeFile(ctx context.Context, filepath string, opts *STTRequest) (*STTResponse, error)

	// 名称返回提供者名称 。
	Name() string

	// 支持Formats返回支持的音频格式 。
	SupportedFormats() []string
}
