package glm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	providerbase "github.com/BaSui01/agentflow/llm/providers/base"

	"github.com/BaSui01/agentflow/types"

	"github.com/BaSui01/agentflow/llm"
)

// GenerateImage 使用 GLM CogView 生成图像.
func (p *GLMProvider) GenerateImage(ctx context.Context, req *llm.ImageGenerationRequest) (*llm.ImageGenerationResponse, error) {
	return providerbase.GenerateImageOpenAICompat(ctx, p.Client, p.Cfg.BaseURL, p.ResolveAPIKey(ctx), p.Name(), "/api/paas/v4/images/generations", req, p.ApplyHeaders)
}

// GenerateVideo 使用 GLM CogVideo 生成视频.
func (p *GLMProvider) GenerateVideo(ctx context.Context, req *llm.VideoGenerationRequest) (*llm.VideoGenerationResponse, error) {
	return providerbase.GenerateVideoOpenAICompat(ctx, p.Client, p.Cfg.BaseURL, p.ResolveAPIKey(ctx), p.Name(), "/api/paas/v4/videos/generations", req, p.ApplyHeaders)
}

// GenerateAudio 使用 GLM 生成音频。
func (p *GLMProvider) GenerateAudio(ctx context.Context, req *llm.AudioGenerationRequest) (*llm.AudioGenerationResponse, error) {
	return providerbase.GenerateAudioOpenAICompat(
		ctx,
		p.Client,
		p.Cfg.BaseURL,
		p.ResolveAPIKey(ctx),
		p.Name(),
		"/api/paas/v4/audio/speech",
		req,
		p.ApplyHeaders,
	)
}

// TranscribeAudio GLM 不支持音频转录.
func (p *GLMProvider) TranscribeAudio(ctx context.Context, req *llm.AudioTranscriptionRequest) (*llm.AudioTranscriptionResponse, error) {
	return nil, providerbase.NotSupportedError(p.Name(), "audio transcription")
}

// CreateEmbedding 使用 GLM 创建嵌入.
func (p *GLMProvider) CreateEmbedding(ctx context.Context, req *llm.EmbeddingRequest) (*llm.EmbeddingResponse, error) {
	return providerbase.CreateEmbeddingOpenAICompat(ctx, p.Client, p.Cfg.BaseURL, p.ResolveAPIKey(ctx), p.Name(), "/api/paas/v4/embeddings", req, p.ApplyHeaders)
}

// CreateFineTuningJob 创建 GLM 微调任务。
// Endpoint: POST /api/paas/v4/fine_tuning/jobs
func (p *GLMProvider) CreateFineTuningJob(ctx context.Context, req *llm.FineTuningJobRequest) (*llm.FineTuningJob, error) {
	endpoint := fmt.Sprintf("%s/api/paas/v4/fine_tuning/jobs", strings.TrimRight(p.Cfg.BaseURL, "/"))
	payload, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	p.ApplyHeaders(httpReq, p.ResolveAPIKey(ctx))
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := p.Client.Do(httpReq)
	if err != nil {
		return nil, &types.Error{
			Code: llm.ErrUpstreamError, Message: err.Error(),
			HTTPStatus: http.StatusBadGateway, Retryable: true, Provider: p.Name(),
		}
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		msg := providerbase.ReadErrorMessage(resp.Body)
		return nil, providerbase.MapHTTPError(resp.StatusCode, msg, p.Name())
	}

	var job llm.FineTuningJob
	if err := json.NewDecoder(resp.Body).Decode(&job); err != nil {
		return nil, &types.Error{
			Code: llm.ErrUpstreamError, Message: err.Error(),
			HTTPStatus: http.StatusBadGateway, Provider: p.Name(),
		}
	}
	return &job, nil
}

// ListFineTuningJobs 列出 GLM 微调任务。
// Endpoint: GET /api/paas/v4/fine_tuning/jobs
func (p *GLMProvider) ListFineTuningJobs(ctx context.Context) ([]llm.FineTuningJob, error) {
	endpoint := fmt.Sprintf("%s/api/paas/v4/fine_tuning/jobs", strings.TrimRight(p.Cfg.BaseURL, "/"))
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	p.ApplyHeaders(httpReq, p.ResolveAPIKey(ctx))

	resp, err := p.Client.Do(httpReq)
	if err != nil {
		return nil, &types.Error{
			Code: llm.ErrUpstreamError, Message: err.Error(),
			HTTPStatus: http.StatusBadGateway, Retryable: true, Provider: p.Name(),
		}
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		msg := providerbase.ReadErrorMessage(resp.Body)
		return nil, providerbase.MapHTTPError(resp.StatusCode, msg, p.Name())
	}

	var listResp struct {
		Data []llm.FineTuningJob `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&listResp); err != nil {
		return nil, &types.Error{
			Code: llm.ErrUpstreamError, Message: err.Error(),
			HTTPStatus: http.StatusBadGateway, Provider: p.Name(),
		}
	}
	return listResp.Data, nil
}

// GetFineTuningJob 获取 GLM 微调任务。
// Endpoint: GET /api/paas/v4/fine_tuning/jobs/{job_id}
func (p *GLMProvider) GetFineTuningJob(ctx context.Context, jobID string) (*llm.FineTuningJob, error) {
	endpoint := fmt.Sprintf("%s/api/paas/v4/fine_tuning/jobs/%s", strings.TrimRight(p.Cfg.BaseURL, "/"), jobID)
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	p.ApplyHeaders(httpReq, p.ResolveAPIKey(ctx))

	resp, err := p.Client.Do(httpReq)
	if err != nil {
		return nil, &types.Error{
			Code: llm.ErrUpstreamError, Message: err.Error(),
			HTTPStatus: http.StatusBadGateway, Retryable: true, Provider: p.Name(),
		}
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		msg := providerbase.ReadErrorMessage(resp.Body)
		return nil, providerbase.MapHTTPError(resp.StatusCode, msg, p.Name())
	}

	var job llm.FineTuningJob
	if err := json.NewDecoder(resp.Body).Decode(&job); err != nil {
		return nil, &types.Error{
			Code: llm.ErrUpstreamError, Message: err.Error(),
			HTTPStatus: http.StatusBadGateway, Provider: p.Name(),
		}
	}
	return &job, nil
}

// CancelFineTuningJob 取消 GLM 微调任务。
// Endpoint: POST /api/paas/v4/fine_tuning/jobs/{job_id}/cancel
func (p *GLMProvider) CancelFineTuningJob(ctx context.Context, jobID string) error {
	endpoint := fmt.Sprintf("%s/api/paas/v4/fine_tuning/jobs/%s/cancel", strings.TrimRight(p.Cfg.BaseURL, "/"), jobID)
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}
	p.ApplyHeaders(httpReq, p.ResolveAPIKey(ctx))

	resp, err := p.Client.Do(httpReq)
	if err != nil {
		return &types.Error{
			Code: llm.ErrUpstreamError, Message: err.Error(),
			HTTPStatus: http.StatusBadGateway, Retryable: true, Provider: p.Name(),
		}
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		msg := providerbase.ReadErrorMessage(resp.Body)
		return providerbase.MapHTTPError(resp.StatusCode, msg, p.Name())
	}
	return nil
}
