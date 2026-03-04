package gemini

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	providerbase "github.com/BaSui01/agentflow/llm/providers/base"

	"github.com/BaSui01/agentflow/types"

	"github.com/BaSui01/agentflow/llm"
)

// =============================================================================
// 图像生成
// =============================================================================

// GenerateImage generates images using Gemini Imagen 4.
// Endpoint: POST /v1beta/models/{model}:predict
// Models: imagen-4.0-generate-001 (standard), imagen-4.0-ultra-generate-001 (ultra), imagen-4.0-fast-generate-001 (fast)
// Also supports native generation via gemini-2.5-flash-image, gemini-3-pro-image-preview
func (p *GeminiProvider) GenerateImage(ctx context.Context, req *llm.ImageGenerationRequest) (*llm.ImageGenerationResponse, error) {
	model := req.Model
	if model == "" {
		model = "imagen-4.0-generate-001"
	}
	endpoint := fmt.Sprintf("/v1beta/models/%s:predict", model)
	return providerbase.GenerateImageOpenAICompat(ctx, p.client, p.cfg.BaseURL, p.resolveAPIKey(ctx), p.Name(), endpoint, req, p.buildHeaders)
}

// =============================================================================
// 视频生成
// =============================================================================

// GenerateVideo 使用 Gemini Veo 生成视频.
// Endpoint: POST /v1beta/models/{model}:predictLongRunning
// Models: veo-3.1-generate-preview (standard), veo-3.1-fast-generate-preview (fast)
func (p *GeminiProvider) GenerateVideo(ctx context.Context, req *llm.VideoGenerationRequest) (*llm.VideoGenerationResponse, error) {
	model := req.Model
	if model == "" {
		model = "veo-3.1-generate-preview"
	}
	endpoint := fmt.Sprintf("/v1beta/models/%s:predictLongRunning", model)
	return providerbase.GenerateVideoOpenAICompat(ctx, p.client, p.cfg.BaseURL, p.resolveAPIKey(ctx), p.Name(), endpoint, req, p.buildHeaders)
}

// =============================================================================
// 音频生成和转录
// =============================================================================

// GenerateAudio 使用 Gemini TTS 生成音频.
// Endpoint: POST /v1beta/models/{model}:generateContent
// Models: gemini-2.5-flash-preview-tts, gemini-2.5-pro-preview-tts
// Supports 30+ voices including Kore, Charon, Fenrir, Aoede, Puck, etc.
func (p *GeminiProvider) GenerateAudio(ctx context.Context, req *llm.AudioGenerationRequest) (*llm.AudioGenerationResponse, error) {
	model := req.Model
	if model == "" {
		model = "gemini-2.5-flash-preview-tts"
	}
	endpoint := fmt.Sprintf("/v1beta/models/%s:generateContent", model)
	return providerbase.GenerateAudioOpenAICompat(ctx, p.client, p.cfg.BaseURL, p.resolveAPIKey(ctx), p.Name(), endpoint, req, p.buildHeaders)
}

