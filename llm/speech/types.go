// Package speech provides unified TTS and STT provider interfaces.
package speech

import (
	"context"
	"io"
	"time"
)

// ============================================================
// Text-to-Speech (TTS)
// ============================================================

// TTSRequest represents a text-to-speech request.
type TTSRequest struct {
	Text           string            `json:"text"`
	Model          string            `json:"model,omitempty"`
	Voice          string            `json:"voice,omitempty"`
	Speed          float64           `json:"speed,omitempty"`           // 0.25-4.0
	ResponseFormat string            `json:"response_format,omitempty"` // mp3, opus, aac, flac, wav, pcm
	Language       string            `json:"language,omitempty"`
	Metadata       map[string]string `json:"metadata,omitempty"`
}

// TTSResponse represents the response from a TTS request.
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

// TTSProvider defines the TTS provider interface.
type TTSProvider interface {
	// Synthesize converts text to speech.
	Synthesize(ctx context.Context, req *TTSRequest) (*TTSResponse, error)

	// SynthesizeToFile converts text to speech and saves to file.
	SynthesizeToFile(ctx context.Context, req *TTSRequest, filepath string) error

	// ListVoices returns available voices.
	ListVoices(ctx context.Context) ([]Voice, error)

	// Name returns the provider name.
	Name() string
}

// Voice represents an available voice.
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
// Speech-to-Text (STT)
// ============================================================

// STTRequest represents a speech-to-text request.
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

// STTResponse represents the response from an STT request.
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

// Segment represents a transcription segment.
type Segment struct {
	ID         int           `json:"id"`
	Start      time.Duration `json:"start"`
	End        time.Duration `json:"end"`
	Text       string        `json:"text"`
	Speaker    string        `json:"speaker,omitempty"`
	Confidence float64       `json:"confidence,omitempty"`
}

// Word represents a transcribed word with timing.
type Word struct {
	Word       string        `json:"word"`
	Start      time.Duration `json:"start"`
	End        time.Duration `json:"end"`
	Confidence float64       `json:"confidence,omitempty"`
	Speaker    string        `json:"speaker,omitempty"`
}

// STTProvider defines the STT provider interface.
type STTProvider interface {
	// Transcribe converts speech to text.
	Transcribe(ctx context.Context, req *STTRequest) (*STTResponse, error)

	// TranscribeFile transcribes an audio file.
	TranscribeFile(ctx context.Context, filepath string, opts *STTRequest) (*STTResponse, error)

	// Name returns the provider name.
	Name() string

	// SupportedFormats returns supported audio formats.
	SupportedFormats() []string
}
