package speech

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/BaSui01/agentflow/internal/tlsutil"
)

// DeepgramProvider使用Deepgram API执行STT.
type DeepgramProvider struct {
	cfg    DeepgramConfig
	client *http.Client
}

// NewDeepgramProvider 创建新的 Deepgram STT 提供者.
func NewDeepgramProvider(cfg DeepgramConfig) *DeepgramProvider {
	if cfg.BaseURL == "" {
		cfg.BaseURL = "https://api.deepgram.com"
	}
	if cfg.Model == "" {
		cfg.Model = "nova-2"
	}
	timeout := cfg.Timeout
	if timeout == 0 {
		timeout = 120 * time.Second
	}

	return &DeepgramProvider{
		cfg:    cfg,
		client: tlsutil.SecureHTTPClient(timeout),
	}
}

func (p *DeepgramProvider) Name() string { return "deepgram" }

func (p *DeepgramProvider) SupportedFormats() []string {
	return []string{"mp3", "mp4", "mp2", "aac", "wav", "flac", "pcm", "m4a", "ogg", "opus", "webm"}
}

type deepgramResponse struct {
	Metadata struct {
		TransactionKey string  `json:"transaction_key"`
		RequestID      string  `json:"request_id"`
		Duration       float64 `json:"duration"`
		Channels       int     `json:"channels"`
	} `json:"metadata"`
	Results struct {
		Channels []struct {
			Alternatives []struct {
				Transcript string  `json:"transcript"`
				Confidence float64 `json:"confidence"`
				Words      []struct {
					Word              string  `json:"word"`
					Start             float64 `json:"start"`
					End               float64 `json:"end"`
					Confidence        float64 `json:"confidence"`
					Speaker           int     `json:"speaker,omitempty"`
					SpeakerConfidence float64 `json:"speaker_confidence,omitempty"`
				} `json:"words"`
				Paragraphs *struct {
					Paragraphs []struct {
						Sentences []struct {
							Text  string  `json:"text"`
							Start float64 `json:"start"`
							End   float64 `json:"end"`
						} `json:"sentences"`
						Speaker int `json:"speaker,omitempty"`
					} `json:"paragraphs"`
				} `json:"paragraphs,omitempty"`
			} `json:"alternatives"`
		} `json:"channels"`
		Utterances []struct {
			Start      float64 `json:"start"`
			End        float64 `json:"end"`
			Confidence float64 `json:"confidence"`
			Channel    int     `json:"channel"`
			Transcript string  `json:"transcript"`
			Speaker    int     `json:"speaker"`
			ID         string  `json:"id"`
		} `json:"utterances,omitempty"`
	} `json:"results"`
}

// 将语音转换为使用Deepgram的文本。
func (p *DeepgramProvider) Transcribe(ctx context.Context, req *STTRequest) (*STTResponse, error) {
	if req.Audio == nil && req.AudioURL == "" {
		return nil, fmt.Errorf("audio input or URL is required")
	}

	model := req.Model
	if model == "" {
		model = p.cfg.Model
	}

	// 构建查询参数
	params := url.Values{}
	params.Set("model", model)
	params.Set("smart_format", "true")
	params.Set("punctuate", "true")

	if req.Language != "" {
		params.Set("language", req.Language)
	}
	if req.Diarization {
		params.Set("diarize", "true")
	}

	endpoint := fmt.Sprintf("%s/v1/listen?%s", strings.TrimRight(p.cfg.BaseURL, "/"), params.Encode())

	var httpReq *http.Request
	var err error

	if req.AudioURL != "" {
		// 基于 URL 的复制
		body := map[string]string{"url": req.AudioURL}
		payload, _ := json.Marshal(body)
		httpReq, err = http.NewRequestWithContext(ctx, "POST", endpoint, bytes.NewReader(payload))
		if err != nil {
			return nil, err
		}
		httpReq.Header.Set("Content-Type", "application/json")
	} else {
		// 直接音频上传
		audioData, err := io.ReadAll(req.Audio)
		if err != nil {
			return nil, fmt.Errorf("failed to read audio: %w", err)
		}
		httpReq, err = http.NewRequestWithContext(ctx, "POST", endpoint, bytes.NewReader(audioData))
		if err != nil {
			return nil, err
		}
		httpReq.Header.Set("Content-Type", "audio/mpeg")
	}

	httpReq.Header.Set("Authorization", "Token "+p.cfg.APIKey)

	resp, err := p.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("deepgram request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		errBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("deepgram error: status=%d body=%s", resp.StatusCode, string(errBody))
	}

	var dResp deepgramResponse
	if err := json.NewDecoder(resp.Body).Decode(&dResp); err != nil {
		return nil, fmt.Errorf("failed to decode deepgram response: %w", err)
	}

	result := &STTResponse{
		Provider:  p.Name(),
		Model:     model,
		Duration:  time.Duration(dResp.Metadata.Duration * float64(time.Second)),
		CreatedAt: time.Now(),
	}

	// 从第一个频道提取记录
	if len(dResp.Results.Channels) > 0 && len(dResp.Results.Channels[0].Alternatives) > 0 {
		alt := dResp.Results.Channels[0].Alternatives[0]
		result.Text = alt.Transcript
		result.Confidence = alt.Confidence

		// 转换单词
		for _, w := range alt.Words {
			word := Word{
				Word:       w.Word,
				Start:      time.Duration(w.Start * float64(time.Second)),
				End:        time.Duration(w.End * float64(time.Second)),
				Confidence: w.Confidence,
			}
			if w.Speaker > 0 {
				word.Speaker = fmt.Sprintf("speaker_%d", w.Speaker)
			}
			result.Words = append(result.Words, word)
		}
	}

	// 将语句转换为分区( 如果启用对号)
	for i, u := range dResp.Results.Utterances {
		result.Segments = append(result.Segments, Segment{
			ID:         i,
			Start:      time.Duration(u.Start * float64(time.Second)),
			End:        time.Duration(u.End * float64(time.Second)),
			Text:       u.Transcript,
			Speaker:    fmt.Sprintf("speaker_%d", u.Speaker),
			Confidence: u.Confidence,
		})
	}

	return result, nil
}

// 转录File转录音频文件.
func (p *DeepgramProvider) TranscribeFile(ctx context.Context, filepath string, opts *STTRequest) (*STTResponse, error) {
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
