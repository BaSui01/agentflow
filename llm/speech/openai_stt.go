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

	"github.com/BaSui01/agentflow/internal/tlsutil"
)

// OpenAISTTProvider使用OpenAI Whisper API执行STT.
type OpenAISTTProvider struct {
	cfg    OpenAISTTConfig
	client *http.Client
}

// NewOpenAISTTProvider 创建新的 OpenAI STT 提供者.
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
		client: tlsutil.SecureHTTPClient(timeout),
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

// 将语音转换为文本 。
func (p *OpenAISTTProvider) Transcribe(ctx context.Context, req *STTRequest) (*STTResponse, error) {
	if req.Audio == nil {
		return nil, fmt.Errorf("audio input is required")
	}

	model := req.Model
	if model == "" {
		model = p.cfg.Model
	}

	// 构建多部分形式
	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)

	// 添加音频文件
	part, err := writer.CreateFormFile("file", "audio.mp3")
	if err != nil {
		return nil, fmt.Errorf("failed to create form file: %w", err)
	}
	if _, err := io.Copy(part, req.Audio); err != nil {
		return nil, fmt.Errorf("failed to copy audio: %w", err)
	}

	// 添加模式
	_ = writer.WriteField("model", model)

	// 添加可选字段
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

	httpReq, err := http.NewRequestWithContext(ctx, "POST",
		strings.TrimRight(p.cfg.BaseURL, "/")+"/v1/audio/transcriptions",
		&buf)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
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

	// 转换片段
	for _, s := range wResp.Segments {
		result.Segments = append(result.Segments, Segment{
			ID:    s.ID,
			Start: time.Duration(s.Start * float64(time.Second)),
			End:   time.Duration(s.End * float64(time.Second)),
			Text:  s.Text,
		})
	}

	// 转换单词
	for _, w := range wResp.Words {
		result.Words = append(result.Words, Word{
			Word:  w.Word,
			Start: time.Duration(w.Start * float64(time.Second)),
			End:   time.Duration(w.End * float64(time.Second)),
		})
	}

	return result, nil
}

// 转录File转录音频文件.
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