// TranscribeAudio 使用 Gemini 进行语音转文本.
// Endpoint: POST /v1beta/models/{model}:generateContent
// Models: gemini-2.5-flash (with audio inline data)
func (p *GeminiProvider) TranscribeAudio(ctx context.Context, req *llm.AudioTranscriptionRequest) (*llm.AudioTranscriptionResponse, error) {
	model := req.Model
	if model == "" {
		model = "gemini-2.5-flash"
	}

	// Gemini STT 通过 generateContent + inline audio data 实现
	import64 := base64.StdEncoding.EncodeToString(req.File)
	mimeType := "audio/mp3"
	if req.ResponseFormat != "" {
		mimeType = "audio/" + req.ResponseFormat
	}

	geminiReq := map[string]any{
		"contents": []map[string]any{
			{
				"parts": []map[string]any{
					{"inline_data": map[string]any{"mime_type": mimeType, "data": import64}},
					{"text": "Transcribe this audio to text. Return only the transcription."},
				},
			},
		},
	}
	if req.Language != "" {
		geminiReq["contents"].([]map[string]any)[0]["parts"].([]map[string]any)[1]["text"] = fmt.Sprintf("Transcribe this audio to text in %s. Return only the transcription.", req.Language)
	}

	endpoint := fmt.Sprintf("%s/v1beta/models/%s:generateContent", strings.TrimRight(p.cfg.BaseURL, "/"), model)
	payload, err := json.Marshal(geminiReq)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	p.buildHeaders(httpReq, p.resolveAPIKey(ctx))

	resp, err := p.client.Do(httpReq)
	if err != nil {
		return nil, &types.Error{Code: llm.ErrUpstreamError, Message: err.Error(), Cause: err, HTTPStatus: http.StatusBadGateway, Retryable: true, Provider: p.Name()}
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		msg := providerbase.ReadErrorMessage(resp.Body)
		return nil, providerbase.MapHTTPError(resp.StatusCode, msg, p.Name())
	}

	var geminiResp struct {
		Candidates []struct {
			Content struct {
				Parts []struct {
					Text string `json:"text"`
				} `json:"parts"`
			} `json:"content"`
		} `json:"candidates"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&geminiResp); err != nil {
		return nil, &types.Error{Code: llm.ErrUpstreamError, Message: err.Error(), Cause: err, HTTPStatus: http.StatusBadGateway, Provider: p.Name()}
	}

	text := ""
	if len(geminiResp.Candidates) > 0 && len(geminiResp.Candidates[0].Content.Parts) > 0 {
		text = geminiResp.Candidates[0].Content.Parts[0].Text
	}

	return &llm.AudioTranscriptionResponse{
		Text:     text,
		Language: req.Language,
	}, nil
}

// =============================================================================
// 嵌入
// =============================================================================

// CreateEmbedding creates embeddings using Gemini.
// Endpoint: POST /v1beta/models/{model}:embedContent
// Models: gemini-embedding-001 (latest, MRL, 128-3072 dims), text-embedding-004 (legacy)
func (p *GeminiProvider) CreateEmbedding(ctx context.Context, req *llm.EmbeddingRequest) (*llm.EmbeddingResponse, error) {
	model := req.Model
	if model == "" {
		model = "gemini-embedding-001"
	}
	endpoint := fmt.Sprintf("/v1beta/models/%s:embedContent", model)
	return providerbase.CreateEmbeddingOpenAICompat(ctx, p.client, p.cfg.BaseURL, p.resolveAPIKey(ctx), p.Name(), endpoint, req, p.buildHeaders)
}

// =============================================================================
// 微调
// =============================================================================

// CreateFineTuningJob 使用 Gemini 创建微调任务.
// Endpoint: POST /v1beta/tunedModels
func (p *GeminiProvider) CreateFineTuningJob(ctx context.Context, req *llm.FineTuningJobRequest) (*llm.FineTuningJob, error) {
	endpoint := fmt.Sprintf("%s/v1beta/tunedModels", strings.TrimRight(p.cfg.BaseURL, "/"))

	geminiReq := map[string]any{
		"baseModel":        req.Model,
		"displayName":      req.Suffix,
		"tuningTask":       map[string]any{"hyperparameters": req.Hyperparameters},
		"trainingDatasets": req.TrainingFile,
	}

	payload, err := json.Marshal(geminiReq)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	p.buildHeaders(httpReq, p.resolveAPIKey(ctx))

	resp, err := p.client.Do(httpReq)
	if err != nil {
		return nil, &types.Error{Code: llm.ErrUpstreamError, Message: err.Error(), Cause: err, HTTPStatus: http.StatusBadGateway, Retryable: true, Provider: p.Name()}
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		msg := providerbase.ReadErrorMessage(resp.Body)
		return nil, providerbase.MapHTTPError(resp.StatusCode, msg, p.Name())
	}

	var geminiResp struct {
		Name     string `json:"name"`
		Metadata struct {
			TotalSteps int    `json:"totalSteps"`
			TunedModel string `json:"tunedModel"`
		} `json:"metadata"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&geminiResp); err != nil {
		return nil, &types.Error{Code: llm.ErrUpstreamError, Message: err.Error(), Cause: err, HTTPStatus: http.StatusBadGateway, Provider: p.Name()}
	}

	return &llm.FineTuningJob{
		ID:     geminiResp.Name,
		Model:  req.Model,
		Status: "queued",
	}, nil
}

