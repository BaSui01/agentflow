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

// 11LabsProvider使用11Labs API执行TTS.
type ElevenLabsProvider struct {
	cfg    ElevenLabsConfig
	client *http.Client
}

// NewElevenLabs Provider 创建了新的"11Labs TTS"供应商.
func NewElevenLabsProvider(cfg ElevenLabsConfig) *ElevenLabsProvider {
	if cfg.BaseURL == "" {
		cfg.BaseURL = "https://api.elevenlabs.io"
	}
	if cfg.Model == "" {
		cfg.Model = "eleven_multilingual_v2"
	}
	timeout := cfg.Timeout
	if timeout == 0 {
		timeout = 60 * time.Second
	}

	return &ElevenLabsProvider{
		cfg:    cfg,
		client: &http.Client{Timeout: timeout},
	}
}

func (p *ElevenLabsProvider) Name() string { return "elevenlabs" }

type elevenLabsTTSRequest struct {
	Text          string `json:"text"`
	ModelID       string `json:"model_id"`
	VoiceSettings *struct {
		Stability       float64 `json:"stability,omitempty"`
		SimilarityBoost float64 `json:"similarity_boost,omitempty"`
		Style           float64 `json:"style,omitempty"`
		UseSpeakerBoost bool    `json:"use_speaker_boost,omitempty"`
	} `json:"voice_settings,omitempty"`
}

// 合成会使用"十一律"将文本转换为语音.
func (p *ElevenLabsProvider) Synthesize(ctx context.Context, req *TTSRequest) (*TTSResponse, error) {
	model := req.Model
	if model == "" {
		model = p.cfg.Model
	}
	voiceID := req.Voice
	if voiceID == "" {
		voiceID = p.cfg.VoiceID
	}
	if voiceID == "" {
		voiceID = "21m00Tcm4TlvDq8ikWAM" // Rachel - default voice
	}

	body := elevenLabsTTSRequest{
		Text:    req.Text,
		ModelID: model,
	}

	payload, _ := json.Marshal(body)
	endpoint := fmt.Sprintf("%s/v1/text-to-speech/%s", strings.TrimRight(p.cfg.BaseURL, "/"), voiceID)

	// 添加输出格式查询参数
	format := req.ResponseFormat
	if format == "" {
		format = "mp3_44100_128"
	}
	endpoint += "?output_format=" + format

	httpReq, err := http.NewRequestWithContext(ctx, "POST", endpoint, bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	httpReq.Header.Set("xi-api-key", p.cfg.APIKey)
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := p.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("elevenlabs request failed: %w", err)
	}

	if resp.StatusCode >= 400 {
		defer resp.Body.Close()
		errBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("elevenlabs error: status=%d body=%s", resp.StatusCode, string(errBody))
	}

	return &TTSResponse{
		Provider:  p.Name(),
		Model:     model,
		Audio:     resp.Body,
		Format:    "mp3",
		CharCount: len(req.Text),
		CreatedAt: time.Now(),
	}, nil
}

// 将文本转换为语音并保存为文件。
func (p *ElevenLabsProvider) SynthesizeToFile(ctx context.Context, req *TTSRequest, filepath string) error {
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

type elevenLabsVoice struct {
	VoiceID  string `json:"voice_id"`
	Name     string `json:"name"`
	Category string `json:"category"`
	Labels   struct {
		Gender      string `json:"gender"`
		Age         string `json:"age"`
		Accent      string `json:"accent"`
		Description string `json:"description"`
		UseCase     string `json:"use_case"`
	} `json:"labels"`
	PreviewURL string `json:"preview_url"`
}

type elevenLabsVoicesResponse struct {
	Voices []elevenLabsVoice `json:"voices"`
}

// ListVoices 返回可用的 11Labs 声音 。
func (p *ElevenLabsProvider) ListVoices(ctx context.Context) ([]Voice, error) {
	endpoint := fmt.Sprintf("%s/v1/voices", strings.TrimRight(p.cfg.BaseURL, "/"))

	httpReq, err := http.NewRequestWithContext(ctx, "GET", endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	httpReq.Header.Set("xi-api-key", p.cfg.APIKey)

	resp, err := p.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("failed to list voices: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		errBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("elevenlabs error: status=%d body=%s", resp.StatusCode, string(errBody))
	}

	var vResp elevenLabsVoicesResponse
	if err := json.NewDecoder(resp.Body).Decode(&vResp); err != nil {
		return nil, err
	}

	voices := make([]Voice, len(vResp.Voices))
	for i, v := range vResp.Voices {
		voices[i] = Voice{
			ID:          v.VoiceID,
			Name:        v.Name,
			Gender:      v.Labels.Gender,
			Description: v.Labels.Description,
			PreviewURL:  v.PreviewURL,
		}
	}

	return voices, nil
}
