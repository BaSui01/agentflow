package speech

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"strings"
	"time"
)

// OpenAISTTProvider implements STT using OpenAI Whisper API.
type OpenAISTTProvider struct {
	cfg    OpenAISTTConfig
	client *http.Client
}

// NewOpenAISTTProvider creates a new OpenAI STT provider.
func NewOpenAISTTProvider(cfg OpenAISTTConfig) *OpenAISTTProvider {
	if cfg.BaseURL == "" {
		cfg.BaseURL = "https://api.openai.com"
	}
	if cfg.Model == "" {
		cfg.Model = "whisper-1"
	}
	timeout := cfg.Timeout
	if timeout == 0 {
		timeout = 120 * time.Second
	}

	return &OpenAISTTProvider{
		cfg:    cfg,
		client: &http.Client{Timeout: timeout},
	}
}

func (p *OpenAISTTProvider) Name() string { return "openai-stt" }

func (p *OpenAISTTProvider) SupportedFormats() []string {
	return []string{"flac", "m4a", "mp3", "mp4", "mpeg", "mpga", "oga", "ogg", "wav", "webm"}
}

type whisperResponse struct {
	Text     string  `json:"text"`
	Language string  `json:"language,omitempty"`
	Duration float64 `json:"duration,omitempty"`
	Segments []struct {
		ID               int     `json:"id"`
		Start            float64 `json:"start"`
		End              float64 `json:"end"`
		Text             string  `json:"text"`
		AvgLogprob       float64 `json:"avg_logprob,omitempty"`
		CompressionRatio float64 `json:"compression_ratio,omitempty"`
		NoSpeechProb     float64 `json:"no_speech_prob,omitempty"`
	} `json:"segments,omitempty"`
	Words []struct {
		Word  string  `json:"word"`
		Start float64 `json:"start"`
		End   float64 `json:"end"`
	} `json:"words,omitempty"`
}

// Transcribe converts speech to text.
func (p *OpenAISTTProvider) Transcribe(ctx context.Context, req *STTRequest) (*STTResponse, error) {
	if req.Audio == nil {
		return nil, fmt.Errorf("audio input is required")
	}

	model := req.Model
	if model == "" {
		model = p.cfg.Model
	}

	// Build multipart form
	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)

	// Add audio file
	part, err := writer.CreateFormFile("file", "audio.mp3")
	if err != nil {
		return nil, fmt.Errorf("failed to create form file: %w", err)
	}
	if _, err := io.Copy(part, req.Audio); err != nil {
		return nil, fmt.Errorf("failed to copy audio: %w", err)
	}

	// Add model
	_ = writer.WriteField("model", model)

	// Add optional fields
	if req.Language != "" {
		_ = writer.WriteField("language", req.Language)
	}
	if req.Prompt != "" {
		_ = writer.WriteField("prompt", req.Prompt)
	}
	format := req.ResponseFormat
	if format == "" {
		format = "verbose_json"
	}
	_ = writer.WriteField("response_format", format)

	if len(req.TimestampGranularities) > 0 {
		for _, g := range req.TimestampGranularities {
			_ = writer.WriteField("timestamp_granularities[]", g)
		}
	}

	writer.Close()

	httpReq, _ := http.NewRequestWithContext(ctx, "POST",
		strings.TrimRight(p.cfg.BaseURL, "/")+"/v1/audio/transcriptions",
		&buf)
	httpReq.Header.Set("Authorization", "Bearer "+p.cfg.APIKey)
	httpReq.Header.Set("Content-Type", writer.FormDataContentType())

	resp, err := p.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("whisper request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		errBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("whisper error: status=%d body=%s", resp.StatusCode, string(errBody))
	}

	var wResp whisperResponse
	if err := json.NewDecoder(resp.Body).Decode(&wResp); err != nil {
		return nil, fmt.Errorf("failed to decode whisper response: %w", err)
	}

	result := &STTResponse{
		Provider:  p.Name(),
		Model:     model,
		Text:      wResp.Text,
		Language:  wResp.Language,
		Duration:  time.Duration(wResp.Duration * float64(time.Second)),
		CreatedAt: time.Now(),
	}

	// Convert segments
	for _, s := range wResp.Segments {
		result.Segments = append(result.Segments, Segment{
			ID:    s.ID,
			Start: time.Duration(s.Start * float64(time.Second)),
			End:   time.Duration(s.End * float64(time.Second)),
			Text:  s.Text,
		})
	}

	// Convert words
	for _, w := range wResp.Words {
		result.Words = append(result.Words, Word{
			Word:  w.Word,
			Start: time.Duration(w.Start * float64(time.Second)),
			End:   time.Duration(w.End * float64(time.Second)),
		})
	}

	return result, nil
}

// TranscribeFile transcribes an audio file.
func (p *OpenAISTTProvider) TranscribeFile(ctx context.Context, filepath string, opts *STTRequest) (*STTResponse, error) {
	file, err := os.Open(filepath)
	if err != nil {
		return nil, fmt.Errorf("failed to open file: %w", err)
	}
	defer file.Close()

	if opts == nil {
		opts = &STTRequest{}
	}
	opts.Audio = file

	return p.Transcribe(ctx, opts)
}
