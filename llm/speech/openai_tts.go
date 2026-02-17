package speech

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

// OpenAITTSProvider implements TTS using OpenAI's API.
type OpenAITTSProvider struct {
	cfg    OpenAITTSConfig
	client *http.Client
}

// NewOpenAITTSProvider creates a new OpenAI TTS provider.
func NewOpenAITTSProvider(cfg OpenAITTSConfig) *OpenAITTSProvider {
	if cfg.BaseURL == "" {
		cfg.BaseURL = "https://api.openai.com"
	}
	if cfg.Model == "" {
		cfg.Model = "tts-1-hd"
	}
	if cfg.Voice == "" {
		cfg.Voice = "alloy"
	}
	timeout := cfg.Timeout
	if timeout == 0 {
		timeout = 60 * time.Second
	}

	return &OpenAITTSProvider{
		cfg:    cfg,
		client: &http.Client{Timeout: timeout},
	}
}

func (p *OpenAITTSProvider) Name() string { return "openai-tts" }

type openAITTSRequest struct {
	Model          string  `json:"model"`
	Input          string  `json:"input"`
	Voice          string  `json:"voice"`
	ResponseFormat string  `json:"response_format,omitempty"`
	Speed          float64 `json:"speed,omitempty"`
}

// Synthesize converts text to speech.
func (p *OpenAITTSProvider) Synthesize(ctx context.Context, req *TTSRequest) (*TTSResponse, error) {
	model := req.Model
	if model == "" {
		model = p.cfg.Model
	}
	voice := req.Voice
	if voice == "" {
		voice = p.cfg.Voice
	}
	format := req.ResponseFormat
	if format == "" {
		format = "mp3"
	}

	body := openAITTSRequest{
		Model:          model,
		Input:          req.Text,
		Voice:          voice,
		ResponseFormat: format,
	}
	if req.Speed > 0 {
		body.Speed = req.Speed
	}

	payload, _ := json.Marshal(body)
	httpReq, err := http.NewRequestWithContext(ctx, "POST",
		strings.TrimRight(p.cfg.BaseURL, "/")+"/v1/audio/speech",
		bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	httpReq.Header.Set("Authorization", "Bearer "+p.cfg.APIKey)
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := p.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("openai tts request failed: %w", err)
	}

	if resp.StatusCode >= 400 {
		defer resp.Body.Close()
		errBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("openai tts error: status=%d body=%s", resp.StatusCode, string(errBody))
	}

	return &TTSResponse{
		Provider:  p.Name(),
		Model:     model,
		Audio:     resp.Body,
		Format:    format,
		CharCount: len(req.Text),
		CreatedAt: time.Now(),
	}, nil
}

// SynthesizeToFile converts text to speech and saves to file.
func (p *OpenAITTSProvider) SynthesizeToFile(ctx context.Context, req *TTSRequest, filepath string) error {
	resp, err := p.Synthesize(ctx, req)
	if err != nil {
		return err
	}
	defer resp.Audio.Close()

	file, err := os.Create(filepath)
	if err != nil {
		return fmt.Errorf("failed to create file: %w", err)
	}
	defer file.Close()

	_, err = io.Copy(file, resp.Audio)
	return err
}

// ListVoices returns available OpenAI voices.
func (p *OpenAITTSProvider) ListVoices(ctx context.Context) ([]Voice, error) {
	return []Voice{
		{ID: "alloy", Name: "Alloy", Gender: "neutral", Description: "Neutral, balanced voice"},
		{ID: "echo", Name: "Echo", Gender: "male", Description: "Warm, conversational male voice"},
		{ID: "fable", Name: "Fable", Gender: "neutral", Description: "Expressive, narrative voice"},
		{ID: "onyx", Name: "Onyx", Gender: "male", Description: "Deep, authoritative male voice"},
		{ID: "nova", Name: "Nova", Gender: "female", Description: "Friendly, upbeat female voice"},
		{ID: "shimmer", Name: "Shimmer", Gender: "female", Description: "Clear, professional female voice"},
	}, nil
}