// ListFineTuningJobs 列出 Gemini 微调任务.
// Endpoint: GET /v1beta/tunedModels
func (p *GeminiProvider) ListFineTuningJobs(ctx context.Context) ([]llm.FineTuningJob, error) {
	endpoint := fmt.Sprintf("%s/v1beta/tunedModels", strings.TrimRight(p.cfg.BaseURL, "/"))

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	p.buildHeaders(httpReq, p.resolveAPIKey(ctx))

	resp, err := p.client.Do(httpReq)
	if err != nil {
		return nil, &types.Error{Code: llm.ErrUpstreamError, Message: err.Error(), Cause: err, HTTPStatus: http.StatusBadGateway, Retryable: true, Provider: p.Name()}
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		msg := providerbase.ReadErrorMessage(resp.Body)
		return nil, providerbase.MapHTTPError(resp.StatusCode, msg, p.Name())
	}

	var listResp struct {
		TunedModels []struct {
			Name        string `json:"name"`
			BaseModel   string `json:"baseModel"`
			DisplayName string `json:"displayName"`
			State       string `json:"state"`
		} `json:"tunedModels"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&listResp); err != nil {
		return nil, &types.Error{Code: llm.ErrUpstreamError, Message: err.Error(), Cause: err, HTTPStatus: http.StatusBadGateway, Provider: p.Name()}
	}

	jobs := make([]llm.FineTuningJob, len(listResp.TunedModels))
	for i, m := range listResp.TunedModels {
		jobs[i] = llm.FineTuningJob{
			ID:     m.Name,
			Model:  m.BaseModel,
			Status: strings.ToLower(m.State),
		}
	}
	return jobs, nil
}

// GetFineTuningJob 获取 Gemini 微调任务.
// Endpoint: GET /v1beta/{name}
func (p *GeminiProvider) GetFineTuningJob(ctx context.Context, jobID string) (*llm.FineTuningJob, error) {
	endpoint := fmt.Sprintf("%s/v1beta/%s", strings.TrimRight(p.cfg.BaseURL, "/"), jobID)

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	p.buildHeaders(httpReq, p.resolveAPIKey(ctx))

	resp, err := p.client.Do(httpReq)
	if err != nil {
		return nil, &types.Error{Code: llm.ErrUpstreamError, Message: err.Error(), Cause: err, HTTPStatus: http.StatusBadGateway, Retryable: true, Provider: p.Name()}
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		msg := providerbase.ReadErrorMessage(resp.Body)
		return nil, providerbase.MapHTTPError(resp.StatusCode, msg, p.Name())
	}

	var tunedModel struct {
		Name      string `json:"name"`
		BaseModel string `json:"baseModel"`
		State     string `json:"state"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&tunedModel); err != nil {
		return nil, &types.Error{Code: llm.ErrUpstreamError, Message: err.Error(), Cause: err, HTTPStatus: http.StatusBadGateway, Provider: p.Name()}
	}

	return &llm.FineTuningJob{
		ID:     tunedModel.Name,
		Model:  tunedModel.BaseModel,
		Status: strings.ToLower(tunedModel.State),
	}, nil
}

// CancelFineTuningJob 取消 Gemini 微调任务.
// Endpoint: DELETE /v1beta/{name}
func (p *GeminiProvider) CancelFineTuningJob(ctx context.Context, jobID string) error {
	endpoint := fmt.Sprintf("%s/v1beta/%s", strings.TrimRight(p.cfg.BaseURL, "/"), jobID)

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodDelete, endpoint, nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}
	p.buildHeaders(httpReq, p.resolveAPIKey(ctx))

	resp, err := p.client.Do(httpReq)
	if err != nil {
		return &types.Error{Code: llm.ErrUpstreamError, Message: err.Error(), Cause: err, HTTPStatus: http.StatusBadGateway, Retryable: true, Provider: p.Name()}
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		msg := providerbase.ReadErrorMessage(resp.Body)
		return providerbase.MapHTTPError(resp.StatusCode, msg, p.Name())
	}
	return nil
}
