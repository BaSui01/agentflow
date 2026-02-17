package music

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// MiniMaxProvider implements music generation using MiniMax API.
type MiniMaxProvider struct {
	cfg    MiniMaxMusicConfig
	client *http.Client
}

// NewMiniMaxProvider creates a new MiniMax music provider.
func NewMiniMaxProvider(cfg MiniMaxMusicConfig) *MiniMaxProvider {
	if cfg.BaseURL == "" {
		cfg.BaseURL = "https://api.minimax.io"
	}
	if cfg.Model == "" {
		cfg.Model = "music-01"
	}
	timeout := cfg.Timeout
	if timeout == 0 {
		timeout = 300 * time.Second
	}
	return &MiniMaxProvider{
		cfg:    cfg,
		client: &http.Client{Timeout: timeout},
	}
}

func (p *MiniMaxProvider) Name() string { return "minimax-music" }

type miniMaxMusicRequest struct {
	Model            string `json:"model"`
	Prompt           string `json:"prompt,omitempty"`
	Lyrics           string `json:"lyrics,omitempty"`
	ReferenceAudio   string `json:"refer_audio,omitempty"`
	ReferenceVoiceID string `json:"refer_voice_id,omitempty"`
	AudioSetting     struct {
		SampleRate int    `json:"sample_rate,omitempty"`
		Bitrate    int    `json:"bitrate,omitempty"`
		Format     string `json:"format,omitempty"`
	} `json:"audio_setting,omitempty"`
}

type miniMaxMusicResponse struct {
	BaseResp struct {
		StatusCode int    `json:"status_code"`
		StatusMsg  string `json:"status_msg"`
	} `json:"base_resp"`
	Data struct {
		Audio string `json:"audio"` // Base64 encoded
	} `json:"data"`
	ExtraInfo struct {
		AudioLength float64 `json:"audio_length"`
	} `json:"extra_info"`
}

// Generate creates music using MiniMax API.
func (p *MiniMaxProvider) Generate(ctx context.Context, req *GenerateRequest) (*GenerateResponse, error) {
	model := req.Model
	if model == "" {
		model = p.cfg.Model
	}

	body := miniMaxMusicRequest{
		Model:  model,
		Prompt: req.Prompt,
	}
	if req.ReferenceAudio != "" {
		body.ReferenceAudio = req.ReferenceAudio
	}
	body.AudioSetting.SampleRate = 44100
	body.AudioSetting.Bitrate = 128000
	body.AudioSetting.Format = "mp3"

	payload, _ := json.Marshal(body)
	endpoint := fmt.Sprintf("%s/v1/music_generation", strings.TrimRight(p.cfg.BaseURL, "/"))

	httpReq, err := http.NewRequestWithContext(ctx, "POST", endpoint, bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	httpReq.Header.Set("Authorization", "Bearer "+p.cfg.APIKey)
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := p.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("minimax music request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		errBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("minimax music error: status=%d body=%s", resp.StatusCode, string(errBody))
	}

	var mResp miniMaxMusicResponse
	if err := json.NewDecoder(resp.Body).Decode(&mResp); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	if mResp.BaseResp.StatusCode != 0 {
		return nil, fmt.Errorf("minimax music error: %s", mResp.BaseResp.StatusMsg)
	}

	return &GenerateResponse{
		Provider: p.Name(),
		Model:    model,
		Tracks: []MusicData{{
			B64Audio: mResp.Data.Audio,
			Duration: mResp.ExtraInfo.AudioLength,
		}},
		Usage: MusicUsage{
			TracksGenerated: 1,
			DurationSeconds: mResp.ExtraInfo.AudioLength,
		},
		CreatedAt: time.Now(),
	}, nil
}
